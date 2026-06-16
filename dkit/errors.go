package dkit

import "github.com/dev-ofa/core-go/model/datax"

const (
	// ErrCodeDKitInvalidOption indicates invalid DKit options or configuration.
	ErrCodeDKitInvalidOption = 20100
	// ErrCodeDKitLockNotAcquired indicates that a non-blocking lock attempt did not acquire the lock.
	ErrCodeDKitLockNotAcquired = 20101
	// ErrCodeDKitAlreadyUnlocked indicates that the lock has already been released or expired.
	ErrCodeDKitAlreadyUnlocked = 20102
	// ErrCodeDKitElectionNotEnabled indicates that election APIs are used before election is enabled.
	ErrCodeDKitElectionNotEnabled = 20103
	// ErrCodeDKitNoAvailableNumber indicates that the bounded number range is temporarily exhausted.
	ErrCodeDKitNoAvailableNumber = 20104
	// ErrCodeDKitBackendUnavailable indicates that the coordination backend is unavailable.
	ErrCodeDKitBackendUnavailable = 10100
	// ErrCodeDKitDefaultKitNotConfigured indicates that the provider-owned default Kit has not been initialized.
	ErrCodeDKitDefaultKitNotConfigured = 10101
)

// ErrInvalidOption indicates that a DKit option is invalid.
var ErrInvalidOption = datax.NewError(ErrCodeDKitInvalidOption, "dkit: invalid option", nil)

// ErrLockNotAcquired indicates that a non-blocking lock attempt did not acquire the lock.
var ErrLockNotAcquired = datax.NewError(ErrCodeDKitLockNotAcquired, "dkit: lock not acquired", nil)

// ErrAlreadyUnlocked indicates that the lock has already been released or expired.
var ErrAlreadyUnlocked = datax.NewError(ErrCodeDKitAlreadyUnlocked, "dkit: already unlocked", nil)

// ErrElectionNotEnabled indicates that election or heartbeat APIs are used before election is enabled.
var ErrElectionNotEnabled = datax.NewError(ErrCodeDKitElectionNotEnabled, "dkit: election not enabled", nil)

// ErrBackendUnavailable indicates that the underlying coordination backend is unavailable.
var ErrBackendUnavailable = datax.NewError(ErrCodeDKitBackendUnavailable, "dkit: backend unavailable", nil)

// ErrNoAvailableNumber indicates that the bounded number range is temporarily exhausted.
var ErrNoAvailableNumber = datax.NewError(ErrCodeDKitNoAvailableNumber, "dkit: no available number", nil)

// ErrDefaultKitNotConfigured indicates that the provider-owned default Kit has not been initialized.
var ErrDefaultKitNotConfigured = datax.NewError(ErrCodeDKitDefaultKitNotConfigured, "dkit: default kit not configured", nil)
