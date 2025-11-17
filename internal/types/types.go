package types

import "time"

type ChatbotTopic struct {
	ID        string
	Name      string
	Color     string
	CreatedAt time.Time
}

type ChatbotInfo struct {
	ID           string
	Name         string
	Description  string
	SystemPrompt string
	Topics       []ChatbotTopic
}
