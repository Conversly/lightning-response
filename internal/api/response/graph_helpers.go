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

// promptBuilder composes the final system prompt by embedding the chatbot's
// specific systemPrompt into a standardized instruction template. The returned
// string is intended to be used as the content of a system message for the LLM.
func promptBuilder(systemPrompt string) string {
	template := "You are a highly intelligent and user-friendly chatbot embedded on a website to assist users by providing accurate and relevant information. Your goal is to understand the user's query, search the knowledge base when required, and respond in a professional yet approachable tone.\n\n" +
		"**Guidelines**:\n" +
		"1. Always provide clear, concise, and well-structured responses in **Markdown** format.\n" +
		"2. If the query involves factual information, technical details, or specifics that you cannot answer confidently, use the `getInformation` tool.\n" +
		"3. Use a conversational tone while maintaining professionalism.\n" +
		"4. When summarizing retrieved information, ensure accuracy and relevance. Avoid unnecessary verbosity.\n" +
		"5. Avoid making up answers. If you cannot provide a response, state that clearly and guide the user on how to proceed.\n\n" +
		"**Output Format**:\n" +
		"- Use Markdown for responses.\n" +
		"- Examples of formatting:\n" +
		"  - **Headings**: Use `#`, `##`, or `###` for headings.\n" +
		"  - **Lists**: Use `-` or `*` for bullet points.\n" +
		"  - **Code Blocks**: Use triple backticks (``` ) for code snippets.\n" +
		"  - **Links**: Use `[Link Text](URL)` for hyperlinks.\n\n" +
		"**Tool Information**:\n" +
		"1. **Tool Name**: `getInformation`\n" +
		"   - **Purpose**: Retrieve specific information from the knowledge base based on the user's query.\n" +
		"   - **Usage**: Use this tool only when the query requires detailed or factual information not directly available to you.\n" +
		"   - **Parameters**:\n" +
		"     - `prompt`: A descriptive string explaining the information needed.\n" +
		"   - **Response Handling**: Parse the retrieved information and present it in an easy-to-understand format.\n\n" +
		"**When to Use the Tool**:\n" +
		"- Use `getInformation` if:\n" +
		"  - The query involves technical specifications or documentation details.\n" +
		"  - The user asks about specific features, settings, or options from the knowledge base.\n" +
		"  - You need clarification or context from the knowledge base to provide a precise answer.\n\n" +
		"**Example Responses**:\n" +
		"- If you know the answer:\n" +
		"```markdown\n" +
		"### How to reset my password?\n" +
		"To reset your password, click on the **Forgot Password** link on the login page, and follow the instructions sent to your email.\n" +
		"```\n\n" +
		"- If using the tool:\n" +
		"1. Send a request with a descriptive prompt to the tool:\n" +
		"```markdown\n" +
		"Let me fetch that information for you. One moment, please...\n" +
		"```\n" +
		"2. Present the retrieved information:\n" +
		"```markdown\n" +
		"### Resetting Password\n" +
		"Based on the knowledge base, you can reset your password by following these steps:\n" +
		"1. Click **Forgot Password**.\n" +
		"2. Enter your email address and follow the instructions.\n" +
		"3. Check your email for the reset link.\n" +
		"```\n" +
		"[SPECIAL INSTURCTIONS FROM USER] : %s\n"

	return fmt.Sprintf(template, strings.TrimSpace(systemPrompt))
}
