package pass

import (
	"context"
	"testing"

	"github.com/dev-ofa/core-go/trace"
	"github.com/stretchr/testify/require"
)

func TestTraceContextKeysUseStandardHeaders(t *testing.T) {
	ctx := context.Background()
	ctx = CtxSetTraceID(ctx, "trace-1")
	ctx = CtxSetRequestID(ctx, "request-1")
	ctx = CtxSetRemainingTimeoutMS(ctx, "3000")
	ctx = CtxSetOperator(ctx, "operator-1")
	ctx = CtxSetTenantID(ctx, "tenant-1")
	ctx = CtxSetAppID(ctx, "app-1")

	traceID, ok := CtxGetTraceID(ctx)
	require.True(t, ok)
	require.Equal(t, "trace-1", traceID)
	requestID, ok := CtxGetRequestID(ctx)
	require.True(t, ok)
	require.Equal(t, "request-1", requestID)
	remainingTimeout, ok := CtxGetRemainingTimeoutMS(ctx)
	require.True(t, ok)
	require.Equal(t, "3000", remainingTimeout)
	operator, ok := CtxGetOperator(ctx)
	require.True(t, ok)
	require.Equal(t, "operator-1", operator)
	tenantID, ok := CtxGetTenantID(ctx)
	require.True(t, ok)
	require.Equal(t, "tenant-1", tenantID)
	appID, ok := CtxGetAppID(ctx)
	require.True(t, ok)
	require.Equal(t, "app-1", appID)

	require.Equal(t, "trace-1", ctx.Value(trace.HeaderTraceID))
	require.Equal(t, "request-1", ctx.Value(trace.HeaderRequestID))
	require.Equal(t, "3000", ctx.Value(trace.HeaderRemainingTimeoutMS))
}

func TestPassHeadersEnumeration(t *testing.T) {
	ctx := context.Background()
	ctx = CtxSetPassVal(ctx, KeyTraceID, "trace-1")
	ctx = CtxSetPassVal(ctx, "OFA_PASS_FEATURE_FLAG", "gray")

	headers := CtxPassHeaders(ctx)
	require.Equal(t, map[string]string{
		trace.HeaderTraceID:     "trace-1",
		"OFA_PASS_FEATURE_FLAG": "gray",
	}, headers)

	headers["OFA_PASS_FEATURE_FLAG"] = "mutated"
	again := CtxPassHeaders(ctx)
	require.Equal(t, "gray", again["OFA_PASS_FEATURE_FLAG"])
}

func TestPassValKeepsDirectKeysCompatible(t *testing.T) {
	ctx := context.Background()
	ctx = CtxSetPassVal(ctx, KeyRequestID, "request-1")
	ctx = CtxSetPassVal(ctx, KeyRemainingTimeoutMS, "3000")

	requestID, ok := CtxGetRequestID(ctx)
	require.True(t, ok)
	require.Equal(t, "request-1", requestID)
	remainingTimeout, ok := CtxGetRemainingTimeoutMS(ctx)
	require.True(t, ok)
	require.Equal(t, "3000", remainingTimeout)

	headers := CtxPassHeaders(ctx)
	_, exists := headers[trace.HeaderRequestID]
	require.False(t, exists)
	_, exists = headers[trace.HeaderRemainingTimeoutMS]
	require.False(t, exists)
}

func TestFixedKeyAvoidsDoublePrefix(t *testing.T) {
	require.Equal(t, "TRACE_ID", KeyTraceID)
	require.Equal(t, "REQUEST_ID", KeyRequestID)
	require.Equal(t, "REMAINING_TIMEOUT_MS", KeyRemainingTimeoutMS)
	require.Equal(t, trace.HeaderTraceID, FixedKey("TRACE_ID"))
	require.Equal(t, trace.HeaderTraceID, FixedKey("OFA_TRACE_ID"))
	require.Equal(t, trace.HeaderTraceID, FixedKey(trace.HeaderTraceID))
	require.Equal(t, trace.HeaderRequestID, FixedKeyDirect("REQUEST_ID"))
	require.Equal(t, trace.HeaderRequestID, FixedKeyDirect("OFA_REQUEST_ID"))
	require.Equal(t, trace.HeaderRequestID, FixedKeyDirect(trace.HeaderRequestID))
	require.Equal(t, trace.HeaderRemainingTimeoutMS, FixedKeyDirect("OFA_REMAINING_TIMEOUT_MS"))
}
