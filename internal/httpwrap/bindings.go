package httpwrap

import (
	"encoding/json"
	"net/http"
)

// BindBody decodes JSON request body into the target struct.
func BindBody(r *http.Request, target interface{}) *ErrorResponse {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		return &ErrorResponse{
			StatusCode: http.StatusBadRequest,
			ErrorMsg:   err.Error(),
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
