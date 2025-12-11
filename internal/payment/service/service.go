package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/agent-tech/x402-api-backend/internal/payment"
	"github.com/agent-tech/x402-api-backend/internal/payment/repository"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// Service implements the payment.Service interface.
type Service struct {
	repo                  repository.Repository
	basePaymentService    *BasePaymentService
	x402Verifier          *X402Verifier
	privyService          *PrivyService
	solanaNetwork         string
	baseNetwork           string
	solanaReceiverAddress string
	bscReceiverAddress    string
}

// Ensure Service implements payment.Service.
var _ payment.Service = (*Service)(nil)

// New creates a new payment service.
func New(
	repo repository.Repository,
	basePaymentService *BasePaymentService,
	x402Verifier *X402Verifier,
	privyService *PrivyService,
	solanaNetwork string,
	baseNetwork string,
	solanaReceiverAddress string,
	bscReceiverAddress string,
) *Service {
	return &Service{
		repo:                  repo,
		basePaymentService:    basePaymentService,
		x402Verifier:          x402Verifier,
		privyService:          privyService,
		solanaNetwork:         solanaNetwork,
		baseNetwork:           baseNetwork,
		solanaReceiverAddress: solanaReceiverAddress,
		bscReceiverAddress:    bscReceiverAddress,
	}
}

// CreateIntent creates a new payment intent with either email or wallet as receiver.
func (s *Service) CreateIntent(ctx context.Context, params *payment.CreateIntentParams) (*payment.Intent, error) {
	if params.Email == "" && params.Recipient == "" {
		return nil, fmt.Errorf("%w: either email or recipient is required", payment.ErrInvalidInput)
	}

	if params.Email != "" && params.Recipient != "" {
		return nil, fmt.Errorf("%w: cannot provide both email and recipient", payment.ErrInvalidInput)
	}

	payerChain := params.PayerChain
	if payerChain == "" {
		payerChain = "solana"
	}

	if !payment.ValidPayerChains[payerChain] {
		return nil, fmt.Errorf("%w: must be solana, base, or bsc", payment.ErrInvalidPayerChain)
	}

	intentID := uuid.New().String()
	expiresAt := time.Now().Add(10 * time.Minute)

	logger := log.WithFields(log.Fields{
		"intent_id":   intentID,
		"payer_chain": payerChain,
	})

	var walletAddress string
	var email *string

	if params.Email != "" {
		logger = logger.WithField("email", params.Email)
		logger.Info("Creating payment intent with email receiver")

		wallet, err := s.privyService.WalletForEmail(ctx, params.Email)
		if err != nil {
			logger.WithError(err).Error("Failed to get wallet for email")

			return nil, fmt.Errorf("resolve email to wallet: %w", err)
		}

		walletAddress = wallet
		email = &params.Email
		logger.WithField("wallet", walletAddress).Info("Resolved email to wallet address")
	} else {
		logger = logger.WithField("recipient", params.Recipient)
		logger.Info("Creating payment intent with wallet receiver")
		walletAddress = params.Recipient
	}

	now := time.Now()

	// Set source recipient based on payer chain
	// For Solana/BSC, users pay to a chain-specific receiver address
	// For Base, users pay directly to the merchant recipient
	var sourceRecipient *string
	switch payerChain {
	case "solana":
		sourceRecipient = &s.solanaReceiverAddress
	case "bsc":
		sourceRecipient = &s.bscReceiverAddress
	case "base":
		// For Base as payer chain, no separate source recipient needed
		sourceRecipient = nil
	}

	intent := &payment.Intent{
		IntentID:          intentID,
		PayerChain:        payerChain,
		TargetChain:       "base",
		MerchantRecipient: walletAddress,
		SourceRecipient:   sourceRecipient,
		ReceiverEmail:     email,
		Amount:            params.Amount,
		Status:            payment.StatusAwaitingPayment,
		CreatedAt:         now,
		ExpiresAt:         expiresAt,
	}

	createdIntent, err := s.repo.Create(ctx, intent)
	if err != nil {
		logger.WithError(err).Error("Failed to create payment intent")

		return nil, fmt.Errorf("create payment intent: %w", err)
	}

	logger.Info("Payment intent created, awaiting payment proof")

	return createdIntent, nil
}

// SubmitProof submits a payment proof for an existing intent.
func (s *Service) SubmitProof(ctx context.Context, intentID string, proof string) (*payment.Intent, error) {
	logger := log.WithField("intent_id", intentID)
	logger.Info("Submitting proof for payment intent")

	intent, err := s.repo.GetByIntentID(ctx, intentID)
	if err != nil {
		return nil, err
	}

	if intent.Status != payment.StatusAwaitingPayment {
		return nil, fmt.Errorf("%w: intent status is %s, expected %s", payment.ErrInvalidStatus, intent.Status, payment.StatusAwaitingPayment)
	}

	if time.Now().After(intent.ExpiresAt) {
		s.repo.UpdateExpired(ctx, intentID)

		return nil, payment.ErrExpired
	}

	if err := s.x402Verifier.ValidateProofAmount(proof, intent.Amount); err != nil {
		logger.WithError(err).Error("Proof validation failed")

		return nil, fmt.Errorf("%w: %s", payment.ErrProofValidation, err.Error())
	}

	err = s.repo.UpdateWithProof(ctx, intentID, proof, payment.StatusPending)
	if err != nil {
		logger.WithError(err).Error("Failed to update intent with proof")

		return nil, fmt.Errorf("update payment intent: %w", err)
	}

	logger.Info("Proof submitted, launching async processing")

	go func() {
		asyncCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		s.processIntentAsync(asyncCtx, intentID, proof, intent.MerchantRecipient, intent.PayerChain)
	}()

	intent.Status = payment.StatusPending
	intent.SolSettleProof = &proof

	return intent, nil
}

// GetIntent retrieves a payment intent by ID with combined status + receipt data.
func (s *Service) GetIntent(ctx context.Context, intentID string) (*payment.Intent, error) {
	log.WithField("intent_id", intentID).Info("Querying payment intent")

	intent, err := s.repo.GetByIntentID(ctx, intentID)
	if err != nil {
		return nil, err
	}

	if (intent.Status == payment.StatusAwaitingPayment || intent.Status == payment.StatusPending) && time.Now().After(intent.ExpiresAt) {
		s.repo.UpdateExpired(ctx, intentID)
		intent.Status = payment.StatusExpired
	}

	return intent, nil
}

// processIntentAsync handles the async verification, settlement, and Base payment flow.
func (s *Service) processIntentAsync(ctx context.Context, intentID string, settleProof string, merchantRecipient string, payerChain string) {
	logger := log.WithFields(log.Fields{
		"intent_id":   intentID,
		"payer_chain": payerChain,
	})
	logger.Info("Starting async intent processing")

	select {
	case <-ctx.Done():
		logger.Error("Context cancelled before verification")
		s.updateIntentError(intentID, "Processing timeout before verification")

		return
	default:
	}

	// Step 1: Verify X402 proof
	logger.Info("Verifying X402 proof with facilitator")

	details, err := s.x402Verifier.VerifyAndExtractDetails(settleProof, merchantRecipient, payerChain)
	if err != nil {
		logger.WithError(err).Error("Proof verification failed")
		s.updateIntentError(intentID, fmt.Sprintf("Verification failed: %s", err.Error()))

		return
	}

	logger.Info("Proof verified successfully")

	select {
	case <-ctx.Done():
		logger.Error("Context cancelled before settlement")
		s.updateIntentError(intentID, "Processing timeout before settlement")

		return
	default:
	}

	// Step 2: Settle payment on source chain via X402 facilitator
	logger.Info("Settling payment on source chain via X402 facilitator")

	settleResp, err := s.x402Verifier.SettlePaymentForChain(settleProof, merchantRecipient, payerChain)
	if err != nil {
		logger.WithError(err).Error("Source chain settlement failed")
		s.updateIntentError(intentID, fmt.Sprintf("Settlement failed: %s", err.Error()))

		return
	}

	// Extract transaction hash from settlement response
	details.TxHash = &settleResp.Transaction
	if settleResp.Payer != nil {
		details.PayerWallet = settleResp.Payer
	}

	logger.WithField("tx_hash", settleResp.Transaction).Info("Source chain settlement successful, updating to SOURCE_SETTLED")

	// Step 3: Update to SOURCE_SETTLED with settlement data
	if err := s.updateIntentSourceSettled(ctx, intentID, details); err != nil {
		logger.WithError(err).Error("Failed to update intent to SOURCE_SETTLED")

		return
	}

	select {
	case <-ctx.Done():
		logger.Error("Context cancelled before Base payment")
		s.updateIntentError(intentID, "Processing timeout before Base payment")

		return
	default:
	}

	// Step 4: Trigger Base payment
	logger.Info("Triggering Base payment")

	if err := s.triggerBasePayment(ctx, intentID); err != nil {
		logger.WithError(err).Error("Base payment failed")
	}
}

// updateIntentError updates the intent with error status.
func (s *Service) updateIntentError(intentID string, errorMsg string) {
	ctx := context.Background()

	err := s.repo.UpdateStatus(ctx, intentID, payment.StatusVerificationFailed, &errorMsg)
	if err != nil {
		log.WithError(err).Error("Failed to update intent with error")
	}
}

// updateIntentSourceSettled updates the intent with source chain settlement data.
func (s *Service) updateIntentSourceSettled(ctx context.Context, intentID string, details *payment.ProofDetails) error {
	return s.repo.UpdateSourceSettled(ctx, intentID, details)
}

// triggerBasePayment triggers Base payment.
func (s *Service) triggerBasePayment(ctx context.Context, intentID string) error {
	logger := log.WithField("intent_id", intentID)
	logger.Info("Executing Base payment")

	intent, err := s.repo.GetByIntentID(ctx, intentID)
	if err != nil {
		return fmt.Errorf("find intent: %w", err)
	}

	if intent.Status != payment.StatusSourceSettled {
		return fmt.Errorf("cannot trigger Base payment: intent status is %s", intent.Status)
	}

	err = s.repo.UpdateStatus(ctx, intentID, payment.StatusBaseSettling, nil)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	amount, err := strconv.ParseFloat(intent.Amount, 64)
	if err != nil {
		s.repo.UpdateStatus(ctx, intentID, payment.StatusSourceSettled, nil)

		return fmt.Errorf("invalid amount: %w", err)
	}

	result, err := s.basePaymentService.ExecutePayment(ctx, payment.PaymentRequest{
		IntentID:         intent.IntentID,
		Amount:           amount,
		RecipientAddress: intent.MerchantRecipient,
	})

	if err != nil {
		logger.WithError(err).Error("Base payment failed, rolling back to SOURCE_SETTLED")
		s.repo.UpdateStatus(ctx, intentID, payment.StatusSourceSettled, nil)

		return fmt.Errorf("Base payment failed: %w", err)
	}

	err = s.repo.UpdateBaseSettled(ctx, intentID, result.TxHash, result.Proof)
	if err != nil {
		logger.WithError(err).Error("Failed to update payment intent")

		return fmt.Errorf("update payment intent: %w", err)
	}

	logger.Info("Base payment completed successfully")

	return nil
}

// SolanaNetwork returns the configured Solana network.
func (s *Service) SolanaNetwork() string {
	return s.solanaNetwork
}

// BaseNetwork returns the configured Base network.
func (s *Service) BaseNetwork() string {
	return s.baseNetwork
}

// BasePaymentExplorerBase returns the Base explorer base URL.
func (s *Service) BasePaymentExplorerBase() string {
	return s.basePaymentService.ExplorerBase()
}

// SourceChainExplorerURL returns the explorer URL for the source/payer chain.
func (s *Service) SourceChainExplorerURL(payerChain string) string {
	switch payerChain {
	case "solana":
		if s.solanaNetwork == "solana-mainnet-beta" {
			return "https://solscan.io"
		}

		return "https://solscan.io?cluster=devnet"
	case "base":
		if s.baseNetwork == "base" {
			return "https://basescan.org"
		}

		return "https://sepolia.basescan.org"
	case "bsc":
		return "https://testnet.bscscan.com"
	default:
		return "https://solscan.io?cluster=devnet"
	}
}
