package routes

import (
	"github.com/Conversly/lightning-response/internal/api/feedback"
	"github.com/Conversly/lightning-response/internal/api/response"
	"github.com/Conversly/lightning-response/internal/config"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/gin-gonic/gin"
)

// SetupRoutes configures all application routes
func SetupRoutes(router *gin.Engine, db *loaders.PostgresClient, cfg *config.Config) {
	// Middleware is already applied in main.go
	// Setup route groups
	SetupHealthRoutes(router, db)
	response.RegisterRoutes(router, db, cfg)
	feedback.RegisterRoutes(router, db, cfg)
	Setup404Handler(router)
}
