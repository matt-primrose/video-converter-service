package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/matt-primrose/video-converter-service/internal/config"
	"github.com/matt-primrose/video-converter-service/internal/storage"
	"github.com/matt-primrose/video-converter-service/internal/transcoder"
	"github.com/matt-primrose/video-converter-service/pkg/models"
)

// downloadSourceFile downloads the source file from the specified URI using storage interface
func (w *Worker) downloadSourceFile(ctx context.Context, job *models.ConversionJob) (string, error) {
	sourceURI := job.Source.URI
	sourceType := strings.ToLower(job.Source.Type)

	slog.Info("Downloading source file",
		"jobId", job.JobID,
		"sourceUri", sourceURI,
		"sourceType", sourceType,
	)

	// Create download-specific storage instance
	downloadStorage, err := storage.NewDownloadOnlyStorage(sourceType, w.config)
	if err != nil {
		return "", fmt.Errorf("failed to create download storage: %w", err)
	}

	// Use storage interface to download the file
	return downloadStorage.DownloadFile(ctx, sourceURI, job.JobID)
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

// uploadOutputFiles uploads the converted files to storage using storage interface
func (w *Worker) uploadOutputFiles(ctx context.Context, job *models.ConversionJob, result *transcoder.TranscodeResult) error {
	slog.Info("Uploading output files",
		"jobId", job.JobID,
		"outputCount", len(result.Outputs),
		"storageType", w.outputStorage.GetType(),
	)

	// Build file map for upload
	fileMap := make(map[string]string)

	for _, output := range result.Outputs {
		for _, file := range output.Files {
			// Create destination path: jobId/outputName/filename
			destPath := filepath.Join(job.JobID, output.Name, filepath.Base(file.Path))
			fileMap[file.Path] = destPath

			slog.Debug("Mapping file for upload",
				"jobId", job.JobID,
				"localPath", file.Path,
				"destPath", destPath,
			)
		}
	}

	// Upload all files using storage interface
	if err := w.outputStorage.UploadFiles(ctx, fileMap); err != nil {
		return fmt.Errorf("failed to upload files via storage interface: %w", err)
	}

	slog.Info("Successfully uploaded all output files",
		"jobId", job.JobID,
		"fileCount", len(fileMap),
		"storageType", w.outputStorage.GetType(),
	)

	return nil
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
