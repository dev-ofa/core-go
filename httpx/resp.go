package httpx

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/dev-ofa/core-go/model/datax"
)

// Wrapper describes a standard application response wrapper.
type Wrapper interface {
	SetData(ret any)
	Validate() error
}

// RespHandler handles a successful HTTP response.
type RespHandler interface {
	HandleResponse(resp *http.Response, respWrapper Wrapper) error
}

// RawResp copies the response metadata and body bytes.
func RawResp(resp *http.Response, bs *[]byte) *RawRespHandler {
	return &RawRespHandler{resp: resp, bs: bs}
}

// RawRespHandler stores a raw HTTP response.
type RawRespHandler struct {
	resp *http.Response
	bs   *[]byte
}

// HandleResponse implements RespHandler.
func (h *RawRespHandler) HandleResponse(resp *http.Response, _ Wrapper) error {
	if h.resp != nil {
		*h.resp = *resp
	}
	if h.bs != nil {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read http body failed: %w", err)
		}
		*h.bs = body
	}
	return nil
}

// InitialAgent installs the raw response handler.
func (h *RawRespHandler) InitialAgent(a *Agent) error {
	a.respHandler = h
	return nil
}

// RespHandlerPredicate selects a response handler by inspecting the response.
type RespHandlerPredicate struct {
	Predicate   func(response *http.Response) bool
	RespHandler RespHandler
}

// HybridResp builds a response handler selected by predicates.
func HybridResp(predicate ...RespHandlerPredicate) *HybridHandler {
	return &HybridHandler{predicates: predicate}
}

// HybridHandler delegates response handling to matching handlers.
type HybridHandler struct {
	predicates []RespHandlerPredicate
}

// HandleResponse implements RespHandler.
func (h *HybridHandler) HandleResponse(resp *http.Response, respWrapper Wrapper) error {
	for i, p := range h.predicates {
		if p.Predicate == nil || !p.Predicate(resp) {
			continue
		}
		if p.RespHandler == nil {
			return fmt.Errorf("hybrid resp handler is nil at %d", i)
		}
		if err := p.RespHandler.HandleResponse(resp, respWrapper); err != nil {
			return fmt.Errorf("hybrid resp handle failed at %d: %w", i, err)
		}
		return nil
	}
	return nil
}

// InitialAgent installs the hybrid response handler.
func (h *HybridHandler) InitialAgent(a *Agent) error {
	a.respHandler = h
	return nil
}

// JSONResp decodes a JSON response into ret. ret must be a pointer.
func JSONResp(ret any) *JSONRespHandler {
	return &JSONRespHandler{ret: ret}
}

// JsonResp is kept for compatibility with goreq naming.
func JsonResp(ret any) *JSONRespHandler {
	return JSONResp(ret)
}

// JSONRespHandler decodes JSON responses and validates optional wrappers.
type JSONRespHandler struct {
	ret any
}

// HandleResponse implements RespHandler.
func (h *JSONRespHandler) HandleResponse(resp *http.Response, respWrapper Wrapper) error {
	ret := h.ret
	if respWrapper != nil {
		respWrapper.SetData(h.ret)
		ret = respWrapper
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body failed: %w", err)
	}
	if len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, ret); err != nil {
		return fmt.Errorf("unmarshal body failed: %w, body: %s", err, string(body))
	}
	if respWrapper != nil {
		return respWrapper.Validate()
	}
	return nil
}

// InitialAgent installs the JSON response handler.
func (h *JSONRespHandler) InitialAgent(a *Agent) error {
	if h.ret == nil || reflect.TypeOf(h.ret).Kind() != reflect.Ptr {
		return datax.NewValidationError("result payload should be ptr", nil, nil)
	}
	a.respHandler = h
	return nil
}
