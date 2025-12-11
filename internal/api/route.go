package api

import (
	"github.com/agent-tech/x402-api-backend/internal/api/middleware"
	healthapi "github.com/agent-tech/x402-api-backend/internal/health/api"
	paymentapi "github.com/agent-tech/x402-api-backend/internal/payment/api"
	"github.com/agent-tech/x402-api-backend/internal/payment/service"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// NewRouter creates and configures the chi router.
func NewRouter(paymentSvc *service.Service, corsOrigins []string, apiPrefix string, recaptchaConfig *middleware.RecaptchaConfig) chi.Router {
	r := chi.NewRouter()

	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Logger())
	r.Use(middleware.ErrorHandler())
	r.Use(middleware.CORS(corsOrigins))
	r.Use(chimiddleware.Recoverer)

	// reCAPTCHA middleware (if enabled)
	if recaptchaConfig != nil && recaptchaConfig.SecretKey != "" {
		r.Use(middleware.Recaptcha(*recaptchaConfig))
	}

	healthapi.AddRoutes(r)

	r.Route("/"+apiPrefix, func(r chi.Router) {
		paymentapi.AddRoutes(r, paymentSvc)
	})

	return r
}
