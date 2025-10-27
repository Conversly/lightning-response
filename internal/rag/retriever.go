package rag

import (
	"context"
	"fmt"

	"github.com/Conversly/lightning-response/internal/embedder"
	"github.com/Conversly/lightning-response/internal/loaders"
)

type Config struct {
	ChatbotID string
	TopK      int
}

type Retriever interface {
	Retrieve(ctx context.Context, query string) ([]loaders.EmbeddingResult, error)
}

type NoopRetriever struct{}

func NewNoopRetriever() *NoopRetriever { return &NoopRetriever{} }

func (n *NoopRetriever) Retrieve(ctx context.Context, query string) ([]loaders.EmbeddingResult, error) {
	return []loaders.EmbeddingResult{}, nil
}

// PgVectorRetriever retrieves documents from PostgreSQL using pgvector
type PgVectorRetriever struct {
	db        *loaders.PostgresClient
	embedder  *embedder.GeminiEmbedder
	chatbotID string
	topK      int
}

// NewPgVectorRetriever creates a new retriever with chatbotID and topK configured at initialization
func NewPgVectorRetriever(db *loaders.PostgresClient, embedder *embedder.GeminiEmbedder, chatbotID string, topK int) *PgVectorRetriever {
	return &PgVectorRetriever{
		db:        db,
		embedder:  embedder,
		chatbotID: chatbotID,
		topK:      topK,
	}
}

// Retrieve searches for relevant documents using the query
func (r *PgVectorRetriever) Retrieve(ctx context.Context, query string) ([]loaders.EmbeddingResult, error) {
	queryEmbedding, err := r.embedder.EmbedText(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	results, err := r.db.SearchEmbeddings(ctx, r.chatbotID, queryEmbedding, r.topK)
	if err != nil {
		return nil, fmt.Errorf("failed to search embeddings: %w", err)
	}

	return results, nil
}
