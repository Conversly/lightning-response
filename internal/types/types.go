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

// ToolParameter represents a single parameter for a custom action/tool
type ToolParameter struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// CustomAction represents an action loaded from the database
type CustomAction struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	DisplayName string                 `json:"display_name"`
	Description string                 `json:"description"`
	IsEnabled   bool                   `json:"is_enabled"`
	APIConfig   map[string]interface{} `json:"api_config"`
	ToolSchema  map[string]interface{} `json:"tool_schema"`
	Parameters  []ToolParameter        `json:"parameters"`
}
