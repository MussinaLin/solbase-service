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

// SubmitSolanaProof handles POST /intents/:id/solana-proof
func (h *PaymentIntentsHandler) SubmitSolanaProof(c *gin.Context) {
	intentID := c.Param("id")

	var req dto.SolanaProofRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error:      "Bad Request",
			Message:    err.Error(),
			StatusCode: http.StatusBadRequest,
		})
		return
	}

	resp, err := h.service.SubmitSolanaProof(intentID, req)
	if err != nil {
		log.WithError(err).Error("Failed to submit Solana proof")

		statusCode := http.StatusInternalServerError
		if err.Error() == "payment intent not found" {
			statusCode = http.StatusNotFound
		} else if err.Error()[:27] == "payment intent" && err.Error()[len(err.Error())-6:] == "status" {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, dto.ErrorResponse{
			Error:      http.StatusText(statusCode),
			Message:    err.Error(),
			StatusCode: statusCode,
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// TriggerBasePayment handles POST /intents/:id/trigger-base-payment
func (h *PaymentIntentsHandler) TriggerBasePayment(c *gin.Context) {
	intentID := c.Param("id")

	resp, err := h.service.TriggerBasePayment(intentID)
	if err != nil {
		log.WithError(err).Error("Failed to trigger Base payment")

		statusCode := http.StatusInternalServerError
		if err.Error() == "payment intent not found" {
			statusCode = http.StatusNotFound
		} else if err.Error()[:7] == "Solana" {
			statusCode = http.StatusBadRequest
		}

		c.JSON(statusCode, dto.ErrorResponse{
			Error:      http.StatusText(statusCode),
			Message:    err.Error(),
			StatusCode: statusCode,
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetReceipt handles GET /intents/:id/receipt
func (h *PaymentIntentsHandler) GetReceipt(c *gin.Context) {
	intentID := c.Param("id")

	resp, err := h.service.GetReceipt(intentID)
	if err != nil {
		log.WithError(err).Error("Failed to get receipt")
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
