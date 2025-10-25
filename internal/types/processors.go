package types

import (
    "context"
    "time"
)

type Processor interface {
    Process(ctx context.Context, chatbotID, userID string) (*ProcessedContent, error)
    GetSourceType() SourceType
}

// ProcessedContent represents the processed content with chunks
type ProcessedContent struct {
    SourceType  SourceType             `json:"sourceType"`
    Content     string                 `json:"content"`
    Topic       string                 `json:"topic"`
    Chunks      []ContentChunk         `json:"chunks"`
    Metadata    map[string]interface{} `json:"metadata"`
    ProcessedAt time.Time              `json:"processedAt"`
}

type ContentChunk struct {
    DatasourceID int                    `json:"datasourceId,omitempty"`
    Content      string                 `json:"content"`
    Embedding    []float64              `json:"embedding,omitempty"`
    Metadata     map[string]interface{} `json:"metadata,omitempty"`
    ChunkIndex   int                    `json:"chunkIndex"`
}

// Config holds configuration for processors
type Config struct {
    ChunkSize    int
    ChunkOverlap int
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
    return &Config{
        ChunkSize:    1000,
        ChunkOverlap: 200,
    }
}