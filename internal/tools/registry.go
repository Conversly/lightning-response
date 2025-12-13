package tools

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/types"
	"github.com/Conversly/lightning-response/internal/utils"
)

// GetEnabledTools returns all enabled tools for a chatbot
func GetEnabledTools(
	ctx context.Context,
	chatbotID string,
	topK int32,
	customActions []types.CustomAction,
	db *loaders.PostgresClient,
	embedder *embedder.GeminiEmbedder,
) ([]tool.InvokableTool, error) {
	enabledTools := make([]tool.InvokableTool, 0)

	// Always enable RAG tool
	ragTool := NewRAGTool(db, embedder, chatbotID, int(topK))
	enabledTools = append(enabledTools, ragTool)
	utils.Zlog.Info("Registered RAG tool",
		zap.String("chatbot_id", chatbotID),
		zap.Int("topK", int(topK)))

	// Load custom actions from chatbot configuration
	if len(customActions) > 0 {
		utils.Zlog.Info("Loading custom actions",
			zap.String("chatbot_id", chatbotID),
			zap.Int("action_count", len(customActions)))

		for _, action := range customActions {
			// Create custom action tool
			actionTool, err := NewCustomActionTool(&action)
			if err != nil {
				utils.Zlog.Error("Failed to create custom action tool",
					zap.String("chatbot_id", chatbotID),
					zap.String("action_name", action.Name),
					zap.Error(err))
				continue
			}

			enabledTools = append(enabledTools, actionTool)
			utils.Zlog.Info("Registered custom action tool",
				zap.String("chatbot_id", chatbotID),
				zap.String("action_name", action.Name),
				zap.String("action_display_name", action.DisplayName))
		}
	}

	utils.Zlog.Info("Enabled tools for chatbot",
		zap.String("chatbot_id", chatbotID),
		zap.Int("tool_count", len(enabledTools)))

	return enabledTools, nil
}
