package middleware

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
)

// RecaptchaConfig holds the configuration for reCAPTCHA middleware.
type RecaptchaConfig struct {
	SecretKey    string
	MinScore     float64
	VerifyURL    string
	EnabledPaths []string // Paths that require reCAPTCHA verification
	SkipOnError  bool     // Skip verification if reCAPTCHA service is down
}

// recaptchaVerifyResponse represents the response from Google reCAPTCHA API.
type recaptchaVerifyResponse struct {
	Success     bool     `json:"success"`
	Score       float64  `json:"score"`
	Action      string   `json:"action"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
	ErrorCodes  []string `json:"error-codes,omitempty"`
}

// Recaptcha returns a middleware that verifies Google reCAPTCHA v3 tokens.
func Recaptcha(config RecaptchaConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip if reCAPTCHA is not enabled (empty secret key)
			if config.SecretKey == "" {
				log.Warn("reCAPTCHA middleware: secret key not configured, skipping verification")
				next.ServeHTTP(w, r)
				return
			}

			// Check if current path requires reCAPTCHA verification
			currentPath := r.URL.Path
			requiresVerification := false

			for _, path := range config.EnabledPaths {
				if pathMatches(currentPath, path) {
					requiresVerification = true
					break
				}
			}

			// Skip verification if path is not in enabled paths
			if !requiresVerification {
				next.ServeHTTP(w, r)
				return
			}

			// Extract reCAPTCHA token from request header
			token := r.Header.Get("X-Recaptcha-Token")
			if token == "" {
				log.Warn("reCAPTCHA middleware: token not found in request")
				writeJSONError(w, http.StatusBadRequest, "Bad Request", "reCAPTCHA token is required")
				return
			}

			// Verify token with Google reCAPTCHA API
			verified, score, err := verifyRecaptchaToken(token, r.RemoteAddr, config)
			if err != nil {
				log.WithFields(log.Fields{
					"error": err,
					"path":  currentPath,
				}).Error("reCAPTCHA middleware: verification failed")

				// Skip verification if configured to do so on error
				if config.SkipOnError {
					log.Warn("reCAPTCHA middleware: skipping verification due to error")
					next.ServeHTTP(w, r)
					return
				}

				writeJSONError(w, http.StatusServiceUnavailable, "Service Unavailable", "Failed to verify reCAPTCHA")
				return
			}

			// Check if verification was successful
			if !verified {
				log.WithFields(log.Fields{
					"path":  currentPath,
					"score": score,
				}).Error("reCAPTCHA middleware: verification failed")

				writeJSONError(w, http.StatusForbidden, "Forbidden", "reCAPTCHA verification failed")
				return
			}

			// Check if score meets minimum threshold
			if score < config.MinScore {
				log.WithFields(log.Fields{
					"path":      currentPath,
					"score":     score,
					"min_score": config.MinScore,
				}).Warn("reCAPTCHA middleware: score below threshold")

				writeJSONError(w, http.StatusForbidden, "Forbidden", "reCAPTCHA score too low")
				return
			}

			log.WithFields(log.Fields{
				"path":  currentPath,
				"score": score,
			}).Debug("reCAPTCHA middleware: verification successful")

			next.ServeHTTP(w, r)
		})
	}
}

// verifyRecaptchaToken verifies the reCAPTCHA token with Google's API.
func verifyRecaptchaToken(token, remoteIP string, config RecaptchaConfig) (bool, float64, error) {
	// Convert to form data (Google reCAPTCHA API expects form data)
	formData := fmt.Sprintf("secret=%s&response=%s", config.SecretKey, token)
	if remoteIP != "" {
		formData += fmt.Sprintf("&remoteip=%s", remoteIP)
	}

	// Send request to Google reCAPTCHA API
	resp, err := http.Post(
		config.VerifyURL,
		"application/x-www-form-urlencoded",
		strings.NewReader(formData),
	)
	if err != nil {
		return false, 0, fmt.Errorf("failed to send request to reCAPTCHA API: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, 0, fmt.Errorf("failed to read reCAPTCHA API response: %w", err)
	}

	// Parse response
	var verifyResp recaptchaVerifyResponse
	if err := json.Unmarshal(body, &verifyResp); err != nil {
		return false, 0, fmt.Errorf("failed to parse reCAPTCHA API response: %w", err)
	}

	// Log error codes if present
	if len(verifyResp.ErrorCodes) > 0 {
		log.WithFields(log.Fields{
			"error_codes": verifyResp.ErrorCodes,
		}).Error("reCAPTCHA API returned error codes")
	}

	return verifyResp.Success, verifyResp.Score, nil
}

// pathMatches checks if the current path matches the configured path pattern.
// Supports exact match and wildcard suffix (e.g., "/api/*").
func pathMatches(currentPath, pattern string) bool {
	// Exact match
	if currentPath == pattern {
		return true
	}

	// Wildcard suffix match
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(currentPath, prefix)
	}

	return false
}

// writeJSONError writes a JSON error response.
func writeJSONError(w http.ResponseWriter, statusCode int, error, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	json.NewEncoder(w).Encode(errorResponse{
		Error:      error,
		Message:    message,
		StatusCode: statusCode,
	})
}
