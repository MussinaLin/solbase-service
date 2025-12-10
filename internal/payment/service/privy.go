package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/agent-tech/x402-api-backend/internal/payment"
	"github.com/agent-tech/x402-api-backend/internal/payment/repository"
	log "github.com/sirupsen/logrus"
)

// PrivyService handles interactions with the Privy API for wallet management.
type PrivyService struct {
	emailWalletRepo repository.EmailWalletRepository
	appID           string
	appSecret       string
	authURL         string
	apiURL          string
	httpClient      *http.Client
}

// NewPrivyService creates a new Privy service instance.
func NewPrivyService(emailWalletRepo repository.EmailWalletRepository, appID, appSecret string) *PrivyService {
	return &PrivyService{
		emailWalletRepo: emailWalletRepo,
		appID:           appID,
		appSecret:       appSecret,
		authURL:         "https://auth.privy.io/api/v1",
		apiURL:          "https://api.privy.io/v1",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WalletForEmail retrieves an existing wallet for the email or creates a new one via Privy.
func (s *PrivyService) WalletForEmail(ctx context.Context, email string) (string, error) {
	logger := log.WithField("email", email)
	logger.Info("Getting or creating wallet for email")

	emailWallet, err := s.emailWalletRepo.GetByEmail(ctx, email)
	if err == nil {
		logger.WithField("wallet", emailWallet.WalletAddress).Info("Found existing email-wallet mapping in database")

		return emailWallet.WalletAddress, nil
	}

	if err != payment.ErrNotFound {
		return "", fmt.Errorf("database error: %w", err)
	}

	logger.Info("Email not found in database, checking Privy")

	privyUser, err := s.userByEmail(email)
	if err != nil {
		logger.WithError(err).Info("User not found in Privy, creating new user")

		privyUser, err = s.createUserWithEmail(email)
		if err != nil {
			return "", fmt.Errorf("create Privy user: %w", err)
		}
	}

	walletAddress := s.extractEthereumWallet(privyUser)
	if walletAddress == "" {
		logger.Info("Creating ethereum wallet for user")

		walletAddress, err = s.createWalletForUser(privyUser.ID)
		if err != nil {
			return "", fmt.Errorf("create wallet: %w", err)
		}
	}

	err = s.emailWalletRepo.Create(ctx, email, walletAddress, privyUser.ID)
	if err != nil {
		logger.WithError(err).Warn("Failed to cache email-wallet mapping in database")
	}

	logger.WithField("wallet", walletAddress).Info("Successfully created wallet for email")

	return walletAddress, nil
}

// privyUser represents a Privy user response.
type privyUser struct {
	ID             string          `json:"id"`
	LinkedAccounts []linkedAccount `json:"linked_accounts"`
	CreatedAt      int64           `json:"created_at"`
}

type linkedAccount struct {
	Type        string `json:"type"`
	Address     string `json:"address,omitempty"`
	ChainType   string `json:"chain_type,omitempty"`
	WalletIndex int    `json:"wallet_index,omitempty"`
}

// userByEmail queries Privy for an existing user by email.
func (s *PrivyService) userByEmail(email string) (*privyUser, error) {
	url := fmt.Sprintf("%s/users/email/address", s.authURL)

	payload := map[string]string{
		"address": email,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	s.setAuthHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("user not found")
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("Privy API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var user privyUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &user, nil
}

// createUserWithEmail creates a new Privy user with the given email.
func (s *PrivyService) createUserWithEmail(email string) (*privyUser, error) {
	url := fmt.Sprintf("%s/users/import", s.authURL)

	payload := map[string]interface{}{
		"users": []map[string]interface{}{
			{
				"linked_accounts": []map[string]interface{}{
					{
						"type":    "email",
						"address": email,
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	s.setAuthHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("Privy API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var importResp struct {
		Results []struct {
			Action  string `json:"action"`
			Index   int    `json:"index"`
			Success bool   `json:"success"`
			ID      string `json:"id"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&importResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(importResp.Results) == 0 || !importResp.Results[0].Success {
		return nil, fmt.Errorf("user import failed")
	}

	return &privyUser{
		ID: importResp.Results[0].ID,
		LinkedAccounts: []linkedAccount{
			{Type: "email", Address: email},
		},
	}, nil
}

// createWalletForUser creates an ethereum wallet for a Privy user.
func (s *PrivyService) createWalletForUser(userID string) (string, error) {
	url := fmt.Sprintf("%s/wallets", s.apiURL)

	payload := map[string]interface{}{
		"chain_type": "ethereum",
		"owner": map[string]string{
			"user_id": userID,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	s.setAuthHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)

		return "", fmt.Errorf("Privy API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var walletResp struct {
		ID        string `json:"id"`
		Address   string `json:"address"`
		ChainType string `json:"chain_type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&walletResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	return walletResp.Address, nil
}

// extractEthereumWallet extracts the ethereum wallet address from a user's linked accounts.
func (s *PrivyService) extractEthereumWallet(user *privyUser) string {
	for _, account := range user.LinkedAccounts {
		if account.Type == "wallet" && account.ChainType == "ethereum" {
			return account.Address
		}
	}

	return ""
}

// setAuthHeaders sets the required authentication headers for Privy API requests.
func (s *PrivyService) setAuthHeaders(req *http.Request) {
	credentials := base64.StdEncoding.EncodeToString([]byte(s.appID + ":" + s.appSecret))
	req.Header.Set("Authorization", "Basic "+credentials)
	req.Header.Set("privy-app-id", s.appID)
	req.Header.Set("Content-Type", "application/json")
}
