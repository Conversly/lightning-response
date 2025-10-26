package response

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino-ext/components/model/gemini"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/utils"
)

// GraphState represents the shared state accessible by all nodes in the graph
// This is thread-safe and managed by Eino's ProcessState
type GraphState struct {
	Messages        []*schema.Message      // Conversation history
	RAGDocs         []*schema.Document     // Retrieved documents
	KVs             map[string]interface{} // General key-value storage
	ChatbotID       string                 // Current chatbot context
	ToolCallCount   int                    // Track tool invocations
	ConversationKey string                 // Unique conversation identifier
	Citations       []string               // Collected citations from RAG tool
}

// ChatbotConfig represents the runtime configuration for a specific chatbot
// This is loaded from the database per tenant/chatbot
type ChatbotConfig struct {
	ChatbotID    string
	SystemPrompt string
	Temperature  float32  // Changed to float32 for Gemini compatibility
	Model        string   // e.g., "gemini-2.0-flash-exp"
	MaxTokens    int      // Maximum tokens in response
	TopK         int32    // Gemini-specific: controls diversity (1-40)
	ToolConfigs  []string // e.g., ["rag"] more tools can be added
}

// GraphCache stores compiled graphs per chatbot to avoid recompilation
// Key: chatbotID, Value: compiled Runnable
var (
	graphCache sync.Map // map[string]compose.Runnable[[]*schema.Message, *schema.Message]
	
	// RetrieverCache stores RAG retrievers per chatbot
	// Key: chatbotID, Value: retriever instance
	retrieverCache sync.Map // map[string]interface{} - actual retriever type
	
	// GlobalTools stores non-RAG tools that are shared across tenants
	// These are initialized once at startup
	globalTools     map[string]interface{} // tool.InvokableTool
	globalToolsOnce sync.Once
)

// InitializeGraphEngine initializes global resources for the graph engine
// This should be called once at application startup
func InitializeGraphEngine(ctx context.Context, db *loaders.PostgresClient, embedder *embedder.GeminiEmbedder) error {
	var initErr error
	
	globalToolsOnce.Do(func() {
		utils.Zlog.Info("Initializing graph engine global resources")
		
		// Initialize global tools map
		globalTools = make(map[string]interface{})
		
		// Set global dependencies for RAG tools
		SetGlobalDependencies(db, embedder)
		
		utils.Zlog.Info("Graph engine initialized successfully")
	})
	
	return initErr
}

// GetOrCreateTenantGraph retrieves a cached graph or creates a new one for the chatbot
// This is the core function that builds the Eino graph with all nodes and edges
func GetOrCreateTenantGraph(ctx context.Context, cfg *ChatbotConfig) (compose.Runnable[[]*schema.Message, *schema.Message], error) {
	// Check cache first
	if cached, ok := graphCache.Load(cfg.ChatbotID); ok {
		utils.Zlog.Debug("Using cached graph", zap.String("chatbot_id", cfg.ChatbotID))
		return cached.(compose.Runnable[[]*schema.Message, *schema.Message]), nil
	}
	
	utils.Zlog.Info("Building new graph for chatbot", zap.String("chatbot_id", cfg.ChatbotID))
	
	// Build the graph
	graph, err := buildTenantGraph(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to build graph for chatbot %s: %w", cfg.ChatbotID, err)
	}
	
	// Cache it
	graphCache.Store(cfg.ChatbotID, graph)
	
	return graph, nil
}

// buildTenantGraph constructs the complete Eino graph for a specific chatbot
// This implements the ReAct agent pattern with conditional edges
func buildTenantGraph(ctx context.Context, cfg *ChatbotConfig) (compose.Runnable[[]*schema.Message, *schema.Message], error) {
	// Create Gemini ChatModel with fastest model
	chatModel, err := gemini.NewChatModel(ctx, &gemini.Config{
		APIKey:      os.Getenv("GEMINI_API_KEY"),
		Model:       "gemini-2.0-flash-exp", // Fastest Gemini model
		Temperature: cfg.Temperature,
		MaxTokens:   cfg.MaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini chat model: %w", err)
	}

	utils.Zlog.Info("Created Gemini chat model",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.String("model", "gemini-2.0-flash-exp"))

	// Create graph with state initialization
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

	// Add ChatModel node with state handler
	graph.AddChatModelNode("model", chatModel,
		compose.WithStatePreHandler(func(ctx context.Context, input []*schema.Message, state *GraphState) ([]*schema.Message, error) {
			// Update state with incoming messages
			state.Messages = append(state.Messages, input...)
			utils.Zlog.Debug("State updated with messages",
				zap.String("chatbot_id", cfg.ChatbotID),
				zap.Int("total_messages", len(state.Messages)))
			return state.Messages, nil
		}),
	)

	// Get enabled tools for this chatbot
	tools, err := GetEnabledTools(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled tools: %w", err)
	}

	if len(tools) > 0 {
		// Add ToolsNode
		toolsNode := compose.NewToolsNode(tools)
		graph.AddToolsNode("tools", toolsNode,
			compose.WithStatePostHandler(func(ctx context.Context, output []*schema.Message, state *GraphState) ([]*schema.Message, error) {
				// Track tool calls
				state.ToolCallCount++
				utils.Zlog.Debug("Tool executed",
					zap.String("chatbot_id", cfg.ChatbotID),
					zap.Int("tool_call_count", state.ToolCallCount))
				return output, nil
			}),
		)

		// Define graph edges (ReAct pattern)
		graph.AddEdge(compose.START, "model")
		graph.AddBranch("model", createShouldContinueFunc())
		graph.AddEdge("tools", "model")
		graph.AddEdge("model", compose.END)

		utils.Zlog.Info("Built graph with ReAct pattern",
			zap.String("chatbot_id", cfg.ChatbotID),
			zap.Int("tool_count", len(tools)))
	} else {
		// No tools - direct flow (model only)
		graph.AddEdge(compose.START, "model")
		graph.AddEdge("model", compose.END)

		utils.Zlog.Info("Built graph without tools",
			zap.String("chatbot_id", cfg.ChatbotID))
	}

	// Compile the graph
	compiled, err := graph.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("graph compilation failed: %w", err)
	}

	utils.Zlog.Info("Graph compiled successfully",
		zap.String("chatbot_id", cfg.ChatbotID))

	return compiled, nil
}

// createShouldContinueFunc returns a conditional branch function
// This determines whether to continue to tools or end the conversation
func createShouldContinueFunc() func(context.Context, *GraphState) (string, error) {
	return func(ctx context.Context, state *GraphState) (string, error) {
		// If no messages, this shouldn't happen
		if len(state.Messages) == 0 {
			return compose.END, nil
		}

		// Get the last message (should be from assistant)
		lastMessage := state.Messages[len(state.Messages)-1]

		// If there are tool calls, route to tools node
		if len(lastMessage.ToolCalls) > 0 {
			utils.Zlog.Debug("Routing to tools",
				zap.Int("tool_calls", len(lastMessage.ToolCalls)),
				zap.String("chatbot_id", state.ChatbotID))
			return "tools", nil
		}

		// Otherwise, end the conversation
		utils.Zlog.Debug("Ending conversation", zap.String("chatbot_id", state.ChatbotID))
		return compose.END, nil
	}
}

func ClearGraphCache(chatbotID string) {
	graphCache.Delete(chatbotID)
	utils.Zlog.Info("Cleared graph cache", zap.String("chatbot_id", chatbotID))
}

// ClearAllGraphCaches removes all cached graphs
// Useful for testing or major configuration changes
func ClearAllGraphCaches() {
	graphCache.Range(func(key, value interface{}) bool {
		graphCache.Delete(key)
		return true
	})
	utils.Zlog.Info("Cleared all graph caches")
}

// GetCachedGraphCount returns the number of cached graphs (for monitoring)
func GetCachedGraphCount() int {
	count := 0
	graphCache.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}




