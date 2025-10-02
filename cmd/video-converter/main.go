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
	fmt.Printf("=== Video Converter Test Mode ===\n")
	fmt.Printf("Test Type: %s\n", testType)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
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
		fmt.Printf("Unknown test type: %s\n", testType)
		fmt.Println("Available types: direct, worker, upload, create-video")
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
	fmt.Println("--- Direct Transcoding Test ---")

	// Validate required input parameter
	if inputFile == "" {
		fmt.Printf("Error: Input video file is required for direct transcoding test\n")
		fmt.Printf("Usage: -input \"/path/to/video.mp4\"\n")
		os.Exit(1)
	}

	// Check if input exists
	if _, err := os.Stat(inputFile); os.IsNotExist(err) {
		fmt.Printf("Input file not found: %s\n", inputFile)
		os.Exit(1)
	}

	// Set default output if not specified
	if outputFile == "" {
		dir := filepath.Dir(inputFile)
		base := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile))
		outputFile = filepath.Join(dir, base+"_720p.mp4")
	}

	fmt.Printf("Input: %s\n", inputFile)
	fmt.Printf("Output: %s\n", outputFile)

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
		fmt.Printf("Transcoding failed: %v\n", err)
		os.Exit(1)
	}

	duration := time.Since(start)
	if stat, err := os.Stat(outputFile); err == nil {
		fmt.Printf("✅ Transcoding completed successfully!\n")
		fmt.Printf("Duration: %v\n", duration)
		fmt.Printf("Output size: %d bytes\n", stat.Size())
	} else {
		fmt.Printf("❌ Output file not created\n")
		os.Exit(1)
	}
}

func testWorkerProcessing(jobFile string, waitTime time.Duration, cfg *config.Config) {
	fmt.Println("--- Worker Processing Test ---")

	// Validate required job file parameter
	if jobFile == "" {
		fmt.Printf("Error: Job configuration file is required for worker test\n")
		fmt.Printf("Usage: -job \"examples/local-job.json\" (for local) or -job \"examples/docker-job.json\" (for Docker)\n")
		os.Exit(1)
	}

	// Create worker
	w, err := worker.New(cfg)
	if err != nil {
		fmt.Printf("Failed to create worker: %v\n", err)
		os.Exit(1)
	}

	// Load job
	jobData, err := os.ReadFile(jobFile)
	if err != nil {
		fmt.Printf("Failed to read job file %s: %v\n", jobFile, err)
		os.Exit(1)
	}

	var job models.ConversionJob
	if err := json.Unmarshal(jobData, &job); err != nil {
		fmt.Printf("Failed to parse job JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Job ID: %s\n", job.JobID)
	fmt.Printf("Source: %s\n", job.Source.URI)

	// Check source exists
	if _, err := os.Stat(job.Source.URI); os.IsNotExist(err) {
		fmt.Printf("Source video not found: %s\n", job.Source.URI)
		os.Exit(1)
	}

	// Start worker
	ctx, cancel := context.WithCancel(context.Background())
	go w.Start(ctx)
	time.Sleep(1 * time.Second) // Let workers start

	// Submit job
	if err := w.SubmitJob(&job); err != nil {
		fmt.Printf("Failed to submit job: %v\n", err)
		cancel()
		os.Exit(1)
	}

	fmt.Printf("Job submitted, waiting up to %v for processing...\n", waitTime)

	// Monitor job completion instead of fixed wait
	startTime := time.Now()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			elapsed := time.Since(startTime)
			if elapsed > waitTime {
				fmt.Printf("Job timed out after %v\n", elapsed)
				cancel()
				goto checkResults
			}

			// Check if job completed by looking for output files or completed status
			fmt.Printf("Job still processing... elapsed: %v\n", elapsed.Round(time.Second))

		case <-time.After(waitTime):
			fmt.Printf("Job wait time exceeded\n")
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
	fmt.Println("--- File Upload Test ---")

	// Validate required job file parameter
	if jobFile == "" {
		fmt.Printf("Error: Job configuration file is required for upload test\n")
		fmt.Printf("Usage: -job \"examples/local-job.json\" (for local) or -job \"examples/docker-job.json\" (for Docker)\n")
		os.Exit(1)
	}

	// Load job
	jobData, err := os.ReadFile(jobFile)
	if err != nil {
		fmt.Printf("Failed to read job file %s: %v\n", jobFile, err)
		os.Exit(1)
	}

	var job models.ConversionJob
	if err := json.Unmarshal(jobData, &job); err != nil {
		fmt.Printf("Failed to parse job JSON: %v\n", err)
		os.Exit(1)
	}

	// Check temp directory for existing files
	tempDir := filepath.Join(cfg.Processing.TempDir, job.JobID)
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		fmt.Printf("No temp directory found: %s\n", tempDir)
		fmt.Println("Run a worker test first to generate transcoded files.")
		os.Exit(1)
	}

	fmt.Printf("Found temp directory: %s\n", tempDir)

	// Mock upload process - copy files from temp to outputs staging area
	outputPath := cfg.Processing.OutputsDir
	if outputPath == "" {
		// Fallback to local storage path for backward compatibility
		outputPath = cfg.Storage.Local.Path
	}
	outputsDir := filepath.Join(outputPath, job.JobID)
	fmt.Printf("Target outputs directory: %s\n", outputsDir)

	if err := os.MkdirAll(outputsDir, 0755); err != nil {
		fmt.Printf("Failed to create outputs directory: %v\n", err)
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
		fmt.Printf("Copied: %s (%d bytes)\n", relPath, info.Size())
		return nil
	})

	if err != nil {
		fmt.Printf("Error during file copying: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Upload test completed!\n")
	fmt.Printf("Files copied: %d\n", totalFiles)
	fmt.Printf("Total size: %d bytes\n", totalSize)
}

func createTestVideo(outputPath string, duration time.Duration, resolution string, cfg *config.Config) {
	fmt.Println("--- Create Test Video ---")

	// Validate required output path parameter
	if outputPath == "" {
		fmt.Printf("Error: Output path is required for create-video test\n")
		fmt.Printf("Usage: -input \"./video_source/sample.mp4\"\n")
		os.Exit(1)
	}

	// Parse resolution
	parts := strings.Split(resolution, "x")
	if len(parts) != 2 {
		fmt.Printf("Invalid resolution format: %s (use WIDTHxHEIGHT)\n", resolution)
		os.Exit(1)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		fmt.Printf("Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Creating %s video at %s for %v\n", resolution, outputPath, duration)

	// Create test pattern video using FFmpeg
	durationStr := fmt.Sprintf("%.1f", duration.Seconds())
	cmd := exec.Command(cfg.FFmpeg.BinaryPath,
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=duration=%s:size=%s:rate=30", durationStr, resolution),
		"-c:v", "libx264", "-preset", "fast", "-crf", "23",
		"-pix_fmt", "yuv420p", "-y", outputPath)

	start := time.Now()
	if err := cmd.Run(); err != nil {
		fmt.Printf("Video creation failed: %v\n", err)
		os.Exit(1)
	}

	createDuration := time.Since(start)
	if stat, err := os.Stat(outputPath); err == nil {
		fmt.Printf("✅ Test video created successfully!\n")
		fmt.Printf("Creation time: %v\n", createDuration)
		fmt.Printf("File size: %d bytes\n", stat.Size())
	} else {
		fmt.Printf("❌ Test video not created\n")
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
	fmt.Printf("\nChecking outputs directory: %s\n", outputsDir)

	if _, err := os.Stat(outputsDir); err == nil {
		fmt.Println("✅ Outputs directory exists! Listing files:")
		totalFiles := 0
		totalSize := int64(0)

		err := filepath.Walk(outputsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				relPath, _ := filepath.Rel(outputsDir, path)
				fmt.Printf("  %s (%d bytes)\n", relPath, info.Size())
				totalFiles++
				totalSize += info.Size()
			}
			return nil
		})

		if err != nil {
			fmt.Printf("Error listing files: %v\n", err)
		} else {
			fmt.Printf("Total: %d files, %d bytes\n", totalFiles, totalSize)
		}
	} else {
		fmt.Printf("❌ Outputs directory does not exist: %v\n", err)

		// Check temp directory
		tempDir := filepath.Join(cfg.Processing.TempDir, job.JobID)
		if _, err := os.Stat(tempDir); err == nil {
			fmt.Printf("💡 Temp directory exists: %s\n", tempDir)
			fmt.Println("Files may still be in temp directory. Use 'upload' test to copy them.")
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
