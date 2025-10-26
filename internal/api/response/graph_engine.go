package response

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/utils"
)

// GraphState represents the shared state accessible by all nodes in the graph
// This is thread-safe and managed by Eino's ProcessState
type GraphState struct {
	Messages        []*schema.Message      // Conversation history
	RAGDocs         []*schema.Document     // Retrieved documents
	KVs             map[string]interface{} // General key-value storage
	TenantID        string                 // Current tenant context
	ChatbotID       string                 // Current chatbot context
	ToolCallCount   int                    // Track tool invocations
	ConversationKey string                 // Unique conversation identifier
}

// ChatbotConfig represents the runtime configuration for a specific chatbot
// This is loaded from the database per tenant/chatbot
type ChatbotConfig struct {
	ChatbotID    string
	TenantID     string
	SystemPrompt string
	Temperature  float64
	Model        string
	TopK         int
	RAGIndex     string   // Viking DB collection name
	ToolConfigs  []string // e.g., ["rag", "http_api", "search"]
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
func InitializeGraphEngine(ctx context.Context) error {
	var initErr error
	
	globalToolsOnce.Do(func() {
		utils.Zlog.Info("Initializing graph engine global resources")
		
		// Initialize global tools map
		globalTools = make(map[string]interface{})
		
		// TODO: Initialize shared tools here
		// Example:
		// globalTools["http_api"] = createHTTPTool()
		// globalTools["search"] = createSearchTool()
		
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
	// Create graph with state initialization
	graph := compose.NewGraph[[]*schema.Message, *schema.Message](
		compose.WithGenLocalState(func(ctx context.Context) *GraphState {
			return &GraphState{
				Messages:        make([]*schema.Message, 0),
				RAGDocs:         make([]*schema.Document, 0),
				KVs:             make(map[string]interface{}),
				TenantID:        cfg.TenantID,
				ChatbotID:       cfg.ChatbotID,
				ToolCallCount:   0,
				ConversationKey: "",
			}
		}),
	)
	
	// TODO: Add ChatModel node
	// chatModel := createChatModel(ctx, cfg)
	// graph.AddChatModelNode("model", chatModel)
	
	// TODO: Add ToolsNode with tenant-specific tools
	// tools := getEnabledTools(ctx, cfg)
	// toolsNode := compose.NewToolsNode(tools)
	// graph.AddToolsNode("tools", toolsNode)
	
	// TODO: Define graph edges (ReAct pattern)
	// graph.AddEdge(compose.START, "model")
	// graph.AddBranch("model", createShouldContinueFunc(cfg))
	// graph.AddEdge("tools", "model")
	// graph.AddEdge("model", compose.END)
	
	// Compile the graph
	compiled, err := graph.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("graph compilation failed: %w", err)
	}
	
	return compiled, nil
}

// createShouldContinueFunc returns a conditional branch function
// This determines whether to continue to tools or end the conversation
func createShouldContinueFunc(cfg *ChatbotConfig) func(context.Context, *GraphState) (string, error) {
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
				zap.String("chatbot_id", cfg.ChatbotID))
			return "tools", nil
		}
		
		// Otherwise, end the conversation
		utils.Zlog.Debug("Ending conversation", zap.String("chatbot_id", cfg.ChatbotID))
		return compose.END, nil
	}
}

// ClearGraphCache removes a specific chatbot's graph from cache
// Useful when chatbot configuration changes
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
