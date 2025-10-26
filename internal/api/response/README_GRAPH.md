# Eino Graph Implementation for Response API

This directory contains the Eino graph-based implementation for the `/response` API endpoint. This approach provides a flexible, multi-tenant RAG-powered chatbot system with per-tenant tool configuration.

## Architecture Overview

### Core Components

1. **Graph Engine** (`graph_engine.go`)
   - Manages graph compilation and caching
   - Handles shared state management
   - Provides graph lifecycle functions

2. **Tools** (`graph_tools.go`)
   - RAG tool wrapper
   - HTTP API tool
   - Web search tool
   - Dynamic tool selection per tenant

3. **Database Integration** (`graph_db.go`)
   - Conversation history management
   - Chatbot configuration loading
   - Tenant validation

4. **Service** (`graph_service.go`)
   - Main orchestration layer
   - Graph invocation with runtime options
   - Response formatting

## Key Design Decisions

### ✅ Single Graph with Dynamic Tool Binding

**Pattern**: One compiled graph per chatbot, tools bound dynamically at runtime

```go
// Compile ONCE per chatbot at first request
compiledGraph, err := GetOrCreateTenantGraph(ctx, cfg)

// Invoke with user-specific tool binding
result, err := compiledGraph.Invoke(ctx, messages,
    compose.WithBindTools(enabledToolInfos),
    compose.WithChatModelOption(model.WithTemperature(cfg.Temperature)),
)
```

**Benefits**:
- ✅ Minimal compilation overhead (cached per chatbot)
- ✅ Dynamic tool selection per request
- ✅ One graph handles all users for a chatbot
- ✅ Runtime configuration via CallOptions

### ✅ Graph State Management

**Pattern**: Thread-safe shared state using Eino's `ProcessState`

```go
type GraphState struct {
    Messages        []*schema.Message      // Conversation history
    RAGDocs         []*schema.Document     // Retrieved documents
    KVs             map[string]interface{} // Key-value storage
    TenantID        string
    ChatbotID       string
    ToolCallCount   int
}
```

**Access pattern**:
```go
err := compose.ProcessState[*GraphState](ctx, func(ctx context.Context, state *GraphState) error {
    state.ToolCallCount++
    state.Messages = append(state.Messages, newMessage)
    return nil
})
```

### ✅ Per-Tenant RAG Retrievers

**Pattern**: Separate retriever instances cached per chatbot

```go
// Cache retriever per chatbot (different collections/indexes)
retriever := getOrCreateRAGRetriever(ctx, cfg)
ragTool := CreateRAGToolFromRetriever(retriever, cfg.ChatbotID, cfg.RAGIndex)
```

**Benefits**:
- ✅ Each tenant has isolated RAG index
- ✅ Retrievers are cached and reused
- ✅ No cross-tenant data leakage

## Request Flow

```
1. Request arrives at Controller
   ↓
2. Validate tenant (web_id + origin_url)
   ↓
3. Load ChatbotConfig from database
   - System prompt, temperature, model
   - Tool configurations (["rag", "http_api"])
   - RAG index name
   ↓
4. Get or create compiled graph (cached)
   - If cached: return immediately
   - If new: build graph → compile → cache
   ↓
5. Load conversation history from DB
   - Convert to []*schema.Message
   ↓
6. Execute graph with runtime options
   graph.Invoke(ctx, messages,
       WithBindTools(tools),
       WithTemperature(cfg.Temperature))
   ↓
7. Save conversation to database
   - Insert user query
   - Insert assistant response
   ↓
8. Return response to client
```

## ReAct Agent Pattern

The graph implements a standard ReAct (Reasoning + Acting) pattern:

```
START → ChatModel → [Branch]
                      ↓
            ┌─────────┴─────────┐
            ↓                   ↓
         ToolsNode            END
            ↓
         ChatModel
         (loop)
```

**Conditional Edge Logic**:
```go
func shouldContinue(ctx context.Context, state *GraphState) (string, error) {
    lastMessage := state.Messages[len(state.Messages)-1]
    
    if len(lastMessage.ToolCalls) > 0 {
        return "tools", nil  // LLM wants to call tools
    }
    return compose.END, nil  // LLM has final answer
}
```

## Tool Selection Mechanism

### How the LLM Chooses Tools

1. **Tool schemas sent to LLM** via `WithBindTools()`:
   ```json
   {
     "tools": [
       {
         "type": "function",
         "function": {
           "name": "query_knowledge_base_chatbot123",
           "description": "Query the knowledge base...",
           "parameters": {
             "type": "object",
             "properties": {
               "query": {"type": "string", "description": "Search query"}
             }
           }
         }
       }
     ]
   }
   ```

2. **LLM decides** which tool(s) to call based on:
   - User query content
   - Tool descriptions
   - System prompt guidance

3. **LLM outputs** structured tool calls:
   ```go
   ToolCalls: []schema.ToolCall{
       {
           ID: "call_abc123",
           Function: schema.FunctionCall{
               Name: "query_knowledge_base_chatbot123",
               Arguments: `{"query": "How to optimize queries?"}`,
           },
       },
   }
   ```

4. **ToolsNode executes** the specific tool by name matching

## Configuration

### ChatbotConfig (from database)

```go
type ChatbotConfig struct {
    ChatbotID    string      // Unique chatbot identifier
    TenantID     string      // Parent tenant
    SystemPrompt string      // Custom instructions
    Temperature  float64     // LLM temperature (0.0-1.0)
    Model        string      // e.g., "gpt-4o-mini"
    TopK         int         // RAG retrieval count
    RAGIndex     string      // Viking DB collection name
    ToolConfigs  []string    // ["rag", "http_api", "search"]
}
```

### Database Schema (expected)

```sql
-- Businesses table
CREATE TABLE businesses (
    tenant_id VARCHAR PRIMARY KEY,
    web_id VARCHAR UNIQUE NOT NULL,
    allowed_domains TEXT[], -- Array of allowed origins
    created_at TIMESTAMP DEFAULT NOW()
);

-- Chatbots table
CREATE TABLE chatbots (
    chatbot_id VARCHAR PRIMARY KEY,
    tenant_id VARCHAR REFERENCES businesses(tenant_id),
    system_prompt TEXT,
    temperature FLOAT,
    model VARCHAR,
    top_k INT,
    rag_index VARCHAR,
    tool_configs JSONB, -- ["rag", "http_api"]
    created_at TIMESTAMP DEFAULT NOW()
);

-- Messages table
CREATE TABLE messages (
    id BIGSERIAL PRIMARY KEY,
    unique_client_id VARCHAR NOT NULL,
    chatbot_id VARCHAR NOT NULL,
    message TEXT NOT NULL,
    type VARCHAR NOT NULL, -- 'query' or 'response'
    origin_url VARCHAR,
    created_at TIMESTAMP DEFAULT NOW(),
    INDEX idx_client_chatbot (unique_client_id, chatbot_id, created_at)
);
```

## Performance Considerations

### Graph Compilation

**✅ DO**: Compile once per chatbot, cache aggressively
```go
// ✅ Cached - very fast
compiledGraph, _ := GetOrCreateTenantGraph(ctx, cfg)
```

**❌ DON'T**: Compile per request
```go
// ❌ Slow - recompiles every time
graph := buildTenantGraph(ctx, cfg)
compiled, _ := graph.Compile(ctx)
```

### Typical Latency Profile

- **First request** (cold): ~100-200ms (graph compilation + LLM call)
- **Subsequent requests** (warm): ~50-100ms (cached graph + LLM call)
- **LLM call**: 500-2000ms (depends on model and complexity)
- **RAG retrieval**: 50-200ms (depends on vector DB)

### Caching Strategy

```go
// Graphs cached per chatbot
graphCache.Store(chatbotID, compiledGraph)

// Retrievers cached per chatbot
retrieverCache.Store(chatbotID, retriever)

// Global tools shared across all tenants
globalTools["http_api"] = httpTool
```

## TODOs / Integration Points

The current implementation has placeholders for:

### 1. ChatModel Integration
```go
// TODO in graph_engine.go:
chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
    APIKey: os.Getenv("OPENAI_API_KEY"),
    Model:  cfg.Model,
})
graph.AddChatModelNode("model", chatModel)
```

### 2. RAG Retriever (Viking DB)
```go
// TODO in graph_tools.go:
retriever, err := volc_vikingdb.NewRetriever(ctx, &volc_vikingdb.RetrieverConfig{
    Host:       "api-vikingdb.volces.com",
    Region:     "cn-beijing",
    AK:         os.Getenv("VIKING_AK"),
    SK:         os.Getenv("VIKING_SK"),
    Collection: cfg.RAGIndex,
    TopK:       &cfg.TopK,
})
```

### 3. Database Queries
```go
// TODO in graph_db.go:
// - LoadConversationHistory: SELECT messages FROM messages WHERE...
// - SaveConversationMessages: INSERT INTO messages...
// - GetChatbotConfig: SELECT * FROM chatbots WHERE...
// - ValidateTenantAccess: SELECT tenant_id FROM businesses WHERE...
```

### 4. HTTP Tool Implementation
```go
// TODO in graph_tools.go:
client := &http.Client{Timeout: 10 * time.Second}
resp, err := client.Do(httpReq)
```

### 5. Search Tool Implementation
```go
// TODO in graph_tools.go:
// Integrate with search API (Google, Bing, etc.)
```

## Usage Example

### Initialization (in main.go)

```go
func main() {
    // ... setup DB, config ...
    
    graphService := response.NewGraphService(dbClient, config)
    if err := graphService.Initialize(ctx); err != nil {
        log.Fatal(err)
    }
    
    controller := response.NewGraphController(graphService)
    
    // ... setup routes ...
}
```

### API Request

```bash
curl -X POST http://localhost:8080/response \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-api-key" \
  -d '{
    "query": "How do I optimize my queries?",
    "mode": "default",
    "user": {
      "unique_client_id": "user_12345",
      "conversly_web_id": "e6a4437b-85c6-42f9-a447-f5bc8b6ca3f4"
    },
    "metadata": {
      "origin_url": "https://example.com"
    }
  }'
```

### Response

```json
{
  "request_id": "req_abc123",
  "mode": "default",
  "answer": "To optimize your queries, you should...",
  "sources": [
    {
      "title": "Query Optimization Guide",
      "url": "https://docs.example.com/optimize",
      "snippet": "Best practices for query optimization..."
    }
  ],
  "usage": {
    "prompt_tokens": 150,
    "completion_tokens": 200,
    "total_tokens": 350,
    "latency_ms": 1250
  },
  "conversation_key": "user_12345"
}
```

## Monitoring

### Cache Statistics

```go
stats := graphService.GetCacheStats()
// {
//   "cached_graphs": 15
// }
```

### Clearing Cache

```go
// Clear specific chatbot
graphService.ClearCache("chatbot_123")

// Clear all
graphService.ClearCache("")
```

## Testing Strategy

1. **Unit Tests**
   - Tool creation and invocation
   - State management
   - Graph building

2. **Integration Tests**
   - End-to-end request flow
   - Database interaction
   - Graph execution

3. **Load Tests**
   - Concurrent requests
   - Cache effectiveness
   - Memory usage

## References

- [Eino Documentation](https://github.com/cloudwego/eino)
- [Eino Graph Compose](https://github.com/cloudwego/eino/tree/main/compose)
- [Eino Tools](https://github.com/cloudwego/eino/tree/main/components/tool)
- [Architecture Doc](../../docs/flow.md)
