package payment

import (
	"context"
	"time"
)

// Status constants for payment intents.
const (
	StatusAwaitingPayment    = "AWAITING_PAYMENT"
	StatusPending            = "PENDING"
	StatusVerificationFailed = "VERIFICATION_FAILED"
	StatusSourceSettled      = "SOURCE_SETTLED"
	StatusBaseSettling       = "BASE_SETTLING"
	StatusBaseSettled        = "BASE_SETTLED"
	StatusExpired            = "EXPIRED"
)

// ValidPayerChains defines the valid payer chains.
var ValidPayerChains = map[string]bool{
	"solana": true,
	"base":   true,
	"bsc":    true,
}

// Intent represents a payment intent domain model.
type Intent struct {
	ID                string
	IntentID          string
	PayerChain        string
	TargetChain       string
	MerchantRecipient string
	SourceRecipient   *string // Solana/BSC receiver address (nil for Base)
	ReceiverEmail     *string
	Amount            string
	Status            string
	PayerWallet       *string
	SolTxHash         *string
	SolSettleProof    *string
	SolSettledAt      *time.Time
	BaseTxHash        *string
	BaseSettleProof   *string
	BaseSettledAt     *time.Time
	ErrorMessage      *string
	CreatedAt         time.Time
	ExpiresAt         time.Time
	CompletedAt       *time.Time
}

// Service defines the payment intent service interface.
type Service interface {
	CreateIntent(ctx context.Context, params *CreateIntentParams) (*Intent, error)
	SubmitProof(ctx context.Context, intentID string, proof string) (*Intent, error)
	GetIntent(ctx context.Context, intentID string) (*Intent, error)
}

// CreateIntentParams represents parameters for creating an intent.
type CreateIntentParams struct {
	Email      string
	Recipient  string
	Amount     string
	PayerChain string
}

// ProofDetails contains extracted information from a verified X402 proof.
type ProofDetails struct {
	Amount      string
	PayerWallet *string
	TxHash      *string
}

// PaymentRequest represents a Base payment request.
type PaymentRequest struct {
	IntentID         string
	Amount           float64
	RecipientAddress string
}

// PaymentResult represents the result of a Base payment.
type PaymentResult struct {
	TxHash string
	Proof  string
}

// SourcePayment represents payment details on the source/payer chain.
type SourcePayment struct {
	Chain       string
	TxHash      string
	SettleProof string
	SettledAt   time.Time
	ExplorerURL string
}

// BasePayment represents Base payment details.
type BasePayment struct {
	TxHash      string
	SettleProof string
	SettledAt   time.Time
	ExplorerURL string
}

// IntentWithPayments represents an intent with associated payment details.
type IntentWithPayments struct {
	Intent        *Intent
	SourcePayment *SourcePayment
	BasePayment   *BasePayment
}
