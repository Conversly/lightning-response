package routes

import (
	"github.com/Conversly/lightning-response/internal/controllers"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/gin-gonic/gin"
)

// SetupHealthRoutes configures health check endpoints
func SetupHealthRoutes(router *gin.Engine, db *loaders.PostgresClient) {
	healthController := controllers.NewHealthController(db)

	router.GET("/health", healthController.HealthCheck)
	router.GET("/health/live", healthController.Liveness)
	router.GET("/health/ready", healthController.Readiness)
}
