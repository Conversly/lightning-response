package channels

import (
	"context"

	"github.com/Conversly/lightning-response/internal/core"
)

// ChannelAdapter defines the interface for channel-specific implementations
type ChannelAdapter interface {
	// ValidateRequest validates the incoming request and returns the chatbot ID
	ValidateRequest(ctx context.Context, req interface{}) (chatbotID string, err error)

	// GetSystemPrompt returns the system prompt for this channel
	// May return a channel-specific override or fall back to default
	GetSystemPrompt(ctx context.Context, chatbotID string) (string, error)

	// GetChannel returns the channel type
	GetChannel() core.Channel

	// SaveMessages saves conversation messages for this channel
	SaveMessages(ctx context.Context, records ...core.MessageRecord) error
}

