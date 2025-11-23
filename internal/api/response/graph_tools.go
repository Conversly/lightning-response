package response

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/tools"
	internalUtils "github.com/Conversly/lightning-response/internal/utils"
)

type HTTPToolRequest struct {
	URL     string            `json:"url" jsonschema:"required,description=The API endpoint URL"`
	Method  string            `json:"method" jsonschema:"required,enum=GET,enum=POST,enum=PUT,enum=DELETE,description=HTTP method"`
	Headers map[string]string `json:"headers,omitempty" jsonschema:"description=HTTP headers"`
	Body    string            `json:"body,omitempty" jsonschema:"description=Request body (for POST/PUT)"`
}

func GetEnabledTools(ctx context.Context, cfg *ChatbotConfig, deps *GraphDependencies) ([]tool.InvokableTool, error) {
	enabledTools := make([]tool.InvokableTool, 0)

	for _, toolName := range cfg.ToolConfigs {
		switch toolName {
		case "rag":
			// Create RAG tool
			ragTool := tools.NewRAGTool(
				deps.DB,
				deps.Embedder,
				cfg.ChatbotID,
				int(cfg.TopK),
			)
			enabledTools = append(enabledTools, ragTool)
			internalUtils.Zlog.Info("Registered RAG tool",
				zap.String("chatbot_id", cfg.ChatbotID),
				zap.Int("topK", int(cfg.TopK)))
		default:
			internalUtils.Zlog.Warn("Unknown tool configuration",
				zap.String("chatbot_id", cfg.ChatbotID),
				zap.String("tool", toolName))
		}
	}

	// Load custom actions from chatbot configuration
	if len(cfg.CustomActions) > 0 {
		internalUtils.Zlog.Info("Loading custom actions",
			zap.String("chatbot_id", cfg.ChatbotID),
			zap.Int("action_count", len(cfg.CustomActions)))

		for _, action := range cfg.CustomActions {
			// Create custom action tool
			actionTool, err := tools.NewCustomActionTool(&action)
			if err != nil {
				internalUtils.Zlog.Error("Failed to create custom action tool",
					zap.String("chatbot_id", cfg.ChatbotID),
					zap.String("action_name", action.Name),
					zap.Error(err))
				continue
			}

			enabledTools = append(enabledTools, actionTool)
			internalUtils.Zlog.Info("Registered custom action tool",
				zap.String("chatbot_id", cfg.ChatbotID),
				zap.String("action_name", action.Name),
				zap.String("action_display_name", action.DisplayName))
		}
	}

	internalUtils.Zlog.Info("Enabled tools for chatbot",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int("tool_count", len(enabledTools)))

	return enabledTools, nil
}
