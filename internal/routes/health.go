package routes

import (
	"net/http"

	"github.com/Conversly/lightning-response/internal/controllers"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/gin-gonic/gin"
)

// SetupHealthRoutes configures health check endpoints
func SetupHealthRoutes(router *gin.Engine, db *loaders.PostgresClient) {
	healthController := controllers.NewHealthController(db)

	// Root endpoint
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	// Health check endpoint
	router.GET("/health", healthController.HealthCheck)
}
