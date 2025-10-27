package response

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino/components/model"
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
	return InitializeGraphEngine(ctx, s.db, s.embedder)
}

// BuildAndRunGraph is the main entry point for processing a request using Eino graphs
func (s *GraphService) BuildAndRunGraph(ctx context.Context, req *Request) (*Response, error) {
	startTime := time.Now()

	utils.Zlog.Info("Processing request with graph",
		zap.String("web_id", req.User.ConverslyWebID),
		zap.String("client_id", req.User.UniqueClientID))

	chatbotID, err := ValidateChatbotAccess(ctx, s.db, req.User.ConverslyWebID, req.Metadata.OriginURL)
	if err != nil {
		return &Response{
			Response:  "",
			Citations: []string{},
			Success:   false,
		}, fmt.Errorf("chatbot validation failed: %w", err)
	}

	// Step 2: Load chatbot configuration using the validated chatbot ID from API key manager
	info, err := s.db.GetChatbotInfo(ctx, chatbotID)
	if err != nil {
		return &Response{
			Response:  "",
			Citations: []string{},
			Success:   false,
		}, fmt.Errorf("failed to load chatbot config: %w", err)
	}

	// Build runtime chatbot config (tools to include RAG only for now)
	cfg := &ChatbotConfig{
		ChatbotID:     fmt.Sprintf("%d", info.ID),
		SystemPrompt:  info.SystemPrompt,
		Temperature:   0.7,
		Model:         "gemini-2.0-flash-exp",
		MaxTokens:     1024,
		TopK:          5,
		ToolConfigs:   []string{"rag"},
		GeminiAPIKeys: s.cfg.GeminiAPIKeys, // Pass API keys from config
	}

	// Step 3: Get or create the compiled graph for this chatbot
	compiledGraph, err := GetOrCreateChatbotGraph(ctx, cfg)
	if err != nil {
		return &Response{
			Response:  "",
			Citations: []string{},
			Success:   false,
		}, fmt.Errorf("failed to get chatbot graph: %w", err)
	}

	// Step 4: Parse conversation history from request query field
	// The query field contains the whole JSON array of previous conversation
	messages, err := ParseConversationMessages(req.Query)
	if err != nil {
		return &Response{
			Response:  "",
			Citations: []string{},
			Success:   false,
		}, fmt.Errorf("failed to parse conversation: %w", err)
	}

	// Step 5: Execute the graph with runtime options
	result, citations, err := s.invokeGraph(ctx, compiledGraph, messages, cfg)
	if err != nil {
		return &Response{
			Response:  "",
			Citations: []string{},
			Success:   false,
		}, fmt.Errorf("graph execution failed: %w", err)
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
			Message:        req.Query,
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

	// Invoke graph with Gemini-specific runtime options
	result, err := graph.Invoke(ctx, messages,
		// Common options
		compose.WithChatModelOption(model.WithTemperature(cfg.Temperature)),
		compose.WithChatModelOption(model.WithMaxTokens(cfg.MaxTokens)),

		// Gemini-specific options
		compose.WithChatModelOption(gemini.WithTopK(cfg.TopK)),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("graph invocation failed: %w", err)
	}

	// Extract citations from the result
	citations := extractCitations(result)

	utils.Zlog.Debug("Graph execution completed",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int("citations", len(citations)))

	return result, citations, nil
}

// extractCitations extracts citation URLs from the message
func extractCitations(msg *schema.Message) []string {
	citations := []string{}

	// Best-effort: attempt to pull citations if embedded as plain text tags like [1], [2] with URLs
	// Eino's ResponseMeta type is provider-specific; skip structured extraction for now

	return citations
}
