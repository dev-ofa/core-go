package dkit

import (
	"context"
	"time"
)

// DefaultLockTTL is the default lease duration used by lock helpers.
const DefaultLockTTL = 30 * time.Second

// DefaultLockSpinInterval is the default interval used by blocking lock retries.
const DefaultLockSpinInterval = 500 * time.Millisecond

// DistributedMutex protects a distributed resource identified by a resource key.
type DistributedMutex interface {
	// Lock waits until the lock is acquired, the context is canceled, or MaxWaitTime is reached.
	Lock(ctx context.Context, ops ...LockOptionOp) error
	// TryLock attempts to acquire the lock once and returns false when it is held by another owner.
	TryLock(ctx context.Context, ops ...LockOptionOp) (bool, error)
	// Unlock releases the lock owned by the current holder.
	Unlock(ctx context.Context) error
	// ExistLock reports whether a non-expired lock currently exists.
	ExistLock(ctx context.Context) (bool, error)
}

// LockOption configures lock acquisition behavior.
type LockOption struct {
	// TTL is the lock lease duration.
	TTL time.Duration
	// ReentrantIdentity allows the same logical holder to re-enter a supported lock implementation.
	ReentrantIdentity string
	// SpinInterval controls the retry interval used by blocking Lock implementations.
	SpinInterval time.Duration
	// MaxWaitTime limits how long Lock may wait for the lock.
	MaxWaitTime time.Duration
}

// LockOptionOp mutates LockOption.
type LockOptionOp func(option *LockOption)

// NewLockOption builds lock options from a default TTL and functional options.
func NewLockOption(defaultTTL time.Duration, ops []LockOptionOp) *LockOption {
	if defaultTTL <= 0 {
		defaultTTL = DefaultLockTTL
	}
	opt := &LockOption{
		TTL:          defaultTTL,
		SpinInterval: DefaultLockSpinInterval,
	}

	for _, op := range ops {
		if op != nil {
			op(opt)
		}
	}
	if opt.TTL <= 0 {
		opt.TTL = defaultTTL
	}
	if opt.SpinInterval <= 0 {
		opt.SpinInterval = DefaultLockSpinInterval
	}
	return opt
}

// LockTTL configures the lock lease duration.
func LockTTL(d time.Duration) LockOptionOp {
	return func(option *LockOption) {
		if d > 0 {
			option.TTL = d
		}
	}
}

// LockWithMaxWait configures the maximum wait duration for blocking Lock.
func LockWithMaxWait(waitTime time.Duration) LockOptionOp {
	return func(option *LockOption) {
		if waitTime > 0 {
			option.MaxWaitTime = waitTime
		}
	}
}

// LockWithSpinInterval configures the retry interval for blocking Lock.
func LockWithSpinInterval(interval time.Duration) LockOptionOp {
	return func(option *LockOption) {
		if interval > 0 {
			option.SpinInterval = interval
		}
	}
}

// Reentrant configures the reentrant holder identity.
func Reentrant(identity string) LockOptionOp {
	return func(option *LockOption) {
		option.ReentrantIdentity = identity
	}
}
