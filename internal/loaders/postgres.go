package loaders

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
)



type PostgresClient struct {
	dsn  string
	pool *pgxpool.Pool
}

// OriginDomainRecord represents a single row from the origin_domains table
type OriginDomainRecord struct {
	ID        int
	UserID    string
	ChatbotID int
	APIKey    string
	Domain    string
}

// ChatbotInfo represents chatbot information
type ChatbotInfo struct {
	ID           int
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
	pool, err := pgxpool.ConnectConfig(context.Background(), cfg)
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


// BatchInsertEmbeddings inserts a batch of embeddings into the database
func (c *PostgresClient) BatchInsertEmbeddings(ctx context.Context, userID, chatbotID string, chunks []EmbeddingData) error {
	if len(chunks) == 0 {
		return nil
	}

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		INSERT INTO embeddings (
			user_id, chatbot_id, text, vector, 
			created_at, updated_at, data_source_id, citation
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	now := formatTimeForDB(time.Now().UTC())
	successCount := 0

	for _, chunk := range chunks {
		_, err := tx.Exec(ctx, query,
			userID,
			chatbotID,
			chunk.Text,
			chunk.Vector,
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

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully inserted %d/%d embeddings", successCount, len(chunks))
	return nil
}

// UpdateDataSourceStatus updates the status of data sources to COMPLETED
func (c *PostgresClient) UpdateDataSourceStatus(ctx context.Context, dataSourceIDs []int, status string) error {
	if len(dataSourceIDs) == 0 {
		return nil
	}

	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	query := `
		UPDATE data_source 
		SET status = $1, updated_at = $2
		WHERE id = ANY($3)
	`

	now := formatTimeForDB(time.Now().UTC())
	result, err := tx.Exec(ctx, query, status, now, dataSourceIDs)
	if err != nil {
		return fmt.Errorf("failed to update data source status: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
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

// GetChatbotInfo retrieves chatbot information by chatbot ID
func (c *PostgresClient) GetChatbotInfo(ctx context.Context, chatbotID int) (*ChatbotInfo, error) {
	query := `
		SELECT id, name, description, system_prompt
		FROM chatbot
		WHERE id = $1
	`

	var info ChatbotInfo
	err := c.pool.QueryRow(ctx, query, chatbotID).Scan(
		&info.ID,
		&info.Name,
		&info.Description,
		&info.SystemPrompt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get chatbot info for id=%d: %w", chatbotID, err)
	}

	log.Printf("Retrieved chatbot info for id=%d, name=%s", info.ID, info.Name)
	return &info, nil
}

// SearchEmbeddings searches for similar embeddings using vector similarity
func (c *PostgresClient) SearchEmbeddings(ctx context.Context, chatbotID string, queryVector []float64, topK int) ([]EmbeddingResult, error) {
	// Format embedding as vector string for PostgreSQL
	vectorStr := "["
	for i, val := range queryVector {
		if i > 0 {
			vectorStr += ","
		}
		vectorStr += fmt.Sprintf("%f", val)
	}
	vectorStr += "]"

	query := `
		SELECT text, citation
		FROM embeddings 
		WHERE chatbot_id = $1
		ORDER BY vector <=> $2::vector
		LIMIT $3
	`

	rows, err := c.pool.Query(ctx, query, chatbotID, vectorStr, topK)
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
