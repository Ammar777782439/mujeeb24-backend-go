package gemini

import (
	"fmt"
	"net/http"
)

// GeminiInteractionHTTPError is a typed outage signal. Do not expose raw
// provider bodies to failover routing or customer-facing error logs.
type GeminiInteractionHTTPError struct {
	StatusCode int
}

func (e *GeminiInteractionHTTPError) Error() string {
	return fmt.Sprintf("gemini interactions http %d", e.StatusCode)
}

func (e *GeminiInteractionHTTPError) Retryable() bool {
	return e.StatusCode == http.StatusRequestTimeout || e.StatusCode == http.StatusTooManyRequests ||
		(e.StatusCode >= http.StatusInternalServerError && e.StatusCode <= 599)
}
