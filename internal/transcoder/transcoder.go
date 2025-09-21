package transcoder

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/matt-primrose/video-converter-service/internal/config"
	"github.com/matt-primrose/video-converter-service/pkg/models"
)

// Transcoder provides video transcoding capabilities using FFmpeg
type Transcoder struct {
	config     *config.Config
	ffmpegBin  string
	ffprobeBin string
	tempDir    string
}

// NewTranscoder creates a new transcoder instance
func NewTranscoder(cfg *config.Config) (*Transcoder, error) {
	t := &Transcoder{
		config:     cfg,
		ffmpegBin:  cfg.FFmpeg.BinaryPath,
		ffprobeBin: cfg.FFmpeg.ProbePath,
		tempDir:    cfg.Processing.TempDir,
	}

	// Verify FFmpeg installation
	if err := t.verifyFFmpeg(); err != nil {
		return nil, fmt.Errorf("ffmpeg verification failed: %w", err)
	}

	// Ensure temp directory exists
	if err := os.MkdirAll(t.tempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	return t, nil
}

// verifyFFmpeg checks if FFmpeg is installed and accessible
func (t *Transcoder) verifyFFmpeg() error {
	cmd := exec.Command(t.ffmpegBin, "-version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ffmpeg not found or not executable: %w", err)
	}

	slog.Info("FFmpeg verified", "version", strings.Split(string(output), "\n")[0])
	return nil
}

// TranscodeResult represents the result of a transcoding operation
type TranscodeResult struct {
	Outputs    []models.ConversionOutput `json:"outputs"`
	Duration   time.Duration             `json:"duration"`
	Statistics TranscodeStatistics       `json:"statistics"`
}

// TranscodeStatistics contains detailed statistics about the transcoding process
type TranscodeStatistics struct {
	InputDuration    time.Duration    `json:"inputDuration"`
	FramesProcessed  int              `json:"framesProcessed"`
	AverageSpeed     float64          `json:"averageSpeed"`
	BitrateReduction float64          `json:"bitrateReduction"`
	CompressionRatio float64          `json:"compressionRatio"`
	ProcessingTime   time.Duration    `json:"processingTime"`
	OutputFilesSizes map[string]int64 `json:"outputFilesSizes"`
}

// ProgressCallback is called during transcoding to report progress
type ProgressCallback func(progress float64, currentFrame, totalFrames int, speed float64)

// Transcode performs video transcoding based on the job template
func (t *Transcoder) Transcode(ctx context.Context, job *models.ConversionJob,
	template *config.JobTemplate, inputPath string, progressCallback ProgressCallback) (*TranscodeResult, error) {

	startTime := time.Now()
	slog.Info("Starting transcoding",
		"jobId", job.JobID,
		"inputPath", inputPath,
		"outputCount", len(template.Outputs),
	)

	// Create job-specific temp directory
	jobTempDir := filepath.Join(t.tempDir, job.JobID)
	if err := os.MkdirAll(jobTempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create job temp directory: %w", err)
	}
	// Note: Cleanup is handled by the caller (worker) after file upload

	// Get input video information
	inputInfo, err := t.getVideoInfo(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get input video info: %w", err)
	}

	var outputs []models.ConversionOutput
	var totalProcessingTime time.Duration
	outputSizes := make(map[string]int64)

	// Process each output configuration
	for i, output := range template.Outputs {
		slog.Info("Processing output",
			"jobId", job.JobID,
			"outputIndex", i,
			"outputName", output.Name,
			"package", output.Package,
		)

		outputResult, err := t.processOutput(ctx, inputPath, &output, jobTempDir,
			inputInfo, template.FFmpeg, progressCallback)
		if err != nil {
			return nil, fmt.Errorf("failed to process output '%s': %w", output.Name, err)
		}

		outputs = append(outputs, *outputResult)

		// Extract processing time from metadata
		if processingTimeStr, exists := outputResult.Metadata["processing_time"]; exists {
			if processingTime, err := time.ParseDuration(processingTimeStr); err == nil {
				totalProcessingTime += processingTime
			}
		}

		// Calculate output file sizes
		for _, file := range outputResult.Files {
			outputSizes[file.Path] = file.Size
		}
	}

	// Calculate statistics
	stats := TranscodeStatistics{
		InputDuration:    inputInfo.Duration,
		ProcessingTime:   totalProcessingTime,
		OutputFilesSizes: outputSizes,
	}

	// Calculate compression ratio if we have file size info
	if inputInfo.Size > 0 {
		var totalOutputSize int64
		for _, size := range outputSizes {
			totalOutputSize += size
		}
		if totalOutputSize > 0 {
			stats.CompressionRatio = float64(inputInfo.Size) / float64(totalOutputSize)
		}
	}

	result := &TranscodeResult{
		Outputs:    outputs,
		Duration:   time.Since(startTime),
		Statistics: stats,
	}

	slog.Info("Transcoding completed",
		"jobId", job.JobID,
		"duration", result.Duration,
		"outputCount", len(outputs),
	)

	return result, nil
}

// processOutput handles a single output configuration
func (t *Transcoder) processOutput(ctx context.Context, inputPath string,
	output *config.OutputConfig, jobTempDir string, inputInfo *VideoInfo,
	ffmpegConfig config.JobFFmpegConfig, progressCallback ProgressCallback) (*models.ConversionOutput, error) {

	outputDir := filepath.Join(jobTempDir, output.Name)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	switch strings.ToLower(output.Package) {
	case "hls":
		return t.transcodeHLS(ctx, inputPath, output, outputDir, inputInfo, ffmpegConfig, progressCallback)
	case "progressive", "mp4":
		return t.transcodeProgressive(ctx, inputPath, output, outputDir, inputInfo, ffmpegConfig, progressCallback)
	default:
		return nil, fmt.Errorf("unsupported package type: %s", output.Package)
	}
}
