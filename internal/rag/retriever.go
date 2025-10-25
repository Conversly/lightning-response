package rag

import (
    "context"
)

// Document represents a retrieved chunk/source
type Document struct {
    Title   string
    URL     string
    Snippet string
}

// Config contains minimal RAG settings for the retriever
type Config struct {
    TopK int
}

// Retriever abstracts retrieval of documents relevant to a query.
// Implementations can use pgvector or any other vector store.
type Retriever interface {
    Retrieve(ctx context.Context, tenantID string, query string, topK int) ([]Document, error)
}

// NoopRetriever is a placeholder implementation returning no documents.
type NoopRetriever struct{}

func NewNoopRetriever() *NoopRetriever { return &NoopRetriever{} }

func (n *NoopRetriever) Retrieve(ctx context.Context, tenantID string, query string, topK int) ([]Document, error) {
    return []Document{}, nil
}
