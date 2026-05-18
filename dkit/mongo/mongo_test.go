package mongo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dev-ofa/core-go/dkit"
)

func TestMongoAtomic_GetUniqueRandomNumber(t *testing.T) {
	db := testDatabase(t)
	prefix := testPrefix(t)
	dropCollections(t, db, prefix+"_random", prefix+"_mutex")

	atomic, err := NewMongoAtomic(Database(db), CollectionPrefix(prefix))
	if err != nil {
		t.Fatalf("new mongo atomic: %v", err)
	}
	defer func() {
		if err := atomic.Close(); err != nil {
			t.Fatalf("close atomic: %v", err)
		}
	}()

	max := 20
	seen := map[int]bool{}
	for i := 0; i < 5; i++ {
		num, err := atomic.GetUniqueRandomNumber(context.Background(), max)
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

func TestNewMongoAtomic(t *testing.T) {
	_, err := NewMongoAtomic()
	if err == nil {
		t.Fatalf("expected error for missing database")
	}
	if !dkitErrorIsInvalidOption(err) {
		t.Fatalf("want invalid option got %v", err)
	}
}

func TestMongoAtomic_ElectionNotEnabled(t *testing.T) {
	db := testDatabase(t)
	prefix := testPrefix(t)
	dropCollections(t, db, prefix+"_random", prefix+"_mutex")

	atomic, err := NewMongoAtomic(Database(db), CollectionPrefix(prefix), TTL(time.Second))
	if err != nil {
		t.Fatalf("new mongo atomic: %v", err)
	}
	if atomic.NodeKey() != "" {
		t.Fatalf("node key should be empty before election enabled")
	}
	if atomic.IsLeader() {
		t.Fatalf("leader should be false before election enabled")
	}
	if _, err := atomic.AliveNodes(); err != dkit.ErrElectionNotEnabled {
		t.Fatalf("alive nodes want ErrElectionNotEnabled got %v", err)
	}
	if ok, err := atomic.IsAlive("node"); ok || err != dkit.ErrElectionNotEnabled {
		t.Fatalf("is alive want false and ErrElectionNotEnabled got %v %v", ok, err)
	}
}

func dkitErrorIsInvalidOption(err error) bool {
	return errors.Is(err, dkit.ErrInvalidOption)
}
