package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/matt-primrose/video-converter-service/internal/config"
	"github.com/matt-primrose/video-converter-service/internal/transcoder"
	"github.com/matt-primrose/video-converter-service/pkg/models"
)

// Worker manages the conversion job processing
type Worker struct {
	config     *config.Config
	transcoder *transcoder.Transcoder
	jobQueue   chan *models.ConversionJob
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

// New creates a new worker instance
func New(cfg *config.Config) (*Worker, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Initialize basic transcoder
	tc, err := transcoder.NewTranscoder(cfg)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize transcoder: %w", err)
	}

	return &Worker{
		config:     cfg,
		transcoder: tc,
		jobQueue:   make(chan *models.ConversionJob, cfg.Processing.MaxConcurrentJobs*2), // Buffer for queuing
		ctx:        ctx,
		cancel:     cancel,
	}, nil
}

// Start starts the worker pool
func (w *Worker) Start(ctx context.Context) {
	slog.Info("Starting worker pool",
		"maxConcurrentJobs", w.config.Processing.MaxConcurrentJobs)

	// Start worker goroutines
	for i := 0; i < w.config.Processing.MaxConcurrentJobs; i++ {
		w.wg.Add(1)
		go w.workerLoop(i)
	}

	// Wait for context cancellation
	<-ctx.Done()
	slog.Info("Stopping worker pool...")

	// Cancel all workers
	w.cancel()

	// Close job queue to prevent new jobs
	close(w.jobQueue)

	// Wait for all workers to finish
	w.wg.Wait()
	slog.Info("Worker pool stopped")
}

// SubmitJob submits a new job to the worker queue
func (w *Worker) SubmitJob(job *models.ConversionJob) error {
	job.CreatedAt = time.Now()
	job.Status = models.JobStatus{
		State:   models.JobStatePending,
		Message: "Job queued for processing",
	}

	select {
	case w.jobQueue <- job:
		slog.Info("Job queued", "jobId", job.JobID)
		return nil
	default:
		return fmt.Errorf("job queue is full")
	}
}

// workerLoop is the main processing loop for a single worker
func (w *Worker) workerLoop(workerID int) {
	defer w.wg.Done()

	slog.Info("Starting worker", "workerId", workerID)

	for {
		select {
		case <-w.ctx.Done():
			slog.Info("Worker stopping", "workerId", workerID)
			return
		case job, ok := <-w.jobQueue:
			if !ok {
				slog.Info("Job queue closed, worker stopping", "workerId", workerID)
				return
			}

			w.processJob(workerID, job)
		}
	}
}

// processJob processes a single conversion job
func (w *Worker) processJob(workerID int, job *models.ConversionJob) {
	slog.Info("Processing job",
		"workerId", workerID,
		"jobId", job.JobID,
		"videoId", job.VideoID,
		"template", job.Template,
	)

	// Update job status
	job.Status.State = models.JobStateProcessing
	job.Status.StartedAt = time.Now()
	job.Status.Message = "Processing started"

	// Get job template
	template, exists := w.config.JobTemplates[job.Template]
	if !exists {
		job.Status.State = models.JobStateFailed
		job.Status.Error = fmt.Sprintf("Job template '%s' not found", job.Template)
		job.Status.CompletedAt = time.Now()
		slog.Error("Job template not found",
			"jobId", job.JobID,
			"template", job.Template,
		)
		return
	}

	// Process the job with timeout
	jobCtx, cancel := context.WithTimeout(w.ctx,
		time.Duration(w.config.Processing.JobTimeoutMinutes)*time.Minute)
	defer cancel()

	if err := w.executeConversion(jobCtx, job, &template); err != nil {
		job.Status.State = models.JobStateFailed
		job.Status.Error = err.Error()
		job.Status.CompletedAt = time.Now()
		slog.Error("Job conversion failed",
			"jobId", job.JobID,
			"error", err,
		)
		return
	}

	// Mark job as completed
	job.Status.State = models.JobStateCompleted
	job.Status.Progress = 1.0
	job.Status.CompletedAt = time.Now()
	job.Status.Message = "Conversion completed successfully"

	slog.Info("Job completed",
		"workerId", workerID,
		"jobId", job.JobID,
		"completed_at", job.Status.CompletedAt.Format(time.RFC3339),
	)
}

// executeConversion performs the actual video conversion
func (w *Worker) executeConversion(ctx context.Context, job *models.ConversionJob, template *config.JobTemplate) error {
	slog.Info("Starting conversion execution",
		"jobId", job.JobID,
		"sourceUri", job.Source.URI,
		"outputCount", len(template.Outputs),
	)

	// Step 1: Download source file from job.Source.URI
	inputPath, err := w.downloadSourceFile(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to download source file: %w", err)
	}
	// Note: File cleanup is handled after upload by cleaning the entire job temp directory

	// Step 2: Validate source file (basic validation)
	if err := w.validateSourceFile(inputPath); err != nil {
		return fmt.Errorf("source file validation failed: %w", err)
	}

	// Step 3: Progress callback to update job status
	progressCallback := func(progress float64, currentFrame, totalFrames int, speed float64) {
		job.Status.Progress = progress
		slog.Debug("Conversion progress",
			"jobId", job.JobID,
			"progress", fmt.Sprintf("%.2f%%", progress*100),
			"frame", currentFrame,
			"totalFrames", totalFrames,
			"speed", fmt.Sprintf("%.2fx", speed),
		)
	}

	// Step 4: Perform transcoding
	result, err := w.transcoder.Transcode(ctx, job, template, inputPath, progressCallback)
	if err != nil {
		return fmt.Errorf("transcoding failed: %w", err)
	}

	// Step 5: Upload output files to storage (placeholder)
	if err := w.uploadOutputFiles(ctx, job, result); err != nil {
		return fmt.Errorf("failed to upload output files: %w", err)
	}

	// Step 5.5: Cleanup job temp directory after successful upload
	jobTempDir := filepath.Join(w.config.Processing.TempDir, job.JobID)
	if err := os.RemoveAll(jobTempDir); err != nil {
		slog.Warn("Failed to clean up job temp directory", "jobId", job.JobID, "path", jobTempDir, "error", err)
	}

	// Step 6: Send notifications if configured
	if err := w.sendNotifications(ctx, job, template, result); err != nil {
		slog.Warn("Failed to send notifications", "jobId", job.JobID, "error", err)
		// Don't fail the job for notification errors
	}

	slog.Info("Conversion execution completed",
		"jobId", job.JobID,
		"duration", formatDuration(result.Duration),
		"outputCount", len(result.Outputs),
	)

	return nil
}

// GetJobStatus returns the current status of all jobs (placeholder)
func (w *Worker) GetJobStatus() map[string]models.JobStatus {
	// TODO: Implement job status tracking
	// This would typically involve storing job statuses in memory or a database
	return make(map[string]models.JobStatus)
}

// formatDuration formats a time.Duration into a human-readable string
// showing hours, minutes, and seconds as appropriate
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d)/float64(time.Millisecond))
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	} else {
		return fmt.Sprintf("%ds", seconds)
	}
}
