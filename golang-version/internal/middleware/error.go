package middleware

import (
	"net/http"

	"github.com/agent-tech/x402-api-backend/internal/dto"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// ErrorHandler is a middleware that handles panics and errors
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Log the error
				log.WithFields(log.Fields{
					"error": err,
					"path":  c.Request.URL.Path,
				}).Error("Panic recovered")

				// Return error response
				c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
					Error:      "Internal Server Error",
					Message:    "An unexpected error occurred",
					StatusCode: http.StatusInternalServerError,
				})

				// Abort the request
				c.Abort()
			}
		}()

		c.Next()
	}
}
