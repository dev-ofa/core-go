package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"reflect"
	"time"

	"github.com/dev-ofa/core-go/trace/logging"
)

const (
	defaultTimeoutQuota  = 5 * time.Second
	defaultConnectTimout = 3 * time.Second
	defaultRetryAttempts = 3
	defaultRetryBase     = 100 * time.Millisecond
	defaultRetryMaxDelay = time.Second
)

var (
	// DefaultClient is used when an Agent does not specify a client.
	DefaultClient = NewClient()
)

// Agent describes one HTTP call and its option pipeline.
type Agent struct {
	url    string
	method string
	ctx    context.Context

	reqPreHandlers      []ReqPreHandler
	respHandler         RespHandler
	respWrapper         Wrapper
	client              *http.Client
	expectedStatusCodes []int
	retryOpt            *RetryOpt
	timeoutQuota        time.Duration
	service             ServiceOptions
	cancel              context.CancelFunc

	existedOps []AgentOp
}

// RetryOpt configures limited retries. Attempts means total attempts.
type RetryOpt struct {
	// MaxDelay is the cap of exponential backoff delay.
	MaxDelay time.Duration
	// BaseDelay is the first backoff delay before jitter.
	BaseDelay time.Duration
	// RetryAppError indicates whether wrapper validation errors can be retried.
	RetryAppError bool
	// Attempts is the total attempt count. The spec default is 3 when retry is enabled.
	Attempts int
	// Idempotent allows retry for non-idempotent HTTP methods only when explicitly proven safe.
	Idempotent bool
}

// AgentOp configures an Agent before execution.
type AgentOp interface {
	InitialAgent(*Agent) error
}

// AgentOpFunc adapts a function into an AgentOp.
type AgentOpFunc func(agent *Agent) error

// InitialAgent implements AgentOp.
func (f AgentOpFunc) InitialAgent(agent *Agent) error {
	return f(agent)
}

// NewClient returns a net/http client with finite connect timeout and safe pooling defaults.
func NewClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   defaultConnectTimout,
		KeepAlive: 30 * time.Second,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer.DialContext
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 100
	transport.IdleConnTimeout = 90 * time.Second
	transport.TLSHandshakeTimeout = defaultConnectTimout
	return &http.Client{Transport: transport}
}

// Do executes the HTTP call.
func (a *Agent) Do() error {
	if err := a.init(); err != nil {
		return err
	}
	if a.cancel != nil {
		defer a.cancel()
	}
	_, err := a.executeHTTP(executeHandle)
	return err
}

// DoStream executes the HTTP call and returns a successful response stream.
// The caller must close resp.Body. Response handlers and wrappers are not used.
func (a *Agent) DoStream() (*http.Response, error) {
	if err := a.init(); err != nil {
		if a.cancel != nil {
			a.cancel()
		}
		return nil, err
	}
	resp, err := a.executeHTTP(executeStream)
	if err != nil {
		if a.cancel != nil {
			a.cancel()
		}
		return nil, err
	}
	if a.cancel != nil {
		resp.Body = cancelOnCloseReadCloser{ReadCloser: resp.Body, cancel: a.cancel}
		a.cancel = nil
	}
	return resp, nil
}

func (a *Agent) init() error {
	for _, op := range a.existedOps {
		if err := op.InitialAgent(a); err != nil {
			return err
		}
	}
	if len(a.expectedStatusCodes) == 0 {
		a.expectedStatusCodes = append(a.expectedStatusCodes, http.StatusOK)
	}
	if a.ctx == nil {
		a.ctx = context.Background()
	}
	var err error
	a.ctx, _, err = ensureTraceContext(a.ctx)
	if err != nil {
		return err
	}
	if _, ok := a.ctx.Deadline(); !ok {
		timeout := a.timeoutQuota
		if timeout <= 0 {
			timeout = defaultTimeoutQuota
		}
		ctx, cancel := context.WithTimeout(a.ctx, timeout)
		a.ctx = ctx
		a.cancel = cancel
	}
	if a.client == nil {
		a.client = DefaultClient
	}
	if a.client.Transport == nil {
		a.client.Transport = DefaultClient.Transport
	}
	return nil
}

func (a *Agent) prepareRequest() (*http.Request, string, error) {
	if deadline, ok := a.ctx.Deadline(); ok && time.Until(deadline) <= 0 {
		return nil, "", ErrTimeoutBudgetExhausted
	}
	req, err := http.NewRequestWithContext(a.ctx, a.method, a.url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("new request failed: %w", err)
	}
	for _, h := range a.reqPreHandlers {
		newReq, handleErr := h.PreHandleRequest(req)
		if handleErr != nil {
			return nil, "", handleErr
		}
		if newReq != nil {
			req = newReq
		}
	}
	ctx, requestID, err := injectTraceHeaders(a.ctx, req)
	if err != nil {
		return nil, requestID, err
	}
	a.ctx = ctx
	if a.service.EnableDiscovery {
		traceID := req.Header.Get(HeaderTraceID)
		resolved, originalHost, err := resolveURL(a.ctx, req.URL, a.service, traceID, requestID)
		if err != nil {
			return nil, requestID, err
		}
		req.URL = resolved
		if originalHost != "" {
			req.Host = originalHost
		}
	}
	return req, requestID, nil
}

type executeMode int

const (
	executeHandle executeMode = iota
	executeStream
)

func (a *Agent) executeHTTP(mode executeMode) (*http.Response, error) {
	if a.retryOpt == nil {
		_, resp, err := a.doHTTP(mode)
		return resp, err
	}
	return a.retryDoHTTP(mode)
}

func (a *Agent) retryDoHTTP(mode executeMode) (*http.Response, error) {
	attempts := a.retryOpt.Attempts
	if attempts <= 0 {
		attempts = defaultRetryAttempts
	}
	if !a.canRetryMethod() {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		result, resp, err := a.doHTTP(mode)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if attempt == attempts || !a.shouldRetry(err, result) {
			return nil, err
		}
		delay := a.retryDelay(attempt)
		if deadline, ok := a.ctx.Deadline(); ok && time.Until(deadline) <= delay {
			return nil, lastErr
		}
		timer := time.NewTimer(delay)
		select {
		case <-a.ctx.Done():
			timer.Stop()
			return nil, a.ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

type attemptResult struct {
	statusCode int
	requestID  string
}

func (a *Agent) doHTTP(mode executeMode) (*attemptResult, *http.Response, error) {
	req, requestID, err := a.prepareRequest()
	if err != nil {
		return &attemptResult{requestID: requestID}, nil, err
	}
	start := time.Now()
	logging.CtxInfof(a.ctx, "httpx request start method=%s path=%s", req.Method, req.URL.Path)
	resp, err := a.client.Do(req)
	if err != nil {
		wrapped := &CallError{Method: req.Method, URL: req.URL.String(), RequestID: requestID, Err: fmt.Errorf("request do failed: %w", err)}
		a.logEnd(req, 0, start, wrapped)
		return &attemptResult{requestID: requestID}, nil, wrapped
	}

	result := &attemptResult{statusCode: resp.StatusCode, requestID: requestID}
	if !a.isInExpectedStatusCodes(resp.StatusCode) {
		body, readErr := io.ReadAll(resp.Body)
		statusErr := newHTTPStatusError(a.expectedStatusCodes, resp, body, readErr)
		wrapped := &CallError{Method: req.Method, URL: req.URL.String(), RequestID: requestID, StatusCode: resp.StatusCode, Err: statusErr}
		a.logEnd(req, resp.StatusCode, start, wrapped)
		_ = resp.Body.Close()
		return result, nil, wrapped
	}
	if mode == executeStream {
		a.logEnd(req, resp.StatusCode, start, nil)
		return result, resp, nil
	}
	defer resp.Body.Close()
	if a.respHandler != nil {
		if err := a.respHandler.HandleResponse(resp, a.respWrapper); err != nil {
			appErr := &applicationError{err: err}
			wrapped := &CallError{Method: req.Method, URL: req.URL.String(), RequestID: requestID, StatusCode: resp.StatusCode, Err: appErr}
			a.logEnd(req, resp.StatusCode, start, wrapped)
			return result, nil, wrapped
		}
	}
	a.logEnd(req, resp.StatusCode, start, nil)
	return result, nil, nil
}

func (a *Agent) logEnd(req *http.Request, statusCode int, start time.Time, err error) {
	duration := time.Since(start).Milliseconds()
	if err == nil {
		logging.CtxInfof(a.ctx, "httpx request end method=%s path=%s status_code=%d duration_ms=%d", req.Method, req.URL.Path, statusCode, duration)
		return
	}
	var appErr *applicationError
	if errors.As(err, &appErr) {
		logging.CtxWarnf(a.ctx, "httpx request end method=%s path=%s status_code=%d duration_ms=%d error=%v", req.Method, req.URL.Path, statusCode, duration, err)
		return
	}
	logging.CtxErrorf(a.ctx, "httpx request end method=%s path=%s status_code=%d duration_ms=%d error=%v", req.Method, req.URL.Path, statusCode, duration, err)
}

func (a *Agent) canRetryMethod() bool {
	if a.retryOpt != nil && a.retryOpt.Idempotent {
		return true
	}
	switch a.method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func (a *Agent) shouldRetry(err error, result *attemptResult) bool {
	var appErr *applicationError
	if errors.As(err, &appErr) {
		return a.retryOpt != nil && a.retryOpt.RetryAppError
	}
	if result != nil && result.statusCode >= http.StatusInternalServerError {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}

func (a *Agent) retryDelay(attempt int) time.Duration {
	base := a.retryOpt.BaseDelay
	if base <= 0 {
		base = defaultRetryBase
	}
	maxDelay := a.retryOpt.MaxDelay
	if maxDelay <= 0 {
		maxDelay = defaultRetryMaxDelay
	}
	delay := base << (attempt - 1)
	if delay > maxDelay {
		delay = maxDelay
	}
	if delay <= 0 {
		return 0
	}
	jitter := time.Duration(rand.Int63n(int64(delay))) // #nosec G404: retry jitter does not require crypto randomness.
	return delay/2 + jitter/2
}

func (a *Agent) isInExpectedStatusCodes(code int) bool {
	for _, ac := range a.expectedStatusCodes {
		if ac == code {
			return true
		}
	}
	return false
}

// Ops appends options to the Agent.
func (a *Agent) Ops(ops ...AgentOp) *Agent {
	a.existedOps = append(a.existedOps, ops...)
	return a
}

// Retry configures limited retry.
func Retry(opt *RetryOpt) AgentOpFunc {
	return func(agent *Agent) error {
		if opt == nil {
			opt = &RetryOpt{}
		}
		agent.retryOpt = opt
		return nil
	}
}

// ExpectedStatusCodes configures accepted HTTP status codes.
func ExpectedStatusCodes(codes []int) AgentOpFunc {
	return func(agent *Agent) error {
		agent.expectedStatusCodes = append([]int(nil), codes...)
		return nil
	}
}

// Context configures the request context.
func Context(ctx context.Context) AgentOpFunc {
	return func(agent *Agent) error {
		agent.ctx = ctx
		return nil
	}
}

// Client configures the HTTP client.
func Client(client *http.Client) AgentOpFunc {
	return func(agent *Agent) error {
		agent.client = client
		return nil
	}
}

// TimeoutQuota configures a default timeout quota when context has no deadline.
func TimeoutQuota(timeout time.Duration) AgentOpFunc {
	return func(agent *Agent) error {
		agent.timeoutQuota = timeout
		return nil
	}
}

// Service configures service discovery.
func Service(opt ServiceOptions) AgentOpFunc {
	return func(agent *Agent) error {
		agent.service = opt
		return nil
	}
}

// SetHeader adds request headers.
func SetHeader(header http.Header) AgentOpFunc {
	return func(agent *Agent) error {
		agent.reqPreHandlers = append(agent.reqPreHandlers, ReqPreHandlerFunc(func(req *http.Request) (*http.Request, error) {
			for k, v := range header {
				req.Header[k] = append([]string(nil), v...)
			}
			return req, nil
		}))
		return nil
	}
}

// RespWrapper configures a standard response wrapper.
func RespWrapper(wrapper Wrapper) AgentOpFunc {
	return func(agent *Agent) error {
		if wrapper == nil || reflect.TypeOf(wrapper).Kind() != reflect.Ptr {
			return fmt.Errorf("response wrapper should be ptr")
		}
		agent.respWrapper = wrapper
		return nil
	}
}

// CustomRespHandler configures a custom response handler.
func CustomRespHandler(handler RespHandler) AgentOpFunc {
	return func(agent *Agent) error {
		agent.respHandler = handler
		return nil
	}
}

// Get starts a GET request.
func Get(url string, ops ...AgentOp) *Agent {
	return newAgent(url, http.MethodGet, ops...)
}

// Post starts a POST request.
func Post(url string, ops ...AgentOp) *Agent {
	return newAgent(url, http.MethodPost, ops...)
}

// Put starts a PUT request.
func Put(url string, ops ...AgentOp) *Agent {
	return newAgent(url, http.MethodPut, ops...)
}

// Patch starts a PATCH request.
func Patch(url string, ops ...AgentOp) *Agent {
	return newAgent(url, http.MethodPatch, ops...)
}

// Delete starts a DELETE request.
func Delete(url string, ops ...AgentOp) *Agent {
	return newAgent(url, http.MethodDelete, ops...)
}

func newAgent(url string, method string, ops ...AgentOp) *Agent {
	return &Agent{
		url:        url,
		method:     method,
		existedOps: ops,
		ctx:        context.Background(),
	}
}

type applicationError struct {
	err error
}

func (e *applicationError) Error() string {
	return e.err.Error()
}

func (e *applicationError) Unwrap() error {
	return e.err
}

type cancelOnCloseReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelOnCloseReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}
