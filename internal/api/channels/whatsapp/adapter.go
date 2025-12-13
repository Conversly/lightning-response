package whatsapp

import (
	"context"
	"fmt"

	"github.com/Conversly/lightning-response/internal/config"
	"github.com/Conversly/lightning-response/internal/core"
	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/loaders"
)

// Adapter implements the ChannelAdapter interface for WhatsApp
type Adapter struct {
	db       *loaders.PostgresClient
	cfg      *config.Config
	embedder *embedder.GeminiEmbedder
}

// NewAdapter creates a new WhatsApp adapter
func NewAdapter(db *loaders.PostgresClient, cfg *config.Config, embedder *embedder.GeminiEmbedder) *Adapter {
	return &Adapter{
		db:       db,
		cfg:      cfg,
		embedder: embedder,
	}
}

// ValidateRequest validates WhatsApp webhook request
// TODO: Implement Meta signature verification
func (a *Adapter) ValidateRequest(ctx context.Context, req interface{}) (string, error) {
	// TODO: Implement WhatsApp-specific request validation
	// 1. Verify Meta webhook signature (X-Hub-Signature-256)
	// 2. Parse webhook payload
	// 3. Look up whatsappAccount by phone_number_id
	// 4. Return associated chatbot ID
	return "", fmt.Errorf("whatsapp adapter not implemented")
}

// GetSystemPrompt returns the WhatsApp-specific system prompt
func (a *Adapter) GetSystemPrompt(ctx context.Context, chatbotID string) (string, error) {
	// TODO: Check chatbot_channel_prompts for WhatsApp override
	// If not found, fall back to default chatbot systemPrompt
	return "", fmt.Errorf("whatsapp adapter not implemented")
}

// GetChannel returns the WhatsApp channel type
func (a *Adapter) GetChannel() core.Channel {
	return core.ChannelWhatsApp
}

// SaveMessages saves WhatsApp messages
func (a *Adapter) SaveMessages(ctx context.Context, records ...core.MessageRecord) error {
	// Set channel on all records
	for i := range records {
		records[i].Channel = core.ChannelWhatsApp
	}
	return core.SaveConversationMessagesBackground(ctx, a.db, records...)
}

