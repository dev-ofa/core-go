package pass

import (
	"context"
	"maps"
	"strings"
	"time"

	"github.com/dev-ofa/core-go/trace"
)

// KeyTraceID and related keys are the standard OFA string keys.
const (
	KeyTraceID            = trace.HeaderTraceID
	KeyRequestID          = trace.HeaderRequestID
	KeyRemainingTimeoutMS = trace.HeaderRemainingTimeoutMS
	KeyRequestDeadline    = "ofa-request-deadline"
	KeyOperator           = trace.HeaderOperator
	KeyTenantID           = trace.HeaderTenantID
	KeyAppID              = trace.HeaderAppID
	KeyLocale             = trace.HeaderLocale
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

// CtxGetRemainingTimeoutMS reads the direct remaining-timeout header value from context.
func CtxGetRemainingTimeoutMS(ctx context.Context) (string, bool) {
	val, ok := CtxGetDirectVal(ctx, KeyRemainingTimeoutMS)
	return val, ok
}

// CtxSetRemainingTimeoutMS writes the direct remaining-timeout header value into context.
func CtxSetRemainingTimeoutMS(ctx context.Context, val string) context.Context {
	return CtxSetDirectVal(ctx, KeyRemainingTimeoutMS, val)
}

// CtxGetRequestDeadline reads the standard in-process authoritative deadline.
func CtxGetRequestDeadline(ctx context.Context) (time.Time, bool) {
	val, ok := ctx.Value(FixedKeyValue(KeyRequestDeadline)).(time.Time)
	return val, ok && !val.IsZero()
}

// CtxSetRequestDeadline writes the standard in-process authoritative deadline.
func CtxSetRequestDeadline(ctx context.Context, val time.Time) context.Context {
	return context.WithValue(ctx, FixedKeyValue(KeyRequestDeadline), val)
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

// CtxGetLocale reads locale from context.
func CtxGetLocale(ctx context.Context) (string, bool) {
	val, ok := CtxGetPassVal(ctx, KeyLocale)
	return val, ok
}

// CtxSetLocale writes locale into context.
func CtxSetLocale(ctx context.Context, val string) context.Context {
	return CtxSetPassVal(ctx, KeyLocale, val)
}

// CtxGetPassVal reads a value with pass prefix.
func CtxGetPassVal(ctx context.Context, key string) (string, bool) {
	val, ok := ctx.Value(FixedKey(key)).(string)
	return val, ok
}

// CtxSetPassVal writes a value with pass prefix.
func CtxSetPassVal(ctx context.Context, key string, val string) context.Context {
	fixedKey := FixedKey(key)
	ctx = context.WithValue(ctx, fixedKey, val)
	headers := CtxPassHeaders(ctx)
	headers[fixedKey] = val
	return context.WithValue(ctx, passHeadersContextKey, headers)
}

// CtxPassHeaders returns all ofa-pass-* headers stored through CtxSetPassVal.
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

// FixedKeyValue builds a standard in-process OFA string ContextKey.
func FixedKeyValue(key string) string {
	key = normalizeKey(key)
	if strings.HasPrefix(key, "ofa-") {
		return key
	}
	return "ofa-" + key
}

// FixedKey builds the ofa-pass-* key.
func FixedKey(key string) string {
	key = normalizeKey(key)
	if strings.HasPrefix(key, "ofa-pass-") {
		return key
	}
	key = strings.TrimPrefix(key, "ofa-")
	return "ofa-pass-" + key
}

// FixedKeyDirect builds the ofa-direct-* key.
func FixedKeyDirect(key string) string {
	key = normalizeKey(key)
	if strings.HasPrefix(key, "ofa-direct-") {
		return key
	}
	key = strings.TrimPrefix(key, "ofa-")
	return "ofa-direct-" + key
}

func normalizeKey(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	return strings.ReplaceAll(key, "_", "-")
}
