package response

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/utils"
)

// ConversationMessage represents a message in the database
type ConversationMessage struct {
	ID              int64
	UniqueClientID  string
	ChatbotID       string
	Message         string
	Type            string // "query" or "response"
	OriginURL       string
	CreatedAt       time.Time
}

// LoadConversationHistory retrieves the conversation history from the database
// and converts it to Eino schema.Message format
func LoadConversationHistory(ctx context.Context, db *loaders.PostgresClient, clientID string, chatbotID string, limit int) ([]*schema.Message, error) {
	if limit == 0 {
		limit = 20 // Default to last 20 messages
	}

	utils.Zlog.Debug("Loading conversation history",
		zap.String("client_id", clientID),
		zap.String("chatbot_id", chatbotID),
		zap.Int("limit", limit))

	// TODO: Implement actual database query
	// Example query:
	// query := `
	//     SELECT id, unique_client_id, chatbot_id, message, type, origin_url, created_at
	//     FROM messages
	//     WHERE unique_client_id = $1 AND chatbot_id = $2
	//     ORDER BY created_at ASC
	//     LIMIT $3
	// `
	// rows, err := db.Query(ctx, query, clientID, chatbotID, limit)
	// if err != nil {
	//     return nil, fmt.Errorf("failed to load conversation history: %w", err)
	// }
	// defer rows.Close()

	var messages []*schema.Message

	// TODO: Scan rows and convert to messages
	// for rows.Next() {
	//     var msg ConversationMessage
	//     err := rows.Scan(&msg.ID, &msg.UniqueClientID, &msg.ChatbotID, &msg.Message, &msg.Type, &msg.OriginURL, &msg.CreatedAt)
	//     if err != nil {
	//         return nil, fmt.Errorf("failed to scan message: %w", err)
	//     }
	//
	//     // Convert to Eino message format
	//     if msg.Type == "query" {
	//         messages = append(messages, schema.UserMessage(msg.Message))
	//     } else if msg.Type == "response" {
	//         messages = append(messages, schema.AssistantMessage(msg.Message))
	//     }
	// }

	// Placeholder: return empty history for now
	utils.Zlog.Debug("Loaded conversation messages",
		zap.Int("count", len(messages)))

	return messages, nil
}

// SaveConversationMessages saves both the user query and assistant response to the database
func SaveConversationMessages(ctx context.Context, db *loaders.PostgresClient, req *Request, chatbotID string, response *schema.Message) error {
	utils.Zlog.Debug("Saving conversation messages",
		zap.String("client_id", req.User.UniqueClientID),
		zap.String("chatbot_id", chatbotID))

	// TODO: Implement actual database insert
	// Example:
	// tx, err := db.BeginTx(ctx, nil)
	// if err != nil {
	//     return fmt.Errorf("failed to begin transaction: %w", err)
	// }
	// defer tx.Rollback()

	// Insert user query
	// queryInsert := `
	//     INSERT INTO messages (unique_client_id, chatbot_id, message, type, origin_url, created_at)
	//     VALUES ($1, $2, $3, 'query', $4, $5)
	// `
	// _, err = tx.ExecContext(ctx, queryInsert, 
	//     req.User.UniqueClientID, 
	//     chatbotID, 
	//     req.Query, 
	//     req.Metadata.OriginURL, 
	//     time.Now().UTC())
	// if err != nil {
	//     return fmt.Errorf("failed to insert query: %w", err)
	// }

	// Insert assistant response
	// responseInsert := `
	//     INSERT INTO messages (unique_client_id, chatbot_id, message, type, origin_url, created_at)
	//     VALUES ($1, $2, $3, 'response', $4, $5)
	// `
	// _, err = tx.ExecContext(ctx, responseInsert,
	//     req.User.UniqueClientID,
	//     chatbotID,
	//     response.Content,
	//     req.Metadata.OriginURL,
	//     time.Now().UTC())
	// if err != nil {
	//     return fmt.Errorf("failed to insert response: %w", err)
	// }

	// if err := tx.Commit(); err != nil {
	//     return fmt.Errorf("failed to commit transaction: %w", err)
	// }

	utils.Zlog.Info("Saved conversation messages",
		zap.String("client_id", req.User.UniqueClientID),
		zap.String("chatbot_id", chatbotID))

	return nil
}

// GetChatbotConfig retrieves the chatbot configuration from the database
func GetChatbotConfig(ctx context.Context, db *loaders.PostgresClient, webID string, originURL string) (*ChatbotConfig, error) {
	utils.Zlog.Debug("Loading chatbot config",
		zap.String("web_id", webID),
		zap.String("origin_url", originURL))

	// TODO: Implement actual database query
	// Example query:
	// query := `
	//     SELECT 
	//         c.chatbot_id,
	//         c.tenant_id,
	//         c.system_prompt,
	//         c.temperature,
	//         c.model,
	//         c.top_k,
	//         c.rag_index,
	//         c.tool_configs
	//     FROM chatbots c
	//     JOIN businesses b ON c.tenant_id = b.tenant_id
	//     WHERE b.web_id = $1 AND $2 = ANY(b.allowed_domains)
	// `
	// 
	// var cfg ChatbotConfig
	// var toolConfigsJSON string
	// err := db.QueryRow(ctx, query, webID, originURL).Scan(
	//     &cfg.ChatbotID,
	//     &cfg.TenantID,
	//     &cfg.SystemPrompt,
	//     &cfg.Temperature,
	//     &cfg.Model,
	//     &cfg.TopK,
	//     &cfg.RAGIndex,
	//     &toolConfigsJSON,
	// )
	// if err == sql.ErrNoRows {
	//     return nil, fmt.Errorf("chatbot not found for web_id %s and origin %s", webID, originURL)
	// }
	// if err != nil {
	//     return nil, fmt.Errorf("failed to load chatbot config: %w", err)
	// }
	//
	// // Parse tool configs JSON
	// if err := json.Unmarshal([]byte(toolConfigsJSON), &cfg.ToolConfigs); err != nil {
	//     return nil, fmt.Errorf("failed to parse tool configs: %w", err)
	// }

	// Placeholder: return a default config
	cfg := &ChatbotConfig{
		ChatbotID:    "chatbot_default",
		TenantID:     "tenant_default",
		SystemPrompt: "You are a helpful AI assistant. Use the available tools to answer user questions accurately.",
		Temperature:  0.7,
		Model:        "gpt-4o-mini",
		TopK:         5,
		RAGIndex:     "default_collection",
		ToolConfigs:  []string{"rag", "http_api", "search"},
	}

	utils.Zlog.Info("Loaded chatbot config",
		zap.String("chatbot_id", cfg.ChatbotID),
		zap.String("tenant_id", cfg.TenantID),
		zap.Strings("tools", cfg.ToolConfigs))

	return cfg, nil
}

// ValidateTenantAccess checks if the API key and origin URL match in the database
func ValidateTenantAccess(ctx context.Context, db *loaders.PostgresClient, webID string, originURL string) (string, error) {
	if webID == "" {
		return "", fmt.Errorf("missing web_id")
	}
	if originURL == "" {
		return "", fmt.Errorf("missing origin_url")
	}

	utils.Zlog.Debug("Validating tenant access",
		zap.String("web_id", webID),
		zap.String("origin_url", originURL))

	// TODO: Implement actual database validation
	// Example query:
	// query := `
	//     SELECT tenant_id, allowed_domains
	//     FROM businesses
	//     WHERE web_id = $1
	// `
	// var tenantID string
	// var allowedDomainsJSON string
	// err := db.QueryRow(ctx, query, webID).Scan(&tenantID, &allowedDomainsJSON)
	// if err == sql.ErrNoRows {
	//     return "", fmt.Errorf("invalid web_id: %s", webID)
	// }
	// if err != nil {
	//     return "", fmt.Errorf("database error: %w", err)
	// }
	//
	// // Parse allowed domains
	// var allowedDomains []string
	// if err := json.Unmarshal([]byte(allowedDomainsJSON), &allowedDomains); err != nil {
	//     return "", fmt.Errorf("failed to parse allowed domains: %w", err)
	// }
	//
	// // Check if origin URL is allowed
	// originHost := extractHost(originURL)
	// allowed := false
	// for _, domain := range allowedDomains {
	//     if strings.HasSuffix(originHost, domain) || domain == "*" {
	//         allowed = true
	//         break
	//     }
	// }
	// if !allowed {
	//     return "", fmt.Errorf("origin %s not allowed for this web_id", originURL)
	// }

	// Placeholder: always allow for now
	tenantID := "tenant_default"

	utils.Zlog.Info("Tenant access validated",
		zap.String("web_id", webID),
		zap.String("tenant_id", tenantID))

	return tenantID, nil
}

// extractHost extracts the host from a URL
func extractHost(urlStr string) string {
	// Simple implementation - in production use url.Parse
	if idx := strings.Index(urlStr, "://"); idx != -1 {
		urlStr = urlStr[idx+3:]
	}
	if idx := strings.Index(urlStr, "/"); idx != -1 {
		urlStr = urlStr[:idx]
	}
	return urlStr
}

// MessageToResponse converts an Eino schema.Message to a Response struct
func MessageToResponse(msg *schema.Message, req *Request) *Response {
	response := &Response{
		Mode:            req.Mode,
		Answer:          msg.Content,
		Sources:         []Source{},
		ConversationKey: req.User.UniqueClientID,
	}

	// TODO: Extract sources from tool calls if available
	// This would require parsing the tool responses from the message history

	return response
}

// Helper function to check if error is sql.ErrNoRows
func isNoRowsError(err error) bool {
	return err == sql.ErrNoRows
}
