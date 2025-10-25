package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Conversly/db-ingestor/internal/types"
)

// GeminiEmbedder handles embedding generation with rotating API keys
type GeminiEmbedder struct {
	apiKeys     []string
	client      *http.Client
	baseURL     string
	keyIndex    uint64        // atomic counter for round-robin key selection
	rateLimiter chan struct{} // global rate limiter across all workers
}

// NewGeminiEmbedder creates a new embedder with API keys
func NewGeminiEmbedder(keys []string) (*GeminiEmbedder, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("at least one API key is required")
	}
	maxConcurrentRequests := 5

	return &GeminiEmbedder{
		apiKeys:     keys,
		client:      &http.Client{Timeout: 30 * time.Second},
		baseURL:     "https://generativelanguage.googleapis.com/v1beta/models",
		keyIndex:    0,
		rateLimiter: make(chan struct{}, maxConcurrentRequests),
	}, nil
}

// getNextKey returns the next API key using round-robin selection
// Thread-safe: uses atomic operations to ensure fair distribution across goroutines
func (g *GeminiEmbedder) getNextKey() string {
	if len(g.apiKeys) == 1 {
		return g.apiKeys[0]
	}
	// Atomically increment and get the next index
	// This ensures thread-safe round-robin across all concurrent requests
	idx := atomic.AddUint64(&g.keyIndex, 1)
	// Use modulo to wrap around when we exceed the number of keys
	return g.apiKeys[idx%uint64(len(g.apiKeys))]
}

// normalize normalizes a vector to unit length
func normalize(vec []float64) []float64 {
	if len(vec) == 0 {
		return vec
	}

	norm := 0.0
	for _, v := range vec {
		norm += v * v
	}
	norm = math.Sqrt(norm)

	if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
		return vec
	}

	normalized := make([]float64, len(vec))
	for i, v := range vec {
		normalized[i] = v / norm
	}
	return normalized
}

func (g *GeminiEmbedder) EmbedText(ctx context.Context, text string) ([]float64, error) {
	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}

	select {
	case g.rateLimiter <- struct{}{}:
		defer func() { <-g.rateLimiter }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	reqBody := types.EmbeddingRequest{
		Model: "text-embedding-004",
		Content: types.EmbeddingContent{
			Parts: []types.Part{
				{Text: text},
			},
		},
		TaskType:            "RETRIEVAL_DOCUMENT",
		OutputDimensionality: 768,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiKey := g.getNextKey()
	url := fmt.Sprintf("%s/text-embedding-004:embedContent?key=%s", g.baseURL, apiKey)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var embeddingResp types.EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(embeddingResp.Embedding.Values) == 0 {
		return nil, fmt.Errorf("no embeddings returned from API")
	}

	if len(embeddingResp.Embedding.Values) != 768 {
		return nil, fmt.Errorf("expected 768 dimensions, got %d", len(embeddingResp.Embedding.Values))
	}

	normalized := normalize(embeddingResp.Embedding.Values)
	return normalized, nil
}

func (g *GeminiEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("no texts provided")
	}

	embeddings := make([][]float64, len(texts))
	errors := make([]error, len(texts))

	var wg sync.WaitGroup

	for i, text := range texts {
		wg.Add(1)
		go func(idx int, txt string) {
			defer wg.Done()
			embedding, err := g.EmbedText(ctx, txt)
			embeddings[idx] = embedding
			errors[idx] = err
		}(i, text)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			return nil, fmt.Errorf("failed to embed text at index %d: %w", i, err)
		}
	}

	return embeddings, nil
}
