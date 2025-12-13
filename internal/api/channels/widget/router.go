package widget

import (
	"context"

	"github.com/Conversly/lightning-response/internal/config"
	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RegisterRoutes registers the widget /response endpoints
func RegisterRoutes(router *gin.Engine, db *loaders.PostgresClient, cfg *config.Config) {
	ctx := context.Background()
	emb, err := embedder.NewGeminiEmbedder(cfg.GeminiAPIKeys)
	if err != nil {
		utils.Zlog.Error("failed to create embedder", zap.Error(err))
	}

	// Create service
	svc := NewGraphService(db, cfg, emb)
	_ = svc.Initialize(ctx)

	// Create controller
	ctrl := NewController(svc)

	// Register routes
	router.POST("/response", ctrl.Respond)
	router.POST("/playground/response", ctrl.PlaygroundResponse)
}

