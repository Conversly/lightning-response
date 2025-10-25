package controllers

import (
	"context"
	"net/http"
	"time"

	"github.com/Conversly/db-ingestor/internal/loaders"
	"github.com/Conversly/db-ingestor/internal/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type HealthController struct {
	db *loaders.PostgresClient
}

func NewHealthController(db *loaders.PostgresClient) *HealthController {
	return &HealthController{db: db}
}

// HealthCheck godoc
// @Summary Check application health
// @Description Check if the application and database are healthy
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /health [get]
func (h *HealthController) HealthCheck(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool := h.db.GetPool()
	if err := pool.Ping(ctx); err != nil {
		utils.Zlog.Error("Database health check failed", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":    "unhealthy",
			"database":  "down",
			"timestamp": time.Now().UTC(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"database":  "up",
		"timestamp": time.Now().UTC(),
	})
}

// Liveness godoc
// @Summary Liveness probe
// @Description Check if the application is alive
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /health/live [get]
func (h *HealthController) Liveness(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "alive",
		"timestamp": time.Now().UTC(),
	})
}

// Readiness godoc
// @Summary Readiness probe
// @Description Check if the application is ready to serve traffic
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /health/ready [get]
func (h *HealthController) Readiness(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool := h.db.GetPool()
	if err := pool.Ping(ctx); err != nil {
		utils.Zlog.Error("Readiness check failed", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":    "not ready",
			"database":  "down",
			"timestamp": time.Now().UTC(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "ready",
		"database":  "up",
		"timestamp": time.Now().UTC(),
	})
}
