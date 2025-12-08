package handlers

import (
	"net/http"
	"runtime"

	"github.com/agent-tech/x402-api-backend/internal/database"
	"github.com/agent-tech/x402-api-backend/internal/dto"
	"github.com/gin-gonic/gin"
)

// HealthHandler handles health check requests
type HealthHandler struct{}

// NewHealthHandler creates a new health handler
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// Check handles GET /health
func (h *HealthHandler) Check(c *gin.Context) {
	info := make(map[string]interface{})

	// Check database health
	dbStatus := "up"
	if err := database.Ping(); err != nil {
		dbStatus = "down"
	}

	info["database"] = map[string]string{
		"status": dbStatus,
	}

	// Check memory
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	memStatus := "up"
	// Consider memory as "down" if we're using more than 1GB
	if m.Alloc > 1024*1024*1024 {
		memStatus = "degraded"
	}

	info["memory"] = map[string]interface{}{
		"status":    memStatus,
		"alloc_mb":  m.Alloc / 1024 / 1024,
		"sys_mb":    m.Sys / 1024 / 1024,
	}

	// Determine overall status
	status := "ok"
	if dbStatus == "down" {
		status = "error"
	} else if memStatus == "degraded" {
		status = "degraded"
	}

	c.JSON(http.StatusOK, dto.HealthResponse{
		Status: status,
		Info:   info,
	})
}
