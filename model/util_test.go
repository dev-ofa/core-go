package model

import (
	"context"
	"testing"

	"github.com/dev-ofa/core-go/pass"
)

type updateAuditDoc struct {
	UpdateAudit
}

func TestUpdateLockAndAuditAllowsNilRepoOpt(t *testing.T) {
	ctx := pass.CtxSetOperator(context.Background(), "vinci")
	doc := &updateAuditDoc{}

	ret, err := UpdateLockAndAudit(ctx, doc, nil)
	if err != nil {
		t.Fatalf("update lock and audit: %v", err)
	}
	if ret == nil {
		t.Fatalf("updates result should not be nil")
	}
	if doc.UpdatedBy != "vinci" {
		t.Fatalf("updated by want vinci got %s", doc.UpdatedBy)
	}
	if doc.UpdatedAt.IsZero() {
		t.Fatalf("updated at should be set")
	}
}
