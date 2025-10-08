package storage

import (
	"context"
)

// Storage defines the interface for different storage backends
type Storage interface {
	// DownloadFile downloads a file from the storage backend to a local temporary file
	// Returns the local file path and any error
	DownloadFile(ctx context.Context, sourceURI string, jobID string) (string, error)

	// UploadFile uploads a local file to the storage backend
	// sourcePath is the local file path, destinationPath is the target path in storage
	UploadFile(ctx context.Context, sourcePath string, destinationPath string) error

	// UploadFiles uploads multiple files to the storage backend
	// fileMap maps local file paths to destination paths
	UploadFiles(ctx context.Context, fileMap map[string]string) error

	// GetFileURL returns a publicly accessible URL for a file (if supported)
	GetFileURL(destinationPath string) (string, error)

	// DeleteFile deletes a file from the storage backend
	DeleteFile(ctx context.Context, destinationPath string) error

	// ListFiles lists files in a directory/container (for cleanup, monitoring, etc.)
	ListFiles(ctx context.Context, prefix string) ([]string, error)

	// GetType returns the storage type name
	GetType() string
}

// DownloadResult contains information about a downloaded file
type DownloadResult struct {
	LocalPath    string
	OriginalPath string
	Size         int64
	ContentType  string
}

// UploadResult contains information about an uploaded file
type UploadResult struct {
	RemotePath string
	LocalPath  string
	Size       int64
	PublicURL  string
}

// StorageConfig contains common configuration for all storage types
type StorageConfig struct {
	TempDir    string
	OutputsDir string
}

// FileInfo represents metadata about a file in storage
type FileInfo struct {
	Path         string
	Size         int64
	LastModified string
	ContentType  string
	ETag         string
}
