package vectoramp

import (
	"fmt"
	"net/http"
)

// APIError is returned when the VectorAmp API responds with a non-2xx status.
type APIError struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	Message    string
}

// Error returns a concise status/message string for the API error.
func (e *APIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message != "" {
		return fmt.Sprintf("vectoramp: api error %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("vectoramp: api error %d", e.StatusCode)
}
