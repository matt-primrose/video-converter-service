package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/matt-primrose/video-converter-service/internal/config"
	"github.com/matt-primrose/video-converter-service/pkg/models"
)

// Worker manages the conversion job processing
type Worker struct {
	config   *config.Config
	jobQueue chan *models.ConversionJob
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

// New creates a new worker instance
func New(cfg *config.Config) *Worker {
	ctx, cancel := context.WithCancel(context.Background())

	return &Worker{
		config:   cfg,
		jobQueue: make(chan *models.ConversionJob, cfg.Processing.MaxConcurrentJobs*2), // Buffer for queuing
		ctx:      ctx,
		cancel:   cancel,
	}
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
		"duration", job.Status.CompletedAt.Sub(job.Status.StartedAt),
	)
}

// executeConversion performs the actual video conversion
func (w *Worker) executeConversion(ctx context.Context, job *models.ConversionJob, template *config.JobTemplate) error {
	slog.Info("Starting conversion execution",
		"jobId", job.JobID,
		"sourceUri", job.Source.URI,
		"outputCount", len(template.Outputs),
	)

	// TODO: Implement the actual conversion logic:
	// 1. Download source file from job.Source.URI
	// 2. Validate source file
	// 3. For each output in template.Outputs:
	//    - Generate ffmpeg command based on profiles
	//    - Execute ffmpeg conversion
	//    - Upload output files to storage
	// 4. Send notifications if configured
	// 5. Clean up temporary files

	// For now, simulate processing time
	select {
	case <-ctx.Done():
		return fmt.Errorf("conversion cancelled due to timeout")
	case <-time.After(time.Second * 2): // Simulate work
		slog.Info("Conversion simulation completed", "jobId", job.JobID)
	}

	return nil
}

// GetJobStatus returns the current status of all jobs (placeholder)
func (w *Worker) GetJobStatus() map[string]models.JobStatus {
	// TODO: Implement job status tracking
	// This would typically involve storing job statuses in memory or a database
	return make(map[string]models.JobStatus)
}
