package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dev-ofa/core-go/model/datax"
	"github.com/dev-ofa/core-go/pass"
	"github.com/stretchr/testify/require"
)

func TestDoInjectsTraceAndTimeoutHeaders(t *testing.T) {
	var gotTraceID string
	var gotRequestID string
	var gotTimeout string
	var gotCustomPass string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceID = r.Header.Get(HeaderTraceID)
		gotRequestID = r.Header.Get(HeaderRequestID)
		gotTimeout = r.Header.Get(HeaderRemainingTimeoutMS)
		gotCustomPass = r.Header.Get("ofa-pass-feature-flag")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer server.Close()

	ctx := pass.CtxSetTraceID(context.Background(), "8f14e45fceea167a5a36dedd4bea2543")
	ctx = pass.CtxSetPassVal(ctx, "FEATURE_FLAG", "gray")
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var resp map[string]bool

	err := Get(server.URL, Context(ctx), JSONResp(&resp)).Do()
	require.NoError(t, err)
	require.Equal(t, "8f14e45fceea167a5a36dedd4bea2543", gotTraceID)
	require.NotEmpty(t, gotRequestID)
	require.NotEmpty(t, gotTimeout)
	require.Equal(t, "gray", gotCustomPass)
	require.True(t, resp["ok"])
}

func TestRequestBuildersAndHeaders(t *testing.T) {
	t.Run("GET form params are appended to the query and preserve custom headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "custom-value", r.Header.Get("X-Custom"))
			require.Equal(t, []string{"base", "extra"}, r.URL.Query()["key"])
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		}))
		defer server.Close()

		var resp map[string]bool
		err := Get(server.URL+"?key=base",
			SetHeader(http.Header{"X-Custom": []string{"custom-value"}}),
			FormReq(url.Values{"key": []string{"extra"}}),
			JsonResp(&resp),
		).Do()

		require.NoError(t, err)
		require.True(t, resp["ok"])
	})

	t.Run("POST supports text/json/raw/reader request bodies", func(t *testing.T) {
		tests := []struct {
			name         string
			op           AgentOp
			wantType     string
			wantBody     string
			decodeJSON   bool
			expectedJSON map[string]string
		}{
			{
				name:     "text",
				op:       TextReq("hello"),
				wantType: "text/plain; charset=utf-8",
				wantBody: "hello",
			},
			{
				name:         "json",
				op:           JsonReq(map[string]string{"name": "core-go"}),
				wantType:     "application/json; charset=utf-8",
				decodeJSON:   true,
				expectedJSON: map[string]string{"name": "core-go"},
			},
			{
				name:     "raw",
				op:       RawReq("application/octet-stream", []byte("raw-body")),
				wantType: "application/octet-stream",
				wantBody: "raw-body",
			},
			{
				name:     "reader",
				op:       ReaderReq("application/custom", strings.NewReader("reader-body")),
				wantType: "application/custom",
				wantBody: "reader-body",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.Equal(t, tt.wantType, r.Header.Get("Content-Type"))
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					if tt.decodeJSON {
						var got map[string]string
						require.NoError(t, json.Unmarshal(body, &got))
						require.Equal(t, tt.expectedJSON, got)
					} else {
						require.Equal(t, tt.wantBody, string(body))
					}
					_, _ = w.Write([]byte(`{"ok":true}`))
				}))
				defer server.Close()

				var resp map[string]bool
				err := Post(server.URL, tt.op, JsonResp(&resp)).Do()

				require.NoError(t, err)
				require.True(t, resp["ok"])
			})
		}
	})

	t.Run("ReaderReq replays the full body after a retryable transport error", func(t *testing.T) {
		attempts := 0
		client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			attempts++
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.Equal(t, "reader-body", string(body))
			if attempts == 1 {
				return nil, retryableNetError{"dial failed"}
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
				Request:    r,
			}, nil
		})}

		var resp map[string]bool
		err := Post("http://example.test",
			Client(client),
			ReaderReq("application/custom", strings.NewReader("reader-body")),
			JsonResp(&resp),
			Retry(&RetryOpt{Attempts: 2, Idempotent: true, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}),
		).Do()

		require.NoError(t, err)
		require.Equal(t, 2, attempts)
		require.True(t, resp["ok"])
	})
}

func TestHTTPStatusHandling(t *testing.T) {
	t.Run("unexpected status codes return a recognizable upstream HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "teapot", http.StatusTeapot)
		}))
		defer server.Close()

		err := Get(server.URL).Do()

		require.Error(t, err)
		var callErr *CallError
		require.ErrorAs(t, err, &callErr)
		require.Equal(t, http.StatusTeapot, callErr.StatusCode)
		var upstreamErr *datax.UpstreamError
		require.ErrorAs(t, err, &upstreamErr)
		require.Equal(t, http.MethodGet, upstreamErr.Operation)
		require.Equal(t, server.URL, upstreamErr.Target)
		require.Equal(t, datax.ErrCodeUnexpected, datax.CodeOf(err))
		var statusErr *HTTPStatusError
		require.ErrorAs(t, err, &statusErr)
		require.Equal(t, http.StatusTeapot, statusErr.StatusCode)
		var httpErr *datax.ErrHttp
		require.ErrorAs(t, err, &httpErr)
		require.Equal(t, http.StatusTeapot, httpErr.StatusCode)
		require.Contains(t, string(httpErr.Body), "teapot")
	})

	t.Run("ExpectedStatusCodes allows non-200 status codes", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"created":true}`))
		}))
		defer server.Close()

		var resp map[string]bool
		err := Post(server.URL, ExpectedStatusCodes([]int{http.StatusCreated}), JsonResp(&resp)).Do()

		require.NoError(t, err)
		require.True(t, resp["created"])
	})
}

func TestWrapperValidationRetry(t *testing.T) {
	t.Run("wrapper business failures are not retried by default", func(t *testing.T) {
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    30001,
				"message": "business failed",
				"data":    map[string]any{"name": "ignored"},
			})
		}))
		defer server.Close()

		var resp struct {
			Name string `json:"name"`
		}
		err := Get(server.URL, JsonResp(&resp), RespWrapper(NewCommonWrapper()), Retry(&RetryOpt{Attempts: 3})).Do()

		require.Error(t, err)
		require.Equal(t, 1, attempts)
		var wrapperErr *WrapperError
		require.ErrorAs(t, err, &wrapperErr)
		require.Equal(t, 30001, wrapperErr.Code)
		require.Equal(t, 30001, datax.CodeOf(err))
		require.Same(t, wrapperErr.Data, datax.ErrorData(err))
	})

	t.Run("explicit retryable wrappers retry business failures within the remaining budget", func(t *testing.T) {
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if attempts == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":    30001,
					"message": "temporary business failed",
					"data":    map[string]any{"name": "bad"},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"message": "ok",
				"data":    map[string]any{"name": "core-go"},
			})
		}))
		defer server.Close()

		var resp struct {
			Name string `json:"name"`
		}
		err := Get(server.URL,
			JsonResp(&resp),
			RespWrapper(&retryableWrapper{}),
			Retry(&RetryOpt{Attempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}),
		).Do()

		require.NoError(t, err)
		require.Equal(t, 2, attempts)
		require.Equal(t, "core-go", resp.Name)
	})

	t.Run("RetryAppError-compatible wrappers retry business failures", func(t *testing.T) {
		attempts := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if attempts == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":    30001,
					"message": "temporary business failed",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"message": "ok",
				"data":    map[string]any{"name": "core-go"},
			})
		}))
		defer server.Close()

		var resp struct {
			Name string `json:"name"`
		}
		err := Get(server.URL,
			JsonResp(&resp),
			RespWrapper(NewCommonWrapper()),
			Retry(&RetryOpt{Attempts: 2, RetryAppError: true, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}),
		).Do()

		require.NoError(t, err)
		require.Equal(t, 2, attempts)
		require.Equal(t, "core-go", resp.Name)
	})
}

func TestCommonWrapperJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":       0,
			"message":    "ok",
			"request_id": r.Header.Get(HeaderRequestID),
			"data": map[string]any{
				"name": "core-go",
			},
		})
	}))
	defer server.Close()

	var resp struct {
		Name string `json:"name"`
	}
	err := Get(server.URL, JSONResp(&resp), RespWrapper(NewCommonWrapper())).Do()
	require.NoError(t, err)
	require.Equal(t, "core-go", resp.Name)
}

func TestRetryOnlyForIdempotentMethodsByDefault(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "temporary", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	var resp map[string]bool
	err := Get(server.URL, JSONResp(&resp), Retry(&RetryOpt{BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})).Do()
	require.Error(t, err)
	require.Equal(t, 1, attempts)

	attempts = 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, retryableNetError{"dial failed"}
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Request:    req,
		}, nil
	})}
	resp = nil
	err = Get("http://example.test", Client(client), JSONResp(&resp), Retry(&RetryOpt{BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})).Do()
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.True(t, resp["ok"])

	attempts = 0
	err = Post(server.URL, JSONReq(map[string]string{"x": "y"}), Retry(&RetryOpt{Attempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond})).Do()
	require.Error(t, err)
	require.Equal(t, 1, attempts)
}

func TestServiceDiscoveryRewritesURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "inventory.prod", r.Host)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	resolver := ResolverFunc(func(ctx context.Context, req ResolveRequest) (*ResolveResponse, error) {
		require.Equal(t, "inventory", req.ServiceName)
		require.Equal(t, "prod", req.Namespace)
		return &ResolveResponse{
			ServiceName: req.ServiceName,
			Namespace:   req.Namespace,
			Instances: []Instance{{
				Host:         server.Listener.Addr().(*net.TCPAddr).IP.String(),
				Port:         server.Listener.Addr().(*net.TCPAddr).Port,
				Scheme:       "http",
				HealthStatus: HealthStatusHealthy,
			}},
		}, nil
	})

	var resp map[string]bool
	err := Get("http://inventory.prod/api",
		Service(ServiceOptions{EnableDiscovery: true, Resolver: resolver}),
		JSONResp(&resp),
	).Do()
	require.NoError(t, err)
	require.True(t, resp["ok"])
}

func TestServiceDiscoveryFailureAndOverride(t *testing.T) {
	t.Run("fails fast when discovery is enabled without a resolver", func(t *testing.T) {
		err := Get("http://inventory.prod/api",
			Service(ServiceOptions{EnableDiscovery: true}),
		).Do()

		require.ErrorIs(t, err, ErrServiceDiscoveryDisabled)
		require.Equal(t, ErrCodeHTTPServiceDiscoveryDisabled, datax.CodeOf(err))
	})

	t.Run("returns ErrNoHealthyInstance when no instance is healthy", func(t *testing.T) {
		resolver := ResolverFunc(func(ctx context.Context, req ResolveRequest) (*ResolveResponse, error) {
			return &ResolveResponse{
				ServiceName: req.ServiceName,
				Namespace:   req.Namespace,
				Instances: []Instance{{
					Host:         "127.0.0.1",
					Port:         8080,
					HealthStatus: HealthStatusUnhealthy,
				}},
			}, nil
		})

		err := Get("http://inventory.prod/api",
			Service(ServiceOptions{EnableDiscovery: true, Resolver: resolver}),
		).Do()

		require.ErrorIs(t, err, ErrNoHealthyInstance)
		require.Equal(t, ErrCodeHTTPNoHealthyInstance, datax.CodeOf(err))
	})

	t.Run("wraps unknown resolver errors as UpstreamError", func(t *testing.T) {
		root := errors.New("resolver boom")
		err := Get("http://inventory.prod/api",
			Service(ServiceOptions{
				EnableDiscovery: true,
				Resolver: ResolverFunc(func(ctx context.Context, req ResolveRequest) (*ResolveResponse, error) {
					return nil, root
				}),
			}),
		).Do()

		var upstreamErr *datax.UpstreamError
		require.ErrorAs(t, err, &upstreamErr)
		require.ErrorIs(t, err, root)
		require.Equal(t, "http://inventory.prod/api", upstreamErr.Target)
		require.Equal(t, http.MethodGet, upstreamErr.Operation)
		require.Equal(t, datax.ErrCodeUnexpected, datax.CodeOf(err))
	})

	t.Run("wraps unknown picker errors as UpstreamError", func(t *testing.T) {
		root := errors.New("picker boom")
		resolver := ResolverFunc(func(ctx context.Context, req ResolveRequest) (*ResolveResponse, error) {
			return &ResolveResponse{
				ServiceName: req.ServiceName,
				Namespace:   req.Namespace,
				Instances: []Instance{{
					Host:         "127.0.0.1",
					Port:         8080,
					HealthStatus: HealthStatusHealthy,
				}},
			}, nil
		})
		picker := InstancePickerFunc(func(ctx context.Context, req ResolveRequest, resp *ResolveResponse) (*Instance, error) {
			return nil, root
		})

		err := Get("http://inventory.prod/api",
			Service(ServiceOptions{EnableDiscovery: true, Resolver: resolver, Picker: picker}),
		).Do()

		var upstreamErr *datax.UpstreamError
		require.ErrorAs(t, err, &upstreamErr)
		require.ErrorIs(t, err, root)
		require.Equal(t, "http://inventory.prod/api", upstreamErr.Target)
		require.Equal(t, http.MethodGet, upstreamErr.Operation)
		require.Equal(t, datax.ErrCodeUnexpected, datax.CodeOf(err))
	})

	t.Run("InstanceOverride bypasses the resolver and preserves the original Host", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "inventory.prod", r.Host)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer server.Close()

		addr := server.Listener.Addr().(*net.TCPAddr)
		var resp map[string]bool
		err := Get("http://inventory.prod/api",
			Service(ServiceOptions{
				EnableDiscovery: true,
				Resolver: ResolverFunc(func(ctx context.Context, req ResolveRequest) (*ResolveResponse, error) {
					return nil, fmt.Errorf("resolver should not be called")
				}),
				InstanceOverride: &Instance{
					Host:   addr.IP.String(),
					Port:   addr.Port,
					Scheme: "http",
				},
			}),
			JsonResp(&resp),
		).Do()

		require.NoError(t, err)
		require.True(t, resp["ok"])
	})
}

func TestContextFromHeaders(t *testing.T) {
	header := http.Header{}
	header.Set(HeaderTraceID, "8f14e45fceea167a5a36dedd4bea2543")
	header.Set(HeaderRequestID, "req_20260420_153045_7k2m9q4x8c1v6b3n")
	header.Set(HeaderOperator, "user-1")
	header.Set(HeaderTenantID, "tenant-1")
	header.Set(HeaderAppID, "app-1")
	header.Set(HeaderLocale, "en-US")
	header.Set("ofa-pass-feature-flag", "gray")
	header.Set(HeaderRemainingTimeoutMS, "5000")

	ctx, cancel := ContextFromHeaders(context.Background(), header, time.Second, 50*time.Millisecond)
	defer cancel()

	traceID, ok := pass.CtxGetTraceID(ctx)
	require.True(t, ok)
	require.Equal(t, "8f14e45fceea167a5a36dedd4bea2543", traceID)
	requestID, ok := pass.CtxGetRequestID(ctx)
	require.True(t, ok)
	require.Equal(t, "req_20260420_153045_7k2m9q4x8c1v6b3n", requestID)
	operator, ok := pass.CtxGetOperator(ctx)
	require.True(t, ok)
	require.Equal(t, "user-1", operator)
	tenantID, ok := pass.CtxGetTenantID(ctx)
	require.True(t, ok)
	require.Equal(t, "tenant-1", tenantID)
	appID, ok := pass.CtxGetAppID(ctx)
	require.True(t, ok)
	require.Equal(t, "app-1", appID)
	locale, ok := pass.CtxGetLocale(ctx)
	require.True(t, ok)
	require.Equal(t, "en-US", locale)
	requestDeadline, ok := pass.CtxGetRequestDeadline(ctx)
	require.True(t, ok)
	passHeaders := pass.CtxPassHeaders(ctx)
	require.Equal(t, "gray", passHeaders["ofa-pass-feature-flag"])
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.LessOrEqual(t, time.Until(deadline), 50*time.Millisecond)
	require.Equal(t, requestDeadline, deadline)

	ctxWithoutTimeout, cancelWithoutTimeout := ContextFromHeaders(context.Background(), http.Header{}, 0, 0)
	defer cancelWithoutTimeout()
	_, ok = ctxWithoutTimeout.Deadline()
	require.False(t, ok)
	locale, ok = pass.CtxGetLocale(ctxWithoutTimeout)
	require.True(t, ok)
	require.Equal(t, "zh-CN", locale)
}

func TestContextFromHeadersFallsBackToAcceptLanguage(t *testing.T) {
	header := http.Header{}
	header.Set("Accept-Language", "en;q=0.9,zh-CN;q=0.8")

	ctx, cancel := ContextFromHeaders(context.Background(), header, 0, 0)
	defer cancel()

	locale, ok := pass.CtxGetLocale(ctx)
	require.True(t, ok)
	require.Equal(t, "en-US", locale)
}

func TestTimeoutBudgetExhausted(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	err := Get("http://example.invalid", Context(ctx)).Do()

	require.ErrorIs(t, err, ErrTimeoutBudgetExhausted)
	require.Equal(t, ErrCodeHTTPTimeoutBudgetExhausted, datax.CodeOf(err))
}

func TestInvalidOptionsAndResponseReadError(t *testing.T) {
	t.Run("JsonResp requires the target to be a pointer", func(t *testing.T) {
		err := Get("http://example.invalid", JsonResp(struct{}{})).Do()

		require.Error(t, err)
		require.Contains(t, err.Error(), "result payload should be ptr")
	})

	t.Run("RespWrapper requires the wrapper to be a pointer", func(t *testing.T) {
		err := Get("http://example.invalid", RespWrapper(valueWrapper{})).Do()

		require.Error(t, err)
		require.Contains(t, err.Error(), "response wrapper should be ptr")
	})

	t.Run("returns an application error when reading the response body fails", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{},
				Body:       errReadCloser{err: errors.New("read failed")},
				Request:    req,
			}, nil
		})}

		var resp map[string]bool
		err := Get("http://example.test", Client(client), JsonResp(&resp)).Do()

		require.Error(t, err)
		require.Contains(t, err.Error(), "read body failed")
	})
}

func TestRawAndHybridResponseHandlers(t *testing.T) {
	t.Run("RawResp copies the response and body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Resp", "ok")
			_, _ = w.Write([]byte("raw-response"))
		}))
		defer server.Close()

		var resp http.Response
		var body []byte
		err := Get(server.URL, RawResp(&resp, &body)).Do()

		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "ok", resp.Header.Get("X-Resp"))
		require.Equal(t, "raw-response", string(body))
	})

	t.Run("does not pre-read the body for successful responses", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{},
				Body:       &trackingReadCloser{r: strings.NewReader("stream-response")},
				Request:    req,
			}, nil
		})}

		var body string
		handler := RespHandlerFunc(func(resp *http.Response, respWrapper Wrapper) error {
			trackingBody, ok := resp.Body.(*trackingReadCloser)
			require.True(t, ok)
			require.Zero(t, trackingBody.reads)
			bs, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			body = string(bs)
			return nil
		})
		err := Get("http://example.test", Client(client), AgentOpFunc(func(a *Agent) error {
			a.respHandler = handler
			return nil
		})).Do()

		require.NoError(t, err)
		require.Equal(t, "stream-response", body)
	})

	t.Run("HybridResp selects a handler by predicate", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer server.Close()

		var resp map[string]bool
		err := Get(server.URL, HybridResp(RespHandlerPredicate{
			Predicate: func(response *http.Response) bool {
				return strings.Contains(response.Header.Get("Content-Type"), "application/json")
			},
			RespHandler: JsonResp(&resp),
		})).Do()

		require.NoError(t, err)
		require.True(t, resp["ok"])
	})

	t.Run("HybridResp only runs the first matching handler", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer server.Close()

		firstCalls := 0
		secondCalls := 0
		err := Get(server.URL, HybridResp(
			RespHandlerPredicate{
				Predicate: func(response *http.Response) bool { return true },
				RespHandler: RespHandlerFunc(func(resp *http.Response, respWrapper Wrapper) error {
					firstCalls++
					_, err := io.ReadAll(resp.Body)
					return err
				}),
			},
			RespHandlerPredicate{
				Predicate: func(response *http.Response) bool { return true },
				RespHandler: RespHandlerFunc(func(resp *http.Response, respWrapper Wrapper) error {
					secondCalls++
					return nil
				}),
			},
		)).Do()

		require.NoError(t, err)
		require.Equal(t, 1, firstCalls)
		require.Zero(t, secondCalls)
	})
}

func TestDoStream(t *testing.T) {
	t.Run("returns an open successful response body for the caller to close", func(t *testing.T) {
		trackingBody := &trackingReadCloser{r: strings.NewReader("stream-response")}
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       trackingBody,
				Request:    req,
			}, nil
		})}

		resp, err := Get("http://example.test", Client(client), TimeoutQuota(time.Second)).DoStream()
		require.NoError(t, err)
		require.Equal(t, "text/plain", resp.Header.Get("Content-Type"))
		require.Zero(t, trackingBody.reads)
		require.Zero(t, trackingBody.closes)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, "stream-response", string(body))
		require.NoError(t, resp.Body.Close())
		require.Equal(t, 1, trackingBody.closes)
	})

	t.Run("does not retry failing status codes for streamed responses by default", func(t *testing.T) {
		attempts := 0
		failedBody := &trackingReadCloser{r: strings.NewReader("temporary")}
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			attempts++
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Status:     "500 Internal Server Error",
				Header:     http.Header{},
				Body:       failedBody,
				Request:    req,
			}, nil
		})}

		resp, err := Get("http://example.test",
			Client(client),
			Retry(&RetryOpt{Attempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond}),
		).DoStream()
		require.Nil(t, resp)
		require.Error(t, err)
		require.Equal(t, 1, attempts)
		require.Equal(t, 1, failedBody.closes)
	})

	t.Run("closes the error response body for unexpected status codes", func(t *testing.T) {
		trackingBody := &trackingReadCloser{r: strings.NewReader("teapot")}
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusTeapot,
				Status:     "418 I'm a teapot",
				Header:     http.Header{},
				Body:       trackingBody,
				Request:    req,
			}, nil
		})}

		resp, err := Get("http://example.test", Client(client)).DoStream()
		require.Nil(t, resp)
		require.Error(t, err)
		var callErr *CallError
		require.ErrorAs(t, err, &callErr)
		require.Equal(t, http.StatusTeapot, callErr.StatusCode)
		var upstreamErr *datax.UpstreamError
		require.ErrorAs(t, err, &upstreamErr)
		var statusErr *HTTPStatusError
		require.ErrorAs(t, err, &statusErr)
		require.Equal(t, http.StatusTeapot, statusErr.StatusCode)
		var httpErr *datax.ErrHttp
		require.ErrorAs(t, err, &httpErr)
		require.Equal(t, http.StatusTeapot, httpErr.StatusCode)
		require.Equal(t, "teapot", string(httpErr.Body))
		require.Equal(t, 1, trackingBody.closes)
		require.Greater(t, trackingBody.reads, 0)
	})

	t.Run("closing a successful response body releases the internal timeout context", func(t *testing.T) {
		var reqCtx context.Context
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			reqCtx = req.Context()
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{},
				Body:       &trackingReadCloser{r: strings.NewReader("stream-response")},
				Request:    req,
			}, nil
		})}

		resp, err := Get("http://example.test", Client(client), TimeoutQuota(time.Second)).DoStream()
		require.NoError(t, err)
		require.NotNil(t, reqCtx)
		select {
		case <-reqCtx.Done():
			t.Fatal("request context should stay active before response body close")
		default:
		}

		require.NoError(t, resp.Body.Close())
		select {
		case <-reqCtx.Done():
		case <-time.After(time.Second):
			t.Fatal("request context should be canceled after response body close")
		}
	})

	t.Run("does not run response handlers or wrappers", func(t *testing.T) {
		trackingBody := &trackingReadCloser{r: strings.NewReader(`{"ok":true}`)}
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       trackingBody,
				Request:    req,
			}, nil
		})}
		wrapper := &countingWrapper{}
		var decoded map[string]bool

		resp, err := Get("http://example.test",
			Client(client),
			JSONResp(&decoded),
			RespWrapper(wrapper),
		).DoStream()
		require.NoError(t, err)
		require.Empty(t, decoded)
		require.Zero(t, wrapper.setDataCalls)
		require.Zero(t, wrapper.validateCalls)
		require.Zero(t, trackingBody.reads)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, `{"ok":true}`, string(body))
		require.NoError(t, resp.Body.Close())
	})
}

func TestRandomPickerSelectionRules(t *testing.T) {
	picker := RandomPicker{}

	_, err := picker.Pick(context.Background(), ResolveRequest{}, nil)
	require.ErrorIs(t, err, ErrNoHealthyInstance)

	_, err = picker.Pick(context.Background(), ResolveRequest{}, &ResolveResponse{Instances: []Instance{{
		Host:         "127.0.0.1",
		HealthStatus: HealthStatusUnhealthy,
	}}})
	require.ErrorIs(t, err, ErrNoHealthyInstance)

	inst, err := picker.Pick(context.Background(), ResolveRequest{PreferredZone: "zone-a"}, &ResolveResponse{Instances: []Instance{
		{Host: "10.0.0.1", HealthStatus: HealthStatusHealthy, Zone: "zone-b"},
		{Host: "10.0.0.2", HealthStatus: HealthStatusHealthy, Zone: "zone-a"},
	}})
	require.NoError(t, err)
	require.Equal(t, "10.0.0.2", inst.Host)

	inst, err = picker.Pick(context.Background(), ResolveRequest{ResolveMode: ResolveModeAll}, &ResolveResponse{Instances: []Instance{{
		Host:         "10.0.0.3",
		HealthStatus: HealthStatusUnhealthy,
	}}})
	require.NoError(t, err)
	require.Equal(t, "10.0.0.3", inst.Host)
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

type RespHandlerFunc func(resp *http.Response, respWrapper Wrapper) error

func (f RespHandlerFunc) HandleResponse(resp *http.Response, respWrapper Wrapper) error {
	return f(resp, respWrapper)
}

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errReadCloser struct {
	err error
}

func (e errReadCloser) Read(p []byte) (int, error) {
	return 0, e.err
}

func (e errReadCloser) Close() error {
	return nil
}

type trackingReadCloser struct {
	r      io.Reader
	reads  int
	closes int
}

func (t *trackingReadCloser) Read(p []byte) (int, error) {
	t.reads++
	return t.r.Read(p)
}

func (t *trackingReadCloser) Close() error {
	t.closes++
	return nil
}

type valueWrapper struct{}

func (valueWrapper) SetData(ret any) {}

func (valueWrapper) Validate() error {
	return nil
}

type retryableWrapper struct {
	CommonWrapper
}

func (w *retryableWrapper) Validate() error {
	err := w.CommonWrapper.Validate()
	if err == nil {
		return nil
	}
	return datax.WithRetryableError(err)
}

type countingWrapper struct {
	setDataCalls   int
	validateCalls  int
	underlyingData any
}

func (w *countingWrapper) SetData(ret any) {
	w.setDataCalls++
	w.underlyingData = ret
}

func (w *countingWrapper) Validate() error {
	w.validateCalls++
	return nil
}

type retryableNetError struct {
	msg string
}

func (e retryableNetError) Error() string {
	return e.msg
}

func (e retryableNetError) Timeout() bool {
	return false
}

func (e retryableNetError) Temporary() bool {
	return true
}
