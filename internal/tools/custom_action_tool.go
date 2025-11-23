package tools

import (
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

	"github.com/Conversly/lightning-response/internal/utils"
)

// CustomActionTool is an invokable tool that executes an HTTP action defined in DB
type CustomActionTool struct {
	id        string
	name      string
	desc      string
	chatbotID string
	apiConfig map[string]interface{}
}

// NewCustomActionTool creates a new instance
func NewCustomActionTool(actionID, name, desc, chatbotID string, apiConfig map[string]interface{}) *CustomActionTool {
	return &CustomActionTool{
		id:        actionID,
		name:      name,
		desc:      desc,
		chatbotID: chatbotID,
		apiConfig: apiConfig,
	}
}

// Info returns ToolInfo for LLM consumption
func (c *CustomActionTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	// Minimal params: accept a single `input` object containing parameters
	return &schema.ToolInfo{
		Name: fmt.Sprintf("action_%s", c.name),
		Desc: c.desc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"input": {
				Type:     schema.Object,
				Desc:     "Parameters for the action as a JSON object",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun executes the action HTTP request using provided JSON args
func (c *CustomActionTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		utils.Zlog.Error("CustomActionTool: failed to parse arguments", zap.Error(err), zap.String("action", c.name))
		return "", fmt.Errorf("invalid arguments: %w", err)
	}

	// Allow either top-level params or {"input": {...}}
	if v, ok := args["input"]; ok {
		if m, ok := v.(map[string]interface{}); ok {
			args = m
		}
	}

	// Read config values
	method := "GET"
	if m, ok := c.apiConfig["method"].(string); ok && m != "" {
		method = strings.ToUpper(m)
	}
	baseURL := ""
	if b, ok := c.apiConfig["base_url"].(string); ok {
		baseURL = b
	}
	endpoint := ""
	if e, ok := c.apiConfig["endpoint"].(string); ok {
		endpoint = e
	}
	headers := map[string]string{}
	if h, ok := c.apiConfig["headers"].(map[string]interface{}); ok {
		for k, v := range h {
			if s, ok := v.(string); ok {
				headers[k] = s
			}
		}
	}
	bodyTemplate := ""
	if b, ok := c.apiConfig["body_template"].(string); ok {
		bodyTemplate = b
	}

	timeoutSecs := 30
	if t, ok := c.apiConfig["timeout_seconds"].(float64); ok {
		timeoutSecs = int(t)
	}

	// Simple template substitution for {{key}} in URL, endpoint, headers, body
	substitute := func(s string) string {
		out := s
		for k, v := range args {
			placeholder := "{{" + k + "}}"
			var rep string
			switch vv := v.(type) {
			case string:
				rep = vv
			default:
				b, _ := json.Marshal(vv)
				rep = string(b)
			}
			out = strings.ReplaceAll(out, placeholder, rep)
		}
		return out
	}

	fullURL := strings.TrimRight(baseURL, "") + substitute(endpoint)

	var body io.Reader
	if bodyTemplate != "" {
		bstr := substitute(bodyTemplate)
		body = strings.NewReader(bstr)
	} else if method == "POST" || method == "PUT" || method == "PATCH" {
		// if no body template provided, marshal args
		b, _ := json.Marshal(args)
		body = strings.NewReader(string(b))
		if _, ok := headers["Content-Type"]; !ok {
			headers["Content-Type"] = "application/json"
		}
	}

	// Build HTTP client
	client := &http.Client{
		Timeout: time.Duration(timeoutSecs) * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		utils.Zlog.Error("CustomActionTool: failed to create request", zap.Error(err), zap.String("action", c.name))
		return "", err
	}

	for k, v := range headers {
		req.Header.Set(k, substitute(v))
	}

	// Auth handling (basic support for bearer/api_key)
	if authType, ok := c.apiConfig["auth_type"].(string); ok {
		if authType == "bearer" {
			if av, ok := c.apiConfig["auth_value"].(string); ok && av != "" {
				token := av
				// If stored as encrypted:..., keep as-is here; decryption not implemented in this MVP
				if strings.HasPrefix(token, "encrypted:") {
					token = strings.TrimPrefix(token, "encrypted:")
				}
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}
		if authType == "api_key" {
			if av, ok := c.apiConfig["auth_value"].(string); ok && av != "" {
				// default to X-API-Key header unless configured in headers
				if _, exists := headers["X-API-Key"]; !exists {
					req.Header.Set("X-API-Key", av)
				}
			}
		}
	}

	utils.Zlog.Info("Invoking custom action HTTP request",
		zap.String("action", c.name),
		zap.String("url", fullURL),
		zap.String("method", method))

	resp, err := client.Do(req)
	if err != nil {
		utils.Zlog.Error("CustomActionTool: http request failed", zap.Error(err), zap.String("action", c.name))
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// Return a simple structured JSON output similar to RAG tool
	out := map[string]interface{}{
		"status": resp.StatusCode,
		"body":   string(respBody),
	}

	outB, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("failed to marshal action output: %w", err)
	}

	utils.Zlog.Info("Custom action completed",
		zap.String("action", c.name),
		zap.Int("status", resp.StatusCode))

	return string(outB), nil
}

// Ensure CustomActionTool implements InvokableTool
var _ tool.InvokableTool = (*CustomActionTool)(nil)
