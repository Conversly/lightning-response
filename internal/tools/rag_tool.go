package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/rag"
	"github.com/Conversly/lightning-response/internal/utils"
)

// RAGToolInput defines the expected input for the RAG tool
type RAGToolInput struct {
	Query string `json:"query" jsonschema:"required,description=The search query to find relevant information from the knowledge base"`
}

// RAGToolOutput defines the output structure
type RAGToolOutput struct {
	Results   []string `json:"results"`
	Citations []string `json:"citations"`
	Count     int      `json:"count"`
}

// RAGTool implements the Eino InvokableTool interface for knowledge base retrieval
type RAGTool struct {
	db        *loaders.PostgresClient
	embedder  *embedder.GeminiEmbedder
	chatbotID string
	topK      int
}

// NewRAGTool creates a new RAG tool instance
func NewRAGTool(db *loaders.PostgresClient, embedder *embedder.GeminiEmbedder, chatbotID string, topK int) *RAGTool {
	return &RAGTool{
		db:        db,
		embedder:  embedder,
		chatbotID: chatbotID,
		topK:      topK,
	}
}

// Info returns the tool's metadata for the LLM
func (r *RAGTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "search_knowledge_base",
		Desc: "Search the knowledge base for relevant information. Use this tool when you need to find specific information, facts, or context from the knowledge base to answer the user's question accurately. The tool returns relevant documents with citations.",
		ParamsOneOf: schema.NewParamsOneOfByParams(
			map[string]*schema.ParameterInfo{
				"query": {
					Type:     schema.String,
					Desc:     "The search query to find relevant information. Should be a clear, specific question or search phrase.",
					Required: true,
				},
			},
		),
	}, nil
}

// InvokableRun executes the RAG retrieval
func (r *RAGTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	// Parse input arguments
	var input RAGToolInput
	if err := json.Unmarshal([]byte(argumentsInJSON), &input); err != nil {
		utils.Zlog.Error("Failed to parse RAG tool arguments",
			zap.String("chatbot_id", r.chatbotID),
			zap.Error(err))
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	if input.Query == "" {
		return "", fmt.Errorf("query parameter is required")
	}

	utils.Zlog.Info("RAG tool invoked",
		zap.String("chatbot_id", r.chatbotID),
		zap.String("query", input.Query))

	// Create retriever and perform search
	retriever := rag.NewPgVectorRetriever(r.db, r.embedder, r.chatbotID, r.topK)
	results, err := retriever.Retrieve(ctx, input.Query)
	if err != nil {
		utils.Zlog.Error("RAG retrieval failed",
			zap.String("chatbot_id", r.chatbotID),
			zap.Error(err))
		return "", fmt.Errorf("retrieval failed: %w", err)
	}

	// Format results
	output := RAGToolOutput{
		Results:   make([]string, 0, len(results)),
		Citations: make([]string, 0, len(results)),
		Count:     len(results),
	}

	for i, res := range results {
		// Add content with index
		content := fmt.Sprintf("[%d] %s", i+1, res.Text)
		output.Results = append(output.Results, content)

		// Debug log each result
		utils.Zlog.Debug("Processing RAG result",
			zap.String("chatbot_id", r.chatbotID),
			zap.Int("index", i),
			zap.String("text_preview", func() string {
				if len(content) > 100 {
					return content[:100]
				}
				return content
			}()),
			zap.Bool("has_citation", res.Citation != nil),
			zap.String("citation", func() string {
				if res.Citation != nil {
					return *res.Citation
				}
				return "nil"
			}()))

		// Add citation if available
		if res.Citation != nil && *res.Citation != "" {
			output.Citations = append(output.Citations, *res.Citation)
			utils.Zlog.Debug("Added citation to output",
				zap.String("chatbot_id", r.chatbotID),
				zap.String("citation", *res.Citation))
		}
	}

	// Marshal output to JSON
	outputJSON, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("failed to marshal output: %w", err)
	}

	utils.Zlog.Info("RAG tool completed",
		zap.String("chatbot_id", r.chatbotID),
		zap.Int("results_count", len(results)))

	return string(outputJSON), nil
}

// Ensure RAGTool implements InvokableTool
var _ tool.InvokableTool = (*RAGTool)(nil)
