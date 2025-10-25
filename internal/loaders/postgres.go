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


func formatTimeForDB(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05.000000")
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

// EmbeddingData represents the data needed to insert an embedding
type EmbeddingData struct {
	Topic        string
	Text         string
	Vector       []float64
	DataSourceID *int
	Citation     *string
}