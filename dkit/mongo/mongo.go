package mongo

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/dev-ofa/core-go/dkit"
	"github.com/dev-ofa/core-go/trace/logging"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const driverErrCodeExistedAndConfigDiff = 85

// LockDetail stores one distributed lock lease.
type LockDetail struct {
	Key      string    `bson:"_id"`
	Expires  time.Time `bson:"expires"`
	Identity string    `bson:"identity"`
}

// RandInfo stores a temporary snowflake machine ID allocation.
type RandInfo struct {
	ID      int       `bson:"_id"`
	Expires time.Time `bson:"expires"`
}

var _ dkit.Atomic = (*Atomic)(nil)

// Atomic implements DKit primitives with MongoDB collections.
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
	mongoDB    *mongo.Database
	prefix     string
}

// BuilderOptionOp mutates BuilderOption.
type BuilderOptionOp func(*BuilderOption)

// NewBuilderOption returns the default Atomic options.
func NewBuilderOption() *BuilderOption {
	return &BuilderOption{
		defaultTTL: dkit.DefaultLockTTL,
		prefix:     "dkit",
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

// Database configures the MongoDB database used by Atomic.
func Database(db *mongo.Database) BuilderOptionOp {
	return func(option *BuilderOption) {
		option.mongoDB = db
	}
}

// CollectionPrefix configures the collection prefix.
func CollectionPrefix(prefix string) BuilderOptionOp {
	return func(option *BuilderOption) {
		if prefix != "" {
			option.prefix = prefix
		}
	}
}

// NewAtomic creates a MongoDB-backed Atomic.
func NewAtomic(ops ...BuilderOptionOp) (*Atomic, error) {
	opt := NewBuilderOption()
	for _, op := range ops {
		if op != nil {
			op(opt)
		}
	}
	if opt.mongoDB == nil {
		return nil, fmt.Errorf("%w: mongo database is nil", dkit.ErrInvalidOption)
	}

	mutexImpl := NewMutexImpl(opt.mongoDB.Collection(fmt.Sprintf("%s_mutex", opt.prefix)), opt.defaultTTL)
	randomImpl := NewRandomNumberImpl(opt.mongoDB.Collection(fmt.Sprintf("%s_random", opt.prefix)))
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

// NewMongoAtomic is kept as a compatibility alias for go-dev/dkit callers.
func NewMongoAtomic(ops ...BuilderOptionOp) (*Atomic, error) {
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
	if at.opt.mongoDB == nil {
		return fmt.Errorf("%w: mongo database is nil", dkit.ErrInvalidOption)
	}
	at.mu.Lock()
	defer at.mu.Unlock()
	if at.electImpl != nil {
		return fmt.Errorf("%w: election already enabled", dkit.ErrInvalidOption)
	}
	elect := NewElectionImpl(opt,
		at.opt.mongoDB.Collection(fmt.Sprintf("%s_elect", at.opt.prefix)),
		at.opt.mongoDB.Collection(fmt.Sprintf("%s_heartbeat", at.opt.prefix)))

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

func ensureTTLIndex(ctx context.Context, cls *mongo.Collection) error {
	if cls == nil {
		return fmt.Errorf("%w: mongo collection is nil", dkit.ErrInvalidOption)
	}
	_, err := cls.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expires", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(1),
	})
	if err == nil {
		return nil
	}
	if !isIndexOptionConflict(err) {
		return fmt.Errorf("create ttl index failed: %w", err)
	}

	logging.CtxWarnf(ctx, "recreate ttl index because existing index has different options: %v", err)
	if dropErr := cls.Indexes().DropAll(ctx); dropErr != nil {
		return fmt.Errorf("drop indexes failed: %w", dropErr)
	}
	if _, createErr := cls.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expires", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(1),
	}); createErr != nil {
		return fmt.Errorf("recreate ttl index failed: %w", createErr)
	}
	return nil
}

func isIndexOptionConflict(err error) bool {
	var commandErr mongo.CommandError
	if errors.As(err, &commandErr) && commandErr.Code == driverErrCodeExistedAndConfigDiff {
		return true
	}
	return false
}
