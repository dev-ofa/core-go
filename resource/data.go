package resource

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/url"
	"strings"
)

type dataHandler struct {
	maxBytes int64
}

func newDataHandler(opts Options) ResourceHandler {
	return dataHandler{maxBytes: opts.DataMaxBytes}
}

func (h dataHandler) Open(_ context.Context, id Identifier) (*Stream, error) {
	mediaType, body, err := parseDataURL(id.SourceURI)
	if err != nil {
		return nil, err
	}
	if id.MediaType != "" && mediaType != "" && !sameMediaType(id.MediaType, mediaType) {
		return nil, fmt.Errorf("data media type %q does not match identifier media_type %q", mediaType, id.MediaType)
	}
	if h.maxBytes > 0 && int64(len(body)) > h.maxBytes {
		return nil, ErrSizeLimitExceeded
	}
	if id.MediaType != "" {
		mediaType = id.MediaType
	}
	return &Stream{
		Body:      io.NopCloser(bytes.NewReader(body)),
		MediaType: mediaType,
		Size:      int64(len(body)),
		SourceURI: id.SourceURI,
	}, nil
}

func (h dataHandler) Upload(context.Context, UploadInput) (Identifier, error) {
	return Identifier{}, ErrUploadUnsupported
}

func parseDataURL(raw string) (string, []byte, error) {
	if !strings.HasPrefix(raw, "data:") {
		return "", nil, fmt.Errorf("invalid data URL")
	}
	payloadStart := strings.IndexByte(raw, ',')
	if payloadStart < 0 {
		return "", nil, fmt.Errorf("invalid data URL payload")
	}
	meta := raw[len("data:"):payloadStart]
	payload := raw[payloadStart+1:]

	parts := strings.Split(meta, ";")
	mediaType := "text/plain;charset=US-ASCII"
	if parts[0] != "" {
		mediaType = parts[0]
	}
	isBase64 := false
	for _, part := range parts[1:] {
		if strings.EqualFold(part, "base64") {
			isBase64 = true
		}
	}
	if isBase64 {
		body, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", nil, err
		}
		return mediaType, body, nil
	}
	body, err := url.PathUnescape(payload)
	if err != nil {
		return "", nil, err
	}
	return mediaType, []byte(body), nil
}

func sameMediaType(a string, b string) bool {
	aType, _, aErr := mime.ParseMediaType(a)
	bType, _, bErr := mime.ParseMediaType(b)
	if aErr != nil || bErr != nil {
		return strings.EqualFold(a, b)
	}
	return strings.EqualFold(aType, bType)
}
