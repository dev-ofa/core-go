package dkit

import (
	"context"
	"sync"
)

var defaultKitLocator struct {
	mu  sync.RWMutex
	kit Kit
}

// InitDefaultKit initializes and registers the provider-owned default Kit.
//
// The default Kit is only replaced after initialization succeeds. Callers that
// need explicit ownership can continue using NewDefaultKitWithContext directly.
func InitDefaultKit(ctx context.Context, atomic Atomic) (Kit, error) {
	kit, err := NewDefaultKitWithContext(ctx, atomic)
	if err != nil {
		return nil, err
	}
	SetDefaultKit(kit)
	return kit, nil
}

// SetDefaultKit replaces the provider-owned default Kit.
//
// It is intended for application bootstrap and tests that need controlled
// replacement. Passing nil clears the default Kit.
func SetDefaultKit(kit Kit) {
	defaultKitLocator.mu.Lock()
	defaultKitLocator.kit = kit
	defaultKitLocator.mu.Unlock()
}

// DefaultKit returns the provider-owned default Kit.
//
// DefaultKit panics when the default Kit has not been initialized. Applications
// should call SetDefaultKit or InitDefaultKit during bootstrap before business
// code attempts to discover DKit.
func DefaultKit() Kit {
	defaultKitLocator.mu.RLock()
	kit := defaultKitLocator.kit
	defaultKitLocator.mu.RUnlock()
	if kit == nil {
		panic(ErrDefaultKitNotConfigured)
	}
	return kit
}

// ResetDefaultKit clears the provider-owned default Kit.
//
// ResetDefaultKit does not close the previous Kit; the bootstrap code that owns
// the instance remains responsible for closing it.
func ResetDefaultKit() {
	SetDefaultKit(nil)
}
