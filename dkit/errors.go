package dkit

import "errors"

// ErrInvalidOption indicates that a DKit option is invalid.
var ErrInvalidOption = errors.New("dkit: invalid option")

// ErrLockNotAcquired indicates that a non-blocking lock attempt did not acquire the lock.
var ErrLockNotAcquired = errors.New("dkit: lock not acquired")

// ErrAlreadyUnlocked indicates that the lock has already been released or expired.
var ErrAlreadyUnlocked = errors.New("dkit: already unlocked")

// ErrElectionNotEnabled indicates that election or heartbeat APIs are used before election is enabled.
var ErrElectionNotEnabled = errors.New("dkit: election not enabled")

// ErrBackendUnavailable indicates that the underlying coordination backend is unavailable.
var ErrBackendUnavailable = errors.New("dkit: backend unavailable")

// ErrNoAvailableNumber indicates that the bounded number range is temporarily exhausted.
var ErrNoAvailableNumber = errors.New("dkit: no available number")
