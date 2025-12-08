package services

import (
	"fmt"
	"strconv"
	"time"

	"github.com/coinbase/x402/go/pkg/facilitatorclient"
	"github.com/coinbase/x402/go/pkg/types"
	log "github.com/sirupsen/logrus"
)

// X402Verifier handles X402 proof verification
type X402Verifier struct {
	client        *facilitatorclient.FacilitatorClient
	solanaNetwork string
}

// ProofDetails contains extracted information from a verified X402 proof
type ProofDetails struct {
	Amount      string
	PayerWallet *string
	TxHash      *string
}

// NewX402Verifier creates a new X402 verifier instance
func NewX402Verifier(facilitatorURL string, solanaNetwork string) *X402Verifier {
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

	log.WithFields(log.Fields{
		"facilitator_url": facilitatorURL,
		"solana_network":  solanaNetwork,
	}).Info("X402 verifier initialized")

	return &X402Verifier{
		client:        client,
		solanaNetwork: solanaNetwork,
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
	requirements := &types.PaymentRequirements{
		Scheme:  "exact-evm",
		Network: v.solanaNetwork,
		PayTo:   merchantRecipient,
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
		Network: v.solanaNetwork,
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

// VerifyAndExtractDetails verifies an X402 proof and extracts payment details
// This is used in the proof-first flow where we need to extract amount from the proof
func (v *X402Verifier) VerifyAndExtractDetails(settleProof string, merchantRecipient string) (*ProofDetails, error) {
	log.WithField("recipient", merchantRecipient).Info("Verifying X402 proof and extracting details")

	// Decode the payment payload from base64-encoded proof
	payload, err := types.DecodePaymentPayloadFromBase64(settleProof)
	if err != nil {
		log.WithError(err).Error("Failed to decode payment payload")
		return nil, fmt.Errorf("invalid proof encoding: %w", err)
	}

	// Create payment requirements
	requirements := &types.PaymentRequirements{
		Scheme:  "exact-evm",
		Network: v.solanaNetwork,
		PayTo:   merchantRecipient,
	}

	// Verify the proof with the facilitator
	verifyResp, err := v.client.Verify(payload, requirements)
	if err != nil {
		log.WithError(err).Error("X402 proof verification failed")
		return nil, fmt.Errorf("verification failed: %w", err)
	}

	// Check if the proof is valid
	if !verifyResp.IsValid {
		reason := "unknown"
		if verifyResp.InvalidReason != nil {
			reason = *verifyResp.InvalidReason
		}
		log.WithField("reason", reason).Error("X402 proof is invalid")
		return nil, fmt.Errorf("proof verification failed: %s", reason)
	}

	// Extract details
	details := &ProofDetails{}

	// Extract amount from payload's Authorization if available
	// Structure: payload.Payload.Authorization.Value (string in wei/microdollars)
	if payload.Payload != nil && payload.Payload.Authorization != nil && payload.Payload.Authorization.Value != "" {
		// Value is a string representing the amount in the smallest unit
		// Parse it and convert to dollars (assuming 6 decimals for USDC)
		valueStr := payload.Payload.Authorization.Value
		// Try to parse as integer first
		if value, err := strconv.ParseUint(valueStr, 10, 64); err == nil {
			details.Amount = fmt.Sprintf("%.2f", float64(value)/1e6)
		} else {
			// If parsing fails, use the raw value
			details.Amount = valueStr
		}

		// Extract payer (From address) from authorization
		if payload.Payload.Authorization.From != "" {
			from := payload.Payload.Authorization.From
			details.PayerWallet = &from
		}
	} else {
		// Default amount if not extractable
		details.Amount = "0.00"
	}

	// Override payer wallet from verification response if available (Payer is *string)
	if verifyResp.Payer != nil && *verifyResp.Payer != "" {
		details.PayerWallet = verifyResp.Payer
	}

	log.WithFields(log.Fields{
		"payer":  verifyResp.Payer,
		"amount": details.Amount,
	}).Info("X402 proof verified and details extracted")

	return details, nil
}
