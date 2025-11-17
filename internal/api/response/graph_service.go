package response

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/config"
	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/rag"
	"github.com/Conversly/lightning-response/internal/types"
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
		Response:     "",
		BaseResponse: types.BaseResponse{Success: false},
		Citations:    []string{},
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

	info, err := s.db.GetChatbotInfoWithTopics(ctx, chatbotID)
	if err != nil {
		return errorResponse(fmt.Errorf("failed to load chatbot config: %w", err))
	}

	cfg := &ChatbotConfig{
		ChatbotID:     info.ID,
		SystemPrompt:  info.SystemPrompt,
		Temperature:   0.7,
		Model:         "gemini-2.0-flash-lite",
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

	// Step 7: Save messages in background (non-blocking)
	go func() {
		saveCtx := context.Background()
		userUUID, err := uuid.NewV7()
		if err != nil {
			utils.Zlog.Error("Failed to generate user message id", zap.Error(err))
			return
		}
		userMsgID := userUUID.String()
		if err := SaveConversationMessagesBackground(saveCtx, s.db, MessageRecord{
			UniqueClientID: req.User.UniqueClientID,
			ChatbotID:      info.ID,
			Message:        ExtractLastUserContent(req.Query),
			Role:           "user",
			Citations:      []string{},
			MessageUID:     userMsgID,
		}, MessageRecord{
			UniqueClientID: req.User.UniqueClientID,
			ChatbotID:      info.ID,
			Message:        response.Response,
			Role:           "assistant",
			Citations:      response.Citations,
			MessageUID:     assistantMsgID,
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

	fmt.Println(result)
	if err != nil {
		return nil, nil, fmt.Errorf("graph invocation failed: %w", err)
	}

	// Parse structured citations suffix if present and strip it from content
	citations := extractCitations(result)

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

	// Use playground chatbot configuration directly (no validation or DB fetch)
	cfg := &ChatbotConfig{
		ChatbotID:     req.Chatbot.ChatbotId,
		SystemPrompt:  req.Chatbot.ChatbotSystemPrompt,
		Temperature:   float32(req.Chatbot.ChatbotTemperature),
		Model:         req.Chatbot.ChatbotModel,
		MaxTokens:     1024,
		TopK:          5,
		ToolConfigs:   []string{"rag"},
		GeminiAPIKeys: s.cfg.GeminiAPIKeys,
	}

	// Set default model if not provided
	if cfg.Model == "" {
		cfg.Model = "gemini-2.0-flash-lite"
	}

	// Set default temperature if not provided
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.7
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
		if err := SaveConversationMessagesBackground(saveCtx, s.db, MessageRecord{
			UniqueClientID: req.User.UniqueClientID,
			ChatbotID:      req.Chatbot.ChatbotId,
			Message:        ExtractLastUserContent(req.Query),
			Role:           "user",
			Citations:      []string{},
			MessageUID:     userMsgID,
		}, MessageRecord{
			UniqueClientID: req.User.UniqueClientID,
			ChatbotID:      req.Chatbot.ChatbotId,
			Message:        response.Response,
			Role:           "assistant",
			Citations:      response.Citations,
			MessageUID:     assistantMsgID,
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

// extractCitations extracts citation URLs from the message
func extractCitations(msg *schema.Message) []string {
	// Look for structured suffix appended by the graph: <<<CITATIONS>>>[...]<<<END>>>
	const startTag = "<<<CITATIONS>>>"
	const endTag = "<<<END>>>"

	content := msg.Content
	utils.Zlog.Debug("Extracting citations from message",
		zap.String("content_length", fmt.Sprintf("%d", len(content))),
		zap.String("content_preview", func() string {
			if len(content) > 200 {
				return content[:200]
			}
			return content
		}()))

	start := strings.LastIndex(content, startTag)
	end := strings.LastIndex(content, endTag)

	utils.Zlog.Debug("Citation tag positions",
		zap.Int("start_tag_pos", start),
		zap.Int("end_tag_pos", end))

	if start == -1 || end == -1 || end <= start {
		utils.Zlog.Debug("No citations found in message - missing or invalid tags")
		return []string{}
	}

	jsonPart := content[start+len(startTag) : end]
	utils.Zlog.Debug("Extracted JSON part for citations",
		zap.String("json_part", jsonPart))

	var citations []string
	if err := json.Unmarshal([]byte(jsonPart), &citations); err != nil {
		utils.Zlog.Error("Failed to parse citations JSON",
			zap.Error(err),
			zap.String("json_part", jsonPart))
		return []string{}
	}

	utils.Zlog.Debug("Successfully extracted citations",
		zap.Int("citation_count", len(citations)),
		zap.Strings("citations", citations))

	// Strip the suffix from the content
	msg.Content = strings.TrimSpace(content[:start])
	return citations
}
