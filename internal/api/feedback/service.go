package feedback

import (
	"context"
	"fmt"

	"github.com/Conversly/lightning-response/internal/api/channels/widget"
	"github.com/Conversly/lightning-response/internal/loaders"
)

type Service struct {
	db *loaders.PostgresClient
}

func NewService(db *loaders.PostgresClient) *Service {
	return &Service{db: db}
}

// SubmitFeedback validates access and updates the message feedback
func (s *Service) SubmitFeedback(ctx context.Context, req *Request) error {
	if req == nil {
		return fmt.Errorf("nil request")
	}

	chatbotID, err := widget.ValidateChatbotAccess(ctx, req.User.ConverslyWebID, req.Metadata.OriginURL)
	if err != nil {
		return fmt.Errorf("access validation failed: %w", err)
	}

	var val int16
	switch req.Feedback {
	case "like":
		val = 1
	case "dislike":
		val = 2
	case "neutral":
		val = 3
	case "", "none":
		val = 0
	default:
		return fmt.Errorf("invalid feedback: %s", req.Feedback)
	}

	var commentPtr *string
	if req.Comment != "" {
		commentPtr = &req.Comment
	}

	return s.db.UpdateMessageFeedback(ctx, chatbotID, req.MessageID, val, commentPtr)
}
