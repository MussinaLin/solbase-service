package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agent-tech/x402-api-backend/database"
	"github.com/agent-tech/x402-api-backend/internal/api"
	"github.com/agent-tech/x402-api-backend/internal/api/middleware"
	"github.com/agent-tech/x402-api-backend/internal/config"
	paymentrepo "github.com/agent-tech/x402-api-backend/internal/payment/repository/sqlc"
	paymentsvc "github.com/agent-tech/x402-api-backend/internal/payment/service"
	"github.com/agent-tech/x402-api-backend/internal/storage"
	log "github.com/sirupsen/logrus"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	storage.MigrationsFS = database.Migrations
	storage.MigrationsDir = "migrations"

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	cfg.SetupLogger()

	log.WithFields(log.Fields{
		"port":    cfg.Port,
		"env":     cfg.NodeEnv,
		"network": cfg.BaseNetwork,
	}).Info("Starting X402 API Backend")

	db, err := storage.Connect(cfg.DatabaseURL, cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer storage.Close()

	log.Info("Initializing services...")

	paymentRepo := paymentrepo.New(db.Queries)
	emailWalletRepo := paymentrepo.NewEmailWalletRepository(db.Queries)

	x402Verifier := paymentsvc.NewX402Verifier(
		cfg.FacilitatorURL,
		cfg.SolanaNetwork,
		cfg.BaseSourceNetwork,
		cfg.BSCNetwork,
	)

	basePaymentService, err := paymentsvc.NewBasePaymentService(cfg.BaseNetwork, cfg.BaseProxyPrivateKey)
	if err != nil {
		return fmt.Errorf("initialize Base payment service: %w", err)
	}
	defer basePaymentService.Close()

	privyService := paymentsvc.NewPrivyService(emailWalletRepo, cfg.PrivyAppID, cfg.PrivyAppSecret)

	paymentSvc := paymentsvc.New(
		paymentRepo,
		basePaymentService,
		x402Verifier,
		privyService,
		cfg.SolanaNetwork,
		cfg.BaseNetwork,
		cfg.BaseSourceNetwork,
		cfg.BSCNetwork,
		cfg.SolanaReceiverAddress,
		cfg.BSCReceiverAddress,
	)

	// Setup reCAPTCHA configuration
	var recaptchaConfig *middleware.RecaptchaConfig
	if cfg.RecaptchaSecretKey != "" {
		recaptchaConfig = &middleware.RecaptchaConfig{
			SecretKey:    cfg.RecaptchaSecretKey,
			MinScore:     cfg.RecaptchaMinScore,
			VerifyURL:    cfg.RecaptchaVerifyURL,
			EnabledPaths: cfg.RecaptchaEnabledPaths,
			SkipOnError:  cfg.RecaptchaSkipOnError,
		}
		log.WithFields(log.Fields{
			"enabled_paths": cfg.RecaptchaEnabledPaths,
			"min_score":     cfg.RecaptchaMinScore,
		}).Info("reCAPTCHA middleware enabled")
	} else {
		log.Info("reCAPTCHA middleware disabled (no secret key configured)")
	}

	router := api.NewRouter(paymentSvc, cfg.CORSOrigins, cfg.APIPrefix, recaptchaConfig)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Info("Shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.WithError(err).Error("Server shutdown error")
		}

		basePaymentService.Close()
		storage.Close()
	}()

	log.WithFields(log.Fields{
		"port":       cfg.Port,
		"api_prefix": cfg.APIPrefix,
	}).Info("Server starting...")

	fmt.Printf("\n🚀 Application is running on: http://localhost:%d\n", cfg.Port)
	fmt.Printf("❤️  Health check available at: http://localhost:%d/health\n", cfg.Port)
	fmt.Printf("📚 API base URL: http://localhost:%d/%s\n\n", cfg.Port, cfg.APIPrefix)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}
