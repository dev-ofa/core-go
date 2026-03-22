package model

import (
	"context"
	"fmt"
	"time"

	"github.com/dev-ofa/core-go/pass"
	"github.com/shiningrush/goext/timex"
)

// CtxCreateAudit fills creator and tenant/app audit fields from context.
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

// CtxUpdateAudit fills updater audit fields from context.
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

// CtxUpdateAuditAndEnv updates audit fields and keeps a hook for env sync.
func CtxUpdateAuditAndEnv(ctx context.Context, entity any) error {
	if err := CtxUpdateAudit(ctx, entity); err != nil {
		return err
	}

	return nil
}

// CtxDeleteAudit fills deleter audit fields from context when supported.
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

// CtxAudit fills operator and tenant/app fields for a batch of auditors.
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

// CreateAuditor exposes creation audit behavior.
type CreateAuditor interface {
	GetCreatorInfo() (string, time.Time)
	SetCreator(user string)
	GetCreateTimeRaw() interface{}
}

// CreateAudit stores creator and creation time in time.Time.
type CreateAudit struct {
	// CreatedAt is the creation time.
	CreatedAt time.Time `bson:"created_at,omitempty" json:"created_at,omitempty" gorm:"created_at;type:datetime(3);autoUpdateTime:false"`
	// CreatedBy is the creator id.
	CreatedBy string `bson:"created_by,omitempty" json:"created_by,omitempty" gorm:"created_by;type:varchar(255)"`
}

// GetCreateTimeRaw returns the raw creation time value.
func (c *CreateAudit) GetCreateTimeRaw() interface{} {
	return c.CreatedAt
}

// GetCreatorInfo returns creator id and creation time.
func (c *CreateAudit) GetCreatorInfo() (string, time.Time) {
	return c.CreatedBy, c.CreatedAt
}

// SetCreator sets creator id and time.
func (c *CreateAudit) SetCreator(user string) {
	c.SetUser(user)
}

// SetUser sets creator id and time.
func (c *CreateAudit) SetUser(user string) {
	c.CreatedBy = user
	c.CreatedAt = timex.Now()
}

var _ CreateAuditor = (*CreateAuditMs)(nil)

// CreateAuditMs stores creator and creation time in milliseconds.
type CreateAuditMs struct {
	// CreatedAt is the creation time in milliseconds.
	CreatedAt int64 `bson:"created_at,omitempty" json:"created_at,omitempty" gorm:"created_at;type:bigint unsigned;autoUpdateTime:false"`
	// CreatedBy is the creator id.
	CreatedBy string `bson:"created_by,omitempty" json:"created_by,omitempty" gorm:"created_by;type:varchar(255)"`
}

// GetCreateTimeRaw returns the raw creation time value.
func (c *CreateAuditMs) GetCreateTimeRaw() interface{} {
	return c.CreatedAt
}

// GetCreatorInfo returns creator id and creation time.
func (c *CreateAuditMs) GetCreatorInfo() (string, time.Time) {
	if c.CreatedAt == 0 {
		return c.CreatedBy, time.Time{}
	}
	return c.CreatedBy, time.UnixMilli(c.CreatedAt)
}

// SetCreator sets creator id and time.
func (c *CreateAuditMs) SetCreator(user string) {
	c.SetUser(user)
}

// SetUser sets creator id and time.
func (c *CreateAuditMs) SetUser(user string) {
	c.CreatedBy = user
	c.CreatedAt = timex.Now().UnixMilli()
}

// UpdateAuditor exposes update audit behavior.
type UpdateAuditor interface {
	GetUpdateInfo() (string, time.Time)
	SetUpdater(user string)
	GetUpdateTimeRaw() interface{}
}

var _ UpdateAuditor = (*UpdateAudit)(nil)

// UpdateAudit stores updater and update time in time.Time.
type UpdateAudit struct {
	// UpdatedAt is the update time.
	UpdatedAt time.Time `bson:"updated_at,omitempty" json:"updated_at,omitempty" gorm:"updated_at;type:datetime(3);autoUpdateTime:false"`
	// UpdatedBy is the updater id.
	UpdatedBy string `bson:"updated_by,omitempty" json:"updated_by,omitempty" gorm:"updated_by;type:varchar(255)"`
}

// GetUpdateTimeRaw returns the raw update time value.
func (c *UpdateAudit) GetUpdateTimeRaw() interface{} {
	return c.UpdatedAt
}

// GetUpdateInfo returns updater id and update time.
func (c *UpdateAudit) GetUpdateInfo() (string, time.Time) {
	return c.UpdatedBy, c.UpdatedAt
}

// SetUpdater sets updater id and time.
func (c *UpdateAudit) SetUpdater(user string) {
	c.SetUser(user)
}

// SetUser sets updater id and time.
func (c *UpdateAudit) SetUser(user string) {
	c.UpdatedBy = user
	c.UpdatedAt = timex.Now()
}

var _ UpdateAuditor = (*UpdateAuditMs)(nil)

// UpdateAuditMs stores updater and update time in milliseconds.
type UpdateAuditMs struct {
	// UpdatedAt is the update time in milliseconds.
	UpdatedAt int64 `bson:"updated_at,omitempty" json:"updated_at,omitempty" gorm:"updated_at;type:bigint unsigned;autoUpdateTime:false"`
	// UpdatedBy is the updater id.
	UpdatedBy string `bson:"updated_by,omitempty" json:"updated_by,omitempty" gorm:"updated_by;type:varchar(255)"`
}

// GetUpdateTimeRaw returns the raw update time value.
func (c *UpdateAuditMs) GetUpdateTimeRaw() interface{} {
	return c.UpdatedAt
}

// GetUpdateInfo returns updater id and update time.
func (c *UpdateAuditMs) GetUpdateInfo() (string, time.Time) {
	if c.UpdatedAt == 0 {
		return c.UpdatedBy, time.Time{}
	}
	return c.UpdatedBy, time.UnixMilli(c.UpdatedAt)
}

// SetUpdater sets updater id and time.
func (c *UpdateAuditMs) SetUpdater(user string) {
	c.SetUser(user)
}

// SetUser sets updater id and time.
func (c *UpdateAuditMs) SetUser(user string) {
	c.UpdatedBy = user
	c.UpdatedAt = timex.Now().UnixMilli()
}

// DeleteAuditor exposes delete audit behavior.
type DeleteAuditor interface {
	GetDeleteInfo() (string, time.Time)
	SetDeleter(user string)
	GetDeleteTimeRaw() interface{}
}

var _ DeleteAuditor = (*DeleteAudit)(nil)

// DeleteAudit stores deleter and delete time in time.Time.
type DeleteAudit struct {
	// DeletedAt is the delete time.
	DeletedAt time.Time `bson:"deleted_at,omitempty" json:"deleted_at,omitempty" gorm:"deleted_at;type:datetime(3);autoUpdateTime:false"`
	// DeletedBy is the deleter id.
	DeletedBy string `bson:"deleted_by,omitempty" json:"deleted_by,omitempty" gorm:"deleted_by;type:varchar(255)"`
}

// GetDeleteTimeRaw returns the raw delete time value.
func (c *DeleteAudit) GetDeleteTimeRaw() interface{} {
	if c == nil {
		return time.Time{}
	}
	return c.DeletedAt
}

// GetDeleteInfo returns deleter id and delete time.
func (c *DeleteAudit) GetDeleteInfo() (string, time.Time) {
	return c.DeletedBy, c.DeletedAt
}

// SetDeleter sets deleter id and time.
func (c *DeleteAudit) SetDeleter(user string) {
	c.SetUser(user)
}

// SetUser sets deleter id and time.
func (c *DeleteAudit) SetUser(user string) {
	c.DeletedBy = user
	c.DeletedAt = timex.Now()
}

var _ DeleteAuditor = (*DeleteAuditMs)(nil)

// DeleteAuditMs stores deleter and delete time in milliseconds.
type DeleteAuditMs struct {
	// DeletedAt is the delete time in milliseconds.
	DeletedAt int64 `bson:"deleted_at,omitempty" json:"deleted_at,omitempty" gorm:"deleted_at;type:bigint unsigned;autoUpdateTime:false"`
	// DeletedBy is the deleter id.
	DeletedBy string `bson:"deleted_by,omitempty" json:"deleted_by,omitempty" gorm:"deleted_by;type:varchar(255)"`
}

// GetDeleteTimeRaw returns the raw delete time value.
func (c *DeleteAuditMs) GetDeleteTimeRaw() interface{} {
	if c == nil {
		return 0
	}
	return c.DeletedAt
}

// GetDeleteInfo returns deleter id and delete time.
func (c *DeleteAuditMs) GetDeleteInfo() (string, time.Time) {
	if c.DeletedAt == 0 {
		return c.DeletedBy, time.Time{}
	}
	return c.DeletedBy, time.UnixMilli(c.DeletedAt)
}

// SetDeleter sets deleter id and time.
func (c *DeleteAuditMs) SetDeleter(user string) {
	c.SetUser(user)
}

// SetUser sets deleter id and time.
func (c *DeleteAuditMs) SetUser(user string) {
	c.DeletedBy = user
	c.DeletedAt = timex.Now().UnixMilli()
}

// TenantCarrier exposes tenant and app identifiers.
type TenantCarrier interface {
	GetTenantID() string
	SetTenantID(id string)
	GetAppID() string
	SetAppID(id string)
}

// TenantAudit stores tenant and app identifiers.
type TenantAudit struct {
	// TenantID is the tenant identifier.
	TenantID string `bson:"tenant_id,omitempty" json:"tenant_id"  gorm:"tenant_id;type:varchar(255)"`
	// AppID is the application identifier.
	AppID string `bson:"app_id,omitempty" json:"app_id"  gorm:"app_id;type:varchar(255)"`
}

// GetTenantID returns tenant id.
func (c *TenantAudit) GetTenantID() string {
	return c.TenantID
}

// SetTenantID sets tenant id.
func (c *TenantAudit) SetTenantID(id string) {
	c.TenantID = id
}

// GetAppID returns app id.
func (c *TenantAudit) GetAppID() string {
	return c.AppID
}

// SetAppID sets app id.
func (c *TenantAudit) SetAppID(id string) {
	c.AppID = id
}
