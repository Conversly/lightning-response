package types

type EmbeddingRequest struct {
	Model               string           `json:"model"`
	Content             EmbeddingContent `json:"content"`
	TaskType            string           `json:"taskType,omitempty"`
	OutputDimensionality int             `json:"outputDimensionality,omitempty"`
}

// EmbeddingContent represents the content structure for embedding
type EmbeddingContent struct {
	Parts []Part `json:"parts"`
}

// Part represents a single part of the content
type Part struct {
	Text string `json:"text"`
}

// EmbeddingResponse represents the response from Gemini embedding API
type EmbeddingResponse struct {
	Embedding Embedding `json:"embedding"`
}

// Embedding represents the embedding vector
type Embedding struct {
	Values []float64 `json:"values"`
}