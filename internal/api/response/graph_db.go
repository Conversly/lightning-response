package response

import (
	"encoding/json"
	"fmt"
	"strings"

	"context"

	"github.com/Conversly/lightning-response/internal/loaders"
	"github.com/Conversly/lightning-response/internal/utils"
	"github.com/cloudwego/eino/schema"
)

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
			messages = append(messages, schema.AssistantMessage(content, nil))
		}
	}

	return messages, nil
}

// ValidateChatbotAccess validates web_id + origin_url mapping using ApiKeyManager
// Returns the chatbot ID associated with the validated API key and domain
func ValidateChatbotAccess(ctx context.Context, db *loaders.PostgresClient, converslyWebID string, originURL string) (int, error) {
	// Ensure maps are loaded at least once
	if err := utils.GetApiKeyManager().LoadFromDatabase(ctx, db); err != nil {
		// Non-fatal if already loaded; continue validation even if reload fails
	}

	domain := extractHost(originURL)
	// For now, converslyWebID is used as API key token
	domainInfo, exists := utils.GetApiKeyManager().ValidateDomain(domain)
	if !exists || domainInfo.APIKey != converslyWebID {
		return 0, fmt.Errorf("invalid api key and origin mapping for domain=%s", domain)
	}
	return domainInfo.ChatbotID, nil
}

// MessageRecord represents a saved message
type MessageRecord struct {
	UniqueClientID string
	ChatbotID      int
	Message        string
	Role           string // user | assistant
	Citations      []string
}

// SaveConversationMessagesBackground is a placeholder that can be wired to DB later
func SaveConversationMessagesBackground(ctx context.Context, db *loaders.PostgresClient, records ...MessageRecord) error {
	// TODO: implement persistence when table is ready. For now, no-op.
	return nil
}
