package dkit

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/sony/sonyflake"
)

// NumberAllocator allocates a unique number from a bounded range.
type NumberAllocator interface {
	// GetUniqueRandomNumber returns a unique number in [0, max).
	GetUniqueRandomNumber(ctx context.Context, max int) (int, error)
}

// MutexFactory creates distributed mutexes and exposes their default TTL.
type MutexFactory interface {
	// NewMutex creates a distributed mutex bound to key.
	NewMutex(key string) DistributedMutex
	// GetMutexDefaultTTL returns the default lock TTL used by helper methods.
	GetMutexDefaultTTL() time.Duration
}

// ElectionOption configures leader election and optional heartbeat behavior.
type ElectionOption struct {
	// NodeKey uniquely identifies the current node in the election domain.
	NodeKey string
	// KeepHeartbeat enables heartbeat reporting for the current node.
	KeepHeartbeat bool
	// UnhealthyTime defines the leader or heartbeat lease duration.
	UnhealthyTime time.Duration
	// Timeout defines the timeout for one backend operation.
	Timeout time.Duration
	// IsolationKey separates independent election domains.
	IsolationKey string
	// CanElect is called before campaigning; returning false prevents this node from becoming leader.
	CanElect func() bool
	// OnLeaderChanged receives leader-state transition notifications.
	OnLeaderChanged LeaderChangedHandler
}

// ElectionController exposes leader election and heartbeat operations.
type ElectionController interface {
	// EnableElection enables leader election and optional heartbeat behavior.
	EnableElection(opt *ElectionOption) error
	// NodeKey returns the current node identity.
	NodeKey() string
	// IsLeader reports whether the current node is the leader.
	IsLeader() bool
	// AliveNodes returns non-expired heartbeat nodes when heartbeat is enabled.
	AliveNodes() ([]string, error)
	// IsAlive reports whether nodeKey has a non-expired heartbeat.
	IsAlive(nodeKey string) (bool, error)
}

// Atomic groups backend-backed distributed primitives.
type Atomic interface {
	NumberAllocator
	MutexFactory
	ElectionController

	// Close releases resources held by the backend implementation.
	Close() error
}

// IDGenerator generates globally unique IDs.
type IDGenerator interface {
	// NextID returns a globally unique ID.
	NextID(ctx context.Context) (uint64, error)
	// NextIDString returns a globally unique ID formatted as a string.
	NextIDString(ctx context.Context) (string, error)
}

// SnowflakeIDGenerator is kept for compatibility with go-dev/dkit.
type SnowflakeIDGenerator interface {
	// GetID returns a globally unique ID and panics when generation fails.
	GetID() uint64
	// GetIDString returns a globally unique string ID and panics when generation fails.
	GetIDString() string
}

// Action is executed while holding a distributed mutex.
type Action func(ctx context.Context) error

// Kit combines backend-backed primitives with ID generation and lock helpers.
type Kit interface {
	Atomic
	IDGenerator
	SnowflakeIDGenerator

	// MutexTryDo tries to acquire mutexKey with the backend default TTL before executing action.
	MutexTryDo(mutexKey string, action Action) (bool, error)
	// MutexCtxTryDo tries to acquire mutexKey in ctx before executing action.
	MutexCtxTryDo(ctx context.Context, mutexKey string, action Action) (bool, error)
	// MutexDo acquires mutexKey with the backend default TTL before executing action.
	MutexDo(mutexKey string, action Action) error
	// MutexCtxDo acquires mutexKey in ctx before executing action.
	MutexCtxDo(ctx context.Context, mutexKey string, action Action) error
}

// NewDefaultKit creates a Kit using context.Background for machine ID allocation.
func NewDefaultKit(atomic Atomic) (Kit, error) {
	return NewDefaultKitWithContext(context.Background(), atomic)
}

// NewDefaultKitWithContext creates a Kit using atomic to allocate a snowflake machine ID.
func NewDefaultKitWithContext(ctx context.Context, atomic Atomic) (Kit, error) {
	if atomic == nil {
		return nil, fmt.Errorf("%w: atomic is nil", ErrInvalidOption)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	num, err := atomic.GetUniqueRandomNumber(ctx, math.MaxUint16)
	if err != nil {
		return nil, err
	}
	if num < 0 || num > math.MaxUint16 {
		return nil, fmt.Errorf("%w: machine id %d out of uint16 range", ErrInvalidOption, num)
	}

	ins := sonyflake.NewSonyflake(sonyflake.Settings{
		MachineID: func() (uint16, error) {
			return uint16(num), nil
		},
	})
	if ins == nil {
		return nil, fmt.Errorf("%w: create sonyflake failed", ErrInvalidOption)
	}

	return &defaultKit{Atomic: atomic, ins: ins}, nil
}

type defaultKit struct {
	Atomic

	ins *sonyflake.Sonyflake
}

// NextID returns a globally unique ID.
func (d *defaultKit) NextID(ctx context.Context) (uint64, error) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
	}
	i, err := d.ins.NextID()
	if err != nil {
		return 0, fmt.Errorf("get snowflake id failed: %w", err)
	}
	return i, nil
}

// NextIDString returns a globally unique ID formatted as a string.
func (d *defaultKit) NextIDString(ctx context.Context) (string, error) {
	i, err := d.NextID(ctx)
	if err != nil {
		return "", err
	}
	return strconv.FormatUint(i, 10), nil
}

// GetID returns a globally unique ID and panics when generation fails.
func (d *defaultKit) GetID() uint64 {
	i, err := d.NextID(context.Background())
	if err != nil {
		panic(err)
	}
	return i
}

// GetIDString returns a globally unique string ID and panics when generation fails.
func (d *defaultKit) GetIDString() string {
	i, err := d.NextIDString(context.Background())
	if err != nil {
		panic(err)
	}
	return i
}

// MutexTryDo tries to acquire mutexKey before executing action.
func (d *defaultKit) MutexTryDo(mutexKey string, action Action) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.GetMutexDefaultTTL())
	defer cancel()

	return d.MutexCtxTryDo(ctx, mutexKey, action)
}

// MutexCtxTryDo tries to acquire mutexKey in ctx before executing action.
func (d *defaultKit) MutexCtxTryDo(ctx context.Context, mutexKey string, action Action) (getLock bool, err error) {
	if action == nil {
		return false, fmt.Errorf("%w: action is nil", ErrInvalidOption)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	mux := d.NewMutex(mutexKey)
	if mux == nil {
		return false, fmt.Errorf("%w: mutex is nil", ErrInvalidOption)
	}
	ok, err := mux.TryLock(ctx)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	defer func() {
		if uErr := mux.Unlock(ctx); uErr != nil {
			if err != nil {
				err = fmt.Errorf("unlock failed: %v, action error: %w", uErr, err)
				return
			}
			err = fmt.Errorf("unlock failed: %w", uErr)
		}
	}()

	if err = action(ctx); err != nil {
		return true, err
	}
	return true, nil
}

// MutexDo acquires mutexKey before executing action.
func (d *defaultKit) MutexDo(mutexKey string, action Action) error {
	ctx, cancel := context.WithTimeout(context.Background(), d.GetMutexDefaultTTL())
	defer cancel()

	return d.MutexCtxDo(ctx, mutexKey, action)
}

// MutexCtxDo acquires mutexKey in ctx before executing action.
func (d *defaultKit) MutexCtxDo(ctx context.Context, mutexKey string, action Action) (err error) {
	if action == nil {
		return fmt.Errorf("%w: action is nil", ErrInvalidOption)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	mux := d.NewMutex(mutexKey)
	if mux == nil {
		return fmt.Errorf("%w: mutex is nil", ErrInvalidOption)
	}
	if err := mux.Lock(ctx); err != nil {
		return err
	}
	defer func() {
		if uErr := mux.Unlock(ctx); uErr != nil {
			if err != nil {
				err = fmt.Errorf("unlock failed: %v, action error: %w", uErr, err)
				return
			}
			err = fmt.Errorf("unlock failed: %w", uErr)
		}
	}()

	if err = action(ctx); err != nil {
		return err
	}
	return nil
}
