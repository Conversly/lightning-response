# API Key Manager

A fast, thread-safe in-memory manager for API keys and origin domain validation.

## Overview

The API Key Manager loads all API keys and their associated origin domains from the `origin_domains` PostgreSQL table into memory for fast validation. It provides:

- **Fast lookups**: O(1) average-case lookups for API keys and domains
- **Thread-safe**: Safe for concurrent access from multiple goroutines
- **Multiple lookup patterns**: Validate by API key, domain, or combined validation
- **Singleton pattern**: One global instance initialized at startup

## Database Schema

The manager loads data from the `origin_domains` table:

```sql
CREATE TABLE "origin_domains" (
	"id" serial PRIMARY KEY NOT NULL,
	"user_id" uuid NOT NULL,
	"chatbot_id" integer NOT NULL,
	"api_key" varchar(255) NOT NULL,
	"domain" varchar NOT NULL,
	"created_at" timestamp (6) DEFAULT now()
);
```

## Data Structure

The manager maintains two maps for efficient lookups:

1. **API Key Map**: `apiKey -> chatbotID -> []domains`
   - Lookup all domains for a specific API key and chatbot
   - Check if an API key is valid

2. **Domain Map**: `domain -> {apiKey, chatbotID, userID}`
   - Reverse lookup to validate a domain
   - Get the associated API key and chatbot ID

## Usage

### 1. Initialize at Startup

```go
package main

import (
    "context"
    "log"
    
    "your-app/internal/loaders"
    "your-app/internal/utils"
)

func main() {
    // Create PostgreSQL client
    pgClient, err := loaders.NewPostgresClient(dsn, workerCount, batchSize)
    if err != nil {
        log.Fatalf("Failed to create postgres client: %v", err)
    }
    defer pgClient.Close()
    
    // Load API keys into memory
    ctx := context.Background()
    manager := utils.GetApiKeyManager()
    if err := manager.LoadFromDatabase(ctx, pgClient.GetPool()); err != nil {
        log.Fatalf("Failed to load API keys: %v", err)
    }
    
    // Optional: Print debug info
    manager.DebugInfo()
    
    // Start your application...
}
```

### 2. Validate Requests

#### Combined Validation (Recommended)

```go
func handleRequest(apiKey string, chatbotID int, origin string) error {
    manager := utils.GetApiKeyManager()
    
    if !manager.ValidateApiKeyAndDomain(apiKey, chatbotID, origin) {
        return errors.New("invalid API key, chatbot ID, or origin domain")
    }
    
    // Request is valid, proceed...
    return nil
}
```

#### Separate Validation (For Detailed Errors)

```go
func validateRequest(apiKey string, chatbotID int, origin string) error {
    manager := utils.GetApiKeyManager()
    
    // Check if API key exists
    _, exists := manager.ValidateApiKey(apiKey)
    if !exists {
        return errors.New("invalid API key")
    }
    
    // Check domain and get its info
    domainInfo, exists := manager.ValidateDomain(origin)
    if !exists {
        return fmt.Errorf("domain %s is not registered", origin)
    }
    
    // Verify domain belongs to the correct chatbot
    if domainInfo.ChatbotID != chatbotID {
        return fmt.Errorf("domain belongs to chatbot %d, not %d", 
            domainInfo.ChatbotID, chatbotID)
    }
    
    // Verify API key matches
    if domainInfo.APIKey != apiKey {
        return errors.New("API key mismatch for domain")
    }
    
    log.Printf("Authorized request from user %s", domainInfo.UserID)
    return nil
}
```

### 3. Query Domain Information

```go
// Get all domains for a specific chatbot
func getChatbotDomains(apiKey string, chatbotID int) []string {
    manager := utils.GetApiKeyManager()
    
    domains, found := manager.GetDomainsForChatbot(apiKey, chatbotID)
    if !found {
        return nil
    }
    
    return domains
}

// Get all chatbot IDs for an API key
func getChatbots(apiKey string) []int {
    manager := utils.GetApiKeyManager()
    
    chatbotIDs, found := manager.GetChatbotsForApiKey(apiKey)
    if !found {
        return nil
    }
    
    return chatbotIDs
}
```

## API Reference

### GetApiKeyManager()

Returns the singleton instance of the API key manager.

```go
manager := utils.GetApiKeyManager()
```

### LoadFromDatabase(ctx, pool)

Loads all origin domains from the database into memory. Call this once during application startup.

```go
err := manager.LoadFromDatabase(ctx, pgPool)
```

### ValidateApiKeyAndDomain(apiKey, chatbotID, domain)

Validates that an API key, chatbot ID, and domain combination is valid. Returns `true` if all match.

```go
isValid := manager.ValidateApiKeyAndDomain("api_key_123", 42, "example.com")
```

### ValidateApiKey(apiKey)

Checks if an API key exists. Returns a map of chatbot IDs to domains and a boolean indicating existence.

```go
chatbots, exists := manager.ValidateApiKey("api_key_123")
```

### ValidateDomain(domain)

Checks if a domain is registered. Returns domain info (API key, chatbot ID, user ID) and existence status.

```go
info, exists := manager.ValidateDomain("example.com")
if exists {
    fmt.Printf("Domain belongs to chatbot %d, user %s\n", info.ChatbotID, info.UserID)
}
```

### GetDomainsForChatbot(apiKey, chatbotID)

Returns all domains for a specific API key and chatbot ID combination.

```go
domains, found := manager.GetDomainsForChatbot("api_key_123", 42)
```

### GetChatbotsForApiKey(apiKey)

Returns all chatbot IDs associated with an API key.

```go
chatbotIDs, found := manager.GetChatbotsForApiKey("api_key_123")
```

### GetTotalApiKeys()

Returns the total number of unique API keys loaded.

```go
count := manager.GetTotalApiKeys()
```

### GetTotalDomains()

Returns the total number of registered domains.

```go
count := manager.GetTotalDomains()
```

### DebugInfo()

Prints debugging information about loaded data. Useful during development.

```go
manager.DebugInfo()
```

## Middleware Example

Here's an example of using the manager in HTTP middleware:

```go
package middleware

import (
    "net/http"
    "strconv"
    
    "your-app/internal/utils"
)

func ApiKeyAuth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Extract API key from header
        apiKey := r.Header.Get("X-API-Key")
        if apiKey == "" {
            http.Error(w, "Missing API key", http.StatusUnauthorized)
            return
        }
        
        // Extract chatbot ID from query or path
        chatbotIDStr := r.URL.Query().Get("chatbot_id")
        chatbotID, err := strconv.Atoi(chatbotIDStr)
        if err != nil {
            http.Error(w, "Invalid chatbot ID", http.StatusBadRequest)
            return
        }
        
        // Get origin domain
        origin := r.Header.Get("Origin")
        if origin == "" {
            origin = r.Header.Get("Referer")
        }
        
        // Validate
        manager := utils.GetApiKeyManager()
        if !manager.ValidateApiKeyAndDomain(apiKey, chatbotID, origin) {
            http.Error(w, "Unauthorized", http.StatusForbidden)
            return
        }
        
        // Valid request, continue
        next.ServeHTTP(w, r)
    })
}
```

## Reloading Data

To reload the data from the database (e.g., after updates):

```go
manager := utils.GetApiKeyManager()
err := manager.LoadFromDatabase(ctx, pgPool)
if err != nil {
    log.Printf("Failed to reload API keys: %v", err)
}
```

The reload is atomic - the old data remains available until the new data is fully loaded.

## Performance

- **Lookup time**: O(1) average case for all operations
- **Memory usage**: Minimal - only stores API keys, chatbot IDs, domains, and user IDs
- **Thread safety**: Uses RWMutex for concurrent read access with minimal lock contention

## Best Practices

1. **Initialize once** at application startup
2. **Use combined validation** (`ValidateApiKeyAndDomain`) for most cases
3. **Handle reload** gracefully if you need to update data without restart
4. **Monitor memory** if you have millions of domains (though unlikely)
5. **Log validation failures** for security monitoring
