package model

import (
	"context"
	"fmt"
	"time"

	"github.com/dev-ofa/core-go/pass"
	"github.com/shiningrush/goext/timex"
)

func CtxCreateAudit(ctx context.Context, entity any) error {
	if cr, ok := entity.(CreateAuditor); ok {
		u, ok := pass.CtxGetOperator(ctx)
		if !ok {
			return fmt.Errorf("there is no user in context")
		}
		cr.SetCreator(u)
	}

	if tc, ok := entity.(TenantCarrier); ok {
		tId, ok := pass.CtxGetTenantID(ctx)
		if !ok {
			return fmt.Errorf("there is no tenantid in context")
		}
		tc.SetTenantID(tId)

		aId, ok := pass.CtxGetAppID(ctx)
		if !ok {
			return fmt.Errorf("there is no appid in context")
		}
		tc.SetAppID(aId)
	}

	return CtxUpdateAudit(ctx, entity)
}

func CtxUpdateAudit(ctx context.Context, entity any) error {
	if uc, ok := entity.(UpdateAuditor); ok {
		u, ok := pass.CtxGetOperator(ctx)
		if !ok {
			return fmt.Errorf("there is no user in context")
		}
		uc.SetUpdater(u)
	}

	return nil
}

func CtxUpdateAuditAndEnv(ctx context.Context, entity any) error {
	if err := CtxUpdateAudit(ctx, entity); err != nil {
		return err
	}

	return nil
}

func CtxDeleteAudit(ctx context.Context, entity any) (bool, error) {
	if uc, ok := entity.(DeleteAuditor); ok {
		u, ok := pass.CtxGetOperator(ctx)
		if !ok {
			return false, fmt.Errorf("there is no user in context")
		}
		uc.SetDeleter(u)
		return true, nil
	}

	return false, nil
}

func CtxAudit(ctx context.Context, auditors []any) error {
	for _, v := range auditors {
		if op, ok := v.(OperatorCarrier); ok {
			u, ok := pass.CtxGetOperator(ctx)
			if !ok {
				return fmt.Errorf("there is no user in context")
			}
			op.SetUser(u)
		}

		if tc, ok := v.(TenantCarrier); ok {
			tId, ok := pass.CtxGetTenantID(ctx)
			if !ok {
				return fmt.Errorf("there is no tenantid in context")
			}
			tc.SetTenantID(tId)

			aId, ok := pass.CtxGetAppID(ctx)
			if !ok {
				return fmt.Errorf("there is no appid in context")
			}
			tc.SetAppID(aId)
		}
	}
	return nil
}

var _ CreateAuditor = (*CreateAudit)(nil)

type CreateAuditor interface {
	GetCreatorInfo() (string, time.Time)
	SetCreator(user string)
	GetCreateTimeRaw() interface{}
}

type CreateAudit struct {
	CreatedAt time.Time `bson:"created_at,omitempty" json:"created_at,omitempty" gorm:"created_at;type:datetime(3);autoUpdateTime:false"`
	CreatedBy string    `bson:"created_by,omitempty" json:"created_by,omitempty" gorm:"created_by;type:varchar(255)"`
}

func (c *CreateAudit) GetCreateTimeRaw() interface{} {
	return c.CreatedAt
}

func (c *CreateAudit) GetCreatorInfo() (string, time.Time) {
	return c.CreatedBy, c.CreatedAt
}

func (c *CreateAudit) SetCreator(user string) {
	c.SetUser(user)
}

func (c *CreateAudit) SetUser(user string) {
	c.CreatedBy = user
	c.CreatedAt = timex.Now()
}

var _ CreateAuditor = (*CreateAuditMs)(nil)

type CreateAuditMs struct {
	CreatedAt int64  `bson:"created_at,omitempty" json:"created_at,omitempty" gorm:"created_at;type:bigint unsigned;autoUpdateTime:false"`
	CreatedBy string `bson:"created_by,omitempty" json:"created_by,omitempty" gorm:"created_by;type:varchar(255)"`
}

func (c *CreateAuditMs) GetCreateTimeRaw() interface{} {
	return c.CreatedAt
}

func (c *CreateAuditMs) GetCreatorInfo() (string, time.Time) {
	if c.CreatedAt == 0 {
		return c.CreatedBy, time.Time{}
	}
	return c.CreatedBy, time.UnixMilli(c.CreatedAt)
}

func (c *CreateAuditMs) SetCreator(user string) {
	c.SetUser(user)
}

func (c *CreateAuditMs) SetUser(user string) {
	c.CreatedBy = user
	c.CreatedAt = timex.Now().UnixMilli()
}

type UpdateAuditor interface {
	GetUpdateInfo() (string, time.Time)
	SetUpdater(user string)
	GetUpdateTimeRaw() interface{}
}

var _ UpdateAuditor = (*UpdateAudit)(nil)

type UpdateAudit struct {
	UpdatedAt time.Time `bson:"updated_at,omitempty" json:"updated_at,omitempty" gorm:"updated_at;type:datetime(3);autoUpdateTime:false"`
	UpdatedBy string    `bson:"updated_by,omitempty" json:"updated_by,omitempty" gorm:"updated_by;type:varchar(255)"`
}

func (c *UpdateAudit) GetUpdateTimeRaw() interface{} {
	return c.UpdatedAt
}

func (c *UpdateAudit) GetUpdateInfo() (string, time.Time) {
	return c.UpdatedBy, c.UpdatedAt
}

func (c *UpdateAudit) SetUpdater(user string) {
	c.SetUser(user)
}

func (c *UpdateAudit) SetUser(user string) {
	c.UpdatedBy = user
	c.UpdatedAt = timex.Now()
}

var _ UpdateAuditor = (*UpdateAuditMs)(nil)

type UpdateAuditMs struct {
	UpdatedAt int64  `bson:"updated_at,omitempty" json:"updated_at,omitempty" gorm:"updated_at;type:bigint unsigned;autoUpdateTime:false"`
	UpdatedBy string `bson:"updated_by,omitempty" json:"updated_by,omitempty" gorm:"updated_by;type:varchar(255)"`
}

func (c *UpdateAuditMs) GetUpdateTimeRaw() interface{} {
	return c.UpdatedAt
}

func (c *UpdateAuditMs) GetUpdateInfo() (string, time.Time) {
	if c.UpdatedAt == 0 {
		return c.UpdatedBy, time.Time{}
	}
	return c.UpdatedBy, time.UnixMilli(c.UpdatedAt)
}

func (c *UpdateAuditMs) SetUpdater(user string) {
	c.SetUser(user)
}

func (c *UpdateAuditMs) SetUser(user string) {
	c.UpdatedBy = user
	c.UpdatedAt = timex.Now().UnixMilli()
}

type DeleteAuditor interface {
	GetDeleteInfo() (string, time.Time)
	SetDeleter(user string)
	GetDeleteTimeRaw() interface{}
}

var _ DeleteAuditor = (*DeleteAudit)(nil)

type DeleteAudit struct {
	DeletedAt time.Time `bson:"deleted_at,omitempty" json:"deleted_at,omitempty" gorm:"deleted_at;type:datetime(3);autoUpdateTime:false"`
	DeletedBy string    `bson:"deleted_by,omitempty" json:"deleted_by,omitempty" gorm:"deleted_by;type:varchar(255)"`
}

func (c *DeleteAudit) GetDeleteTimeRaw() interface{} {
	if c == nil {
		return time.Time{}
	}
	return c.DeletedAt
}

func (c *DeleteAudit) GetDeleteInfo() (string, time.Time) {
	return c.DeletedBy, c.DeletedAt
}

func (c *DeleteAudit) SetDeleter(user string) {
	c.SetUser(user)
}

func (c *DeleteAudit) SetUser(user string) {
	c.DeletedBy = user
	c.DeletedAt = timex.Now()
}

var _ DeleteAuditor = (*DeleteAuditMs)(nil)

type DeleteAuditMs struct {
	DeletedAt int64  `bson:"deleted_at,omitempty" json:"deleted_at,omitempty" gorm:"deleted_at;type:bigint unsigned;autoUpdateTime:false"`
	DeletedBy string `bson:"deleted_by,omitempty" json:"deleted_by,omitempty" gorm:"deleted_by;type:varchar(255)"`
}

func (c *DeleteAuditMs) GetDeleteTimeRaw() interface{} {
	if c == nil {
		return 0
	}
	return c.DeletedAt
}

func (c *DeleteAuditMs) GetDeleteInfo() (string, time.Time) {
	if c.DeletedAt == 0 {
		return c.DeletedBy, time.Time{}
	}
	return c.DeletedBy, time.UnixMilli(c.DeletedAt)
}

func (c *DeleteAuditMs) SetDeleter(user string) {
	c.SetUser(user)
}

func (c *DeleteAuditMs) SetUser(user string) {
	c.DeletedBy = user
	c.DeletedAt = timex.Now().UnixMilli()
}

type TenantCarrier interface {
	GetTenantID() string
	SetTenantID(id string)
	GetAppID() string
	SetAppID(id string)
}

type TenantAudit struct {
	TenantID string `bson:"tenant_id,omitempty" json:"tenant_id"  gorm:"tenant_id;type:varchar(255)"`
	AppID    string `bson:"app_id,omitempty" json:"app_id"  gorm:"app_id;type:varchar(255)"`
}

func (c *TenantAudit) GetTenantID() string {
	return c.TenantID
}

func (c *TenantAudit) SetTenantID(id string) {
	c.TenantID = id
}

func (c *TenantAudit) GetAppID() string {
	return c.AppID
}

func (c *TenantAudit) SetAppID(id string) {
	c.AppID = id
}
