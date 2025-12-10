package repository

import (
	"context"

	"github.com/agent-tech/x402-api-backend/internal/payment"
)

//go:generate mockgen -source=repository.go -destination=../mocks/repository.go -package=mocks

// Repository defines the payment intent repository interface.
type Repository interface {
	Create(ctx context.Context, intent *payment.Intent) (*payment.Intent, error)
	GetByIntentID(ctx context.Context, intentID string) (*payment.Intent, error)
	UpdateStatus(ctx context.Context, intentID, status string, errorMsg *string) error
	UpdateWithProof(ctx context.Context, intentID, proof, status string) error
	UpdateSourceSettled(ctx context.Context, intentID string, details *payment.ProofDetails) error
	UpdateBaseSettled(ctx context.Context, intentID, txHash, proof string) error
	UpdateExpired(ctx context.Context, intentID string) error
}

// EmailWalletRepository defines the email-wallet mapping repository.
type EmailWalletRepository interface {
	GetByEmail(ctx context.Context, email string) (*EmailWallet, error)
	Create(ctx context.Context, email, wallet, privyUserID string) error
}

// EmailWallet represents an email-to-wallet mapping.
type EmailWallet struct {
	Email         string
	WalletAddress string
	PrivyUserID   string
}
