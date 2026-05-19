package redis

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/dev-ofa/core-go/dkit"
	goredis "github.com/redis/go-redis/v9"
)

const defaultKeyPrefix = "dkit"

var _ dkit.Atomic = (*Atomic)(nil)

// Atomic implements DKit primitives with Redis keys.
type Atomic struct {
	mu sync.RWMutex

	randomImpl *RandomNumberImpl
	mutexImpl  *MutexImpl
	electImpl  *ElectionImpl

	opt *BuilderOption
}

// BuilderOption configures Atomic.
type BuilderOption struct {
	defaultTTL time.Duration
	redisCli   goredis.UniversalClient
	keyPrefix  string
}

// BuilderOptionOp mutates BuilderOption.
type BuilderOptionOp func(*BuilderOption)

// NewBuilderOption returns the default Atomic options.
func NewBuilderOption() *BuilderOption {
	return &BuilderOption{
		defaultTTL: dkit.DefaultLockTTL,
		keyPrefix:  defaultKeyPrefix,
	}
}

// TTL configures the default lock TTL.
func TTL(ttl time.Duration) BuilderOptionOp {
	return func(option *BuilderOption) {
		if ttl > 0 {
			option.defaultTTL = ttl
		}
	}
}

// Client configures the Redis client used by Atomic.
func Client(cli goredis.UniversalClient) BuilderOptionOp {
	return func(option *BuilderOption) {
		option.redisCli = cli
	}
}

// KeyPrefix configures the Redis key prefix.
func KeyPrefix(prefix string) BuilderOptionOp {
	return func(option *BuilderOption) {
		if prefix != "" {
			option.keyPrefix = prefix
		}
	}
}

// NewAtomic creates a Redis-backed Atomic.
func NewAtomic(ops ...BuilderOptionOp) (*Atomic, error) {
	opt := NewBuilderOption()
	for _, op := range ops {
		if op != nil {
			op(opt)
		}
	}
	if opt.redisCli == nil {
		return nil, fmt.Errorf("%w: redis client is nil", dkit.ErrInvalidOption)
	}

	mutexImpl := NewMutexImpl(opt.redisCli, opt.keyPrefix, opt.defaultTTL)
	randomImpl := NewRandomNumberImpl(opt.redisCli, opt.keyPrefix)
	for _, impl := range []interface{ Init() error }{mutexImpl, randomImpl} {
		if err := impl.Init(); err != nil {
			return nil, err
		}
	}

	return &Atomic{
		randomImpl: randomImpl,
		mutexImpl:  mutexImpl,
		opt:        opt,
	}, nil
}

// NewRedisAtomic is kept as a compatibility alias for go-dev/dkit callers.
func NewRedisAtomic(ops ...BuilderOptionOp) (*Atomic, error) {
	return NewAtomic(ops...)
}

// NodeKey returns the election node key.
func (at *Atomic) NodeKey() string {
	at.mu.RLock()
	elect := at.electImpl
	at.mu.RUnlock()
	if elect == nil {
		return ""
	}
	return elect.NodeKey()
}

// EnableElection enables leader election and optional heartbeat.
func (at *Atomic) EnableElection(opt *dkit.ElectionOption) error {
	if at.opt == nil || at.opt.redisCli == nil {
		return fmt.Errorf("%w: redis client is nil", dkit.ErrInvalidOption)
	}
	at.mu.Lock()
	defer at.mu.Unlock()
	if at.electImpl != nil {
		return fmt.Errorf("%w: election already enabled", dkit.ErrInvalidOption)
	}
	elect := NewElectionImpl(opt, at.opt.redisCli, at.opt.keyPrefix)
	if err := elect.Init(); err != nil {
		return err
	}
	at.electImpl = elect
	return nil
}

// IsLeader reports whether this node is currently leader.
func (at *Atomic) IsLeader() bool {
	at.mu.RLock()
	elect := at.electImpl
	at.mu.RUnlock()
	if elect == nil {
		return false
	}
	return elect.IsLeader()
}

// AliveNodes returns alive heartbeat nodes.
func (at *Atomic) AliveNodes() ([]string, error) {
	at.mu.RLock()
	elect := at.electImpl
	at.mu.RUnlock()
	if elect == nil {
		return nil, dkit.ErrElectionNotEnabled
	}
	return elect.AliveNodes()
}

// IsAlive reports whether nodeKey has a non-expired heartbeat.
func (at *Atomic) IsAlive(nodeKey string) (bool, error) {
	at.mu.RLock()
	elect := at.electImpl
	at.mu.RUnlock()
	if elect == nil {
		return false, dkit.ErrElectionNotEnabled
	}
	return elect.IsAlive(nodeKey)
}

// Close releases election resources.
func (at *Atomic) Close() error {
	at.mu.Lock()
	elect := at.electImpl
	at.electImpl = nil
	at.mu.Unlock()
	if elect != nil {
		elect.Close()
	}
	return nil
}

// GetUniqueRandomNumber allocates a temporary unique number.
func (at *Atomic) GetUniqueRandomNumber(ctx context.Context, max int) (int, error) {
	return at.randomImpl.GetUniqueRandomNumber(ctx, max)
}

// NewMutex creates a distributed mutex.
func (at *Atomic) NewMutex(key string) dkit.DistributedMutex {
	return at.mutexImpl.NewMutex(key)
}

// GetMutexDefaultTTL returns the default lock TTL.
func (at *Atomic) GetMutexDefaultTTL() time.Duration {
	return at.mutexImpl.GetMutexDefaultTTL()
}

func buildMutexKey(prefix, key string) string {
	return fmt.Sprintf("%s:mutex:%s", prefix, key)
}

func buildRandomKey(prefix string, num int) string {
	return fmt.Sprintf("%s:random:%d", prefix, num)
}

func buildLeaderKey(prefix, isolationKey string) string {
	return fmt.Sprintf("%s:leader:%s", prefix, encodeComponent(isolationKey))
}

func buildHeartbeatKey(prefix, isolationKey, nodeKey string) string {
	return fmt.Sprintf("%s:heartbeat:%s:%s", prefix, encodeComponent(isolationKey), encodeComponent(nodeKey))
}

func heartbeatMatchPattern(prefix, isolationKey string) string {
	return fmt.Sprintf("%s:heartbeat:%s:*", prefix, encodeComponent(isolationKey))
}

func encodeComponent(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeComponent(value string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
