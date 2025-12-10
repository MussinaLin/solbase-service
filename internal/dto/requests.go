package dto

import "time"

// CreateIntentRequest represents the request to create a payment intent
// Either email or recipient must be provided, but not both
type CreateIntentRequest struct {
	Email     string `json:"email"`                        // Email address (resolves to wallet via Privy)
	Recipient string `json:"recipient"`                    // Direct wallet address
	Amount    string `json:"amount" binding:"required"`
}

// CreateIntentResponse represents the response after creating an intent
type CreateIntentResponse struct {
	IntentID          string    `json:"intent_id"`
	Email             *string   `json:"email,omitempty"` // Only present if email was provided
	MerchantRecipient string    `json:"merchant_recipient"`
	Amount            string    `json:"amount"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// SubmitProofRequest represents the request to submit a proof for an existing intent
type SubmitProofRequest struct {
	SettleProof string `json:"settle_proof" binding:"required"`
}

// SubmitProofResponse represents the response after submitting a proof
type SubmitProofResponse struct {
	IntentID          string    `json:"intent_id"`
	MerchantRecipient string    `json:"merchant_recipient"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// QueryIntentRequest represents the query parameter for getting intent
type QueryIntentRequest struct {
	IntentID string `form:"intent_id" binding:"required,uuid4"`
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
	ReceiverEmail     *string        `json:"receiver_email,omitempty"`
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
