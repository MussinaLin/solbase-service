package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/agent-tech/x402-api-backend/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	log "github.com/sirupsen/logrus"
)

// PrivyService handles interactions with the Privy API for wallet management
type PrivyService struct {
	queries    *db.Queries
	appID      string
	appSecret  string
	authURL    string
	apiURL     string
	httpClient *http.Client
}

// NewPrivyService creates a new Privy service instance
func NewPrivyService(queries *db.Queries, appID, appSecret string) *PrivyService {
	return &PrivyService{
		queries:   queries,
		appID:     appID,
		appSecret: appSecret,
		authURL:   "https://auth.privy.io/api/v1",
		apiURL:    "https://api.privy.io/v1",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetOrCreateWalletForEmail retrieves an existing wallet for the email or creates a new one via Privy
func (s *PrivyService) GetOrCreateWalletForEmail(email string) (string, error) {
	logger := log.WithField("email", email)
	logger.Info("Getting or creating wallet for email")

	ctx := context.Background()

	// First, check if email exists in our database
	emailWallet, err := s.queries.GetEmailWalletByEmail(ctx, email)
	if err == nil {
		// Found in database
		logger.WithField("wallet", emailWallet.WalletAddress).Info("Found existing email-wallet mapping in database")
		return emailWallet.WalletAddress, nil
	}
	if err != pgx.ErrNoRows {
		return "", fmt.Errorf("database error: %w", err)
	}

	// Not in database, check if user exists in Privy
	logger.Info("Email not found in database, checking Privy")

	privyUser, err := s.getUserByEmail(email)
	if err != nil {
		logger.WithError(err).Info("User not found in Privy, creating new user")

		// Create new user with email in Privy
		privyUser, err = s.createUserWithEmail(email)
		if err != nil {
			return "", fmt.Errorf("failed to create Privy user: %w", err)
		}
	}

	// Check if user already has an ethereum wallet
	walletAddress := s.extractEthereumWallet(privyUser)
	if walletAddress == "" {
		// Create wallet for user
		logger.Info("Creating ethereum wallet for user")
		walletAddress, err = s.createWalletForUser(privyUser.ID)
		if err != nil {
			return "", fmt.Errorf("failed to create wallet: %w", err)
		}
	}

	// Store the mapping in our database
	now := time.Now()
	_, err = s.queries.CreateEmailWallet(ctx, db.CreateEmailWalletParams{
		ID:            uuid.New().String(),
		Email:         email,
		WalletAddress: walletAddress,
		PrivyUserID:   privyUser.ID,
		CreatedAt:     pgtype.Timestamp{Time: now, Valid: true},
		UpdatedAt:     pgtype.Timestamp{Time: now, Valid: true},
	})
	if err != nil {
		// Log error but don't fail - the wallet was created successfully
		logger.WithError(err).Warn("Failed to cache email-wallet mapping in database")
	}

	logger.WithField("wallet", walletAddress).Info("Successfully created wallet for email")
	return walletAddress, nil
}

// privyUser represents a Privy user response
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

// getUserByEmail queries Privy for an existing user by email
func (s *PrivyService) getUserByEmail(email string) (*privyUser, error) {
	url := fmt.Sprintf("%s/users/email/address", s.authURL)

	payload := map[string]string{
		"address": email,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
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
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &user, nil
}

// createUserWithEmail creates a new Privy user with the given email
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
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
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
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(importResp.Results) == 0 || !importResp.Results[0].Success {
		return nil, fmt.Errorf("user import failed")
	}

	// Return a user object with the new ID
	return &privyUser{
		ID: importResp.Results[0].ID,
		LinkedAccounts: []linkedAccount{
			{Type: "email", Address: email},
		},
	}, nil
}

// createWalletForUser creates an ethereum wallet for a Privy user
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
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
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
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return walletResp.Address, nil
}

// extractEthereumWallet extracts the ethereum wallet address from a user's linked accounts
func (s *PrivyService) extractEthereumWallet(user *privyUser) string {
	for _, account := range user.LinkedAccounts {
		if account.Type == "wallet" && account.ChainType == "ethereum" {
			return account.Address
		}
	}
	return ""
}

// setAuthHeaders sets the required authentication headers for Privy API requests
func (s *PrivyService) setAuthHeaders(req *http.Request) {
	// Basic auth: base64(appID:appSecret)
	credentials := base64.StdEncoding.EncodeToString([]byte(s.appID + ":" + s.appSecret))
	req.Header.Set("Authorization", "Basic "+credentials)
	req.Header.Set("privy-app-id", s.appID)
	req.Header.Set("Content-Type", "application/json")
}
