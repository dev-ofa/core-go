package model

import (
	"context"

	"github.com/shiningrush/droplet/data"
)

type IDType interface {
	string | int | int64 | uint64
}

type EntityConstraint[P IDType] interface {
	GetID() P
	SetID(P)
}

type Entity[P IDType] struct {
	ID P `bson:"_id,omitempty" json:"id" gorm:"primaryKey"`

	CreateAudit `bson:"inline"  gorm:"embedded"`
}

func (e *Entity[P]) GetID() P {
	return e.ID
}

func (e *Entity[P]) SetID(id P) {
	e.ID = id
}

type OperatorCarrier interface {
	SetUser(user string)
}

type SortAble = data.SortAble

type CtxKey string

const (
	CtxKeyRepoOpt CtxKey = "repo-opt"
)

type Pager struct {
	// PageSize 每页条数
	PageSize int `json:"page_size,omitempty" auto_read:"page_size,query"`
	// PageNumber 页数
	PageNum int `json:"page_num,omitempty" auto_read:"page_num,query"`
	// PageToken 用于Feed流式获取分页数据，正常分页无需
	PageToken string `json:"page_token,omitempty" auto_read:"page_token,query"`
}

func (p *Pager) GetPageInfo() (pageSize, pageNumber int, pageToken string) {
	return p.PageSize, p.PageNum, p.PageToken
}
func (p *Pager) SetPageNumber(pageNumber int) {
	p.PageNum = pageNumber
}
func (p *Pager) InitialDefaultVal() {
	if p.PageSize == 0 {
		p.PageSize = 20
	}
}

type PagedResult[T any] struct {
	Rows       []T `json:"rows"`
	TotalCount int `json:"total_count"`
}

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

type RepoOp func(opt *RepoOpt)

func SetCtxRepoDeployIsolation(ctx context.Context, di DeployIsolation) context.Context {
	exOpt := CtxRepoOptOrNew(ctx)
	exOpt.DeployIsolation = di

	return context.WithValue(ctx, CtxKeyRepoOpt, exOpt)
}

func SetCtxRepoDataIsolation(ctx context.Context, di DataIsolation) context.Context {
	exOpt := CtxRepoOptOrNew(ctx)
	exOpt.DataIsolation = di

	return context.WithValue(ctx, CtxKeyRepoOpt, exOpt)
}

func SetCtxRepoUpdateRunContext(ctx context.Context, urc UpdateRunContext) context.Context {
	exOpt := CtxRepoOptOrNew(ctx)
	exOpt.UpdateRunContext = urc

	return context.WithValue(ctx, CtxKeyRepoOpt, exOpt)
}

func SetCtxSoftDelete(ctx context.Context, sd SoftDelete) context.Context {
	exOpt := CtxRepoOptOrNew(ctx)
	exOpt.SoftDelete = sd

	return context.WithValue(ctx, CtxKeyRepoOpt, exOpt)
}

func SetCtxFixedStrategy(ctx context.Context, fs FixedStrategy) context.Context {
	exOpt := CtxRepoOptOrNew(ctx)
	exOpt.TryFixSyncDelay = fs

	return context.WithValue(ctx, CtxKeyRepoOpt, exOpt)
}

func CtxRepoOptOrNew(ctx context.Context) RepoOpt {
	v := ctx.Value(CtxKeyRepoOpt)
	if v != nil {
		return v.(RepoOpt)
	}
	return RepoOpt{}
}

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

type RepoOpt struct {
	DeployIsolation  DeployIsolation
	DataIsolation    DataIsolation
	UpdateRunContext UpdateRunContext
	TryFixSyncDelay  FixedStrategy
	SoftDelete       SoftDelete
}

type FixedStrategy string

const (
	FixedStrategyNone    FixedStrategy = "none"
	FixedStrategyBackoff FixedStrategy = "backoff"
)

type UpdateRunContext string

const (
	UpdateRunContextAtCreating UpdateRunContext = "at-creating"
	UpdateRunContextAlways     UpdateRunContext = "always"
)

type DeployIsolation string

const (
	DeployIsolationNone    DeployIsolation = "none"
	DeployIsolationCluster DeployIsolation = "cluster"
	DeployIsolationEnv     DeployIsolation = "env"
)

type DataIsolation string

const (
	DataIsolationNone   DataIsolation = "none"
	DataIsolationTenant DataIsolation = "tenant"
	DataIsolationUser   DataIsolation = "user"
	DataIsolationApp    DataIsolation = "app"
)

type SoftDelete string

const (
	SoftDeleteEnable  SoftDelete = ""
	SoftDeleteDisable SoftDelete = "disable"
)
