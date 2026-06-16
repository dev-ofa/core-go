package dkit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dev-ofa/core-go/model/datax"
)

type fakeAtomic struct {
	num       int
	numErr    error
	mutex     *fakeMutex
	defaultTT time.Duration
}

func (f *fakeAtomic) GetUniqueRandomNumber(context.Context, int) (int, error) {
	return f.num, f.numErr
}

func (f *fakeAtomic) NewMutex(string) DistributedMutex {
	if f.mutex == nil {
		return nil
	}
	return f.mutex
}

func (f *fakeAtomic) EnableElection(*ElectionOption) error {
	return nil
}

func (f *fakeAtomic) NodeKey() string {
	return "node-1"
}

func (f *fakeAtomic) IsLeader() bool {
	return false
}

func (f *fakeAtomic) AliveNodes() ([]string, error) {
	return nil, ErrElectionNotEnabled
}

func (f *fakeAtomic) IsAlive(string) (bool, error) {
	return false, ErrElectionNotEnabled
}

func (f *fakeAtomic) GetMutexDefaultTTL() time.Duration {
	if f.defaultTT > 0 {
		return f.defaultTT
	}
	return DefaultLockTTL
}

func (f *fakeAtomic) Close() error {
	return nil
}

type fakeMutex struct {
	lockErr   error
	tryOK     bool
	tryErr    error
	unlockErr error

	lockCount   int
	tryCount    int
	unlockCount int
}

func (f *fakeMutex) Lock(context.Context, ...LockOptionOp) error {
	f.lockCount++
	return f.lockErr
}

func (f *fakeMutex) TryLock(context.Context, ...LockOptionOp) (bool, error) {
	f.tryCount++
	return f.tryOK, f.tryErr
}

func (f *fakeMutex) Unlock(context.Context) error {
	f.unlockCount++
	return f.unlockErr
}

func (f *fakeMutex) ExistLock(context.Context) (bool, error) {
	return f.lockCount > 0 || f.tryOK, nil
}

func TestNewDefaultKitWithContext(t *testing.T) {
	t.Run("nil atomic returns invalid option", func(t *testing.T) {
		_, err := NewDefaultKitWithContext(context.Background(), nil)
		if !errors.Is(err, ErrInvalidOption) {
			t.Fatalf("want ErrInvalidOption got %v", err)
		}
		if datax.CodeOf(err) != ErrCodeDKitInvalidOption {
			t.Fatalf("want invalid option code got %d", datax.CodeOf(err))
		}
	})

	t.Run("allocator error is returned", func(t *testing.T) {
		allocErr := errors.New("allocate failed")
		_, err := NewDefaultKitWithContext(context.Background(), &fakeAtomic{numErr: allocErr})
		if !errors.Is(err, allocErr) {
			t.Fatalf("want allocator error got %v", err)
		}
	})

	t.Run("machine id out of range is rejected", func(t *testing.T) {
		_, err := NewDefaultKitWithContext(context.Background(), &fakeAtomic{num: -1})
		if !errors.Is(err, ErrInvalidOption) {
			t.Fatalf("want ErrInvalidOption got %v", err)
		}
	})

	t.Run("generates string id", func(t *testing.T) {
		kit, err := NewDefaultKitWithContext(context.Background(), &fakeAtomic{num: 1, mutex: &fakeMutex{}})
		if err != nil {
			t.Fatalf("new kit: %v", err)
		}
		id := kit.GetIDString()
		if id == "" {
			t.Fatalf("id should not be empty")
		}
	})

	t.Run("generates snowflake id type", func(t *testing.T) {
		kit, err := NewDefaultKitWithContext(context.Background(), &fakeAtomic{num: 1, mutex: &fakeMutex{}})
		if err != nil {
			t.Fatalf("new kit: %v", err)
		}
		id := kit.GetSnowflakeID()
		if id == "" {
			t.Fatalf("id should not be zero")
		}
		if len(id.String()) != 19 {
			t.Fatalf("id should be 19 digits with the fixed snowflake epoch, got %s", id.String())
		}
		if kit.GetSnowflakeID() == "" {
			t.Fatalf("panic helper id should not be zero")
		}
		if _, err := kit.NextID(context.Background()); err != nil {
			t.Fatalf("compat next id: %v", err)
		}
		if id, err := kit.NextIDString(context.Background()); err != nil || strings.TrimSpace(id) == "" {
			t.Fatalf("compat next id string got id=%q err=%v", id, err)
		}
	})

	t.Run("compat next id respects canceled context", func(t *testing.T) {
		kit, err := NewDefaultKitWithContext(context.Background(), &fakeAtomic{num: 1, mutex: &fakeMutex{}})
		if err != nil {
			t.Fatalf("new kit: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = kit.NextID(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context canceled got %v", err)
		}
	})
}

func TestDefaultKitLocator(t *testing.T) {
	t.Cleanup(ResetDefaultKit)
	ResetDefaultKit()

	assertPanicIs(t, ErrDefaultKitNotConfigured, func() {
		_ = DefaultKit()
	})
	assertPanicCodeIs(t, ErrCodeDKitDefaultKitNotConfigured, func() {
		_ = DefaultKit()
	})

	kit, err := NewDefaultKit(&fakeAtomic{num: 1, mutex: &fakeMutex{}})
	if err != nil {
		t.Fatalf("new kit: %v", err)
	}
	SetDefaultKit(kit)

	got := DefaultKit()
	if got != kit {
		t.Fatalf("default kit mismatch")
	}
	id := DefaultKit().GetIDString()
	if strings.TrimSpace(id) == "" {
		t.Fatalf("id should not be empty")
	}

	ResetDefaultKit()
	assertPanicIs(t, ErrDefaultKitNotConfigured, func() {
		_ = DefaultKit()
	})
}

func TestInitDefaultKit(t *testing.T) {
	t.Cleanup(ResetDefaultKit)
	ResetDefaultKit()

	kit, err := InitDefaultKit(context.Background(), &fakeAtomic{num: 1, mutex: &fakeMutex{}})
	if err != nil {
		t.Fatalf("init default kit: %v", err)
	}
	if got := DefaultKit(); got != kit {
		t.Fatalf("default kit got %v", got)
	}

	allocErr := errors.New("allocate failed")
	if _, err := InitDefaultKit(context.Background(), &fakeAtomic{numErr: allocErr}); !errors.Is(err, allocErr) {
		t.Fatalf("want allocator error got %v", err)
	}
	if got := DefaultKit(); got != kit {
		t.Fatalf("failed init should keep previous default, got %v", got)
	}
}

func assertPanicIs(t *testing.T, target error, fn func()) {
	t.Helper()
	defer func() {
		recovered := recover()
		if !errors.Is(recovered.(error), target) {
			t.Fatalf("want panic %v got %v", target, recovered)
		}
	}()
	fn()
}

func assertPanicCodeIs(t *testing.T, code int, fn func()) {
	t.Helper()
	defer func() {
		recovered := recover()
		err, ok := recovered.(error)
		if !ok {
			t.Fatalf("want panic error got %v", recovered)
		}
		if datax.CodeOf(err) != code {
			t.Fatalf("want panic code %d got %d", code, datax.CodeOf(err))
		}
	}()
	fn()
}

func TestDefaultKitMutexCtxTryDo(t *testing.T) {
	t.Run("lock not acquired skips action", func(t *testing.T) {
		mux := &fakeMutex{tryOK: false}
		kit, err := NewDefaultKit(&fakeAtomic{num: 1, mutex: mux})
		if err != nil {
			t.Fatalf("new kit: %v", err)
		}
		called := false
		ok, err := kit.MutexCtxTryDo(context.Background(), "job", func(context.Context) error {
			called = true
			return nil
		})
		if err != nil {
			t.Fatalf("try do: %v", err)
		}
		if ok {
			t.Fatalf("lock should not be acquired")
		}
		if called {
			t.Fatalf("action should not be called")
		}
		if mux.unlockCount != 0 {
			t.Fatalf("unlock should not be called")
		}
	})

	t.Run("success runs action and unlocks", func(t *testing.T) {
		mux := &fakeMutex{tryOK: true}
		kit, err := NewDefaultKit(&fakeAtomic{num: 1, mutex: mux})
		if err != nil {
			t.Fatalf("new kit: %v", err)
		}
		called := false
		ok, err := kit.MutexCtxTryDo(context.Background(), "job", func(context.Context) error {
			called = true
			return nil
		})
		if err != nil {
			t.Fatalf("try do: %v", err)
		}
		if !ok || !called {
			t.Fatalf("action should run after lock is acquired")
		}
		if mux.unlockCount != 1 {
			t.Fatalf("unlock count want 1 got %d", mux.unlockCount)
		}
	})

	t.Run("action error is returned and lock is released", func(t *testing.T) {
		actionErr := errors.New("action failed")
		mux := &fakeMutex{tryOK: true}
		kit, err := NewDefaultKit(&fakeAtomic{num: 1, mutex: mux})
		if err != nil {
			t.Fatalf("new kit: %v", err)
		}
		ok, err := kit.MutexCtxTryDo(context.Background(), "job", func(context.Context) error {
			return actionErr
		})
		if !ok {
			t.Fatalf("lock should be acquired")
		}
		if !errors.Is(err, actionErr) {
			t.Fatalf("want action error got %v", err)
		}
		if mux.unlockCount != 1 {
			t.Fatalf("unlock count want 1 got %d", mux.unlockCount)
		}
	})

	t.Run("unlock error after action error keeps action error in chain", func(t *testing.T) {
		actionErr := errors.New("action failed")
		unlockErr := errors.New("unlock failed")
		mux := &fakeMutex{tryOK: true, unlockErr: unlockErr}
		kit, err := NewDefaultKit(&fakeAtomic{num: 1, mutex: mux})
		if err != nil {
			t.Fatalf("new kit: %v", err)
		}
		_, err = kit.MutexCtxTryDo(context.Background(), "job", func(context.Context) error {
			return actionErr
		})
		if !errors.Is(err, actionErr) {
			t.Fatalf("want action error in chain got %v", err)
		}
		if !strings.Contains(err.Error(), unlockErr.Error()) {
			t.Fatalf("want unlock error text got %v", err)
		}
	})
}

func TestDefaultKitMutexCtxDo(t *testing.T) {
	t.Run("lock error skips action", func(t *testing.T) {
		lockErr := errors.New("lock failed")
		mux := &fakeMutex{lockErr: lockErr}
		kit, err := NewDefaultKit(&fakeAtomic{num: 1, mutex: mux})
		if err != nil {
			t.Fatalf("new kit: %v", err)
		}
		called := false
		err = kit.MutexCtxDo(context.Background(), "job", func(context.Context) error {
			called = true
			return nil
		})
		if !errors.Is(err, lockErr) {
			t.Fatalf("want lock error got %v", err)
		}
		if called {
			t.Fatalf("action should not run when lock fails")
		}
		if mux.unlockCount != 0 {
			t.Fatalf("unlock should not be called")
		}
	})

	t.Run("success runs action and unlocks", func(t *testing.T) {
		mux := &fakeMutex{}
		kit, err := NewDefaultKit(&fakeAtomic{num: 1, mutex: mux})
		if err != nil {
			t.Fatalf("new kit: %v", err)
		}
		called := false
		err = kit.MutexCtxDo(context.Background(), "job", func(context.Context) error {
			called = true
			return nil
		})
		if err != nil {
			t.Fatalf("do: %v", err)
		}
		if !called {
			t.Fatalf("action should run")
		}
		if mux.lockCount != 1 || mux.unlockCount != 1 {
			t.Fatalf("lock/unlock counts want 1/1 got %d/%d", mux.lockCount, mux.unlockCount)
		}
	})

	t.Run("nil action is rejected", func(t *testing.T) {
		kit, err := NewDefaultKit(&fakeAtomic{num: 1, mutex: &fakeMutex{}})
		if err != nil {
			t.Fatalf("new kit: %v", err)
		}
		err = kit.MutexCtxDo(context.Background(), "job", nil)
		if !errors.Is(err, ErrInvalidOption) {
			t.Fatalf("want ErrInvalidOption got %v", err)
		}
	})

	t.Run("nil mutex is rejected", func(t *testing.T) {
		kit, err := NewDefaultKit(&fakeAtomic{num: 1})
		if err != nil {
			t.Fatalf("new kit: %v", err)
		}
		err = kit.MutexCtxDo(nil, "job", func(context.Context) error {
			return nil
		})
		if !errors.Is(err, ErrInvalidOption) {
			t.Fatalf("want ErrInvalidOption got %v", err)
		}
	})
}
