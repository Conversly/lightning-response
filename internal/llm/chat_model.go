package llm

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
	"google.golang.org/genai"

	"github.com/Conversly/lightning-response/internal/utils"
)

// MultiKeyChatModel wraps multiple Gemini chat models with round-robin key rotation
// This distributes API requests across multiple keys to avoid rate limits
type MultiKeyChatModel struct {
	models   []model.ToolCallingChatModel
	keyIndex uint64 // atomic counter for round-robin selection
}

// NewMultiKeyChatModel creates a chat model that rotates between multiple API keys
func NewMultiKeyChatModel(ctx context.Context, apiKeys []string, modelName string, temperature *float32, maxTokens *int) (*MultiKeyChatModel, error) {
	if len(apiKeys) == 0 {
		return nil, fmt.Errorf("at least one API key is required")
	}

	models := make([]model.ToolCallingChatModel, len(apiKeys))

	for i, key := range apiKeys {
		// Create Gemini client for this key
		client, err := genai.NewClient(ctx, &genai.ClientConfig{
			APIKey: key,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create Gemini client for key %d: %w", i+1, err)
		}

		// Create chat model with this client
		chatModel, err := gemini.NewChatModel(ctx, &gemini.Config{
			Client:      client,
			Model:       modelName,
			Temperature: temperature,
			MaxTokens:   maxTokens,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create chat model for key %d: %w", i+1, err)
		}

		models[i] = chatModel
	}

	utils.Zlog.Info("Created multi-key chat model with round-robin rotation",
		zap.Int("key_count", len(apiKeys)),
		zap.String("model", modelName))

	return &MultiKeyChatModel{
		models:   models,
		keyIndex: 0,
	}, nil
}

// getNextModel returns the next model using round-robin selection
// Thread-safe: uses atomic operations to ensure fair distribution
func (m *MultiKeyChatModel) getNextModel() model.ToolCallingChatModel {
	if len(m.models) == 1 {
		return m.models[0]
	}
	idx := atomic.AddUint64(&m.keyIndex, 1)
	return m.models[idx%uint64(len(m.models))]
}

// Generate implements model.ChatModel interface
func (m *MultiKeyChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	return m.getNextModel().Generate(ctx, input, opts...)
}

// Stream implements model.ChatModel interface
func (m *MultiKeyChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return m.getNextModel().Stream(ctx, input, opts...)
}

// WithTools implements model.ToolCallingChatModel interface
func (m *MultiKeyChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	newModels := make([]model.ToolCallingChatModel, len(m.models))

	for i, chatModel := range m.models {
		modelWithTools, err := chatModel.WithTools(tools)
		if err != nil {
			return nil, fmt.Errorf("failed to bind tools to model %d: %w", i+1, err)
		}
		newModels[i] = modelWithTools
	}

	return &MultiKeyChatModel{
		models:   newModels,
		keyIndex: m.keyIndex,
	}, nil
}

// BindTools implements model.ToolCallingChatModel interface
func (m *MultiKeyChatModel) BindTools(tools []*schema.ToolInfo) error {
	// Create new models with tools bound
	newModels := make([]model.ToolCallingChatModel, len(m.models))

	for i, chatModel := range m.models {
		modelWithTools, err := chatModel.WithTools(tools)
		if err != nil {
			return fmt.Errorf("failed to bind tools to model %d: %w", i+1, err)
		}
		newModels[i] = modelWithTools
	}

	// Replace models with tool-bound versions
	m.models = newModels
	return nil
}
