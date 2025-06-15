package model

import (
	"context"
	"fmt"
	"time"
)

type UpdatesResult struct {
	HasOriginalUpdate bool
	// OriginalUpdatedAt may be time.Time or int64
	OriginalUpdatedAt any
}

func UpdateLockAndAudit[T any](ctx context.Context, doc T, opt *RepoOpt) (*UpdatesResult, error) {
	var originalUpdated time.Time
	var originalRawUpdated any
	if ur, ok := interface{}(doc).(UpdateAuditor); ok {
		_, originalUpdated = ur.GetUpdateInfo()
		originalRawUpdated = ur.GetUpdateTimeRaw()
	}

	switch opt.UpdateRunContext {
	case UpdateRunContextAlways:
		if err := CtxUpdateAuditAndEnv(ctx, doc); err != nil {
			return nil, fmt.Errorf("audit env failed: %w", err)
		}
	default:
		if err := CtxUpdateAudit(ctx, doc); err != nil {
			return nil, fmt.Errorf("audit failed: %w", err)
		}
	}

	var ret UpdatesResult
	if !originalUpdated.IsZero() {
		ret.HasOriginalUpdate = true
		ret.OriginalUpdatedAt = originalRawUpdated
	}

	return &ret, nil
}
