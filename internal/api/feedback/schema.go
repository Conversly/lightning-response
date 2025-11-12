package feedback

// Request mirrors the access envelope of /response and carries feedback details
type Request struct {
	MessageID string      `json:"messageId"`
	Feedback  string      `json:"feedback"` // like | dislike | neutral
	Comment   string      `json:"comment,omitempty"`
	User      RequestUser `json:"user"`
	Metadata  RequestMeta `json:"metadata"`
}

type RequestUser struct {
	ConverslyWebID string                 `json:"converslyWebId"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

type RequestMeta struct {
	OriginURL string `json:"originUrl"`
}

// Response is a minimal ack payload
type Response struct {
	RequestID string `json:"request_id,omitempty"`
	Success   bool   `json:"success"`
}
