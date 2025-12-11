package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/agent-tech/x402-api-backend/internal/payment"
	"github.com/coinbase/x402/go/pkg/facilitatorclient"
	"github.com/coinbase/x402/go/pkg/types"
	log "github.com/sirupsen/logrus"
)

// X402Verifier handles X402 proof verification.
type X402Verifier struct {
	client            *facilitatorclient.FacilitatorClient
	solanaNetwork     string
	baseSourceNetwork string
	bscNetwork        string
}

// NewX402Verifier creates a new X402 verifier instance.
func NewX402Verifier(facilitatorURL string, solanaNetwork string, baseSourceNetwork string, bscNetwork string) *X402Verifier {
	config := &types.FacilitatorConfig{
		URL: facilitatorURL,
		Timeout: func() time.Duration {
			return 30 * time.Second
		},
		CreateAuthHeaders: func() (map[string]map[string]string, error) {
			return nil, nil
		},
	}

	client := facilitatorclient.NewFacilitatorClient(config)

	log.WithFields(log.Fields{
		"facilitator_url":     facilitatorURL,
		"solana_network":      solanaNetwork,
		"base_source_network": baseSourceNetwork,
		"bsc_network":         bscNetwork,
	}).Info("X402 verifier initialized")

	return &X402Verifier{
		client:            client,
		solanaNetwork:     solanaNetwork,
		baseSourceNetwork: baseSourceNetwork,
		bscNetwork:        bscNetwork,
	}
}

// networkForChain returns the appropriate network string for the given payer chain.
func (v *X402Verifier) networkForChain(payerChain string) (string, error) {
	switch payerChain {
	case "solana":
		return v.solanaNetwork, nil
	case "base":
		return v.baseSourceNetwork, nil
	case "bsc":
		return v.bscNetwork, nil
	default:
		return "", fmt.Errorf("unsupported payer chain: %s", payerChain)
	}
}

// SettlePaymentForChain settles a payment with the X402 facilitator for the specified chain.
func (v *X402Verifier) SettlePaymentForChain(settleProof string, merchantRecipient string, payerChain string) (*types.SettleResponse, error) {
	log.WithFields(log.Fields{
		"recipient":   merchantRecipient,
		"payer_chain": payerChain,
	}).Info("Settling payment with X402 facilitator")

	network, err := v.networkForChain(payerChain)
	if err != nil {
		return nil, err
	}

	payload, err := types.DecodePaymentPayloadFromBase64(settleProof)
	if err != nil {
		log.WithError(err).Error("Failed to decode payment payload")

		return nil, fmt.Errorf("invalid proof encoding: %w", err)
	}

	requirements := &types.PaymentRequirements{
		Scheme:  "exact-evm",
		Network: network,
		PayTo:   merchantRecipient,
	}

	settleResp, err := v.client.Settle(payload, requirements)
	if err != nil {
		log.WithError(err).Error("X402 settlement failed")

		return nil, fmt.Errorf("settlement failed: %w", err)
	}

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

// VerifyAndExtractDetails verifies an X402 proof and extracts payment details.
func (v *X402Verifier) VerifyAndExtractDetails(settleProof string, merchantRecipient string, payerChain string) (*payment.ProofDetails, error) {
	log.WithFields(log.Fields{
		"recipient":   merchantRecipient,
		"payer_chain": payerChain,
	}).Info("Verifying X402 proof and extracting details")

	network, err := v.networkForChain(payerChain)
	if err != nil {
		return nil, err
	}

	payload, err := types.DecodePaymentPayloadFromBase64(settleProof)
	if err != nil {
		log.WithError(err).Error("Failed to decode payment payload")

		return nil, fmt.Errorf("invalid proof encoding: %w", err)
	}

	requirements := &types.PaymentRequirements{
		Scheme:  "exact-evm",
		Network: network,
		PayTo:   merchantRecipient,
	}

	verifyResp, err := v.client.Verify(payload, requirements)
	if err != nil {
		log.WithError(err).Error("X402 proof verification failed")

		return nil, fmt.Errorf("verification failed: %w", err)
	}

	if !verifyResp.IsValid {
		reason := "unknown"
		if verifyResp.InvalidReason != nil {
			reason = *verifyResp.InvalidReason
		}

		log.WithField("reason", reason).Error("X402 proof is invalid")

		return nil, fmt.Errorf("proof verification failed: %s", reason)
	}

	details := &payment.ProofDetails{}

	if payload.Payload != nil && payload.Payload.Authorization != nil && payload.Payload.Authorization.Value != "" {
		valueStr := payload.Payload.Authorization.Value

		if value, err := strconv.ParseUint(valueStr, 10, 64); err == nil {
			details.Amount = fmt.Sprintf("%.2f", float64(value)/1e6)
		} else {
			details.Amount = valueStr
		}

		if payload.Payload.Authorization.From != "" {
			from := payload.Payload.Authorization.From
			details.PayerWallet = &from
		}
	} else {
		details.Amount = "0.00"
	}

	if verifyResp.Payer != nil && *verifyResp.Payer != "" {
		details.PayerWallet = verifyResp.Payer
	}

	log.WithFields(log.Fields{
		"payer":  verifyResp.Payer,
		"amount": details.Amount,
	}).Info("X402 proof verified and details extracted")

	return details, nil
}

// ValidateProofAmount validates that the proof matches the intent's amount.
// Uses integer arithmetic to avoid floating-point precision issues.
func (v *X402Verifier) ValidateProofAmount(settleProof string, intentAmount string) error {
	payload, err := types.DecodePaymentPayloadFromBase64(settleProof)
	if err != nil {
		return fmt.Errorf("invalid proof encoding: %w", err)
	}

	if payload.Payload == nil || payload.Payload.Authorization == nil || payload.Payload.Authorization.Value == "" {
		return fmt.Errorf("proof does not contain amount information")
	}

	valueStr := payload.Payload.Authorization.Value

	// Proof amount is in microdollars (6 decimal places)
	proofMicrodollars, err := strconv.ParseUint(valueStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid proof amount format: %w", err)
	}

	// Convert intent amount (string like "10.00" or "10.5") to microdollars
	// Using integer arithmetic to avoid floating-point precision issues
	intentMicrodollars, err := amountToMicrodollars(intentAmount)
	if err != nil {
		return fmt.Errorf("invalid intent amount format: %w", err)
	}

	if proofMicrodollars != intentMicrodollars {
		proofDollars := float64(proofMicrodollars) / 1e6
		intentDollars := float64(intentMicrodollars) / 1e6

		return fmt.Errorf("amount mismatch: proof has %.6f, intent requires %.6f", proofDollars, intentDollars)
	}

	return nil
}

// amountToMicrodollars converts a dollar amount string (e.g., "10.50") to microdollars (uint64).
// Uses string parsing to avoid floating-point precision issues.
func amountToMicrodollars(amount string) (uint64, error) {
	// Split on decimal point
	parts := strings.Split(amount, ".")

	var dollars uint64
	var cents uint64

	// Parse whole dollars
	if parts[0] != "" {
		d, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid dollars: %w", err)
		}

		dollars = d
	}

	// Parse fractional part if present
	if len(parts) == 2 && parts[1] != "" {
		// Pad or truncate to 6 decimal places (microdollars)
		fractional := parts[1]
		if len(fractional) > 6 {
			fractional = fractional[:6]
		}

		for len(fractional) < 6 {
			fractional += "0"
		}

		c, err := strconv.ParseUint(fractional, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid fractional: %w", err)
		}

		cents = c
	}

	return dollars*1e6 + cents, nil
}
