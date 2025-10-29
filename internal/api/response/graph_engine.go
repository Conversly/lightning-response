package response

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/llm"
	"github.com/Conversly/lightning-response/internal/loaders"
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

	// Get enabled tools for this chatbot
	enabledTools, err := GetEnabledTools(ctx, cfg, deps)
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled tools: %w", err)
	}

	// Create multi-key chat model with round-robin rotation
	baseChatModel, err := llm.NewMultiKeyChatModel(
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
		zap.Int("key_count", len(cfg.GeminiAPIKeys)),
		zap.Int("tool_count", len(enabledTools)))

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

	// Prepare chat model with tools if available
	hasTools := len(enabledTools) > 0

	if hasTools {
		// Extract tool info from tools and bind to model
		toolInfos := make([]*schema.ToolInfo, 0, len(enabledTools))
		for _, t := range enabledTools {
			info, err := t.Info(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get tool info: %w", err)
			}
			toolInfos = append(toolInfos, info)
		}

		// Bind tools to model using BindTools
		if err := baseChatModel.BindTools(toolInfos); err != nil {
			return nil, fmt.Errorf("failed to bind tools to model: %w", err)
		}

		utils.Zlog.Info("Bound tools to chat model",
			zap.String("chatbot_id", cfg.ChatbotID),
			zap.Int("tool_count", len(toolInfos)))
	}

	// Add ChatModel node with state handlers
	graph.AddChatModelNode("model", baseChatModel,
		compose.WithStatePreHandler(func(ctx context.Context, input []*schema.Message, state *GraphState) ([]*schema.Message, error) {
			// Update state with incoming messages
			state.Messages = append(state.Messages, input...)

			// Prepare messages with system prompt at the beginning
			finalMessages := make([]*schema.Message, 0, len(state.Messages)+1)

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
			// Store assistant message in state
			state.Messages = append(state.Messages, output)
			return output, nil
		}),
	)

	if hasTools {
		// Create ToolsNode - convert InvokableTool to BaseTool
		baseTools := make([]tool.BaseTool, len(enabledTools))
		for i, t := range enabledTools {
			baseTools[i] = t
		}

		toolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
			Tools: baseTools,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create tools node: %w", err)
		}

		// Add ToolsNode with state handlers
		graph.AddToolsNode("tools", toolsNode,
			compose.WithStatePreHandler(func(ctx context.Context, input *schema.Message, state *GraphState) (*schema.Message, error) {
				state.ToolCallCount++
				utils.Zlog.Info("Executing tool calls",
					zap.String("chatbot_id", cfg.ChatbotID),
					zap.Int("tool_call_count", state.ToolCallCount),
					zap.Int("num_calls", len(input.ToolCalls)))
				return input, nil
			}),
			compose.WithStatePostHandler(func(ctx context.Context, output []*schema.Message, state *GraphState) ([]*schema.Message, error) {
				// Extract citations from RAG tool responses
				for _, msg := range output {
					if msg.ToolCallID != "" {
						// Parse RAG tool output for citations
						var ragOutput struct {
							Citations []string `json:"citations"`
						}
						if err := json.Unmarshal([]byte(msg.Content), &ragOutput); err == nil {
							if len(ragOutput.Citations) > 0 {
								state.Citations = append(state.Citations, ragOutput.Citations...)
								utils.Zlog.Debug("Captured citations from RAG tool",
									zap.String("chatbot_id", cfg.ChatbotID),
									zap.Int("citations", len(ragOutput.Citations)))
							}
						}
					}
				}
				// Store tool messages in state
				state.Messages = append(state.Messages, output...)
				return output, nil
			}),
		)

		// Define routing logic
		graph.AddEdge(compose.START, "model")

		// Create a routing branch from model
		// The condition function receives the output from the model node
		routeBranch := compose.NewGraphBranch(
			func(ctx context.Context, msg *schema.Message) (endNode string, err error) {
				// Check if the message has tool calls
				if msg != nil && msg.Role == schema.Assistant && len(msg.ToolCalls) > 0 {
					utils.Zlog.Debug("Model requested tool calls, routing to tools node",
						zap.String("chatbot_id", cfg.ChatbotID),
						zap.Int("tool_calls", len(msg.ToolCalls)))
					return "tools", nil
				}

				// No tool calls, end the conversation
				return compose.END, nil
			},
			map[string]bool{
				"tools":     true,
				compose.END: true,
			},
		)

		graph.AddBranch("model", routeBranch)

		// After tools execute, go back to model
		graph.AddEdge("tools", "model")

		utils.Zlog.Info("Built graph with tools",
			zap.String("chatbot_id", cfg.ChatbotID),
			zap.Int("tool_count", len(enabledTools)))
	} else {
		// Direct flow (model only) when no tools
		graph.AddEdge(compose.START, "model")
		graph.AddEdge("model", compose.END)

		utils.Zlog.Info("Built graph without tools",
			zap.String("chatbot_id", cfg.ChatbotID))
	}

	// Compile the graph
	compiled, err := graph.Compile(ctx,
		compose.WithMaxRunSteps(10), // Limit iterations to prevent infinite loops
	)
	if err != nil {
		return nil, fmt.Errorf("graph compilation failed: %w", err)
	}

	utils.Zlog.Info("Graph compiled successfully",
		zap.String("chatbot_id", cfg.ChatbotID))

	return compiled, nil
}
