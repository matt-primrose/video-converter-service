package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// LocalStorage implements the Storage interface for local filesystem storage
type LocalStorage struct {
	config   StorageConfig
	basePath string
}

// NewLocalStorage creates a new local storage instance
func NewLocalStorage(basePath string, config StorageConfig) *LocalStorage {
	return &LocalStorage{
		config:   config,
		basePath: basePath,
	}
}

// DownloadFile for local storage means copying from one local path to temp directory
func (ls *LocalStorage) DownloadFile(ctx context.Context, sourceURI string, jobID string) (string, error) {
	// Remove file:// prefix if present
	localPath := strings.TrimPrefix(sourceURI, "file://")

	// Check if file exists
	if _, err := os.Stat(localPath); err != nil {
		return "", fmt.Errorf("source file not found: %w", err)
	}

	// Create temp directory for this job
	tempDir := filepath.Join(ls.config.TempDir, jobID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Get file extension and create temp file path
	ext := filepath.Ext(localPath)
	tempFile := filepath.Join(tempDir, "source"+ext)

	// Copy file
	if err := ls.copyFile(localPath, tempFile); err != nil {
		return "", fmt.Errorf("failed to copy local file: %w", err)
	}

	slog.Info("Successfully copied local file",
		"jobId", jobID,
		"sourcePath", localPath,
		"tempPath", tempFile,
	)

	return tempFile, nil
}

// UploadFile uploads a file to the local storage base path
func (ls *LocalStorage) UploadFile(ctx context.Context, sourcePath string, destinationPath string) error {
	fullDestPath := filepath.Join(ls.basePath, destinationPath)

	// Create destination directory
	destDir := filepath.Dir(fullDestPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Copy file
	if err := ls.copyFile(sourcePath, fullDestPath); err != nil {
		return fmt.Errorf("failed to copy file to destination: %w", err)
	}

	slog.Debug("Uploaded file to local storage",
		"sourcePath", sourcePath,
		"destinationPath", fullDestPath,
	)

	return nil
}

// UploadFiles uploads multiple files to local storage
func (ls *LocalStorage) UploadFiles(ctx context.Context, fileMap map[string]string) error {
	for sourcePath, destinationPath := range fileMap {
		if err := ls.UploadFile(ctx, sourcePath, destinationPath); err != nil {
			return fmt.Errorf("failed to upload file %s: %w", sourcePath, err)
		}
	}
	return nil
}

// GetFileURL returns a file:// URL for local files
func (ls *LocalStorage) GetFileURL(destinationPath string) (string, error) {
	fullPath := filepath.Join(ls.basePath, destinationPath)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Convert to file:// URL
	return "file://" + filepath.ToSlash(absPath), nil
}

// DeleteFile deletes a file from local storage
func (ls *LocalStorage) DeleteFile(ctx context.Context, destinationPath string) error {
	fullPath := filepath.Join(ls.basePath, destinationPath)

	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	slog.Debug("Deleted file from local storage", "path", fullPath)
	return nil
}

// ListFiles lists files in a directory
func (ls *LocalStorage) ListFiles(ctx context.Context, prefix string) ([]string, error) {
	searchPath := filepath.Join(ls.basePath, prefix)

	var files []string
	err := filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			// Get relative path from base
			relPath, err := filepath.Rel(ls.basePath, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(relPath))
		}

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	return files, nil
}

// GetType returns the storage type
func (ls *LocalStorage) GetType() string {
	return "local"
}

// copyFile copies a file from source to destination
func (ls *LocalStorage) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	return nil
}
