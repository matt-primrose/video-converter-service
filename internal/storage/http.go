package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
)

// HTTPStorage implements the Storage interface for HTTP/HTTPS downloads
// This is primarily used for downloading files from HTTP sources
type HTTPStorage struct {
	config StorageConfig
	client *http.Client
}

// NewHTTPStorage creates a new HTTP storage instance
func NewHTTPStorage(config StorageConfig) *HTTPStorage {
	return &HTTPStorage{
		config: config,
		client: &http.Client{}, // Use default HTTP client
	}
}

// DownloadFile downloads a file from HTTP/HTTPS URL
func (hs *HTTPStorage) DownloadFile(ctx context.Context, sourceURI string, jobID string) (string, error) {
	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", sourceURI, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set user agent
	req.Header.Set("User-Agent", "video-converter-service/1.0")

	// Make request
	resp, err := hs.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP request failed with status: %d %s", resp.StatusCode, resp.Status)
	}

	// Create temp directory for this job
	tempDir := filepath.Join(hs.config.TempDir, jobID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Determine file extension from URL or Content-Type
	ext := hs.getFileExtension(sourceURI, resp.Header.Get("Content-Type"))
	tempFile := filepath.Join(tempDir, "source"+ext)

	// Create output file
	outFile, err := os.Create(tempFile)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer outFile.Close()

	// Copy data
	bytesWritten, err := io.Copy(outFile, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	slog.Info("Successfully downloaded HTTP file",
		"jobId", jobID,
		"sourceUrl", sourceURI,
		"tempPath", tempFile,
		"size", bytesWritten,
		"contentType", resp.Header.Get("Content-Type"),
	)

	return tempFile, nil
}

// UploadFile is not supported for HTTP storage
func (hs *HTTPStorage) UploadFile(ctx context.Context, sourcePath string, destinationPath string) error {
	return fmt.Errorf("upload not supported for HTTP storage")
}

// UploadFiles is not supported for HTTP storage
func (hs *HTTPStorage) UploadFiles(ctx context.Context, fileMap map[string]string) error {
	return fmt.Errorf("upload not supported for HTTP storage")
}

// GetFileURL returns the original HTTP URL
func (hs *HTTPStorage) GetFileURL(destinationPath string) (string, error) {
	// For HTTP storage, the destination path is typically the original URL
	return destinationPath, nil
}

// DeleteFile is not supported for HTTP storage
func (hs *HTTPStorage) DeleteFile(ctx context.Context, destinationPath string) error {
	return fmt.Errorf("delete not supported for HTTP storage")
}

// ListFiles is not supported for HTTP storage
func (hs *HTTPStorage) ListFiles(ctx context.Context, prefix string) ([]string, error) {
	return nil, fmt.Errorf("list files not supported for HTTP storage")
}

// GetType returns the storage type
func (hs *HTTPStorage) GetType() string {
	return "http"
}

// getFileExtension determines the file extension from URL or Content-Type
func (hs *HTTPStorage) getFileExtension(url, contentType string) string {
	// First try to get extension from URL
	if ext := filepath.Ext(url); ext != "" {
		return ext
	}

	// Fall back to Content-Type header
	switch contentType {
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/x-msvideo":
		return ".avi"
	case "video/x-matroska":
		return ".mkv"
	case "video/webm":
		return ".webm"
	default:
		// Default to .mp4 if we can't determine
		return ".mp4"
	}
}
