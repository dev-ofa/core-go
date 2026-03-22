package model

import (
	"context"
)

// IDType defines supported identifier types.
type IDType interface {
	string | int | int64 | uint64
}

// EntityConstraint describes entities that expose GetID/SetID.
type EntityConstraint[P IDType] interface {
	GetID() P
	SetID(P)
}

// Entity is a base model with ID and create audit fields.
type Entity[P IDType] struct {
	// ID is the primary identifier.
	ID P `bson:"_id,omitempty" json:"id" gorm:"primaryKey"`

	// CreateAudit stores creation metadata.
	CreateAudit `bson:"inline"  gorm:"embedded"`
}

// GetID returns the entity id.
func (e *Entity[P]) GetID() P {
	return e.ID
}

// SetID sets the entity id.
func (e *Entity[P]) SetID(id P) {
	e.ID = id
}

// OperatorCarrier allows setting operator for auditing.
type OperatorCarrier interface {
	SetUser(user string)
}

// CtxKey is the context key type for repo options.
type CtxKey string

const (
	// CtxKeyRepoOpt stores RepoOpt in context.
	CtxKeyRepoOpt CtxKey = "repo-opt"
)

// Pager carries pagination input.
type Pager struct {
	// PageSize is the number of items per page.
	PageSize int `json:"page_size,omitempty" auto_read:"page_size,query"`
	// PageNum is the current page number.
	PageNum int `json:"page_num,omitempty" auto_read:"page_num,query"`
	// PageToken is the feed pagination token when using cursor-based paging.
	PageToken string `json:"page_token,omitempty" auto_read:"page_token,query"`
}

// GetPageInfo returns page size, number, and token.
func (p *Pager) GetPageInfo() (pageSize, pageNumber int, pageToken string) {
	return p.PageSize, p.PageNum, p.PageToken
}
// SetPageNumber updates the page number.
func (p *Pager) SetPageNumber(pageNumber int) {
	p.PageNum = pageNumber
}
// InitialDefaultVal sets default paging values when empty.
func (p *Pager) InitialDefaultVal() {
	if p.PageSize == 0 {
		p.PageSize = 20
	}
}

// PagedResult wraps paginated rows and total count.
type PagedResult[T any] struct {
	// Rows is the data slice.
	Rows       []T `json:"rows"`
	// TotalCount is the total number of rows.
	TotalCount int `json:"total_count"`
}

// Repo defines the repository interface for CRUD and batch operations.
type Repo[P IDType, T EntityConstraint[P]] interface {
	Get(ctx context.Context, id P) (ret T, err error)
	Create(ctx context.Context, doc T) (T, error)
	Update(ctx context.Context, doc T) (T, error)
	Upsert(ctx context.Context, doc T) (T, error)
	Patch(ctx context.Context, doc T) error
	Delete(ctx context.Context, doc T) error
	// BatchCreate 请注意该接口不保证事务性
	BatchCreate(ctx context.Context, docs []T) error
	BatchUpdate(ctx context.Context, docs []T) error
	BatchDelete(ctx context.Context, docs []T) (int, error)
	BatchDeleteByIDs(ctx context.Context, ids []P) (int, error)
}

// RepoOp is a functional option for RepoOpt.
type RepoOp func(opt *RepoOpt)

// SetCtxRepoDeployIsolation sets deploy isolation in context.
func SetCtxRepoDeployIsolation(ctx context.Context, di DeployIsolation) context.Context {
	exOpt := CtxRepoOptOrNew(ctx)
	exOpt.DeployIsolation = di

	return context.WithValue(ctx, CtxKeyRepoOpt, exOpt)
}

// SetCtxRepoDataIsolation sets data isolation in context.
func SetCtxRepoDataIsolation(ctx context.Context, di DataIsolation) context.Context {
	exOpt := CtxRepoOptOrNew(ctx)
	exOpt.DataIsolation = di

	return context.WithValue(ctx, CtxKeyRepoOpt, exOpt)
}

// SetCtxRepoUpdateRunContext sets update run context in context.
func SetCtxRepoUpdateRunContext(ctx context.Context, urc UpdateRunContext) context.Context {
	exOpt := CtxRepoOptOrNew(ctx)
	exOpt.UpdateRunContext = urc

	return context.WithValue(ctx, CtxKeyRepoOpt, exOpt)
}

// SetCtxSoftDelete sets soft delete behavior in context.
func SetCtxSoftDelete(ctx context.Context, sd SoftDelete) context.Context {
	exOpt := CtxRepoOptOrNew(ctx)
	exOpt.SoftDelete = sd

	return context.WithValue(ctx, CtxKeyRepoOpt, exOpt)
}

// SetCtxFixedStrategy sets the sync-delay fix strategy in context.
func SetCtxFixedStrategy(ctx context.Context, fs FixedStrategy) context.Context {
	exOpt := CtxRepoOptOrNew(ctx)
	exOpt.TryFixSyncDelay = fs

	return context.WithValue(ctx, CtxKeyRepoOpt, exOpt)
}

// CtxRepoOptOrNew gets RepoOpt from context or returns default.
func CtxRepoOptOrNew(ctx context.Context) RepoOpt {
	v := ctx.Value(CtxKeyRepoOpt)
	if v != nil {
		return v.(RepoOpt)
	}
	return RepoOpt{}
}

// CtxMergeRepoOpt merges context repo options with explicit options.
func CtxMergeRepoOpt(ctx context.Context, merge *RepoOpt) *RepoOpt {
	ctxOpt := CtxRepoOptOrNew(ctx)

	if merge == nil {
		return &ctxOpt
	}

	if ctxOpt.DataIsolation == "" {
		ctxOpt.DataIsolation = merge.DataIsolation
	}

	if ctxOpt.DeployIsolation == "" {
		ctxOpt.DeployIsolation = merge.DeployIsolation
	}

	if ctxOpt.UpdateRunContext == "" {
		ctxOpt.UpdateRunContext = merge.UpdateRunContext
	}

	if ctxOpt.SoftDelete == "" {
		ctxOpt.SoftDelete = merge.SoftDelete
	}

	if ctxOpt.TryFixSyncDelay == "" {
		ctxOpt.TryFixSyncDelay = merge.TryFixSyncDelay
	}

	return &ctxOpt
}

// RepoOpt carries repository behavior flags.
type RepoOpt struct {
	// DeployIsolation controls deployment isolation scope.
	DeployIsolation  DeployIsolation
	// DataIsolation controls data isolation scope.
	DataIsolation    DataIsolation
	// UpdateRunContext controls audit update timing.
	UpdateRunContext UpdateRunContext
	// TryFixSyncDelay controls sync delay fix strategy.
	TryFixSyncDelay  FixedStrategy
	// SoftDelete controls soft delete behavior.
	SoftDelete       SoftDelete
}

// FixedStrategy defines the sync delay fix strategy.
type FixedStrategy string

const (
	// FixedStrategyNone disables fix behavior.
	FixedStrategyNone    FixedStrategy = "none"
	// FixedStrategyBackoff uses backoff to fix sync delay.
	FixedStrategyBackoff FixedStrategy = "backoff"
)

// UpdateRunContext defines when audit updates are applied.
type UpdateRunContext string

const (
	// UpdateRunContextAtCreating applies updates only at creation.
	UpdateRunContextAtCreating UpdateRunContext = "at-creating"
	// UpdateRunContextAlways applies updates on every update.
	UpdateRunContextAlways     UpdateRunContext = "always"
)

// DeployIsolation defines deployment isolation levels.
type DeployIsolation string

const (
	// DeployIsolationNone disables deployment isolation.
	DeployIsolationNone    DeployIsolation = "none"
	// DeployIsolationCluster isolates by cluster.
	DeployIsolationCluster DeployIsolation = "cluster"
	// DeployIsolationEnv isolates by environment.
	DeployIsolationEnv     DeployIsolation = "env"
)

// DataIsolation defines data isolation levels.
type DataIsolation string

const (
	// DataIsolationNone disables data isolation.
	DataIsolationNone   DataIsolation = "none"
	// DataIsolationTenant isolates by tenant.
	DataIsolationTenant DataIsolation = "tenant"
	// DataIsolationUser isolates by user.
	DataIsolationUser   DataIsolation = "user"
	// DataIsolationApp isolates by app.
	DataIsolationApp    DataIsolation = "app"
)

// SoftDelete defines soft delete behavior.
type SoftDelete string

const (
	// SoftDeleteEnable enables soft delete by default.
	SoftDeleteEnable  SoftDelete = ""
	// SoftDeleteDisable disables soft delete.
	SoftDeleteDisable SoftDelete = "disable"
)
