package service

import (
	"fmt"
	"strconv"
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

// VerifyProof verifies an X402 settlement proof.
func (v *X402Verifier) VerifyProof(settleProof string, amount string, merchantRecipient string) error {
	log.WithFields(log.Fields{
		"amount":    amount,
		"recipient": merchantRecipient,
	}).Info("Verifying X402 settlement proof")

	payload, err := types.DecodePaymentPayloadFromBase64(settleProof)
	if err != nil {
		log.WithError(err).Error("Failed to decode payment payload")

		return fmt.Errorf("invalid proof encoding: %w", err)
	}

	requirements := &types.PaymentRequirements{
		Scheme:  "exact-evm",
		Network: v.solanaNetwork,
		PayTo:   merchantRecipient,
	}

	verifyResp, err := v.client.Verify(payload, requirements)
	if err != nil {
		log.WithError(err).Error("X402 proof verification failed")

		return fmt.Errorf("verification failed: %w", err)
	}

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

// SettlePayment settles a payment with the X402 facilitator.
func (v *X402Verifier) SettlePayment(settleProof string, amount string, merchantRecipient string) (*types.SettleResponse, error) {
	log.WithFields(log.Fields{
		"amount":    amount,
		"recipient": merchantRecipient,
	}).Info("Settling payment with X402 facilitator")

	payload, err := types.DecodePaymentPayloadFromBase64(settleProof)
	if err != nil {
		log.WithError(err).Error("Failed to decode payment payload")

		return nil, fmt.Errorf("invalid proof encoding: %w", err)
	}

	requirements := &types.PaymentRequirements{
		Scheme:  "exact-evm",
		Network: v.solanaNetwork,
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
func (v *X402Verifier) ValidateProofAmount(settleProof string, intentAmount string) error {
	payload, err := types.DecodePaymentPayloadFromBase64(settleProof)
	if err != nil {
		return fmt.Errorf("invalid proof encoding: %w", err)
	}

	if payload.Payload == nil || payload.Payload.Authorization == nil || payload.Payload.Authorization.Value == "" {
		return fmt.Errorf("proof does not contain amount information")
	}

	valueStr := payload.Payload.Authorization.Value

	proofAmount, err := strconv.ParseUint(valueStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid proof amount format: %w", err)
	}

	proofAmountDollars := fmt.Sprintf("%.2f", float64(proofAmount)/1e6)

	amount, err := strconv.ParseFloat(intentAmount, 64)
	if err != nil {
		return fmt.Errorf("invalid intent amount format: %w", err)
	}

	intentAmountStr := fmt.Sprintf("%.2f", amount)

	if proofAmountDollars != intentAmountStr {
		return fmt.Errorf("amount mismatch: proof has %s, intent requires %s", proofAmountDollars, intentAmountStr)
	}

	return nil
}
