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
	// HeaderLocale is the full-link locale header defined by the i18n spec.
	HeaderLocale = trace.HeaderLocale
	// HeaderRequestID is the single-hop request header defined by the tracing spec.
	HeaderRequestID = trace.HeaderRequestID
	// HeaderRemainingTimeoutMS is the single-hop timeout budget header.
	HeaderRemainingTimeoutMS = trace.HeaderRemainingTimeoutMS

	acceptLanguageHeader = "Accept-Language"
	defaultLocale        = "zh-CN"
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
	ctx = pass.CtxSetLocale(ctx, resolveLocale(header))

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
		deadline := time.Now().Add(timeout)
		return contextWithAuthoritativeDeadline(ctx, deadline)
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
	if deadline, ok := authoritativeDeadline(ctx); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return ctx, requestID, ErrTimeoutBudgetExhausted
		}
		remainingMS := strconv.FormatInt(remaining.Milliseconds(), 10)
		req.Header.Set(HeaderRemainingTimeoutMS, remainingMS)
	}
	return ctx, requestID, nil
}

func contextWithPassHeaders(ctx context.Context, header http.Header) context.Context {
	for key, vals := range header {
		if !strings.HasPrefix(strings.ToLower(key), "ofa-pass-") || len(vals) == 0 {
			continue
		}
		ctx = pass.CtxSetPassVal(ctx, key, vals[0])
	}
	return ctx
}

func resolveLocale(header http.Header) string {
	if locale := normalizeLocale(header.Get(HeaderLocale)); locale != "" {
		return locale
	}
	acceptLanguage := strings.TrimSpace(header.Get(acceptLanguageHeader))
	for _, part := range strings.Split(acceptLanguage, ",") {
		candidate := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if locale := normalizeLocale(candidate); locale != "" {
			return locale
		}
	}
	return defaultLocale
}

func normalizeLocale(value string) string {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), "_", "-"))
	switch normalized {
	case "zh", "zh-cn", "zh-hans", "zh-hans-cn":
		return "zh-CN"
	case "en", "en-us":
		return "en-US"
	default:
		return ""
	}
}

func authoritativeDeadline(ctx context.Context) (time.Time, bool) {
	if deadline, ok := pass.CtxGetRequestDeadline(ctx); ok {
		return deadline, true
	}
	if deadline, ok := ctx.Deadline(); ok {
		return deadline, true
	}
	return time.Time{}, false
}

func contextWithAuthoritativeDeadline(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	if existing, ok := ctx.Deadline(); ok && existing.Before(deadline) {
		deadline = existing
	}
	ctx = pass.CtxSetRequestDeadline(ctx, deadline)
	return context.WithDeadline(ctx, deadline)
}
