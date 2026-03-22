package pass

import (
	"context"
	"strings"
)

// KeyTraceID and related keys are context keys for tracing and tenancy values.
const (
	KeyTraceID   = "OFA_TRACE_ID"
	KeyRequestID = "OFA_REQUEST_ID"
	KeyOperator  = "OFA_OPERATOR"
	KeyTenantID  = "OFA_TENANT_ID"
	KeyAppID     = "OFA_APP_ID"
)

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
	val, ok := ctx.Value(FixedKey(key)).(string)
	return val, ok
}

// CtxSetPassVal writes a value with pass prefix.
func CtxSetPassVal(ctx context.Context, key string, val string) context.Context {
	return context.WithValue(ctx, FixedKey(key), val)
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
	return "OFA_PASS_" + strings.ToUpper(key)
}

// FixedKeyDirect builds the direct prefixed key.
func FixedKeyDirect(key string) string {
	return "OFA_DIRECT_" + strings.ToUpper(key)
}
