package response

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	einoUtils "github.com/cloudwego/eino/components/tool/utils"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/rag"
	internalUtils "github.com/Conversly/lightning-response/internal/utils"
)

var (
	globalDB       *loaders.PostgresClient
	globalEmbedder *embedder.GeminiEmbedder
)

type RAGToolRequest struct {
	Query string `json:"query" jsonschema:"required,description=The search query to find relevant documents"`
}

type RAGToolResponse struct {
	Content  string              `json:"content" jsonschema:"description=Retrieved document content"`
	Sources  []RAGSourceDocument `json:"sources" jsonschema:"description=Source documents"`
	DocCount int                 `json:"doc_count" jsonschema:"description=Number of documents retrieved"`
}

type RAGSourceDocument struct {
	Text     string  `json:"text,omitempty"`
	Citation *string `json:"citation,omitempty"`
}

// CreateRAGToolFromRetriever wraps a retriever as an Eino InvokableTool
// This allows the LLM to call the RAG retriever as a tool
func CreateRAGToolFromRetriever(retriever rag.Retriever, chatbotID string, _ string) (tool.InvokableTool, error) {
	// Define the RAG query function
	ragFunc := func(ctx context.Context, req *RAGToolRequest, opts ...tool.Option) (*RAGToolResponse, error) {
		internalUtils.Zlog.Debug("RAG tool invoked",
			zap.String("chatbot_id", chatbotID),
			zap.String("query", req.Query))

		// Call the actual retriever
		docs, err := retriever.Retrieve(ctx, req.Query)
		if err != nil {
			return nil, fmt.Errorf("retrieval failed: %w", err)
		}

		// Format documents into content and extract citations
		var content strings.Builder
		sources := make([]RAGSourceDocument, 0, len(docs))

		for i, doc := range docs {
			// Add numbered citation to content
			content.WriteString(fmt.Sprintf("[%d] %s\n\n", i+1, doc.Text))

			// Add source with proper types matching database schema
			sources = append(sources, RAGSourceDocument{
				Text:     doc.Text,
				Citation: doc.Citation,
			})
		}

		return &RAGToolResponse{
			Content:  content.String(),
			Sources:  sources,
			DocCount: len(sources),
		}, nil
	}

	// Create the tool using Eino's InferOptionableTool helper
	// This automatically generates the JSON schema from the struct tags
	ragTool, err := einoUtils.InferOptionableTool(
		fmt.Sprintf("query_knowledge_base_%s", chatbotID),
		fmt.Sprintf("Query the knowledge base for chatbot %s. Use this to find relevant information from the indexed documents. Returns content with citation URLs.", chatbotID),
		ragFunc,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create RAG tool: %w", err)
	}
	return ragTool, nil
}

type HTTPToolRequest struct {
	URL     string            `json:"url" jsonschema:"required,description=The API endpoint URL"`
	Method  string            `json:"method" jsonschema:"required,enum=GET,enum=POST,enum=PUT,enum=DELETE,description=HTTP method"`
	Headers map[string]string `json:"headers,omitempty" jsonschema:"description=HTTP headers"`
	Body    string            `json:"body,omitempty" jsonschema:"description=Request body (for POST/PUT)"`
}

func GetEnabledTools(ctx context.Context, cfg *ChatbotConfig) ([]tool.InvokableTool, error) {
	var tools []tool.InvokableTool
	var errs []error

	for _, toolName := range cfg.ToolConfigs {
		switch toolName {
		case "rag":
			retriever, err := getOrCreateRAGRetriever(ctx, cfg)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to create RAG retriever: %w", err))
				continue
			}

			ragTool, err := CreateRAGToolFromRetriever(retriever, cfg.ChatbotID, "")
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to create RAG tool: %w", err))
				continue
			}
			tools = append(tools, ragTool)
			internalUtils.Zlog.Debug("Added RAG tool", zap.String("chatbot_id", cfg.ChatbotID))

		default:
			internalUtils.Zlog.Warn("Unknown tool configuration",
				zap.String("tool", toolName),
				zap.String("chatbot_id", cfg.ChatbotID))
		}
	}

	if len(errs) > 0 && len(tools) == 0 {
		return nil, fmt.Errorf("failed to create any tools: %v", errs)
	}

	internalUtils.Zlog.Info("Enabled tools for chatbot",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int("tool_count", len(tools)))

	return tools, nil
}

// SetGlobalDependencies sets the global database and embedder for RAG tool creation
// This should be called during graph engine initialization
func SetGlobalDependencies(db *loaders.PostgresClient, embedder *embedder.GeminiEmbedder) {
	globalDB = db
	globalEmbedder = embedder
	internalUtils.Zlog.Info("Global dependencies set for RAG tools")
}

// getOrCreateRAGRetriever retrieves or creates a RAG retriever for a chatbot
func getOrCreateRAGRetriever(ctx context.Context, cfg *ChatbotConfig) (rag.Retriever, error) {
	// Check cache
	if cached, ok := retrieverCache.Load(cfg.ChatbotID); ok {
		internalUtils.Zlog.Debug("Using cached retriever", zap.String("chatbot_id", cfg.ChatbotID))
		return cached.(rag.Retriever), nil
	}

	internalUtils.Zlog.Info("Creating new RAG retriever",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.Int32("top_k", cfg.TopK))

	if globalDB == nil || globalEmbedder == nil {
		return nil, fmt.Errorf("global dependencies not set - call SetGlobalDependencies first")
	}

	// Create PgVector retriever with chatbot-specific configuration
	retriever := rag.NewPgVectorRetriever(
		globalDB,
		globalEmbedder,
		cfg.ChatbotID,
		int(cfg.TopK),
	)

	// Cache it
	retrieverCache.Store(cfg.ChatbotID, retriever)

	internalUtils.Zlog.Info("RAG retriever created and cached",
		zap.String("chatbot_id", cfg.ChatbotID))

	return retriever, nil
}
