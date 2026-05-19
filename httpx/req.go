package httpx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// ReqPreHandler mutates or replaces a request before it is sent.
type ReqPreHandler interface {
	PreHandleRequest(req *http.Request) (*http.Request, error)
}

// ReqPreHandlerFunc adapts a function into a request pre-handler.
type ReqPreHandlerFunc func(req *http.Request) (*http.Request, error)

// PreHandleRequest implements ReqPreHandler.
func (f ReqPreHandlerFunc) PreHandleRequest(req *http.Request) (*http.Request, error) {
	return f(req)
}

// TextReq sends a text/plain request body.
func TextReq(reqBody string) AgentOp {
	return AgentOpFunc(func(agent *Agent) error {
		agent.reqPreHandlers = append(agent.reqPreHandlers, ReqPreHandlerFunc(func(req *http.Request) (*http.Request, error) {
			req.Header.Set("Content-Type", "text/plain; charset=utf-8")
			r := strings.NewReader(reqBody)
			req.ContentLength = int64(r.Len())
			req.Body = io.NopCloser(r)
			return req, nil
		}))
		return nil
	})
}

// JSONReq sends an application/json request body.
func JSONReq(reqBody any) AgentOp {
	return AgentOpFunc(func(agent *Agent) error {
		agent.reqPreHandlers = append(agent.reqPreHandlers, ReqPreHandlerFunc(func(req *http.Request) (*http.Request, error) {
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
			buffer := bytes.Buffer{}
			if err := json.NewEncoder(&buffer).Encode(reqBody); err != nil {
				return nil, fmt.Errorf("json marshal failed: %w", err)
			}
			req.ContentLength = int64(buffer.Len())
			req.Body = io.NopCloser(bytes.NewReader(buffer.Bytes()))
			return req, nil
		}))
		return nil
	})
}

// JsonReq is kept for compatibility with goreq naming.
func JsonReq(reqBody any) AgentOp {
	return JSONReq(reqBody)
}

// RawReq sends the provided bytes as the request body.
func RawReq(contentType string, body []byte) AgentOp {
	return AgentOpFunc(func(agent *Agent) error {
		agent.reqPreHandlers = append(agent.reqPreHandlers, ReqPreHandlerFunc(func(req *http.Request) (*http.Request, error) {
			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}
			req.ContentLength = int64(len(body))
			req.Body = io.NopCloser(bytes.NewReader(body))
			return req, nil
		}))
		return nil
	})
}

// ReaderReq sends the provided reader as the request body.
func ReaderReq(contentType string, body io.Reader) AgentOp {
	var (
		once     sync.Once
		bodyBuf  []byte
		bodyErr  error
	)
	return AgentOpFunc(func(agent *Agent) error {
		agent.reqPreHandlers = append(agent.reqPreHandlers, ReqPreHandlerFunc(func(req *http.Request) (*http.Request, error) {
			once.Do(func() {
				if body == nil {
					return
				}
				bodyBuf, bodyErr = io.ReadAll(body)
			})
			if bodyErr != nil {
				return nil, fmt.Errorf("read request body failed: %w", bodyErr)
			}
			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}
			req.ContentLength = int64(len(bodyBuf))
			req.Body = io.NopCloser(bytes.NewReader(bodyBuf))
			return req, nil
		}))
		return nil
	})
}

// FormReq sends URL values as query parameters for GET and as form body otherwise.
func FormReq(values url.Values) AgentOp {
	return AgentOpFunc(func(agent *Agent) error {
		agent.reqPreHandlers = append(agent.reqPreHandlers, ReqPreHandlerFunc(func(req *http.Request) (*http.Request, error) {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
			switch agent.method {
			case http.MethodGet, http.MethodHead:
				query := req.URL.Query()
				for k, vals := range values {
					for _, v := range vals {
						query.Add(k, v)
					}
				}
				req.URL.RawQuery = query.Encode()
			default:
				encoded := values.Encode()
				r := strings.NewReader(encoded)
				req.ContentLength = int64(r.Len())
				req.Body = io.NopCloser(r)
			}
			return req, nil
		}))
		return nil
	})
}
