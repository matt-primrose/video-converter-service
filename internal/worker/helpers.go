package worker

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/matt-primrose/video-converter-service/internal/config"
	"github.com/matt-primrose/video-converter-service/internal/transcoder"
	"github.com/matt-primrose/video-converter-service/pkg/models"
)

// downloadSourceFile downloads the source file from the specified URI
func (w *Worker) downloadSourceFile(ctx context.Context, job *models.ConversionJob) (string, error) {
	sourceURI := job.Source.URI
	sourceType := strings.ToLower(job.Source.Type)

	slog.Info("Downloading source file",
		"jobId", job.JobID,
		"sourceUri", sourceURI,
		"sourceType", sourceType,
	)

	switch sourceType {
	case "http", "https":
		return w.downloadHTTPFile(ctx, job.JobID, sourceURI)
	case "local":
		return w.copyLocalFile(ctx, job.JobID, sourceURI)
	case "azure-blob":
		return w.downloadAzureBlobFile(ctx, job.JobID, sourceURI)
	case "s3":
		return w.downloadS3File(ctx, job.JobID, sourceURI)
	default:
		return "", fmt.Errorf("unsupported source type: %s", sourceType)
	}
}

// downloadHTTPFile downloads a file from HTTP/HTTPS URL
func (w *Worker) downloadHTTPFile(ctx context.Context, jobID, url string) (string, error) {
	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Make HTTP request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP request failed with status: %s", resp.Status)
	}

	// Create temporary file
	tempDir := filepath.Join(w.config.Processing.TempDir, jobID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Determine file extension from Content-Type or URL
	ext := w.getFileExtension(resp.Header.Get("Content-Type"), url)
	tempFile := filepath.Join(tempDir, "source"+ext)

	// Create output file
	out, err := os.Create(tempFile)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer out.Close()

	// Copy content
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save file: %w", err)
	}

	slog.Info("Successfully downloaded HTTP file",
		"jobId", jobID,
		"localPath", tempFile,
	)

	return tempFile, nil
}

// copyLocalFile copies a local file to the temp directory
func (w *Worker) copyLocalFile(ctx context.Context, jobID, localPath string) (string, error) {
	// Check if file exists
	if _, err := os.Stat(localPath); err != nil {
		return "", fmt.Errorf("source file not found: %w", err)
	}

	// Create temp directory
	tempDir := filepath.Join(w.config.Processing.TempDir, jobID)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Get file extension
	ext := filepath.Ext(localPath)
	tempFile := filepath.Join(tempDir, "source"+ext)

	// Copy file
	src, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(tempFile)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("failed to copy file: %w", err)
	}

	slog.Info("Successfully copied local file",
		"jobId", jobID,
		"sourcePath", localPath,
		"tempPath", tempFile,
	)

	return tempFile, nil
}

// downloadAzureBlobFile downloads from Azure Blob Storage (placeholder)
func (w *Worker) downloadAzureBlobFile(ctx context.Context, jobID, blobURI string) (string, error) {
	// TODO: Implement Azure Blob Storage download
	return "", fmt.Errorf("azure blob storage download not yet implemented")
}

// downloadS3File downloads from Amazon S3 (placeholder)
func (w *Worker) downloadS3File(ctx context.Context, jobID, s3URI string) (string, error) {
	// TODO: Implement S3 download
	return "", fmt.Errorf("S3 download not yet implemented")
}

// validateSourceFile performs basic validation on the source file
func (w *Worker) validateSourceFile(filePath string) error {
	// Check file exists and is readable
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("cannot access source file: %w", err)
	}

	// Check file is not empty
	if fileInfo.Size() == 0 {
		return fmt.Errorf("source file is empty")
	}

	// Check file size doesn't exceed limits
	maxSizeGB := int64(w.config.Processing.MaxTempDiskGB)
	if maxSizeGB > 0 && fileInfo.Size() > maxSizeGB*1024*1024*1024 {
		return fmt.Errorf("source file size exceeds maximum allowed (%dGB)", maxSizeGB)
	}

	slog.Info("Source file validation passed",
		"filePath", filePath,
		"size", fileInfo.Size(),
	)

	return nil
}

// uploadOutputFiles uploads the converted files to storage (placeholder)
func (w *Worker) uploadOutputFiles(ctx context.Context, job *models.ConversionJob, result *transcoder.TranscodeResult) error {
	slog.Info("Uploading output files",
		"jobId", job.JobID,
		"outputCount", len(result.Outputs),
	)

	// For local storage, copy files to outputs directory
	if w.config.Storage.Type == "local" {
		return w.copyOutputFilesToLocal(job, result)
	}

	// TODO: Implement actual upload logic for other storage types (Azure, S3, etc.)
	// For now, just log the files that would be uploaded
	for _, output := range result.Outputs {
		slog.Info("Output ready for upload",
			"jobId", job.JobID,
			"outputName", output.Name,
			"type", output.Type,
			"fileCount", len(output.Files),
		)

		for _, file := range output.Files {
			slog.Debug("File ready for upload",
				"jobId", job.JobID,
				"filePath", file.Path,
				"size", file.Size,
				"checksum", file.Checksum,
			)
		}
	}

	return nil
}

// copyOutputFilesToLocal copies output files to the local outputs directory
func (w *Worker) copyOutputFilesToLocal(job *models.ConversionJob, result *transcoder.TranscodeResult) error {
	// Create job-specific directory within the local storage path
	outputsDir := filepath.Join(w.config.Storage.Local.Path, job.JobID)

	// Create job-specific output directory
	if err := os.MkdirAll(outputsDir, 0755); err != nil {
		return fmt.Errorf("failed to create outputs directory: %w", err)
	}

	// Copy all output files
	for _, output := range result.Outputs {
		outputTypeDir := filepath.Join(outputsDir, output.Type)
		if err := os.MkdirAll(outputTypeDir, 0755); err != nil {
			return fmt.Errorf("failed to create output type directory %s: %w", output.Type, err)
		}

		for _, file := range output.Files {
			destPath := filepath.Join(outputTypeDir, filepath.Base(file.Path))
			if err := w.copyFile(file.Path, destPath); err != nil {
				return fmt.Errorf("failed to copy file %s to %s: %w", file.Path, destPath, err)
			}
			slog.Info("Copied output file",
				"jobId", job.JobID,
				"outputType", output.Type,
				"source", file.Path,
				"destination", destPath,
			)
		}
	}

	slog.Info("All output files copied to local storage",
		"jobId", job.JobID,
		"outputsDir", outputsDir,
	)

	return nil
}

// copyFile copies a file from src to dst
func (w *Worker) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// sendNotifications sends completion notifications (placeholder)
func (w *Worker) sendNotifications(ctx context.Context, job *models.ConversionJob,
	template *config.JobTemplate, result *transcoder.TranscodeResult) error {

	if template.Notifications.WebhookURL == "" {
		slog.Debug("No webhook configured for notifications", "jobId", job.JobID)
		return nil
	}

	if !template.Notifications.OnComplete {
		slog.Debug("Completion notifications disabled", "jobId", job.JobID)
		return nil
	}

	slog.Info("Sending completion notification",
		"jobId", job.JobID,
		"webhookUrl", template.Notifications.WebhookURL,
	)

	// TODO: Implement actual webhook notification
	// This would typically involve sending an HTTP POST with job results

	return nil
}

// cleanupFile removes a file and logs any errors
func (w *Worker) cleanupFile(filePath string) {
	if filePath == "" {
		return
	}

	if err := os.Remove(filePath); err != nil {
		slog.Warn("Failed to cleanup file", "filePath", filePath, "error", err)
	} else {
		slog.Debug("Cleaned up file", "filePath", filePath)
	}
}

// getFileExtension determines file extension from content type or URL
func (w *Worker) getFileExtension(contentType, url string) string {
	// Try to determine from content type
	contentTypeMap := map[string]string{
		"video/mp4":        ".mp4",
		"video/quicktime":  ".mov",
		"video/x-msvideo":  ".avi",
		"video/webm":       ".webm",
		"video/x-matroska": ".mkv",
	}

	if ext, exists := contentTypeMap[contentType]; exists {
		return ext
	}

	// Fall back to URL extension
	if ext := filepath.Ext(url); ext != "" {
		return ext
	}

	// Default to .mp4
	return ".mp4"
}
