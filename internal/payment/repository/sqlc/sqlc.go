package sqlc

import (
	"context"
	"fmt"
	"time"

	"github.com/agent-tech/x402-api-backend/internal/db"
	"github.com/agent-tech/x402-api-backend/internal/payment"
	"github.com/agent-tech/x402-api-backend/internal/payment/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Repository implements the payment repository interface using sqlc.
type Repository struct {
	queries *db.Queries
}

// Ensure Repository implements repository.Repository.
var _ repository.Repository = (*Repository)(nil)

// New creates a new sqlc repository.
func New(queries *db.Queries) *Repository {
	return &Repository{
		queries: queries,
	}
}

// Create creates a new payment intent.
func (r *Repository) Create(ctx context.Context, intent *payment.Intent) (*payment.Intent, error) {
	params := db.CreatePaymentIntentParams{
		ID:                uuid.New().String(),
		IntentID:          intent.IntentID,
		PayerChain:        intent.PayerChain,
		TargetChain:       intent.TargetChain,
		MerchantRecipient: intent.MerchantRecipient,
		Amount:            intent.Amount,
		Status:            intent.Status,
		CreatedAt:         pgtype.Timestamp{Time: intent.CreatedAt, Valid: true},
		ExpiresAt:         pgtype.Timestamp{Time: intent.ExpiresAt, Valid: true},
	}

	if intent.ReceiverEmail != nil {
		params.ReceiverEmail = pgtype.Text{String: *intent.ReceiverEmail, Valid: true}
	}

	dbIntent, err := r.queries.CreatePaymentIntent(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create payment intent: %w", err)
	}

	return r.toIntent(dbIntent), nil
}

// GetByIntentID retrieves a payment intent by its intent ID.
func (r *Repository) GetByIntentID(ctx context.Context, intentID string) (*payment.Intent, error) {
	dbIntent, err := r.queries.GetPaymentIntentByIntentID(ctx, intentID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, payment.ErrNotFound
		}

		return nil, fmt.Errorf("get payment intent: %w", err)
	}

	return r.toIntent(dbIntent), nil
}

// UpdateStatus updates the status of a payment intent.
func (r *Repository) UpdateStatus(ctx context.Context, intentID, status string, errorMsg *string) error {
	params := db.UpdatePaymentIntentStatusParams{
		IntentID: intentID,
		Status:   status,
	}

	if errorMsg != nil {
		params.ErrorMessage = pgtype.Text{String: *errorMsg, Valid: true}
	}

	err := r.queries.UpdatePaymentIntentStatus(ctx, params)
	if err != nil {
		return fmt.Errorf("update payment intent status: %w", err)
	}

	return nil
}

// UpdateWithProof updates the intent with proof and status.
func (r *Repository) UpdateWithProof(ctx context.Context, intentID, proof, status string) error {
	err := r.queries.UpdatePaymentIntentWithProof(ctx, db.UpdatePaymentIntentWithProofParams{
		IntentID:       intentID,
		SolSettleProof: pgtype.Text{String: proof, Valid: true},
		Status:         status,
	})
	if err != nil {
		return fmt.Errorf("update payment intent with proof: %w", err)
	}

	return nil
}

// UpdateSourceSettled updates the intent with source chain settlement data.
func (r *Repository) UpdateSourceSettled(ctx context.Context, intentID string, details *payment.ProofDetails) error {
	now := time.Now()

	params := db.UpdatePaymentIntentSolSettledParams{
		IntentID:     intentID,
		Status:       payment.StatusSourceSettled,
		Amount:       details.Amount,
		SolSettledAt: pgtype.Timestamp{Time: now, Valid: true},
	}

	if details.PayerWallet != nil {
		params.PayerWallet = pgtype.Text{String: *details.PayerWallet, Valid: true}
	}

	if details.TxHash != nil {
		params.SolTxHash = pgtype.Text{String: *details.TxHash, Valid: true}
	}

	err := r.queries.UpdatePaymentIntentSolSettled(ctx, params)
	if err != nil {
		return fmt.Errorf("update payment intent source settled: %w", err)
	}

	return nil
}

// UpdateBaseSettled updates the intent with Base settlement data.
func (r *Repository) UpdateBaseSettled(ctx context.Context, intentID, txHash, proof string) error {
	now := time.Now()

	err := r.queries.UpdatePaymentIntentBaseSettled(ctx, db.UpdatePaymentIntentBaseSettledParams{
		IntentID:        intentID,
		Status:          payment.StatusBaseSettled,
		BaseTxHash:      pgtype.Text{String: txHash, Valid: true},
		BaseSettleProof: pgtype.Text{String: proof, Valid: true},
		BaseSettledAt:   pgtype.Timestamp{Time: now, Valid: true},
		CompletedAt:     pgtype.Timestamp{Time: now, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("update payment intent base settled: %w", err)
	}

	return nil
}

// UpdateExpired marks the intent as expired.
func (r *Repository) UpdateExpired(ctx context.Context, intentID string) error {
	err := r.queries.UpdatePaymentIntentExpired(ctx, intentID)
	if err != nil {
		return fmt.Errorf("update payment intent expired: %w", err)
	}

	return nil
}

// toIntent converts a db.PaymentIntent to payment.Intent.
func (r *Repository) toIntent(dbIntent db.PaymentIntent) *payment.Intent {
	intent := &payment.Intent{
		ID:                dbIntent.ID,
		IntentID:          dbIntent.IntentID,
		PayerChain:        dbIntent.PayerChain,
		TargetChain:       dbIntent.TargetChain,
		MerchantRecipient: dbIntent.MerchantRecipient,
		Amount:            dbIntent.Amount,
		Status:            dbIntent.Status,
		CreatedAt:         dbIntent.CreatedAt.Time,
		ExpiresAt:         dbIntent.ExpiresAt.Time,
	}

	if dbIntent.ReceiverEmail.Valid {
		intent.ReceiverEmail = &dbIntent.ReceiverEmail.String
	}

	if dbIntent.PayerWallet.Valid {
		intent.PayerWallet = &dbIntent.PayerWallet.String
	}

	if dbIntent.SolTxHash.Valid {
		intent.SolTxHash = &dbIntent.SolTxHash.String
	}

	if dbIntent.SolSettleProof.Valid {
		intent.SolSettleProof = &dbIntent.SolSettleProof.String
	}

	if dbIntent.SolSettledAt.Valid {
		intent.SolSettledAt = &dbIntent.SolSettledAt.Time
	}

	if dbIntent.BaseTxHash.Valid {
		intent.BaseTxHash = &dbIntent.BaseTxHash.String
	}

	if dbIntent.BaseSettleProof.Valid {
		intent.BaseSettleProof = &dbIntent.BaseSettleProof.String
	}

	if dbIntent.BaseSettledAt.Valid {
		intent.BaseSettledAt = &dbIntent.BaseSettledAt.Time
	}

	if dbIntent.ErrorMessage.Valid {
		intent.ErrorMessage = &dbIntent.ErrorMessage.String
	}

	if dbIntent.CompletedAt.Valid {
		intent.CompletedAt = &dbIntent.CompletedAt.Time
	}

	return intent
}

// EmailWalletRepository implements the email wallet repository using sqlc.
type EmailWalletRepository struct {
	queries *db.Queries
}

// Ensure EmailWalletRepository implements repository.EmailWalletRepository.
var _ repository.EmailWalletRepository = (*EmailWalletRepository)(nil)

// NewEmailWalletRepository creates a new email wallet repository.
func NewEmailWalletRepository(queries *db.Queries) *EmailWalletRepository {
	return &EmailWalletRepository{
		queries: queries,
	}
}

// GetByEmail retrieves an email wallet mapping by email.
func (r *EmailWalletRepository) GetByEmail(ctx context.Context, email string) (*repository.EmailWallet, error) {
	dbWallet, err := r.queries.GetEmailWalletByEmail(ctx, email)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, payment.ErrNotFound
		}

		return nil, fmt.Errorf("get email wallet: %w", err)
	}

	return &repository.EmailWallet{
		Email:         dbWallet.Email,
		WalletAddress: dbWallet.WalletAddress,
		PrivyUserID:   dbWallet.PrivyUserID,
	}, nil
}

// Create creates a new email wallet mapping.
func (r *EmailWalletRepository) Create(ctx context.Context, email, wallet, privyUserID string) error {
	now := time.Now()

	_, err := r.queries.CreateEmailWallet(ctx, db.CreateEmailWalletParams{
		ID:            uuid.New().String(),
		Email:         email,
		WalletAddress: wallet,
		PrivyUserID:   privyUserID,
		CreatedAt:     pgtype.Timestamp{Time: now, Valid: true},
		UpdatedAt:     pgtype.Timestamp{Time: now, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("create email wallet: %w", err)
	}

	return nil
}
