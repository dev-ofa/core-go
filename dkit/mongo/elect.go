package mongo

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dev-ofa/core-go/dkit"
	"github.com/dev-ofa/core-go/trace/logging"
	"github.com/shiningrush/goext/runx/eventx"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	// LeaderKey is the default election key.
	LeaderKey = "leader"
	// DefaultUnhealthySeconds is the default lease duration in seconds.
	DefaultUnhealthySeconds = 5
	// DefaultTimeout is the default MongoDB operation timeout in seconds.
	DefaultTimeout = 2
)

// Payload stores one heartbeat lease.
type Payload struct {
	NodeKey      string    `bson:"_id"`
	IsolationKey string    `bson:"isolation_key"`
	Expires      time.Time `bson:"expires"`
}

// LeaderPayload stores one leader lease.
type LeaderPayload struct {
	ID           string    `bson:"_id"`
	NodeKey      string    `bson:"node_key"`
	IsolationKey string    `bson:"isolation_key"`
	Expires      time.Time `bson:"expires"`
}

// ElectionImpl implements MongoDB-backed leader election and heartbeat.
type ElectionImpl struct {
	opt *dkit.ElectionOption

	leaderFlag atomic.Bool
	hbCls      *mongo.Collection
	electCls   *mongo.Collection

	wg          sync.WaitGroup
	closeCh     chan struct{}
	closeOnce   sync.Once
	readyCh     chan struct{}
	readyOnce   sync.Once
	electTicker *time.Ticker
	hbTicker    *time.Ticker
}

// NewElectionImpl creates an ElectionImpl.
func NewElectionImpl(opt *dkit.ElectionOption, electCls, hbCls *mongo.Collection) *ElectionImpl {
	return &ElectionImpl{
		opt:      opt,
		closeCh:  make(chan struct{}),
		readyCh:  make(chan struct{}),
		electCls: electCls,
		hbCls:    hbCls,
	}
}

// NodeKey returns the current node identity.
func (impl *ElectionImpl) NodeKey() string {
	if impl.opt == nil {
		return ""
	}
	return impl.opt.NodeKey
}

// Init initializes indexes and starts election workers.
func (impl *ElectionImpl) Init() error {
	if err := impl.readOpt(); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), impl.opt.Timeout)
	defer cancel()
	if err := ensureTTLIndex(ctx, impl.electCls); err != nil {
		return err
	}
	if impl.opt.KeepHeartbeat {
		if err := ensureTTLIndex(ctx, impl.hbCls); err != nil {
			return err
		}
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
	if impl.electCls == nil {
		return fmt.Errorf("%w: election collection is nil", dkit.ErrInvalidOption)
	}
	if impl.opt.KeepHeartbeat && impl.hbCls == nil {
		return fmt.Errorf("%w: heartbeat collection is nil", dkit.ErrInvalidOption)
	}
	return nil
}

func (impl *ElectionImpl) setLeaderFlag(isLeader bool) {
	old := impl.leaderFlag.Swap(isLeader)
	if old == isLeader {
		return
	}
	eventx.PublishSync(context.Background(), dkit.LeaderChangedEvent{
		IsLeader:     isLeader,
		NodeKey:      impl.opt.NodeKey,
		IsolationKey: impl.opt.IsolationKey,
	})
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
	ctx, cancel := context.WithTimeout(context.Background(), impl.opt.Timeout)
	defer cancel()
	filter := bson.M{
		"expires":       bson.M{"$gte": time.Now()},
		"isolation_key": impl.opt.IsolationKey,
	}
	cur, err := impl.hbCls.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("find alive nodes failed: %w", err)
	}
	defer cur.Close(ctx)

	var ret []Payload
	if err := cur.All(ctx, &ret); err != nil {
		return nil, fmt.Errorf("decode alive nodes failed: %w", err)
	}

	aliveNodes := make([]string, 0, len(ret))
	for i := range ret {
		aliveNodes = append(aliveNodes, ret[i].NodeKey)
	}
	return aliveNodes, nil
}

// IsAlive reports whether nodeKey has a non-expired heartbeat.
func (impl *ElectionImpl) IsAlive(nodeKey string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), impl.opt.Timeout)
	defer cancel()
	filter := bson.M{
		"_id":           nodeKey,
		"expires":       bson.M{"$gte": time.Now()},
		"isolation_key": impl.opt.IsolationKey,
	}
	var p Payload
	err := impl.hbCls.FindOne(ctx, filter).Decode(&p)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query alive node failed: %w", err)
	}
	return true, nil
}

// Close stops workers and deregisters owned leader and heartbeat records.
func (impl *ElectionImpl) Close() {
	impl.closeOnce.Do(func() {
		close(impl.closeCh)
		impl.wg.Wait()

		ctx, cancel := context.WithTimeout(context.Background(), impl.opt.Timeout)
		defer cancel()
		if impl.leaderFlag.Load() {
			if _, err := impl.electCls.DeleteOne(ctx, bson.M{"_id": buildLeaderKey(impl.opt.IsolationKey), "node_key": impl.opt.NodeKey}); err != nil {
				logging.CtxErrorf(ctx, "deregister leader failed: %s", err)
			}
		}
		if impl.opt.KeepHeartbeat {
			if _, err := impl.hbCls.DeleteOne(ctx, bson.M{"_id": impl.opt.NodeKey, "isolation_key": impl.opt.IsolationKey}); err != nil {
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

	cur, err := impl.electCls.Find(ctx, bson.M{"isolation_key": impl.opt.IsolationKey})
	if err != nil {
		return fmt.Errorf("find election data failed: %w", err)
	}
	defer cur.Close(ctx)
	var ret []LeaderPayload
	if err := cur.All(ctx, &ret); err != nil {
		return fmt.Errorf("decode election data failed: %w", err)
	}

	now := time.Now()
	if len(ret) > 0 {
		leader := ret[0]
		if leader.NodeKey == impl.opt.NodeKey {
			impl.setLeaderFlag(true)
			return nil
		}
		if leader.Expires.Before(now) {
			updateRet, err := impl.electCls.UpdateOne(ctx,
				bson.M{"_id": leader.ID, "node_key": leader.NodeKey, "expires": leader.Expires},
				bson.M{"$set": bson.M{"node_key": impl.opt.NodeKey, "expires": now.Add(impl.opt.UnhealthyTime)}},
			)
			if err != nil {
				return fmt.Errorf("update leader failed: %w", err)
			}
			if updateRet.ModifiedCount > 0 {
				impl.setLeaderFlag(true)
			}
		}
		return nil
	}

	_, err = impl.electCls.InsertOne(ctx, LeaderPayload{
		ID:           buildLeaderKey(impl.opt.IsolationKey),
		NodeKey:      impl.opt.NodeKey,
		IsolationKey: impl.opt.IsolationKey,
		Expires:      now.Add(impl.opt.UnhealthyTime),
	})
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil
		}
		return fmt.Errorf("insert leader failed: %w", err)
	}
	impl.setLeaderFlag(true)
	return nil
}

func (impl *ElectionImpl) continueLeader() error {
	ctx, cancel := context.WithTimeout(context.Background(), impl.opt.Timeout)
	defer cancel()
	ret, err := impl.electCls.UpdateOne(ctx,
		bson.M{"_id": buildLeaderKey(impl.opt.IsolationKey), "node_key": impl.opt.NodeKey},
		bson.M{"$set": bson.M{"expires": time.Now().Add(impl.opt.UnhealthyTime)}},
	)
	if err != nil {
		return fmt.Errorf("update leader failed: %w", err)
	}
	if ret.MatchedCount == 0 {
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
	_, err := impl.hbCls.UpdateOne(ctx,
		bson.M{"_id": impl.opt.NodeKey},
		bson.M{"$set": bson.M{
			"expires":       time.Now().Add(impl.opt.UnhealthyTime),
			"isolation_key": impl.opt.IsolationKey,
		}},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("update heartbeat failed: %w", err)
	}
	return nil
}

func buildLeaderKey(isolationKey string) string {
	if isolationKey != "" {
		return fmt.Sprintf("%s-%s", LeaderKey, isolationKey)
	}
	return LeaderKey
}
