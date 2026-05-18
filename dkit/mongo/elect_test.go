package mongo

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dev-ofa/core-go/dkit"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var mongoFaultWorkers sync.Map

func TestKeeper_Sanity(t *testing.T) {
	db := testDatabase(t)
	prefix := testPrefix(t)
	electCol, hbCol := prefix+"_elect", prefix+"_hb"
	dropCollections(t, db, electCol, hbCol)
	mongoFaultWorkers = sync.Map{}

	w1 := initMongoWorker(t, "worker-1", "", db, electCol, hbCol)
	w2 := initMongoWorker(t, "worker-2", "", db, electCol, hbCol)
	w3 := initMongoWorker(t, "worker-3", "", db, electCol, hbCol)
	w4 := initMongoWorker(t, "worker-4", "other", db, electCol, hbCol)
	defer w2.Close()
	defer w3.Close()
	defer w4.Close()

	if w3.NodeKey() != "worker-3" {
		t.Fatalf("worker-3 node key mismatch: %s", w3.NodeKey())
	}
	if !w1.IsLeader() {
		t.Fatalf("worker-1 should be leader")
	}
	if !w4.IsLeader() {
		t.Fatalf("worker-4 should be leader in isolated domain")
	}

	waitForAliveNodes(t, w2, []string{"worker-1", "worker-2", "worker-3"})
	waitForAliveNodes(t, w4, []string{"worker-4"})

	w1.Close()
	waitForCondition(t, time.Second, func() bool {
		return w2.IsLeader() || w3.IsLeader()
	}, "new leader after worker-1 close")
	waitForAliveNodes(t, w2, []string{"worker-2", "worker-3"})

	var newLeader *ElectionImpl
	if w2.IsLeader() {
		newLeader = w2
	} else {
		newLeader = w3
	}
	mongoFaultWorkers.Store(newLeader.NodeKey(), true)
	waitForCondition(t, time.Second, func() bool {
		return !newLeader.IsLeader()
	}, "leader should step down when CanElect returns false")
}

func TestKeeper_Crash(t *testing.T) {
	db := testDatabase(t)
	prefix := testPrefix(t)
	electCol, hbCol := prefix+"_elect", prefix+"_hb"
	dropCollections(t, db, electCol, hbCol)
	mongoFaultWorkers = sync.Map{}

	w1 := initMongoWorker(t, "worker-1", "", db, electCol, hbCol)
	w2 := initMongoWorker(t, "worker-2", "", db, electCol, hbCol)
	w3 := initMongoWorker(t, "worker-3", "", db, electCol, hbCol)
	defer w2.Close()
	defer w3.Close()

	if !w1.IsLeader() {
		t.Fatalf("worker-1 should be leader")
	}
	close(w1.closeCh)
	w1.wg.Wait()

	waitForCondition(t, 2*time.Second, func() bool {
		return w2.IsLeader() || w3.IsLeader()
	}, "new leader after worker-1 crash")
	waitForAliveNodes(t, w3, []string{"worker-2", "worker-3"})
}

func TestKeeper_Concurrency(t *testing.T) {
	db := testDatabase(t)
	prefix := testPrefix(t)
	electCol, hbCol := prefix+"_elect", prefix+"_hb"
	dropCollections(t, db, electCol, hbCol)
	mongoFaultWorkers = sync.Map{}

	curCnt := 8
	workers := make([]*ElectionImpl, 0, curCnt)
	for i := 0; i < curCnt; i++ {
		workers = append(workers, initMongoWorker(t, fmt.Sprintf("worker-%d", i), "", db, electCol, hbCol))
	}
	defer func() {
		for _, w := range workers {
			w.Close()
		}
	}()

	latest := initMongoWorker(t, "latest-0", "", db, electCol, hbCol)
	defer latest.Close()
	waitForCondition(t, time.Second, func() bool {
		nodes, err := latest.AliveNodes()
		return err == nil && len(nodes) == curCnt+1
	}, "all workers should be alive")

	leaderCount := 0
	for _, w := range append(workers, latest) {
		if w.IsLeader() {
			leaderCount++
		}
	}
	if leaderCount != 1 {
		t.Fatalf("leader count want 1 got %d", leaderCount)
	}
}

func TestKeeper_Reconnect(t *testing.T) {
	db := testDatabase(t)
	prefix := testPrefix(t)
	electCol, hbCol := prefix+"_elect", prefix+"_hb"
	dropCollections(t, db, electCol, hbCol)
	mongoFaultWorkers = sync.Map{}

	w1 := initMongoWorker(t, "worker-1", "", db, electCol, hbCol)
	if !w1.IsLeader() {
		t.Fatalf("worker-1 should be leader")
	}
	w1.Close()

	w1 = initMongoWorker(t, "worker-1", "", db, electCol, hbCol)
	defer w1.Close()
	if !w1.IsLeader() {
		t.Fatalf("worker-1 should be leader after reconnect")
	}
}

func initMongoWorker(t *testing.T, key string, isolationKey string, db *mongo.Database, electCol string, hbCol string) *ElectionImpl {
	t.Helper()
	w := NewElectionImpl(&dkit.ElectionOption{
		NodeKey:       key,
		KeepHeartbeat: true,
		UnhealthyTime: 200 * time.Millisecond,
		Timeout:       time.Second,
		IsolationKey:  isolationKey,
		CanElect: func() bool {
			_, fault := mongoFaultWorkers.Load(key)
			return !fault
		},
	}, db.Collection(electCol), db.Collection(hbCol))
	if err := w.Init(); err != nil {
		t.Fatalf("init worker %s: %v", key, err)
	}
	return w
}

func waitForAliveNodes(t *testing.T, w *ElectionImpl, want []string) {
	t.Helper()
	waitForCondition(t, time.Second, func() bool {
		nodes, err := w.AliveNodes()
		if err != nil || len(nodes) != len(want) {
			return false
		}
		found := map[string]bool{}
		for _, node := range nodes {
			found[node] = true
		}
		for _, node := range want {
			if !found[node] {
				return false
			}
		}
		return true
	}, fmt.Sprintf("alive nodes %v", want))
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool, desc string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", desc)
}

func TestBuildLeaderKey(t *testing.T) {
	if got := buildLeaderKey(""); got != LeaderKey {
		t.Fatalf("empty isolation key want %s got %s", LeaderKey, got)
	}
	if got := buildLeaderKey("tenant"); got != "leader-tenant" {
		t.Fatalf("isolation key want leader-tenant got %s", got)
	}
}

func TestElectionImpl_InvalidOptions(t *testing.T) {
	err := NewElectionImpl(&dkit.ElectionOption{}, nil, nil).Init()
	if err == nil {
		t.Fatalf("expected invalid option error")
	}
}

func TestAtomic_EnableElectionCallback(t *testing.T) {
	db := testDatabase(t)
	prefix := testPrefix(t)
	dropCollections(t, db, prefix+"_random", prefix+"_mutex", prefix+"_elect", prefix+"_heartbeat")

	atomicBackend, err := NewMongoAtomic(Database(db), CollectionPrefix(prefix))
	if err != nil {
		t.Fatalf("new mongo atomic: %v", err)
	}
	defer func() {
		if err := atomicBackend.Close(); err != nil {
			t.Fatalf("close atomic: %v", err)
		}
	}()

	events := make(chan dkit.LeaderChangedEvent, 2)
	err = atomicBackend.EnableElection(&dkit.ElectionOption{
		NodeKey:       "node-1",
		KeepHeartbeat: true,
		UnhealthyTime: 200 * time.Millisecond,
		Timeout:       time.Second,
		OnLeaderChanged: func(event dkit.LeaderChangedEvent) {
			events <- event
		},
	})
	if err != nil {
		t.Fatalf("enable election: %v", err)
	}
	select {
	case event := <-events:
		if !event.IsLeader || event.NodeKey != "node-1" {
			t.Fatalf("unexpected event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected leader changed event")
	}
}

func TestIsAlive(t *testing.T) {
	db := testDatabase(t)
	prefix := testPrefix(t)
	electCol, hbCol := prefix+"_elect", prefix+"_hb"
	dropCollections(t, db, electCol, hbCol)
	mongoFaultWorkers = sync.Map{}

	w := initMongoWorker(t, "worker-1", "", db, electCol, hbCol)
	defer w.Close()
	waitForCondition(t, time.Second, func() bool {
		ok, err := w.IsAlive("worker-1")
		return err == nil && ok
	}, "worker-1 alive")
	ok, err := w.IsAlive("missing")
	if err != nil {
		t.Fatalf("is alive missing: %v", err)
	}
	if ok {
		t.Fatalf("missing node should not be alive")
	}
}
