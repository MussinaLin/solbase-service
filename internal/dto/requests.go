package dto

import "time"

// CreateIntentRequest represents the request to create a payment intent (proof-first approach)
type CreateIntentRequest struct {
	SettleProof       string `json:"settle_proof" binding:"required"`
	MerchantRecipient string `json:"merchant_recipient" binding:"required,eth_addr"`
}

// QueryIntentRequest represents the query parameter for getting intent
type QueryIntentRequest struct {
	IntentID string `form:"intent_id" binding:"required,uuid4"`
}

// CreateIntentResponse represents the response after creating an intent
type CreateIntentResponse struct {
	IntentID          string    `json:"intent_id"`
	MerchantRecipient string    `json:"merchant_recipient"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// SolanaPayment represents Solana payment details
type SolanaPayment struct {
	TxHash      string    `json:"tx_hash"`
	SettleProof string    `json:"settle_proof"`
	SettledAt   time.Time `json:"settled_at"`
	ExplorerURL string    `json:"explorer_url"`
}

// BasePayment represents Base payment details
type BasePayment struct {
	TxHash      string    `json:"tx_hash"`
	SettleProof string    `json:"settle_proof"`
	SettledAt   time.Time `json:"settled_at"`
	ExplorerURL string    `json:"explorer_url"`
}

// GetIntentResponse represents the combined status + receipt response
type GetIntentResponse struct {
	IntentID          string         `json:"intent_id"`
	Status            string         `json:"status"`
	Amount            *string        `json:"amount,omitempty"`
	MerchantRecipient string         `json:"merchant_recipient"`
	PayerWallet       *string        `json:"payer_wallet,omitempty"`
	ErrorMessage      *string        `json:"error_message,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	ExpiresAt         time.Time      `json:"expires_at"`
	CompletedAt       *time.Time     `json:"completed_at,omitempty"`
	SolanaPayment     *SolanaPayment `json:"solana_payment,omitempty"`
	BasePayment       *BasePayment   `json:"base_payment,omitempty"`
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status string                 `json:"status"`
	Info   map[string]interface{} `json:"info"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error      string `json:"error"`
	Message    string `json:"message"`
	StatusCode int    `json:"statusCode"`
}
