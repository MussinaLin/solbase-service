package dto

import "time"

// CreateIntentRequest represents the request to create a payment intent
type CreateIntentRequest struct {
	Amount            float64 `json:"amount" binding:"required,min=0.01"`
	MerchantRecipient string  `json:"merchant_recipient" binding:"required,eth_addr"`
}

// QueryIntentRequest represents the query parameter for getting intent
type QueryIntentRequest struct {
	IntentID string `form:"intent_id" binding:"required,uuid4"`
}

// SolanaProofRequest represents the request to submit Solana payment proof
type SolanaProofRequest struct {
	SettleProof  string  `json:"settle_proof" binding:"required"`
	TxHash       string  `json:"tx_hash" binding:"required"`
	PayerWallet  *string `json:"payer_wallet,omitempty"`
}

// TriggerBasePaymentRequest is empty but kept for consistency
type TriggerBasePaymentRequest struct{}

// CreateIntentResponse represents the response after creating an intent
type CreateIntentResponse struct {
	IntentID          string    `json:"intent_id"`
	Amount            string    `json:"amount"`
	MerchantRecipient string    `json:"merchant_recipient"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

// GetIntentResponse represents the response when querying an intent
type GetIntentResponse struct {
	IntentID          string     `json:"intent_id"`
	Amount            string     `json:"amount"`
	MerchantRecipient string     `json:"merchant_recipient"`
	PayerWallet       *string    `json:"payer_wallet,omitempty"`
	Status            string     `json:"status"`
	SolTxHash         *string    `json:"sol_tx_hash,omitempty"`
	BaseTxHash        *string    `json:"base_tx_hash,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	SolSettledAt      *time.Time `json:"sol_settled_at,omitempty"`
	BaseSettledAt     *time.Time `json:"base_settled_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	ExpiresAt         time.Time  `json:"expires_at"`
}

// SubmitSolanaProofResponse represents the response after submitting Solana proof
type SubmitSolanaProofResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	IntentID string `json:"intent_id"`
	Status   string `json:"status"`
}

// TriggerBasePaymentResponse represents the response after triggering Base payment
type TriggerBasePaymentResponse struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	BaseTxHash string `json:"base_tx_hash"`
	Status     string `json:"status"`
}

// SolanaPayment represents Solana payment details in receipt
type SolanaPayment struct {
	TxHash      string    `json:"tx_hash"`
	SettleProof string    `json:"settle_proof"`
	SettledAt   time.Time `json:"settled_at"`
	ExplorerURL string    `json:"explorer_url"`
}

// BasePayment represents Base payment details in receipt
type BasePayment struct {
	TxHash      string    `json:"tx_hash"`
	SettleProof string    `json:"settle_proof"`
	SettledAt   time.Time `json:"settled_at"`
	ExplorerURL string    `json:"explorer_url"`
}

// ReceiptResponse represents the full payment receipt
type ReceiptResponse struct {
	IntentID          string         `json:"intent_id"`
	Amount            string         `json:"amount"`
	MerchantRecipient string         `json:"merchant_recipient"`
	PayerWallet       *string        `json:"payer_wallet,omitempty"`
	Status            string         `json:"status"`
	SolanaPayment     *SolanaPayment `json:"solana_payment,omitempty"`
	BasePayment       *BasePayment   `json:"base_payment,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	CompletedAt       *time.Time     `json:"completed_at,omitempty"`
	ExpiresAt         time.Time      `json:"expires_at"`
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
