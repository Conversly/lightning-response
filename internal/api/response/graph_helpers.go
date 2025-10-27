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

func extractHost(urlStr string) string {
	if idx := strings.Index(urlStr, "://"); idx != -1 {
		urlStr = urlStr[idx+3:]
	}
	if idx := strings.Index(urlStr, "/"); idx != -1 {
		urlStr = urlStr[:idx]
	}
	return urlStr
}

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

func ValidateChatbotAccess(ctx context.Context, db *loaders.PostgresClient, converslyWebID string, originURL string) (int, error) {
	if err := utils.GetApiKeyManager().LoadFromDatabase(ctx, db); err != nil {

	}
	domain := extractHost(originURL)
	domainInfo, exists := utils.GetApiKeyManager().ValidateDomain(domain)
	if !exists || domainInfo.APIKey != converslyWebID {
		return 0, fmt.Errorf("invalid api key and origin mapping for domain=%s", domain)
	}
	return domainInfo.ChatbotID, nil
}

// ExtractLastUserContent returns the content of the last user turn from the raw conversation JSON.
func ExtractLastUserContent(queryJSON string) string {
	var rawMessages []map[string]interface{}
	if err := json.Unmarshal([]byte(queryJSON), &rawMessages); err != nil {
		return ""
	}
	for i := len(rawMessages) - 1; i >= 0; i-- {
		role, _ := rawMessages[i]["role"].(string)
		if role == "user" {
			content, _ := rawMessages[i]["content"].(string)
			return content
		}
	}
	return ""
}
