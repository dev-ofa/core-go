package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dev-ofa/core-go/dkit"
)

func TestRedisAtomic_GetUniqueRandomNumber(t *testing.T) {
	_, cli := testRedisClient(t)
	atomicBackend, err := NewRedisAtomic(Client(cli), KeyPrefix(testPrefix(t)))
	if err != nil {
		t.Fatalf("new redis atomic: %v", err)
	}
	defer func() {
		if err := atomicBackend.Close(); err != nil {
			t.Fatalf("close atomic: %v", err)
		}
	}()

	max := 20
	seen := map[int]bool{}
	for i := 0; i < 5; i++ {
		num, err := atomicBackend.GetUniqueRandomNumber(context.Background(), max)
		if err != nil {
			t.Fatalf("get unique random number: %v", err)
		}
		if num < 0 || num >= max {
			t.Fatalf("number out of range: %d", num)
		}
		if seen[num] {
			t.Fatalf("number should not repeat while lease is alive: %d", num)
		}
		seen[num] = true
	}
}

func TestRedisAtomic_GetUniqueRandomNumberExhausted(t *testing.T) {
	_, cli := testRedisClient(t)
	prefix := testPrefix(t)
	atomicBackend, err := NewRedisAtomic(Client(cli), KeyPrefix(prefix))
	if err != nil {
		t.Fatalf("new redis atomic: %v", err)
	}
	defer func() {
		if err := atomicBackend.Close(); err != nil {
			t.Fatalf("close atomic: %v", err)
		}
	}()

	for i := 0; i < 2; i++ {
		if _, err := atomicBackend.GetUniqueRandomNumber(context.Background(), 2); err != nil {
			t.Fatalf("prefill number: %v", err)
		}
	}

	_, err = atomicBackend.GetUniqueRandomNumber(context.Background(), 2)
	if !errors.Is(err, dkit.ErrNoAvailableNumber) {
		t.Fatalf("want ErrNoAvailableNumber got %v", err)
	}
}

func TestNewRedisAtomic(t *testing.T) {
	_, err := NewRedisAtomic()
	if err == nil {
		t.Fatalf("expected error for missing client")
	}
	if !dkitErrorIsInvalidOption(err) {
		t.Fatalf("want invalid option got %v", err)
	}
}

func TestRedisAtomic_ElectionNotEnabled(t *testing.T) {
	_, cli := testRedisClient(t)
	atomicBackend, err := NewRedisAtomic(Client(cli), KeyPrefix(testPrefix(t)), TTL(time.Second))
	if err != nil {
		t.Fatalf("new redis atomic: %v", err)
	}
	if atomicBackend.NodeKey() != "" {
		t.Fatalf("node key should be empty before election enabled")
	}
	if atomicBackend.IsLeader() {
		t.Fatalf("leader should be false before election enabled")
	}
	if _, err := atomicBackend.AliveNodes(); err != dkit.ErrElectionNotEnabled {
		t.Fatalf("alive nodes want ErrElectionNotEnabled got %v", err)
	}
	if ok, err := atomicBackend.IsAlive("node"); ok || err != dkit.ErrElectionNotEnabled {
		t.Fatalf("is alive want false and ErrElectionNotEnabled got %v %v", ok, err)
	}
}

func TestRedisAtomic_EnableElectionTwice(t *testing.T) {
	_, cli := testRedisClient(t)
	atomicBackend, err := NewRedisAtomic(Client(cli), KeyPrefix(testPrefix(t)), TTL(time.Second))
	if err != nil {
		t.Fatalf("new redis atomic: %v", err)
	}
	defer func() {
		if err := atomicBackend.Close(); err != nil {
			t.Fatalf("close atomic: %v", err)
		}
	}()

	opt := &dkit.ElectionOption{
		NodeKey:       "node-1",
		KeepHeartbeat: true,
		UnhealthyTime: 200 * time.Millisecond,
		Timeout:       time.Second,
	}
	if err := atomicBackend.EnableElection(opt); err != nil {
		t.Fatalf("enable election first time: %v", err)
	}
	if err := atomicBackend.EnableElection(opt); !errors.Is(err, dkit.ErrInvalidOption) {
		t.Fatalf("enable election second time want ErrInvalidOption got %v", err)
	}
}

func dkitErrorIsInvalidOption(err error) bool {
	return errors.Is(err, dkit.ErrInvalidOption)
}
