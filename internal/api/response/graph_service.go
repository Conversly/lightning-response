package response

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/config"
	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/utils"
)

type GraphService struct {
	db       *loaders.PostgresClient
	cfg      *config.Config
	embedder *embedder.GeminiEmbedder
}

func NewGraphService(db *loaders.PostgresClient, cfg *config.Config, embedder *embedder.GeminiEmbedder) *GraphService {
	return &GraphService{
		db:       db,
		cfg:      cfg,
		embedder: embedder,
	}
}

func (s *GraphService) Initialize(ctx context.Context) error {
	// No initialization needed - caching removed
	utils.Zlog.Info("Graph service initialized (no caching)")
	return nil
}

// errorResponse creates a failed Response with the given error
func errorResponse(err error) (*Response, error) {
	return &Response{
		Response:  "",
		Citations: []string{},
		Success:   false,
	}, err
}

func (s *GraphService) BuildAndRunGraph(ctx context.Context, req *Request) (*Response, error) {
	startTime := time.Now()

	utils.Zlog.Info("Processing request with graph",
		zap.String("web_id", req.User.ConverslyWebID),
		zap.String("client_id", req.User.UniqueClientID))

	chatbotID, err := ValidateChatbotAccess(ctx, req.User.ConverslyWebID, req.Metadata.OriginURL)
	if err != nil {
		return errorResponse(fmt.Errorf("chatbot validation failed: %w", err))
	}

	info, err := s.db.GetChatbotInfo(ctx, chatbotID)
	if err != nil {
		return errorResponse(fmt.Errorf("failed to load chatbot config: %w", err))
	}

	cfg := &ChatbotConfig{
		ChatbotID:     fmt.Sprintf("%d", info.ID),
		SystemPrompt:  info.SystemPrompt,
		Temperature:   0.7,
		Model:         "gemini-2.0-flash-exp",
		MaxTokens:     1024,
		TopK:          5,
		ToolConfigs:   []string{"rag"},
		GeminiAPIKeys: s.cfg.GeminiAPIKeys,
	}

	deps := &GraphDependencies{
		DB:       s.db,
		Embedder: s.embedder,
	}

	compiledGraph, err := BuildChatbotGraph(ctx, cfg, deps)
	if err != nil {
		return errorResponse(fmt.Errorf("failed to build chatbot graph: %w", err))
	}

	messages, err := ParseConversationMessages(req.Query)
	if err != nil {
		return errorResponse(fmt.Errorf("failed to parse conversation: %w", err))
	}
	result, citations, err := s.invokeGraph(ctx, compiledGraph, messages, cfg)
	if err != nil {
		return errorResponse(fmt.Errorf("graph execution failed: %w", err))
	}

	// Step 6: Build response
	response := &Response{
		Response:  result.Content,
		Citations: citations,
		Success:   true,
	}

	// Step 7: Save messages in background (non-blocking)
	go func() {
		saveCtx := context.Background()

		if err := SaveConversationMessagesBackground(saveCtx, s.db, MessageRecord{
			UniqueClientID: req.User.UniqueClientID,
			ChatbotID:      info.ID,
			Message:        ExtractLastUserContent(req.Query),
			Role:           "user",
			Citations:      []string{},
		}, MessageRecord{
			UniqueClientID: req.User.UniqueClientID,
			ChatbotID:      info.ID,
			Message:        response.Response,
			Role:           "assistant",
			Citations:      response.Citations,
		}); err != nil {
			utils.Zlog.Error("Failed to save messages in background", zap.Error(err))
		}
	}()

	latencyMS := time.Since(startTime).Milliseconds()
	utils.Zlog.Info("Request completed",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int64("latency_ms", latencyMS),
		zap.Bool("success", response.Success))

	return response, nil
}

// invokeGraph executes the compiled graph with runtime configuration
func (s *GraphService) invokeGraph(
	ctx context.Context,
	graph compose.Runnable[[]*schema.Message, *schema.Message],
	messages []*schema.Message,
	cfg *ChatbotConfig,
) (*schema.Message, []string, error) {
	utils.Zlog.Debug("Invoking graph",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int("message_count", len(messages)))

	result, err := graph.Invoke(ctx, messages)
	if err != nil {
		return nil, nil, fmt.Errorf("graph invocation failed: %w", err)
	}

	// Parse structured citations suffix if present and strip it from content
	citations := extractCitations(result)

	utils.Zlog.Debug("Graph execution completed",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int("citations", len(citations)))

	return result, citations, nil
}

// extractCitations extracts citation URLs from the message
func extractCitations(msg *schema.Message) []string {
	// Look for structured suffix appended by the graph: <<<CITATIONS>>>[...]<<<END>>>
	const startTag = "<<<CITATIONS>>>"
	const endTag = "<<<END>>>"

	content := msg.Content
	start := strings.LastIndex(content, startTag)
	end := strings.LastIndex(content, endTag)
	if start == -1 || end == -1 || end <= start {
		return []string{}
	}

	jsonPart := content[start+len(startTag) : end]
	var citations []string
	if err := json.Unmarshal([]byte(jsonPart), &citations); err != nil {
		return []string{}
	}

	// Strip the suffix from the content
	msg.Content = strings.TrimSpace(content[:start])
	return citations
}
