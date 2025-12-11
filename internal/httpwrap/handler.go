package httpwrap

import (
	"encoding/json"
	"net/http"

	log "github.com/sirupsen/logrus"
)

// errorBody represents the JSON structure for error responses.
type errorBody struct {
	Error      string `json:"error"`
	Message    string `json:"message"`
	StatusCode int    `json:"statusCode"`
}

// Handler wraps a HandlerFunc to return an http.HandlerFunc.
func Handler(fn HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, errResp := fn(r)

		if errResp != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(errResp.StatusCode)

			body := errorBody{
				Error:      http.StatusText(errResp.StatusCode),
				Message:    errResp.ErrorMsg,
				StatusCode: errResp.StatusCode,
			}

			if err := json.NewEncoder(w).Encode(body); err != nil {
				log.WithError(err).Error("Failed to encode error response")
			}

			return
		}

		if resp.Header != nil {
			for k, v := range resp.Header {
				for _, val := range v {
					w.Header().Add(k, val)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)

		if resp.Body != nil {
			if err := json.NewEncoder(w).Encode(resp.Body); err != nil {
				log.WithError(err).Error("Failed to encode response body")
			}
		}
	}
}
