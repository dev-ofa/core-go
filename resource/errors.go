package resource

import (
	"errors"
	"fmt"
)

var (
	// ErrUnsupportedScheme means no handler is registered for the resource scheme.
	ErrUnsupportedScheme = errors.New("resource: unsupported scheme")
	// ErrOpenUnsupported means a handler does not implement opening.
	ErrOpenUnsupported = errors.New("resource: open unsupported")
	// ErrUploadUnsupported means a handler does not implement uploading.
	ErrUploadUnsupported = errors.New("resource: upload unsupported")
	// ErrSizeLimitExceeded means a resource body exceeded the configured size limit.
	ErrSizeLimitExceeded = errors.New("resource: size limit exceeded")
	// ErrTimeoutBudgetExhausted means the current context deadline has no remaining budget.
	ErrTimeoutBudgetExhausted = errors.New("resource: timeout budget exhausted")
)

// ParseError wraps resource identifier parse failures.
type ParseError struct {
	Raw string
	Err error
}

// Error implements error.
func (e *ParseError) Error() string {
	return fmt.Sprintf("resource parse failed: %v", e.Err)
}

// Unwrap returns the root cause.
func (e *ParseError) Unwrap() error {
	return e.Err
}

// OpenError wraps resource open failures.
type OpenError struct {
	Identifier Identifier
	Err        error
}

// Error implements error.
func (e *OpenError) Error() string {
	return fmt.Sprintf("resource open scheme=%s failed: %v", e.Identifier.Scheme, e.Err)
}

// Unwrap returns the root cause.
func (e *OpenError) Unwrap() error {
	return e.Err
}

// DownloadError wraps download failures.
type DownloadError struct {
	DstPath string
	Err     error
}

// Error implements error.
func (e *DownloadError) Error() string {
	return fmt.Sprintf("resource download dst=%s failed: %v", e.DstPath, e.Err)
}

// Unwrap returns the root cause.
func (e *DownloadError) Unwrap() error {
	return e.Err
}

// UploadError wraps upload failures.
type UploadError struct {
	Scheme string
	Err    error
}

// Error implements error.
func (e *UploadError) Error() string {
	return fmt.Sprintf("resource upload scheme=%s failed: %v", e.Scheme, e.Err)
}

// Unwrap returns the root cause.
func (e *UploadError) Unwrap() error {
	return e.Err
}
