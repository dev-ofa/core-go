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
	ErrNotFound = &BaseError{Code: ErrCodeNotFound, Message: "data not found"}
	ErrConflict = &BaseError{Code: ErrCodeConflict, Message: "data is existed or has be updated"}
)

type BaseError struct {
	Code      int
	Message   string
	Data      interface{}
	SourceSrv string
}

func (e *BaseError) Error() string {
	if e.SourceSrv != "" {
		return fmt.Sprintf("call: %s failed, code: %d, msg: %s", e.SourceSrv, e.Code, e.Message)
	}
	return e.Message
}

func (e *BaseError) Is(err error) bool {
	return IsErrCode(e.Code, err)
}

type ErrWrapper struct {
	Code int
	Msg  string
	Data interface{}
}

func (e *ErrWrapper) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("wrapper validate failed, code: [%d], msg : [%s], data: [%+v]", e.Code, e.Msg, e.Data)
	}

	return fmt.Sprintf("wrapper validate failed, code: [%d], msg : [%s]", e.Code, e.Msg)
}

type ErrHttp struct {
	StatusCode int
	Body       []byte
}

func (e *ErrHttp) Error() string {
	if len(e.Body) == 0 {
		return fmt.Sprintf("http validate failed, status: [%d]", e.StatusCode)
	}
	return fmt.Sprintf("http validate failed, status: [%d], body : [%s]", e.StatusCode, string(e.Body))
}

type ErrCall struct {
	Url       string
	RequestID string
	Method    string

	SrcErr error
}

func (e *ErrCall) Error() string {
	return fmt.Sprintf("%s [%s] failed, reqid:[%s], source err: [%s]", e.Method, e.Url, e.RequestID, e.SrcErr)
}

func (e *ErrCall) Unwrap() error {
	return e.SrcErr
}

func NewNotFoundError(msg string) error {
	if msg == "" {
		return ErrNotFound
	}
	return &BaseError{
		Code:    ErrCodeNotFound,
		Message: msg,
	}
}

func NewConflictError(msg string) error {
	if msg == "" {
		return ErrConflict
	}
	return &BaseError{
		Code:    ErrCodeConflict,
		Message: msg,
	}
}

func NewInternalError(msg string) error {
	if msg == "" {
		msg = "internal server error"
	}
	return &BaseError{
		Code:    ErrCodeInternal,
		Message: msg,
	}
}

func NewFriendlyError(msg string) error {
	return &BaseError{
		Code:    ErrCodeFriendly,
		Message: msg,
	}
}

type ValidateError struct {
	BaseError
}

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

type ValidateErrItem struct {
	ParamName string      `json:"paramName"`
	Reason    string      `json:"reason"`
	Detail    interface{} `json:"detail"`
}

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
