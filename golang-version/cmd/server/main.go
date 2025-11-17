package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

	// X402 verifier
	x402Verifier := services.NewX402Verifier(cfg.FacilitatorURL)

	// Base payment service
	basePaymentService, err := services.NewBasePaymentService(cfg.BaseNetwork, cfg.BaseProxyPrivateKey)
	if err != nil {
		log.Fatalf("Failed to initialize Base payment service: %v", err)
	}
	defer basePaymentService.Close()

	// Payment intent service
	paymentIntentService := services.NewPaymentIntentService(
		db,
		basePaymentService,
		x402Verifier,
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
		// Payment intents endpoints
		api.POST("/intents", paymentIntentsHandler.CreateIntent)
		api.GET("/intents", paymentIntentsHandler.GetIntent)
		api.POST("/intents/:id/solana-proof", paymentIntentsHandler.SubmitSolanaProof)
		api.POST("/intents/:id/trigger-base-payment", paymentIntentsHandler.TriggerBasePayment)
		api.GET("/intents/:id/receipt", paymentIntentsHandler.GetReceipt)
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
