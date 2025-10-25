package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const (
	DefaultDownloadTimeout = 5 * time.Minute
	MaxDownloadSize        = 100 * 1024 * 1024 // 100MB
)

// DownloadedFile represents a downloaded file with its metadata
type DownloadedFile struct {
	Content     []byte
	ContentType string
	Filename    string
	Size        int64
}

// FileDownloader handles downloading files from URLs
type FileDownloader struct {
	client  *http.Client
	timeout time.Duration
	maxSize int64
}

// NewFileDownloader creates a new file downloader with default settings
func NewFileDownloader() *FileDownloader {
	return &FileDownloader{
		client: &http.Client{
			Timeout: DefaultDownloadTimeout,
		},
		timeout: DefaultDownloadTimeout,
		maxSize: MaxDownloadSize,
	}
}

// DownloadFile downloads a file from the given URL
func (d *FileDownloader) DownloadFile(ctx context.Context, url string, expectedContentType string) (*DownloadedFile, error) {
	Zlog.Info("Starting file download",
		zap.String("url", url),
		zap.String("expectedContentType", expectedContentType))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download file: status code %d", resp.StatusCode)
	}

	// Validate content type if provided
	actualContentType := resp.Header.Get("Content-Type")
	if expectedContentType != "" && actualContentType != expectedContentType {
		Zlog.Warn("Content type mismatch",
			zap.String("expected", expectedContentType),
			zap.String("actual", actualContentType))
	}

	// Check content length
	if resp.ContentLength > d.maxSize {
		return nil, fmt.Errorf("file size exceeds maximum allowed size: %d bytes", d.maxSize)
	}

	// Read the file content with size limit
	limitedReader := io.LimitReader(resp.Body, d.maxSize)
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}

	Zlog.Info("File downloaded successfully",
		zap.String("url", url),
		zap.Int("size", len(content)))

	return &DownloadedFile{
		Content:     content,
		ContentType: actualContentType,
		Filename:    extractFilenameFromURL(url),
		Size:        int64(len(content)),
	}, nil
}

// extractFilenameFromURL extracts filename from URL (simple implementation)
func extractFilenameFromURL(url string) string {
	// Simple extraction - can be improved
	return "downloaded_file"
}
