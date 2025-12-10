package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
)

type Config struct {
	// Server
	Port      int
	NodeEnv   string
	APIPrefix string

	// Database
	DatabaseURL string

	// Solana
	SolanaReceiverAddress string
	SolanaNetwork         string

	// Base Chain
	BaseNetwork         string
	BaseSourceNetwork   string // Network for Base as payer chain
	BaseProxyPrivateKey string

	// BSC Chain
	BSCNetwork string

	// CORS
	CORSOrigins []string

	// X402
	FacilitatorURL string

	// Privy
	PrivyAppID     string
	PrivyAppSecret string

	// Logging
	LogLevel string

	// Rate Limiting
	RateLimitTTL int
	RateLimitMax int
}

// Load loads configuration from environment variables and .env file
func Load() (*Config, error) {
	// Try to load .env file (ignore error if not found)
	_ = godotenv.Load()

	cfg := &Config{
		Port:                  getEnvAsInt("PORT", 3001),
		NodeEnv:               getEnv("NODE_ENV", "development"),
		APIPrefix:             getEnv("API_PREFIX", "api"),
		DatabaseURL:           getEnv("DATABASE_URL", ""),
		SolanaReceiverAddress: getEnv("SOLANA_RECEIVER_ADDRESS", ""),
		SolanaNetwork:         getEnv("SOLANA_NETWORK", "solana-devnet"),
		BaseNetwork:           getEnv("BASE_NETWORK", "base-sepolia"),
		BaseSourceNetwork:     getEnv("BASE_SOURCE_NETWORK", "base-sepolia"),
		BaseProxyPrivateKey:   getEnv("BASE_PROXY_PRIVATE_KEY", ""),
		BSCNetwork:            getEnv("BSC_NETWORK", "bsc-testnet"),
		CORSOrigins:           getEnvAsSlice("CORS_ORIGINS", []string{"http://localhost:3000"}),
		FacilitatorURL:        getEnv("FACILITATOR_URL", "https://x402.org/facilitator"),
		PrivyAppID:            getEnv("PRIVY_APP_ID", ""),
		PrivyAppSecret:        getEnv("PRIVY_APP_SECRET", ""),
		LogLevel:              getEnv("LOG_LEVEL", "debug"),
		RateLimitTTL:          getEnvAsInt("RATE_LIMIT_TTL", 60),
		RateLimitMax:          getEnvAsInt("RATE_LIMIT_MAX", 10),
	}

	// Validate required fields
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks that all required configuration is present
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	if c.SolanaReceiverAddress == "" {
		return fmt.Errorf("SOLANA_RECEIVER_ADDRESS is required")
	}

	if c.BaseProxyPrivateKey == "" {
		return fmt.Errorf("BASE_PROXY_PRIVATE_KEY is required")
	}

	// Validate private key format (0x + 64 hex characters)
	if !strings.HasPrefix(c.BaseProxyPrivateKey, "0x") || len(c.BaseProxyPrivateKey) != 66 {
		return fmt.Errorf("BASE_PROXY_PRIVATE_KEY must be a valid hex private key (0x + 64 hex chars)")
	}

	// Validate networks
	validSolanaNetworks := map[string]bool{
		"solana-devnet":       true,
		"solana-mainnet-beta": true,
	}
	if !validSolanaNetworks[c.SolanaNetwork] {
		return fmt.Errorf("SOLANA_NETWORK must be one of: solana-devnet, solana-mainnet-beta")
	}

	validBaseNetworks := map[string]bool{
		"base-sepolia": true,
		"base":         true,
	}
	if !validBaseNetworks[c.BaseNetwork] {
		return fmt.Errorf("BASE_NETWORK must be one of: base-sepolia, base")
	}
	if !validBaseNetworks[c.BaseSourceNetwork] {
		return fmt.Errorf("BASE_SOURCE_NETWORK must be one of: base-sepolia, base")
	}

	validBSCNetworks := map[string]bool{
		"bsc-testnet": true,
		"bsc":         true,
	}
	if !validBSCNetworks[c.BSCNetwork] {
		return fmt.Errorf("BSC_NETWORK must be one of: bsc-testnet, bsc")
	}

	// Validate Privy credentials
	if c.PrivyAppID == "" {
		return fmt.Errorf("PRIVY_APP_ID is required")
	}
	if c.PrivyAppSecret == "" {
		return fmt.Errorf("PRIVY_APP_SECRET is required")
	}

	return nil
}

// SetupLogger configures the global logger based on config
func (c *Config) SetupLogger() {
	// Set log level
	level, err := log.ParseLevel(c.LogLevel)
	if err != nil {
		level = log.DebugLevel
	}
	log.SetLevel(level)

	// Set JSON formatter for production
	if c.NodeEnv == "production" {
		log.SetFormatter(&log.JSONFormatter{})
	} else {
		log.SetFormatter(&log.TextFormatter{
			FullTimestamp: true,
			ForceColors:   true,
		})
	}
}

// Helper functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}

func getEnvAsSlice(key string, defaultValue []string) []string {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	return strings.Split(valueStr, ",")
}
