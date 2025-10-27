package response

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/llm"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/rag"
	"github.com/Conversly/lightning-response/internal/utils"
)

type GraphState struct {
	Messages        []*schema.Message      // Conversation history
	RAGDocs         []*schema.Document     // Retrieved documents
	KVs             map[string]interface{} // General key-value storage
	ChatbotID       string                 // Current chatbot context
	ToolCallCount   int                    // Track tool invocations
	ConversationKey string                 // Unique conversation identifier
	Citations       []string               // Collected citations from RAG tool
}

type ChatbotConfig struct {
	ChatbotID     string
	SystemPrompt  string
	Temperature   float32  // Changed to float32 for Gemini compatibility
	Model         string   // e.g., "gemini-2.0-flash-exp"
	MaxTokens     int      // Maximum tokens in response
	TopK          int32    // Gemini-specific: controls diversity (1-40)
	ToolConfigs   []string // e.g., ["rag"] more tools can be added
	GeminiAPIKeys []string // Multiple API keys for rate limit distribution
}

// GraphDependencies holds dependencies needed for graph building
type GraphDependencies struct {
	DB       *loaders.PostgresClient
	Embedder *embedder.GeminiEmbedder
}

// BuildChatbotGraph creates a new graph for each request
func BuildChatbotGraph(ctx context.Context, cfg *ChatbotConfig, deps *GraphDependencies) (compose.Runnable[[]*schema.Message, *schema.Message], error) {
	// Validate API keys
	if len(cfg.GeminiAPIKeys) == 0 {
		return nil, fmt.Errorf("at least one Gemini API key is required")
	}

	temp := cfg.Temperature
	maxToks := cfg.MaxTokens

	// Create multi-key chat model with round-robin rotation
	chatModel, err := llm.NewMultiKeyChatModel(
		ctx,
		cfg.GeminiAPIKeys,
		cfg.Model,
		&temp,
		&maxToks,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create multi-key chat model: %w", err)
	}

	utils.Zlog.Info("Created multi-key Gemini chat model",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.String("model", cfg.Model),
		zap.Int("key_count", len(cfg.GeminiAPIKeys)))

	graph := compose.NewGraph[[]*schema.Message, *schema.Message](
		compose.WithGenLocalState(func(ctx context.Context) *GraphState {
			return &GraphState{
				Messages:        make([]*schema.Message, 0),
				RAGDocs:         make([]*schema.Document, 0),
				KVs:             make(map[string]interface{}),
				ChatbotID:       cfg.ChatbotID,
				ToolCallCount:   0,
				ConversationKey: "",
				Citations:       make([]string, 0),
			}
		}),
	)

	// Add ChatModel node with state handlers
	graph.AddChatModelNode("model", chatModel,
		compose.WithStatePreHandler(func(ctx context.Context, input []*schema.Message, state *GraphState) ([]*schema.Message, error) {
			// Update state with incoming messages
			state.Messages = append(state.Messages, input...)

			// If RAG is enabled, retrieve documents and capture citations into state
			ragEnabled := false
			for _, tool := range cfg.ToolConfigs {
				if tool == "rag" {
					ragEnabled = true
					break
				}
			}

			if ragEnabled {
				// Create retriever for this request
				retriever := rag.NewPgVectorRetriever(
					deps.DB,
					deps.Embedder,
					cfg.ChatbotID,
					int(cfg.TopK),
				)

				// Get last user message content
				var query string
				for i := len(state.Messages) - 1; i >= 0; i-- {
					if state.Messages[i] != nil && state.Messages[i].Content != "" {
						query = state.Messages[i].Content
						break
					}
				}

				if query != "" {
					results, rerr := retriever.Retrieve(ctx, query)
					if rerr != nil {
						utils.Zlog.Warn("RAG retrieval failed; continuing without citations",
							zap.String("chatbot_id", cfg.ChatbotID),
							zap.Error(rerr))
					} else {
						citations := make([]string, 0, len(results))
						for _, res := range results {
							if res.Citation != nil && *res.Citation != "" {
								citations = append(citations, *res.Citation)
							}
						}
						state.Citations = citations
						utils.Zlog.Debug("Captured citations from RAG",
							zap.String("chatbot_id", cfg.ChatbotID),
							zap.Int("citations", len(state.Citations)))
					}
				}
			}

			// Prepare messages with system prompt at the beginning
			finalMessages := make([]*schema.Message, 0, len(state.Messages)+1)

			// Add system message with built prompt at the start
			systemPromptContent := promptBuilder(cfg.SystemPrompt)
			finalMessages = append(finalMessages, schema.SystemMessage(systemPromptContent))

			// Add all conversation messages
			finalMessages = append(finalMessages, state.Messages...)

			utils.Zlog.Debug("State updated with messages",
				zap.String("chatbot_id", cfg.ChatbotID),
				zap.Int("total_messages", len(finalMessages)))
			return finalMessages, nil
		}),
		compose.WithStatePostHandler(func(ctx context.Context, output *schema.Message, state *GraphState) (*schema.Message, error) {
			if len(state.Citations) > 0 {
				// Append structured citations suffix so caller can parse and strip
				suffixBytes, _ := json.Marshal(state.Citations)
				output.Content = output.Content + "\n\n<<<CITATIONS>>>" + string(suffixBytes) + "<<<END>>>"
			}
			return output, nil
		}),
	)

	// Direct flow (model only) for now
	graph.AddEdge(compose.START, "model")
	graph.AddEdge("model", compose.END)

	utils.Zlog.Info("Built graph without tools",
		zap.String("chatbot_id", cfg.ChatbotID))

	// Compile the graph
	compiled, err := graph.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("graph compilation failed: %w", err)
	}

	utils.Zlog.Info("Graph compiled successfully",
		zap.String("chatbot_id", cfg.ChatbotID))

	return compiled, nil
}
