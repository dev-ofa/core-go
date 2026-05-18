package mongo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dev-ofa/core-go/dkit"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// MutexImpl creates MongoDB-backed distributed mutexes.
type MutexImpl struct {
	cls        *mongo.Collection
	defaultTTL time.Duration
}

// NewMutexImpl creates a MutexImpl.
func NewMutexImpl(cls *mongo.Collection, ttl time.Duration) *MutexImpl {
	return &MutexImpl{
		cls:        cls,
		defaultTTL: ttl,
	}
}

// Init initializes collection indexes.
func (impl *MutexImpl) Init() error {
	return ensureTTLIndex(context.Background(), impl.cls)
}

// NewMutex creates a mutex bound to key.
func (impl *MutexImpl) NewMutex(key string) dkit.DistributedMutex {
	return &Mutex{
		key:        key,
		mongoCls:   impl.cls,
		defaultTTL: impl.defaultTTL,
	}
}

// GetMutexDefaultTTL returns the default lock TTL.
func (impl *MutexImpl) GetMutexDefaultTTL() time.Duration {
	if impl.defaultTTL > 0 {
		return impl.defaultTTL
	}
	return dkit.DefaultLockTTL
}

// Mutex is a MongoDB-backed distributed mutex.
type Mutex struct {
	key string

	mongoCls   *mongo.Collection
	lockDetail *LockDetail
	defaultTTL time.Duration
}

// Lock waits until the lock is acquired, the context is canceled, or MaxWaitTime is reached.
func (m *Mutex) Lock(ctx context.Context, ops ...dkit.LockOptionOp) error {
	if ctx == nil {
		ctx = context.Background()
	}
	opt := dkit.NewLockOption(m.defaultTTL, ops)
	if err := m.tryLock(ctx, opt); err != nil {
		return err
	}
	if m.lockDetail != nil {
		return nil
	}

	maxWaitTimeout := make(<-chan time.Time)
	if opt.MaxWaitTime > 0 {
		maxWaitTimeout = time.After(opt.MaxWaitTime)
	}
	ticker := time.NewTicker(opt.SpinInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := m.tryLock(ctx, opt); err != nil {
				return err
			}
			if m.lockDetail != nil {
				return nil
			}
		case <-maxWaitTimeout:
			return dkit.ErrLockNotAcquired
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// TryLock attempts to acquire the lock once.
func (m *Mutex) TryLock(ctx context.Context, ops ...dkit.LockOptionOp) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	opt := dkit.NewLockOption(m.defaultTTL, ops)
	if err := m.tryLock(ctx, opt); err != nil {
		return false, err
	}
	return m.lockDetail != nil, nil
}

func (m *Mutex) tryLock(ctx context.Context, opt *dkit.LockOption) error {
	if m.mongoCls == nil {
		return fmt.Errorf("%w: mongo collection is nil", dkit.ErrInvalidOption)
	}
	detail := LockDetail{}
	err := m.mongoCls.FindOne(ctx, bson.M{"_id": m.key}).Decode(&detail)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return fmt.Errorf("get lock detail failed: %w", err)
	}

	if errors.Is(err, mongo.ErrNoDocuments) {
		d := &LockDetail{
			Key:      m.key,
			Expires:  time.Now().Add(opt.TTL),
			Identity: opt.ReentrantIdentity,
		}
		if _, err := m.mongoCls.InsertOne(ctx, d); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return nil
			}
			return fmt.Errorf("insert lock detail failed: %w", err)
		}
		m.lockDetail = d
		return nil
	}

	if detail.Expires.Before(time.Now()) {
		exp := time.Now().Add(opt.TTL)
		ret, err := m.mongoCls.UpdateOne(ctx, bson.M{"_id": m.key, "expires": detail.Expires}, bson.M{
			"$set": bson.M{
				"expires":  exp,
				"identity": opt.ReentrantIdentity,
			},
		})
		if err != nil {
			return fmt.Errorf("get lock failed: %w", err)
		}
		if ret.ModifiedCount == 0 {
			m.lockDetail = nil
			return nil
		}
		detail.Expires = exp
		detail.Identity = opt.ReentrantIdentity
		m.lockDetail = &detail
		return nil
	}

	if opt.ReentrantIdentity != "" && detail.Identity == opt.ReentrantIdentity {
		m.lockDetail = &detail
		return nil
	}
	return nil
}

// Unlock releases the lock owned by this mutex instance.
func (m *Mutex) Unlock(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if m.lockDetail == nil {
		return dkit.ErrAlreadyUnlocked
	}
	ret, err := m.mongoCls.DeleteOne(ctx, bson.M{"_id": m.key, "expires": m.lockDetail.Expires})
	if err != nil {
		return fmt.Errorf("delete lock detail failed: %w", err)
	}
	if ret.DeletedCount == 0 {
		return dkit.ErrAlreadyUnlocked
	}
	m.lockDetail = nil
	return nil
}

// ExistLock reports whether a non-expired lock currently exists.
func (m *Mutex) ExistLock(ctx context.Context) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	detail := LockDetail{}
	err := m.mongoCls.FindOne(ctx, bson.M{"_id": m.key}).Decode(&detail)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return false, fmt.Errorf("get lock detail failed: %w", err)
	}
	if errors.Is(err, mongo.ErrNoDocuments) || detail.Expires.Before(time.Now()) {
		return false, nil
	}
	return true, nil
}
