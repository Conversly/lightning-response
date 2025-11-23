package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/types"
	"github.com/Conversly/lightning-response/internal/utils"
)

// CustomActionTool implements the Eino InvokableTool interface for custom HTTP actions
type CustomActionTool struct {
	action     *types.CustomAction
	httpClient *http.Client
}

// NewCustomActionTool creates a new custom action tool instance
func NewCustomActionTool(action *types.CustomAction) (*CustomActionTool, error) {
	// Validate action configuration
	if action == nil {
		return nil, fmt.Errorf("action cannot be nil")
	}
	if action.Name == "" {
		return nil, fmt.Errorf("action name cannot be empty")
	}
	if action.APIConfig.BaseURL == "" {
		return nil, fmt.Errorf("action base URL cannot be empty")
	}

	// Set default timeout if not specified
	timeout := 30
	if action.APIConfig.TimeoutSeconds > 0 {
		timeout = action.APIConfig.TimeoutSeconds
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !action.APIConfig.FollowRedirects && len(via) > 0 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	return &CustomActionTool{
		action:     action,
		httpClient: httpClient,
	}, nil
}

// Info returns the tool's metadata for the LLM
func (t *CustomActionTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	// Convert tool schema properties to Eino ParameterInfo format
	params := make(map[string]*schema.ParameterInfo)

	properties := t.action.ToolSchema.Properties
	if properties == nil {
		return nil, fmt.Errorf("tool schema properties cannot be nil")
	}

	for paramName, paramDefRaw := range properties {
		paramDef, ok := paramDefRaw.(map[string]interface{})
		if !ok {
			continue
		}

		paramInfo := &schema.ParameterInfo{
			Desc: getStringField(paramDef, "description"),
		}

		// Convert type
		paramType := getStringField(paramDef, "type")
		switch paramType {
		case "string":
			paramInfo.Type = schema.String
		case "number", "integer":
			paramInfo.Type = schema.Number
		case "boolean":
			paramInfo.Type = schema.Boolean
		case "array":
			paramInfo.Type = schema.Array
		case "object":
			paramInfo.Type = schema.Object
		default:
			paramInfo.Type = schema.String
		}

		// Check if required
		paramInfo.Required = contains(t.action.ToolSchema.Required, paramName)

		params[paramName] = paramInfo
	}

	return &schema.ToolInfo{
		Name:        t.action.Name,
		Desc:        t.action.Description,
		ParamsOneOf: schema.NewParamsOneOfByParams(params),
	}, nil
}

// InvokableRun executes the HTTP request
func (t *CustomActionTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	startTime := time.Now()

	// Parse input arguments
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(argumentsInJSON), &params); err != nil {
		utils.Zlog.Error("Failed to parse custom action arguments",
			zap.String("action_name", t.action.Name),
			zap.Error(err))
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	utils.Zlog.Info("Custom action tool invoked",
		zap.String("action_name", t.action.Name),
		zap.String("chatbot_id", t.action.ChatbotID),
		zap.Any("params", params))

	// Execute with retry logic
	maxRetries := 1
	if t.action.APIConfig.RetryCount > 0 {
		maxRetries = t.action.APIConfig.RetryCount + 1
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s...
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			utils.Zlog.Info("Retrying custom action",
				zap.String("action_name", t.action.Name),
				zap.Int("attempt", attempt+1),
				zap.Duration("backoff", backoff))
			time.Sleep(backoff)
		}

		result, err := t.executeRequest(ctx, params)
		if err == nil {
			duration := time.Since(startTime)
			utils.Zlog.Info("Custom action completed successfully",
				zap.String("action_name", t.action.Name),
				zap.Duration("duration", duration),
				zap.Int("attempt", attempt+1))
			return result, nil
		}

		lastErr = err
		utils.Zlog.Warn("Custom action attempt failed",
			zap.String("action_name", t.action.Name),
			zap.Int("attempt", attempt+1),
			zap.Error(err))
	}

	utils.Zlog.Error("Custom action failed after all retries",
		zap.String("action_name", t.action.Name),
		zap.Int("max_retries", maxRetries),
		zap.Error(lastErr))

	return "", fmt.Errorf("action failed after %d attempts: %w", maxRetries, lastErr)
}

// executeRequest performs the actual HTTP request
func (t *CustomActionTool) executeRequest(ctx context.Context, params map[string]interface{}) (string, error) {
	// Build URL with parameter interpolation
	url := t.buildURL(params)

	// Build request body if needed
	var body io.Reader
	if t.action.APIConfig.Method != types.MethodGET && t.action.APIConfig.BodyTemplate != "" {
		bodyStr := t.interpolateParams(t.action.APIConfig.BodyTemplate, params)
		body = bytes.NewBufferString(bodyStr)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, string(t.action.APIConfig.Method), url, body)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	t.addHeaders(req, params)

	// Add authentication
	t.addAuthentication(req)

	// Execute request
	utils.Zlog.Debug("Executing HTTP request",
		zap.String("action_name", t.action.Name),
		zap.String("method", string(t.action.APIConfig.Method)),
		zap.String("url", url))

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check if status code is successful
	if !t.isSuccessStatusCode(resp.StatusCode) {
		return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(responseBody))
	}

	// Apply response mapping if configured
	result := string(responseBody)
	if t.action.APIConfig.ResponseMapping != "" {
		mapped, err := t.applyResponseMapping(responseBody, t.action.APIConfig.ResponseMapping)
		if err != nil {
			utils.Zlog.Warn("Failed to apply response mapping, returning raw response",
				zap.String("action_name", t.action.Name),
				zap.Error(err))
		} else {
			result = mapped
		}
	}

	utils.Zlog.Debug("HTTP request completed",
		zap.String("action_name", t.action.Name),
		zap.Int("status_code", resp.StatusCode),
		zap.Int("response_length", len(result)))

	return result, nil
}

// buildURL constructs the full URL with parameter interpolation
func (t *CustomActionTool) buildURL(params map[string]interface{}) string {
	// Start with base URL + endpoint
	url := strings.TrimSuffix(t.action.APIConfig.BaseURL, "/")
	endpoint := t.interpolateParams(t.action.APIConfig.Endpoint, params)
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}
	url = url + endpoint

	// Add query parameters
	if len(t.action.APIConfig.QueryParams) > 0 {
		queryParts := make([]string, 0, len(t.action.APIConfig.QueryParams))
		for key, value := range t.action.APIConfig.QueryParams {
			interpolatedValue := t.interpolateParams(value, params)
			queryParts = append(queryParts, fmt.Sprintf("%s=%s", key, interpolatedValue))
		}
		url = url + "?" + strings.Join(queryParts, "&")
	}

	return url
}

// interpolateParams replaces {{param_name}} placeholders with actual values
func (t *CustomActionTool) interpolateParams(template string, params map[string]interface{}) string {
	result := template
	for key, value := range params {
		placeholder := "{{" + key + "}}"
		replacement := fmt.Sprint(value)
		result = strings.ReplaceAll(result, placeholder, replacement)
	}
	return result
}

// addHeaders adds configured headers to the request
func (t *CustomActionTool) addHeaders(req *http.Request, params map[string]interface{}) {
	// Add default content type for POST/PUT/PATCH
	if t.action.APIConfig.Method == types.MethodPOST ||
		t.action.APIConfig.Method == types.MethodPUT ||
		t.action.APIConfig.Method == types.MethodPATCH {
		req.Header.Set("Content-Type", "application/json")
	}

	// Add configured headers
	for key, value := range t.action.APIConfig.Headers {
		interpolatedValue := t.interpolateParams(value, params)
		req.Header.Set(key, interpolatedValue)
	}
}

// addAuthentication adds authentication to the request
func (t *CustomActionTool) addAuthentication(req *http.Request) {
	if t.action.APIConfig.AuthType == types.AuthNone || t.action.APIConfig.AuthValue == "" {
		return
	}

	switch t.action.APIConfig.AuthType {
	case types.AuthBearer:
		req.Header.Set("Authorization", "Bearer "+t.action.APIConfig.AuthValue)
	case types.AuthAPIKey:
		// Default to X-API-Key header, but can be overridden in headers config
		if req.Header.Get("X-API-Key") == "" {
			req.Header.Set("X-API-Key", t.action.APIConfig.AuthValue)
		}
	case types.AuthBasic:
		// AuthValue should be in format "username:password"
		req.Header.Set("Authorization", "Basic "+t.action.APIConfig.AuthValue)
	}
}

// isSuccessStatusCode checks if the status code is considered successful
func (t *CustomActionTool) isSuccessStatusCode(statusCode int) bool {
	// If success codes are specified, use them
	if len(t.action.APIConfig.SuccessCodes) > 0 {
		for _, code := range t.action.APIConfig.SuccessCodes {
			if code == statusCode {
				return true
			}
		}
		return false
	}

	// Default: 2xx status codes are successful
	return statusCode >= 200 && statusCode < 300
}

// applyResponseMapping extracts specific fields from JSON response
func (t *CustomActionTool) applyResponseMapping(responseBody []byte, mapping string) (string, error) {
	// Parse response as JSON
	var responseData interface{}
	if err := json.Unmarshal(responseBody, &responseData); err != nil {
		return "", fmt.Errorf("response is not valid JSON: %w", err)
	}

	// mapping can be a JSONPath-like expression, e.g., "data.result"
	// For simplicity, we'll support dot notation for nested objects
	parts := strings.Split(mapping, ".")
	current := responseData

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = v[part]
			if !ok {
				return "", fmt.Errorf("field '%s' not found in response", part)
			}
		default:
			return "", fmt.Errorf("cannot traverse '%s' in non-object value", part)
		}
	}

	// Convert result to JSON string
	result, err := json.Marshal(current)
	if err != nil {
		return "", fmt.Errorf("failed to marshal mapped result: %w", err)
	}

	return string(result), nil
}

// Helper functions

func getStringField(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
