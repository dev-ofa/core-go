package httpx

import (
	"fmt"
	"net/http"

	"github.com/dev-ofa/core-go/model/datax"
)

const (
	// ErrCodeHTTPTimeoutBudgetExhausted means the current authoritative deadline has no remaining budget.
	ErrCodeHTTPTimeoutBudgetExhausted = 10110
	// ErrCodeHTTPNoHealthyInstance means service discovery returned no callable healthy instance.
	ErrCodeHTTPNoHealthyInstance = 10111
	// ErrCodeHTTPServiceDiscoveryDisabled means a discovery-only option was used without a resolver.
	ErrCodeHTTPServiceDiscoveryDisabled = 20110
	// ErrCodeHTTPWrapperDefault is used when a wrapper error does not carry an application code.
	ErrCodeHTTPWrapperDefault = datax.ErrCodeExpected
)

var (
	// ErrTimeoutBudgetExhausted means the current authoritative deadline has no remaining budget.
	ErrTimeoutBudgetExhausted = datax.NewError(ErrCodeHTTPTimeoutBudgetExhausted, "httpx: timeout budget exhausted", nil)
	// ErrNoHealthyInstance means service discovery returned no callable healthy instance.
	ErrNoHealthyInstance = datax.NewError(ErrCodeHTTPNoHealthyInstance, "httpx: no healthy service instance", nil)
	// ErrServiceDiscoveryDisabled means a discovery-only option was used without a resolver.
	ErrServiceDiscoveryDisabled = datax.NewError(ErrCodeHTTPServiceDiscoveryDisabled, "httpx: service discovery is disabled", nil)
)

// HTTPStatusError reports a non-expected HTTP status code and the response body.
type HTTPStatusError struct {
	// StatusCode is the actual HTTP response status code.
	StatusCode int
	// ExpectedStatusCodes is the allow list configured by the caller.
	ExpectedStatusCodes []int
	// Body contains the response body read from the failed response.
	Body []byte
	// ReadBodyErr stores the error raised while reading the failed response body.
	ReadBodyErr error
	// Cause stores the normalized HTTP error used by the new error chain.
	Cause error
}

// Error implements the error interface.
func (e *HTTPStatusError) Error() string {
	if e.ReadBodyErr != nil {
		return fmt.Sprintf("http status %d is not expected(%v), read body failed: %v", e.StatusCode, e.ExpectedStatusCodes, e.ReadBodyErr)
	}
	if e.Cause != nil {
		return fmt.Sprintf("http status %d is not expected(%v): %v", e.StatusCode, e.ExpectedStatusCodes, e.Cause)
	}
	return fmt.Sprintf("http status %d is not expected(%v), body: %s", e.StatusCode, e.ExpectedStatusCodes, string(e.Body))
}

// Unwrap returns the normalized HTTP error.
func (e *HTTPStatusError) Unwrap() error {
	return e.Cause
}

// CallError wraps request metadata around the root cause.
type CallError struct {
	// Method is the HTTP request method.
	Method string
	// URL is the final request URL.
	URL string
	// RequestID is the generated single-hop request id.
	RequestID string
	// StatusCode is the response status code if a response was received.
	StatusCode int
	// Err is the root cause.
	Err error
}

// Error implements the error interface.
func (e *CallError) Error() string {
	if e.StatusCode != 0 {
		return fmt.Sprintf("httpx call %s %s request_id=%s status_code=%d failed: %v", e.Method, e.URL, e.RequestID, e.StatusCode, e.Err)
	}
	return fmt.Sprintf("httpx call %s %s request_id=%s failed: %v", e.Method, e.URL, e.RequestID, e.Err)
}

// Unwrap returns the root cause.
func (e *CallError) Unwrap() error {
	return e.Err
}

func newHTTPStatusError(expected []int, resp *http.Response, body []byte, readErr error) *HTTPStatusError {
	return &HTTPStatusError{
		StatusCode:          resp.StatusCode,
		ExpectedStatusCodes: expected,
		Body:                body,
		ReadBodyErr:         readErr,
	}
}

func newCallError(method string, target string, requestID string, statusCode int, err error) *CallError {
	return &CallError{
		Method:     method,
		URL:        target,
		RequestID:  requestID,
		StatusCode: statusCode,
		Err:        err,
	}
}
