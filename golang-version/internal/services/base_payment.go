package services

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	log "github.com/sirupsen/logrus"
)

// BasePaymentService handles USDC transfers on Base chain
type BasePaymentService struct {
	client         *ethclient.Client
	privateKey     *ecdsa.PrivateKey
	fromAddress    common.Address
	usdcContract   common.Address
	network        string
	explorerBase   string
}

// PaymentRequest represents a Base payment request
type PaymentRequest struct {
	IntentID         string
	Amount           float64 // USD amount
	RecipientAddress string  // 0x... EVM address
}

// PaymentResult represents the result of a Base payment
type PaymentResult struct {
	TxHash string
	Proof  string
}

// NewBasePaymentService creates a new Base payment service
func NewBasePaymentService(network string, privateKeyHex string) (*BasePaymentService, error) {
	// Determine RPC URL and USDC contract based on network
	var rpcURL string
	var usdcContract common.Address
	var explorerBase string

	switch network {
	case "base":
		rpcURL = "https://mainnet.base.org"
		usdcContract = common.HexToAddress("0x833589fcd6edb6e08f4c7c32d4f71b54bda02913")
		explorerBase = "https://basescan.org"
	case "base-sepolia":
		rpcURL = "https://sepolia.base.org"
		usdcContract = common.HexToAddress("0x036CbD53842c5426634e7929541eC2318f3dCF7e")
		explorerBase = "https://sepolia.basescan.org"
	default:
		return nil, fmt.Errorf("unsupported network: %s", network)
	}

	// Connect to the Ethereum client
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Base network: %w", err)
	}

	// Remove 0x prefix if present
	privateKeyHex = strings.TrimPrefix(privateKeyHex, "0x")

	// Parse private key
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	// Derive public key and address
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("error casting public key to ECDSA")
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	log.WithFields(log.Fields{
		"network":      network,
		"proxy_wallet": fromAddress.Hex(),
		"usdc_contract": usdcContract.Hex(),
	}).Info("Base payment service initialized")

	return &BasePaymentService{
		client:       client,
		privateKey:   privateKey,
		fromAddress:  fromAddress,
		usdcContract: usdcContract,
		network:      network,
		explorerBase: explorerBase,
	}, nil
}

// ExecutePayment executes a USDC transfer on Base chain
func (s *BasePaymentService) ExecutePayment(ctx context.Context, request PaymentRequest) (*PaymentResult, error) {
	log.WithFields(log.Fields{
		"intent_id": request.IntentID,
		"amount":    request.Amount,
		"recipient": request.RecipientAddress,
	}).Info("Starting Base payment")

	recipient := common.HexToAddress(request.RecipientAddress)

	// USDC has 6 decimals
	amountInUsdc := new(big.Int)
	amountFloat := new(big.Float).SetFloat64(request.Amount * 1e6)
	amountFloat.Int(amountInUsdc)

	log.WithFields(log.Fields{
		"from":    s.fromAddress.Hex(),
		"to":      recipient.Hex(),
		"amount":  amountInUsdc.String(),
		"contract": s.usdcContract.Hex(),
	}).Info("Payment details")

	// Check wallet USDC balance
	balance, err := s.getUSDCBalance(ctx, s.fromAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to check balance: %w", err)
	}

	balanceInUsdc := new(big.Float).Quo(new(big.Float).SetInt(balance), big.NewFloat(1e6))
	log.WithField("balance", balanceInUsdc.Text('f', 6)).Info("Wallet USDC balance")

	if balance.Cmp(amountInUsdc) < 0 {
		return nil, fmt.Errorf("insufficient USDC balance: have %s, need %s", balanceInUsdc.Text('f', 6), request.Amount)
	}

	// Prepare USDC transfer transaction
	log.Info("Sending USDC transfer transaction...")

	txHash, err := s.transferUSDC(ctx, recipient, amountInUsdc)
	if err != nil {
		return nil, fmt.Errorf("USDC transfer failed: %w", err)
	}

	log.WithField("tx_hash", txHash).Info("Transaction sent")

	// Wait for transaction confirmation
	log.Info("Waiting for transaction confirmation...")

	receipt, err := s.waitForTransaction(ctx, common.HexToHash(txHash))
	if err != nil {
		return nil, fmt.Errorf("transaction confirmation failed: %w", err)
	}

	log.WithFields(log.Fields{
		"block_number": receipt.BlockNumber.Uint64(),
		"status":       receipt.Status,
	}).Info("Transaction confirmed")

	if receipt.Status != 1 {
		return nil, fmt.Errorf("transaction failed with status: %d", receipt.Status)
	}

	explorerURL := fmt.Sprintf("%s/tx/%s", s.explorerBase, txHash)
	log.WithField("explorer", explorerURL).Info("Transaction successful")

	return &PaymentResult{
		TxHash: txHash,
		Proof:  fmt.Sprintf("base_settlement_%s", txHash),
	}, nil
}

// getUSDCBalance gets the USDC balance of an address
func (s *BasePaymentService) getUSDCBalance(ctx context.Context, address common.Address) (*big.Int, error) {
	// ERC20 balanceOf ABI
	balanceOfABI := `[{"constant":true,"inputs":[{"name":"_owner","type":"address"}],"name":"balanceOf","outputs":[{"name":"balance","type":"uint256"}],"type":"function"}]`

	parsedABI, err := abi.JSON(strings.NewReader(balanceOfABI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ABI: %w", err)
	}

	data, err := parsedABI.Pack("balanceOf", address)
	if err != nil {
		return nil, fmt.Errorf("failed to pack data: %w", err)
	}

	msg := ethereum.CallMsg{
		To:   &s.usdcContract,
		Data: data,
	}

	result, err := s.client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, fmt.Errorf("contract call failed: %w", err)
	}

	balance := new(big.Int).SetBytes(result)
	return balance, nil
}

// transferUSDC sends a USDC transfer transaction
func (s *BasePaymentService) transferUSDC(ctx context.Context, to common.Address, amount *big.Int) (string, error) {
	// ERC20 transfer ABI
	transferABI := `[{"constant":false,"inputs":[{"name":"_to","type":"address"},{"name":"_value","type":"uint256"}],"name":"transfer","outputs":[{"name":"","type":"bool"}],"type":"function"}]`

	parsedABI, err := abi.JSON(strings.NewReader(transferABI))
	if err != nil {
		return "", fmt.Errorf("failed to parse ABI: %w", err)
	}

	data, err := parsedABI.Pack("transfer", to, amount)
	if err != nil {
		return "", fmt.Errorf("failed to pack data: %w", err)
	}

	// Get nonce
	nonce, err := s.client.PendingNonceAt(ctx, s.fromAddress)
	if err != nil {
		return "", fmt.Errorf("failed to get nonce: %w", err)
	}

	// Get gas price
	gasPrice, err := s.client.SuggestGasPrice(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get gas price: %w", err)
	}

	// Get chain ID
	chainID, err := s.client.NetworkID(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get chain ID: %w", err)
	}

	// Create transaction
	tx := types.NewTransaction(
		nonce,
		s.usdcContract,
		big.NewInt(0),
		uint64(100000), // Gas limit for ERC20 transfer
		gasPrice,
		data,
	)

	// Sign transaction
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), s.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Send transaction
	err = s.client.SendTransaction(ctx, signedTx)
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	return signedTx.Hash().Hex(), nil
}

// waitForTransaction waits for a transaction to be mined
func (s *BasePaymentService) waitForTransaction(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	for {
		receipt, err := s.client.TransactionReceipt(ctx, txHash)
		if err == nil {
			return receipt, nil
		}

		// If error is not "not found", return it
		if !strings.Contains(err.Error(), "not found") {
			return nil, err
		}

		// Wait a bit before checking again
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
			// Continue loop
		}
	}
}

// Close closes the Ethereum client connection
func (s *BasePaymentService) Close() {
	if s.client != nil {
		s.client.Close()
	}
}
