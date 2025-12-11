package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/agent-tech/x402-api-backend/internal/httpwrap"
	"github.com/agent-tech/x402-api-backend/internal/payment"
	"github.com/agent-tech/x402-api-backend/internal/payment/service"
	"github.com/go-chi/chi/v5"
	log "github.com/sirupsen/logrus"
)

// sanitizeInternalError returns a generic error message while logging the detailed error.
func sanitizeInternalError(err error, context string) string {
	log.WithError(err).WithField("context", context).Error("Internal error")

	return "an internal error occurred, please try again later"
}

// AddRoutes registers payment routes.
func AddRoutes(r chi.Router, svc *service.Service) {
	r.Post("/intents", httpwrap.Handler(createIntent(svc)))
	r.Post("/intents/{intent_id}", httpwrap.Handler(submitProof(svc)))
	r.Get("/intents", httpwrap.Handler(getIntent(svc)))
}

// Request DTOs

// CreateIntentRequest represents the request to create a payment intent.
type CreateIntentRequest struct {
	Email      string `json:"email"`
	Recipient  string `json:"recipient"`
	Amount     string `json:"amount"`
	PayerChain string `json:"payer_chain"`
}

// SubmitProofRequest represents the request to submit a proof.
type SubmitProofRequest struct {
	SettleProof string `json:"settle_proof"`
}

// Response DTOs

// CreateIntentResponse represents the response after creating an intent.
type CreateIntentResponse struct {
	IntentID          string    `json:"intent_id"`
	Email             *string   `json:"email,omitempty"`
	MerchantRecipient string    `json:"merchant_recipient"`
	SourceRecipient   *string   `json:"source_recipient,omitempty"`
	Amount            string    `json:"amount"`
	PayerChain        string    `json:"payer_chain"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// SubmitProofResponse represents the response after submitting a proof.
type SubmitProofResponse struct {
	IntentID          string    `json:"intent_id"`
	MerchantRecipient string    `json:"merchant_recipient"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// SourcePaymentResponse represents payment details on the source/payer chain.
type SourcePaymentResponse struct {
	Chain       string    `json:"chain"`
	TxHash      string    `json:"tx_hash"`
	SettleProof string    `json:"settle_proof"`
	SettledAt   time.Time `json:"settled_at"`
	ExplorerURL string    `json:"explorer_url"`
}

// BasePaymentResponse represents Base payment details.
type BasePaymentResponse struct {
	TxHash      string    `json:"tx_hash"`
	SettleProof string    `json:"settle_proof"`
	SettledAt   time.Time `json:"settled_at"`
	ExplorerURL string    `json:"explorer_url"`
}

// IntentResponse represents the combined status + receipt response.
type IntentResponse struct {
	IntentID          string                 `json:"intent_id"`
	Status            string                 `json:"status"`
	Amount            *string                `json:"amount,omitempty"`
	PayerChain        string                 `json:"payer_chain"`
	MerchantRecipient string                 `json:"merchant_recipient"`
	ReceiverEmail     *string                `json:"receiver_email,omitempty"`
	PayerWallet       *string                `json:"payer_wallet,omitempty"`
	ErrorMessage      *string                `json:"error_message,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	ExpiresAt         time.Time              `json:"expires_at"`
	CompletedAt       *time.Time             `json:"completed_at,omitempty"`
	SourcePayment     *SourcePaymentResponse `json:"source_payment,omitempty"`
	BasePayment       *BasePaymentResponse   `json:"base_payment,omitempty"`
}

// Handlers

func createIntent(svc *service.Service) httpwrap.HandlerFunc {
	return func(r *http.Request) (*httpwrap.Response, *httpwrap.ErrorResponse) {
		var req CreateIntentRequest

		if errResp := httpwrap.BindBody(r, &req); errResp != nil {
			return nil, errResp
		}

		ctx := r.Context()

		intent, err := svc.CreateIntent(ctx, &payment.CreateIntentParams{
			Email:      req.Email,
			Recipient:  req.Recipient,
			Amount:     req.Amount,
			PayerChain: req.PayerChain,
		})
		if err != nil {
			// User-facing validation errors - safe to expose
			if errors.Is(err, payment.ErrInvalidInput) ||
				errors.Is(err, payment.ErrInvalidPayerChain) ||
				errors.Is(err, payment.ErrInvalidEmail) ||
				errors.Is(err, payment.ErrInvalidRecipient) ||
				errors.Is(err, payment.ErrInvalidAmount) {
				return nil, &httpwrap.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					ErrorMsg:   err.Error(),
					Err:        err,
				}
			}

			// Internal errors - sanitize to avoid leaking implementation details
			return nil, &httpwrap.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				ErrorMsg:   sanitizeInternalError(err, "create_intent"),
				Err:        err,
			}
		}

		return &httpwrap.Response{
			StatusCode: http.StatusCreated,
			Body: &CreateIntentResponse{
				IntentID:          intent.IntentID,
				Email:             intent.ReceiverEmail,
				MerchantRecipient: intent.MerchantRecipient,
				SourceRecipient:   intent.SourceRecipient,
				Amount:            intent.Amount,
				PayerChain:        intent.PayerChain,
				Status:            intent.Status,
				CreatedAt:         intent.CreatedAt,
				ExpiresAt:         intent.ExpiresAt,
			},
		}, nil
	}
}

func submitProof(svc *service.Service) httpwrap.HandlerFunc {
	return func(r *http.Request) (*httpwrap.Response, *httpwrap.ErrorResponse) {
		intentID := chi.URLParam(r, "intent_id")
		if intentID == "" {
			return nil, httpwrap.NewInvalidParamErrorResponse("intent_id")
		}

		var req SubmitProofRequest

		if errResp := httpwrap.BindBody(r, &req); errResp != nil {
			return nil, errResp
		}

		if req.SettleProof == "" {
			return nil, &httpwrap.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				ErrorMsg:   "settle_proof is required",
			}
		}

		ctx := r.Context()

		intent, err := svc.SubmitProof(ctx, intentID, req.SettleProof)
		if err != nil {
			if errors.Is(err, payment.ErrNotFound) {
				return nil, httpwrap.NewNotFoundErrorResponse("payment intent not found")
			}

			// User-facing errors - safe to expose
			if errors.Is(err, payment.ErrInvalidStatus) ||
				errors.Is(err, payment.ErrProofValidation) ||
				errors.Is(err, payment.ErrExpired) ||
				errors.Is(err, payment.ErrConcurrentUpdate) {
				return nil, &httpwrap.ErrorResponse{
					StatusCode: http.StatusBadRequest,
					ErrorMsg:   err.Error(),
					Err:        err,
				}
			}

			// Internal errors - sanitize to avoid leaking implementation details
			return nil, &httpwrap.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				ErrorMsg:   sanitizeInternalError(err, "submit_proof"),
				Err:        err,
			}
		}

		return &httpwrap.Response{
			StatusCode: http.StatusOK,
			Body: &SubmitProofResponse{
				IntentID:          intent.IntentID,
				MerchantRecipient: intent.MerchantRecipient,
				Status:            intent.Status,
				CreatedAt:         intent.CreatedAt,
				ExpiresAt:         intent.ExpiresAt,
			},
		}, nil
	}
}

func getIntent(svc *service.Service) httpwrap.HandlerFunc {
	return func(r *http.Request) (*httpwrap.Response, *httpwrap.ErrorResponse) {
		intentID := r.URL.Query().Get("intent_id")
		if intentID == "" {
			return nil, httpwrap.NewInvalidParamErrorResponse("intent_id")
		}

		ctx := r.Context()

		intent, err := svc.GetIntent(ctx, intentID)
		if err != nil {
			if errors.Is(err, payment.ErrNotFound) {
				return nil, httpwrap.NewNotFoundErrorResponse("payment intent not found")
			}

			// Internal errors - sanitize to avoid leaking implementation details
			return nil, &httpwrap.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				ErrorMsg:   sanitizeInternalError(err, "get_intent"),
				Err:        err,
			}
		}

		response := &IntentResponse{
			IntentID:          intent.IntentID,
			Status:            intent.Status,
			PayerChain:        intent.PayerChain,
			MerchantRecipient: intent.MerchantRecipient,
			ReceiverEmail:     intent.ReceiverEmail,
			PayerWallet:       intent.PayerWallet,
			ErrorMessage:      intent.ErrorMessage,
			CreatedAt:         intent.CreatedAt,
			ExpiresAt:         intent.ExpiresAt,
			CompletedAt:       intent.CompletedAt,
		}

		if intent.Amount != "" {
			response.Amount = &intent.Amount
		}

		sourceExplorerBase := svc.SourceChainExplorerURL(intent.PayerChain)
		baseExplorerBase := svc.BasePaymentExplorerBase()

		if intent.SolTxHash != nil && intent.SolSettleProof != nil && intent.SolSettledAt != nil {
			response.SourcePayment = &SourcePaymentResponse{
				Chain:       intent.PayerChain,
				TxHash:      *intent.SolTxHash,
				SettleProof: *intent.SolSettleProof,
				SettledAt:   *intent.SolSettledAt,
				ExplorerURL: fmt.Sprintf("%s/tx/%s", sourceExplorerBase, *intent.SolTxHash),
			}
		}

		if intent.BaseTxHash != nil && intent.BaseSettleProof != nil && intent.BaseSettledAt != nil {
			response.BasePayment = &BasePaymentResponse{
				TxHash:      *intent.BaseTxHash,
				SettleProof: *intent.BaseSettleProof,
				SettledAt:   *intent.BaseSettledAt,
				ExplorerURL: fmt.Sprintf("%s/tx/%s", baseExplorerBase, *intent.BaseTxHash),
			}
		}

		return &httpwrap.Response{
			StatusCode: http.StatusOK,
			Body:       response,
		}, nil
	}
}
