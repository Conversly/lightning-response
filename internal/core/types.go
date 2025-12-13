package core

import (
	"github.com/cloudwego/eino/schema"

	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/types"
)

// GraphState holds the state during graph execution
type GraphState struct {
	Messages        []*schema.Message      // Conversation history
	RAGDocs         []*schema.Document     // Retrieved documents
	KVs             map[string]interface{} // General key-value storage
	ChatbotID       string                 // Current chatbot context
	ToolCallCount   int                    // Track tool invocations
	ConversationKey string                 // Unique conversation identifier
	Citations       []string               // Collected citations from RAG tool
}

// ChatbotConfig holds configuration for building a chatbot graph
type ChatbotConfig struct {
	ChatbotID     string
	SystemPrompt  string
	Temperature   float32              // Changed to float32 for Gemini compatibility
	Model         string               // e.g., "gemini-2.0-flash-lite"
	MaxTokens     int                  // Maximum tokens in response
	TopK          int32                // Gemini-specific: controls diversity (1-40)
	ToolConfigs   []string             // e.g., ["rag"] more tools can be added
	GeminiAPIKeys []string             // Multiple API keys for rate limit distribution
	CustomActions []types.CustomAction // Custom actions loaded from database
}

// GraphDependencies holds dependencies needed for graph building
type GraphDependencies struct {
	DB       *loaders.PostgresClient
	Embedder *embedder.GeminiEmbedder
}

// Channel represents the message channel type
type Channel string

const (
	ChannelWidget   Channel = "WIDGET"
	ChannelWhatsApp Channel = "WHATSAPP"
	ChannelVoice    Channel = "VOICE"
)

// MessageRecord represents a message to be saved
type MessageRecord struct {
	UniqueClientID  string
	ChatbotID       string
	Message         string
	Role            string // user | assistant | agent
	Citations       []string
	MessageUID      string
	Channel         Channel                // WIDGET | WHATSAPP | VOICE
	ChannelMetadata map[string]interface{} // Optional channel-specific metadata
}

