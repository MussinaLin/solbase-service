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

// CreateIntent creates a new payment intent
func (s *PaymentIntentService) CreateIntent(req dto.CreateIntentRequest) (*dto.CreateIntentResponse, error) {
	intentID := uuid.New().String()
	expiresAt := time.Now().Add(10 * time.Minute) // 10 minutes expiry

	log.WithField("intent_id", intentID).Info("Creating payment intent")

	intent := &models.PaymentIntent{
		ID:                uuid.New().String(),
		IntentID:          intentID,
		Amount:            fmt.Sprintf("%.2f", req.Amount),
		MerchantRecipient: req.MerchantRecipient,
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

	log.WithField("intent_id", intentID).Info("Payment intent created successfully")

	return &dto.CreateIntentResponse{
		IntentID:          intent.IntentID,
		Amount:            intent.Amount,
		MerchantRecipient: intent.MerchantRecipient,
		Status:            intent.Status,
		CreatedAt:         intent.CreatedAt,
		ExpiresAt:         intent.ExpiresAt,
	}, nil
}

// GetIntent retrieves a payment intent by ID
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

	return &dto.GetIntentResponse{
		IntentID:          intent.IntentID,
		Amount:            intent.Amount,
		MerchantRecipient: intent.MerchantRecipient,
		PayerWallet:       intent.PayerWallet,
		Status:            intent.Status,
		SolTxHash:         intent.SolTxHash,
		BaseTxHash:        intent.BaseTxHash,
		CreatedAt:         intent.CreatedAt,
		SolSettledAt:      intent.SolSettledAt,
		BaseSettledAt:     intent.BaseSettledAt,
		CompletedAt:       intent.CompletedAt,
		ExpiresAt:         intent.ExpiresAt,
	}, nil
}

// SubmitSolanaProof submits Solana payment proof and triggers Base payment
func (s *PaymentIntentService) SubmitSolanaProof(intentID string, req dto.SolanaProofRequest) (*dto.SubmitSolanaProofResponse, error) {
	log.WithField("intent_id", intentID).Info("Submitting Solana proof")

	// Find the intent
	var intent models.PaymentIntent
	if err := s.db.Where("intent_id = ?", intentID).First(&intent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("payment intent not found")
		}
		return nil, fmt.Errorf("failed to query payment intent: %w", err)
	}

	// Validate status
	if !intent.CanSubmitSolanaProof() {
		return nil, fmt.Errorf("payment intent %s is not in PENDING status (current: %s)", intentID, intent.Status)
	}

	// Verify proof with X402 facilitator
	if err := s.x402Verifier.VerifyProof(req.SettleProof, intent.Amount, intent.MerchantRecipient); err != nil {
		log.WithError(err).Error("Proof verification failed")
		return nil, fmt.Errorf("invalid settlement proof: %w", err)
	}

	// Update intent with Solana payment details
	now := time.Now()
	intent.Status = models.StatusSolSettled
	intent.SolSettleProof = &req.SettleProof
	intent.SolTxHash = &req.TxHash
	intent.PayerWallet = req.PayerWallet
	intent.SolSettledAt = &now

	if err := s.db.Save(&intent).Error; err != nil {
		log.WithError(err).Error("Failed to update payment intent")
		return nil, fmt.Errorf("failed to update payment intent: %w", err)
	}

	log.WithField("intent_id", intentID).Info("Solana payment verified")

	// Asynchronously trigger Base payment
	go func() {
		if err := s.triggerBasePaymentAsync(intentID); err != nil {
			log.WithError(err).WithField("intent_id", intentID).Error("Failed to trigger Base payment")
		}
	}()

	return &dto.SubmitSolanaProofResponse{
		Success:  true,
		Message:  "Solana payment verified",
		IntentID: intent.IntentID,
		Status:   intent.Status,
	}, nil
}

// TriggerBasePayment manually triggers Base payment
func (s *PaymentIntentService) TriggerBasePayment(intentID string) (*dto.TriggerBasePaymentResponse, error) {
	log.WithField("intent_id", intentID).Info("Manually triggering Base payment")

	// Find the intent
	var intent models.PaymentIntent
	if err := s.db.Where("intent_id = ?", intentID).First(&intent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("payment intent not found")
		}
		return nil, fmt.Errorf("failed to query payment intent: %w", err)
	}

	// Validate status
	if !intent.CanTriggerBasePayment() {
		return nil, fmt.Errorf("Solana payment not settled for intent %s (current status: %s)", intentID, intent.Status)
	}

	// Set status to BASE_SETTLING
	intent.Status = models.StatusBaseSettling
	if err := s.db.Save(&intent).Error; err != nil {
		return nil, fmt.Errorf("failed to update status: %w", err)
	}

	// Execute Base payment
	amount, err := strconv.ParseFloat(intent.Amount, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	result, err := s.basePaymentService.ExecutePayment(context.Background(), PaymentRequest{
		IntentID:         intent.IntentID,
		Amount:           amount,
		RecipientAddress: intent.MerchantRecipient,
	})

	if err != nil {
		log.WithError(err).Error("Base payment failed")

		// Rollback to SOL_SETTLED to allow retry
		intent.Status = models.StatusSolSettled
		if updateErr := s.db.Save(&intent).Error; updateErr != nil {
			log.WithError(updateErr).Error("Failed to rollback status")
		}

		return nil, fmt.Errorf("Base payment failed: %w", err)
	}

	// Update intent with Base payment details
	now := time.Now()
	intent.Status = models.StatusBaseSettled
	intent.BaseTxHash = &result.TxHash
	intent.BaseSettleProof = &result.Proof
	intent.BaseSettledAt = &now
	intent.CompletedAt = &now

	if err := s.db.Save(&intent).Error; err != nil {
		log.WithError(err).Error("Failed to update payment intent")
		return nil, fmt.Errorf("failed to update payment intent: %w", err)
	}

	log.WithField("intent_id", intentID).Info("Base payment completed")

	return &dto.TriggerBasePaymentResponse{
		Success:    true,
		Message:    "Base payment completed",
		BaseTxHash: result.TxHash,
		Status:     intent.Status,
	}, nil
}

// triggerBasePaymentAsync is an internal helper to trigger Base payment asynchronously
func (s *PaymentIntentService) triggerBasePaymentAsync(intentID string) error {
	_, err := s.TriggerBasePayment(intentID)
	return err
}

// GetReceipt retrieves a full payment receipt
func (s *PaymentIntentService) GetReceipt(intentID string) (*dto.ReceiptResponse, error) {
	log.WithField("intent_id", intentID).Info("Generating receipt")

	var intent models.PaymentIntent
	if err := s.db.Where("intent_id = ?", intentID).First(&intent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query payment intent: %w", err)
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

	receipt := &dto.ReceiptResponse{
		IntentID:          intent.IntentID,
		Amount:            intent.Amount,
		MerchantRecipient: intent.MerchantRecipient,
		PayerWallet:       intent.PayerWallet,
		Status:            intent.Status,
		CreatedAt:         intent.CreatedAt,
		CompletedAt:       intent.CompletedAt,
		ExpiresAt:         intent.ExpiresAt,
	}

	// Add Solana payment details if available
	if intent.SolTxHash != nil && intent.SolSettleProof != nil && intent.SolSettledAt != nil {
		receipt.SolanaPayment = &dto.SolanaPayment{
			TxHash:      *intent.SolTxHash,
			SettleProof: *intent.SolSettleProof,
			SettledAt:   *intent.SolSettledAt,
			ExplorerURL: fmt.Sprintf("%s/tx/%s", solanaExplorerBase, *intent.SolTxHash),
		}
	}

	// Add Base payment details if available
	if intent.BaseTxHash != nil && intent.BaseSettleProof != nil && intent.BaseSettledAt != nil {
		receipt.BasePayment = &dto.BasePayment{
			TxHash:      *intent.BaseTxHash,
			SettleProof: *intent.BaseSettleProof,
			SettledAt:   *intent.BaseSettledAt,
			ExplorerURL: fmt.Sprintf("%s/tx/%s", baseExplorerBase, *intent.BaseTxHash),
		}
	}

	return receipt, nil
}
