package feedback

import "github.com/Conversly/lightning-response/internal/types"

type Request struct {
	MessageID string            `json:"messageId"`
	Feedback  string            `json:"feedback"` // like | dislike | neutral
	Comment   string            `json:"comment,omitempty"`
	User      types.RequestUser `json:"user"`
	Metadata  types.RequestMeta `json:"metadata"`
}

// Response is a minimal ack payload
type Response struct {
	types.BaseResponse
}
