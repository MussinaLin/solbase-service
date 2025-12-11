package httpwrap

import (
	"encoding/json"
	"io"
	"net/http"
)

// MaxRequestBodySize is the maximum allowed request body size (1MB).
const MaxRequestBodySize = 1 << 20 // 1 MB

// BindBody decodes JSON request body into the target struct.
// Limits request body size to prevent memory exhaustion attacks.
func BindBody(r *http.Request, target interface{}) *ErrorResponse {
	// Limit request body size to prevent memory exhaustion
	r.Body = http.MaxBytesReader(nil, r.Body, MaxRequestBodySize)

	// Ensure body is closed after reading
	defer func() {
		// Drain any remaining body content before closing
		_, _ = io.Copy(io.Discard, r.Body)
		r.Body.Close()
	}()

	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		// Check if the error is due to body size limit
		if err.Error() == "http: request body too large" {
			return &ErrorResponse{
				StatusCode: http.StatusRequestEntityTooLarge,
				ErrorMsg:   "request body too large",
				Err:        err,
			}
		}

		return &ErrorResponse{
			StatusCode: http.StatusBadRequest,
			ErrorMsg:   "invalid JSON request body",
			Err:        err,
		}
	}

	return nil
}

// NewInvalidParamErrorResponse creates an error response for invalid parameters.
func NewInvalidParamErrorResponse(param string) *ErrorResponse {
	return &ErrorResponse{
		StatusCode: http.StatusBadRequest,
		ErrorMsg:   "invalid parameter: " + param,
	}
}

// NewNotFoundErrorResponse creates a not found error response.
func NewNotFoundErrorResponse(msg string) *ErrorResponse {
	return &ErrorResponse{
		StatusCode: http.StatusNotFound,
		ErrorMsg:   msg,
	}
}

// NewInternalErrorResponse creates an internal server error response.
func NewInternalErrorResponse(err error) *ErrorResponse {
	return &ErrorResponse{
		StatusCode: http.StatusInternalServerError,
		ErrorMsg:   err.Error(),
		Err:        err,
	}
}
