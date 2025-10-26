package response

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/config"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/utils"
)

// GraphService orchestrates the Eino graph-based response pipeline
// This replaces the old Service with a graph-based approach
type GraphService struct {
	db  *loaders.PostgresClient
	cfg *config.Config
}

// NewGraphService creates a new graph-based service
func NewGraphService(db *loaders.PostgresClient, cfg *config.Config) *GraphService {
	return &GraphService{db: db, cfg: cfg}
}

// Initialize sets up the graph engine (call once at startup)
func (s *GraphService) Initialize(ctx context.Context) error {
	return InitializeGraphEngine(ctx)
}

// ValidateAndResolveTenant validates API key and origin domain
func (s *GraphService) ValidateAndResolveTenant(ctx context.Context, webID string, originURL string) (string, error) {
	return ValidateTenantAccess(ctx, s.db, webID, originURL)
}

// BuildAndRunGraph is the main entry point for processing a request using Eino graphs
func (s *GraphService) BuildAndRunGraph(ctx context.Context, req *Request, tenantID string) (*Response, error) {
	startTime := time.Now()

	utils.Zlog.Info("Processing request with graph",
		zap.String("tenant_id", tenantID),
		zap.String("client_id", req.User.UniqueClientID),
		zap.String("mode", req.Mode))

	// Step 1: Load chatbot configuration
	cfg, err := GetChatbotConfig(ctx, s.db, req.User.ConverslyWebID, req.Metadata.OriginURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load chatbot config: %w", err)
	}

	// Step 2: Get or create the compiled graph for this chatbot
	compiledGraph, err := GetOrCreateTenantGraph(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant graph: %w", err)
	}

	// Step 3: Load conversation history
	conversationHistory, err := LoadConversationHistory(ctx, s.db, req.User.UniqueClientID, cfg.ChatbotID, 20)
	if err != nil {
		utils.Zlog.Warn("Failed to load conversation history, continuing with empty history",
			zap.Error(err))
		conversationHistory = []*schema.Message{}
	}

	// Step 4: Append the current user query
	conversationHistory = append(conversationHistory, schema.UserMessage(req.Query))

	// Step 5: Execute the graph with runtime options
	result, err := s.invokeGraph(ctx, compiledGraph, conversationHistory, cfg)
	if err != nil {
		return nil, fmt.Errorf("graph execution failed: %w", err)
	}

	// Step 6: Save the conversation to database
	if err := SaveConversationMessages(ctx, s.db, req, cfg.ChatbotID, result); err != nil {
		utils.Zlog.Error("Failed to save conversation", zap.Error(err))
		// Don't fail the request if saving fails
	}

	// Step 7: Convert to response format
	response := MessageToResponse(result, req)
	
	// Add usage/latency metrics
	latencyMS := time.Since(startTime).Milliseconds()
	if response.Usage == nil {
		response.Usage = &Usage{}
	}
	response.Usage.LatencyMS = latencyMS

	utils.Zlog.Info("Request completed",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int64("latency_ms", latencyMS))

	return response, nil
}

// invokeGraph executes the compiled graph with runtime configuration
func (s *GraphService) invokeGraph(
	ctx context.Context,
	graph compose.Runnable[[]*schema.Message, *schema.Message],
	messages []*schema.Message,
	cfg *ChatbotConfig,
) (*schema.Message, error) {
	utils.Zlog.Debug("Invoking graph",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int("message_count", len(messages)))

	// TODO: Add runtime options when ChatModel is implemented
	// Options examples:
	// - compose.WithChatModelOption(model.WithTemperature(cfg.Temperature))
	// - compose.WithChatModelOption(model.WithMaxTokens(cfg.MaxTokens))
	// - compose.WithCallbacks(createLoggingHandler(cfg.ChatbotID))

	result, err := graph.Invoke(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("graph invocation failed: %w", err)
	}

	return result, nil
}

// GetCacheStats returns statistics about cached graphs (for monitoring)
func (s *GraphService) GetCacheStats() map[string]interface{} {
	return map[string]interface{}{
		"cached_graphs": GetCachedGraphCount(),
	}
}

// ClearCache clears a specific chatbot's cache or all caches
func (s *GraphService) ClearCache(chatbotID string) {
	if chatbotID == "" {
		ClearAllGraphCaches()
	} else {
		ClearGraphCache(chatbotID)
	}
}
