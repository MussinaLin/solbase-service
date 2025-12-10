package handlers

import (
	"net/http"

	"github.com/agent-tech/x402-api-backend/internal/dto"
	"github.com/agent-tech/x402-api-backend/internal/services"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

// PaymentIntentsHandler handles payment intent HTTP requests
type PaymentIntentsHandler struct {
	service *services.PaymentIntentService
}

// NewPaymentIntentsHandler creates a new payment intents handler
func NewPaymentIntentsHandler(service *services.PaymentIntentService) *PaymentIntentsHandler {
	return &PaymentIntentsHandler{
		service: service,
	}
}

// CreateIntent handles POST /intents
// Creates a new payment intent with either email or wallet address as receiver
func (h *PaymentIntentsHandler) CreateIntent(c *gin.Context) {
	var req dto.CreateIntentRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:      "Bad Request",
			Message:    err.Error(),
			StatusCode: http.StatusBadRequest,
		})
		return
	}

	resp, err := h.service.CreateIntent(req)
	if err != nil {
		log.WithError(err).Error("Failed to create intent")

		// Check for validation errors
		if contains(err.Error(), "either email or recipient is required", "cannot provide both") {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{
				Error:      "Bad Request",
				Message:    err.Error(),
				StatusCode: http.StatusBadRequest,
			})
			return
		}

		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:      "Internal Server Error",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// SubmitProof handles POST /intents/:intent_id
// Submits an X402 proof for an existing payment intent
func (h *PaymentIntentsHandler) SubmitProof(c *gin.Context) {
	intentID := c.Param("intent_id")
	if intentID == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:      "Bad Request",
			Message:    "intent_id is required",
			StatusCode: http.StatusBadRequest,
		})
		return
	}

	var req dto.SubmitProofRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:      "Bad Request",
			Message:    err.Error(),
			StatusCode: http.StatusBadRequest,
		})
		return
	}

	resp, err := h.service.SubmitProof(intentID, req)
	if err != nil {
		log.WithError(err).Error("Failed to submit proof")

		// Check for specific error types
		if err.Error() == "payment intent not found" {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{
				Error:      "Not Found",
				Message:    err.Error(),
				StatusCode: http.StatusNotFound,
			})
			return
		}

		// Check for validation errors (bad request)
		if contains(err.Error(), "cannot submit proof", "proof validation failed", "has expired") {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{
				Error:      "Bad Request",
				Message:    err.Error(),
				StatusCode: http.StatusBadRequest,
			})
			return
		}

		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:      "Internal Server Error",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetIntent handles GET /intents?intent_id={id}
// Returns combined status + receipt information
func (h *PaymentIntentsHandler) GetIntent(c *gin.Context) {
	var query dto.QueryIntentRequest

	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:      "Bad Request",
			Message:    err.Error(),
			StatusCode: http.StatusBadRequest,
		})
		return
	}

	resp, err := h.service.GetIntent(query.IntentID)
	if err != nil {
		log.WithError(err).Error("Failed to get intent")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:      "Internal Server Error",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	if resp == nil {
		c.JSON(http.StatusNotFound, dto.ErrorResponse{
			Error:      "Not Found",
			Message:    "Payment intent not found",
			StatusCode: http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// contains checks if any of the substrings are in the string
func contains(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}
