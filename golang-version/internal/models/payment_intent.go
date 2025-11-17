package models

import (
	"time"

	"gorm.io/gorm"
)

// PaymentIntent represents a cross-chain payment intent
// Mirrors the Prisma schema from the TypeScript version
type PaymentIntent struct {
	ID                 string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	IntentID           string         `gorm:"type:varchar(36);uniqueIndex;not null" json:"intent_id"`
	PayerChain         string         `gorm:"type:varchar(50);not null" json:"payer_chain"`
	TargetChain        string         `gorm:"type:varchar(50);not null" json:"target_chain"`
	PayerWallet        *string        `gorm:"type:varchar(100)" json:"payer_wallet,omitempty"`
	MerchantRecipient  string         `gorm:"type:varchar(42);not null" json:"merchant_recipient"`
	Amount             string         `gorm:"type:varchar(50);not null" json:"amount"`

	// Solana proofs
	SolCommitReceipt   *string        `gorm:"type:text" json:"sol_commit_receipt,omitempty"`
	SolSettleProof     *string        `gorm:"type:text" json:"sol_settle_proof,omitempty"`
	SolTxHash          *string        `gorm:"type:varchar(100)" json:"sol_tx_hash,omitempty"`

	// Base proofs
	BaseCommitReceipt  *string        `gorm:"type:text" json:"base_commit_receipt,omitempty"`
	BaseSettleProof    *string        `gorm:"type:text" json:"base_settle_proof,omitempty"`
	BaseTxHash         *string        `gorm:"type:varchar(66)" json:"base_tx_hash,omitempty"`

	// Status tracking
	Status             string         `gorm:"type:varchar(20);not null;index" json:"status"`

	// Timestamps
	CreatedAt          time.Time      `gorm:"not null" json:"created_at"`
	ExpiresAt          time.Time      `gorm:"not null;index" json:"expires_at"`
	SolSettledAt       *time.Time     `json:"sol_settled_at,omitempty"`
	BaseSettledAt      *time.Time     `json:"base_settled_at,omitempty"`
	CompletedAt        *time.Time     `json:"completed_at,omitempty"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName specifies the table name for GORM
func (PaymentIntent) TableName() string {
	return "payment_intents"
}

// Payment status constants
const (
	StatusPending      = "PENDING"
	StatusSolSettled   = "SOL_SETTLED"
	StatusBaseSettling = "BASE_SETTLING"
	StatusBaseSettled  = "BASE_SETTLED"
	StatusCompleted    = "COMPLETED"
	StatusExpired      = "EXPIRED"
)

// IsExpired checks if the payment intent has expired
func (p *PaymentIntent) IsExpired() bool {
	return time.Now().After(p.ExpiresAt)
}

// CanSubmitSolanaProof checks if Solana proof can be submitted
func (p *PaymentIntent) CanSubmitSolanaProof() bool {
	return p.Status == StatusPending && !p.IsExpired()
}

// CanTriggerBasePayment checks if Base payment can be triggered
func (p *PaymentIntent) CanTriggerBasePayment() bool {
	return p.Status == StatusSolSettled
}
