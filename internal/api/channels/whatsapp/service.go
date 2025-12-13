package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	"github.com/Conversly/lightning-response/internal/utils"
)

const (
	metaGraphAPIBaseURL = "https://graph.facebook.com/v21.0"
)

// Service handles WhatsApp message processing and responses
type Service struct {
	db       *loaders.PostgresClient
	cfg      *config.Config
	embedder *embedder.GeminiEmbedder
	client   *http.Client
}

// NewService creates a new WhatsApp service
func NewService(db *loaders.PostgresClient, cfg *config.Config, embedder *embedder.GeminiEmbedder) *Service {
	return &Service{
		db:       db,
		cfg:      cfg,
		embedder: embedder,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// WhatsAppMessageRequest represents the Meta API request body
type WhatsAppMessageRequest struct {
	MessagingProduct string                  `json:"messaging_product"`
	RecipientType    string                  `json:"recipient_type"`
	To               string                  `json:"to"`
	Context          *WhatsAppMessageContext `json:"context,omitempty"`
	Type             string                  `json:"type"`
	Text             *WhatsAppTextContent    `json:"text,omitempty"`
}

// WhatsAppMessageContext for replying to a specific message
type WhatsAppMessageContext struct {
	MessageID string `json:"message_id"`
}

// WhatsAppTextContent for text messages
type WhatsAppTextContent struct {
	PreviewURL bool   `json:"preview_url"`
	Body       string `json:"body"`
}

// WhatsAppMessageResponse from Meta API
type WhatsAppMessageResponse struct {
	MessagingProduct string `json:"messaging_product"`
	Contacts         []struct {
		Input string `json:"input"`
		WaID  string `json:"wa_id"`
	} `json:"contacts"`
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

// ProcessMessageRequest contains all info needed to process a WhatsApp message
type ProcessMessageRequest struct {
	ChatbotID          string
	PhoneNumberID      string
	UserWaID           string
	UserPhoneNumber    string // Full phone number for contact
	UserDisplayName    string // Profile name from WhatsApp
	MessageText        string
	IncomingMsgID      string
	AccessToken        string
	WabaID             string // WhatsApp Business Account ID
	DisplayPhoneNumber string // Business phone number display
}

// ProcessAndRespond processes the incoming message and sends response via Meta API
func (s *Service) ProcessAndRespond(ctx context.Context, req *ProcessMessageRequest) error {
	startTime := time.Now()
	now := time.Now().UTC().Format(time.RFC3339)

	utils.Zlog.Info("Processing WhatsApp message",
		zap.String("chatbot_id", req.ChatbotID),
		zap.String("user_wa_id", req.UserWaID))

	// 1. Upsert WhatsApp contact (create if new, update last_seen if existing)
	contactMetadata := &loaders.WhatsAppContactMetadata{
		WaID:                 req.UserWaID,
		ProfileName:          req.UserDisplayName,
		FirstSeenAt:          now, // Will be ignored on conflict (keeps original)
		LastSeenAt:           now,
		LastInboundMessageID: req.IncomingMsgID,
		WabaID:               req.WabaID,
		PhoneNumberID:        req.PhoneNumberID,
		DisplayPhoneNumber:   req.DisplayPhoneNumber,
		Source:               "inbound",
	}
	if err := s.db.UpsertWhatsAppContact(ctx, req.ChatbotID, req.UserPhoneNumber, req.UserDisplayName, contactMetadata); err != nil {
		utils.Zlog.Warn("Failed to upsert WhatsApp contact",
			zap.String("chatbot_id", req.ChatbotID),
			zap.String("phone", req.UserPhoneNumber),
			zap.Error(err))
		// Don't fail the request, continue processing
	}

	// 2. Load conversation history for context
	history, err := s.db.GetWhatsAppConversationHistory(ctx, req.UserWaID, req.ChatbotID, 10)
	if err != nil {
		utils.Zlog.Debug("Failed to load conversation history",
			zap.String("chatbot_id", req.ChatbotID),
			zap.String("user_wa_id", req.UserWaID),
			zap.Error(err))
		history = nil
	}

	// 3. Get chatbot info
	chatbotInfo, err := s.db.GetChatbotInfoWithTopics(ctx, req.ChatbotID)
	if err != nil {
		return fmt.Errorf("failed to get chatbot info: %w", err)
	}

	// 4. Build chatbot config
	cfg := &core.ChatbotConfig{
		ChatbotID:     chatbotInfo.ID,
		SystemPrompt:  chatbotInfo.SystemPrompt,
		Temperature:   0.7,
		Model:         "gemini-2.0-flash-lite",
		MaxTokens:     1024,
		TopK:          5,
		ToolConfigs:   []string{"rag"},
		GeminiAPIKeys: s.cfg.GeminiAPIKeys,
		CustomActions: chatbotInfo.CustomActions,
	}

	// 5. Build graph dependencies
	deps := &core.GraphDependencies{
		DB:       s.db,
		Embedder: s.embedder,
	}

	// 6. Build the graph
	compiledGraph, err := core.BuildChatbotGraph(ctx, cfg, deps)
	if err != nil {
		return fmt.Errorf("failed to build chatbot graph: %w", err)
	}

	// 7. Prepare messages (history + current message)
	messages := s.buildMessageHistory(history, req.MessageText)

	// 8. Invoke graph
	result, citations, err := s.invokeGraph(ctx, compiledGraph, messages, cfg)
	if err != nil {
		return fmt.Errorf("graph invocation failed: %w", err)
	}

	// 9. Send response via Meta API
	sentMsgID, err := s.sendWhatsAppMessage(ctx, req.PhoneNumberID, req.UserWaID, result.Content, req.IncomingMsgID, req.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to send WhatsApp message: %w", err)
	}

	// 10. Save messages in background with channel metadata
	go func() {
		saveCtx := context.Background()
		userUUID, err := uuid.NewV7()
		if err != nil {
			utils.Zlog.Error("Failed to generate user message id", zap.Error(err))
			return
		}
		userMsgID := userUUID.String()

		assistantUUID, err := uuid.NewV7()
		if err != nil {
			utils.Zlog.Error("Failed to generate assistant message id", zap.Error(err))
			return
		}
		assistantMsgID := assistantUUID.String()

		// Channel metadata for WhatsApp messages
		userMetadata := map[string]interface{}{
			"wa_message_id":   req.IncomingMsgID,
			"phone_number_id": req.PhoneNumberID,
			"wa_id":           req.UserWaID,
		}
		assistantMetadata := map[string]interface{}{
			"wa_message_id":   sentMsgID,
			"phone_number_id": req.PhoneNumberID,
			"wa_id":           req.UserWaID,
			"reply_to":        req.IncomingMsgID,
		}

		if err := core.SaveConversationMessagesBackground(saveCtx, s.db, core.MessageRecord{
			UniqueClientID:  req.UserWaID,
			ChatbotID:       req.ChatbotID,
			Message:         req.MessageText,
			Role:            "user",
			Citations:       []string{},
			MessageUID:      userMsgID,
			Channel:         core.ChannelWhatsApp,
			ChannelMetadata: userMetadata,
		}, core.MessageRecord{
			UniqueClientID:  req.UserWaID,
			ChatbotID:       req.ChatbotID,
			Message:         result.Content,
			Role:            "assistant",
			Citations:       citations,
			MessageUID:      assistantMsgID,
			Channel:         core.ChannelWhatsApp,
			ChannelMetadata: assistantMetadata,
		}); err != nil {
			utils.Zlog.Error("Failed to save WhatsApp messages", zap.Error(err))
		}
	}()

	latencyMS := time.Since(startTime).Milliseconds()
	utils.Zlog.Info("WhatsApp message processed and sent",
		zap.String("chatbot_id", req.ChatbotID),
		zap.String("sent_msg_id", sentMsgID),
		zap.Int64("latency_ms", latencyMS))

	return nil
}

// buildMessageHistory constructs the message array for graph invocation
func (s *Service) buildMessageHistory(history []map[string]interface{}, currentMessage string) []*schema.Message {
	var messages []*schema.Message

	// Add conversation history if available
	for _, msg := range history {
		role, ok := msg["role"].(string)
		if !ok {
			continue
		}
		content, ok := msg["content"].(string)
		if !ok {
			continue
		}

		switch role {
		case "user":
			messages = append(messages, schema.UserMessage(content))
		case "assistant":
			messages = append(messages, schema.AssistantMessage(content, nil))
		}
	}

	// Add current message
	messages = append(messages, schema.UserMessage(currentMessage))

	return messages
}

// invokeGraph executes the compiled graph
func (s *Service) invokeGraph(
	ctx context.Context,
	graph compose.Runnable[[]*schema.Message, *schema.Message],
	messages []*schema.Message,
	cfg *core.ChatbotConfig,
) (*schema.Message, []string, error) {
	utils.Zlog.Debug("Invoking graph for WhatsApp",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int("message_count", len(messages)))

	result, err := graph.Invoke(ctx, messages)
	if err != nil {
		return nil, nil, fmt.Errorf("graph invocation failed: %w", err)
	}

	// Extract citations
	citations := core.ExtractCitations(result)

	// Fallback: if no citations, run retriever on last user message
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
			}
		}
	}

	return result, citations, nil
}

// sendWhatsAppMessage sends a text message via Meta Graph API
func (s *Service) sendWhatsAppMessage(ctx context.Context, phoneNumberID, to, body, replyToMsgID, accessToken string) (string, error) {
	url := fmt.Sprintf("%s/%s/messages", metaGraphAPIBaseURL, phoneNumberID)

	reqBody := &WhatsAppMessageRequest{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		To:               to,
		Type:             "text",
		Text: &WhatsAppTextContent{
			PreviewURL: false,
			Body:       body,
		},
	}

	// Add context if replying to a message
	if replyToMsgID != "" {
		reqBody.Context = &WhatsAppMessageContext{
			MessageID: replyToMsgID,
		}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&errBody)
		return "", fmt.Errorf("meta API error (status %d): %v", resp.StatusCode, errBody)
	}

	var msgResp WhatsAppMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&msgResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(msgResp.Messages) == 0 {
		return "", fmt.Errorf("no message ID returned from Meta API")
	}

	return msgResp.Messages[0].ID, nil
}
