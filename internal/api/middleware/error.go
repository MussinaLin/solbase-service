package middleware

import (
	"encoding/json"
	"net/http"
	"runtime/debug"

	log "github.com/sirupsen/logrus"
)

// errorResponse represents a JSON error response.
type errorResponse struct {
	Error      string `json:"error"`
	Message    string `json:"message"`
	StatusCode int    `json:"statusCode"`
}

// ErrorHandler returns a middleware that handles panics and recovers gracefully.
func ErrorHandler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.WithFields(log.Fields{
						"error": err,
						"stack": string(debug.Stack()),
						"path":  r.URL.Path,
					}).Error("Panic recovered")

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)

					json.NewEncoder(w).Encode(errorResponse{
						Error:      "Internal Server Error",
						Message:    "An unexpected error occurred",
						StatusCode: http.StatusInternalServerError,
					})
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
