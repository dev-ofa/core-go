package redis

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dev-ofa/core-go/dkit"
	goredis "github.com/redis/go-redis/v9"
)

const deleteIfValueScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
end
return 0
`

var mutexTokenSeq atomic.Uint64

// MutexImpl creates Redis-backed distributed mutexes.
type MutexImpl struct {
	redisCli   goredis.UniversalClient
	keyPrefix  string
	defaultTTL time.Duration
}

// NewMutexImpl creates a MutexImpl.
func NewMutexImpl(cli goredis.UniversalClient, keyPrefix string, ttl time.Duration) *MutexImpl {
	return &MutexImpl{
		redisCli:   cli,
		keyPrefix:  keyPrefix,
		defaultTTL: ttl,
	}
}

// Init initializes mutex resources.
func (impl *MutexImpl) Init() error {
	return nil
}

// NewMutex creates a mutex bound to key.
func (impl *MutexImpl) NewMutex(key string) dkit.DistributedMutex {
	return &Mutex{
		key:        key,
		redisCli:   impl.redisCli,
		redisKey:   buildMutexKey(impl.keyPrefix, key),
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

// Mutex is a Redis-backed distributed mutex.
type Mutex struct {
	mu  sync.Mutex
	key string

	redisCli goredis.UniversalClient
	redisKey string

	lockValue  string
	defaultTTL time.Duration
}

// Lock waits until the lock is acquired, the context is canceled, or MaxWaitTime is reached.
func (m *Mutex) Lock(ctx context.Context, ops ...dkit.LockOptionOp) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	opt := dkit.NewLockOption(m.defaultTTL, ops)
	if err := m.tryLock(ctx, opt); err != nil {
		return err
	}
	if m.lockValue != "" {
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
			if m.lockValue != "" {
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	opt := dkit.NewLockOption(m.defaultTTL, ops)
	if err := m.tryLock(ctx, opt); err != nil {
		return false, err
	}
	return m.lockValue != "", nil
}

func (m *Mutex) tryLock(ctx context.Context, opt *dkit.LockOption) error {
	if m.redisCli == nil {
		return fmt.Errorf("%w: redis client is nil", dkit.ErrInvalidOption)
	}
	if m.lockValue != "" {
		return nil
	}

	lockValue := opt.ReentrantIdentity
	if lockValue == "" {
		lockValue = nextLockValue()
	}
	ok, err := m.redisCli.SetNX(ctx, m.redisKey, lockValue, opt.TTL).Result()
	if err != nil {
		return fmt.Errorf("acquire lock failed: %w", err)
	}
	if ok {
		m.lockValue = lockValue
		return nil
	}
	if opt.ReentrantIdentity == "" {
		return nil
	}

	holder, err := m.redisCli.Get(ctx, m.redisKey).Result()
	if errors.Is(err, goredis.Nil) {
		m.lockValue = ""
		return nil
	}
	if err != nil {
		return fmt.Errorf("get lock holder failed: %w", err)
	}
	if holder == opt.ReentrantIdentity {
		m.lockValue = opt.ReentrantIdentity
		return nil
	}
	m.lockValue = ""
	return nil
}

// Unlock releases the lock owned by this mutex instance.
func (m *Mutex) Unlock(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if m.lockValue == "" {
		return dkit.ErrAlreadyUnlocked
	}
	ret, err := m.redisCli.Eval(ctx, deleteIfValueScript, []string{m.redisKey}, m.lockValue).Int64()
	if err != nil {
		return fmt.Errorf("delete lock detail failed: %w", err)
	}
	if ret == 0 {
		return dkit.ErrAlreadyUnlocked
	}
	m.lockValue = ""
	return nil
}

// ExistLock reports whether a non-expired lock currently exists.
func (m *Mutex) ExistLock(ctx context.Context) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if m.redisCli == nil {
		return false, fmt.Errorf("%w: redis client is nil", dkit.ErrInvalidOption)
	}
	ret, err := m.redisCli.Exists(ctx, m.redisKey).Result()
	if err != nil {
		return false, fmt.Errorf("get lock detail failed: %w", err)
	}
	return ret > 0, nil
}

func nextLockValue() string {
	return fmt.Sprintf("lock-%d-%d", time.Now().UnixNano(), mutexTokenSeq.Add(1))
}
