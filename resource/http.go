package resource

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/dev-ofa/core-go/httpx"
)

type httpHandler struct {
	client        *http.Client
	maxBytes      int64
	timeoutQuota  time.Duration
	redirectLimit int
	retryAttempts int
	retryBase     time.Duration
	retryMaxDelay time.Duration
}

func newHTTPHandler(opts Options) ResourceHandler {
	client := &http.Client{}
	if opts.HTTPClient != nil {
		*client = *opts.HTTPClient
	}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if opts.RedirectLimit >= 0 && len(via) >= opts.RedirectLimit {
			return fmt.Errorf("stopped after %d redirects", opts.RedirectLimit)
		}
		return nil
	}
	return httpHandler{
		client:        client,
		maxBytes:      opts.MaxBytes,
		timeoutQuota:  opts.TimeoutQuota,
		redirectLimit: opts.RedirectLimit,
		retryAttempts: opts.RetryAttempts,
		retryBase:     opts.RetryBaseDelay,
		retryMaxDelay: opts.RetryMaxDelay,
	}
}

func (h httpHandler) Open(ctx context.Context, id Identifier) (*Stream, error) {
	resp, err := httpx.Get(id.SourceURI,
		httpx.Context(ctx),
		httpx.Client(h.client),
		httpx.TimeoutQuota(h.timeoutQuota),
		httpx.ExpectedStatusCodes(successStatusCodes()),
		httpx.Retry(&httpx.RetryOpt{
			Attempts:  h.retryAttempts,
			BaseDelay: h.retryBase,
			MaxDelay:  h.retryMaxDelay,
		}),
	).DoStream()
	if err != nil {
		return nil, err
	}
	if h.maxBytes > 0 && resp.ContentLength > h.maxBytes {
		_ = resp.Body.Close()
		return nil, ErrSizeLimitExceeded
	}
	body := io.ReadCloser(resp.Body)
	if h.maxBytes > 0 {
		body = &limitReadCloser{body: body, max: h.maxBytes}
	}
	mediaType := resp.Header.Get("Content-Type")
	if mediaType == "" {
		mediaType = id.MediaType
	}
	filename := id.Params["filename"]
	if filename == "" {
		filename = filenameFromDisposition(resp.Header.Get("Content-Disposition"))
	}
	return &Stream{
		Body:      body,
		MediaType: mediaType,
		Filename:  filename,
		Size:      resp.ContentLength,
		SourceURI: id.SourceURI,
		Headers:   resp.Header.Clone(),
	}, nil
}

func (h httpHandler) Upload(context.Context, UploadInput) (Identifier, error) {
	return Identifier{}, ErrUploadUnsupported
}

func filenameFromDisposition(disposition string) string {
	if disposition == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		return ""
	}
	filename := params["filename"]
	if strings.ContainsAny(filename, `/\`+"\x00") || strings.Contains(filename, "..") {
		return ""
	}
	return filename
}

type limitReadCloser struct {
	body io.ReadCloser
	max  int64
	read int64
	over bool
}

func (l *limitReadCloser) Read(p []byte) (int, error) {
	if l.over {
		return 0, ErrSizeLimitExceeded
	}
	remaining := l.max - l.read
	readLimit := remaining + 1
	if readLimit <= 0 {
		readLimit = 1
	}
	if int64(len(p)) > readLimit {
		p = p[:int(readLimit)]
	}
	n, err := l.body.Read(p)
	l.read += int64(n)
	if l.read > l.max {
		l.over = true
		allowed := n - int(l.read-l.max)
		if allowed < 0 {
			allowed = 0
		}
		return allowed, ErrSizeLimitExceeded
	}
	return n, err
}

func (l *limitReadCloser) Close() error {
	return l.body.Close()
}

func successStatusCodes() []int {
	return []int{
		http.StatusOK,
		http.StatusCreated,
		http.StatusAccepted,
		http.StatusNonAuthoritativeInfo,
		http.StatusNoContent,
		http.StatusResetContent,
		http.StatusPartialContent,
		http.StatusMultiStatus,
		http.StatusAlreadyReported,
		http.StatusIMUsed,
	}
}
