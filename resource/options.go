package resource

import (
	"net/http"
	"time"

	"github.com/dev-ofa/core-go/httpx"
)

const (
	defaultMaxBytes       int64 = 32 << 20
	defaultDataMaxBytes   int64 = 1 << 20
	defaultTimeoutQuota         = 5 * time.Second
	defaultConnectTimeout       = 3 * time.Second
	defaultRedirectLimit        = 5
	defaultRetryAttempts        = 3
	defaultRetryBaseDelay       = 100 * time.Millisecond
	defaultRetryMaxDelay        = time.Second
)

// Option configures a Manager.
type Option func(*Options)

// Options controls Manager defaults and built-in handlers.
type Options struct {
	HTTPClient     *http.Client
	MaxBytes       int64
	DataMaxBytes   int64
	TimeoutQuota   time.Duration
	ConnectTimeout time.Duration
	RedirectLimit  int
	RetryAttempts  int
	RetryBaseDelay time.Duration
	RetryMaxDelay  time.Duration
	EnableHTTP     bool
}

func defaultOptions() Options {
	return Options{
		HTTPClient:     httpx.NewClient(),
		MaxBytes:       defaultMaxBytes,
		DataMaxBytes:   defaultDataMaxBytes,
		TimeoutQuota:   defaultTimeoutQuota,
		ConnectTimeout: defaultConnectTimeout,
		RedirectLimit:  defaultRedirectLimit,
		RetryAttempts:  defaultRetryAttempts,
		RetryBaseDelay: defaultRetryBaseDelay,
		RetryMaxDelay:  defaultRetryMaxDelay,
		EnableHTTP:     true,
	}
}

// WithHTTPClient sets the client used by default http/https handlers.
func WithHTTPClient(client *http.Client) Option {
	return func(opts *Options) {
		if client != nil {
			opts.HTTPClient = client
		}
	}
}

// WithMaxBytes sets the default stream and download body size limit.
func WithMaxBytes(maxBytes int64) Option {
	return func(opts *Options) {
		if maxBytes > 0 {
			opts.MaxBytes = maxBytes
		}
	}
}

// WithDataMaxBytes sets the default data URL body size limit.
func WithDataMaxBytes(maxBytes int64) Option {
	return func(opts *Options) {
		if maxBytes > 0 {
			opts.DataMaxBytes = maxBytes
		}
	}
}

// WithTimeoutQuota sets the default timeout when ctx has no deadline.
func WithTimeoutQuota(timeout time.Duration) Option {
	return func(opts *Options) {
		if timeout > 0 {
			opts.TimeoutQuota = timeout
		}
	}
}

// WithRedirectLimit sets the default HTTP redirect limit.
func WithRedirectLimit(limit int) Option {
	return func(opts *Options) {
		if limit >= 0 {
			opts.RedirectLimit = limit
		}
	}
}

// WithRetry configures limited retries for built-in idempotent reads.
func WithRetry(attempts int, baseDelay time.Duration, maxDelay time.Duration) Option {
	return func(opts *Options) {
		if attempts > 0 {
			opts.RetryAttempts = attempts
		}
		if baseDelay > 0 {
			opts.RetryBaseDelay = baseDelay
		}
		if maxDelay > 0 {
			opts.RetryMaxDelay = maxDelay
		}
	}
}

// WithHTTPEnabled toggles the built-in http handler. https remains enabled.
func WithHTTPEnabled(enabled bool) Option {
	return func(opts *Options) {
		opts.EnableHTTP = enabled
	}
}
