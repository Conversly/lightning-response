package widget

import (
	"context"
	"fmt"
	"strings"

	"github.com/Conversly/lightning-response/internal/utils"
)

// extractHost extracts the host from a URL
func extractHost(urlStr string) string {
	if idx := strings.Index(urlStr, "://"); idx != -1 {
		urlStr = urlStr[idx+3:]
	}
	if idx := strings.Index(urlStr, "/"); idx != -1 {
		urlStr = urlStr[:idx]
	}
	return urlStr
}

// ValidateChatbotAccess validates the API key and origin domain
func ValidateChatbotAccess(ctx context.Context, converslyWebID string, originURL string) (string, error) {
	domain := extractHost(originURL)

	// Special handling for localhost
	if domain == "localhost" || strings.HasPrefix(domain, "localhost:") {
		// For localhost, just validate the API key exists
		chatbots, exists := utils.GetApiKeyManager().ValidateApiKey(converslyWebID)
		if !exists {
			return "", fmt.Errorf("invalid api key for localhost domain")
		}
		// Return the first chatbot ID associated with this API key
		for chatbotID := range chatbots {
			return chatbotID, nil
		}
		return "", fmt.Errorf("no chatbot found for api key")
	}

	domainInfo, exists := utils.GetApiKeyManager().ValidateDomain(domain)
	if !exists || domainInfo.APIKey != converslyWebID {
		return "", fmt.Errorf("invalid api key and origin mapping for domain=%s", domain)
	}
	return domainInfo.ChatbotID, nil
}

