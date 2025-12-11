package service

import (
	"context"
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/agent-tech/x402-api-backend/internal/payment"
	"github.com/agent-tech/x402-api-backend/internal/payment/repository"
	"github.com/agent-tech/x402-api-backend/pkg/utils"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

// amountRegex validates amount format: positive number with up to 6 decimal places
var amountRegex = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,6})?$`)

// USDC contract/mint addresses per network
var usdcAssets = map[string]string{
	// Solana
	"solana-devnet":       "4zMMC9srt5Ri5X14GAgXhaHii3GnPAEERYPJgZJDncDU", // Devnet USDC
	"solana-mainnet-beta": "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", // Mainnet USDC
	// Base
	"base-sepolia": "0x036CbD53842c5426634e7929541eC2318f3dCF7e", // Base Sepolia USDC
	"base":         "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913", // Base Mainnet USDC
	// BSC
	"bsc-testnet": "0x64544969ed7EBf5f083679233325356EbE738930", // BSC Testnet USDC
	"bsc":         "0x8AC76a51cc950d9822D68b83fE1Ad97B32Cd580d", // BSC Mainnet USDC
}

// Service implements the payment.Service interface.
type Service struct {
	repo                  repository.Repository
	basePaymentService    *BasePaymentService
	x402Verifier          *X402Verifier
	privyService          *PrivyService
	solanaNetwork         string
	baseNetwork           string
	baseSourceNetwork     string
	bscNetwork            string
	solanaReceiverAddress string
	bscReceiverAddress    string
	wg                    sync.WaitGroup // tracks async goroutines for graceful shutdown
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
	baseSourceNetwork string,
	bscNetwork string,
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
		baseSourceNetwork:     baseSourceNetwork,
		bscNetwork:            bscNetwork,
		solanaReceiverAddress: solanaReceiverAddress,
		bscReceiverAddress:    bscReceiverAddress,
	}
}

// validateEmail checks if the email format is valid using net/mail.
func validateEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// validateAmount checks if amount is a valid positive number with up to 6 decimal places.
func validateAmount(amount string) error {
	if !amountRegex.MatchString(amount) {
		return payment.ErrInvalidAmount
	}

	val, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return payment.ErrInvalidAmount
	}

	if val <= 0 {
		return fmt.Errorf("%w: amount must be positive", payment.ErrInvalidAmount)
	}

	// Maximum amount check (e.g., 1 million USDC)
	if val > 1000000 {
		return fmt.Errorf("%w: amount exceeds maximum allowed (1,000,000)", payment.ErrInvalidAmount)
	}

	return nil
}

// CreateIntent creates a new payment intent with either email or wallet as receiver.
func (s *Service) CreateIntent(ctx context.Context, params *payment.CreateIntentParams) (*payment.IntentWithRequirements, error) {
	if params.Email == "" && params.Recipient == "" {
		return nil, fmt.Errorf("%w: either email or recipient is required", payment.ErrInvalidInput)
	}

	if params.Email != "" && params.Recipient != "" {
		return nil, fmt.Errorf("%w: cannot provide both email and recipient", payment.ErrInvalidInput)
	}

	// Validate amount
	if err := validateAmount(params.Amount); err != nil {
		return nil, err
	}

	// Validate email format if provided
	if params.Email != "" && !validateEmail(params.Email) {
		return nil, payment.ErrInvalidEmail
	}

	// Validate recipient address format if provided
	if params.Recipient != "" && !utils.IsValidEthAddress(params.Recipient) {
		return nil, payment.ErrInvalidRecipient
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

	// Build X402 PaymentRequirements
	paymentRequirements := s.buildPaymentRequirements(createdIntent)

	logger.Info("Payment intent created, awaiting payment proof")

	return &payment.IntentWithRequirements{
		Intent:              createdIntent,
		PaymentRequirements: paymentRequirements,
	}, nil
}

// buildPaymentRequirements constructs X402 PaymentRequirements for an intent.
// Note: The intent.MerchantRecipient (Base wallet address) is stored separately
// and will be used later when executing the Base chain transfer via BASE_PROXY_PRIVATE_KEY.
func (s *Service) buildPaymentRequirements(intent *payment.Intent) *payment.PaymentRequirements {
	// Determine network and payTo address based on payer chain
	// IMPORTANT: payTo must be in the correct format for the payer chain:
	// - Solana: base58 Solana address (SOLANA_RECEIVER_ADDRESS)
	// - BSC: 0x Ethereum address (BSC_RECEIVER_ADDRESS)
	// - Base: 0x Ethereum address (merchant_recipient - user's input wallet)
	//
	// The merchant_recipient (final receiver on Base) is stored in the intent
	// and will be used when executing the Base chain payment after settlement.
	var network string
	var payTo string

	switch intent.PayerChain {
	case "solana":
		network = s.solanaNetwork
		payTo = s.solanaReceiverAddress // Solana address (base58)
	case "bsc":
		network = s.bscNetwork
		payTo = s.bscReceiverAddress // BSC address (0x...)
	case "base":
		network = s.baseSourceNetwork
		payTo = intent.MerchantRecipient // User's Base wallet (0x...)
	default:
		network = s.solanaNetwork
		payTo = s.solanaReceiverAddress
	}

	// Get USDC asset address for the network
	asset := usdcAssets[network]

	// Convert amount to microdollars (USDC has 6 decimals)
	amountMicros, _ := amountToMicrodollars(intent.Amount)
	maxAmountRequired := strconv.FormatUint(amountMicros, 10)

	// Calculate timeout in seconds until expiry
	maxTimeoutSeconds := int(time.Until(intent.ExpiresAt).Seconds())
	if maxTimeoutSeconds < 0 {
		maxTimeoutSeconds = 0
	}

	return &payment.PaymentRequirements{
		Scheme:            "exact",
		Network:           network,
		MaxAmountRequired: maxAmountRequired,
		PayTo:             payTo,
		Asset:             asset,
		MaxTimeoutSeconds: maxTimeoutSeconds,
		Resource:          fmt.Sprintf("/api/intents/%s", intent.IntentID),
		Description:       fmt.Sprintf("Payment of %s USDC", intent.Amount),
	}
}

// SubmitProof submits a payment proof for an existing intent.
func (s *Service) SubmitProof(ctx context.Context, intentID string, proof string) (*payment.Intent, error) {
	logger := log.WithField("intent_id", intentID)
	logger.Info("Submitting proof for payment intent")

	// First, get the intent to validate proof amount and check expiration
	intent, err := s.repo.GetByIntentID(ctx, intentID)
	if err != nil {
		return nil, err
	}

	// Check expiration first
	if time.Now().After(intent.ExpiresAt) {
		if err := s.repo.UpdateExpired(ctx, intentID); err != nil {
			logger.WithError(err).Error("Failed to update intent as expired")
		}

		return nil, payment.ErrExpired
	}

	// Validate proof amount before attempting atomic update
	if err := s.x402Verifier.ValidateProofAmount(proof, intent.Amount); err != nil {
		logger.WithError(err).Error("Proof validation failed")

		return nil, fmt.Errorf("%w: %s", payment.ErrProofValidation, err.Error())
	}

	// Use optimistic locking: atomically update only if status is still AWAITING_PAYMENT
	// This prevents double-spend vulnerability from concurrent proof submissions
	rowsAffected, err := s.repo.UpdateWithProofIfStatus(ctx, intentID, proof, payment.StatusPending, payment.StatusAwaitingPayment)
	if err != nil {
		logger.WithError(err).Error("Failed to update intent with proof")

		return nil, fmt.Errorf("update payment intent: %w", err)
	}

	// If no rows were affected, the status was already changed (concurrent update or already processed)
	if rowsAffected == 0 {
		// Re-fetch to see current status
		currentIntent, fetchErr := s.repo.GetByIntentID(ctx, intentID)
		if fetchErr != nil {
			return nil, fmt.Errorf("%w: could not verify current status", payment.ErrConcurrentUpdate)
		}

		logger.WithField("current_status", currentIntent.Status).Warn("Concurrent update detected, intent already processed")

		return nil, fmt.Errorf("%w: intent status is %s, expected %s", payment.ErrInvalidStatus, currentIntent.Status, payment.StatusAwaitingPayment)
	}

	logger.Info("Proof submitted, launching async processing")

	// Track the goroutine for graceful shutdown
	s.wg.Add(1)

	go func() {
		defer s.wg.Done()

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
	logger := log.WithField("intent_id", intentID)
	logger.Info("Querying payment intent")

	intent, err := s.repo.GetByIntentID(ctx, intentID)
	if err != nil {
		return nil, err
	}

	if (intent.Status == payment.StatusAwaitingPayment || intent.Status == payment.StatusPending) && time.Now().After(intent.ExpiresAt) {
		if err := s.repo.UpdateExpired(ctx, intentID); err != nil {
			logger.WithError(err).Error("Failed to update intent as expired")
			// Continue anyway - we'll return the expired status in the response
		}

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

	// Step 4: Handle Base payment based on payer chain
	// For Base chain payments: X402 settlement already transferred funds directly to merchant
	// For Solana/BSC payments: Need to transfer from proxy wallet to merchant on Base
	if payerChain == "base" {
		// Base chain: Settlement already sent funds to merchant, just mark as complete
		logger.Info("Base chain payment: Settlement complete, marking as BASE_SETTLED")

		if err := s.repo.UpdateBaseSettledDirect(ctx, intentID); err != nil {
			logger.WithError(err).Error("Failed to update intent to BASE_SETTLED")
			errMsg := fmt.Sprintf("Failed to finalize: %s", err.Error())
			if updateErr := s.repo.UpdateStatus(ctx, intentID, payment.StatusSourceSettled, &errMsg); updateErr != nil {
				logger.WithError(updateErr).Error("Failed to update intent with error")
			}
		}

		return
	}

	// Solana/BSC chain: Need to trigger Base payment from proxy wallet
	select {
	case <-ctx.Done():
		logger.Error("Context cancelled before Base payment")
		s.updateIntentError(intentID, "Processing timeout before Base payment")

		return
	default:
	}

	logger.Info("Triggering Base payment from proxy wallet")

	if err := s.triggerBasePayment(ctx, intentID); err != nil {
		logger.WithError(err).Error("Base payment failed")
		// Update status with error message so it's visible in GetIntent
		errMsg := fmt.Sprintf("Base payment failed: %s", err.Error())
		if updateErr := s.repo.UpdateStatus(ctx, intentID, payment.StatusSourceSettled, &errMsg); updateErr != nil {
			logger.WithError(updateErr).Error("Failed to update intent with Base payment error")
		}
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
		if rollbackErr := s.repo.UpdateStatus(ctx, intentID, payment.StatusSourceSettled, nil); rollbackErr != nil {
			logger.WithError(rollbackErr).Error("Failed to rollback status after amount parse error")
		}

		return fmt.Errorf("invalid amount: %w", err)
	}

	result, err := s.basePaymentService.ExecutePayment(ctx, payment.PaymentRequest{
		IntentID:         intent.IntentID,
		Amount:           amount,
		RecipientAddress: intent.MerchantRecipient,
	})

	if err != nil {
		logger.WithError(err).Error("Base payment failed, rolling back to SOURCE_SETTLED")

		if rollbackErr := s.repo.UpdateStatus(ctx, intentID, payment.StatusSourceSettled, nil); rollbackErr != nil {
			logger.WithError(rollbackErr).Error("Failed to rollback status after Base payment failure")
		}

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

// Shutdown gracefully waits for all async payment processing goroutines to complete.
// Call this during application shutdown to ensure no payments are left in an inconsistent state.
func (s *Service) Shutdown() {
	log.Info("Waiting for async payment processing to complete...")
	s.wg.Wait()
	log.Info("All async payment processing completed")
}
