package resource

import (
	"fmt"

	"github.com/dev-ofa/core-go/model/datax"
)

const (
	// ErrCodeResourceUnsupportedScheme means no handler is registered for the resource scheme.
	ErrCodeResourceUnsupportedScheme = 20200
	// ErrCodeResourceOpenUnsupported means a handler does not implement opening.
	ErrCodeResourceOpenUnsupported = 20201
	// ErrCodeResourceUploadUnsupported means a handler does not implement uploading.
	ErrCodeResourceUploadUnsupported = 20202
	// ErrCodeResourceSizeLimitExceeded means a resource body exceeded the configured size limit.
	ErrCodeResourceSizeLimitExceeded = 20203
	// ErrCodeResourceTimeoutBudgetExhausted means the current context deadline has no remaining budget.
	ErrCodeResourceTimeoutBudgetExhausted = 10120
)

var (
	// ErrUnsupportedScheme means no handler is registered for the resource scheme.
	ErrUnsupportedScheme = datax.NewError(ErrCodeResourceUnsupportedScheme, "resource: unsupported scheme", nil)
	// ErrOpenUnsupported means a handler does not implement opening.
	ErrOpenUnsupported = datax.NewError(ErrCodeResourceOpenUnsupported, "resource: open unsupported", nil)
	// ErrUploadUnsupported means a handler does not implement uploading.
	ErrUploadUnsupported = datax.NewError(ErrCodeResourceUploadUnsupported, "resource: upload unsupported", nil)
	// ErrSizeLimitExceeded means a resource body exceeded the configured size limit.
	ErrSizeLimitExceeded = datax.NewError(ErrCodeResourceSizeLimitExceeded, "resource: size limit exceeded", nil)
	// ErrTimeoutBudgetExhausted means the current context deadline has no remaining budget.
	ErrTimeoutBudgetExhausted = datax.NewError(ErrCodeResourceTimeoutBudgetExhausted, "resource: timeout budget exhausted", nil)
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
