package pass

import (
	"context"
	"maps"
	"strings"
)

// KeyTraceID and related keys are context keys for tracing and tenancy values.
const (
	KeyTraceID            = "TRACE_ID"
	KeyRequestID          = "REQUEST_ID"
	KeyRemainingTimeoutMS = "REMAINING_TIMEOUT_MS"
	KeyOperator           = "OPERATOR"
	KeyTenantID           = "TENANT_ID"
	KeyAppID              = "APP_ID"
)

type contextKey string

const passHeadersContextKey contextKey = "ofa_pass_headers"

// CtxGetTraceID reads trace id from context.
func CtxGetTraceID(ctx context.Context) (string, bool) {
	val, ok := CtxGetPassVal(ctx, KeyTraceID)
	return val, ok
}

// CtxSetTraceID writes trace id into context.
func CtxSetTraceID(ctx context.Context, val string) context.Context {
	return CtxSetPassVal(ctx, KeyTraceID, val)
}

// CtxGetRequestID reads request id from context.
func CtxGetRequestID(ctx context.Context) (string, bool) {
	val, ok := CtxGetDirectVal(ctx, KeyRequestID)
	return val, ok
}

// CtxSetRequestID writes request id into context.
func CtxSetRequestID(ctx context.Context, val string) context.Context {
	return CtxSetDirectVal(ctx, KeyRequestID, val)
}

// CtxGetRemainingTimeoutMS reads remaining timeout in milliseconds from context.
func CtxGetRemainingTimeoutMS(ctx context.Context) (string, bool) {
	val, ok := CtxGetDirectVal(ctx, KeyRemainingTimeoutMS)
	return val, ok
}

// CtxSetRemainingTimeoutMS writes remaining timeout in milliseconds into context.
func CtxSetRemainingTimeoutMS(ctx context.Context, val string) context.Context {
	return CtxSetDirectVal(ctx, KeyRemainingTimeoutMS, val)
}

// CtxGetOperator reads operator id from context.
func CtxGetOperator(ctx context.Context) (string, bool) {
	val, ok := CtxGetPassVal(ctx, KeyOperator)
	return val, ok
}

// CtxSetOperator writes operator id into context.
func CtxSetOperator(ctx context.Context, val string) context.Context {
	return CtxSetPassVal(ctx, KeyOperator, val)
}

// CtxGetTenantID reads tenant id from context.
func CtxGetTenantID(ctx context.Context) (string, bool) {
	val, ok := CtxGetPassVal(ctx, KeyTenantID)
	return val, ok
}

// CtxSetTenantID writes tenant id into context.
func CtxSetTenantID(ctx context.Context, val string) context.Context {
	return CtxSetPassVal(ctx, KeyTenantID, val)
}

// CtxGetAppID reads app id from context.
func CtxGetAppID(ctx context.Context) (string, bool) {
	val, ok := CtxGetPassVal(ctx, KeyAppID)
	return val, ok
}

// CtxSetAppID writes app id into context.
func CtxSetAppID(ctx context.Context, val string) context.Context {
	return CtxSetPassVal(ctx, KeyAppID, val)
}

// CtxGetPassVal reads a value with pass prefix.
func CtxGetPassVal(ctx context.Context, key string) (string, bool) {
	if isDirectKey(key) {
		return CtxGetDirectVal(ctx, key)
	}
	val, ok := ctx.Value(FixedKey(key)).(string)
	return val, ok
}

// CtxSetPassVal writes a value with pass prefix.
func CtxSetPassVal(ctx context.Context, key string, val string) context.Context {
	if isDirectKey(key) {
		return CtxSetDirectVal(ctx, key, val)
	}
	fixedKey := FixedKey(key)
	ctx = context.WithValue(ctx, fixedKey, val)
	headers := CtxPassHeaders(ctx)
	headers[fixedKey] = val
	return context.WithValue(ctx, passHeadersContextKey, headers)
}

// CtxPassHeaders returns all OFA_PASS headers stored through CtxSetPassVal.
func CtxPassHeaders(ctx context.Context) map[string]string {
	headers, _ := ctx.Value(passHeadersContextKey).(map[string]string)
	ret := make(map[string]string, len(headers))
	maps.Copy(ret, headers)
	return ret
}

// CtxGetDirectVal reads a value with direct prefix.
func CtxGetDirectVal(ctx context.Context, key string) (string, bool) {
	val, ok := ctx.Value(FixedKeyDirect(key)).(string)
	return val, ok
}

// CtxSetDirectVal writes a value with direct prefix.
func CtxSetDirectVal(ctx context.Context, key string, val string) context.Context {
	return context.WithValue(ctx, FixedKeyDirect(key), val)
}

// FixedKey builds the pass prefixed key.
func FixedKey(key string) string {
	key = strings.ToUpper(key)
	if strings.HasPrefix(key, "OFA_PASS_") {
		return key
	}
	key = strings.TrimPrefix(key, "OFA_")
	return "OFA_PASS_" + key
}

// FixedKeyDirect builds the direct prefixed key.
func FixedKeyDirect(key string) string {
	key = strings.ToUpper(key)
	if strings.HasPrefix(key, "OFA_DIRECT_") {
		return key
	}
	key = strings.TrimPrefix(key, "OFA_")
	return "OFA_DIRECT_" + key
}

func isDirectKey(key string) bool {
	switch strings.TrimPrefix(strings.ToUpper(key), "OFA_") {
	case KeyRequestID, KeyRemainingTimeoutMS:
		return true
	default:
		return false
	}
}
