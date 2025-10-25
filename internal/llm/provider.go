package llm

import (
    "context"
)

// Message is a minimal chat message format for the provider
type Message struct {
    Role    string // system | user | assistant
    Content string
}

// Usage captures token/latency accounting if available
type Usage struct {
    PromptTokens     int
    CompletionTokens int
    TotalTokens      int
}

// ModelConfig contains per-request model settings
type ModelConfig struct {
    Model        string
    Temperature  float32
    SystemPrompt string
}

// Provider abstracts the LLM call; implementations can wrap Eino models.
type Provider interface {
    Generate(ctx context.Context, messages []Message, cfg ModelConfig) (text string, usage Usage, err error)
}

// NoopProvider returns a canned response for skeleton wiring.
type NoopProvider struct{}

func NewNoopProvider() *NoopProvider { return &NoopProvider{} }

func (n *NoopProvider) Generate(ctx context.Context, messages []Message, cfg ModelConfig) (string, Usage, error) {
    return "This is a stubbed LLM response.", Usage{}, nil
}
