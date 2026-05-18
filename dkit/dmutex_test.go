package dkit

import (
	"testing"
	"time"
)

func TestNewLockOption(t *testing.T) {
	opt := NewLockOption(0, nil)
	if opt.TTL != DefaultLockTTL {
		t.Fatalf("default ttl want %s got %s", DefaultLockTTL, opt.TTL)
	}
	if opt.SpinInterval != DefaultLockSpinInterval {
		t.Fatalf("default spin interval want %s got %s", DefaultLockSpinInterval, opt.SpinInterval)
	}

	opt = NewLockOption(time.Second, []LockOptionOp{
		LockTTL(2 * time.Second),
		LockWithMaxWait(3 * time.Second),
		LockWithSpinInterval(100 * time.Millisecond),
		Reentrant("holder-1"),
	})
	if opt.TTL != 2*time.Second {
		t.Fatalf("ttl want 2s got %s", opt.TTL)
	}
	if opt.MaxWaitTime != 3*time.Second {
		t.Fatalf("max wait want 3s got %s", opt.MaxWaitTime)
	}
	if opt.SpinInterval != 100*time.Millisecond {
		t.Fatalf("spin interval want 100ms got %s", opt.SpinInterval)
	}
	if opt.ReentrantIdentity != "holder-1" {
		t.Fatalf("reentrant identity want holder-1 got %s", opt.ReentrantIdentity)
	}
}
