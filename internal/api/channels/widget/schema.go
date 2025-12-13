package widget

import "github.com/Conversly/lightning-response/internal/types"

// Request defines the input contract for the /response endpoint
// query contains the whole JSON array of previous conversation as a string
//
// Example:
// [
//
//	{ "role": "user", "content": "Hello!" },
//	{ "role": "assistant", "content": "Hi there, how can I help you?" },
//	{ "role": "user", "content": "Tell me a joke." }
//
// ]
type Request struct {
	Query     string            `json:"query"`
	Mode      string            `json:"mode"` // default | thinking | deep thinking
	User      types.RequestUser `json:"user"`
	Metadata  types.RequestMeta `json:"metadata"`
	ChatbotID string            `json:"chatbotId"`
}

// PlaygroundChatbot contains chatbot configuration for playground requests
type PlaygroundChatbot struct {
	ChatbotId           string  `json:"chatbotId"`
	ChatbotSystemPrompt string  `json:"chatbotSystemPrompt"`
	ChatbotModel        string  `json:"chatbotModel"`
	ChatbotTemperature  float64 `json:"chatbotTemperature"`
}

// PlaygroundRequest defines the input for playground endpoint
type PlaygroundRequest struct {
	Query     string            `json:"query"`
	Mode      string            `json:"mode"` // default | thinking | deep thinking
	Chatbot   PlaygroundChatbot `json:"chatbot"`
	ChatbotId string            `json:"chatbotId"`
	User      types.RequestUser `json:"user"`
}

// Response defines a minimal structured response payload
type Response struct {
	types.BaseResponse
	MessageID string   `json:"message_id,omitempty"`
	Response  string   `json:"response"`
	Citations []string `json:"citations"`
}

// Source represents a citation source
type Source struct {
	Title   string `json:"title,omitempty"`
	URL     string `json:"url,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

// Usage tracks token usage and latency
type Usage struct {
	PromptTokens     int   `json:"prompt_tokens,omitempty"`
	CompletionTokens int   `json:"completion_tokens,omitempty"`
	TotalTokens      int   `json:"total_tokens,omitempty"`
	LatencyMS        int64 `json:"latency_ms,omitempty"`
}

