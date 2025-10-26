package routes

import (
	"github.com/Conversly/lightning-response/internal/config"
	"github.com/Conversly/lightning-response/internal/api/response"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/middleware"
	"github.com/gin-gonic/gin"
)

// SetupRoutes configures all application routes
func SetupRoutes(router *gin.Engine, db *loaders.PostgresClient, cfg *config.Config) {
	// Apply global middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(middleware.CORS())
	router.Use(middleware.RequestID())

	// Setup route groups
	SetupHealthRoutes(router, db)
	SetupAPIRoutes(router, db, cfg)
	response.RegisterRoutes(router, db, cfg)
	SetupRootRoutes(router, cfg)
	Setup404Handler(router)
}
