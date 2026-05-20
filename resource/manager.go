package resource

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Manager routes resource operations to handlers by source scheme.
type Manager struct {
	opts     Options
	mu       sync.RWMutex
	handlers map[string]ResourceHandler
}

// NewManager creates a Manager with built-in handlers for https, data, and optionally http.
func NewManager(opts ...Option) *Manager {
	options := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	m := &Manager{
		opts:     options,
		handlers: map[string]ResourceHandler{},
	}
	httpHandler := newHTTPHandler(options)
	m.handlers["https"] = httpHandler
	if options.EnableHTTP {
		m.handlers["http"] = httpHandler
	}
	m.handlers["data"] = newDataHandler(options)
	return m
}

// Register registers or replaces a handler for a scheme.
func (m *Manager) Register(scheme string, handler ResourceHandler) error {
	if handler == nil {
		return fmt.Errorf("handler is nil")
	}
	if scheme == "" || strings.ToLower(scheme) != scheme || !schemeRegexp.MatchString(scheme) {
		return fmt.Errorf("invalid scheme %q", scheme)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[scheme] = handler
	return nil
}

// Open parses raw, routes by scheme, and returns a stream.
func (m *Manager) Open(ctx context.Context, raw string) (*Stream, error) {
	id, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	handler, err := m.handler(id.Scheme)
	if err != nil {
		return nil, &OpenError{Identifier: id, Err: err}
	}
	stream, err := handler.Open(ctx, id)
	if err != nil {
		return nil, &OpenError{Identifier: id, Err: err}
	}
	if stream == nil || stream.Body == nil {
		return nil, &OpenError{Identifier: id, Err: fmt.Errorf("handler returned empty stream")}
	}
	return stream, nil
}

// Download opens raw and atomically writes its body to dstPath.
func (m *Manager) Download(ctx context.Context, raw string, dstPath string) error {
	if dstPath == "" {
		return &DownloadError{DstPath: dstPath, Err: fmt.Errorf("dstPath is empty")}
	}
	stream, err := m.Open(ctx, raw)
	if err != nil {
		return &DownloadError{DstPath: dstPath, Err: err}
	}
	defer stream.Body.Close()

	dir := filepath.Dir(dstPath)
	info, statErr := os.Stat(dir)
	if statErr != nil {
		return &DownloadError{DstPath: dstPath, Err: statErr}
	} else if !info.IsDir() {
		return &DownloadError{DstPath: dstPath, Err: fmt.Errorf("destination parent is not a directory")}
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(dstPath)+".tmp-*")
	if err != nil {
		return &DownloadError{DstPath: dstPath, Err: err}
	}
	tmpPath := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := io.Copy(tmp, stream.Body); err != nil {
		_ = tmp.Close()
		return &DownloadError{DstPath: dstPath, Err: err}
	}
	if stream.Size >= 0 && m.opts.MaxBytes > 0 && stream.Size > m.opts.MaxBytes {
		_ = tmp.Close()
		return &DownloadError{DstPath: dstPath, Err: ErrSizeLimitExceeded}
	}
	if err := tmp.Close(); err != nil {
		return &DownloadError{DstPath: dstPath, Err: err}
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		return &DownloadError{DstPath: dstPath, Err: err}
	}
	ok = true
	return nil
}

// Upload routes an upload request to the handler registered for scheme.
func (m *Manager) Upload(ctx context.Context, scheme string, in UploadInput) (Identifier, error) {
	if scheme == "" || strings.ToLower(scheme) != scheme || !schemeRegexp.MatchString(scheme) {
		return Identifier{}, &UploadError{Scheme: scheme, Err: fmt.Errorf("invalid scheme %q", scheme)}
	}
	handler, err := m.handler(scheme)
	if err != nil {
		return Identifier{}, &UploadError{Scheme: scheme, Err: err}
	}
	id, err := handler.Upload(ctx, in)
	if err != nil {
		return Identifier{}, &UploadError{Scheme: scheme, Err: err}
	}
	return id, nil
}

func (m *Manager) handler(scheme string) (ResourceHandler, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	handler, ok := m.handlers[scheme]
	if !ok {
		return nil, ErrUnsupportedScheme
	}
	return handler, nil
}

func ensureDeadline(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok || timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func remainingTimeout(ctx context.Context) (time.Duration, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return 0, nil
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 0, ErrTimeoutBudgetExhausted
	}
	return remaining, nil
}
