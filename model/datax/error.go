package datax

import (
	"errors"
	"fmt"
)

// - 对于预期内or不需要告警的错误，Body中业务错误码请使用 >= 20000 以上的数值，如 20100, 20300
// - 对于非预期or需要告警的业务错误，Body中业务错误码请使用 < 20000 的数值，推荐 >= 10000，避免与RFC和字节基础设施中的某些StatusCode定义冲突
const (
	// ErrCodeUnexpected is the default unexpected error code.
	ErrCodeUnexpected = 10000
	// ErrCodeInternal is kept as a compatibility alias for the default unexpected error code.
	ErrCodeInternal = ErrCodeUnexpected

	// ErrCodeExpected is the default expected error code.
	ErrCodeExpected = 20000
	// ErrCodeValidate mean that the format of request's parameter is not validated(e.g. not match business logic)
	ErrCodeValidate = 20001
	// ErrCodeNotFound mean that the record you querying does not found
	ErrCodeNotFound = 20002
	// ErrCodeConflict mean that the record you want to insert/update is conflicted with others
	ErrCodeConflict = 20003
)

var (
	// ErrNotFound is kept for compatibility with callers that compare shared not-found errors.
	ErrNotFound = &BaseError{Code: ErrCodeNotFound, Message: "data not found"}
	// ErrConflict is kept for compatibility with callers that compare shared conflict errors.
	ErrConflict = &BaseError{Code: ErrCodeConflict, Message: "data is existed or has be updated"}
)

// CodedError is implemented by errors that carry a stable code.
type CodedError interface {
	error
	ErrorCode() int
}

// CauseError is implemented by errors that wrap a cause.
type CauseError interface {
	error
	Unwrap() error
}

// CoreError is the core error type with a stable code, message, and cause.
type CoreError struct {
	// Code is the stable error code.
	Code int
	// Message is the human-readable message.
	Message string
	// Cause is the original error.
	Cause error
}

// Error implements error.
func (e *CoreError) Error() string {
	if e.Message != "" && e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("error code %d", e.Code)
}

// Unwrap exposes the source error.
func (e *CoreError) Unwrap() error {
	return e.Cause
}

// ErrorCode returns the stable error code.
func (e *CoreError) ErrorCode() int {
	return e.Code
}

// Is checks whether target carries the same error code.
func (e *CoreError) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}
	var coded CodedError
	return e.Code != 0 && errors.As(target, &coded) && coded.ErrorCode() == e.Code
}

// NewError returns a core error.
func NewError(code int, message string, cause error) *CoreError {
	return &CoreError{Code: code, Message: message, Cause: cause}
}

// BaseError is the legacy common error type with code and message.
type BaseError struct {
	// Code is the business error code.
	Code int
	// Message is the human-readable message.
	Message string
	// Data carries extra error details.
	Data interface{}
	// SourceSrv is the upstream service name if any.
	SourceSrv string
}

// Error implements error.
func (e *BaseError) Error() string {
	if e.SourceSrv != "" {
		return fmt.Sprintf("call: %s failed, code: %d, msg: %s", e.SourceSrv, e.Code, e.Message)
	}
	return e.Message
}

// ErrorCode returns the stable error code.
func (e *BaseError) ErrorCode() int {
	return e.Code
}

// Is checks whether target carries the same error code.
func (e *BaseError) Is(target error) bool {
	if e == nil || target == nil {
		return false
	}
	var coded CodedError
	return e.Code != 0 && errors.As(target, &coded) && coded.ErrorCode() == e.Code
}

// ErrWrapper is the legacy wrapper validation error shape.
type ErrWrapper struct {
	// Code is the business error code.
	Code int
	// Msg is the error message.
	Msg string
	// Data carries extra error details.
	Data interface{}
}

// Error implements error.
func (e *ErrWrapper) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("wrapper validate failed, code: [%d], msg : [%s], data: [%+v]", e.Code, e.Msg, e.Data)
	}

	return fmt.Sprintf("wrapper validate failed, code: [%d], msg : [%s]", e.Code, e.Msg)
}

// ErrorCode returns the stable error code.
func (e *ErrWrapper) ErrorCode() int {
	return e.Code
}

// ErrHttp reports HTTP validation errors from external calls.
type ErrHttp struct {
	CoreError
	// StatusCode is the HTTP status code.
	StatusCode int
	// Body is the raw response body.
	Body []byte
}

// Error implements error.
func (e *ErrHttp) Error() string {
	if len(e.Body) == 0 {
		return fmt.Sprintf("http validate failed, status: [%d]", e.StatusCode)
	}
	return fmt.Sprintf("http validate failed, status: [%d], body : [%s]", e.StatusCode, string(e.Body))
}

// NewErrHttp returns an HTTP protocol response error.
func NewErrHttp(statusCode int, body []byte) *ErrHttp {
	return &ErrHttp{
		CoreError:  CoreError{Code: ErrCodeUnexpected, Message: "http validate failed"},
		StatusCode: statusCode,
		Body:       body,
	}
}

// ErrCall is the legacy upstream call failure shape.
type ErrCall struct {
	// Url is the request URL.
	Url string
	// RequestID is the upstream request id.
	RequestID string
	// Method is the HTTP method.
	Method string

	// SrcErr is the original error.
	SrcErr error
}

// Error implements error.
func (e *ErrCall) Error() string {
	return fmt.Sprintf("%s [%s] failed, reqid:[%s], source err: [%s]", e.Method, e.Url, e.RequestID, e.SrcErr)
}

// Unwrap exposes the source error.
func (e *ErrCall) Unwrap() error {
	return e.SrcErr
}

// RetryableError marks an error as safe to retry.
type RetryableError struct {
	// Cause is the retryable error.
	Cause error
}

// Error implements error.
func (e *RetryableError) Error() string {
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "retryable error"
}

// Unwrap exposes the source error.
func (e *RetryableError) Unwrap() error {
	return e.Cause
}

// WithRetryableError wraps err as safe to retry.
func WithRetryableError(err error) error {
	if err == nil {
		return nil
	}
	return &RetryableError{Cause: err}
}

// IsRetryableError checks whether err is explicitly marked as safe to retry.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	retryableErr := &RetryableError{}
	return errors.As(err, &retryableErr)
}

// ExtraDataError wraps an error with structured extra data.
type ExtraDataError struct {
	// Cause is the original error.
	Cause error
	// Data carries structured extra data.
	Data interface{}
}

// Error implements error.
func (e *ExtraDataError) Error() string {
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "error with extra data"
}

// Unwrap exposes the source error.
func (e *ExtraDataError) Unwrap() error {
	return e.Cause
}

// WithErrorData wraps err with structured extra data.
func WithErrorData(err error, data interface{}) error {
	if err == nil {
		return nil
	}
	return &ExtraDataError{Cause: err, Data: data}
}

// ErrorData returns the first structured data found in err's chain.
func ErrorData(err error) interface{} {
	if err == nil {
		return nil
	}

	extraErr := &ExtraDataError{}
	if errors.As(err, &extraErr) {
		return extraErr.Data
	}

	return nil
}

// ValidationError is an expected error for invalid input or precondition failures.
type ValidationError struct {
	CoreError
	// Items describes invalid fields or validation rules.
	Items []ValidateErrItem
}

// ValidateError is kept as a compatibility alias for the legacy typed validation error name.
type ValidateError = ValidationError

// NewValidationError returns an expected validation error.
func NewValidationError(message string, items []ValidateErrItem, cause error) *ValidationError {
	if message == "" {
		message = "validation failed"
	}
	return &ValidationError{
		CoreError: CoreError{Code: ErrCodeValidate, Message: message, Cause: cause},
		Items:     items,
	}
}

// ResourceError is an expected error for resource failures.
type ResourceError struct {
	CoreError
	// Resource identifies the missing resource when available.
	Resource string
}

// NewResourceError returns an expected resource error.
func NewResourceError(code int, message string, resource string, cause error) *ResourceError {
	if code == 0 {
		code = ErrCodeExpected
	}
	if message == "" {
		message = "resource error"
	}
	if resource != "" {
		message = fmt.Sprintf("%s: %s", message, resource)
	}
	return &ResourceError{
		CoreError: CoreError{Code: code, Message: message, Cause: cause},
		Resource:  resource,
	}
}

// NewResourceNotFoundError returns an expected resource-not-found error.
func NewResourceNotFoundError(resource string, cause error) *ResourceError {
	return NewResourceError(ErrCodeNotFound, "resource not found", resource, cause)
}

// NewResourceConflictError returns an expected resource-conflict error.
func NewResourceConflictError(resource string, cause error) *ResourceError {
	return NewResourceError(ErrCodeConflict, "resource conflict", resource, cause)
}

// InternalError is an unexpected error for internal failures.
type InternalError struct {
	CoreError
}

// NewInternalFailure returns an unexpected internal error.
func NewInternalFailure(message string, cause error) *InternalError {
	if message == "" {
		message = "internal error"
	}
	return &InternalError{CoreError: CoreError{Code: ErrCodeUnexpected, Message: message, Cause: cause}}
}

// UpstreamError wraps an upstream or dependency failure with call context.
type UpstreamError struct {
	// Cause is the upstream failure.
	Cause error
	// Target identifies the upstream service, endpoint, or dependency.
	Target string
	// Operation identifies the failed operation when available.
	Operation string
	// RequestID is the upstream request id when available.
	RequestID string
}

// Error implements error.
func (e *UpstreamError) Error() string {
	message := "upstream call failed"
	if e.Operation != "" && e.Target != "" {
		message = fmt.Sprintf("%s: %s %s", message, e.Operation, e.Target)
	} else if e.Target != "" {
		message = fmt.Sprintf("%s: %s", message, e.Target)
	} else if e.Operation != "" {
		message = fmt.Sprintf("%s: %s", message, e.Operation)
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", message, e.Cause)
	}
	return message
}

// Unwrap exposes the source error.
func (e *UpstreamError) Unwrap() error {
	return e.Cause
}

// NewUpstreamError returns an upstream error container.
func NewUpstreamError(target string, operation string, requestID string, cause error) *UpstreamError {
	return &UpstreamError{
		Cause:     cause,
		Target:    target,
		Operation: operation,
		RequestID: requestID,
	}
}

// NewNotFoundError returns a not-found error with custom message.
func NewNotFoundError(msg string) error {
	if msg == "" {
		return ErrNotFound
	}
	return &BaseError{
		Code:    ErrCodeNotFound,
		Message: msg,
	}
}

// NewConflictError returns a conflict error with custom message.
func NewConflictError(msg string) error {
	if msg == "" {
		return ErrConflict
	}
	return &BaseError{
		Code:    ErrCodeConflict,
		Message: msg,
	}
}

// NewInternalError returns an internal error with default message.
func NewInternalError(msg string) error {
	if msg == "" {
		msg = "internal server error"
	}
	return &InternalError{CoreError: CoreError{Code: ErrCodeUnexpected, Message: msg}}
}

// NewValidateError returns a validation error with detail items.
func NewValidateError(msg string, items []ValidateErrItem) error {
	return NewValidationError(msg, items, nil)
}

// ValidateErrItem describes one invalid parameter.
type ValidateErrItem struct {
	// ParamName is the parameter name.
	ParamName string `json:"paramName"`
	// Reason describes why it's invalid.
	Reason string `json:"reason"`
	// Detail carries extra detail.
	Detail interface{} `json:"detail"`
}

// IsErrCode checks whether err has the given business code.
func IsErrCode(code int, err error) bool {
	return err != nil && CodeOf(err) == code
}

// CodeOf returns the first stable code found in err's chain.
// Nil errors map to 0. Unknown non-nil errors map to ErrCodeUnexpected.
func CodeOf(err error) int {
	if err == nil {
		return 0
	}

	var coded CodedError
	if errors.As(err, &coded) && coded.ErrorCode() != 0 {
		return coded.ErrorCode()
	}

	return ErrCodeUnexpected
}

// IsExpected checks whether err carries an expected error code.
func IsExpected(err error) bool {
	code := CodeOf(err)
	return code >= ErrCodeExpected
}

// IsUnexpected checks whether err carries an unexpected error code.
// Unknown non-nil errors are treated as unexpected.
func IsUnexpected(err error) bool {
	return err != nil && !IsExpected(err)
}
