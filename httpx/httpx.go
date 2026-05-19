package httpx

// CommonWrapper is a spec-compatible response wrapper with code/message/request_id/data.
type CommonWrapper struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Data      any    `json:"data"`

	allowCodes []int
}

// NewCommonWrapper returns a response wrapper that treats code 0 and allowCodes as success.
func NewCommonWrapper(allowCodes ...int) *CommonWrapper {
	return &CommonWrapper{allowCodes: allowCodes}
}

// SetData sets the data target for JSON decoding.
func (c *CommonWrapper) SetData(data any) {
	c.Data = data
}

// Validate validates the application response code.
func (c *CommonWrapper) Validate() error {
	if c.Code == 0 {
		return nil
	}
	for _, code := range c.allowCodes {
		if c.Code == code {
			return nil
		}
	}
	return &WrapperError{Code: c.Code, Message: c.Message, RequestID: c.RequestID, Data: c.Data}
}

// WrapperError describes an application wrapper validation failure.
type WrapperError struct {
	Code      int
	Message   string
	RequestID string
	Data      any
}

// Error implements the error interface.
func (e *WrapperError) Error() string {
	return "httpx wrapper validate failed"
}
