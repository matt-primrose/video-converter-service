package transcoder

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/matt-primrose/video-converter-service/internal/config"
	"github.com/matt-primrose/video-converter-service/pkg/models"
)

// transcodeProgressive performs progressive MP4 transcoding
func (t *Transcoder) transcodeProgressive(ctx context.Context, inputPath string,
	output *config.OutputConfig, outputDir string, inputInfo *VideoInfo,
	ffmpegConfig config.JobFFmpegConfig, progressCallback ProgressCallback) (*models.ConversionOutput, error) {

	startTime := time.Now()
	slog.Info("Starting progressive MP4 transcoding",
		"inputPath", inputPath,
		"outputDir", outputDir,
		"profiles", len(output.Profiles),
	)

	var files []models.OutputFile
	var totalFrames int

	// If we have multiple profiles, create one MP4 file per profile
	if len(output.Profiles) > 0 {
		for i, profile := range output.Profiles {
			slog.Info("Transcoding progressive MP4 profile",
				"profile", profile.Name,
				"resolution", fmt.Sprintf("%dx%d", profile.Width, profile.Height),
				"bitrate", profile.VideoBitrateKbps,
			)

			profileFile, frames, err := t.transcodeProgressiveProfile(ctx, inputPath, &profile,
				outputDir, inputInfo, output, ffmpegConfig, progressCallback)
			if err != nil {
				return nil, fmt.Errorf("failed to transcode progressive profile '%s': %w", profile.Name, err)
			}

			files = append(files, *profileFile)
			if i == 0 { // Use first profile for total frame count
				totalFrames = frames
			}
		}
	} else if output.Profile != "" {
		// Single profile output
		slog.Info("Transcoding single progressive MP4 profile", "profile", output.Profile)

		profile := t.getProfileByName(output.Profile)

		profileFile, frames, err := t.transcodeProgressiveProfile(ctx, inputPath, &profile,
			outputDir, inputInfo, output, ffmpegConfig, progressCallback)
		if err != nil {
			return nil, fmt.Errorf("failed to transcode progressive profile '%s': %w", output.Profile, err)
		}

		files = append(files, *profileFile)
		totalFrames = frames
	} else {
		return nil, fmt.Errorf("no profiles specified for progressive output")
	}

	result := &models.ConversionOutput{
		Name:    output.Name,
		Type:    "progressive",
		Profile: output.Profile,
		Files:   files,
		Metadata: map[string]string{
			"package":         "progressive",
			"container":       output.Container,
			"total_frames":    strconv.Itoa(totalFrames),
			"processing_time": time.Since(startTime).String(),
		},
	}

	slog.Info("Progressive MP4 transcoding completed",
		"outputName", output.Name,
		"fileCount", len(files),
		"duration", time.Since(startTime),
	)

	return result, nil
}

// transcodeProgressiveProfile transcodes a single progressive MP4 profile
func (t *Transcoder) transcodeProgressiveProfile(ctx context.Context, inputPath string,
	profile *config.ProfileConfig, outputDir string, inputInfo *VideoInfo,
	output *config.OutputConfig, ffmpegConfig config.JobFFmpegConfig,
	progressCallback ProgressCallback) (*models.OutputFile, int, error) {

	// Determine container format
	container := output.Container
	if container == "" {
		container = "mp4"
	}

	// Create output filename
	outputFileName := fmt.Sprintf("%s.%s", profile.Name, container)
	outputPath := filepath.Join(outputDir, outputFileName)

	// Build FFmpeg command for progressive output
	args := t.buildProgressiveFFmpegArgs(inputPath, outputPath, profile, ffmpegConfig)

	slog.Debug("Running FFmpeg for progressive MP4",
		"profile", profile.Name,
		"outputPath", outputPath,
		"args", strings.Join(args, " "),
	)

	// Run FFmpeg with progress monitoring
	if err := t.runFFmpegWithProgress(ctx, args, inputInfo.TotalFrames, progressCallback); err != nil {
		return nil, 0, fmt.Errorf("ffmpeg execution failed: %w", err)
	}

	// Create output file info
	outputFile, err := t.createOutputFile(outputPath, t.getMimeType(container))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create output file info: %w", err)
	}

	return outputFile, inputInfo.TotalFrames, nil
}

// buildProgressiveFFmpegArgs builds FFmpeg arguments for progressive MP4 transcoding
func (t *Transcoder) buildProgressiveFFmpegArgs(inputPath, outputPath string,
	profile *config.ProfileConfig, ffmpegConfig config.JobFFmpegConfig) []string {

	args := []string{
		"-i", inputPath,
		"-c:v", "libx264",
		"-c:a", "aac",
	}

	// Add hardware acceleration if configured
	if ffmpegConfig.HWAccel != "" {
		args = append([]string{"-hwaccel", ffmpegConfig.HWAccel}, args...)
	}

	// Video encoding settings
	args = append(args,
		"-vf", fmt.Sprintf("scale=%d:%d", profile.Width, profile.Height),
		"-b:v", fmt.Sprintf("%dk", profile.VideoBitrateKbps),
		"-maxrate", fmt.Sprintf("%dk", profile.VideoBitrateKbps),
		"-bufsize", fmt.Sprintf("%dk", profile.VideoBitrateKbps*2),
		"-profile:v", "high",
		"-level", "4.0",
	)

	// Audio encoding settings
	if profile.AudioBitrateKbps > 0 {
		args = append(args,
			"-b:a", fmt.Sprintf("%dk", profile.AudioBitrateKbps),
		)
	} else {
		args = append(args, "-b:a", "128k")
	}

	// Progressive download optimization
	args = append(args,
		"-movflags", "+faststart", // Move moov atom to beginning for progressive download
		"-pix_fmt", "yuv420p", // Ensure compatibility
	)

	// Add preset if configured
	if ffmpegConfig.Preset != "" {
		args = append(args, "-preset", ffmpegConfig.Preset)
	}

	// Add extra args if configured
	if len(ffmpegConfig.ExtraArgs) > 0 {
		args = append(args, ffmpegConfig.ExtraArgs...)
	}

	// Output file
	args = append(args, "-y", outputPath) // -y to overwrite existing files

	return args
}

// getMimeType returns the MIME type for a given container format
func (t *Transcoder) getMimeType(container string) string {
	mimeTypes := map[string]string{
		"mp4":  "video/mp4",
		"webm": "video/webm",
		"mov":  "video/quicktime",
		"avi":  "video/x-msvideo",
		"mkv":  "video/x-matroska",
	}

	if mimeType, exists := mimeTypes[container]; exists {
		return mimeType
	}

	return "video/mp4" // Default to MP4
}
