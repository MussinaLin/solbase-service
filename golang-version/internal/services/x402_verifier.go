package services

import (
	"fmt"
	"time"

	"github.com/coinbase/x402/go/pkg/facilitatorclient"
	"github.com/coinbase/x402/go/pkg/types"
	log "github.com/sirupsen/logrus"
)

// X402Verifier handles X402 proof verification
type X402Verifier struct {
	client *facilitatorclient.FacilitatorClient
}

// NewX402Verifier creates a new X402 verifier instance
func NewX402Verifier(facilitatorURL string) *X402Verifier {
	config := &types.FacilitatorConfig{
		URL: facilitatorURL,
		Timeout: func() time.Duration {
			return 30 * time.Second
		},
		CreateAuthHeaders: func() (map[string]map[string]string, error) {
			// No auth headers for default facilitator
			return nil, nil
		},
	}

	client := facilitatorclient.NewFacilitatorClient(config)

	log.WithField("facilitator_url", facilitatorURL).Info("X402 verifier initialized")

	return &X402Verifier{
		client: client,
	}
}

// VerifyProof verifies an X402 settlement proof
func (v *X402Verifier) VerifyProof(settleProof string, amount string, merchantRecipient string) error {
	log.WithFields(log.Fields{
		"amount":    amount,
		"recipient": merchantRecipient,
	}).Info("Verifying X402 settlement proof")

	// Decode the payment payload from base64-encoded proof
	payload, err := types.DecodePaymentPayloadFromBase64(settleProof)
	if err != nil {
		log.WithError(err).Error("Failed to decode payment payload")
		return fmt.Errorf("invalid proof encoding: %w", err)
	}

	// Create payment requirements
	// Note: The actual requirements should match what was expected
	requirements := &types.PaymentRequirements{
		Scheme:  "exact-evm",
		Network: "solana-devnet", // This should be configurable based on network
		// Amount should be in microdollars (USD * 1000000)
		// For simplicity, we're just verifying the proof structure
		PayTo: merchantRecipient,
	}

	// Verify the proof with the facilitator
	verifyResp, err := v.client.Verify(payload, requirements)
	if err != nil {
		log.WithError(err).Error("X402 proof verification failed")
		return fmt.Errorf("verification failed: %w", err)
	}

	// Check if the proof is valid
	if !verifyResp.IsValid {
		reason := "unknown"
		if verifyResp.InvalidReason != nil {
			reason = *verifyResp.InvalidReason
		}
		log.WithField("reason", reason).Error("X402 proof is invalid")
		return fmt.Errorf("proof verification failed: %s", reason)
	}

	log.WithFields(log.Fields{
		"payer": verifyResp.Payer,
	}).Info("X402 proof verified successfully")

	return nil
}

// SettlePayment settles a payment with the X402 facilitator
func (v *X402Verifier) SettlePayment(settleProof string, amount string, merchantRecipient string) (*types.SettleResponse, error) {
	log.WithFields(log.Fields{
		"amount":    amount,
		"recipient": merchantRecipient,
	}).Info("Settling payment with X402 facilitator")

	// Decode the payment payload
	payload, err := types.DecodePaymentPayloadFromBase64(settleProof)
	if err != nil {
		log.WithError(err).Error("Failed to decode payment payload")
		return nil, fmt.Errorf("invalid proof encoding: %w", err)
	}

	// Create payment requirements
	requirements := &types.PaymentRequirements{
		Scheme:  "exact-evm",
		Network: "solana-devnet",
		PayTo:   merchantRecipient,
	}

	// Settle the payment
	settleResp, err := v.client.Settle(payload, requirements)
	if err != nil {
		log.WithError(err).Error("X402 settlement failed")
		return nil, fmt.Errorf("settlement failed: %w", err)
	}

	// Check if settlement was successful
	if !settleResp.Success {
		reason := "unknown"
		if settleResp.ErrorReason != nil {
			reason = *settleResp.ErrorReason
		}
		log.WithField("reason", reason).Error("X402 settlement unsuccessful")
		return nil, fmt.Errorf("settlement failed: %s", reason)
	}

	log.WithFields(log.Fields{
		"transaction": settleResp.Transaction,
		"network":     settleResp.Network,
		"payer":       settleResp.Payer,
	}).Info("Payment settled successfully with X402 facilitator")

	return settleResp, nil
}
