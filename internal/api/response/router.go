package response

import (
	"context"

	"github.com/Conversly/lightning-response/internal/config"
	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RegisterRoutes registers the /response endpoint at the root level
func RegisterRoutes(router *gin.Engine, db *loaders.PostgresClient, cfg *config.Config) {
	ctx := context.Background()

	_ = utils.GetApiKeyManager().LoadFromDatabase(ctx, db)

	emb, err := embedder.NewGeminiEmbedder(cfg.GeminiAPIKeys)
	if err != nil {
		utils.Zlog.Error("failed to create embedder", zap.Error(err))
	}

	// Wire service
	svc := NewGraphService(db, cfg, emb)
	_ = svc.Initialize(ctx)

	// Controller
	ctrl := NewController(svc)
	router.POST("/response", ctrl.Respond)
}
