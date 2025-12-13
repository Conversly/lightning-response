package whatsapp

import (
	"github.com/Conversly/lightning-response/internal/config"
	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/utils"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RegisterRoutes registers the WhatsApp webhook endpoints
func RegisterRoutes(router *gin.Engine, db *loaders.PostgresClient, cfg *config.Config) {
	emb, err := embedder.NewGeminiEmbedder(cfg.GeminiAPIKeys)
	if err != nil {
		utils.Zlog.Error("failed to create WhatsApp embedder", zap.Error(err))
	}

	adapter := NewAdapter(db, cfg, emb)
	service := NewService(db, cfg, emb)
	ctrl := NewController(adapter, service)

	// WhatsApp webhook endpoints
	whatsapp := router.Group("/whatsapp")
	{
		// Meta sends GET for verification, POST for messages
		whatsapp.GET("/webhook/:chatbotId", ctrl.VerifyWebhook)
		whatsapp.POST("/webhook/:chatbotId", ctrl.Webhook)
	}

	utils.Zlog.Info("WhatsApp routes registered",
		zap.String("verify_endpoint", "/whatsapp/webhook/:chatbotId [GET]"),
		zap.String("webhook_endpoint", "/whatsapp/webhook/:chatbotId [POST]"))
}
