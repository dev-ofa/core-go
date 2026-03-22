package datax

import (
	"errors"
	"fmt"
)

// - 对于预期内or不需要告警的错误，Body中业务错误码请使用 >= 20000 以上的数值，如 20100, 20300
// - 对于非预期or需要告警的业务错误，Body中业务错误码请使用 < 20000 的数值，推荐 >= 10000，避免与RFC和字节基础设施中的某些StatusCode定义冲突
const (
	// ErrCodeInternal is default error code
	ErrCodeInternal = 10000
	// ErrCodeNotFound mean that the record you querying does not found
	ErrCodeNotFound = 10001
	// ErrCodeConflict mean that the record you want to insert/update is conflicted with others
	ErrCodeConflict = 10002

	// ErrCodeValidate mean that the format of request's parameter is not validated(e.g. not match business logic)
	ErrCodeValidate = 20000
	// ErrCodeFriendly is indicated that the message should display to client
	ErrCodeFriendly = 20001
)

var (
	// ErrNotFound is a shared not-found error instance.
	ErrNotFound = &BaseError{Code: ErrCodeNotFound, Message: "data not found"}
	// ErrConflict is a shared conflict error instance.
	ErrConflict = &BaseError{Code: ErrCodeConflict, Message: "data is existed or has be updated"}
)

// BaseError is the common error type with code and message.
type BaseError struct {
	// Code is the business error code.
	Code      int
	// Message is the human-readable message.
	Message   string
	// Data carries extra error details.
	Data      interface{}
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

// Is checks whether err matches this error code.
func (e *BaseError) Is(err error) bool {
	return IsErrCode(e.Code, err)
}

// ErrWrapper wraps validation errors with code and message.
type ErrWrapper struct {
	// Code is the business error code.
	Code int
	// Msg is the error message.
	Msg  string
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

// ErrHttp reports HTTP validation errors from external calls.
type ErrHttp struct {
	// StatusCode is the HTTP status code.
	StatusCode int
	// Body is the raw response body.
	Body       []byte
}

// Error implements error.
func (e *ErrHttp) Error() string {
	if len(e.Body) == 0 {
		return fmt.Sprintf("http validate failed, status: [%d]", e.StatusCode)
	}
	return fmt.Sprintf("http validate failed, status: [%d], body : [%s]", e.StatusCode, string(e.Body))
}

// ErrCall represents an upstream call failure with context.
type ErrCall struct {
	// Url is the request URL.
	Url       string
	// RequestID is the upstream request id.
	RequestID string
	// Method is the HTTP method.
	Method    string

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
	return &BaseError{
		Code:    ErrCodeInternal,
		Message: msg,
	}
}

// NewFriendlyError returns a friendly error message for clients.
func NewFriendlyError(msg string) error {
	return &BaseError{
		Code:    ErrCodeFriendly,
		Message: msg,
	}
}

// ValidateError is a typed validation error.
type ValidateError struct {
	BaseError
}

// NewValidateError returns a validation error with detail items.
func NewValidateError(msg string, items []ValidateErrItem) error {
	if msg == "" {
		msg = "parameter validate failed"
	}
	return &BaseError{
		Code:    ErrCodeValidate,
		Message: msg,
		Data:    items,
	}
}

// ValidateErrItem describes one invalid parameter.
type ValidateErrItem struct {
	// ParamName is the parameter name.
	ParamName string      `json:"paramName"`
	// Reason describes why it's invalid.
	Reason    string      `json:"reason"`
	// Detail carries extra detail.
	Detail    interface{} `json:"detail"`
}

// IsErrCode checks whether err has the given business code.
func IsErrCode(code int, err error) bool {
	if err == nil {
		return false
	}

	// type assert for high performance
	switch t := err.(type) {
	case *ErrWrapper:
		return t.Code == code
	case *BaseError:
		return t.Code == code
	}

	wErr := &ErrWrapper{}
	if errors.As(err, &wErr) {
		return wErr.Code == code
	}

	bErr := &BaseError{}
	if errors.As(err, &bErr) {
		return bErr.Code == code
	}

	return false
}
