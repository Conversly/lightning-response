package response

import "github.com/Conversly/lightning-response/internal/types"

// Request defines the input contract for the /response endpoint
// Mirrors the architecture doc fields and allows future-safe extension via Metadata
// query contains the whole JSON array of previous conversation as a string

// [
//   { "role": "user", "content": "Hello!" },
//   { "role": "assistant", "content": "Hi there, how can I help you?" },
//   { "role": "user", "content": "Tell me a joke." },
//   { "role": "assistant", "content": "Why did the computer show up at work late? It had a hard drive." }
// ]

type PlaygroundChatbot struct {
	ChatbotId           string  `json:"chatbotId"`
	ChatbotSystemPrompt string  `json:"chatbotSystemPrompt"`
	ChatbotModel        string  `json:"chatbotModel"`
	ChatbotTemperature  float64 `json:"chatbotTemperature"`
}

type PlaygroundRequest struct {
	Query     string            `json:"query"`
	Mode      string            `json:"mode"` // default | thinking | deep thinking
	Chatbot   PlaygroundChatbot `json:"chatbot"`
	ChatbotId string            `json:"chatbotId"`
	User      types.RequestUser `json:"user"`
}

type Request struct {
	Query     string            `json:"query"`
	Mode      string            `json:"mode"` // default | thinking | deep thinking
	User      types.RequestUser `json:"user"`
	Metadata  types.RequestMeta `json:"metadata"`
	ChatbotID string            `json:"chatbotId"`
}

// Response defines a minimal structured response payload
// This matches the format specified in docs/new_flow.md
type Response struct {
	types.BaseResponse
	MessageID string   `json:"message_id,omitempty"`
	Response  string   `json:"response"`
	Citations []string `json:"citations"`
}

type Source struct {
	Title   string `json:"title,omitempty"`
	URL     string `json:"url,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

type Usage struct {
	PromptTokens     int   `json:"prompt_tokens,omitempty"`
	CompletionTokens int   `json:"completion_tokens,omitempty"`
	TotalTokens      int   `json:"total_tokens,omitempty"`
	LatencyMS        int64 `json:"latency_ms,omitempty"`
}
