package response

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/utils"
)

// MessageRecord represents a message to be saved in the database
type MessageRecord struct {
	UniqueClientID string
	ChatbotID      string
	Message        string
	Role           string   // "user" or "assistant"
	Citations      []string
}

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

// ParseConversationMessages parses the query field which contains conversation history
func ParseConversationMessages(queryJSON string) ([]*schema.Message, error) {
	// Parse the JSON array of messages
	var rawMessages []map[string]interface{}
	if err := json.Unmarshal([]byte(queryJSON), &rawMessages); err != nil {
		return nil, fmt.Errorf("failed to parse conversation JSON: %w", err)
	}

	messages := make([]*schema.Message, 0, len(rawMessages))
	for _, raw := range rawMessages {
		role, ok := raw["role"].(string)
		if !ok {
			continue
		}

		content, ok := raw["content"].(string)
		if !ok {
			continue
		}

		if role == "user" {
			messages = append(messages, schema.UserMessage(content))
		} else if role == "assistant" {
			messages = append(messages, schema.AssistantMessage(content))
		}
	}

	return messages, nil
}

// Helper function to check if error is sql.ErrNoRows
func isNoRowsError(err error) bool {
	return err == sql.ErrNoRows
}
