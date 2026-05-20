package resource

import (
	"context"
	"io"
	"net/http"
)

// Identifier is the parsed form of an ofa-res resource identifier.
type Identifier struct {
	Raw       string
	Params    map[string]string
	SourceURI string
	Scheme    string

	AuthID    string
	MediaType string
}

// Stream is an opened resource stream. Callers must close Body.
type Stream struct {
	Body      io.ReadCloser
	MediaType string
	Filename  string
	Size      int64
	SourceURI string
	Headers   http.Header
}

// ResourceHandler opens and uploads resources for a specific source scheme.
type ResourceHandler interface {
	Open(ctx context.Context, id Identifier) (*Stream, error)
	Upload(ctx context.Context, in UploadInput) (Identifier, error)
}

// HandlerFuncs adapts functions into a ResourceHandler.
type HandlerFuncs struct {
	OpenFunc   func(ctx context.Context, id Identifier) (*Stream, error)
	UploadFunc func(ctx context.Context, in UploadInput) (Identifier, error)
}

// Open implements ResourceHandler.
func (h HandlerFuncs) Open(ctx context.Context, id Identifier) (*Stream, error) {
	if h.OpenFunc == nil {
		return nil, ErrOpenUnsupported
	}
	return h.OpenFunc(ctx, id)
}

// Upload implements ResourceHandler.
func (h HandlerFuncs) Upload(ctx context.Context, in UploadInput) (Identifier, error) {
	if h.UploadFunc == nil {
		return Identifier{}, ErrUploadUnsupported
	}
	return h.UploadFunc(ctx, in)
}

// UploadInput describes a single upload request. Body is not closed by Upload.
type UploadInput struct {
	Body       io.Reader
	MediaType  string
	Filename   string
	AuthID     string
	TargetHint string
}
