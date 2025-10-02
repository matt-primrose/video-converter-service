package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/matt-primrose/video-converter-service/internal/config"
	"github.com/matt-primrose/video-converter-service/internal/events"
	"github.com/matt-primrose/video-converter-service/internal/worker"
	"github.com/matt-primrose/video-converter-service/pkg/models"
)

const (
	serviceName    = "video-converter-service"
	serviceVersion = "0.1.0"
)

func main() {
	// Parse command-line flags
	var (
		testMode    = flag.Bool("test", false, "Run in test mode")
		testType    = flag.String("test-type", "direct", "Test type: direct, worker, upload, create-video")
		jobFile     = flag.String("job", "", "Job configuration file (required for worker and upload tests)")
		inputVideo  = flag.String("input", "", "Input video file (required for direct and create-video tests)")
		outputFile  = flag.String("output", "", "Output file (for direct transcoding)")
		logLevel    = flag.String("log-level", "", "Log level override: debug, info, warn, error")
		waitTime    = flag.Duration("wait", 5*time.Minute, "Wait time for worker jobs (default: 5m)")
		videoLength = flag.Duration("video-length", 30*time.Second, "Length for created test videos")
		videoRes    = flag.String("video-res", "3840x2160", "Resolution for created test videos")
	)
	flag.Parse()

	// Initialize logger
	var logger *slog.Logger
	if *testMode {
		// Use text handler for better readability during testing
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	} else {
		// Use JSON handler for production
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}
	slog.SetDefault(logger)

	if *testMode {
		runTestMode(*testType, *jobFile, *inputVideo, *outputFile, *logLevel, *waitTime, *videoLength, *videoRes)
		return
	}

	// Production mode
	runProductionMode()
}

func runTestMode(testType, jobFile, inputVideo, outputFile, logLevel string, waitTime, videoLength time.Duration, videoRes string) {
	slog.Info("Starting Video Converter Test Mode", "test_type", testType)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Override log level if specified
	if logLevel != "" {
		setLogLevel(logLevel)
	} else {
		setLogLevel(cfg.Observability.LogLevel)
	}

	switch testType {
	case "direct":
		testDirectTranscoding(inputVideo, outputFile, cfg)
	case "worker":
		testWorkerProcessing(jobFile, waitTime, cfg)
	case "upload":
		testFileUpload(jobFile, cfg)
	case "create-video":
		createTestVideo(inputVideo, videoLength, videoRes, cfg)
	default:
		slog.Error("Unknown test type", "test_type", testType, "available_types", []string{"direct", "worker", "upload", "create-video"})
		os.Exit(1)
	}
}

func runProductionMode() {

	slog.Info("Starting video converter service",
		"service", serviceName,
		"version", serviceVersion,
	)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Set log level from config
	setLogLevel(cfg.Observability.LogLevel)

	slog.Info("Configuration loaded successfully",
		"storage_type", cfg.Storage.Type,
		"max_concurrent_jobs", cfg.Processing.MaxConcurrentJobs,
		"server_port", cfg.Server.Port,
	)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize components
	w, err := worker.New(cfg)
	if err != nil {
		slog.Error("Failed to initialize worker", "error", err)
		os.Exit(1)
	}

	eventRouter := events.NewRouter(cfg, w)

	// Start HTTP server for health checks
	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler: setupHTTPRoutes(cfg),
	}

	// Start health check server
	healthServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.HealthCheckPort),
		Handler: setupHealthRoutes(w),
	}

	var wg sync.WaitGroup

	// Start main HTTP server
	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("Starting HTTP server", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	// Start health check server
	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("Starting health check server", "addr", healthServer.Addr)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Health check server error", "error", err)
		}
	}()

	// Start event listeners
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := eventRouter.Start(ctx); err != nil {
			slog.Error("Event router error", "error", err)
		}
	}()

	// Start worker pool
	wg.Add(1)
	go func() {
		defer wg.Done()
		w.Start(ctx)
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	slog.Info("Shutting down service...")

	// Cancel context to stop all goroutines
	cancel()

	// Shutdown HTTP servers gracefully
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Error shutting down HTTP server", "error", err)
	}

	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("Error shutting down health server", "error", err)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	slog.Info("Service shutdown complete")
}

// setLogLevel configures the global log level based on config
func setLogLevel(level string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)
}

// setupHTTPRoutes creates the main HTTP server routes
func setupHTTPRoutes(cfg *config.Config) http.Handler {
	mux := http.NewServeMux()

	// WebSocket endpoint for events (if enabled)
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement WebSocket handler
		http.Error(w, "WebSocket handler not implemented", http.StatusNotImplemented)
	})

	// Status endpoint
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"service":"%s","version":"%s","status":"running"}`, serviceName, serviceVersion)
	})

	return mux
}

// setupHealthRoutes creates health check routes
func setupHealthRoutes(w *worker.Worker) http.Handler {
	mux := http.NewServeMux()

	// Liveness probe
	mux.HandleFunc("/healthz", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(http.StatusOK)
		fmt.Fprint(rw, "OK")
	})

	// Readiness probe
	mux.HandleFunc("/ready", func(rw http.ResponseWriter, r *http.Request) {
		// Simple readiness check - service is ready if it's running
		rw.WriteHeader(http.StatusOK)
		fmt.Fprint(rw, "Ready")
	})

	// Enhanced health endpoint
	mux.HandleFunc("/health", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		fmt.Fprintf(rw, `{
			"status": "healthy",
			"service": "%s",
			"version": "%s",
			"timestamp": "%s"
		}`, serviceName, serviceVersion, time.Now().Format(time.RFC3339))
	})

	return mux
}

// Test functions for development and debugging

func testDirectTranscoding(inputFile, outputFile string, cfg *config.Config) {
	slog.Info("Starting Direct Transcoding Test")

	// Validate required input parameter
	if inputFile == "" {
		slog.Error("Input video file is required for direct transcoding test", "usage", "-input \"/path/to/video.mp4\"")
		os.Exit(1)
	}

	// Check if input exists
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		slog.Error("Input file not found", "file", inputFile)
		os.Exit(1)
	}

	// Set default output if not specified
	if outputFile == "" {
		dir := filepath.Dir(inputFile)
		base := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile))
		outputFile = filepath.Join(dir, base+"_720p.mp4")
	}

	slog.Info("Direct transcoding configuration", "input", inputFile, "output", outputFile)

	// Simple 720p transcoding using exec directly
	start := time.Now()
	cmd := exec.Command(cfg.FFmpeg.BinaryPath,
		"-i", inputFile,
		"-c:v", "libx264", "-c:a", "aac",
		"-vf", "scale=1280:720",
		"-b:v", "2500k", "-maxrate", "2500k", "-bufsize", "5000k",
		"-profile:v", "high", "-level", "4.0",
		"-b:a", "128k", "-movflags", "+faststart",
		"-pix_fmt", "yuv420p", "-preset", "fast",
		"-y", outputFile)

	if err := cmd.Run(); err != nil {
		slog.Error("Transcoding failed", "error", err)
		os.Exit(1)
	}

	duration := time.Since(start)
	if stat, err := os.Stat(outputFile); err == nil {
		slog.Info("Transcoding completed successfully", "duration", duration, "output_size_bytes", stat.Size())
	} else {
		slog.Error("Output file not created", "expected_file", outputFile)
		os.Exit(1)
	}
}

func testWorkerProcessing(jobFile string, waitTime time.Duration, cfg *config.Config) {
	slog.Info("Starting Worker Processing Test")

	// Validate required job file parameter
	if jobFile == "" {
		slog.Error("Job configuration file is required for worker test", "usage", "-job \"examples/local-job.json\" (for local) or -job \"examples/docker-job.json\" (for Docker)")
		os.Exit(1)
	}

	// Create worker
	w, err := worker.New(cfg)
	if err != nil {
		slog.Error("Failed to create worker", "error", err)
		os.Exit(1)
	}

	// Load job
	jobData, err := os.ReadFile(jobFile)
	if err != nil {
		slog.Error("Failed to read job file", "file", jobFile, "error", err)
		os.Exit(1)
	}

	var job models.ConversionJob
	if err := json.Unmarshal(jobData, &job); err != nil {
		slog.Error("Failed to parse job JSON", "error", err)
		os.Exit(1)
	}

	slog.Info("Job loaded", "job_id", job.JobID, "source", job.Source.URI)

	// Check source exists
	if _, err := os.Stat(job.Source.URI); os.IsNotExist(err) {
		slog.Error("Source video not found", "source", job.Source.URI)
		os.Exit(1)
	}

	// Start worker
	ctx, cancel := context.WithCancel(context.Background())
	go w.Start(ctx)
	time.Sleep(1 * time.Second) // Let workers start

	// Submit job
	if err := w.SubmitJob(&job); err != nil {
		slog.Error("Failed to submit job", "error", err)
		cancel()
		os.Exit(1)
	}

	slog.Info("Job submitted, monitoring progress", "max_wait_time", waitTime)

	// Monitor job completion instead of fixed wait
	startTime := time.Now()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			elapsed := time.Since(startTime)
			if elapsed > waitTime {
				slog.Warn("Job timed out", "elapsed", elapsed)
				cancel()
				goto checkResults
			}
		case <-time.After(waitTime):
			slog.Warn("Job wait time exceeded")
			cancel()
			goto checkResults
		}
	}

checkResults:
	// Give a moment for cleanup
	time.Sleep(2 * time.Second)

	// Check results
	checkJobResults(&job, cfg)
}

func testFileUpload(jobFile string, cfg *config.Config) {
	slog.Info("Starting File Upload Test")

	// Validate required job file parameter
	if jobFile == "" {
		slog.Error("Job configuration file is required for upload test", "usage", "-job \"examples/local-job.json\" (for local) or -job \"examples/docker-job.json\" (for Docker)")
		os.Exit(1)
	}

	// Load job
	jobData, err := os.ReadFile(jobFile)
	if err != nil {
		slog.Error("Failed to read job file", "file", jobFile, "error", err)
		os.Exit(1)
	}

	var job models.ConversionJob
	if err := json.Unmarshal(jobData, &job); err != nil {
		slog.Error("Failed to parse job JSON", "error", err)
		os.Exit(1)
	}

	// Check temp directory for existing files
	tempDir := filepath.Join(cfg.Processing.TempDir, job.JobID)
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		slog.Error("No temp directory found", "temp_dir", tempDir, "hint", "Run a worker test first to generate transcoded files.")
		os.Exit(1)
	}

	slog.Info("Found temp directory", "temp_dir", tempDir)

	// Mock upload process - copy files from temp to outputs staging area
	outputPath := cfg.Processing.OutputsDir
	if outputPath == "" {
		// Fallback to local storage path for backward compatibility
		outputPath = cfg.Storage.Local.Path
	}
	outputsDir := filepath.Join(outputPath, job.JobID)
	slog.Info("Target outputs directory", "outputs_dir", outputsDir)

	if err := os.MkdirAll(outputsDir, 0755); err != nil {
		slog.Error("Failed to create outputs directory", "error", err)
		os.Exit(1)
	}

	// Find and copy transcoded files
	totalFiles := 0
	totalSize := int64(0)

	err = filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		// Skip source files
		if strings.HasSuffix(path, "source.mp4") {
			return nil
		}

		// Calculate relative path from temp dir
		relPath, err := filepath.Rel(tempDir, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(outputsDir, relPath)
		destDir := filepath.Dir(destPath)

		// Create destination directory
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("failed to create dir %s: %w", destDir, err)
		}

		// Copy file
		if err := copyFile(path, destPath); err != nil {
			return fmt.Errorf("failed to copy %s to %s: %w", path, destPath, err)
		}

		totalFiles++
		totalSize += info.Size()
		slog.Debug("File copied", "path", relPath, "size_bytes", info.Size())
		return nil
	})

	if err != nil {
		slog.Error("Error during file copying", "error", err)
		os.Exit(1)
	}

	slog.Info("Upload test completed", "files_copied", totalFiles, "total_size_bytes", totalSize)
}

func createTestVideo(outputPath string, duration time.Duration, resolution string, cfg *config.Config) {
	slog.Info("Starting Create Test Video")

	// Validate required output path parameter
	if outputPath == "" {
		slog.Error("Output path is required for create-video test", "usage", "-input \"./video_source/sample.mp4\"")
		os.Exit(1)
	}

	// Parse resolution
	parts := strings.Split(resolution, "x")
	if len(parts) != 2 {
		slog.Error("Invalid resolution format", "resolution", resolution, "expected_format", "WIDTHxHEIGHT")
		os.Exit(1)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		slog.Error("Failed to create output directory", "error", err)
		os.Exit(1)
	}

	slog.Info("Creating test video", "resolution", resolution, "output", outputPath, "duration", duration)

	// Create test pattern video using FFmpeg
	durationStr := fmt.Sprintf("%.1f", duration.Seconds())
	cmd := exec.Command(cfg.FFmpeg.BinaryPath,
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=duration=%s:size=%s:rate=30", durationStr, resolution),
		"-c:v", "libx264", "-preset", "fast", "-crf", "23",
		"-pix_fmt", "yuv420p", "-y", outputPath)

	start := time.Now()
	if err := cmd.Run(); err != nil {
		slog.Error("Video creation failed", "error", err)
		os.Exit(1)
	}

	createDuration := time.Since(start)
	if stat, err := os.Stat(outputPath); err == nil {
		slog.Info("Test video created successfully", "creation_time", createDuration, "file_size_bytes", stat.Size())
	} else {
		slog.Error("Test video not created", "expected_file", outputPath)
		os.Exit(1)
	}
}

func checkJobResults(job *models.ConversionJob, cfg *config.Config) {
	outputPath := cfg.Processing.OutputsDir
	if outputPath == "" {
		// Fallback to local storage path for backward compatibility
		outputPath = cfg.Storage.Local.Path
	}
	outputsDir := filepath.Join(outputPath, job.JobID)
	slog.Info("Checking outputs directory", "outputs_dir", outputsDir)

	if _, err := os.Stat(outputsDir); err == nil {
		slog.Info("Outputs directory exists, listing files")
		totalFiles := 0
		totalSize := int64(0)

		err := filepath.Walk(outputsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				relPath, _ := filepath.Rel(outputsDir, path)
				slog.Debug("Found output file", "path", relPath, "size_bytes", info.Size())
				totalFiles++
				totalSize += info.Size()
			}
			return nil
		})

		if err != nil {
			slog.Error("Error listing files", "error", err)
		} else {
			slog.Info("Job results summary", "total_files", totalFiles, "total_size_bytes", totalSize)
		}
	} else {
		slog.Warn("Outputs directory does not exist", "error", err)

		// Check temp directory
		tempDir := filepath.Join(cfg.Processing.TempDir, job.JobID)
		if _, err := os.Stat(tempDir); err == nil {
			slog.Info("Temp directory exists", "temp_dir", tempDir, "hint", "Files may still be in temp directory. Use 'upload' test to copy them.")
		}
	}
}

func copyFile(src, dst string) error {
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
