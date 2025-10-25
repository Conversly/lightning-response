package routes

import (
	"net/http"
	"time"

	"github.com/Conversly/db-ingestor/internal/api/ingestion"
	"github.com/Conversly/db-ingestor/internal/config"
	"github.com/Conversly/db-ingestor/internal/controllers"
	"github.com/Conversly/db-ingestor/internal/loaders"
	"github.com/gin-gonic/gin"
)

func SetupAPIRoutes(router *gin.Engine, db *loaders.PostgresClient, cfg *config.Config) {
	v1 := router.Group("/api/v1")
	{
		systemController := controllers.NewSystemController(cfg)
		v1.GET("/status", systemController.Status)
		v1.GET("/info", systemController.Info)

		ingestion.RegisterRoutes(v1, db, cfg)
	}
}

func SetupRootRoutes(router *gin.Engine, cfg *config.Config) {
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message":     "Welcome to " + cfg.ServiceName,
			"version":     "1.0.0",
			"environment": cfg.Environment,
			"timestamp":   time.Now().UTC(),
		})
	})
}

// Setup404Handler configures the 404 handler
func Setup404Handler(router *gin.Engine) {
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "Not Found",
			"message": "The requested resource was not found",
			"path":    c.Request.URL.Path,
		})
	})
}
