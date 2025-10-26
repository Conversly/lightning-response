package response

// This file contains example implementations and integration code
// for the Eino graph-based response system.

import (
	"context"
	"os"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/utils"
)

// ExampleGraphIntegration shows how to integrate the complete graph system
// This is a reference implementation showing all the pieces working together
func ExampleGraphIntegration() {
	ctx := context.Background()

	// 1. Initialize the graph engine (once at startup)
	if err := InitializeGraphEngine(ctx); err != nil {
		utils.Zlog.Fatal("Failed to initialize graph engine", zap.Error(err))
	}

	// 2. Create a sample chatbot configuration
	cfg := &ChatbotConfig{
		ChatbotID:    "example_chatbot",
		TenantID:     "example_tenant",
		SystemPrompt: "You are a helpful AI assistant with access to a knowledge base and web search.",
		Temperature:  0.7,
		Model:        "gpt-4o-mini",
		TopK:         5,
		RAGIndex:     "example_collection",
		ToolConfigs:  []string{"rag", "search"},
	}

	// 3. Get or create the graph (this will be cached)
	graph, err := GetOrCreateTenantGraph(ctx, cfg)
	if err != nil {
		utils.Zlog.Fatal("Failed to create graph", zap.Error(err))
	}

	// 4. Prepare conversation messages
	messages := []*schema.Message{
		schema.UserMessage("What are the best practices for database optimization?"),
	}

	// 5. Invoke the graph
	result, err := graph.Invoke(ctx, messages)
	if err != nil {
		utils.Zlog.Fatal("Graph invocation failed", zap.Error(err))
	}

	utils.Zlog.Info("Graph execution completed",
		zap.String("response", result.Content),
		zap.Int("tool_calls", len(result.ToolCalls)))
}

// ExampleCustomTool shows how to create a custom tool
func ExampleCustomTool() tool.InvokableTool {
	// Define request/response types
	type CustomToolRequest struct {
		Input string `json:"input" jsonschema:"required,description=The input to process"`
	}

	type CustomToolResponse struct {
		Output string `json:"output" jsonschema:"description=The processed output"`
	}

	// Define the tool function
	customFunc := func(ctx context.Context, req *CustomToolRequest, opts ...tool.Option) (*CustomToolResponse, error) {
		utils.Zlog.Debug("Custom tool invoked", zap.String("input", req.Input))

		// Your custom logic here
		output := "Processed: " + req.Input

		return &CustomToolResponse{
			Output: output,
		}, nil
	}

	// Create the tool
	return utils.InferOptionableTool(
		"custom_tool",
		"A custom tool that processes input",
		customFunc,
	)
}

// ExampleDynamicToolBinding shows how to bind different tools per request
func ExampleDynamicToolBinding(ctx context.Context) {
	cfg := &ChatbotConfig{
		ChatbotID:   "dynamic_chatbot",
		TenantID:    "tenant_abc",
		ToolConfigs: []string{"rag", "http_api", "search"},
	}

	graph, _ := GetOrCreateTenantGraph(ctx, cfg)

	// Scenario 1: User with all tools
	messages1 := []*schema.Message{
		schema.UserMessage("Search the web and query our knowledge base"),
	}

	// All tools are available (bound via ToolConfigs)
	result1, _ := graph.Invoke(ctx, messages1)
	utils.Zlog.Info("Scenario 1", zap.String("response", result1.Content))

	// Scenario 2: Create a config with only RAG
	cfg2 := &ChatbotConfig{
		ChatbotID:   "rag_only_chatbot",
		TenantID:    "tenant_abc",
		ToolConfigs: []string{"rag"},
	}

	graph2, _ := GetOrCreateTenantGraph(ctx, cfg2)
	messages2 := []*schema.Message{
		schema.UserMessage("What's in our knowledge base?"),
	}

	// Only RAG tool is available
	result2, _ := graph2.Invoke(ctx, messages2)
	utils.Zlog.Info("Scenario 2", zap.String("response", result2.Content))
}

// ExampleConversationFlow shows a multi-turn conversation
func ExampleConversationFlow(ctx context.Context) {
	cfg := &ChatbotConfig{
		ChatbotID:    "conversation_bot",
		TenantID:     "tenant_xyz",
		SystemPrompt: "You are a helpful assistant.",
		ToolConfigs:  []string{"rag"},
	}

	graph, _ := GetOrCreateTenantGraph(ctx, cfg)

	// Turn 1
	messages := []*schema.Message{
		schema.UserMessage("What is machine learning?"),
	}

	response1, _ := graph.Invoke(ctx, messages)
	utils.Zlog.Info("Turn 1", zap.String("response", response1.Content))

	// Turn 2 - append to history
	messages = append(messages, response1)
	messages = append(messages, schema.UserMessage("Can you give me examples?"))

	response2, _ := graph.Invoke(ctx, messages)
	utils.Zlog.Info("Turn 2", zap.String("response", response2.Content))

	// Turn 3
	messages = append(messages, response2)
	messages = append(messages, schema.UserMessage("How does it differ from deep learning?"))

	response3, _ := graph.Invoke(ctx, messages)
	utils.Zlog.Info("Turn 3", zap.String("response", response3.Content))
}

// ExampleStateManagement shows how to use graph state
func ExampleStateManagement(ctx context.Context) {
	// This would be inside a custom node function
	_ = compose.ProcessState[*GraphState](ctx, func(ctx context.Context, state *GraphState) error {
		// Thread-safe state access
		state.ToolCallCount++
		state.KVs["last_query"] = "example query"
		state.RAGDocs = append(state.RAGDocs, &schema.Document{
			ID:      "doc1",
			Content: "Example document",
		})

		utils.Zlog.Debug("State updated",
			zap.Int("tool_calls", state.ToolCallCount),
			zap.String("chatbot_id", state.ChatbotID))

		return nil
	})
}

// ExampleCachingStrategy demonstrates caching patterns
func ExampleCachingStrategy(ctx context.Context) {
	// Scenario 1: Same chatbot, multiple users
	// The graph is compiled once and reused
	cfg1 := &ChatbotConfig{ChatbotID: "shared_bot"}

	graph1a, _ := GetOrCreateTenantGraph(ctx, cfg1) // Compiles and caches
	graph1b, _ := GetOrCreateTenantGraph(ctx, cfg1) // Retrieved from cache (fast!)

	// graph1a and graph1b are the same instance
	_ = graph1a
	_ = graph1b

	// Scenario 2: Different chatbots
	// Each gets its own graph
	cfg2 := &ChatbotConfig{ChatbotID: "bot_2"}
	cfg3 := &ChatbotConfig{ChatbotID: "bot_3"}

	graph2, _ := GetOrCreateTenantGraph(ctx, cfg2) // New compilation
	graph3, _ := GetOrCreateTenantGraph(ctx, cfg3) // New compilation

	_ = graph2
	_ = graph3

	// Check cache stats
	count := GetCachedGraphCount()
	utils.Zlog.Info("Cached graphs", zap.Int("count", count))

	// Clear specific cache
	ClearGraphCache("bot_2")

	// Clear all caches
	ClearAllGraphCaches()
}

// ExampleErrorHandling shows error handling patterns
func ExampleErrorHandling(ctx context.Context) {
	cfg := &ChatbotConfig{
		ChatbotID: "error_bot",
		TenantID:  "tenant_error",
	}

	graph, err := GetOrCreateTenantGraph(ctx, cfg)
	if err != nil {
		utils.Zlog.Error("Graph creation failed", zap.Error(err))
		// Handle error appropriately
		return
	}

	messages := []*schema.Message{
		schema.UserMessage("Test query"),
	}

	result, err := graph.Invoke(ctx, messages)
	if err != nil {
		utils.Zlog.Error("Graph invocation failed",
			zap.Error(err),
			zap.String("chatbot_id", cfg.ChatbotID))
		// Handle error - maybe retry, fallback, etc.
		return
	}

	// Check if result is valid
	if result == nil || result.Content == "" {
		utils.Zlog.Warn("Empty response from graph",
			zap.String("chatbot_id", cfg.ChatbotID))
		return
	}

	utils.Zlog.Info("Success", zap.String("response", result.Content))
}

// ExampleMonitoring shows how to add monitoring/observability
func ExampleMonitoring(ctx context.Context) {
	cfg := &ChatbotConfig{
		ChatbotID: "monitored_bot",
		TenantID:  "tenant_monitored",
	}

	graph, _ := GetOrCreateTenantGraph(ctx, cfg)

	// Add metrics/logging
	messages := []*schema.Message{
		schema.UserMessage("Query for monitoring"),
	}

	// Measure latency
	// start := time.Now()
	result, err := graph.Invoke(ctx, messages)
	// latency := time.Since(start)

	if err != nil {
		// Log error metrics
		utils.Zlog.Error("Invocation failed",
			zap.Error(err),
			zap.String("chatbot_id", cfg.ChatbotID))
		return
	}

	// Log success metrics
	utils.Zlog.Info("Invocation succeeded",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int("response_length", len(result.Content)),
		zap.Int("tool_calls", len(result.ToolCalls)))

	// You could also send to monitoring services:
	// - Prometheus metrics
	// - DataDog
	// - CloudWatch
	// - etc.
}

// ExampleRuntimeConfiguration shows different runtime options
func ExampleRuntimeConfiguration(ctx context.Context) {
	cfg := &ChatbotConfig{
		ChatbotID:   "config_bot",
		Temperature: 0.7,
		Model:       "gpt-4o-mini",
	}

	graph, _ := GetOrCreateTenantGraph(ctx, cfg)
	messages := []*schema.Message{
		schema.UserMessage("Tell me about Go programming"),
	}

	// Invoke with default config
	_, _ = graph.Invoke(ctx, messages)

	// TODO: When ChatModel is implemented, add runtime options:
	// result, _ := graph.Invoke(ctx, messages,
	//     compose.WithChatModelOption(model.WithTemperature(0.5)),
	//     compose.WithChatModelOption(model.WithMaxTokens(500)),
	//     compose.WithCallbacks(customHandler),
	// )
}

// ExampleMultiTenantIsolation demonstrates tenant isolation
func ExampleMultiTenantIsolation(ctx context.Context) {
	// Tenant A - has RAG from collection_A
	cfgA := &ChatbotConfig{
		ChatbotID:   "tenant_a_bot",
		TenantID:    "tenant_a",
		RAGIndex:    "collection_a",
		ToolConfigs: []string{"rag"},
	}

	// Tenant B - has RAG from collection_B
	cfgB := &ChatbotConfig{
		ChatbotID:   "tenant_b_bot",
		TenantID:    "tenant_b",
		RAGIndex:    "collection_b",
		ToolConfigs: []string{"rag"},
	}

	// Each tenant gets their own graph with their own RAG retriever
	graphA, _ := GetOrCreateTenantGraph(ctx, cfgA)
	graphB, _ := GetOrCreateTenantGraph(ctx, cfgB)

	// Tenant A query - uses collection_a
	messagesA := []*schema.Message{
		schema.UserMessage("What's in my knowledge base?"),
	}
	resultA, _ := graphA.Invoke(ctx, messagesA)

	// Tenant B query - uses collection_b
	messagesB := []*schema.Message{
		schema.UserMessage("What's in my knowledge base?"),
	}
	resultB, _ := graphB.Invoke(ctx, messagesB)

	// Results are completely isolated
	utils.Zlog.Info("Tenant A result", zap.String("response", resultA.Content))
	utils.Zlog.Info("Tenant B result", zap.String("response", resultB.Content))
}

// ExampleRetrieverCaching shows retriever reuse
func ExampleRetrieverCaching(ctx context.Context) {
	cfg := &ChatbotConfig{
		ChatbotID: "retriever_test",
		RAGIndex:  "test_collection",
	}

	// First call - creates and caches retriever
	retriever1, _ := getOrCreateRAGRetriever(ctx, cfg)

	// Second call - retrieves from cache
	retriever2, _ := getOrCreateRAGRetriever(ctx, cfg)

	// Both are the same instance
	_ = retriever1
	_ = retriever2

	utils.Zlog.Info("Retriever cached and reused",
		zap.String("chatbot_id", cfg.ChatbotID))
}

// ExampleToolCreation shows how to add new tools
func ExampleToolCreation() {
	// Example: Create a calculator tool
	type CalcRequest struct {
		Expression string `json:"expression" jsonschema:"required,description=Mathematical expression to evaluate"`
	}

	type CalcResponse struct {
		Result float64 `json:"result" jsonschema:"description=Calculation result"`
	}

	calcFunc := func(ctx context.Context, req *CalcRequest, opts ...tool.Option) (*CalcResponse, error) {
		// Implement calculation logic
		// For example, use a math parser library
		result := 42.0 // Placeholder

		return &CalcResponse{Result: result}, nil
	}

	calcTool := utils.InferOptionableTool(
		"calculator",
		"Perform mathematical calculations. Use this when you need to compute numerical results.",
		calcFunc,
	)

	// Add to global tools
	globalTools["calculator"] = calcTool

	utils.Zlog.Info("Calculator tool created and registered")
}

// ExampleAdvancedRetriever shows custom retriever implementation
type CustomRetriever struct {
	chatbotID string
	index     string
}

func (r *CustomRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	utils.Zlog.Debug("Custom retriever called",
		zap.String("chatbot_id", r.chatbotID),
		zap.String("query", query))

	// Implement your custom retrieval logic
	// - Could be multiple sources
	// - Could include reranking
	// - Could add metadata filtering

	docs := []*schema.Document{
		{
			ID:      "custom_doc_1",
			Content: "Custom retrieved content for: " + query,
			Metadata: map[string]interface{}{
				"source": "custom_retriever",
				"score":  0.95,
			},
		},
	}

	return docs, nil
}

func ExampleCustomRetrieverUsage(ctx context.Context) {
	customRetriever := &CustomRetriever{
		chatbotID: "custom_bot",
		index:     "custom_index",
	}

	// Cache it
	retrieverCache.Store("custom_bot", customRetriever)

	// Use it in RAG tool
	_, _ = CreateRAGToolFromRetriever(customRetriever, "custom_bot", "custom_index")
}

// Helper function to check if environment is properly configured
func CheckEnvironmentConfiguration() bool {
	required := map[string]string{
		"OPENAI_API_KEY": os.Getenv("OPENAI_API_KEY"),
		"VIKING_AK":      os.Getenv("VIKING_AK"),
		"VIKING_SK":      os.Getenv("VIKING_SK"),
	}

	allSet := true
	for key, value := range required {
		if value == "" {
			utils.Zlog.Warn("Missing required environment variable", zap.String("key", key))
			allSet = false
		}
	}

	return allSet
}
