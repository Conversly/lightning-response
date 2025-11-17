package types

import "time"

type ChatbotTopic struct {
	ID        string
	Name      string
	Color     string
	CreatedAt time.Time
}

type ChatbotInfo struct {
	ID           string
	Name         string
	Description  string
	SystemPrompt string
	Topics       []ChatbotTopic
}

// RequestUser represents common user identity and metadata for API requests.
// Fields are optional so that different endpoints can populate only what they need.
type RequestUser struct {
	UniqueClientID string                 `json:"uniqueClientId,omitempty"`
	ConverslyWebID string                 `json:"converslyWebId,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// RequestMeta captures common metadata attached to API requests.
type RequestMeta struct {
	OriginURL string `json:"originUrl"`
}

// BaseResponse defines common fields returned by API responses.
type BaseResponse struct {
	RequestID string `json:"request_id,omitempty"`
	Success   bool   `json:"success"`
}
