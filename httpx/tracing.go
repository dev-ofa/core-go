package httpx

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dev-ofa/core-go/pass"
	"github.com/dev-ofa/core-go/trace"
)

const (
	// HeaderTraceID is the full-link trace header defined by the tracing spec.
	HeaderTraceID = trace.HeaderTraceID
	// HeaderOperator is the full-link operator header defined by the tracing spec.
	HeaderOperator = trace.HeaderOperator
	// HeaderTenantID is the full-link tenant header defined by the tracing spec.
	HeaderTenantID = trace.HeaderTenantID
	// HeaderAppID is the full-link application header defined by the tracing spec.
	HeaderAppID = trace.HeaderAppID
	// HeaderRequestID is the single-hop request header defined by the tracing spec.
	HeaderRequestID = trace.HeaderRequestID
	// HeaderRemainingTimeoutMS is the single-hop timeout budget header.
	HeaderRemainingTimeoutMS = trace.HeaderRemainingTimeoutMS
)

// ContextFromHeaders rebuilds trace values and the local authoritative deadline
// from inbound HTTP headers. Callers must call the returned cancel function.
func ContextFromHeaders(ctx context.Context, header http.Header, defaultTimeout time.Duration, maxTimeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = contextWithPassHeaders(ctx, header)
	if requestID := header.Get(HeaderRequestID); requestID != "" {
		ctx = pass.CtxSetRequestID(ctx, requestID)
	}

	timeout := defaultTimeout
	if raw := header.Get(HeaderRemainingTimeoutMS); raw != "" {
		if ms, err := strconv.ParseInt(raw, 10, 64); err == nil && ms > 0 {
			timeout = time.Duration(ms) * time.Millisecond
		}
	}
	if maxTimeout > 0 && (timeout == 0 || timeout > maxTimeout) {
		timeout = maxTimeout
	}
	if timeout > 0 {
		ctx = pass.CtxSetRemainingTimeoutMS(ctx, strconv.FormatInt(timeout.Milliseconds(), 10))
		deadline := time.Now().Add(timeout)
		return context.WithDeadline(ctx, deadline)
	}
	return ctx, func() {}
}

func ensureTraceContext(ctx context.Context) (context.Context, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if traceID, ok := pass.CtxGetTraceID(ctx); ok && traceID != "" {
		return ctx, traceID, nil
	}
	traceID, err := trace.NewTraceID()
	if err != nil {
		return ctx, "", err
	}
	return pass.CtxSetTraceID(ctx, traceID), traceID, nil
}

func injectTraceHeaders(ctx context.Context, req *http.Request) (context.Context, string, error) {
	ctx, traceID, err := ensureTraceContext(ctx)
	if err != nil {
		return ctx, "", err
	}
	requestID, err := trace.NewRequestID()
	if err != nil {
		return ctx, "", err
	}
	ctx = pass.CtxSetRequestID(ctx, requestID)

	for key, val := range pass.CtxPassHeaders(ctx) {
		req.Header.Set(key, val)
	}
	req.Header.Set(HeaderTraceID, traceID)
	req.Header.Set(HeaderRequestID, requestID)
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return ctx, requestID, ErrTimeoutBudgetExhausted
		}
		remainingMS := strconv.FormatInt(remaining.Milliseconds(), 10)
		ctx = pass.CtxSetRemainingTimeoutMS(ctx, remainingMS)
		req.Header.Set(HeaderRemainingTimeoutMS, remainingMS)
	}
	return ctx, requestID, nil
}

func contextWithPassHeaders(ctx context.Context, header http.Header) context.Context {
	for key, vals := range header {
		if !strings.HasPrefix(strings.ToUpper(key), "OFA_PASS_") || len(vals) == 0 {
			continue
		}
		ctx = pass.CtxSetPassVal(ctx, key, vals[0])
	}
	return ctx
}
