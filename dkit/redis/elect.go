package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dev-ofa/core-go/dkit"
	"github.com/dev-ofa/core-go/trace/logging"
	goredis "github.com/redis/go-redis/v9"
)

const (
	// LeaderKey is the default election key prefix.
	LeaderKey = "leader"
	// DefaultUnhealthySeconds is the default lease duration in seconds.
	DefaultUnhealthySeconds = 5
	// DefaultTimeout is the default Redis operation timeout in seconds.
	DefaultTimeout = 2
)

const continueLeaderScript = `
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("pexpire", KEYS[1], ARGV[2])
end
return 0
`

// ElectionImpl implements Redis-backed leader election and heartbeat.
type ElectionImpl struct {
	opt *dkit.ElectionOption

	leaderFlag atomic.Bool
	redisCli   goredis.UniversalClient
	keyPrefix  string

	wg          sync.WaitGroup
	closeCh     chan struct{}
	closeOnce   sync.Once
	readyCh     chan struct{}
	readyOnce   sync.Once
	electTicker *time.Ticker
	hbTicker    *time.Ticker
}

// NewElectionImpl creates an ElectionImpl.
func NewElectionImpl(opt *dkit.ElectionOption, cli goredis.UniversalClient, keyPrefix string) *ElectionImpl {
	return &ElectionImpl{
		opt:       opt,
		redisCli:  cli,
		keyPrefix: keyPrefix,
		closeCh:   make(chan struct{}),
		readyCh:   make(chan struct{}),
	}
}

// NodeKey returns the current node identity.
func (impl *ElectionImpl) NodeKey() string {
	if impl.opt == nil {
		return ""
	}
	return impl.opt.NodeKey
}

// Init initializes options and starts election workers.
func (impl *ElectionImpl) Init() error {
	if err := impl.readOpt(); err != nil {
		return err
	}

	impl.wg.Add(1)
	go impl.goElect()
	if impl.opt.KeepHeartbeat {
		impl.wg.Add(1)
		go impl.goHeartBeat()
	}

	<-impl.readyCh
	return nil
}

func (impl *ElectionImpl) readOpt() error {
	if impl.opt == nil {
		return fmt.Errorf("%w: election option is nil", dkit.ErrInvalidOption)
	}
	if impl.opt.NodeKey == "" {
		return fmt.Errorf("%w: node key is empty", dkit.ErrInvalidOption)
	}
	if impl.opt.UnhealthyTime <= 0 {
		impl.opt.UnhealthyTime = time.Second * DefaultUnhealthySeconds
	}
	if impl.opt.Timeout <= 0 {
		impl.opt.Timeout = time.Second * DefaultTimeout
	}
	if impl.redisCli == nil {
		return fmt.Errorf("%w: redis client is nil", dkit.ErrInvalidOption)
	}
	if impl.keyPrefix == "" {
		impl.keyPrefix = defaultKeyPrefix
	}
	return nil
}

func (impl *ElectionImpl) setLeaderFlag(isLeader bool) {
	old := impl.leaderFlag.Swap(isLeader)
	if old == isLeader {
		return
	}
	if impl.opt.OnLeaderChanged != nil {
		impl.opt.OnLeaderChanged(dkit.LeaderChangedEvent{
			IsLeader:     isLeader,
			NodeKey:      impl.opt.NodeKey,
			IsolationKey: impl.opt.IsolationKey,
		})
	}
}

func (impl *ElectionImpl) markReady() {
	impl.readyOnce.Do(func() {
		close(impl.readyCh)
	})
}

// IsLeader reports whether this node is leader.
func (impl *ElectionImpl) IsLeader() bool {
	return impl.leaderFlag.Load()
}

// AliveNodes returns non-expired heartbeat nodes.
func (impl *ElectionImpl) AliveNodes() ([]string, error) {
	if !impl.opt.KeepHeartbeat {
		return nil, fmt.Errorf("%w: heartbeat is disabled", dkit.ErrInvalidOption)
	}
	ctx, cancel := context.WithTimeout(context.Background(), impl.opt.Timeout)
	defer cancel()

	keys, err := impl.scanHeartbeatKeys(ctx)
	if err != nil {
		return nil, err
	}
	aliveNodes := make([]string, 0, len(keys))
	for _, key := range keys {
		idx := strings.LastIndex(key, ":")
		if idx < 0 || idx == len(key)-1 {
			continue
		}
		nodeKey, err := decodeComponent(key[idx+1:])
		if err != nil {
			return nil, fmt.Errorf("decode heartbeat node key failed: %w", err)
		}
		aliveNodes = append(aliveNodes, nodeKey)
	}
	return aliveNodes, nil
}

// IsAlive reports whether nodeKey has a non-expired heartbeat.
func (impl *ElectionImpl) IsAlive(nodeKey string) (bool, error) {
	if !impl.opt.KeepHeartbeat {
		return false, fmt.Errorf("%w: heartbeat is disabled", dkit.ErrInvalidOption)
	}
	ctx, cancel := context.WithTimeout(context.Background(), impl.opt.Timeout)
	defer cancel()

	ret, err := impl.redisCli.Exists(ctx, buildHeartbeatKey(impl.keyPrefix, impl.opt.IsolationKey, nodeKey)).Result()
	if err != nil {
		return false, fmt.Errorf("query alive node failed: %w", err)
	}
	return ret > 0, nil
}

// Close stops workers and deregisters owned leader and heartbeat records.
func (impl *ElectionImpl) Close() {
	impl.closeOnce.Do(func() {
		close(impl.closeCh)
		impl.wg.Wait()

		ctx, cancel := context.WithTimeout(context.Background(), impl.opt.Timeout)
		defer cancel()
		if impl.leaderFlag.Load() {
			if _, err := impl.redisCli.Eval(ctx, deleteIfValueScript, []string{buildLeaderKey(impl.keyPrefix, impl.opt.IsolationKey)}, impl.opt.NodeKey).Int64(); err != nil {
				logging.CtxErrorf(ctx, "deregister leader failed: %s", err)
			}
		}
		if impl.opt.KeepHeartbeat {
			if err := impl.redisCli.Del(ctx, buildHeartbeatKey(impl.keyPrefix, impl.opt.IsolationKey, impl.opt.NodeKey)).Err(); err != nil {
				logging.CtxErrorf(ctx, "deregister heartbeat failed: %s", err)
			}
		}
	})
}

func (impl *ElectionImpl) goElect() {
	defer impl.wg.Done()
	impl.electTicker = time.NewTicker(impl.opt.UnhealthyTime / 2)
	defer impl.electTicker.Stop()

	impl.runElectionOnce()
	for {
		select {
		case <-impl.closeCh:
			return
		case <-impl.electTicker.C:
			impl.runElectionOnce()
		}
	}
}

func (impl *ElectionImpl) runElectionOnce() {
	if impl.opt.CanElect != nil && !impl.opt.CanElect() {
		impl.setLeaderFlag(false)
		impl.markReady()
		return
	}
	impl.elect()
	impl.markReady()
}

func (impl *ElectionImpl) elect() {
	if impl.leaderFlag.Load() {
		if err := impl.continueLeader(); err != nil {
			logging.Errorf("continue leader failed: %s", err)
			impl.setLeaderFlag(false)
		}
		return
	}
	if err := impl.campaign(); err != nil {
		logging.Errorf("campaign failed: %s", err)
	}
}

func (impl *ElectionImpl) campaign() error {
	ctx, cancel := context.WithTimeout(context.Background(), impl.opt.Timeout)
	defer cancel()

	leaderKey := buildLeaderKey(impl.keyPrefix, impl.opt.IsolationKey)
	ok, err := impl.redisCli.SetNX(ctx, leaderKey, impl.opt.NodeKey, impl.opt.UnhealthyTime).Result()
	if err != nil {
		return fmt.Errorf("insert leader failed: %w", err)
	}
	if ok {
		impl.setLeaderFlag(true)
		return nil
	}

	holder, err := impl.redisCli.Get(ctx, leaderKey).Result()
	if errors.Is(err, goredis.Nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find election data failed: %w", err)
	}
	if holder == impl.opt.NodeKey {
		impl.setLeaderFlag(true)
	}
	return nil
}

func (impl *ElectionImpl) continueLeader() error {
	ctx, cancel := context.WithTimeout(context.Background(), impl.opt.Timeout)
	defer cancel()

	ret, err := impl.redisCli.Eval(
		ctx,
		continueLeaderScript,
		[]string{buildLeaderKey(impl.keyPrefix, impl.opt.IsolationKey)},
		impl.opt.NodeKey,
		impl.opt.UnhealthyTime.Milliseconds(),
	).Int64()
	if err != nil {
		return fmt.Errorf("update leader failed: %w", err)
	}
	if ret == 0 {
		return fmt.Errorf("continue leader failed")
	}
	return nil
}

func (impl *ElectionImpl) goHeartBeat() {
	defer impl.wg.Done()
	impl.hbTicker = time.NewTicker(impl.opt.UnhealthyTime / 2)
	defer impl.hbTicker.Stop()

	if err := impl.heartBeat(); err != nil {
		logging.Errorf("heartbeat failed: %s", err)
	}
	for {
		select {
		case <-impl.closeCh:
			return
		case <-impl.hbTicker.C:
			if err := impl.heartBeat(); err != nil {
				logging.Errorf("heartbeat failed: %s", err)
			}
		}
	}
}

func (impl *ElectionImpl) heartBeat() error {
	ctx, cancel := context.WithTimeout(context.Background(), impl.opt.Timeout)
	defer cancel()

	if err := impl.redisCli.Set(ctx, buildHeartbeatKey(impl.keyPrefix, impl.opt.IsolationKey, impl.opt.NodeKey), impl.opt.NodeKey, impl.opt.UnhealthyTime).Err(); err != nil {
		return fmt.Errorf("update heartbeat failed: %w", err)
	}
	return nil
}

func (impl *ElectionImpl) scanHeartbeatKeys(ctx context.Context) ([]string, error) {
	var (
		cursor uint64
		ret    []string
	)
	for {
		keys, nextCursor, err := impl.redisCli.Scan(ctx, cursor, heartbeatMatchPattern(impl.keyPrefix, impl.opt.IsolationKey), 100).Result()
		if err != nil {
			return nil, fmt.Errorf("find alive nodes failed: %w", err)
		}
		ret = append(ret, keys...)
		cursor = nextCursor
		if cursor == 0 {
			return ret, nil
		}
	}
}
