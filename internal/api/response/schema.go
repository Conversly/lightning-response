package response

// Request defines the input contract for the /response endpoint
// Mirrors the architecture doc fields and allows future-safe extension via Metadata
// query : 

// [
//   { "role": "user", "content": "Hello!" },
//   { "role": "assistant", "content": "Hi there, how can I help you?" },
//   { "role": "user", "content": "Tell me a joke." },
//   { "role": "assistant", "content": "Why did the computer show up at work late? It had a hard drive." }
// ]

type Request struct {
    Query   map[string]interface{} `json:"query"`
    Mode    string                 `json:"mode"` // default | thinking | deep thinking
    User    RequestUser            `json:"user"`
    Metadata RequestMeta           `json:"metadata"`
}

type RequestUser struct {
    UniqueClientID string                 `json:"unique_client_id"`
    ConverslyWebID string                 `json:"conversly_web_id"`
    Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

type RequestMeta struct {
    OriginURL string `json:"origin_url"`
}

// Response defines a minimal structured response payload
// This matches the format specified in docs/new_flow.md
type Response struct {
	RequestID string   `json:"request_id,omitempty"`
	Response  string   `json:"response"`
	Citations []string `json:"citations"`
	Success   bool     `json:"success"`
}

type Source struct {
	Title   string  `json:"title,omitempty"`
	URL     string  `json:"url,omitempty"`
	Snippet string  `json:"snippet,omitempty"`
}

type Usage struct {
    PromptTokens     int   `json:"prompt_tokens,omitempty"`
    CompletionTokens int   `json:"completion_tokens,omitempty"`
    TotalTokens      int   `json:"total_tokens,omitempty"`
    LatencyMS        int64 `json:"latency_ms,omitempty"`
}





