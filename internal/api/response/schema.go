package response

// Request defines the input contract for the /response endpoint
// Mirrors the architecture doc fields and allows future-safe extension via Metadata
type Request struct {
    Query   string        `json:"query"`
    Mode    string        `json:"mode"` // default | thinking | deep thinking
    User    RequestUser   `json:"user"`
    Metadata RequestMeta  `json:"metadata"`
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
// This is intentionally compact; expand as agent/tooling matures.
type Response struct {
    RequestID       string        `json:"request_id"`
    Mode            string        `json:"mode"`
    Answer          string        `json:"answer"`
    Sources         []Source      `json:"sources,omitempty"`
    Usage           *Usage        `json:"usage,omitempty"`
    ConversationKey string        `json:"conversation_key,omitempty"`
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
