package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	dbmigrations "github.com/agent-tech/x402-api-backend/db"
	"github.com/agent-tech/x402-api-backend/internal/config"
	"github.com/agent-tech/x402-api-backend/internal/database"
	"github.com/agent-tech/x402-api-backend/internal/handlers"
	"github.com/agent-tech/x402-api-backend/internal/middleware"
	"github.com/agent-tech/x402-api-backend/internal/services"
	"github.com/agent-tech/x402-api-backend/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	log "github.com/sirupsen/logrus"
)

func main() {
	// Set up embedded migrations for the database package
	database.MigrationsFS = dbmigrations.Migrations
	database.MigrationsDir = "migrations"

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Setup logger
	cfg.SetupLogger()

	log.WithFields(log.Fields{
		"port":    cfg.Port,
		"env":     cfg.NodeEnv,
		"network": cfg.BaseNetwork,
	}).Info("Starting X402 API Backend")

	// Connect to database
	db, err := database.Connect(cfg.DatabaseURL, cfg.LogLevel)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// Initialize services
	log.Info("Initializing services...")

	// X402 verifier (supports multiple source chains)
	x402Verifier := services.NewX402Verifier(
		cfg.FacilitatorURL,
		cfg.SolanaNetwork,
		cfg.BaseSourceNetwork,
		cfg.BSCNetwork,
	)

	// Base payment service
	basePaymentService, err := services.NewBasePaymentService(cfg.BaseNetwork, cfg.BaseProxyPrivateKey)
	if err != nil {
		log.Fatalf("Failed to initialize Base payment service: %v", err)
	}
	defer basePaymentService.Close()

	// Privy service for email-to-wallet mapping
	privyService := services.NewPrivyService(db.Queries, cfg.PrivyAppID, cfg.PrivyAppSecret)

	// Payment intent service
	paymentIntentService := services.NewPaymentIntentService(
		db.Queries,
		basePaymentService,
		x402Verifier,
		privyService,
		cfg.SolanaNetwork,
		cfg.BaseNetwork,
	)

	// Initialize handlers
	paymentIntentsHandler := handlers.NewPaymentIntentsHandler(paymentIntentService)
	healthHandler := handlers.NewHealthHandler()

	// Setup Gin router
	if cfg.NodeEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Register custom validators
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterValidation("eth_addr", utils.ValidateEthAddress)
	}

	// Global middleware
	router.Use(middleware.ErrorHandler())
	router.Use(middleware.Logger())

	// CORS middleware
	router.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		for _, allowedOrigin := range cfg.CORSOrigins {
			if origin == allowedOrigin || allowedOrigin == "*" {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
				c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				break
			}
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// Health check endpoint (not under API prefix)
	router.GET("/health", healthHandler.Check)

	// API routes
	api := router.Group("/" + cfg.APIPrefix)
	{
		// POST /intents - Create intent with email or wallet address as receiver
		api.POST("/intents", paymentIntentsHandler.CreateIntent)
		// POST /intents/:intent_id - Submit X402 proof for existing intent
		api.POST("/intents/:intent_id", paymentIntentsHandler.SubmitProof)
		// GET /intents?intent_id={id} - Get combined status + receipt
		api.GET("/intents", paymentIntentsHandler.GetIntent)
	}

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Port)

	log.WithFields(log.Fields{
		"port":       cfg.Port,
		"api_prefix": cfg.APIPrefix,
	}).Info("Server starting...")

	fmt.Printf("\n🚀 Application is running on: http://localhost:%d\n", cfg.Port)
	fmt.Printf("❤️  Health check available at: http://localhost:%d/health\n", cfg.Port)
	fmt.Printf("📚 API base URL: http://localhost:%d/%s\n\n", cfg.Port, cfg.APIPrefix)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Info("Shutting down server...")

		// Cleanup
		basePaymentService.Close()
		database.Close()

		os.Exit(0)
	}()

	// Run server
	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
