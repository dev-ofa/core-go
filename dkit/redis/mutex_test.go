package redis

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dev-ofa/core-go/dkit"
)

func TestRedisMutex_Lock(t *testing.T) {
	_, cli := testRedisClient(t)
	impl := NewMutexImpl(cli, testPrefix(t), time.Minute)
	if err := impl.Init(); err != nil {
		t.Fatalf("init mutex impl: %v", err)
	}

	parallelCnt := 3
	errCh := make(chan error, parallelCnt)
	wg := &sync.WaitGroup{}
	wg.Add(parallelCnt)
	for i := 0; i < parallelCnt; i++ {
		go func() {
			defer wg.Done()
			mux := impl.NewMutex("lock")
			err := mux.Lock(context.Background(), dkit.LockWithSpinInterval(10*time.Millisecond), dkit.LockWithMaxWait(2*time.Second))
			if err != nil {
				errCh <- err
				return
			}
			exist, err := mux.ExistLock(context.Background())
			if err != nil {
				errCh <- err
				return
			}
			if !exist {
				errCh <- errors.New("lock should exist")
				return
			}
			time.Sleep(20 * time.Millisecond)
			if err := mux.Unlock(context.Background()); err != nil {
				errCh <- err
				return
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("mutex worker failed: %v", err)
		}
	}
}

func TestRedisMutex_TryLockReentrantAndExpired(t *testing.T) {
	srv, cli := testRedisClient(t)
	impl := NewMutexImpl(cli, testPrefix(t), 100*time.Millisecond)
	if err := impl.Init(); err != nil {
		t.Fatalf("init mutex impl: %v", err)
	}

	first := impl.NewMutex("lock")
	ok, err := first.TryLock(context.Background(), dkit.Reentrant("worker-1"))
	if err != nil {
		t.Fatalf("first try lock: %v", err)
	}
	if !ok {
		t.Fatalf("first lock should be acquired")
	}

	second := impl.NewMutex("lock")
	ok, err = second.TryLock(context.Background())
	if err != nil {
		t.Fatalf("second try lock: %v", err)
	}
	if ok {
		t.Fatalf("second lock should not be acquired while first lease is alive")
	}

	reentrant := impl.NewMutex("lock")
	ok, err = reentrant.TryLock(context.Background(), dkit.Reentrant("worker-1"))
	if err != nil {
		t.Fatalf("reentrant try lock: %v", err)
	}
	if !ok {
		t.Fatalf("same reentrant identity should acquire lock")
	}

	srv.FastForward(150 * time.Millisecond)
	expired := impl.NewMutex("lock")
	ok, err = expired.TryLock(context.Background())
	if err != nil {
		t.Fatalf("expired try lock: %v", err)
	}
	if !ok {
		t.Fatalf("expired lock should be acquired")
	}
}

func TestRedisMutex_SameInstanceRetryDoesNotLoseOwnership(t *testing.T) {
	_, cli := testRedisClient(t)
	impl := NewMutexImpl(cli, testPrefix(t), time.Minute)
	if err := impl.Init(); err != nil {
		t.Fatalf("init mutex impl: %v", err)
	}

	mux := impl.NewMutex("lock")
	ok, err := mux.TryLock(context.Background())
	if err != nil {
		t.Fatalf("first try lock: %v", err)
	}
	if !ok {
		t.Fatalf("first lock should be acquired")
	}

	ok, err = mux.TryLock(context.Background())
	if err != nil {
		t.Fatalf("second try lock: %v", err)
	}
	if !ok {
		t.Fatalf("same instance should still report ownership")
	}

	if err := mux.Unlock(context.Background()); err != nil {
		t.Fatalf("unlock should succeed after repeated try lock: %v", err)
	}
}
