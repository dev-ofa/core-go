package httpx

import (
	"fmt"

	"github.com/dev-ofa/core-go/model/datax"
)

// CommonWrapper is a spec-compatible response wrapper with code/message/request_id/data.
type CommonWrapper struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Data      any    `json:"data"`

	allowCodes []int
}

// NewCommonWrapper returns a response wrapper that treats code 0 and allowCodes as success.
func NewCommonWrapper(allowCodes ...int) *CommonWrapper {
	return &CommonWrapper{allowCodes: allowCodes}
}

// SetData sets the data target for JSON decoding.
func (c *CommonWrapper) SetData(data any) {
	c.Data = data
}

// Validate validates the application response code.
func (c *CommonWrapper) Validate() error {
	if c.Code == 0 {
		return nil
	}
	for _, code := range c.allowCodes {
		if c.Code == code {
			return nil
		}
	}
	wrapperErr := NewWrapperError(c.Code, c.Message, c.RequestID, c.Data)
	if c.Data != nil {
		return datax.WithErrorData(wrapperErr, c.Data)
	}
	return wrapperErr
}

// WrapperError describes an application wrapper validation failure.
type WrapperError struct {
	datax.BaseError
	Code      int
	Message   string
	RequestID string
	Data      any
}

// NewWrapperError returns a wrapper validation error with a stable application code.
func NewWrapperError(code int, message string, requestID string, data any) *WrapperError {
	if code == 0 {
		code = ErrCodeHTTPWrapperDefault
	}
	if message == "" {
		message = "httpx wrapper validate failed"
	}
	return &WrapperError{
		BaseError: *datax.NewError(code, message, nil),
		Code:      code,
		Message:   message,
		RequestID: requestID,
		Data:      data,
	}
}

// Error implements the error interface.
func (e *WrapperError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("httpx wrapper validate failed code=%d request_id=%s: %s", e.Code, e.RequestID, e.Message)
	}
	return fmt.Sprintf("httpx wrapper validate failed code=%d: %s", e.Code, e.Message)
}
