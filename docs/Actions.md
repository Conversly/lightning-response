# Custom Actions System - Complete Implementation Guide

## Table of Contents
1. [Overview & Architecture](#overview--architecture)
2. [Database Layer](#database-layer)
3. [API Layer (Backend)](#api-layer-backend)
4. [Frontend Input Layer](#frontend-input-layer)
5. [Tool Execution Layer](#tool-execution-layer)
6. [Graph Integration Layer](#graph-integration-layer)
7. [Testing & Validation](#testing--validation)
8. [Security & Rate Limiting](#security--rate-limiting)
9. [Monitoring & Debugging](#monitoring--debugging)

---

## Overview & Architecture

### System Flow
```
User (Frontend) 
    ↓ (Create/Edit Action)
API Layer
    ↓ (Validate & Store)
Database Layer
    ↓ (Load at Runtime)
Tool Factory
    ↓ (Convert to Executable Tool)
Graph Engine
    ↓ (LLM decides when to call)
Tool Execution
    ↓ (HTTP Request)
External API
```

### Key Design Principles
- **Dynamic Tool Creation**: Actions are loaded at runtime and converted to tools
- **LLM-Driven**: The AI decides when to use each tool based on descriptions
- **Type Safety**: Strong validation at every layer
- **Observability**: Comprehensive logging and monitoring
- **Security First**: Input validation, rate limiting, secret encryption

---

## Database Layer

### Schema Design

```sql
-- ============================================
-- 1. CUSTOM ACTIONS TABLE
-- ============================================
CREATE TABLE custom_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chatbot_id UUID NOT NULL REFERENCES chatbots(id) ON DELETE CASCADE,
    
    -- Metadata
    name VARCHAR(100) NOT NULL,              -- e.g., "get_product_price"
    display_name VARCHAR(200) NOT NULL,      -- e.g., "Get Product Price"
    description TEXT NOT NULL,               -- When and why to use this action
    is_enabled BOOLEAN DEFAULT true,         -- Can be disabled without deletion
    
    -- API Configuration (JSONB for flexibility)
    api_config JSONB NOT NULL,               -- Contains all API details
    
    -- Tool Schema (for LLM consumption)
    tool_schema JSONB NOT NULL,              -- JSON Schema format for parameters
    
    -- Metadata
    version INT DEFAULT 1,                   -- For versioning actions
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    created_by UUID REFERENCES users(id),
    last_tested_at TIMESTAMP,                -- When was this last tested
    test_status VARCHAR(50),                 -- 'passed', 'failed', 'not_tested'
    test_result JSONB,                       -- Results from last test
    
    -- Constraints
    CONSTRAINT unique_action_per_chatbot UNIQUE(chatbot_id, name),
    CONSTRAINT valid_name CHECK (name ~ '^[a-z0-9_]+$'), -- Only lowercase, numbers, underscores
    CONSTRAINT valid_method CHECK (
        api_config->>'method' IN ('GET', 'POST', 'PUT', 'DELETE', 'PATCH')
    )
);

-- Indexes for performance
CREATE INDEX idx_custom_actions_chatbot ON custom_actions(chatbot_id) 
    WHERE is_enabled = true;
CREATE INDEX idx_custom_actions_name ON custom_actions(chatbot_id, name);
CREATE INDEX idx_custom_actions_updated ON custom_actions(updated_at DESC);


-- ============================================
-- 3. ACTION TEMPLATES (Optional - for common actions)
-- ============================================
CREATE TABLE action_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL UNIQUE,
    category VARCHAR(50) NOT NULL,           -- e.g., 'ecommerce', 'crm', 'analytics'
    display_name VARCHAR(200) NOT NULL,
    description TEXT NOT NULL,
    icon_url TEXT,                           -- For UI display
    
    -- Pre-filled configuration
    template_config JSONB NOT NULL,          -- Default config to start from
    
    -- Requirements
    required_fields TEXT[] NOT NULL,         -- Fields user must fill
    
    -- Metadata
    is_public BOOLEAN DEFAULT true,
    usage_count INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Example templates insert
INSERT INTO action_templates (name, category, display_name, description, template_config, required_fields) VALUES
('shopify_get_product', 'ecommerce', 'Shopify: Get Product', 
 'Fetch product details from Shopify store', 
 '{"method": "GET", "base_url": "https://{store_name}.myshopify.com", "endpoint": "/admin/api/2024-01/products/{{product_id}}.json", "headers": {"X-Shopify-Access-Token": "{{api_token}}"}}',
 ARRAY['store_name', 'api_token']),
('stripe_create_payment', 'payments', 'Stripe: Create Payment Intent',
 'Create a payment intent in Stripe',
 '{"method": "POST", "base_url": "https://api.stripe.com", "endpoint": "/v1/payment_intents", "headers": {"Authorization": "Bearer {{api_key}}"}}',
 ARRAY['api_key']);

```

### JSONB Structure Examples

#### api_config JSONB Structure
```json
{
  "method": "POST",
  "base_url": "https://api.example.com",
  "endpoint": "/v1/products/{{product_id}}/price",
  "headers": {
    "Authorization": "Bearer {{auth_token}}",
    "Content-Type": "application/json",
    "X-Custom-Header": "value"
  },
  "query_params": {
    "include": "details",
    "format": "json"
  },
  "body_template": "{\"quantity\": {{quantity}}, \"currency\": \"{{currency}}\"}",
  "response_mapping": "$.data.price",
  "success_codes": [200, 201],
  "timeout_seconds": 30,
  "retry_count": 2,
  "auth_type": "bearer",
  "auth_value": "encrypted:abc123...",
  "follow_redirects": true,
  "verify_ssl": true
}
```

#### tool_schema JSONB Structure
```json
{
  "type": "object",
  "properties": {
    "product_id": {
      "type": "string",
      "description": "The unique identifier of the product",
      "pattern": "^[A-Z0-9-]+$"
    },
    "quantity": {
      "type": "integer",
      "description": "Number of items",
      "minimum": 1,
      "maximum": 1000,
      "default": 1
    },
    "currency": {
      "type": "string",
      "description": "Currency code",
      "enum": ["USD", "EUR", "GBP"],
      "default": "USD"
    }
  },
  "required": ["product_id"]
}
```

### Database Functions

```sql
-- Function to get enabled actions for a chatbot
CREATE OR REPLACE FUNCTION get_enabled_custom_actions(p_chatbot_id UUID)
RETURNS TABLE (
    id UUID,
    name VARCHAR,
    display_name VARCHAR,
    description TEXT,
    api_config JSONB,
    tool_schema JSONB
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        ca.id,
        ca.name,
        ca.display_name,
        ca.description,
        ca.api_config,
        ca.tool_schema
    FROM custom_actions ca
    WHERE ca.chatbot_id = p_chatbot_id
      AND ca.is_enabled = true
    ORDER BY ca.name;
END;
$$ LANGUAGE plpgsql;

-- Function to check rate limits
CREATE OR REPLACE FUNCTION check_rate_limit(
    p_chatbot_id UUID,
    p_action_id UUID
) RETURNS BOOLEAN AS $$
DECLARE
    v_limit RECORD;
    v_now TIMESTAMP := NOW();
BEGIN
    -- Get or create rate limit record
    INSERT INTO action_rate_limits (chatbot_id, action_id)
    VALUES (p_chatbot_id, p_action_id)
    ON CONFLICT (chatbot_id, action_id) DO NOTHING;
    
    SELECT * INTO v_limit
    FROM action_rate_limits
    WHERE chatbot_id = p_chatbot_id
      AND action_id = p_action_id
    FOR UPDATE;
    
    -- Check if blocked
    IF v_limit.is_blocked AND v_limit.blocked_until > v_now THEN
        RETURN FALSE;
    END IF;
    
    -- Reset counters if needed
    IF v_limit.minute_reset_at < v_now THEN
        UPDATE action_rate_limits
        SET calls_this_minute = 0,
            minute_reset_at = v_now + INTERVAL '1 minute'
        WHERE chatbot_id = p_chatbot_id AND action_id = p_action_id;
        v_limit.calls_this_minute := 0;
    END IF;
    
    IF v_limit.hour_reset_at < v_now THEN
        UPDATE action_rate_limits
        SET calls_this_hour = 0,
            hour_reset_at = v_now + INTERVAL '1 hour'
        WHERE chatbot_id = p_chatbot_id AND action_id = p_action_id;
        v_limit.calls_this_hour := 0;
    END IF;
    
    IF v_limit.day_reset_at < v_now THEN
        UPDATE action_rate_limits
        SET calls_this_day = 0,
            day_reset_at = v_now + INTERVAL '1 day'
        WHERE chatbot_id = p_chatbot_id AND action_id = p_action_id;
        v_limit.calls_this_day := 0;
    END IF;
    
    -- Check limits
    IF v_limit.calls_this_minute >= v_limit.max_calls_per_minute THEN
        RETURN FALSE;
    END IF;
    
    IF v_limit.calls_this_hour >= v_limit.max_calls_per_hour THEN
        RETURN FALSE;
    END IF;
    
    IF v_limit.calls_this_day >= v_limit.max_calls_per_day THEN
        RETURN FALSE;
    END IF;
    
    -- Increment counters
    UPDATE action_rate_limits
    SET calls_this_minute = calls_this_minute + 1,
        calls_this_hour = calls_this_hour + 1,
        calls_this_day = calls_this_day + 1
    WHERE chatbot_id = p_chatbot_id AND action_id = p_action_id;
    
    RETURN TRUE;
END;
$$ LANGUAGE plpgsql;

-- Function to log action execution
CREATE OR REPLACE FUNCTION log_custom_action_execution(
    p_action_id UUID,
    p_chatbot_id UUID,
    p_conversation_id VARCHAR,
    p_input_params JSONB,
    p_request_url TEXT,
    p_request_method VARCHAR,
    p_response_status INT,
    p_success BOOLEAN,
    p_error_message TEXT,
    p_execution_time_ms INT
) RETURNS UUID AS $$
DECLARE
    v_log_id UUID;
BEGIN
    INSERT INTO custom_action_logs (
        action_id, chatbot_id, conversation_id,
        input_params, request_url, request_method,
        response_status, success, error_message,
        execution_time_ms
    ) VALUES (
        p_action_id, p_chatbot_id, p_conversation_id,
        p_input_params, p_request_url, p_request_method,
        p_response_status, p_success, p_error_message,
        p_execution_time_ms
    ) RETURNING id INTO v_log_id;
    
    RETURN v_log_id;
END;
$$ LANGUAGE plpgsql;
```

---

## API Layer (Backend)

### Go Structs

```go
// models/custom_action.go
package models

import (
    "time"
)

// CustomActionConfig represents the API configuration
type CustomActionConfig struct {
    Method          string            `json:"method" binding:"required,oneof=GET POST PUT DELETE PATCH"`
    BaseURL         string            `json:"base_url" binding:"required,url"`
    Endpoint        string            `json:"endpoint" binding:"required"`
    Headers         map[string]string `json:"headers"`
    QueryParams     map[string]string `json:"query_params"`
    BodyTemplate    string            `json:"body_template"`
    ResponseMapping string            `json:"response_mapping"`
    SuccessCodes    []int             `json:"success_codes"`
    TimeoutSeconds  int               `json:"timeout_seconds" binding:"min=1,max=300"`
    RetryCount      int               `json:"retry_count" binding:"min=0,max=5"`
    AuthType        string            `json:"auth_type" binding:"oneof=none bearer api_key basic"`
    AuthValue       string            `json:"auth_value"`
    FollowRedirects bool              `json:"follow_redirects"`
    VerifySSL       bool              `json:"verify_ssl"`
}

// ToolParameter defines a parameter the LLM must provide
type ToolParameter struct {
    Name        string   `json:"name" binding:"required"`
    Type        string   `json:"type" binding:"required,oneof=string number integer boolean array object"`
    Description string   `json:"description" binding:"required,min=10"`
    Required    bool     `json:"required"`
    Default     string   `json:"default"`
    Enum        []string `json:"enum"`
    Pattern     string   `json:"pattern"`
    Minimum     *float64 `json:"minimum"`
    Maximum     *float64 `json:"maximum"`
    MinLength   *int     `json:"min_length"`
    MaxLength   *int     `json:"max_length"`
}

// CustomAction represents a full action configuration
type CustomAction struct {
    ID           string              `json:"id"`
    ChatbotID    string              `json:"chatbot_id"`
    Name         string              `json:"name" binding:"required,min=3,max=100"`
    DisplayName  string              `json:"display_name" binding:"required,min=3,max=200"`
    Description  string              `json:"description" binding:"required,min=20,max=1000"`
    IsEnabled    bool                `json:"is_enabled"`
    APIConfig    *CustomActionConfig `json:"api_config" binding:"required"`
    Parameters   []ToolParameter     `json:"parameters" binding:"required,min=1"`
    Version      int                 `json:"version"`
    CreatedAt    time.Time           `json:"created_at"`
    UpdatedAt    time.Time           `json:"updated_at"`
    LastTestedAt *time.Time          `json:"last_tested_at"`
    TestStatus   string              `json:"test_status"`
    TestResult   map[string]interface{} `json:"test_result"`
}

// CreateCustomActionRequest is the API request body
type CreateCustomActionRequest struct {
    ChatbotID   string              `json:"chatbot_id" binding:"required,uuid"`
    Name        string              `json:"name" binding:"required,min=3,max=100"`
    DisplayName string              `json:"display_name" binding:"required,min=3,max=200"`
    Description string              `json:"description" binding:"required,min=20,max=1000"`
    APIConfig   *CustomActionConfig `json:"api_config" binding:"required"`
    Parameters  []ToolParameter     `json:"parameters" binding:"required,min=1"`
}

// TestActionRequest is for testing an action before saving
type TestActionRequest struct {
    Config         *CustomActionConfig `json:"config" binding:"required"`
    TestParameters map[string]interface{} `json:"test_parameters"`
}

// TestActionResponse returns test results
type TestActionResponse struct {
    Success        bool                   `json:"success"`
    StatusCode     int                    `json:"status_code"`
    ResponseBody   string                 `json:"response_body"`
    ResponseTime   int                    `json:"response_time_ms"`
    Error          string                 `json:"error,omitempty"`
    RequestURL     string                 `json:"request_url"`
    ExtractedData  interface{}            `json:"extracted_data,omitempty"`
}


```

### API Endpoints

```go
// handlers/custom_actions.go
package handlers

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "go.uber.org/zap"

    "github.com/Conversly/lightning-response/internal/models"
    "github.com/Conversly/lightning-response/internal/utils"
)

type CustomActionsHandler struct {
    db     *loaders.PostgresClient
    logger *zap.Logger
}

// ============================================
// 1. CREATE ACTION
// ============================================
// POST /api/v1/chatbots/:chatbot_id/actions
func (h *CustomActionsHandler) CreateAction(c *gin.Context) {
    chatbotID := c.Param("chatbot_id")
    
    // Validate user owns this chatbot
    userID := c.GetString("user_id") // From auth middleware
    if !h.validateChatbotOwnership(c.Request.Context(), userID, chatbotID) {
        c.JSON(http.StatusForbidden, gin.H{
            "error": "You don't have permission to modify this chatbot",
        })
        return
    }

    var req models.CreateCustomActionRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Invalid request format",
            "details": err.Error(),
        })
        return
    }

    // Additional validation
    if err := h.validateActionConfig(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Invalid configuration",
            "details": err.Error(),
        })
        return
    }

    // Test the action before saving
    testResult, err := h.testAction(c.Request.Context(), req.APIConfig, req.Parameters)
    if err != nil {
        h.logger.Warn("Action test failed",
            zap.String("chatbot_id", chatbotID),
            zap.String("action_name", req.Name),
            zap.Error(err))
        
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Action test failed",
            "details": err.Error(),
            "test_result": testResult,
            "suggestion": "Please check your API configuration and try again",
        })
        return
    }

    // Save to database
    action, err := h.db.CreateCustomAction(c.Request.Context(), &req)
    if err != nil {
        h.logger.Error("Failed to save custom action",
            zap.String("chatbot_id", chatbotID),
            zap.Error(err))
        
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to save action",
        })
        return
    }

    h.logger.Info("Custom action created",
        zap.String("chatbot_id", chatbotID),
        zap.String("action_id", action.ID),
        zap.String("action_name", action.Name))

    c.JSON(http.StatusCreated, gin.H{
        "success": true,
        "action": action,
        "test_result": testResult,
    })
}

// ============================================
// 2. GET ALL ACTIONS FOR CHATBOT
// ============================================
// GET /api/v1/chatbots/:chatbot_id/actions
func (h *CustomActionsHandler) ListActions(c *gin.Context) {
    chatbotID := c.Param("chatbot_id")
    
    userID := c.GetString("user_id")
    if !h.validateChatbotOwnership(c.Request.Context(), userID, chatbotID) {
        c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized"})
        return
    }

    actions, err := h.db.GetCustomActions(c.Request.Context(), chatbotID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to fetch actions",
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "actions": actions,
        "count": len(actions),
    })
}

// ============================================
// 3. GET SINGLE ACTION
// ============================================
// GET /api/v1/chatbots/:chatbot_id/actions/:action_id
func (h *CustomActionsHandler) GetAction(c *gin.Context) {
    chatbotID := c.Param("chatbot_id")
    actionID := c.Param("action_id")
    
    userID := c.GetString("user_id")
    if !h.validateChatbotOwnership(c.Request.Context(), userID, chatbotID) {
        c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized"})
        return
    }

    action, err := h.db.GetCustomAction(c.Request.Context(), actionID, chatbotID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{
            "error": "Action not found",
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "action": action,
    })
}

// ============================================
// 4. UPDATE ACTION
// ============================================
// PUT /api/v1/chatbots/:chatbot_id/actions/:action_id
func (h *CustomActionsHandler) UpdateAction(c *gin.Context) {
    chatbotID := c.Param("chatbot_id")
    actionID := c.Param("action_id")
    
    userID := c.GetString("user_id")
    if !h.validateChatbotOwnership(c.Request.Context(), userID, chatbotID) {
        c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized"})
        return
    }

    var req models.CreateCustomActionRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Invalid request",
            "details": err.Error(),
        })
        return
    }

    // Test before updating
    testResult, err := h.testAction(c.Request.Context(), req.APIConfig, req.Parameters)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Action test failed",
            "test_result": testResult,
        })
        return
    }

    action, err := h.db.UpdateCustomAction(c.Request.Context(), actionID, chatbotID, &req)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to update action",
        })
        return
    }

    h.logger.Info("Custom action updated",
        zap.String("action_id", actionID),
        zap.String("chatbot_id", chatbotID))

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "action": action,
        "test_result": testResult,
    })
}

// ============================================
// 5. DELETE ACTION
// ============================================
// DELETE /api/v1/chatbots/:chatbot_id/actions/:action_id
func (h *CustomActionsHandler) DeleteAction(c *gin.Context) {
    chatbotID := c.Param("chatbot_id")
    actionID := c.Param("action_id")
    
    userID := c.GetString("user_id")
    if !h.validateChatbotOwnership(c.Request.Context(), userID, chatbotID) {
        c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized"})
        return
    }

    if err := h.db.DeleteCustomAction(c.Request.Context(), actionID, chatbotID); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to delete action",
        })
        return
    }

    h.logger.Info("Custom action deleted",
        zap.String("action_id", actionID),
        zap.String("chatbot_id", chatbotID))

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "Action deleted successfully",
    })
}

// ============================================
// 6. TOGGLE ACTION (Enable/Disable)
// ============================================
// PATCH /api/v1/chatbots/:chatbot_id/actions/:action_id/toggle
func (h *CustomActionsHandler) ToggleAction(c *gin.Context) {
    chatbotID := c.Param("chatbot_id")
    actionID := c.Param("action_id")
    
    userID := c.GetString("user_id")
    if !h.validateChatbotOwnership(c.Request.Context(), userID, chatbotID) {
        c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized"})
        return
    }

    var req struct {
        IsEnabled bool `json:"is_enabled"`
    }
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
        return
    }

    if err := h.db.ToggleCustomAction(c.Request.Context(), actionID, chatbotID, req.IsEnabled); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to toggle action",
        })
        return
    }

    h.logger.Info("Custom action toggled",
        zap.String("action_id", actionID),
        zap.Bool("enabled", req.IsEnabled))

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "is_enabled": req.IsEnabled,
    })
}

// ============================================
// 7. TEST ACTION
// ============================================
// POST /api/v1/chatbots/:chatbot_id/actions/test
func (h *CustomActionsHandler) TestAction(c *gin.Context) {
    chatbotID := c.Param("chatbot_id")
    
    userID := c.GetString("user_id")
    if !h.validateChatbotOwnership(c.Request.Context(), userID, chatbotID) {
        c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized"})
        return
    }

    var req models.TestActionRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": "Invalid request",
            "details": err.Error(),
        })
        return
    }

    // Create temporary parameters from test data
    var params []models.ToolParameter
    for key, value := range req.TestParameters {
        params = append(params, models.ToolParameter{
            Name:    key,
            Type:    inferType(value),
            Default: fmt.Sprint(value),
        })
    }

    result, err := h.testAction(c.Request.Context(), req.Config, params)
    
    status := http.StatusOK
    if err != nil {
        status = http.StatusBadRequest
    }

    c.JSON(status, result)
}

// ============================================
// 8. GET ACTION EXECUTION LOGS
// ============================================
// GET /api/v1/chatbots/:chatbot_id/actions/:action_id/logs
func (h *CustomActionsHandler) GetActionLogs(c *gin.Context) {
    chatbotID := c.Param("chatbot_id")
    actionID := c.Param("action_id")
    
    userID := c.GetString("user_id")
    if !h.validateChatbotOwnership(c.Request.Context(), userID, chatbotID) {
        c.JSON(http.StatusForbidden, gin.H{"error": "unauthorized"})
        return
    }

    // Query parameters
    limit := c.DefaultQuery("limit", "50")
    onlyFailed := c.Query("only_failed") == "true"

    logs, err := h.db.GetActionLogs(c.Request.Context(), actionID, limit, onlyFailed)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to fetch logs",
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "logs": logs,
        "count": len(logs),
    })
}

// ============================================
// 9. GET ACTION TEMPLATES
// ============================================
// GET /api/v1/action-templates
func (h *CustomActionsHandler) GetTemplates(c *gin.Context) {
    category := c.Query("category")
    
    templates, err := h.db.GetActionTemplates(c.Request.Context(), category)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Failed to fetch templates",
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "templates": templates,
    })
}

// ============================================
// HELPER FUNCTIONS
// ============================================

func (h *CustomActionsHandler) validateChatbotOwnership(ctx context.Context, userID, chatbotID string) bool {
    // Query database to check if user owns chatbot
    var ownerID string
    err := h.db.QueryRow(ctx, 
        "SELECT owner_id FROM chatbots WHERE id = $1", 
        chatbotID).Scan(&ownerID)
    
    return err == nil && ownerID == userID
}

func (h *CustomActionsHandler) validateActionConfig(req *models.CreateCustomActionRequest) error {
    // Validate name format
    if !isValidActionName(req.Name) {
        return fmt.Errorf("action name must contain only lowercase letters, numbers, and underscores")
    }

    // Validate URL
    if !isValidURL(req.APIConfig.BaseURL) {
        return fmt.Errorf("invalid base URL")
    }

    // Validate parameters
    for _, param := range req.Parameters {
        if !isValidParameterType(param.Type) {
            return fmt.Errorf("invalid parameter type: %s", param.Type)
        }
    }

    // Ensure at least one success code
    if len(req.APIConfig.SuccessCodes) == 0 {
        req.APIConfig.SuccessCodes = []int{200, 201}
    }

    return nil
}

func (h *CustomActionsHandler) testAction(
    ctx context.Context, 
    config *models.CustomActionConfig,
    params []models.ToolParameter,
) (*models.TestActionResponse, error) {
    // Create a temporary tool
    tempAction := &models.CustomAction{
        ID:          "test",
        Name:        "test_action",
        Description: "Test action",
        APIConfig:   config,
        Parameters:  params,
    }

    tool, err := NewCustomActionTool(ctx, tempAction)
    if err != nil {
        return &models.TestActionResponse{
            Success: false,
            Error:   err.Error(),
        }, err
    }

    // Build test parameters
    testParams := make(map[string]interface{})
    for _, p := range params {
        if p.Default != "" {
            testParams[p.Name] = p.Default
        }
    }

    testParamsJSON, _ := json.Marshal(testParams)
    
    startTime := time.Now()
    result, err := tool.InvokableRun(ctx, string(testParamsJSON))
    duration := time.Since(startTime).Milliseconds()

    response := &models.TestActionResponse{
        Success:      err == nil,
        ResponseBody: result,
        ResponseTime: int(duration),
        RequestURL:   config.BaseURL + config.Endpoint,
    }

    if err != nil {
        response.Error = err.Error()
        return response, err
    }

    return response, nil
}

func inferType(value interface{}) string {
    switch value.(type) {
    case string:
        return "string"
    case int, int64, float64:
        return "number"
    case bool:
        return "boolean"
    case []interface{}:
        return "array"
    case map[string]interface{}:
        return "object"
    default:
        return "string"
    }
}
```

### Database Access Layer

```go
// db/custom_actions.go
package loaders

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/Conversly/lightning-response/internal/models"
)

// CreateCustomAction saves a new action
func (c *PostgresClient) CreateCustomAction(
    ctx context.Context, 
    req *models.CreateCustomActionRequest,
) (*models.CustomAction, error) {
    apiConfigJSON, err := json.Marshal(req.APIConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal api_config: %w", err)
    }

    toolSchema := generateToolSchema(req.Parameters)
    toolSchemaJSON, err := json.Marshal(toolSchema)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal tool_schema: %w", err)
    }

    query := `
        INSERT INTO custom_actions (
            chatbot_id, name, display_name, description, 
            api_config, tool_schema
        ) VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id, created_at, updated_at
    `

    var action models.CustomAction
    err = c.pool.QueryRow(ctx, query,
        req.ChatbotID,
        req.Name,
        req.DisplayName,
        req.Description,
        apiConfigJSON,
        toolSchemaJSON,
    ).Scan(&action.ID, &action.CreatedAt, &action.UpdatedAt)

    if err != nil {
        return nil, fmt.Errorf("failed to create action: %w", err)
    }

    action.ChatbotID = req.ChatbotID
    action.Name = req.Name
    action.DisplayName = req.DisplayName
    action.Description = req.Description
    action.APIConfig = req.APIConfig
    action.Parameters = req.Parameters
    action.IsEnabled = true

    return &action, nil
}

// GetCustomActions retrieves all actions for a chatbot
func (c *PostgresClient) GetCustomActions(
    ctx context.Context, 
    chatbotID string,
) ([]*models.CustomAction, error) {
    query := `
        SELECT 
            id, name, display_name, description, is_enabled,
            api_config, tool_schema, version,
            created_at, updated_at, last_tested_at, test_status
        FROM custom_actions
        WHERE chatbot_id = $1
        ORDER BY created_at DESC
    `

    rows, err := c.pool.Query(ctx, query, chatbotID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    actions := make([]*models.CustomAction, 0)
    for rows.Next() {
        var action models.CustomAction
        var apiConfigJSON, toolSchemaJSON []byte

        err := rows.Scan(
            &action.ID,
            &action.Name,
            &action.DisplayName,
            &action.Description,
            &action.IsEnabled,
            &apiConfigJSON,
            &toolSchemaJSON,
            &action.Version,
            &action.CreatedAt,
            &action.UpdatedAt,
            &action.LastTestedAt,
            &action.TestStatus,
        )
        if err != nil {
            return nil, err
        }

        if err := json.Unmarshal(apiConfigJSON, &action.APIConfig); err != nil {
            return nil, err
        }

        // Parse tool schema back to parameters
        var toolSchema map[string]interface{}
        if err := json.Unmarshal(toolSchemaJSON, &toolSchema); err != nil {
            return nil, err
        }
        action.Parameters = parseToolSchemaToParameters(toolSchema)

        action.ChatbotID = chatbotID
        actions = append(actions, &action)
    }

    return actions, nil
}

// GetEnabledCustomActions gets only enabled actions (for runtime)
func (c *PostgresClient) GetEnabledCustomActions(
    ctx context.Context, 
    chatbotID string,
) ([]*models.CustomAction, error) {
    query := `
        SELECT id, name, display_name, description, api_config, tool_schema
        FROM custom_actions
        WHERE chatbot_id = $1 AND is_enabled = true
        ORDER BY name
    `

    rows, err := c.pool.Query(ctx, query, chatbotID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    actions := make([]*models.CustomAction, 0)
    for rows.Next() {
        var action models.CustomAction
        var apiConfigJSON, toolSchemaJSON []byte

        err := rows.Scan(
            &action.ID,
            &action.Name,
            &action.DisplayName,
            &action.Description,
            &apiConfigJSON,
            &toolSchemaJSON,
        )
        if err != nil {
            return nil, err
        }

        if err := json.Unmarshal(apiConfigJSON, &action.APIConfig); err != nil {
            return nil, err
        }

        var toolSchema map[string]interface{}
        if err := json.Unmarshal(toolSchemaJSON, &toolSchema); err != nil {
            return nil, err
        }
        action.Parameters = parseToolSchemaToParameters(toolSchema)

        action.ChatbotID = chatbotID
        actions = append(actions, &action)
    }

    return actions, nil
}

// UpdateCustomAction updates an existing action
func (c *PostgresClient) UpdateCustomAction(
    ctx context.Context,
    actionID string,
    chatbotID string,
    req *models.CreateCustomActionRequest,
) (*models.CustomAction, error) {
    apiConfigJSON, err := json.Marshal(req.APIConfig)
    if err != nil {
        return nil, err
    }

    toolSchema := generateToolSchema(req.Parameters)
    toolSchemaJSON, err := json.Marshal(toolSchema)
    if err != nil {
        return nil, err
    }

    query := `
        UPDATE custom_actions
        SET 
            name = $3,
            display_name = $4,
            description = $5,
            api_config = $6,
            tool_schema = $7,
            updated_at = NOW(),
            version = version + 1
        WHERE id = $1 AND chatbot_id = $2
        RETURNING version, updated_at
    `

    var action models.CustomAction
    err = c.pool.QueryRow(ctx, query,
        actionID, chatbotID,
        req.Name, req.DisplayName, req.Description,
        apiConfigJSON, toolSchemaJSON,
    ).Scan(&action.Version, &action.UpdatedAt)

    if err != nil {
        return nil, err
    }

    action.ID = actionID
    action.ChatbotID = chatbotID
    action.Name = req.Name
    action.DisplayName = req.DisplayName
    action.Description = req.Description
    action.APIConfig = req.APIConfig
    action.Parameters = req.Parameters

    return &action, nil
}

// DeleteCustomAction soft deletes an action
func (c *PostgresClient) DeleteCustomAction(
    ctx context.Context,
    actionID string,
    chatbotID string,
) error {
    query := `DELETE FROM custom_actions WHERE id = $1 AND chatbot_id = $2`
    _, err := c.pool.Exec(ctx, query, actionID, chatbotID)
    return err
}

// ToggleCustomAction enables/disables an action
func (c *PostgresClient) ToggleCustomAction(
    ctx context.Context,
    actionID string,
    chatbotID string,
    isEnabled bool,
) error {
    query := `
        UPDATE custom_actions
        SET is_enabled = $3, updated_at = NOW()
        WHERE id = $1 AND chatbot_id = $2
    `
    _, err := c.pool.Exec(ctx, query, actionID, chatbotID, isEnabled)
    return err
}

// Helper: Generate JSON Schema from parameters
func generateToolSchema(params []models.ToolParameter) map[string]interface{} {
    properties := make(map[string]interface{})
    required := make([]string, 0)

    for _, param := range params {
        propSchema := map[string]interface{}{
            "type":        param.Type,
            "description": param.Description,
        }

        if len(param.Enum) > 0 {
            propSchema["enum"] = param.Enum
        }
        if param.Default != "" {
            propSchema["default"] = param.Default
        }
        if param.Pattern != "" {
            propSchema["pattern"] = param.Pattern
        }
        if param.Minimum != nil {
            propSchema["minimum"] = *param.Minimum
        }
        if param.Maximum != nil {
            propSchema["maximum"] = *param.Maximum
        }
        if param.MinLength != nil {
            propSchema["minLength"] = *param.MinLength
        }
        if param.MaxLength != nil {
            propSchema["maxLength"] = *param.MaxLength
        }

        properties[param.Name] = propSchema

        if param.Required {
            required = append(required, param.Name)
        }
    }

    return map[string]interface{}{
        "type":       "object",
        "properties": properties,
        "required":   required,
    }
}

// Helper: Parse tool schema back to parameters
func parseToolSchemaToParameters(schema map[string]interface{}) []models.ToolParameter {
    params := make([]models.ToolParameter, 0)
    
    properties, ok := schema["properties"].(map[string]interface{})
    if !ok {
        return params
    }

    required, _ := schema["required"].([]interface{})
    requiredMap := make(map[string]bool)
    for _, r := range required {
        if rStr, ok := r.(string); ok {
            requiredMap[rStr] = true
        }
    }

    for name, prop := range properties {
        propMap, ok := prop.(map[string]interface{})
        if !ok {
            continue
        }

        param := models.ToolParameter{
            Name:        name,
            Type:        propMap["type"].(string),
            Description: propMap["description"].(string),
            Required:    requiredMap[name],
        }

        if def, ok := propMap["default"].(string); ok {
            param.Default = def
        }
        if enum, ok := propMap["enum"].([]interface{}); ok {
            for _, e := range enum {
                param.Enum = append(param.Enum, e.(string))
            }
        }

        params = append(params, param)
    }

    return params
}
```

---

## Frontend Input Layer

### React Component Structure

```typescript
// types/customActions.ts
export interface ToolParameter {
  name: string;
  type: 'string' | 'number' | 'integer' | 'boolean' | 'array' | 'object';
  description: string;
  required: boolean;
  default?: string;
  enum?: string[];
  pattern?: string;
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;
}

export interface APIConfig {
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH';
  base_url: string;
  endpoint: string;
  headers?: Record<string, string>;
  query_params?: Record<string, string>;
  body_template?: string;
  response_mapping?: string;
  success_codes: number[];
  timeout_seconds: number;
  retry_count: number;
  auth_type: 'none' | 'bearer' | 'api_key' | 'basic';
  auth_value?: string;
  follow_redirects: boolean;
  verify_ssl: boolean;
}

export interface CustomAction {
  id?: string;
  chatbot_id: string;
  name: string;
  display_name: string;
  description: string;
  is_enabled: boolean;
  api_config: APIConfig;
  parameters: ToolParameter[];
  version?: number;
  created_at?: string;
  updated_at?: string;
  test_status?: 'passed' | 'failed' | 'not_tested';
}

export interface TestResult {
  success: boolean;
  status_code?: number;
  response_body?: string;
  response_time_ms?: number;
  error?: string;
  request_url?: string;
  extracted_data?: any;
}
```

### Main Form Component

```typescript
// components/CustomActionForm.tsx
import React, { useState } from 'react';
import { CustomAction, ToolParameter, TestResult } from '../types/customActions';

interface Props {
  chatbotId: string;
  existingAction?: CustomAction;
  onSave: (action: CustomAction) => Promise<void>;
  onCancel: () => void;
}

export const CustomActionForm: React.FC<Props> = ({
  chatbotId,
  existingAction,
  onSave,
  onCancel,
}) => {
  const [formData, setFormData] = useState<CustomAction>(
    existingAction || {
      chatbot_id: chatbotId,
      name: '',
      display_name: '',
      description: '',
      is_enabled: true,
      api_config: {
        method: 'GET',
        base_url: '',
        endpoint: '',
        headers: {},
        query_params: {},
        success_codes: [200, 201],
        timeout_seconds: 30,
        retry_count: 0,
        auth_type: 'none',
        follow_redirects: true,
        verify_ssl: true,
      },
      parameters: [],
    }
  );

  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [testing, setTesting] = useState(false);
  const [saving, setSaving] = useState(false);
  const [currentStep, setCurrentStep] = useState(1); // Multi-step form

  // Update field helper
  const updateField = (path: string, value: any) => {
    setFormData((prev) => {
      const keys = path.split('.');
      const updated = { ...prev };
      let current: any = updated;

      for (let i = 0; i < keys.length - 1; i++) {
        current[keys[i]] = { ...current[keys[i]] };
        current = current[keys[i]];
      }

      current[keys[keys.length - 1]] = value;
      return updated;
    });
  };

  // Test action before saving
  const handleTest = async () => {
    setTesting(true);
    setTestResult(null);

    try {
      const response = await fetch(`/api/v1/chatbots/${chatbotId}/actions/test`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          config: formData.api_config,
          test_parameters: generateTestParameters(formData.parameters),
        }),
      });

      const result = await response.json();
      setTestResult(result);

      if (result.success) {
        // Show success notification
        alert('Test successful! You can now save this action.');
      }
    } catch (error) {
      setTestResult({
        success: false,
        error: error.message,
      });
    } finally {
      setTesting(false);
    }
  };

  // Save action
  const handleSave = async () => {
    setSaving(true);
    try {
      await onSave(formData);
    } finally {
      setSaving(false);
    }
  };

  // Generate test parameters from defaults
  const generateTestParameters = (params: ToolParameter[]) => {
    const testParams: Record<string, any> = {};
    params.forEach((param) => {
      if (param.default) {
        testParams[param.name] = param.default;
      } else if (param.type === 'string') {
        testParams[param.name] = 'test_value';
      } else if (param.type === 'number' || param.type === 'integer') {
        testParams[param.name] = 123;
      } else if (param.type === 'boolean') {
        testParams[param.name] = true;
      }
    });
    return testParams;
  };

  return (
    <div className="custom-action-form">
      {/* Progress Steps */}
      <div className="steps">
        <Step number={1} title="Basic Info" active={currentStep === 1} />
        <Step number={2} title="API Config" active={currentStep === 2} />
        <Step number={3} title="Parameters" active={currentStep === 3} />
        <Step number={4} title="Test & Save" active={currentStep === 4} />
      </div>

      {/* Step 1: Basic Information */}
      {currentStep === 1 && (
        <BasicInfoStep
          formData={formData}
          updateField={updateField}
          onNext={() => setCurrentStep(2)}
          onCancel={onCancel}
        />
      )}

      {/* Step 2: API Configuration */}
      {currentStep === 2 && (
        <APIConfigStep
          formData={formData}
          updateField={updateField}
          onNext={() => setCurrentStep(3)}
          onBack={() => setCurrentStep(1)}
        />
      )}

      {/* Step 3: Parameters */}
      {currentStep === 3 && (
        <ParametersStep
          formData={formData}
          updateField={updateField}
          onNext={() => setCurrentStep(4)}
          onBack={() => setCurrentStep(2)}
        />
      )}

      {/* Step 4: Test & Save */}
      {currentStep === 4 && (
        <TestAndSaveStep
          formData={formData}
          testResult={testResult}
          testing={testing}
          saving={saving}
          onTest={handleTest}
          onSave={handleSave}
          onBack={() => setCurrentStep(3)}
        />
      )}
    </div>
  );
};
```

### Step 1: Basic Information

```typescript
// components/steps/BasicInfoStep.tsx
import React from 'react';
import { CustomAction } from '../../types/customActions';

interface Props {
  formData: CustomAction;
  updateField: (path: string, value: any) => void;
  onNext: () => void;
  onCancel: () => void;
}

export const BasicInfoStep: React.FC<Props> = ({
  formData,
  updateField,
  onNext,
  onCancel,
}) => {
  const isValid = () => {
    return (
      formData.name.length >= 3 &&
      formData.display_name.length >= 3 &&
      formData.description.length >= 20
    );
  };

  return (
    <div className="step-content">
      <h2>Basic Information</h2>
      <p className="subtitle">
        Provide a name and description for your custom action.
      </p>

      {/* Action Name */}
      <div className="form-group">
        <label htmlFor="name">
          Action Name <span className="required">*</span>
        </label>
        <input
          id="name"
          type="text"
          value={formData.name}
          onChange={(e) => updateField('name', e.target.value.toLowerCase())}
          placeholder="e.g., get_product_price"
          pattern="^[a-z0-9_]+$"
          required
        />
        <small className="help-text">
          Lowercase letters, numbers, and underscores only. This is the internal identifier.
        </small>
      </div>

      {/* Display Name */}
      <div className="form-group">
        <label htmlFor="display_name">
          Display Name <span className="required">*</span>
        </label>
        <input
          id="display_name"
          type="text"
          value={formData.display_name}
          onChange={(e) => updateField('display_name', e.target.value)}
          placeholder="e.g., Get Product Price"
          required
        />
        <small className="help-text">
          A human-readable name shown in the dashboard.
        </small>
      </div>

      {/* Description */}
      <div className="form-group">
        <label htmlFor="description">
          Description <span className="required">*</span>
        </label>
        <textarea
          id="description"
          value={formData.description}
          onChange={(e) => updateField('description', e.target.value)}
          placeholder="Describe when and how this action should be used. The AI will use this to decide when to call this action."
          rows={4}
          required
          minLength={20}
        />
        <small className="help-text">
          {formData.description.length} / 1000 characters. Be specific! 
          The AI uses this to understand when to use this action.
        </small>
      </div>

      {/* Example Good Description */}
      <div className="example-box">
        <strong>💡 Example of a good description:</strong>
        <p>
          "Use this action when the user asks about product prices, availability, 
          or inventory. It fetches real-time pricing data from our e-commerce API. 
          Requires a product ID or product name."
        </p>
      </div>

      {/* Buttons */}
      <div className="form-actions">
        <button type="button" onClick={onCancel} className="btn-secondary">
          Cancel
        </button>
        <button
          type="button"
          onClick={onNext}
          disabled={!isValid()}
          className="btn-primary"
        >
          Next: API Configuration →
        </button>
      </div>
    </div>
  );
};
```

### Step 2: API Configuration

```typescript
// components/steps/APIConfigStep.tsx
import React, { useState } from 'react';
import { CustomAction } from '../../types/customActions';

interface Props {
  formData: CustomAction;
  updateField: (path: string, value: any) => void;
  onNext: () => void;
  onBack: () => void;
}

export const APIConfigStep: React.FC<Props> = ({
  formData,
  updateField,
  onNext,
  onBack,
}) => {
  const [showAdvanced, setShowAdvanced] = useState(false);

  const config = formData.api_config;

  const isValid = () => {
    try {
      new URL(config.base_url);
      return config.base_url && config.endpoint && config.method;
    } catch {
      return false;
    }
  };

  return (
    <div className="step-content">
      <h2>API Configuration</h2>
      <p className="subtitle">Configure how to call your external API.</p>

      {/* HTTP Method */}
      <div className="form-group">
        <label>HTTP Method <span className="required">*</span></label>
        <div className="method-selector">
          {['GET', 'POST', 'PUT', 'DELETE', 'PATCH'].map((method) => (
            <button
              key={method}
              type="button"
              className={`method-btn ${config.method === method ? 'active' : ''}`}
              onClick={() => updateField('api_config.method', method)}
            >
              {method}
            </button>
          ))}
        </div>
      </div>

      {/* Base URL */}
      <div className="form-group">
        <label htmlFor="base_url">
          Base URL <span className="required">*</span>
        </label>
        <input
          id="base_url"
          type="url"
          value={config.base_url}
          onChange={(e) => updateField('api_config.base_url', e.target.value)}
          placeholder="https://api.example.com"
          required
        />
        <small className="help-text">
          The base URL of your API (without the endpoint path).
        </small>
      </div>

      {/* Endpoint */}
      <div className="form-group">
        <label htmlFor="endpoint">
          Endpoint Path <span className="required">*</span>
        </label>
        <input
          id="endpoint"
          type="text"
          value={config.endpoint}
          onChange={(e) => updateField('api_config.endpoint', e.target.value)}
          placeholder="/v1/products/{{product_id}}"
          required
        />
        <small className="help-text">
          Use <code>{`{{variable_name}}`}</code> for dynamic values that will come from parameters.
        </small>
      </div>

      {/* Full URL Preview */}
      <div className="url-preview">
        <strong>Full URL:</strong>
        <code>{config.base_url}{config.endpoint}</code>
      </div>

      {/* Authentication */}
      <div className="form-group">
        <label htmlFor="auth_type">Authentication</label>
        <select
          id="auth_type"
          value={config.auth_type}
          onChange={(e) => updateField('api_config.auth_type', e.target.value)}
        >
          <option value="none">None</option>
          <option value="bearer">Bearer Token</option>
          <option value="api_key">API Key</option>
          <option value="basic">Basic Auth</option>
        </select>
      </div>

      {/* Auth Value (if not none) */}
      {config.auth_type !== 'none' && (
        <div className="form-group">
          <label htmlFor="auth_value">
            {config.auth_type === 'bearer' && 'Bearer Token'}
            {config.auth_type === 'api_key' && 'API Key'}
            {config.auth_type === 'basic' && 'Base64 Encoded Credentials'}
          </label>
          <input
            id="auth_value"
            type="password"
            value={config.auth_value || ''}
            onChange={(e) => updateField('api_config.auth_value', e.target.value)}
            placeholder="Enter your token/key"
          />
          <small className="help-text">
            🔒 This value will be encrypted and stored securely.
          </small>
        </div>
      )}

      {/* Headers */}
      <div className="form-group">
        <label>Custom Headers (Optional)</label>
        <KeyValueEditor
          value={config.headers || {}}
          onChange={(headers) => updateField('api_config.headers', headers)}
          placeholder={{
            key: 'Header-Name',
            value: 'Header-Value',
          }}
        />
        <small className="help-text">
          Add custom HTTP headers. Use <code>{`{{variable}}`}</code> for dynamic values.
        </small>
      </div>

      {/* Request Body (for POST/PUT/PATCH) */}
      {['POST', 'PUT', 'PATCH'].includes(config.method) && (
        <div className="form-group">
          <label htmlFor="body_template">Request Body Template</label>
          <textarea
            id="body_template"
            value={config.body_template || ''}
            onChange={(e) => updateField('api_config.body_template', e.target.value)}
            placeholder='{"product_id": "{{product_id}}", "quantity": {{quantity}}}'
            rows={4}
            style={{ fontFamily: 'monospace' }}
          />
          <small className="help-text">
            JSON body template. Use <code>{`{{variable}}`}</code> for dynamic values from parameters.
          </small>
        </div>
      )}

      {/* Advanced Options Toggle */}
      <button
        type="button"
        className="toggle-advanced"
        onClick={() => setShowAdvanced(!showAdvanced)}
      >
        {showAdvanced ? '▼' : '▶'} Advanced Options
      </button>

      {/* Advanced Options */}
      {showAdvanced && (
        <div className="advanced-options">
          {/* Query Parameters */}
          <div className="form-group">
            <label>Query Parameters (Optional)</label>
            <KeyValueEditor
              value={config.query_params || {}}
              onChange={(params) => updateField('api_config.query_params', params)}
              placeholder={{
                key: 'param_name',
                value: 'param_value',
              }}
            />
          </div>

          {/* Response Mapping */}
          <div className="form-group">
            <label htmlFor="response_mapping">Response Mapping (JSONPath)</label>
            <input
              id="response_mapping"
              type="text"
              value={config.response_mapping || ''}
              onChange={(e) => updateField('api_config.response_mapping', e.target.value)}
              placeholder="$.data.products[0].price"
            />
            <small className="help-text">
              Extract specific data from response. Leave empty to return full response.
            </small>
          </div>

          {/* Success Codes */}
          <div className="form-group">
            <label htmlFor="success_codes">Success HTTP Codes</label>
            <input
              id="success_codes"
              type="text"
              value={config.success_codes.join(', ')}
              onChange={(e) =>
                updateField(
                  'api_config.success_codes',
                  e.target.value.split(',').map((s) => parseInt(s.trim()))
                )
              }
              placeholder="200, 201"
            />
          </div>

          {/* Timeout */}
          <div className="form-group">
            <label htmlFor="timeout">Timeout (seconds)</label>
            <input
              id="timeout"
              type="number"
              value={config.timeout_seconds}
              onChange={(e) =>
                updateField('api_config.timeout_seconds', parseInt(e.target.value))
              }
              min="1"
              max="300"
            />
          </div>

          {/* Retry Count */}
          <div className="form-group">
            <label htmlFor="retry_count">Retry Count</label>
            <input
              id="retry_count"
              type="number"
              value={config.retry_count}
              onChange={(e) =>
                updateField('api_config.retry_count', parseInt(e.target.value))
              }
              min="0"
              max="5"
            />
          </div>

          {/* SSL Verification */}
          <div className="form-group checkbox">
            <label>
              <input
                type="checkbox"
                checked={config.verify_ssl}
                onChange={(e) =>
                  updateField('api_config.verify_ssl', e.target.checked)
                }
              />
              Verify SSL Certificate
            </label>
          </div>
        </div>
      )}

      {/* Buttons */}
      <div className="form-actions">
        <button type="button" onClick={onBack} className="btn-secondary">
          ← Back
        </button>
        <button
          type="button"
          onClick={onNext}
          disabled={!isValid()}
          className="btn-primary"
        >
          Next: Parameters →
        </button>
      </div>
    </div>
  );
};

// Key-Value Pair Editor Component
const KeyValueEditor: React.FC<{
  value: Record<string, string>;
  onChange: (value: Record<string, string>) => void;
  placeholder: { key: string; value: string };
}> = ({ value, onChange, placeholder }) => {
  const pairs = Object.entries(value);

  const addPair = () => {
    onChange({ ...value, '': '' });
  };

  const updatePair = (index: number, key: string, val: string) => {
    const newValue = { ...value };
    const oldKey = pairs[index][0];
    delete newValue[oldKey];
    if (key) newValue[key] = val;
    onChange(newValue);
  };

  const removePair = (index: number) => {
    const newValue = { ...value };
    delete newValue[pairs[index][0]];
    onChange(newValue);
  };

  return (
    <div className="key-value-editor">
      {pairs.map(([key, val], index) => (
        <div key={index} className="kv-row">
          <input
            type="text"
            value={key}
            onChange={(e) => updatePair(index, e.target.value, val)}
            placeholder={placeholder.key}
          />
          <input
            type="text"
            value={val}
            onChange={(e) => updatePair(index, key, e.target.value)}
            placeholder={placeholder.value}
          />
          <button type="button" onClick={() => removePair(index)}>
            ✕
          </button>
        </div>
      ))}
      <button type="button" onClick={addPair} className="add-pair-btn">
        + Add {pairs.length === 0 ? 'Pair' : 'Another'}
      </button>
    </div>
  );
};
```

### Step 3: Parameters

```typescript
// components/steps/ParametersStep.tsx
import React from 'react';
import { CustomAction, ToolParameter } from '../../types/customActions';

interface Props {
  formData: CustomAction;
  updateField: (path: string, value: any) => void;
  onNext: () => void;
  onBack: () => void;
}

export const ParametersStep: React.FC<Props> = ({
  formData,
  updateField,
  onNext,
  onBack,
}) => {
  const addParameter = () => {
    const newParam: ToolParameter = {
      name: '',
      type: 'string',
      description: '',
      required: false,
    };
    updateField('parameters', [...formData.parameters, newParam]);
  };

  const updateParameter = (index: number, field: string, value: any) => {
    const updated = [...formData.parameters];
    updated[index] = { ...updated[index], [field]: value };
    updateField('parameters', updated);
  };

  const removeParameter = (index: number) => {
    const updated = formData.parameters.filter((_, i) => i !== index);
    updateField('parameters', updated);
  };

  const isValid = () => {
    return (
      formData.parameters.length > 0 &&
      formData.parameters.every(
        (p) => p.name && p.type && p.description.length >= 10
      )
    );
  };

  return (
    <div className="step-content">
      <h2>Parameters</h2>
      <p className="subtitle">
        Define what information the AI needs to provide when calling this action.
      </p>

      {formData.parameters.length === 0 && (
        <div className="empty-state">
          <p>No parameters defined yet.</p>
          <p>Add parameters that your API endpoint requires.</p>
        </div>
      )}

      {/* Parameter List */}
      <div className="parameters-list">
        {formData.parameters.map((param, index) => (
          <div key={index} className="parameter-card">
            <div className="param-header">
              <h4>Parameter {index + 1}</h4>
              <button
                type="button"
                onClick={() => removeParameter(index)}
                className="remove-btn"
              >
                Remove
              </button>
            </div>

            <div className="param-content">
              {/* Parameter Name */}
              <div className="form-group">
                <label>Name <span className="required">*</span></label>
                <input
                  type="text"
                  value={param.name}
                  onChange={(e) =>
                    updateParameter(index, 'name', e.target.value.toLowerCase())
                  }
                  placeholder="product_id"
                  pattern="^[a-z0-9_]+$"
                  required
                />
                <small>Lowercase, numbers, underscores only</small>
              </div>

              {/* Parameter Type */}
              <div className="form-group">
                <label>Type <span className="required">*</span></label>
                <select
                  value={param.type}
                  onChange={(e) => updateParameter(index, 'type', e.target.value)}
                  required
                >
                  <option value="string">String</option>
                  <option value="number">Number</option>
                  <option value="integer">Integer</option>
                  <option value="boolean">Boolean</option>
                  <option value="array">Array</option>
                  <option value="object">Object</option>
                </select>
              </div>

              {/* Description */}
              <div className="form-group">
                <label>Description <span className="required">*</span></label>
                <textarea
                  value={param.description}
                  onChange={(e) =>
                    updateParameter(index, 'description', e.target.value)
                  }
                  placeholder="Describe what this parameter is for. The AI uses this to understand what value to provide."
                  rows={2}
                  required
                  minLength={10}
                />
                <small>{param.description.length} / 500 characters</small>
              </div>

              <div className="param-row">
                {/* Required Checkbox */}
                <div className="form-group checkbox">
                  <label>
                    <input
                      type="checkbox"
                      checked={param.required}
                      onChange={(e) =>
                        updateParameter(index, 'required', e.target.checked)
                      }
                    />
                    Required
                  </label>
                </div>

                {/* Default Value */}
                <div className="form-group">
                  <label>Default Value (for testing)</label>
                  <input
                    type="text"
                    value={param.default || ''}
                    onChange={(e) =>
                      updateParameter(index, 'default', e.target.value)
                    }
                    placeholder="Optional default"
                  />
                </div>
              </div>

              {/* Advanced Validation (collapsible) */}
              <details className="validation-options">
                <summary>Advanced Validation</summary>

                {/* Enum Values */}
                {param.type === 'string' && (
                  <div className="form-group">
                    <label>Allowed Values (comma-separated)</label>
                    <input
                      type="text"
                      value={param.enum?.join(', ') || ''}
                      onChange={(e) =>
                        updateParameter(
                          index,
                          'enum',
                          e.target.value.split(',').map((s) => s.trim())
                        )
                      }
                      placeholder="USD, EUR, GBP"
                    />
                  </div>
                )}

                {/* Pattern */}
                {param.type === 'string' && (
                  <div className="form-group">
                    <label>Pattern (regex)</label>
                    <input
                      type="text"
                      value={param.pattern || ''}
                      onChange={(e) => updateParameter(index, 'pattern', e.target.value)}
                      placeholder="^[A-Z0-9-]+$"
                    />
                  </div>
                )}

                {/* Min/Max for numbers */}
                {(param.type === 'number' || param.type === 'integer') && (
                  <>
                    <div className="form-group">
                      <label>Minimum Value</label>
                      <input
                        type="number"
                        value={param.minimum ?? ''}
                        onChange={(e) =>
                          updateParameter(index, 'minimum', parseFloat(e.target.value))
                        }
                      />
                    </div>
                    <div className="form-group">
                      <label>Maximum Value</label>
                      <input
                        type="number"
                        value={param.maximum ?? ''}
                        onChange={(e) =>
                          updateParameter(index, 'maximum', parseFloat(e.target.value))
                        }
                      />
                    </div>
                  </>
                )}

                {/* Min/Max length for strings */}
                {param.type === 'string' && (
                  <>
                    <div className="form-group">
                      <label>Min Length</label>
                      <input
                        type="number"
                        value={param.minLength ?? ''}
                        onChange={(e) =>
                          updateParameter(index, 'minLength', parseInt(e.target.value))
                        }
                      />
                    </div>
                    <div className="form-group">
                      <label>Max Length</label>
                      <input
                        type="number"
                        value={param.maxLength ?? ''}
                        onChange={(e) =>
                          updateParameter(index, 'maxLength', parseInt(e.target.value))
                        }
                      />
                    </div>
                  </>
                )}
              </details>
            </div>
          </div>
        ))}
      </div>

      {/* Add Parameter Button */}
      <button type="button" onClick={addParameter} className="add-param-btn">
        + Add Parameter
      </button>

      {/* Usage Example */}
      {formData.parameters.length > 0 && (
        <div className="usage-example">
          <h4>📝 How this will be used:</h4>
          <p>
            The AI will automatically extract these values from the conversation and
            call your API with:
          </p>
          <pre>
            {JSON.stringify(
              formData.parameters.reduce(
                (acc, p) => ({
                  ...acc,
                  [p.name]: p.default || `<${p.type}>`,
                }),
                {}
              ),
              null,
              2
            )}
          </pre>
        </div>
      )}

      {/* Buttons */}
      <div className="form-actions">
        <button type="button" onClick={onBack} className="btn-secondary">
          ← Back
        </button>
        <button
          type="button"
          onClick={onNext}
          disabled={!isValid()}
          className="btn-primary"
        >
          Next: Test & Save →
        </button>
      </div>
    </div>
  );
};
```

### Step 4: Test & Save

```typescript
// components/steps/TestAndSaveStep.tsx
import React from 'react';
import { CustomAction, TestResult } from '../../types/customActions';

interface Props {
  formData: CustomAction;
  testResult: TestResult | null;
  testing: boolean;
  saving: boolean;
  onTest: () => void;
  onSave: () => void;
  onBack: () => void;
}

export const TestAndSaveStep: React.FC<Props> = ({
  formData,
  testResult,
  testing,
  saving,
  onTest,
  onSave,
  onBack,
}) => {
  return (
    <div className="step-content">
      <h2>Test & Save</h2>
      <p className="subtitle">
        Test your action to ensure it works correctly before saving.
      </p>

      {/* Configuration Summary */}
      <div className="summary-card">
        <h3>Configuration Summary</h3>
        <div className="summary-item">
          <strong>Action Name:</strong> {formData.display_name}
        </div>
        <div className="summary-item">
          <strong>API Endpoint:</strong>
          <code>
            {formData.api_config.method} {formData.api_config.base_url}
            {formData.api_config.endpoint}
          </code>
        </div>
        <div className="summary-item">
          <strong>Parameters:</strong> {formData.parameters.length} defined
        </div>
        <div className="summary-item">
          <strong>Authentication:</strong> {formData.api_config.auth_type}
        </div>
      </div>

      {/* Test Button */}
      <div className="test-section">
        <button
          type="button"
          onClick={onTest}
          disabled={testing}
          className="btn-test"
        >
          {testing ? '⏳ Testing...' : '🧪 Test Action'}
        </button>
        <p className="test-help">
          This will make an actual API call using the default parameter values.
        </p>
      </div>

      {/* Test Result */}
      {testResult && (
        <div className={`test-result ${testResult.success ? 'success' : 'error'}`}>
          <h4>
            {testResult.success ? '✅ Test Successful!' : '❌ Test Failed'}
          </h4>

          {testResult.success ? (
            <>
              <div className="result-item">
                <strong>Status Code:</strong> {testResult.status_code}
              </div>
              <div className="result-item">
                <strong>Response Time:</strong> {testResult.response_time_ms}ms
              </div>
              <div className="result-item">
                <strong>Request URL:</strong>
                <code>{testResult.request_url}</code>
              </div>

              {testResult.extracted_data && (
                <details>
                  <summary>Extracted Data</summary>
                  <pre>{JSON.stringify(testResult.extracted_data, null, 2)}</pre>
                </details>
              )}

              {testResult.response_body && (
                <details>
                  <summary>Full Response</summary>
                  <pre>{testResult.response_body}</pre>
                </details>
              )}
            </>
          ) : (
            <>
              <div className="error-message">
                <strong>Error:</strong> {testResult.error}
              </div>

              {testResult.status_code && (
                <div className="result-item">
                  <strong>Status Code:</strong> {testResult.status_code}
                </div>
              )}

              <div className="troubleshooting">
                <strong>💡 Troubleshooting:</strong>
                <ul>
                  <li>Check that your API endpoint is correct</li>
                  <li>Verify your authentication credentials</li>
                  <li>Ensure parameter defaults are valid values</li>
                  <li>Check if the API is accessible from our servers</li>
                </ul>
              </div>
            </>
          )}
        </div>
      )}

      {/* Warning if not tested */}
      {!testResult && !testing && (
        <div className="warning-box">
          ⚠️ We recommend testing your action before saving to ensure it works correctly.
        </div>
      )}

      {/* Save Button */}
      <div className="form-actions">
        <button type="button" onClick={onBack} className="btn-secondary">
          ← Back
        </button>
        <button
          type="button"
          onClick={onSave}
          disabled={saving || (!testResult?.success && testResult !== null)}
          className="btn-primary btn-save"
        >
          {saving ? '💾 Saving...' : '💾 Save Action'}
        </button>
      </div>

      {/* Success message after testing */}
      {testResult?.success && (
        <div className="success-message">
          ✅ Your action is ready! Click "Save Action" to make it available to your
          chatbot.
        </div>
      )}
    </div>
  );
};
```

---

## Tool Execution Layer

This is the core implementation that converts DB configs into executable tools.

```go
// tools/custom_action_tool.go
package tools

import (
    "bytes"
    "context"
    "crypto/tls"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "regexp"
    "strings"
    "time"

    "github.com/cloudwego/eino/components/tool"
    "github.com/cloudwego/eino/schema"
    "github.com/tidwall/gjson"
    "go.uber.org/zap"

    "github.com/Conversly/lightning-response/internal/models"
    "github.com/Conversly/lightning-response/internal/utils"
)

// CustomActionTool implements the Eino InvokableTool interface
type CustomActionTool struct {
    action     *models.CustomAction
    httpClient *http.Client
    db         *loaders.PostgresClient
    logger     *zap.Logger
}

// NewCustomActionTool creates a tool from a CustomAction model
func NewCustomActionTool(
    ctx context.Context,
    action *models.CustomAction,
    db *loaders.PostgresClient,
) (*CustomActionTool, error) {
    // Validate configuration
    if err := validateActionConfig(action); err != nil {
        return nil, fmt.Errorf("invalid action config: %w", err)
    }

    // Create HTTP client with custom settings
    httpClient := &http.Client{
        Timeout: time.Duration(action.APIConfig.TimeoutSeconds) * time.Second,
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{
                InsecureSkipVerify: !action.APIConfig.VerifySSL,
            },
            MaxIdleConns:        100,
            MaxIdleConnsPerHost: 10,
            IdleConnTimeout:     90 * time.Second,
        },
    }

    if !action.APIConfig.FollowRedirects {
        httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
            return http.ErrUseLastResponse
        }
    }

    return &CustomActionTool{
        action:     action,
        httpClient: httpClient,
        db:         db,
        logger: utils.Zlog.With(
            zap.String("tool", action.Name),
            zap.String("action_id", action.ID),
        ),
    }, nil
}

// Info returns tool metadata for the LLM
func (t *CustomActionTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
    // Convert tool schema to Eino format
    toolSchema := t.action.Parameters

    // Build JSON Schema for parameters
    properties := make(map[string]interface{})
    required := make([]string, 0)

    for _, param := range toolSchema {
        propSchema := map[string]interface{}{
            "type":        param.Type,
            "description": param.Description,
        }

        if len(param.Enum) > 0 {
            propSchema["enum"] = param.Enum
        }
        if param.Default != "" {
            propSchema["default"] = param.Default
        }
        if param.Pattern != "" {
            propSchema["pattern"] = param.Pattern
        }
        if param.Minimum != nil {
            propSchema["minimum"] = *param.Minimum
        }
        if param.Maximum != nil {
            propSchema["maximum"] = *param.Maximum
        }
        if param.MinLength != nil {
            propSchema["minLength"] = *param.MinLength
        }
        if param.MaxLength != nil {
            propSchema["maxLength"] = *param.MaxLength
        }

        properties[param.Name] = propSchema

        if param.Required {
            required = append(required, param.Name)
        }
    }

    parametersSchema := map[string]interface{}{
        "type":       "object",
        "properties": properties,
        "required":   required,
    }

    return &schema.ToolInfo{
        Name:        t.action.Name,
        Desc:        t.action.Description,
        ParamsOneOf: schema.NewParamsOneOfByParams(parametersSchema),
    }, nil
}

// InvokableRun executes the API call
func (t *CustomActionTool) InvokableRun(
    ctx context.Context,
    argumentsInJSON string,
    opts ...tool.Option,
) (string, error) {
    startTime := time.Now()

    // Parse input parameters
    var params map[string]interface{}
    if err := json.Unmarshal([]byte(argumentsInJSON), &params); err != nil {
        return "", fmt.Errorf("failed to parse arguments: %w", err)
    }

    t.logger.Info("Executing custom action",
        zap.Any("params", params))

    // Check rate limits
    if allowed, err := t.checkRateLimit(ctx); !allowed {
        return "", fmt.Errorf("rate limit exceeded: %w", err)
    }

    // Build HTTP request
    req, err := t.buildRequest(ctx, params)
    if err != nil {
        return "", t.handleError(ctx, params, err, startTime)
    }

    // Execute with retries
    var lastErr error
    maxAttempts := t.action.APIConfig.RetryCount + 1

    for attempt := 0; attempt < maxAttempts; attempt++ {
        if attempt > 0 {
            t.logger.Warn("Retrying API call",
                zap.Int("attempt", attempt),
                zap.Error(lastErr))

            // Exponential backoff
            backoff := time.Duration(attempt*attempt) * time.Second
            select {
            case <-time.After(backoff):
            case <-ctx.Done():
                return "", ctx.Err()
            }
        }

        resp, err := t.httpClient.Do(req)
        if err != nil {
            lastErr = err
            continue
        }

        // Process response
        result, err := t.handleResponse(ctx, resp, params)
        resp.Body.Close()

        if err != nil {
            lastErr = err
            // Don't retry on 4xx errors (client errors)
            if resp.StatusCode >= 400 && resp.StatusCode < 500 {
                break
            }
            continue
        }

        // Success! Log and return
        t.logExecution(ctx, params, req, resp, result, time.Since(startTime), nil)
        return result, nil
    }

    // All attempts failed
    return "", t.handleError(ctx, params, lastErr, startTime)
}

// buildRequest constructs the HTTP request
func (t *CustomActionTool) buildRequest(
    ctx context.Context,
    params map[string]interface{},
) (*http.Request, error) {
    cfg := t.action.APIConfig

    // Build URL with variable substitution
    url := cfg.BaseURL + t.replaceVariables(cfg.Endpoint, params)

    // Add query parameters
    if len(cfg.QueryParams) > 0 {
        queryParts := make([]string, 0, len(cfg.QueryParams))
        for key, value := range cfg.QueryParams {
            replacedValue := t.replaceVariables(value, params)
            queryParts = append(queryParts, fmt.Sprintf("%s=%s", key, replacedValue))
        }
        url += "?" + strings.Join(queryParts, "&")
    }

    // Build request body
    var bodyReader io.Reader
    if cfg.BodyTemplate != "" {
        bodyStr := t.replaceVariables(cfg.BodyTemplate, params)
        bodyReader = bytes.NewBufferString(bodyStr)
    }

    // Create request
    req, err := http.NewRequestWithContext(ctx, cfg.Method, url, bodyReader)
    if err != nil {
        return nil, fmt.Errorf("failed to create request: %w", err)
    }

    // Add headers
    for key, value := range cfg.Headers {
        req.Header.Set(key, t.replaceVariables(value, params))
    }

    // Add authentication
    if err := t.addAuthentication(req); err != nil {
        return nil, fmt.Errorf("failed to add authentication: %w", err)
    }

    // Set default Content-Type if not specified
    if req.Header.Get("Content-Type") == "" && cfg.BodyTemplate != "" {
        req.Header.Set("Content-Type", "application/json")
    }

    return req, nil
}

// handleResponse processes the API response
func (t *CustomActionTool) handleResponse(
    ctx context.Context,
    resp *http.Response,
    params map[string]interface{},
) (string, error) {
    cfg := t.action.APIConfig

    // Check if status code is in success list
    isSuccess := false
    for _, code := range cfg.SuccessCodes {
        if resp.StatusCode == code {
            isSuccess = true
            break
        }
    }

    // Read response body
    bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB limit
    if err != nil {
        return "", fmt.Errorf("failed to read response: %w", err)
    }

    if !isSuccess {
        return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
    }

    // Try to parse as JSON
    var responseData interface{}
    if err := json.Unmarshal(bodyBytes, &responseData); err != nil {
        // Not JSON, return raw string
        return string(bodyBytes), nil
    }

    // Apply response mapping if configured
    if cfg.ResponseMapping != "" {
        extracted := t.extractByJSONPath(string(bodyBytes), cfg.ResponseMapping)
        if extracted == "" {
            t.logger.Warn("JSONPath mapping returned empty result",
                zap.String("path", cfg.ResponseMapping))
            // Fall back to full response
            return string(bodyBytes), nil
        }
        return extracted, nil
    }

    // Return formatted JSON
    formattedJSON, _ := json.MarshalIndent(responseData, "", "  ")
    return string(formattedJSON), nil
}

// replaceVariables substitutes {{variable}} placeholders
func (t *CustomActionTool) replaceVariables(
    template string,
    params map[string]interface{},
) string {
    result := template
    re := regexp.MustCompile(`\{\{(\w+)\}\}`)

    matches := re.FindAllStringSubmatch(template, -1)
    for _, match := range matches {
        placeholder := match[0]  // {{variable}}
        varName := match[1]      // variable

        if value, ok := params[varName]; ok {
            // Format value based on type
            var strValue string
            switch v := value.(type) {
            case string:
                strValue = v
            case float64:
                // Check if it's actually an integer
                if v == float64(int64(v)) {
                    strValue = fmt.Sprintf("%d", int64(v))
                } else {
                    strValue = fmt.Sprintf("%f", v)
                }
            default:
                strValue = fmt.Sprint(value)
            }

            result = strings.ReplaceAll(result, placeholder, strValue)
        }
    }

    return result
}

// extractByJSONPath extracts data using JSONPath (using gjson library)
func (t *CustomActionTool) extractByJSONPath(jsonData string, path string) string {
    // Convert $. prefix to gjson format
    gjsonPath := strings.TrimPrefix(path, "$.")

    result := gjson.Get(jsonData, gjsonPath)
    if !result.Exists() {
        return ""
    }

    return result.String()
}

// addAuthentication adds auth headers
func (t *CustomActionTool) addAuthentication(req *http.Request) error {
    cfg := t.action.APIConfig

    switch cfg.AuthType {
    case "bearer":
        if cfg.AuthValue != "" {
            // Decrypt if encrypted
            token := t.decryptAuthValue(cfg.AuthValue)
            req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
        }
    case "api_key":
        if cfg.AuthValue != "" {
            token := t.decryptAuthValue(cfg.AuthValue)
            req.Header.Set("X-API-Key", token)
        }
    case "basic":
        if cfg.AuthValue != "" {
            token := t.decryptAuthValue(cfg.AuthValue)
            req.Header.Set("Authorization", fmt.Sprintf("Basic %s", token))
        }
    }

    return nil
}

// checkRateLimit verifies if action can be executed
func (t *CustomActionTool) checkRateLimit(ctx context.Context) (bool, error) {
    var allowed bool
    err := t.db.QueryRow(ctx,
        "SELECT check_rate_limit($1, $2)",
        t.action.ChatbotID,
        t.action.ID,
    ).Scan(&allowed)

    return allowed, err
}

// logExecution stores execution details
func (t *CustomActionTool) logExecution(
    ctx context.Context,
    params map[string]interface{},
    req *http.Request,
    resp *http.Response,
    result string,
    duration time.Duration,
    err error,
) {
    // Log asynchronously to not block execution
    go func() {
        logCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        success := err == nil
        errorMsg := ""
        if err != nil {
            errorMsg = err.Error()
        }

        statusCode := 0
        if resp != nil {
            statusCode = resp.StatusCode
        }

        paramsJSON, _ := json.Marshal(params)

        _, logErr := t.db.Exec(logCtx, `
            INSERT INTO custom_action_logs (
                action_id, chatbot_id, input_params,
                request_url, request_method, response_status,
                success, error_message, execution_time_ms
            ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        `,
            t.action.ID,
            t.action.ChatbotID,
            paramsJSON,
            req.URL.String(),
            req.Method,
            statusCode,
            success,
            errorMsg,
            duration.Milliseconds(),
        )

        if logErr != nil {
            t.logger.Error("Failed to log action execution", zap.Error(logErr))
        }
    }()
}

// handleError logs error and returns formatted error message
func (t *CustomActionTool) handleError(
    ctx context.Context,
    params map[string]interface{},
    err error,
    duration time.Duration,
) error {
    t.logger.Error("Custom action failed",
        zap.Error(err),
        zap.Duration("duration", duration))

    // Log the failure
    go t.logExecution(ctx, params, nil, nil, "", duration, err)

    return fmt.Errorf("action '%s' failed: %w", t.action.DisplayName, err)
}

// decryptAuthValue decrypts stored auth credentials
func (t *CustomActionTool) decryptAuthValue(encrypted string) string {
    // Check if it's encrypted
    if strings.HasPrefix(encrypted, "encrypted:") {
        // TODO: Implement actual decryption
        // For now, strip the prefix
        return strings.TrimPrefix(encrypted, "encrypted:")
    }
    return encrypted
}

// validateActionConfig ensures config is valid
func validateActionConfig(action *models.CustomAction) error {
    if action.Name == "" {
        return fmt.Errorf("action name is required")
    }
    if action.APIConfig == nil {
        return fmt.Errorf("api_config is required")
    }
    if action.APIConfig.BaseURL == "" {
        return fmt.Errorf("base_url is required")
    }
    if action.APIConfig.Endpoint == "" {
        return fmt.Errorf("endpoint is required")
    }
    if len(action.Parameters) == 0 {
        return fmt.Errorf("at least one parameter is required")
    }
    return nil
}
```

---

## Graph Integration Layer

Integrate custom actions into your existing graph system:

```go
// response/tools.go
package response

import (
    "context"
    "fmt"

    "github.com/cloudwego/eino/components/tool"
    "go.uber.org/zap"

    "github.com/Conversly/lightning-response/internal/rag"
    "github.com/Conversly/lightning-response/internal/tools"
    "github.com/Conversly/lightning-response/internal/utils"
)

// GetEnabledTools loads all tools for a chatbot (built-in + custom)
func GetEnabledTools(
    ctx context.Context,
    cfg *ChatbotConfig,
    deps *GraphDependencies,
) ([]tool.InvokableTool, error) {
    toolsList := make([]tool.InvokableTool, 0)

    // 1. Load built-in tools based on configuration
    for _, toolName := range cfg.ToolConfigs {
        switch toolName {
        case "rag":
            ragTool := rag.NewRAGTool(
                deps.DB,
                deps.Embedder,
                cfg.ChatbotID,
                int(cfg.TopK),
            )
            toolsList = append(toolsList, ragTool)
            utils.Zlog.Debug("Added RAG tool",
                zap.String("chatbot_id", cfg.ChatbotID))

        // Add more built-in tools here as needed
        // case "web_search":
        //     ...
        // case "calendar":
        //     ...
        }
    }

    // 2. Load custom actions from database
    customActions, err := deps.DB.GetEnabledCustomActions(ctx, cfg.ChatbotID)
    if err != nil {
        utils.Zlog.Error("Failed to load custom actions",
            zap.String("chatbot_id", cfg.ChatbotID),
            zap.Error(err))
        return nil, fmt.Errorf("failed to load custom actions: %w", err)
    }

    // 3. Convert custom actions to tools
    for _, action := range customActions {
        customTool, err := tools.NewCustomActionTool(ctx, action, deps.DB)
        if err != nil {
            utils.Zlog.Warn("Failed to create custom tool, skipping",
                zap.String("action_id", action.ID),
                zap.String("action_name", action.Name),
                zap.Error(err))
            continue
        }

        toolsList = append(toolsList, customTool)
        utils.Zlog.Debug("Added custom action tool",
            zap.String("chatbot_id", cfg.ChatbotID),
            zap.String("action_name", action.Name))
    }

    utils.Zlog.Info("Loaded tools for chatbot",
        zap.String("chatbot_id", cfg.ChatbotID),
        zap.Int("total_tools", len(toolsList)),
        zap.Int("custom_actions", len(customActions)),
        zap.Int("built_in_tools", len(toolsList)-len(customActions)))

    return toolsList, nil
}
```

The graph already handles tools automatically via your existing `BuildChatbotGraph` function. No changes needed there! The custom actions are seamlessly integrated.

---

## Testing & Validation

### Unit Tests

```go
// tools/custom_action_tool_test.go
package tools

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/Conversly/lightning-response/internal/models"
)

func TestCustomActionTool_BasicGET(t *testing.T) {
    // Create test server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "GET", r.Method)
        assert.Equal(t, "/products/123", r.URL.Path)

        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"id": "123", "name": "Test Product", "price": 29.99}`))
    }))
    defer server.Close()

    // Create action config
    action := &models.CustomAction{
        ID:          "test-action",
        ChatbotID:   "test-chatbot",
        Name:        "get_product",
        DisplayName: "Get Product",
        Description: "Fetch product details",
        APIConfig: &models.CustomActionConfig{
            Method:         "GET",
            BaseURL:        server.URL,
            Endpoint:       "/products/{{product_id}}",
            SuccessCodes:   []int{200},
            TimeoutSeconds: 30,
            RetryCount:     0,
            AuthType:       "none",
            FollowRedirects: true,
            VerifySSL:      true,
        },
        Parameters: []models.ToolParameter{
            {
                Name:        "product_id",
                Type:        "string",
                Description: "Product ID",
                Required:    true,
            },
        },
    }

    // Create tool
    tool, err := NewCustomActionTool(context.Background(), action, nil)
    require.NoError(t, err)

    // Execute
    result, err := tool.InvokableRun(
        context.Background(),
        `{"product_id": "123"}`,
    )

    // Verify
    require.NoError(t, err)
    assert.Contains(t, result, "Test Product")
    assert.Contains(t, result, "29.99")
}

func TestCustomActionTool_POSTWithBody(t *testing.T) {
    // Create test server
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "POST", r.Method)
        assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

        // Read body
        var body map[string]interface{}
        json.NewDecoder(r.Body).Decode(&body)
        assert.Equal(t, "Test Product", body["name"])
        assert.Equal(t, float64(29.99), body["price"])

        w.WriteHeader(http.StatusCreated)
        w.Write([]byte(`{"id": "456", "status": "created"}`))
    }))
    defer server.Close()

    action := &models.CustomAction{
        ID:          "test-action",
        ChatbotID:   "test-chatbot",
        Name:        "create_product",
        DisplayName: "Create Product",
        Description: "Create a new product",
        APIConfig: &models.CustomActionConfig{
            Method:         "POST",
            BaseURL:        server.URL,
            Endpoint:       "/products",
            BodyTemplate:   `{"name": "{{name}}", "price": {{price}}}`,
            SuccessCodes:   []int{201},
            TimeoutSeconds: 30,
            RetryCount:     0,
            AuthType:       "none",
        },
        Parameters: []models.ToolParameter{
            {Name: "name", Type: "string", Required: true},
            {Name: "price", Type: "number", Required: true},
        },
    }

    tool, err := NewCustomActionTool(context.Background(), action, nil)
    require.NoError(t, err)

    result, err := tool.InvokableRun(
        context.Background(),
        `{"name": "Test Product", "price": 29.99}`,
    )

    require.NoError(t, err)
    assert.Contains(t, result, "created")
}
```

### Integration Tests

```go
// integration_test.go
func TestCustomAction_EndToEnd(t *testing.T) {
    // 1. Create chatbot
    chatbotID := createTestChatbot(t)

    // 2. Create custom action via API
    actionPayload := map[string]interface{}{
        "chatbot_id": chatbotID,
        "name":       "test_action",
        "display_name": "Test Action",
        "description": "Test action for integration testing",
        "api_config": map[string]interface{}{
            "method":   "GET",
            "base_url": testServer.URL,
            "endpoint": "/test",
        },
        "parameters": []map[string]interface{}{
            {
                "name":        "param1",
                "type":        "string",
                "description": "Test parameter",
                "required":    true,
            },
        },
    }

    resp := makeAPIRequest(t, "POST", "/api/v1/chatbots/"+chatbotID+"/actions", actionPayload)
    assert.Equal(t, http.StatusCreated, resp.StatusCode)

    // 3. Send chat message that should trigger the action
    chatResp := sendChatMessage(t, chatbotID, "Please fetch data using test_action with param1=hello")

    // 4. Verify action was called
    assert.Contains(t, chatResp.Response, "expected result")

    // 5. Check logs
    logs := getActionLogs(t, chatbotID)
    assert.Len(t, logs, 1)
    assert.True(t, logs[0].Success)
}
```

---

## Security & Rate Limiting

### Encryption for Sensitive Data

```go
// security/encryption.go
package security

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "io"
)

var encryptionKey []byte // Load from env: AES-256 key

func EncryptAuthValue(plaintext string) (string, error) {
    block, err := aes.NewCipher(encryptionKey)
    if err != nil {
        return "", err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }

    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", err
    }

    ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    return "encrypted:" + base64.StdEncoding.EncodeToString(ciphertext), nil
}

func DecryptAuthValue(encrypted string) (string, error) {
    if !strings.HasPrefix(encrypted, "encrypted:") {
        return encrypted, nil // Not encrypted
    }

    encrypted = strings.TrimPrefix(encrypted, "encrypted:")
    ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
    if err != nil {
        return "", err
    }

    block, err := aes.NewCipher(encryptionKey)
    if err != nil {
        return "", err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }

    nonceSize := gcm.NonceSize()
    if len(ciphertext) < nonceSize {
        return "", fmt.Errorf("ciphertext too short")
    }

    nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
    plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return "", err
    }

    return string(plaintext), nil
}
```

### Rate Limiting Middleware

```go
// middleware/rate_limit.go
func ActionRateLimitMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        actionID := c.Param("action_id")
        chatbotID := c.Param("chatbot_id")

        // Check rate limit
        var allowed bool
        err := db.QueryRow(c.Request.Context(),
            "SELECT check_rate_limit($1, $2)",
            chatbotID, actionID,
        ).Scan(&allowed)

        if err != nil || !allowed {
            c.JSON(http.StatusTooManyRequests, gin.H{
                "error": "Rate limit exceeded",
                "retry_after": 60, // seconds
            })
            c.Abort()
            return
        }

        c.Next()
    }
}
```

---

## Monitoring & Debugging

### Logging Dashboard Component

```typescript
// components/ActionLogsViewer.tsx
export const ActionLogsViewer: React.FC<{ actionId: string }> = ({ actionId }) => {
  const [logs, setLogs] = useState<ActionLog[]>([]);
  const [filter, setFilter] = useState<'all' | 'success' | 'failed'>('all');

  useEffect(() => {
    fetchLogs();
  }, [actionId, filter]);

  const fetchLogs = async () => {
    const response = await fetch(
      `/api/v1/actions/${actionId}/logs?only_failed=${filter === 'failed'}`
    );
    const data = await response.json();
    setLogs(data.logs);
  };

  return (
    <div className="logs-viewer">
      <div className="filters">
        <button onClick={() => setFilter('all')}>All</button>
        <button onClick={() => setFilter('success')}>Success</button>
        <button onClick={() => setFilter('failed')}>Failed</button>
      </div>

      <div className="logs-list">
        {logs.map((log) => (
          <div key={log.id} className={`log-entry ${log.success ? 'success' : 'error'}`}>
            <div className="log-header">
              <span className="timestamp">{formatDate(log.executed_at)}</span>
              <span className="status">
                {log.success ? '✅' : '❌'} {log.response_status}
              </span>
              <span className="duration">{log.execution_time_ms}ms</span>
            </div>

            <details>
              <summary>View Details</summary>
              <div className="log-details">
                <div>
                  <strong>Input:</strong>
                  <pre>{JSON.stringify(log.input_params, null, 2)}</pre>
                </div>
                <div>
                  <strong>URL:</strong>
                  <code>{log.request_url}</code>
                </div>
                {log.error_message && (
                  <div className="error">
                    <strong>Error:</strong>
                    <p>{log.error_message}</p>
                  </div>
                )}
              </div>
            </details>
          </div>
        ))}
      </div>
    </div>
  );
};
```

---

## Summary

This complete system provides:

✅ **Frontend Layer**: Multi-step form with validation and testing
✅ **API Layer**: RESTful endpoints for CRUD operations
✅ **Database Layer**: Comprehensive schema with logging and rate limiting
✅ **Tool Layer**: Dynamic tool creation from configs
✅ **Graph Integration**: Seamless integration with existing Eino framework
✅ **Security**: Encryption, rate limiting, input validation
✅ **Observability**: Comprehensive logging and monitoring

The custom actions will automatically be picked up by your graph engine and the LLM will decide when to use them based on the descriptions you provide!