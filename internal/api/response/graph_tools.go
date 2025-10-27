package response

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
	"go.uber.org/zap"

	internalUtils "github.com/Conversly/lightning-response/internal/utils"
)

type HTTPToolRequest struct {
	URL     string            `json:"url" jsonschema:"required,description=The API endpoint URL"`
	Method  string            `json:"method" jsonschema:"required,enum=GET,enum=POST,enum=PUT,enum=DELETE,description=HTTP method"`
	Headers map[string]string `json:"headers,omitempty" jsonschema:"description=HTTP headers"`
	Body    string            `json:"body,omitempty" jsonschema:"description=Request body (for POST/PUT)"`
}

func GetEnabledTools(ctx context.Context, cfg *ChatbotConfig) ([]tool.InvokableTool, error) {
	internalUtils.Zlog.Info("Enabled tools for chatbot",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int("tool_count", 0))
	return []tool.InvokableTool{}, nil
}
