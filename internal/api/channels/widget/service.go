package widget

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/config"
	"github.com/Conversly/lightning-response/internal/core"
	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/rag"
	"github.com/Conversly/lightning-response/internal/types"
	"github.com/Conversly/lightning-response/internal/utils"
)

// GraphService handles widget response generation
type GraphService struct {
	db       *loaders.PostgresClient
	cfg      *config.Config
	embedder *embedder.GeminiEmbedder
}

// NewGraphService creates a new widget graph service
func NewGraphService(db *loaders.PostgresClient, cfg *config.Config, embedder *embedder.GeminiEmbedder) *GraphService {
	return &GraphService{
		db:       db,
		cfg:      cfg,
		embedder: embedder,
	}
}

// Initialize initializes the graph service
func (s *GraphService) Initialize(ctx context.Context) error {
	utils.Zlog.Info("Widget graph service initialized")
	return nil
}

// errorResponse creates a failed Response with the given error
func errorResponse(err error) (*Response, error) {
	return &Response{
		Response:     "",
		BaseResponse: types.BaseResponse{Success: false},
		Citations:    []string{},
	}, err
}

// BuildAndRunGraph processes a widget request
func (s *GraphService) BuildAndRunGraph(ctx context.Context, req *Request) (*Response, error) {
	startTime := time.Now()

	utils.Zlog.Info("Processing widget request with graph",
		zap.String("web_id", req.User.ConverslyWebID),
		zap.String("client_id", req.User.UniqueClientID))

	chatbotID, err := ValidateChatbotAccess(ctx, req.User.ConverslyWebID, req.Metadata.OriginURL)
	if err != nil {
		return errorResponse(fmt.Errorf("chatbot validation failed: %w", err))
	}

	info, err := s.db.GetChatbotInfoWithTopics(ctx, chatbotID)
	if err != nil {
		return errorResponse(fmt.Errorf("failed to load chatbot config: %w", err))
	}

	cfg := &core.ChatbotConfig{
		ChatbotID:     info.ID,
		SystemPrompt:  info.SystemPrompt,
		Temperature:   0.7,
		Model:         "gemini-2.0-flash-lite",
		MaxTokens:     1024,
		TopK:          5,
		ToolConfigs:   []string{"rag"},
		GeminiAPIKeys: s.cfg.GeminiAPIKeys,
		CustomActions: info.CustomActions,
	}

	deps := &core.GraphDependencies{
		DB:       s.db,
		Embedder: s.embedder,
	}

	compiledGraph, err := core.BuildChatbotGraph(ctx, cfg, deps)
	if err != nil {
		return errorResponse(fmt.Errorf("failed to build chatbot graph: %w", err))
	}

	messages, err := core.ParseConversationMessages(req.Query)
	if err != nil {
		return errorResponse(fmt.Errorf("failed to parse conversation: %w", err))
	}

	result, citations, err := s.invokeGraph(ctx, compiledGraph, messages, cfg)
	if err != nil {
		return errorResponse(fmt.Errorf("graph execution failed: %w", err))
	}

	assistantUUID, err := uuid.NewV7()
	if err != nil {
		return errorResponse(fmt.Errorf("failed to generate assistant message id: %w", err))
	}
	assistantMsgID := assistantUUID.String()
	response := &Response{
		Response:     result.Content,
		Citations:    citations,
		BaseResponse: types.BaseResponse{Success: true},
		MessageID:    assistantMsgID,
	}

	// Save messages in background (non-blocking)
	go func() {
		saveCtx := context.Background()
		userUUID, err := uuid.NewV7()
		if err != nil {
			utils.Zlog.Error("Failed to generate user message id", zap.Error(err))
			return
		}
		userMsgID := userUUID.String()
		if err := core.SaveConversationMessagesBackground(saveCtx, s.db, core.MessageRecord{
			UniqueClientID: req.User.UniqueClientID,
			ChatbotID:      info.ID,
			Message:        core.ExtractLastUserContent(req.Query),
			Role:           "user",
			Citations:      []string{},
			MessageUID:     userMsgID,
			Channel:        core.ChannelWidget,
		}, core.MessageRecord{
			UniqueClientID: req.User.UniqueClientID,
			ChatbotID:      info.ID,
			Message:        response.Response,
			Role:           "assistant",
			Citations:      response.Citations,
			MessageUID:     assistantMsgID,
			Channel:        core.ChannelWidget,
		}); err != nil {
			utils.Zlog.Error("Failed to save messages in background", zap.Error(err))
		}
	}()

	latencyMS := time.Since(startTime).Milliseconds()
	utils.Zlog.Info("Widget request completed",
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
	cfg *core.ChatbotConfig,
) (*schema.Message, []string, error) {
	utils.Zlog.Debug("Invoking graph",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int("message_count", len(messages)))

	result, err := graph.Invoke(ctx, messages)
	if err != nil {
		return nil, nil, fmt.Errorf("graph invocation failed: %w", err)
	}

	// Parse structured citations suffix if present and strip it from content
	citations := core.ExtractCitations(result)

	// Fallback: if graph produced no citations, run retriever on last user message
	if len(citations) == 0 {
		var lastUser string
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i] != nil && messages[i].Role == schema.User {
				lastUser = messages[i].Content
				break
			}
		}

		if lastUser != "" {
			retr := rag.NewPgVectorRetriever(s.db, s.embedder, cfg.ChatbotID, int(cfg.TopK))
			docs, err := retr.Retrieve(ctx, lastUser)
			if err != nil {
				utils.Zlog.Debug("fallback retriever failed",
					zap.String("chatbot_id", cfg.ChatbotID),
					zap.Error(err))
			} else {
				for _, d := range docs {
					if d.Citation != nil && *d.Citation != "" {
						citations = append(citations, *d.Citation)
					}
				}
				if len(citations) > 0 {
					utils.Zlog.Debug("Fallback retriever added citations",
						zap.String("chatbot_id", cfg.ChatbotID),
						zap.Int("citations_added", len(citations)))
				}
			}
		}
	}

	utils.Zlog.Debug("Graph execution completed",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int("citations", len(citations)))

	return result, citations, nil
}

// BuildAndRunPlaygroundGraph executes the graph for playground requests (no validation)
func (s *GraphService) BuildAndRunPlaygroundGraph(ctx context.Context, req *PlaygroundRequest) (*Response, error) {
	startTime := time.Now()

	utils.Zlog.Info("Processing playground request with graph",
		zap.String("chatbot_id", req.Chatbot.ChatbotId),
		zap.String("client_id", req.User.UniqueClientID))

	// Load custom actions for playground chatbot
	customActions, err := s.db.GetCustomActionsByChatbot(ctx, req.Chatbot.ChatbotId)
	if err != nil {
		utils.Zlog.Warn("Failed to load custom actions for playground chatbot",
			zap.String("chatbot_id", req.Chatbot.ChatbotId),
			zap.Error(err))
		customActions = []types.CustomAction{}
	}

	cfg := &core.ChatbotConfig{
		ChatbotID:     req.Chatbot.ChatbotId,
		SystemPrompt:  req.Chatbot.ChatbotSystemPrompt,
		Temperature:   float32(req.Chatbot.ChatbotTemperature),
		Model:         req.Chatbot.ChatbotModel,
		MaxTokens:     1024,
		TopK:          5,
		ToolConfigs:   []string{"rag"},
		GeminiAPIKeys: s.cfg.GeminiAPIKeys,
		CustomActions: customActions,
	}

	// Set default model if not provided
	if cfg.Model == "" {
		cfg.Model = "gemini-2.0-flash-lite"
	}

	// Set default temperature if not provided
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.7
	}

	deps := &core.GraphDependencies{
		DB:       s.db,
		Embedder: s.embedder,
	}

	compiledGraph, err := core.BuildChatbotGraph(ctx, cfg, deps)
	if err != nil {
		return errorResponse(fmt.Errorf("failed to build chatbot graph: %w", err))
	}

	messages, err := core.ParseConversationMessages(req.Query)
	if err != nil {
		return errorResponse(fmt.Errorf("failed to parse conversation: %w", err))
	}

	result, citations, err := s.invokeGraph(ctx, compiledGraph, messages, cfg)
	if err != nil {
		return errorResponse(fmt.Errorf("graph execution failed: %w", err))
	}

	assistantUUID, err := uuid.NewV7()
	if err != nil {
		return errorResponse(fmt.Errorf("failed to generate assistant message id: %w", err))
	}
	assistantMsgID := assistantUUID.String()
	response := &Response{
		Response:     result.Content,
		Citations:    citations,
		BaseResponse: types.BaseResponse{Success: true},
		MessageID:    assistantMsgID,
	}

	// Save messages in background
	go func() {
		saveCtx := context.Background()
		userUUID, err := uuid.NewV7()
		if err != nil {
			utils.Zlog.Error("Failed to generate user message id", zap.Error(err))
			return
		}
		userMsgID := userUUID.String()
		if err := core.SaveConversationMessagesBackground(saveCtx, s.db, core.MessageRecord{
			UniqueClientID: req.User.UniqueClientID,
			ChatbotID:      req.Chatbot.ChatbotId,
			Message:        core.ExtractLastUserContent(req.Query),
			Role:           "user",
			Citations:      []string{},
			MessageUID:     userMsgID,
			Channel:        core.ChannelWidget,
		}, core.MessageRecord{
			UniqueClientID: req.User.UniqueClientID,
			ChatbotID:      req.Chatbot.ChatbotId,
			Message:        response.Response,
			Role:           "assistant",
			Citations:      response.Citations,
			MessageUID:     assistantMsgID,
			Channel:        core.ChannelWidget,
		}); err != nil {
			utils.Zlog.Error("Failed to save playground messages in background", zap.Error(err))
		}
	}()

	latencyMS := time.Since(startTime).Milliseconds()
	utils.Zlog.Info("Playground request completed",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int64("latency_ms", latencyMS),
		zap.Bool("success", response.Success))

	return response, nil
}

