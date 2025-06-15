package model

import (
	"context"

	"github.com/dev-ofa/core-go/pass"
)

func NewByteReqInfo() *ByteReqInfo {
	return &ByteReqInfo{
		UID:       "vinci",
		TID:       "tenant",
		AID:       "app",
		TraceID:   "trace_id",
		RequestID: "request_id",
	}
}

type ByteReqInfo struct {
	UID       string
	TID       string
	AID       string
	TraceID   string
	RequestID string
}

func GenUserInfoContext(info *ByteReqInfo) context.Context {
	ctx := context.TODO()
	ctx = pass.CtxSetTraceID(ctx, info.TraceID)
	ctx = pass.CtxSetRequestID(ctx, info.RequestID)
	ctx = pass.CtxSetOperator(ctx, info.UID)
	ctx = pass.CtxSetTenantID(ctx, info.TID)
	ctx = pass.CtxSetAppID(ctx, info.AID)

	return ctx
}
