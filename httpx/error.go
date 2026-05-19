package httpx

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	// ErrTimeoutBudgetExhausted means the current authoritative deadline has no remaining budget.
	ErrTimeoutBudgetExhausted = errors.New("httpx: timeout budget exhausted")
	// ErrNoHealthyInstance means service discovery returned no callable healthy instance.
	ErrNoHealthyInstance = errors.New("httpx: no healthy service instance")
	// ErrServiceDiscoveryDisabled means a discovery-only option was used without a resolver.
	ErrServiceDiscoveryDisabled = errors.New("httpx: service discovery is disabled")
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
}

// Error implements the error interface.
func (e *HTTPStatusError) Error() string {
	if e.ReadBodyErr != nil {
		return fmt.Sprintf("http status %d is not expected(%v), read body failed: %v", e.StatusCode, e.ExpectedStatusCodes, e.ReadBodyErr)
	}
	return fmt.Sprintf("http status %d is not expected(%v), body: %s", e.StatusCode, e.ExpectedStatusCodes, string(e.Body))
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
