package response

import (
	"context"

	"github.com/Conversly/lightning-response/internal/config"
	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/utils"
	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers the /response endpoint at the root level
func RegisterRoutes(router *gin.Engine, db *loaders.PostgresClient, cfg *config.Config) {
	ctx := context.Background()

	// Load API key/domain map into memory (used for access validation)
	_ = utils.GetApiKeyManager().LoadFromDatabase(ctx, db)

	// Create embedder for RAG retriever
	emb, err := embedder.NewGeminiEmbedder(cfg.GeminiAPIKeys)
	if err != nil {
		// If embedder fails, we still register the route, but requests will fail at runtime
	}

	// Wire service
	svc := NewGraphService(db, cfg, emb)
	_ = svc.Initialize(ctx)

	// Controller
	ctrl := NewController(svc)
	router.POST("/response", ctrl.Respond)
}
