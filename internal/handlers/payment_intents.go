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
// Accepts X402 settle_proof and merchant_recipient, creates intent and processes asynchronously
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
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{
			Error:      "Internal Server Error",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusCreated, resp)
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
