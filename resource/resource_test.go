package resource

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dev-ofa/core-go/httpx"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Run("extracts standard params and custom scheme from a valid identifier", func(t *testing.T) {
		id, err := Parse("ofa-res?auth_id=tenant&media_type=image/png&filename=a.png&x_tag=v#aws_s3://bucket/path/a.png")

		require.NoError(t, err)
		require.Equal(t, "aws_s3", id.Scheme)
		require.Equal(t, "aws_s3://bucket/path/a.png", id.SourceURI)
		require.Equal(t, "tenant", id.AuthID)
		require.Equal(t, "image/png", id.MediaType)
		require.Equal(t, "a.png", id.Params["filename"])
		require.Equal(t, "v", id.Params["x_tag"])
	})

	t.Run("preserves the query inside source_uri", func(t *testing.T) {
		id, err := Parse("ofa-res#https://example.com/a.png?version=1")

		require.NoError(t, err)
		require.Equal(t, "https://example.com/a.png?version=1", id.SourceURI)
	})

	tests := []string{
		"",
		"http://example.com/a.png",
		"ofa-res",
		"ofa-res?bad-name=v#https://example.com/a.png",
		"ofa-res?a=1&a=2#https://example.com/a.png",
		"ofa-res?filename=../a.png#https://example.com/a.png",
		"ofa-res#HTTPS://example.com/a.png",
	}
	for _, raw := range tests {
		t.Run("rejects invalid input "+raw, func(t *testing.T) {
			_, err := Parse(raw)

			require.Error(t, err)
			var parseErr *ParseError
			require.ErrorAs(t, err, &parseErr)
		})
	}
}

func TestDataHandlerOpen(t *testing.T) {
	manager := NewManager(WithDataMaxBytes(16))

	stream, err := manager.Open(context.Background(), "ofa-res?media_type=text/plain#data:text/plain;base64,aGVsbG8=")
	require.NoError(t, err)
	defer stream.Body.Close()
	body, err := io.ReadAll(stream.Body)
	require.NoError(t, err)
	require.Equal(t, "hello", string(body))
	require.Equal(t, "text/plain", stream.MediaType)
	require.Equal(t, int64(5), stream.Size)

	_, err = manager.Open(context.Background(), "ofa-res?media_type=image/png#data:text/plain;base64,aGVsbG8=")
	require.Error(t, err)
	var openErr *OpenError
	require.ErrorAs(t, err, &openErr)

	_, err = manager.Open(context.Background(), "ofa-res#data:text/plain,"+strings.Repeat("a", 17))
	require.ErrorIs(t, err, ErrSizeLimitExceeded)
}

func TestManagerRegisterOpenUpload(t *testing.T) {
	manager := NewManager()
	err := manager.Register("custom", HandlerFuncs{
		OpenFunc: func(ctx context.Context, id Identifier) (*Stream, error) {
			return &Stream{
				Body:      io.NopCloser(strings.NewReader("custom-body")),
				MediaType: id.MediaType,
				Size:      int64(len("custom-body")),
				SourceURI: id.SourceURI,
			}, nil
		},
		UploadFunc: func(ctx context.Context, in UploadInput) (Identifier, error) {
			return Parse("ofa-res#custom://uploaded")
		},
	})
	require.NoError(t, err)

	stream, err := manager.Open(context.Background(), "ofa-res?media_type=text/plain#custom://item")
	require.NoError(t, err)
	body, err := io.ReadAll(stream.Body)
	require.NoError(t, err)
	require.NoError(t, stream.Body.Close())
	require.Equal(t, "custom-body", string(body))

	id, err := manager.Upload(context.Background(), "custom", UploadInput{Body: strings.NewReader("x")})
	require.NoError(t, err)
	require.Equal(t, "custom", id.Scheme)

	_, err = manager.Open(context.Background(), "ofa-res#missing://item")
	require.ErrorIs(t, err, ErrUnsupportedScheme)

	_, err = manager.Upload(context.Background(), "http", UploadInput{})
	require.ErrorIs(t, err, ErrUploadUnsupported)
}

func TestHTTPHandlerOpenDownloadRetryAndHeaders(t *testing.T) {
	attempts := 0
	var gotTimeout string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		gotTimeout = r.Header.Get(httpx.HeaderRemainingTimeoutMS)
		if attempts == 1 {
			http.Error(w, "temporary", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Disposition", `attachment; filename="hello.txt"`)
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	manager := NewManager(WithRetry(2, time.Millisecond, time.Millisecond))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stream, err := manager.Open(ctx, "ofa-res#"+server.URL+"/file")
	require.NoError(t, err)
	require.Equal(t, "text/plain", stream.MediaType)
	require.Equal(t, "hello.txt", stream.Filename)
	body, err := io.ReadAll(stream.Body)
	require.NoError(t, err)
	require.NoError(t, stream.Body.Close())
	require.Equal(t, "hello", string(body))
	require.Equal(t, 2, attempts)
	require.NotEmpty(t, gotTimeout)

	dst := filepath.Join(t.TempDir(), "out.txt")
	err = manager.Download(context.Background(), "ofa-res#"+server.URL+"/file", dst)
	require.NoError(t, err)
	written, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "hello", string(written))
}

func TestHTTPHandlerLimitsAndRedirects(t *testing.T) {
	t.Run("fails when the response exceeds the size limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.(http.Flusher).Flush()
			_, _ = w.Write([]byte("too-large"))
		}))
		defer server.Close()

		manager := NewManager(WithMaxBytes(3))
		stream, err := manager.Open(context.Background(), "ofa-res#"+server.URL)
		require.NoError(t, err)
		_, err = io.ReadAll(stream.Body)
		require.ErrorIs(t, err, ErrSizeLimitExceeded)
		require.NoError(t, stream.Body.Close())
	})

	t.Run("enforces the redirect limit", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/next", http.StatusFound)
		}))
		defer server.Close()

		manager := NewManager(WithRedirectLimit(0))
		_, err := manager.Open(context.Background(), "ofa-res#"+server.URL)
		require.Error(t, err)
	})

	t.Run("allows disabling the default http handler", func(t *testing.T) {
		manager := NewManager(WithHTTPEnabled(false))

		_, err := manager.Open(context.Background(), "ofa-res#http://example.com/a.png")
		require.ErrorIs(t, err, ErrUnsupportedScheme)
	})
}

func TestDownloadCleansTempOnFailure(t *testing.T) {
	manager := NewManager()
	err := manager.Register("fail", HandlerFuncs{
		OpenFunc: func(ctx context.Context, id Identifier) (*Stream, error) {
			return &Stream{
				Body:      errReader{},
				Size:      -1,
				SourceURI: id.SourceURI,
			}, nil
		},
	})
	require.NoError(t, err)

	dir := t.TempDir()
	dst := filepath.Join(dir, "out.bin")
	err = manager.Download(context.Background(), "ofa-res#fail://x", dst)
	require.Error(t, err)
	require.NoFileExists(t, dst)
	entries, readErr := os.ReadDir(dir)
	require.NoError(t, readErr)
	require.Empty(t, entries)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func (errReader) Close() error {
	return nil
}
