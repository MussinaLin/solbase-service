package services

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/agent-tech/x402-api-backend/internal/dto"
	"github.com/agent-tech/x402-api-backend/internal/models"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// PaymentIntentService handles payment intent business logic
type PaymentIntentService struct {
	db                 *gorm.DB
	basePaymentService *BasePaymentService
	x402Verifier       *X402Verifier
	solanaNetwork      string
	baseNetwork        string
}

// NewPaymentIntentService creates a new payment intent service
func NewPaymentIntentService(
	db *gorm.DB,
	basePaymentService *BasePaymentService,
	x402Verifier *X402Verifier,
	solanaNetwork string,
	baseNetwork string,
) *PaymentIntentService {
	return &PaymentIntentService{
		db:                 db,
		basePaymentService: basePaymentService,
		x402Verifier:       x402Verifier,
		solanaNetwork:      solanaNetwork,
		baseNetwork:        baseNetwork,
	}
}

// CreateIntent creates a new payment intent with proof-first approach
// It stores the settle_proof, returns immediately, and processes verification + Base payment asynchronously
func (s *PaymentIntentService) CreateIntent(req dto.CreateIntentRequest) (*dto.CreateIntentResponse, error) {
	intentID := uuid.New().String()
	expiresAt := time.Now().Add(10 * time.Minute) // 10 minutes expiry

	log.WithField("intent_id", intentID).Info("Creating payment intent with proof-first approach")

	intent := &models.PaymentIntent{
		ID:                uuid.New().String(),
		IntentID:          intentID,
		MerchantRecipient: req.MerchantRecipient,
		SolSettleProof:    &req.SettleProof,
		PayerChain:        "solana",
		TargetChain:       "base",
		Status:            models.StatusPending,
		CreatedAt:         time.Now(),
		ExpiresAt:         expiresAt,
	}

	if err := s.db.Create(intent).Error; err != nil {
		log.WithError(err).Error("Failed to create payment intent")
		return nil, fmt.Errorf("failed to create payment intent: %w", err)
	}

	log.WithField("intent_id", intentID).Info("Payment intent created, launching async processing")

	// Launch async processing with 5-minute timeout
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		s.processIntentAsync(ctx, intentID, req.SettleProof, req.MerchantRecipient)
	}()

	return &dto.CreateIntentResponse{
		IntentID:          intent.IntentID,
		MerchantRecipient: intent.MerchantRecipient,
		Status:            intent.Status,
		CreatedAt:         intent.CreatedAt,
		ExpiresAt:         intent.ExpiresAt,
	}, nil
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

	// Step 1: Verify X402 proof and extract details
	logger.Info("Verifying X402 proof")
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
		// Note: triggerBasePaymentInternal already handles rollback to SOL_SETTLED
	}
}

// updateIntentError updates the intent with error status
func (s *PaymentIntentService) updateIntentError(intentID string, errorMsg string) {
	var intent models.PaymentIntent
	if err := s.db.Where("intent_id = ?", intentID).First(&intent).Error; err != nil {
		log.WithError(err).Error("Failed to find intent for error update")
		return
	}

	intent.Status = models.StatusVerificationFailed
	intent.ErrorMessage = &errorMsg

	if err := s.db.Save(&intent).Error; err != nil {
		log.WithError(err).Error("Failed to update intent with error")
	}
}

// updateIntentSolSettled updates the intent with Solana settlement data
func (s *PaymentIntentService) updateIntentSolSettled(intentID string, details *ProofDetails) error {
	var intent models.PaymentIntent
	if err := s.db.Where("intent_id = ?", intentID).First(&intent).Error; err != nil {
		return fmt.Errorf("failed to find intent: %w", err)
	}

	now := time.Now()
	intent.Status = models.StatusSolSettled
	intent.Amount = details.Amount
	intent.PayerWallet = details.PayerWallet
	intent.SolTxHash = details.TxHash
	intent.SolSettledAt = &now

	if err := s.db.Save(&intent).Error; err != nil {
		return fmt.Errorf("failed to update intent: %w", err)
	}

	return nil
}

// triggerBasePaymentInternal triggers Base payment (internal method with context)
func (s *PaymentIntentService) triggerBasePaymentInternal(ctx context.Context, intentID string) error {
	logger := log.WithField("intent_id", intentID)
	logger.Info("Executing Base payment")

	// Find the intent
	var intent models.PaymentIntent
	if err := s.db.Where("intent_id = ?", intentID).First(&intent).Error; err != nil {
		return fmt.Errorf("failed to find intent: %w", err)
	}

	// Validate status
	if !intent.CanTriggerBasePayment() {
		return fmt.Errorf("cannot trigger Base payment: intent status is %s", intent.Status)
	}

	// Set status to BASE_SETTLING
	intent.Status = models.StatusBaseSettling
	if err := s.db.Save(&intent).Error; err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Parse amount
	amount, err := strconv.ParseFloat(intent.Amount, 64)
	if err != nil {
		// Rollback
		intent.Status = models.StatusSolSettled
		s.db.Save(&intent)
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
		intent.Status = models.StatusSolSettled
		if updateErr := s.db.Save(&intent).Error; updateErr != nil {
			logger.WithError(updateErr).Error("Failed to rollback status")
		}

		return fmt.Errorf("Base payment failed: %w", err)
	}

	// Update intent with Base payment details
	now := time.Now()
	intent.Status = models.StatusBaseSettled
	intent.BaseTxHash = &result.TxHash
	intent.BaseSettleProof = &result.Proof
	intent.BaseSettledAt = &now
	intent.CompletedAt = &now

	if err := s.db.Save(&intent).Error; err != nil {
		logger.WithError(err).Error("Failed to update payment intent")
		return fmt.Errorf("failed to update payment intent: %w", err)
	}

	logger.Info("Base payment completed successfully")
	return nil
}

// GetIntent retrieves a payment intent by ID with combined status + receipt data
func (s *PaymentIntentService) GetIntent(intentID string) (*dto.GetIntentResponse, error) {
	log.WithField("intent_id", intentID).Info("Querying payment intent")

	var intent models.PaymentIntent
	if err := s.db.Where("intent_id = ?", intentID).First(&intent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query payment intent: %w", err)
	}

	// Check if expired and update status
	if intent.Status == models.StatusPending && intent.IsExpired() {
		intent.Status = models.StatusExpired
		if err := s.db.Save(&intent).Error; err != nil {
			log.WithError(err).Error("Failed to update expired intent")
		}
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
		PayerWallet:       intent.PayerWallet,
		ErrorMessage:      intent.ErrorMessage,
		CreatedAt:         intent.CreatedAt,
		ExpiresAt:         intent.ExpiresAt,
		CompletedAt:       intent.CompletedAt,
	}

	// Add amount if available
	if intent.Amount != "" {
		response.Amount = &intent.Amount
	}

	// Add Solana payment details if available
	if intent.SolTxHash != nil && intent.SolSettleProof != nil && intent.SolSettledAt != nil {
		response.SolanaPayment = &dto.SolanaPayment{
			TxHash:      *intent.SolTxHash,
			SettleProof: *intent.SolSettleProof,
			SettledAt:   *intent.SolSettledAt,
			ExplorerURL: fmt.Sprintf("%s/tx/%s", solanaExplorerBase, *intent.SolTxHash),
		}
	}

	// Add Base payment details if available
	if intent.BaseTxHash != nil && intent.BaseSettleProof != nil && intent.BaseSettledAt != nil {
		response.BasePayment = &dto.BasePayment{
			TxHash:      *intent.BaseTxHash,
			SettleProof: *intent.BaseSettleProof,
			SettledAt:   *intent.BaseSettledAt,
			ExplorerURL: fmt.Sprintf("%s/tx/%s", baseExplorerBase, *intent.BaseTxHash),
		}
	}

	return response, nil
}
