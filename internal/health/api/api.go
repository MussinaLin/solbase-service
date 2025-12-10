package api

import (
	"net/http"
	"runtime"

	"github.com/agent-tech/x402-api-backend/internal/httpwrap"
	"github.com/agent-tech/x402-api-backend/internal/storage"
	"github.com/go-chi/chi/v5"
)

// AddRoutes registers health routes.
func AddRoutes(r chi.Router) {
	r.Get("/health", httpwrap.Handler(healthCheck()))
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status string                 `json:"status"`
	Info   map[string]interface{} `json:"info"`
}

func healthCheck() httpwrap.HandlerFunc {
	return func(r *http.Request) (*httpwrap.Response, *httpwrap.ErrorResponse) {
		info := make(map[string]interface{})

		dbErr := storage.Ping()
		if dbErr != nil {
			info["database"] = map[string]interface{}{
				"status": "unhealthy",
				"error":  dbErr.Error(),
			}
		} else {
			info["database"] = map[string]interface{}{
				"status": "healthy",
			}
		}

		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		info["memory"] = map[string]interface{}{
			"alloc_mb":       m.Alloc / 1024 / 1024,
			"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
			"sys_mb":         m.Sys / 1024 / 1024,
			"num_gc":         m.NumGC,
		}

		info["goroutines"] = runtime.NumGoroutine()

		status := "healthy"
		if dbErr != nil {
			status = "unhealthy"
		}

		return &httpwrap.Response{
			StatusCode: http.StatusOK,
			Body: &HealthResponse{
				Status: status,
				Info:   info,
			},
		}, nil
	}
}
