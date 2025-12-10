package services

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/agent-tech/x402-api-backend/internal/db"
	"github.com/agent-tech/x402-api-backend/internal/dto"
	"github.com/coinbase/x402/go/pkg/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	log "github.com/sirupsen/logrus"
)

// Payment status constants
const (
	StatusAwaitingPayment    = "AWAITING_PAYMENT"
	StatusPending            = "PENDING"
	StatusVerificationFailed = "VERIFICATION_FAILED"
	StatusSolSettled         = "SOL_SETTLED"
	StatusBaseSettling       = "BASE_SETTLING"
	StatusBaseSettled        = "BASE_SETTLED"
	StatusExpired            = "EXPIRED"
)

// PaymentIntentService handles payment intent business logic
type PaymentIntentService struct {
	queries            *db.Queries
	basePaymentService *BasePaymentService
	x402Verifier       *X402Verifier
	privyService       *PrivyService
	solanaNetwork      string
	baseNetwork        string
}

// NewPaymentIntentService creates a new payment intent service
func NewPaymentIntentService(
	queries *db.Queries,
	basePaymentService *BasePaymentService,
	x402Verifier *X402Verifier,
	privyService *PrivyService,
	solanaNetwork string,
	baseNetwork string,
) *PaymentIntentService {
	return &PaymentIntentService{
		queries:            queries,
		basePaymentService: basePaymentService,
		x402Verifier:       x402Verifier,
		privyService:       privyService,
		solanaNetwork:      solanaNetwork,
		baseNetwork:        baseNetwork,
	}
}

// CreateIntent creates a new payment intent with either email or wallet as receiver
func (s *PaymentIntentService) CreateIntent(req dto.CreateIntentRequest) (*dto.CreateIntentResponse, error) {
	// Validate: must have either email or recipient
	if req.Email == "" && req.Recipient == "" {
		return nil, fmt.Errorf("either email or recipient is required")
	}
	if req.Email != "" && req.Recipient != "" {
		return nil, fmt.Errorf("cannot provide both email and recipient")
	}

	intentID := uuid.New().String()
	expiresAt := time.Now().Add(10 * time.Minute) // 10 minutes expiry

	logger := log.WithFields(log.Fields{
		"intent_id": intentID,
	})

	var walletAddress string
	var email *string

	if req.Email != "" {
		logger = logger.WithField("email", req.Email)
		logger.Info("Creating payment intent with email receiver")

		// Get or create wallet for email via Privy
		wallet, err := s.privyService.GetOrCreateWalletForEmail(req.Email)
		if err != nil {
			logger.WithError(err).Error("Failed to get wallet for email")
			return nil, fmt.Errorf("failed to resolve email to wallet: %w", err)
		}
		walletAddress = wallet
		email = &req.Email
		logger.WithField("wallet", walletAddress).Info("Resolved email to wallet address")
	} else {
		logger = logger.WithField("recipient", req.Recipient)
		logger.Info("Creating payment intent with wallet receiver")
		walletAddress = req.Recipient
	}

	// Create payment intent
	ctx := context.Background()
	now := time.Now()

	params := db.CreatePaymentIntentParams{
		ID:                uuid.New().String(),
		IntentID:          intentID,
		PayerChain:        "solana",
		TargetChain:       "base",
		MerchantRecipient: walletAddress,
		Amount:            req.Amount,
		Status:            StatusAwaitingPayment,
		CreatedAt:         pgtype.Timestamp{Time: now, Valid: true},
		ExpiresAt:         pgtype.Timestamp{Time: expiresAt, Valid: true},
	}

	if email != nil {
		params.ReceiverEmail = pgtype.Text{String: *email, Valid: true}
	}

	intent, err := s.queries.CreatePaymentIntent(ctx, params)
	if err != nil {
		logger.WithError(err).Error("Failed to create payment intent")
		return nil, fmt.Errorf("failed to create payment intent: %w", err)
	}

	logger.Info("Payment intent created, awaiting payment proof")

	return &dto.CreateIntentResponse{
		IntentID:          intent.IntentID,
		Email:             email,
		MerchantRecipient: intent.MerchantRecipient,
		Amount:            intent.Amount,
		Status:            intent.Status,
		CreatedAt:         intent.CreatedAt.Time,
		ExpiresAt:         intent.ExpiresAt.Time,
	}, nil
}

// SubmitProof submits a payment proof for an existing intent
// It validates the proof matches the intent, then triggers async verification and Base payment
func (s *PaymentIntentService) SubmitProof(intentID string, req dto.SubmitProofRequest) (*dto.SubmitProofResponse, error) {
	logger := log.WithField("intent_id", intentID)
	logger.Info("Submitting proof for payment intent")

	ctx := context.Background()

	// Find the intent
	intent, err := s.queries.GetPaymentIntentByIntentID(ctx, intentID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("payment intent not found")
		}
		return nil, fmt.Errorf("failed to find payment intent: %w", err)
	}

	// Validate intent status
	if intent.Status != StatusAwaitingPayment {
		return nil, fmt.Errorf("cannot submit proof: intent status is %s, expected %s", intent.Status, StatusAwaitingPayment)
	}

	// Check if expired
	if time.Now().After(intent.ExpiresAt.Time) {
		s.queries.UpdatePaymentIntentExpired(ctx, intentID)
		return nil, fmt.Errorf("payment intent has expired")
	}

	// Decode and validate proof against intent
	if err := s.validateProofAgainstIntent(req.SettleProof, intent.Amount); err != nil {
		logger.WithError(err).Error("Proof validation failed")
		return nil, fmt.Errorf("proof validation failed: %w", err)
	}

	// Update intent with proof and change status to PENDING
	err = s.queries.UpdatePaymentIntentWithProof(ctx, db.UpdatePaymentIntentWithProofParams{
		IntentID:       intentID,
		SolSettleProof: pgtype.Text{String: req.SettleProof, Valid: true},
		Status:         StatusPending,
	})
	if err != nil {
		logger.WithError(err).Error("Failed to update intent with proof")
		return nil, fmt.Errorf("failed to update payment intent: %w", err)
	}

	logger.Info("Proof submitted, launching async processing")

	// Launch async processing with 5-minute timeout
	go func() {
		asyncCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		s.processIntentAsync(asyncCtx, intentID, req.SettleProof, intent.MerchantRecipient)
	}()

	return &dto.SubmitProofResponse{
		IntentID:          intent.IntentID,
		MerchantRecipient: intent.MerchantRecipient,
		Status:            StatusPending,
		CreatedAt:         intent.CreatedAt.Time,
		ExpiresAt:         intent.ExpiresAt.Time,
	}, nil
}

// validateProofAgainstIntent validates that the proof matches the intent's amount
func (s *PaymentIntentService) validateProofAgainstIntent(settleProof string, intentAmount string) error {
	// Decode the payment payload from base64-encoded proof
	payload, err := types.DecodePaymentPayloadFromBase64(settleProof)
	if err != nil {
		return fmt.Errorf("invalid proof encoding: %w", err)
	}

	// Validate amount
	if payload.Payload != nil && payload.Payload.Authorization != nil && payload.Payload.Authorization.Value != "" {
		// Value is a string representing the amount in the smallest unit (microdollars for USDC)
		valueStr := payload.Payload.Authorization.Value
		proofAmount, err := strconv.ParseUint(valueStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid proof amount format: %w", err)
		}

		// Convert proof amount from microdollars to dollars for comparison
		proofAmountDollars := fmt.Sprintf("%.2f", float64(proofAmount)/1e6)

		// Parse intent amount for comparison
		amount, err := strconv.ParseFloat(intentAmount, 64)
		if err != nil {
			return fmt.Errorf("invalid intent amount format: %w", err)
		}
		intentAmountStr := fmt.Sprintf("%.2f", amount)

		if proofAmountDollars != intentAmountStr {
			return fmt.Errorf("amount mismatch: proof has %s, intent requires %s", proofAmountDollars, intentAmountStr)
		}
	} else {
		return fmt.Errorf("proof does not contain amount information")
	}

	return nil
}

// processIntentAsync handles the async verification and Base payment flow
func (s *PaymentIntentService) processIntentAsync(ctx context.Context, intentID string, settleProof string, merchantRecipient string) {
	logger := log.WithField("intent_id", intentID)
	logger.Info("Starting async intent processing")

	// Check context before verification
	select {
	case <-ctx.Done():
		logger.Error("Context cancelled before verification")
		s.updateIntentError(intentID, "Processing timeout before verification")
		return
	default:
	}

	// Step 1: Verify X402 proof and extract details (verifies on-chain transaction)
	logger.Info("Verifying X402 proof with facilitator")
	details, err := s.x402Verifier.VerifyAndExtractDetails(settleProof, merchantRecipient)
	if err != nil {
		logger.WithError(err).Error("Proof verification failed")
		s.updateIntentError(intentID, fmt.Sprintf("Verification failed: %s", err.Error()))
		return
	}

	// Step 2: Update to SOL_SETTLED with extracted data
	logger.Info("Proof verified, updating to SOL_SETTLED")
	if err := s.updateIntentSolSettled(intentID, details); err != nil {
		logger.WithError(err).Error("Failed to update intent to SOL_SETTLED")
		return
	}

	// Check context before Base payment
	select {
	case <-ctx.Done():
		logger.Error("Context cancelled before Base payment")
		s.updateIntentError(intentID, "Processing timeout before Base payment")
		return
	default:
	}

	// Step 3: Trigger Base payment
	logger.Info("Triggering Base payment")
	if err := s.triggerBasePaymentInternal(ctx, intentID); err != nil {
		logger.WithError(err).Error("Base payment failed")
	}
}

// updateIntentError updates the intent with error status
func (s *PaymentIntentService) updateIntentError(intentID string, errorMsg string) {
	ctx := context.Background()
	err := s.queries.UpdatePaymentIntentStatus(ctx, db.UpdatePaymentIntentStatusParams{
		IntentID:     intentID,
		Status:       StatusVerificationFailed,
		ErrorMessage: pgtype.Text{String: errorMsg, Valid: true},
	})
	if err != nil {
		log.WithError(err).Error("Failed to update intent with error")
	}
}

// updateIntentSolSettled updates the intent with Solana settlement data
func (s *PaymentIntentService) updateIntentSolSettled(intentID string, details *ProofDetails) error {
	ctx := context.Background()
	now := time.Now()

	params := db.UpdatePaymentIntentSolSettledParams{
		IntentID:     intentID,
		Status:       StatusSolSettled,
		Amount:       details.Amount,
		SolSettledAt: pgtype.Timestamp{Time: now, Valid: true},
	}

	if details.PayerWallet != nil {
		params.PayerWallet = pgtype.Text{String: *details.PayerWallet, Valid: true}
	}
	if details.TxHash != nil {
		params.SolTxHash = pgtype.Text{String: *details.TxHash, Valid: true}
	}

	return s.queries.UpdatePaymentIntentSolSettled(ctx, params)
}

// triggerBasePaymentInternal triggers Base payment (internal method with context)
func (s *PaymentIntentService) triggerBasePaymentInternal(ctx context.Context, intentID string) error {
	logger := log.WithField("intent_id", intentID)
	logger.Info("Executing Base payment")

	// Find the intent
	intent, err := s.queries.GetPaymentIntentByIntentID(ctx, intentID)
	if err != nil {
		return fmt.Errorf("failed to find intent: %w", err)
	}

	// Validate status - can trigger Base payment only if SOL_SETTLED
	if intent.Status != StatusSolSettled {
		return fmt.Errorf("cannot trigger Base payment: intent status is %s", intent.Status)
	}

	// Set status to BASE_SETTLING
	err = s.queries.UpdatePaymentIntentStatus(ctx, db.UpdatePaymentIntentStatusParams{
		IntentID: intentID,
		Status:   StatusBaseSettling,
	})
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Parse amount
	amount, err := strconv.ParseFloat(intent.Amount, 64)
	if err != nil {
		// Rollback
		s.queries.UpdatePaymentIntentStatus(ctx, db.UpdatePaymentIntentStatusParams{
			IntentID: intentID,
			Status:   StatusSolSettled,
		})
		return fmt.Errorf("invalid amount: %w", err)
	}

	// Execute Base payment
	result, err := s.basePaymentService.ExecutePayment(ctx, PaymentRequest{
		IntentID:         intent.IntentID,
		Amount:           amount,
		RecipientAddress: intent.MerchantRecipient,
	})

	if err != nil {
		logger.WithError(err).Error("Base payment failed, rolling back to SOL_SETTLED")

		// Rollback to SOL_SETTLED to allow retry
		s.queries.UpdatePaymentIntentStatus(ctx, db.UpdatePaymentIntentStatusParams{
			IntentID: intentID,
			Status:   StatusSolSettled,
		})

		return fmt.Errorf("Base payment failed: %w", err)
	}

	// Update intent with Base payment details
	now := time.Now()
	err = s.queries.UpdatePaymentIntentBaseSettled(ctx, db.UpdatePaymentIntentBaseSettledParams{
		IntentID:        intentID,
		Status:          StatusBaseSettled,
		BaseTxHash:      pgtype.Text{String: result.TxHash, Valid: true},
		BaseSettleProof: pgtype.Text{String: result.Proof, Valid: true},
		BaseSettledAt:   pgtype.Timestamp{Time: now, Valid: true},
		CompletedAt:     pgtype.Timestamp{Time: now, Valid: true},
	})
	if err != nil {
		logger.WithError(err).Error("Failed to update payment intent")
		return fmt.Errorf("failed to update payment intent: %w", err)
	}

	logger.Info("Base payment completed successfully")
	return nil
}

// GetIntent retrieves a payment intent by ID with combined status + receipt data
func (s *PaymentIntentService) GetIntent(intentID string) (*dto.GetIntentResponse, error) {
	log.WithField("intent_id", intentID).Info("Querying payment intent")

	ctx := context.Background()

	intent, err := s.queries.GetPaymentIntentByIntentID(ctx, intentID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query payment intent: %w", err)
	}

	// Check if expired and update status (for both AWAITING_PAYMENT and PENDING)
	if (intent.Status == StatusAwaitingPayment || intent.Status == StatusPending) && time.Now().After(intent.ExpiresAt.Time) {
		s.queries.UpdatePaymentIntentExpired(ctx, intentID)
		intent.Status = StatusExpired
	}

	// Determine explorer URLs
	solanaExplorerBase := "https://solscan.io?cluster=devnet"
	if s.solanaNetwork == "solana-mainnet-beta" {
		solanaExplorerBase = "https://solscan.io"
	}

	baseExplorerBase := "https://sepolia.basescan.org"
	if s.baseNetwork == "base" {
		baseExplorerBase = "https://basescan.org"
	}

	// Build combined response
	response := &dto.GetIntentResponse{
		IntentID:          intent.IntentID,
		Status:            intent.Status,
		MerchantRecipient: intent.MerchantRecipient,
		CreatedAt:         intent.CreatedAt.Time,
		ExpiresAt:         intent.ExpiresAt.Time,
	}

	// Add optional fields
	if intent.Amount != "" {
		response.Amount = &intent.Amount
	}
	if intent.ReceiverEmail.Valid {
		response.ReceiverEmail = &intent.ReceiverEmail.String
	}
	if intent.PayerWallet.Valid {
		response.PayerWallet = &intent.PayerWallet.String
	}
	if intent.ErrorMessage.Valid {
		response.ErrorMessage = &intent.ErrorMessage.String
	}
	if intent.CompletedAt.Valid {
		response.CompletedAt = &intent.CompletedAt.Time
	}

	// Add Solana payment details if available
	if intent.SolTxHash.Valid && intent.SolSettleProof.Valid && intent.SolSettledAt.Valid {
		response.SolanaPayment = &dto.SolanaPayment{
			TxHash:      intent.SolTxHash.String,
			SettleProof: intent.SolSettleProof.String,
			SettledAt:   intent.SolSettledAt.Time,
			ExplorerURL: fmt.Sprintf("%s/tx/%s", solanaExplorerBase, intent.SolTxHash.String),
		}
	}

	// Add Base payment details if available
	if intent.BaseTxHash.Valid && intent.BaseSettleProof.Valid && intent.BaseSettledAt.Valid {
		response.BasePayment = &dto.BasePayment{
			TxHash:      intent.BaseTxHash.String,
			SettleProof: intent.BaseSettleProof.String,
			SettledAt:   intent.BaseSettledAt.Time,
			ExplorerURL: fmt.Sprintf("%s/tx/%s", baseExplorerBase, intent.BaseTxHash.String),
		}
	}

	return response, nil
}
