package loaders

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	pgxvec "github.com/pgvector/pgvector-go/pgx"

	"github.com/Conversly/lightning-response/internal/types"
)

type PostgresClient struct {
	dsn  string
	pool *pgxpool.Pool
}

// OriginDomainRecord represents a single row from the origin_domains table
type OriginDomainRecord struct {
	ID        string
	UserID    string
	ChatbotID string
	APIKey    string
	Domain    string
}

// ChatbotRecord represents a chatbot row in the database.
type ChatbotRecord struct {
	ID           string
	Name         string
	Description  string
	SystemPrompt string
}

// EmbeddingResult represents a retrieved embedding document
type EmbeddingResult struct {
	Text     string
	Citation *string
}

type EmbeddingData struct {
	Topic        string
	Text         string
	Vector       []float64
	DataSourceID *int
	Citation     *string
}

func NewPostgresClient(dsn string, workerCount, batchSize int) (*PostgresClient, error) {
	client := &PostgresClient{
		dsn: dsn,
	}

	pool, err := client.createConnectionPool(workerCount, batchSize)
	if err != nil {
		return nil, err
	}

	client.pool = pool
	log.Println("Successfully connected to PostgreSQL database with connection pool")
	return client, nil
}

func (c *PostgresClient) createConnectionPool(workerCount, batchSize int) (*pgxpool.Pool, error) {
	log.Println("Parsing Postgres DSN")
	cfg, err := pgxpool.ParseConfig(c.dsn)
	if err != nil {
		log.Printf("Failed to parse Postgres DSN: %v", err)
		return nil, fmt.Errorf("failed to parse Postgres DSN: %w", err)
	}

	cfg.MaxConns = int32(workerCount) + 2
	cfg.MinConns = 1
	cfg.HealthCheckPeriod = 30 * time.Second
	cfg.MaxConnLifetime = 60 * time.Minute
	cfg.MaxConnIdleTime = 15 * time.Minute

	log.Printf("Creating Postgres connection pool with MaxConns=%d", cfg.MaxConns)
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		log.Printf("Failed to create pgx pool: %v", err)
		return nil, fmt.Errorf("failed to create pgx pool: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	log.Println("Pinging Postgres to check connectivity")
	if err := pool.Ping(ctx); err != nil {
		log.Printf("Failed to ping Postgres: %v", err)
		pool.Close()
		return nil, fmt.Errorf("failed to ping Postgres: %w", err)
	}

	// Enable pgvector extension
	log.Println("Enabling pgvector extension")
	_, err = pool.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	if err != nil {
		log.Printf("Warning: Failed to enable pgvector extension: %v", err)
		// Don't fail here as the extension might already be enabled or user may lack permissions
	}

	// Register pgvector types with the connection pool
	log.Println("Registering pgvector types")
	conn, err := pool.Acquire(ctx)
	if err != nil {
		log.Printf("Failed to acquire connection for type registration: %v", err)
		pool.Close()
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	err = pgxvec.RegisterTypes(ctx, conn.Conn())
	if err != nil {
		log.Printf("Failed to register pgvector types: %v", err)
		pool.Close()
		return nil, fmt.Errorf("failed to register pgvector types: %w", err)
	}

	log.Println("Postgres connection pool established successfully")
	return pool, nil
}

func (c *PostgresClient) Close() error {
	if c.pool != nil {
		c.pool.Close()
	}
	return nil
}

func (c *PostgresClient) GetPool() *pgxpool.Pool {
	return c.pool
}

func formatTimeForDB(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000000")
}

type MessageRow struct {
	UniqueMsgID     string
	ChatbotID       string
	Citations       []string
	Type            string
	Content         string
	CreatedAt       time.Time
	UniqueConvID    string
	TopicID         string
	Channel         string                 // WIDGET | WHATSAPP | VOICE
	ChannelMetadata map[string]interface{} // Optional metadata (whatsapp message ID, etc)
}

// BatchInsertMessages inserts a batch of messages into the messages table
func (c *PostgresClient) BatchInsertMessages(ctx context.Context, rows []MessageRow) error {
	if len(rows) == 0 {
		return nil
	}

	query := `
        INSERT INTO messages (
            id, chatbot_id, citations, "type", content, created_at, unique_conv_id, topic_id, channel, channel_message_metadata
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    `

	successCount := 0
	for _, r := range rows {
		// Convert empty topic_id to NULL for database insertion
		var topicID interface{}
		if r.TopicID == "" {
			topicID = nil
		} else {
			topicID = r.TopicID
		}

		// Default to WIDGET if channel not specified
		channel := r.Channel
		if channel == "" {
			channel = "WIDGET"
		}

		// Marshal channel metadata to JSON
		var metadataJSON interface{}
		if r.ChannelMetadata != nil {
			jsonBytes, err := json.Marshal(r.ChannelMetadata)
			if err != nil {
				log.Printf("Failed to marshal channel metadata: %v", err)
				metadataJSON = nil
			} else {
				metadataJSON = jsonBytes
			}
		}

		_, err := c.pool.Exec(ctx, query,
			r.UniqueMsgID,
			r.ChatbotID,
			r.Citations,
			r.Type,
			r.Content,
			r.CreatedAt.UTC(),
			r.UniqueConvID,
			topicID,
			channel,
			metadataJSON,
		)
		if err != nil {
			log.Printf("Failed to insert message for conv=%s chatbot_id=%s: %v", r.UniqueConvID, r.ChatbotID, err)
			continue
		}
		successCount++
	}

	if successCount == 0 {
		return fmt.Errorf("failed to insert any messages")
	}

	return nil
}

func (c *PostgresClient) UpdateMessageFeedback(ctx context.Context, chatbotID string, uniqueMsgID string, feedback int16, comment *string) error {
	if uniqueMsgID == "" {
		return fmt.Errorf("unique message id is required")
	}

	query := `
        UPDATE messages
        SET feedback = $1, feedback_comment = $2
        WHERE id = $3 AND chatbot_id = $4
    `

	_, err := c.pool.Exec(ctx, query, feedback, comment, uniqueMsgID, chatbotID)
	if err != nil {
		return fmt.Errorf("failed to update message feedback: %w", err)
	}
	return nil
}

// BatchInsertEmbeddings inserts a batch of embeddings into the database
func (c *PostgresClient) BatchInsertEmbeddings(ctx context.Context, userID, chatbotID string, chunks []EmbeddingData) error {
	if len(chunks) == 0 {
		return nil
	}

	query := `
		INSERT INTO embeddings (
			user_id, chatbot_id, text, vector, 
			created_at, updated_at, data_source_id, citation
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	now := formatTimeForDB(time.Now().UTC())
	successCount := 0

	for _, chunk := range chunks {
		// Convert []float64 to []float32 for pgvector
		vec32 := make([]float32, len(chunk.Vector))
		for i, v := range chunk.Vector {
			vec32[i] = float32(v)
		}
		vec := pgvector.NewVector(vec32)

		_, err := c.pool.Exec(ctx, query,
			userID,
			chatbotID,
			chunk.Text,
			vec,
			now,
			now,
			chunk.DataSourceID,
			chunk.Citation,
		)
		if err != nil {
			log.Printf("Failed to insert embedding for data_source_id=%d: %v", chunk.DataSourceID, err)
			continue
		}
		successCount++
	}

	if successCount == 0 {
		return fmt.Errorf("failed to insert any embeddings")
	}

	log.Printf("Successfully inserted %d/%d embeddings", successCount, len(chunks))
	return nil
}

// UpdateDataSourceStatus updates the status of data sources to COMPLETED
func (c *PostgresClient) UpdateDataSourceStatus(ctx context.Context, dataSourceIDs []int, status string) error {
	if len(dataSourceIDs) == 0 {
		return nil
	}

	query := `
		UPDATE data_source 
		SET status = $1, updated_at = $2
		WHERE id = ANY($3)
	`

	now := formatTimeForDB(time.Now().UTC())
	result, err := c.pool.Exec(ctx, query, status, now, dataSourceIDs)
	if err != nil {
		return fmt.Errorf("failed to update data source status: %w", err)
	}

	rowsAffected := result.RowsAffected()
	log.Printf("Updated status to '%s' for %d data sources", status, rowsAffected)
	return nil
}

// LoadOriginDomains queries all origin domains from the database
func (c *PostgresClient) LoadOriginDomains(ctx context.Context) ([]OriginDomainRecord, error) {
	query := `
		SELECT id, user_id, chatbot_id, api_key, domain
		FROM origin_domains
		ORDER BY api_key, chatbot_id
	`

	rows, err := c.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query origin_domains: %w", err)
	}
	defer rows.Close()

	var records []OriginDomainRecord
	for rows.Next() {
		var record OriginDomainRecord
		if err := rows.Scan(&record.ID, &record.UserID, &record.ChatbotID, &record.APIKey, &record.Domain); err != nil {
			log.Printf("Failed to scan origin_domain row: %v", err)
			continue
		}
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating origin_domains: %w", err)
	}

	log.Printf("Loaded %d origin domain records from database", len(records))
	return records, nil
}

func (c *PostgresClient) GetChatbotInfoWithTopics(ctx context.Context, chatbotID string) (*types.ChatbotInfo, error) {
	query := `
        SELECT 
            c.id AS chatbot_id,
            c.name,
            c.description,
            c.system_prompt,
            t.id AS topic_id,
            t.name AS topic_name,
            t.color AS topic_color,
            t.created_at AS topic_created_at
        FROM chatbot c
        LEFT JOIN chatbot_topics t 
            ON c.id = t.chatbot_id
        WHERE c.id = $1
    `

	rows, err := c.pool.Query(ctx, query, chatbotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var info *types.ChatbotInfo
	topics := []types.ChatbotTopic{}

	for rows.Next() {
		var (
			cID          string
			name         string
			description  string
			systemPrompt string
			topicID      *string
			topicName    *string
			topicColor   *string
			topicCreated *time.Time
		)

		if err := rows.Scan(
			&cID,
			&name,
			&description,
			&systemPrompt,
			&topicID,
			&topicName,
			&topicColor,
			&topicCreated,
		); err != nil {
			return nil, err
		}

		// Initialize info only once
		if info == nil {
			info = &types.ChatbotInfo{
				ID:           cID,
				Name:         name,
				Description:  description,
				SystemPrompt: systemPrompt,
				Topics:       []types.ChatbotTopic{},
			}
		}

		// If topic exists (LEFT JOIN may give NULL)
		if topicID != nil {
			topics = append(topics, types.ChatbotTopic{
				ID:        *topicID,
				Name:      *topicName,
				Color:     *topicColor,
				CreatedAt: *topicCreated,
			})
		}
	}

	if info == nil {
		return nil, fmt.Errorf("chatbot not found")
	}

	info.Topics = topics

	// Load enabled custom actions for this chatbot
	customActions, err := c.GetCustomActionsByChatbot(ctx, chatbotID)
	if err != nil {
		// Log error but don't fail the request - chatbot can work without custom actions
		log.Printf("Warning: Failed to load custom actions for chatbot %s: %v", chatbotID, err)
		info.CustomActions = []types.CustomAction{}
	} else {
		info.CustomActions = customActions
	}

	return info, nil
}

// SearchEmbeddings searches for similar embeddings using vector similarity
func (c *PostgresClient) SearchEmbeddings(ctx context.Context, chatbotID string, queryVector []float64, topK int) ([]EmbeddingResult, error) {
	// Convert queryVector from []float64 to []float32 for pgvector
	vec32 := make([]float32, len(queryVector))
	for i, v := range queryVector {
		vec32[i] = float32(v)
	}
	vec := pgvector.NewVector(vec32)

	log.Printf("Searching embeddings for chatbot_id=%s with topK=%d and vector_dim=%d", chatbotID, topK, len(queryVector))

	// Use cosine distance operator for better semantic search
	// <=> is cosine distance, <-> is L2 distance, <#> is inner product
	query := `
        SELECT text, citation
        FROM embeddings 
        WHERE chatbot_id = $1
        ORDER BY vector <=> $2
        LIMIT $3
    `

	rows, err := c.pool.Query(ctx, query, chatbotID, vec, topK)
	if err != nil {
		return nil, fmt.Errorf("failed to query embeddings: %w", err)
	}
	defer rows.Close()

	var results []EmbeddingResult
	for rows.Next() {
		var result EmbeddingResult
		if err := rows.Scan(&result.Text, &result.Citation); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	log.Printf("Retrieved %d embeddings for chatbot_id=%s", len(results), chatbotID)
	return results, nil
}

// GetCustomActionsByChatbot retrieves all enabled custom actions for a chatbot
func (c *PostgresClient) GetCustomActionsByChatbot(ctx context.Context, chatbotID string) ([]types.CustomAction, error) {
	query := `
		SELECT 
			id,
			chatbot_id,
			name,
			display_name,
			description,
			is_enabled,
			api_config,
			tool_schema,
			version,
			created_at,
			updated_at,
			created_by,
			last_tested_at,
			test_status,
			test_result
		FROM custom_actions
		WHERE chatbot_id = $1 AND is_enabled = true
		ORDER BY name
	`

	rows, err := c.pool.Query(ctx, query, chatbotID)
	if err != nil {
		return nil, fmt.Errorf("failed to query custom actions: %w", err)
	}
	defer rows.Close()

	var actions []types.CustomAction
	for rows.Next() {
		var action types.CustomAction
		var apiConfigJSON []byte
		var toolSchemaJSON []byte
		var testResultJSON []byte

		if err := rows.Scan(
			&action.ID,
			&action.ChatbotID,
			&action.Name,
			&action.DisplayName,
			&action.Description,
			&action.IsEnabled,
			&apiConfigJSON,
			&toolSchemaJSON,
			&action.Version,
			&action.CreatedAt,
			&action.UpdatedAt,
			&action.CreatedBy,
			&action.LastTestedAt,
			&action.TestStatus,
			&testResultJSON,
		); err != nil {
			return nil, fmt.Errorf("failed to scan custom action row: %w", err)
		}

		// Parse api_config JSON
		if err := json.Unmarshal(apiConfigJSON, &action.APIConfig); err != nil {
			return nil, fmt.Errorf("failed to parse api_config for action %s: %w", action.Name, err)
		}

		// Parse tool_schema JSON
		if err := json.Unmarshal(toolSchemaJSON, &action.ToolSchema); err != nil {
			return nil, fmt.Errorf("failed to parse tool_schema for action %s: %w", action.Name, err)
		}

		// Parse test_result JSON if present
		if len(testResultJSON) > 0 && string(testResultJSON) != "null" {
			if err := json.Unmarshal(testResultJSON, &action.TestResult); err != nil {
				log.Printf("Warning: failed to parse test_result for action %s: %v", action.Name, err)
			}
		}

		actions = append(actions, action)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating custom actions: %w", err)
	}

	return actions, nil
}

// WhatsAppAccount represents a row from whatsapp_accounts
type WhatsAppAccount struct {
	ID                 string
	ChatbotID          string
	PhoneNumber        string
	WabaID             string
	PhoneNumberID      string
	AccessToken        string
	VerifiedName       string
	Status             string
	WhatsappBusinessID string
	VerifyToken        *string
}

// GetWhatsAppAccountByPhoneNumberID retrieves the WhatsApp account for a phone number ID
func (c *PostgresClient) GetWhatsAppAccountByPhoneNumberID(ctx context.Context, phoneNumberID string) (*WhatsAppAccount, error) {
	query := `
		SELECT id, chatbot_id, phone_number, waba_id, phone_number_id, 
		       access_token, verified_name, status, whatsapp_business_id, verify_token
		FROM whatsapp_accounts 
		WHERE phone_number_id = $1
		LIMIT 1
	`

	var acc WhatsAppAccount
	err := c.pool.QueryRow(ctx, query, phoneNumberID).Scan(
		&acc.ID, &acc.ChatbotID, &acc.PhoneNumber, &acc.WabaID, &acc.PhoneNumberID,
		&acc.AccessToken, &acc.VerifiedName, &acc.Status, &acc.WhatsappBusinessID, &acc.VerifyToken,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get WhatsApp account: %w", err)
	}

	return &acc, nil
}

// GetWhatsAppAccessToken retrieves the access token for a WhatsApp phone number
func (c *PostgresClient) GetWhatsAppAccessToken(ctx context.Context, phoneNumberID string) (string, error) {
	acc, err := c.GetWhatsAppAccountByPhoneNumberID(ctx, phoneNumberID)
	if err != nil {
		return "", err
	}
	return acc.AccessToken, nil
}

// WhatsAppContactMetadata represents the user metadata stored in whatsapp_contacts
type WhatsAppContactMetadata struct {
	WaID                 string  `json:"wa_id"`
	ProfileName          string  `json:"profile_name,omitempty"`
	FirstSeenAt          string  `json:"first_seen_at"`
	LastSeenAt           string  `json:"last_seen_at"`
	LastInboundMessageID string  `json:"last_inbound_message_id,omitempty"`
	WabaID               string  `json:"waba_id,omitempty"`
	PhoneNumberID        string  `json:"phone_number_id,omitempty"`
	DisplayPhoneNumber   string  `json:"display_phone_number,omitempty"`
	Source               string  `json:"source,omitempty"`
	OptInStatus          *string `json:"opt_in_status,omitempty"`
}

// UpsertWhatsAppContact creates or updates a WhatsApp contact
func (c *PostgresClient) UpsertWhatsAppContact(ctx context.Context, chatbotID, phoneNumber, displayName string, metadata *WhatsAppContactMetadata) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO whatsapp_contacts (id, chatbot_id, phone_number, display_name, whatsapp_user_metadata, created_at, updated_at)
		VALUES (gen_random_uuid()::text, $1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (chatbot_id, phone_number) 
		DO UPDATE SET 
			display_name = COALESCE(EXCLUDED.display_name, whatsapp_contacts.display_name),
			whatsapp_user_metadata = $4,
			updated_at = NOW()
	`

	_, err = c.pool.Exec(ctx, query, chatbotID, phoneNumber, displayName, metadataJSON)
	if err != nil {
		return fmt.Errorf("failed to upsert WhatsApp contact: %w", err)
	}

	return nil
}

// GetWhatsAppConversationHistory retrieves recent messages for a WhatsApp conversation
func (c *PostgresClient) GetWhatsAppConversationHistory(ctx context.Context, uniqueConvID, chatbotID string, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT type, content, created_at
		FROM messages
		WHERE unique_conv_id = $1 AND chatbot_id = $2 AND channel = 'WHATSAPP'
		ORDER BY created_at DESC
		LIMIT $3
	`

	rows, err := c.pool.Query(ctx, query, uniqueConvID, chatbotID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}
	defer rows.Close()

	var messages []map[string]interface{}
	for rows.Next() {
		var msgType, content string
		var createdAt time.Time
		if err := rows.Scan(&msgType, &content, &createdAt); err != nil {
			continue
		}
		messages = append(messages, map[string]interface{}{
			"role":       msgType,
			"content":    content,
			"created_at": createdAt,
		})
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}
