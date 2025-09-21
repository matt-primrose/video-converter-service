package transcoder

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/matt-primrose/video-converter-service/internal/config"
	"github.com/matt-primrose/video-converter-service/pkg/models"
)

// transcodeHLS performs HLS (HTTP Live Streaming) transcoding
func (t *Transcoder) transcodeHLS(ctx context.Context, inputPath string,
	output *config.OutputConfig, outputDir string, inputInfo *VideoInfo,
	ffmpegConfig config.JobFFmpegConfig, progressCallback ProgressCallback) (*models.ConversionOutput, error) {

	startTime := time.Now()
	slog.Info("Starting HLS transcoding",
		"inputPath", inputPath,
		"outputDir", outputDir,
		"profiles", len(output.Profiles),
	)

	var files []models.OutputFile
	var totalFrames int

	// If we have multiple profiles, create an adaptive bitrate ladder
	if len(output.Profiles) > 0 {
		// Create master playlist
		masterPlaylistPath := filepath.Join(outputDir, "master.m3u8")
		masterPlaylist, err := t.createMasterPlaylist(output.Profiles)
		if err != nil {
			return nil, fmt.Errorf("failed to create master playlist: %w", err)
		}

		if err := os.WriteFile(masterPlaylistPath, []byte(masterPlaylist), 0644); err != nil {
			return nil, fmt.Errorf("failed to write master playlist: %w", err)
		}

		// Add master playlist to files
		masterFile, err := t.createOutputFile(masterPlaylistPath, "application/vnd.apple.mpegurl")
		if err != nil {
			return nil, fmt.Errorf("failed to create master playlist file info: %w", err)
		}
		files = append(files, *masterFile)

		// Transcode each profile
		for i, profile := range output.Profiles {
			slog.Info("Transcoding HLS profile",
				"profile", profile.Name,
				"resolution", fmt.Sprintf("%dx%d", profile.Width, profile.Height),
				"bitrate", profile.VideoBitrateKbps,
			)

			profileFiles, frames, err := t.transcodeHLSProfile(ctx, inputPath, &profile,
				outputDir, inputInfo, output, ffmpegConfig, progressCallback)
			if err != nil {
				return nil, fmt.Errorf("failed to transcode HLS profile '%s': %w", profile.Name, err)
			}

			files = append(files, profileFiles...)
			if i == 0 { // Use first profile for total frame count
				totalFrames = frames
			}
		}
	} else if output.Profile != "" {
		// Single profile output
		slog.Info("Transcoding single HLS profile", "profile", output.Profile)

		// Create a profile config from the single profile name
		// This is a simplified approach - in a real implementation, you might have
		// predefined profiles or derive settings from the profile name
		profile := t.getProfileByName(output.Profile)

		profileFiles, frames, err := t.transcodeHLSProfile(ctx, inputPath, &profile,
			outputDir, inputInfo, output, ffmpegConfig, progressCallback)
		if err != nil {
			return nil, fmt.Errorf("failed to transcode HLS profile '%s': %w", output.Profile, err)
		}

		files = append(files, profileFiles...)
		totalFrames = frames
	} else {
		return nil, fmt.Errorf("no profiles specified for HLS output")
	}

	result := &models.ConversionOutput{
		Name:    output.Name,
		Type:    "hls",
		Profile: output.Profile,
		Files:   files,
		Metadata: map[string]string{
			"package":         "hls",
			"segment_length":  strconv.Itoa(output.SegmentLengthS),
			"total_frames":    strconv.Itoa(totalFrames),
			"processing_time": time.Since(startTime).String(),
		},
	}

	slog.Info("HLS transcoding completed",
		"outputName", output.Name,
		"fileCount", len(files),
		"duration", time.Since(startTime),
	)

	return result, nil
}

// transcodeHLSProfile transcodes a single HLS profile
func (t *Transcoder) transcodeHLSProfile(ctx context.Context, inputPath string,
	profile *config.ProfileConfig, outputDir string, inputInfo *VideoInfo,
	output *config.OutputConfig, ffmpegConfig config.JobFFmpegConfig,
	progressCallback ProgressCallback) ([]models.OutputFile, int, error) {

	// Create profile-specific directory
	profileDir := filepath.Join(outputDir, profile.Name)
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return nil, 0, fmt.Errorf("failed to create profile directory: %w", err)
	}

	// Set default segment length if not specified
	segmentLength := output.SegmentLengthS
	if segmentLength == 0 {
		segmentLength = 6 // Default 6 second segments
	}

	// Build FFmpeg command for HLS
	args := t.buildHLSFFmpegArgs(inputPath, profileDir, profile, segmentLength, ffmpegConfig)

	slog.Debug("Running FFmpeg for HLS",
		"profile", profile.Name,
		"args", strings.Join(args, " "),
	)

	// Run FFmpeg with progress monitoring
	if err := t.runFFmpegWithProgress(ctx, args, inputInfo.TotalFrames, progressCallback); err != nil {
		return nil, 0, fmt.Errorf("ffmpeg execution failed: %w", err)
	}

	// Collect output files
	var files []models.OutputFile

	// Add playlist file
	playlistPath := filepath.Join(profileDir, fmt.Sprintf("%s.m3u8", profile.Name))
	if playlistFile, err := t.createOutputFile(playlistPath, "application/vnd.apple.mpegurl"); err == nil {
		files = append(files, *playlistFile)
	}

	// Add segment files
	segmentPattern := filepath.Join(profileDir, fmt.Sprintf("%s_*.ts", profile.Name))
	segmentFiles, err := filepath.Glob(segmentPattern)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to find segment files: %w", err)
	}

	for _, segmentFile := range segmentFiles {
		if file, err := t.createOutputFile(segmentFile, "video/mp2t"); err == nil {
			files = append(files, *file)
		}
	}

	return files, inputInfo.TotalFrames, nil
}

// buildHLSFFmpegArgs builds FFmpeg arguments for HLS transcoding
func (t *Transcoder) buildHLSFFmpegArgs(inputPath, outputDir string, profile *config.ProfileConfig,
	segmentLength int, ffmpegConfig config.JobFFmpegConfig) []string {

	profileName := profile.Name
	playlistPath := filepath.Join(outputDir, fmt.Sprintf("%s.m3u8", profileName))
	segmentPath := filepath.Join(outputDir, fmt.Sprintf("%s_%%03d.ts", profileName))

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
		"-profile:v", "main",
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

	// HLS-specific settings
	args = append(args,
		"-f", "hls",
		"-hls_time", strconv.Itoa(segmentLength),
		"-hls_list_size", "0",
		"-hls_segment_filename", segmentPath,
		"-hls_flags", "independent_segments",
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
	args = append(args, playlistPath)

	return args
}

// createMasterPlaylist creates an HLS master playlist for multiple profiles
func (t *Transcoder) createMasterPlaylist(profiles []config.ProfileConfig) (string, error) {
	var playlist strings.Builder

	playlist.WriteString("#EXTM3U\n")
	playlist.WriteString("#EXT-X-VERSION:6\n\n")

	for _, profile := range profiles {
		// Calculate bandwidth (video + audio bitrate in bits per second)
		bandwidth := (profile.VideoBitrateKbps + profile.AudioBitrateKbps) * 1000

		playlist.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d,NAME=\"%s\"\n",
			bandwidth, profile.Width, profile.Height, profile.Name))
		playlist.WriteString(fmt.Sprintf("%s/%s.m3u8\n\n", profile.Name, profile.Name))
	}

	return playlist.String(), nil
}

// getProfileByName returns a profile configuration by name (simplified implementation)
func (t *Transcoder) getProfileByName(profileName string) config.ProfileConfig {
	// This is a simplified implementation. In a real system, you would
	// maintain a registry of predefined profiles or parse the profile name
	// to derive settings.

	profiles := map[string]config.ProfileConfig{
		"240p": {
			Name:             "240p",
			Width:            426,
			Height:           240,
			VideoBitrateKbps: 400,
			AudioBitrateKbps: 64,
		},
		"360p": {
			Name:             "360p",
			Width:            640,
			Height:           360,
			VideoBitrateKbps: 800,
			AudioBitrateKbps: 96,
		},
		"480p": {
			Name:             "480p",
			Width:            854,
			Height:           480,
			VideoBitrateKbps: 1200,
			AudioBitrateKbps: 128,
		},
		"720p": {
			Name:             "720p",
			Width:            1280,
			Height:           720,
			VideoBitrateKbps: 2500,
			AudioBitrateKbps: 128,
		},
		"1080p": {
			Name:             "1080p",
			Width:            1920,
			Height:           1080,
			VideoBitrateKbps: 5000,
			AudioBitrateKbps: 128,
		},
	}

	if profile, exists := profiles[profileName]; exists {
		return profile
	}

	// Return default profile if not found
	return profiles["720p"]
}
