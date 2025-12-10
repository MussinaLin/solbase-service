package httpwrap

import "net/http"

// Response represents a successful HTTP response.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       interface{}
}

// ErrorResponse represents an error HTTP response.
type ErrorResponse struct {
	StatusCode int
	ErrorMsg   string
	Err        error
}

// HandlerFunc is the signature for httpwrap handlers.
type HandlerFunc func(r *http.Request) (*Response, *ErrorResponse)
