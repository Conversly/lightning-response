package response

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	internalUtils "github.com/Conversly/lightning-response/internal/utils"
)

// RAGToolRequest defines the input schema for the RAG tool
type RAGToolRequest struct {
	Query string `json:"query" jsonschema:"required,description=The search query to find relevant documents"`
}

// RAGToolResponse defines the output schema for the RAG tool
type RAGToolResponse struct {
	Content  string              `json:"content" jsonschema:"description=Retrieved document content"`
	Sources  []RAGSourceDocument `json:"sources" jsonschema:"description=Source documents"`
	DocCount int                 `json:"doc_count" jsonschema:"description=Number of documents retrieved"`
}

// RAGSourceDocument represents a single source document from RAG
type RAGSourceDocument struct {
	Title   string  `json:"title,omitempty"`
	URL     string  `json:"url,omitempty"`
	Snippet string  `json:"snippet,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

// CreateRAGToolFromRetriever wraps a retriever as an Eino InvokableTool
// This allows the LLM to call the RAG retriever as a tool
func CreateRAGToolFromRetriever(retriever interface{}, chatbotID string, ragIndex string) (tool.InvokableTool, error) {
	// Define the RAG query function
	ragFunc := func(ctx context.Context, req *RAGToolRequest, opts ...tool.Option) (*RAGToolResponse, error) {
		internalUtils.Zlog.Debug("RAG tool invoked",
			zap.String("chatbot_id", chatbotID),
			zap.String("query", req.Query))

		// TODO: Call the actual retriever
		// For now, return a placeholder response
		// 
		// Example implementation:
		// docs, err := retriever.Retrieve(ctx, req.Query)
		// if err != nil {
		//     return nil, fmt.Errorf("retrieval failed: %w", err)
		// }

		// Placeholder response
		sources := []RAGSourceDocument{
			{
				Title:   "Document 1",
				URL:     "https://example.com/doc1",
				Snippet: "This is a placeholder document snippet for: " + req.Query,
				Score:   0.95,
			},
		}

		content := "Placeholder RAG content for query: " + req.Query
		
		return &RAGToolResponse{
			Content:  content,
			Sources:  sources,
			DocCount: len(sources),
		}, nil
	}

	// Create the tool using Eino's InferOptionableTool helper
	// This automatically generates the JSON schema from the struct tags
	ragTool := utils.InferOptionableTool(
		fmt.Sprintf("query_knowledge_base_%s", chatbotID),
		fmt.Sprintf("Query the knowledge base for chatbot %s. Use this to find relevant information from the indexed documents.", chatbotID),
		ragFunc,
	)

	return ragTool, nil
}

// HTTPToolRequest defines input for generic HTTP API calls
type HTTPToolRequest struct {
	URL     string            `json:"url" jsonschema:"required,description=The API endpoint URL"`
	Method  string            `json:"method" jsonschema:"required,enum=GET,enum=POST,enum=PUT,enum=DELETE,description=HTTP method"`
	Headers map[string]string `json:"headers,omitempty" jsonschema:"description=HTTP headers"`
	Body    string            `json:"body,omitempty" jsonschema:"description=Request body (for POST/PUT)"`
}

// HTTPToolResponse defines output from HTTP API calls
type HTTPToolResponse struct {
	StatusCode int               `json:"status_code" jsonschema:"description=HTTP status code"`
	Body       string            `json:"body" jsonschema:"description=Response body"`
	Headers    map[string]string `json:"headers,omitempty" jsonschema:"description=Response headers"`
}

// CreateHTTPTool creates a generic HTTP API calling tool
func CreateHTTPTool() (tool.InvokableTool, error) {
	httpFunc := func(ctx context.Context, req *HTTPToolRequest, opts ...tool.Option) (*HTTPToolResponse, error) {
		utils.Zlog.Debug("HTTP tool invoked",
			zap.String("url", req.URL),
			zap.String("method", req.Method))

		// TODO: Implement actual HTTP call
		// For now, return placeholder
		//
		// Example:
		// client := &http.Client{Timeout: 10 * time.Second}
		// httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, strings.NewReader(req.Body))
		// ...

		return &HTTPToolResponse{
			StatusCode: 200,
			Body:       fmt.Sprintf("Placeholder response for %s %s", req.Method, req.URL),
			Headers:    map[string]string{"Content-Type": "application/json"},
		}, nil
	}

	return utils.InferOptionableTool(
		"http_api_call",
		"Make HTTP API calls to external services. Use this when you need to fetch data from external APIs.",
		httpFunc,
	), nil
}

// SearchToolRequest defines input for web search
type SearchToolRequest struct {
	Query   string `json:"query" jsonschema:"required,description=Search query"`
	MaxResults int `json:"max_results,omitempty" jsonschema:"description=Maximum number of results (default: 5)"`
}

// SearchToolResponse defines output from search
type SearchToolResponse struct {
	Results []SearchResult `json:"results" jsonschema:"description=Search results"`
}

// SearchResult represents a single search result
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// CreateSearchTool creates a web search tool
func CreateSearchTool() (tool.InvokableTool, error) {
	searchFunc := func(ctx context.Context, req *SearchToolRequest, opts ...tool.Option) (*SearchToolResponse, error) {
		utils.Zlog.Debug("Search tool invoked", zap.String("query", req.Query))

		// TODO: Implement actual search
		// For now, return placeholder
		maxResults := req.MaxResults
		if maxResults == 0 {
			maxResults = 5
		}

		results := []SearchResult{
			{
				Title:   "Search Result 1",
				URL:     "https://example.com/result1",
				Snippet: "Placeholder search result for: " + req.Query,
			},
		}

		return &SearchToolResponse{
			Results: results,
		}, nil
	}

	return utils.InferOptionableTool(
		"search_web",
		"Search the web for information. Use this when you need to find current information not in the knowledge base.",
		searchFunc,
	), nil
}

// GetEnabledTools returns a list of tools based on the chatbot configuration
func GetEnabledTools(ctx context.Context, cfg *ChatbotConfig) ([]tool.Tool, error) {
	var tools []tool.Tool
	var errs []error

	for _, toolName := range cfg.ToolConfigs {
		switch toolName {
		case "rag":
			// Get or create RAG retriever for this chatbot
			retriever, err := getOrCreateRAGRetriever(ctx, cfg)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to create RAG retriever: %w", err))
				continue
			}

			ragTool, err := CreateRAGToolFromRetriever(retriever, cfg.ChatbotID, cfg.RAGIndex)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to create RAG tool: %w", err))
				continue
			}
			tools = append(tools, ragTool)
			utils.Zlog.Debug("Added RAG tool", zap.String("chatbot_id", cfg.ChatbotID))

		case "http_api":
			if httpTool, ok := globalTools["http_api"].(tool.InvokableTool); ok {
				tools = append(tools, httpTool)
				utils.Zlog.Debug("Added HTTP tool", zap.String("chatbot_id", cfg.ChatbotID))
			} else {
				// Create on-demand if not in global cache
				httpTool, err := CreateHTTPTool()
				if err != nil {
					errs = append(errs, fmt.Errorf("failed to create HTTP tool: %w", err))
					continue
				}
				globalTools["http_api"] = httpTool
				tools = append(tools, httpTool)
			}

		case "search":
			if searchTool, ok := globalTools["search"].(tool.InvokableTool); ok {
				tools = append(tools, searchTool)
				utils.Zlog.Debug("Added search tool", zap.String("chatbot_id", cfg.ChatbotID))
			} else {
				// Create on-demand
				searchTool, err := CreateSearchTool()
				if err != nil {
					errs = append(errs, fmt.Errorf("failed to create search tool: %w", err))
					continue
				}
				globalTools["search"] = searchTool
				tools = append(tools, searchTool)
			}

		default:
			utils.Zlog.Warn("Unknown tool configuration",
				zap.String("tool", toolName),
				zap.String("chatbot_id", cfg.ChatbotID))
		}
	}

	if len(errs) > 0 && len(tools) == 0 {
		return nil, fmt.Errorf("failed to create any tools: %v", errs)
	}

	utils.Zlog.Info("Enabled tools for chatbot",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int("tool_count", len(tools)))

	return tools, nil
}

// getOrCreateRAGRetriever retrieves or creates a RAG retriever for a chatbot
func getOrCreateRAGRetriever(ctx context.Context, cfg *ChatbotConfig) (interface{}, error) {
	// Check cache
	if cached, ok := retrieverCache.Load(cfg.ChatbotID); ok {
		utils.Zlog.Debug("Using cached retriever", zap.String("chatbot_id", cfg.ChatbotID))
		return cached, nil
	}

	utils.Zlog.Info("Creating new RAG retriever",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.String("rag_index", cfg.RAGIndex))

	// TODO: Create actual retriever
	// Example with Viking DB:
	// retriever, err := volc_vikingdb.NewRetriever(ctx, &volc_vikingdb.RetrieverConfig{
	//     Host:       "api-vikingdb.volces.com",
	//     Region:     "cn-beijing",
	//     AK:         os.Getenv("VIKING_AK"),
	//     SK:         os.Getenv("VIKING_SK"),
	//     Collection: cfg.RAGIndex,
	//     Index:      "default_index",
	//     TopK:       &cfg.TopK,
	// })
	// if err != nil {
	//     return nil, fmt.Errorf("failed to create Viking retriever: %w", err)
	// }

	// Placeholder: return a dummy retriever
	retriever := &placeholderRetriever{
		chatbotID: cfg.ChatbotID,
		ragIndex:  cfg.RAGIndex,
	}

	// Cache it
	retrieverCache.Store(cfg.ChatbotID, retriever)

	return retriever, nil
}

// placeholderRetriever is a temporary stub until real retriever is wired
type placeholderRetriever struct {
	chatbotID string
	ragIndex  string
}

func (p *placeholderRetriever) Retrieve(ctx context.Context, query string) ([]*schema.Document, error) {
	return []*schema.Document{
		{
			ID:      "doc1",
			Content: "Placeholder document for: " + query,
		},
	}, nil
}
