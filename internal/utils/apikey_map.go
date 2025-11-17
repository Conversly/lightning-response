package utils

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Conversly/lightning-response/internal/loaders"
)

type ApiKeyManager struct {
	mu        sync.RWMutex
	apiKeyMap map[string]map[string][]string
	domainMap map[string]DomainInfo

	// controls background refresh lifecycle
	refreshCancel context.CancelFunc
}

type DomainInfo struct {
	APIKey    string
	ChatbotID string
	UserID    string
}

var (
	apiKeyManager *ApiKeyManager
	once          sync.Once
)

// GetApiKeyManager returns the singleton manager, initializing it once
func GetApiKeyManager() *ApiKeyManager {
	once.Do(func() {
		apiKeyManager = &ApiKeyManager{
			apiKeyMap: make(map[string]map[string][]string),
			domainMap: make(map[string]DomainInfo),
		}
	})
	return apiKeyManager
}

func (akm *ApiKeyManager) LoadFromDatabase(ctx context.Context, pgClient *loaders.PostgresClient) error {
	// Use the postgres client to load origin domains
	records, err := pgClient.LoadOriginDomains(ctx)
	if err != nil {
		return fmt.Errorf("failed to load origin domains: %w", err)
	}

	// Build temporary maps
	tmpApiKeyMap := make(map[string]map[string][]string)
	tmpDomainMap := make(map[string]DomainInfo)
	loadedCount := 0

	for _, record := range records {
		// Populate apiKey -> chatbotID -> []domains map
		if tmpApiKeyMap[record.APIKey] == nil {
			tmpApiKeyMap[record.APIKey] = make(map[string][]string)
		}
		tmpApiKeyMap[record.APIKey][record.ChatbotID] = append(tmpApiKeyMap[record.APIKey][record.ChatbotID], record.Domain)

		// Populate domain -> DomainInfo reverse lookup
		tmpDomainMap[record.Domain] = DomainInfo{
			APIKey:    record.APIKey,
			ChatbotID: record.ChatbotID,
			UserID:    record.UserID,
		}

		// Zlog.Infof("Loaded origin domain: APIKey=%s, ChatbotID=%d, Domain=%s", record.APIKey, record.ChatbotID, record.Domain)

		loadedCount++
	}

	// Atomically replace the maps
	akm.mu.Lock()
	akm.apiKeyMap = tmpApiKeyMap
	akm.domainMap = tmpDomainMap
	akm.mu.Unlock()

	log.Printf("Loaded %d origin domains into memory", loadedCount)
	return nil
}

// ValidateApiKey checks if an API key exists and returns associated chatbot IDs
func (akm *ApiKeyManager) ValidateApiKey(apiKey string) (map[string][]string, bool) {
	akm.mu.RLock()
	defer akm.mu.RUnlock()

	chatbots, exists := akm.apiKeyMap[apiKey]
	return chatbots, exists
}

// ValidateDomain checks if a domain is registered and returns its info
func (akm *ApiKeyManager) ValidateDomain(domain string) (DomainInfo, bool) {
	akm.mu.RLock()
	defer akm.mu.RUnlock()

	info, exists := akm.domainMap[domain]
	return info, exists
}

// ValidateApiKeyAndDomain performs a combined check for API key and domain
func (akm *ApiKeyManager) ValidateApiKeyAndDomain(apiKey string, domain string) bool {
	akm.mu.RLock()
	defer akm.mu.RUnlock()

	// Check if domain exists and matches the API key
	if info, exists := akm.domainMap[domain]; exists {
		return info.APIKey == apiKey
	}

	// Allow localhost if the API key is valid
	if domain == "localhost" || strings.HasPrefix(domain, "localhost:") {
		_, exists := akm.apiKeyMap[apiKey]
		return exists
	}

	return false
}

func (akm *ApiKeyManager) GetChatbotsForApiKey(apiKey string) ([]string, bool) {
	akm.mu.RLock()
	defer akm.mu.RUnlock()

	chatbots, exists := akm.apiKeyMap[apiKey]
	if !exists {
		return nil, false
	}

	chatbotIDs := make([]string, 0, len(chatbots))
	for id := range chatbots {
		chatbotIDs = append(chatbotIDs, id)
	}

	return chatbotIDs, true
}

// GetTotalApiKeys returns the total number of unique API keys
func (akm *ApiKeyManager) GetTotalApiKeys() int {
	akm.mu.RLock()
	defer akm.mu.RUnlock()

	return len(akm.apiKeyMap)
}

// GetTotalDomains returns the total number of registered domains
func (akm *ApiKeyManager) GetTotalDomains() int {
	akm.mu.RLock()
	defer akm.mu.RUnlock()

	return len(akm.domainMap)
}

// DebugInfo prints debugging information about the loaded data
func (akm *ApiKeyManager) DebugInfo() {
	akm.mu.RLock()
	defer akm.mu.RUnlock()

	log.Printf("API Key Manager Stats:")
	log.Printf("  Total API Keys: %d", len(akm.apiKeyMap))
	log.Printf("  Total Domains: %d", len(akm.domainMap))

	// Print first few entries for debugging
	count := 0
	for apiKey, chatbots := range akm.apiKeyMap {
		if count >= 3 {
			break
		}
		maskedKey := apiKey
		if len(apiKey) > 8 {
			maskedKey = apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
		}
		log.Printf("  API Key %s: %d chatbots", maskedKey, len(chatbots))
		for chatbotID, domains := range chatbots {
			log.Printf("    Chatbot %s: %d domains", chatbotID, len(domains))
		}
		count++
	}
}

// StartAutoRefresh starts a background goroutine that reloads API keys/domains
// from the database at the specified interval. It performs an immediate load
// and then continues refreshing until StopAutoRefresh is called or the context
// is cancelled. Safe to call multiple times; subsequent calls are no-ops while
// a refresher is already running.
func (akm *ApiKeyManager) StartAutoRefresh(ctx context.Context, pgClient *loaders.PostgresClient, interval time.Duration) {
	akm.mu.Lock()
	if akm.refreshCancel != nil {
		// already running
		akm.mu.Unlock()
		return
	}
	refreshCtx, cancel := context.WithCancel(ctx)
	akm.refreshCancel = cancel
	akm.mu.Unlock()

	go func() {
		// initial load with timeout
		func() {
			loadCtx, loadCancel := context.WithTimeout(refreshCtx, 15*time.Second)
			defer loadCancel()
			if err := akm.LoadFromDatabase(loadCtx, pgClient); err != nil {
				log.Printf("ApiKeyManager initial load failed: %v", err)
			}
		}()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-refreshCtx.Done():
				return
			case <-ticker.C:
				loadCtx, loadCancel := context.WithTimeout(refreshCtx, 15*time.Second)
				if err := akm.LoadFromDatabase(loadCtx, pgClient); err != nil {
					log.Printf("ApiKeyManager periodic refresh failed: %v", err)
				}
				loadCancel()
			}
		}
	}()
}

// StopAutoRefresh stops the background refresh goroutine if running.
func (akm *ApiKeyManager) StopAutoRefresh() {
	akm.mu.Lock()
	if akm.refreshCancel != nil {
		akm.refreshCancel()
		akm.refreshCancel = nil
	}
	akm.mu.Unlock()
}
