package pass

import (
	"context"
	"strings"
)

const (
	KeyTraceID   = "OFA_TRACE_ID"
	KeyRequestID = "OFA_REQUEST_ID"
	KeyOperator  = "OFA_OPERATOR"
	KeyTenantID  = "OFA_TENANT_ID"
	KeyAppID     = "OFA_APP_ID"
)

func CtxGetTraceID(ctx context.Context) (string, bool) {
	val, ok := CtxGetPassVal(ctx, KeyTraceID)
	return val, ok
}

func CtxSetTraceID(ctx context.Context, val string) context.Context {
	return CtxSetPassVal(ctx, KeyTraceID, val)
}

func CtxGetRequestID(ctx context.Context) (string, bool) {
	val, ok := CtxGetDirectVal(ctx, KeyRequestID)
	return val, ok
}

func CtxSetRequestID(ctx context.Context, val string) context.Context {
	return CtxSetDirectVal(ctx, KeyRequestID, val)
}

func CtxGetOperator(ctx context.Context) (string, bool) {
	val, ok := CtxGetPassVal(ctx, KeyOperator)
	return val, ok
}

func CtxSetOperator(ctx context.Context, val string) context.Context {
	return CtxSetPassVal(ctx, KeyOperator, val)
}

func CtxGetTenantID(ctx context.Context) (string, bool) {
	val, ok := CtxGetPassVal(ctx, KeyTenantID)
	return val, ok
}

func CtxSetTenantID(ctx context.Context, val string) context.Context {
	return CtxSetPassVal(ctx, KeyTenantID, val)
}

func CtxGetAppID(ctx context.Context) (string, bool) {
	val, ok := CtxGetPassVal(ctx, KeyAppID)
	return val, ok
}

func CtxSetAppID(ctx context.Context, val string) context.Context {
	return CtxSetPassVal(ctx, KeyAppID, val)
}

func CtxGetPassVal(ctx context.Context, key string) (string, bool) {
	val, ok := ctx.Value(FixedKey(key)).(string)
	return val, ok
}

func CtxSetPassVal(ctx context.Context, key string, val string) context.Context {
	return context.WithValue(ctx, FixedKey(key), val)
}

func CtxGetDirectVal(ctx context.Context, key string) (string, bool) {
	val, ok := ctx.Value(FixedKeyDirect(key)).(string)
	return val, ok
}

func CtxSetDirectVal(ctx context.Context, key string, val string) context.Context {
	return context.WithValue(ctx, FixedKeyDirect(key), val)
}

func FixedKey(key string) string {
	return "OFA_PASS_" + strings.ToUpper(key)
}

func FixedKeyDirect(key string) string {
	return "OFA_DIRECT_" + strings.ToUpper(key)
}
