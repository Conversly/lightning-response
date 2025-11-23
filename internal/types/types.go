package types

import "time"

type ChatbotTopic struct {
	ID        string
	Name      string
	Color     string
	CreatedAt time.Time
}

type ChatbotInfo struct {
	ID            string
	Name          string
	Description   string
	SystemPrompt  string
	Topics        []ChatbotTopic
	CustomActions []CustomAction // Added: custom actions for this chatbot
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

// ============================================
// Custom Actions Types
// ============================================

// HttpMethod represents HTTP methods
type HttpMethod string

const (
	MethodGET    HttpMethod = "GET"
	MethodPOST   HttpMethod = "POST"
	MethodPUT    HttpMethod = "PUT"
	MethodDELETE HttpMethod = "DELETE"
	MethodPATCH  HttpMethod = "PATCH"
)

// AuthType represents authentication types
type AuthType string

const (
	AuthNone   AuthType = "none"
	AuthBearer AuthType = "bearer"
	AuthAPIKey AuthType = "api_key"
	AuthBasic  AuthType = "basic"
)

// TestStatus represents test execution status
type TestStatus string

const (
	TestPassed    TestStatus = "passed"
	TestFailed    TestStatus = "failed"
	TestNotTested TestStatus = "not_tested"
)

// ParameterType represents JSON schema types
type ParameterType string

const (
	ParamString  ParameterType = "string"
	ParamNumber  ParameterType = "number"
	ParamInteger ParameterType = "integer"
	ParamBoolean ParameterType = "boolean"
	ParamArray   ParameterType = "array"
	ParamObject  ParameterType = "object"
)

// CustomActionConfig represents the API configuration stored in JSONB
type CustomActionConfig struct {
	Method          HttpMethod        `json:"method"`
	BaseURL         string            `json:"base_url"`
	Endpoint        string            `json:"endpoint"`
	Headers         map[string]string `json:"headers,omitempty"`
	QueryParams     map[string]string `json:"query_params,omitempty"`
	BodyTemplate    string            `json:"body_template,omitempty"`
	ResponseMapping string            `json:"response_mapping,omitempty"`
	SuccessCodes    []int             `json:"success_codes,omitempty"`
	TimeoutSeconds  int               `json:"timeout_seconds,omitempty"`
	RetryCount      int               `json:"retry_count,omitempty"`
	AuthType        AuthType          `json:"auth_type,omitempty"`
	AuthValue       string            `json:"auth_value,omitempty"`
	FollowRedirects bool              `json:"follow_redirects,omitempty"`
	VerifySSL       bool              `json:"verify_ssl,omitempty"`
}

// ToolParameter defines a parameter for the LLM tool
type ToolParameter struct {
	Name        string        `json:"name"`
	Type        ParameterType `json:"type"`
	Description string        `json:"description"`
	Required    bool          `json:"required,omitempty"`
	Default     string        `json:"default,omitempty"`
	Enum        []string      `json:"enum,omitempty"`
	Pattern     string        `json:"pattern,omitempty"`
	Minimum     *float64      `json:"minimum,omitempty"`
	Maximum     *float64      `json:"maximum,omitempty"`
	MinLength   *int          `json:"min_length,omitempty"`
	MaxLength   *int          `json:"max_length,omitempty"`
}

// ToolSchema represents JSON Schema format for tool parameters
type ToolSchema struct {
	Type       string                 `json:"type"` // Always "object"
	Properties map[string]interface{} `json:"properties"`
	Required   []string               `json:"required"`
}

// CustomAction represents a custom action configuration from the database
type CustomAction struct {
	ID           string                 `json:"id"`
	ChatbotID    string                 `json:"chatbot_id"`
	Name         string                 `json:"name"`
	DisplayName  string                 `json:"display_name"`
	Description  string                 `json:"description"`
	IsEnabled    bool                   `json:"is_enabled"`
	APIConfig    CustomActionConfig     `json:"api_config"`
	ToolSchema   ToolSchema             `json:"tool_schema"`
	Version      int                    `json:"version"`
	CreatedAt    *time.Time             `json:"created_at,omitempty"`
	UpdatedAt    *time.Time             `json:"updated_at,omitempty"`
	CreatedBy    *string                `json:"created_by,omitempty"`
	LastTestedAt *time.Time             `json:"last_tested_at,omitempty"`
	TestStatus   TestStatus             `json:"test_status"`
	TestResult   map[string]interface{} `json:"test_result,omitempty"`
}
