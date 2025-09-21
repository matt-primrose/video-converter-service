package transcoder

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/matt-primrose/video-converter-service/internal/config"
)

func TestTranscoder_NewTranscoder(t *testing.T) {
	cfg := &config.Config{
		Processing: config.ProcessingConfig{
			TempDir: "/tmp/test-transcoder",
		},
		FFmpeg: config.FFmpegConfig{
			BinaryPath: "ffmpeg",
		},
	}

	transcoder, err := NewTranscoder(cfg)
	if err != nil {
		t.Skipf("Skipping test - FFmpeg not available: %v", err)
		return
	}

	if transcoder == nil {
		t.Error("Expected transcoder to be initialized")
		return
	}

	if transcoder.ffmpegBin != "ffmpeg" {
		t.Errorf("Expected ffmpegBin to be 'ffmpeg', got %s", transcoder.ffmpegBin)
	}

	// Cleanup temp directory
	os.RemoveAll(cfg.Processing.TempDir)
}

func TestTranscoder_GetVideoInfo(t *testing.T) {
	cfg := &config.Config{
		Processing: config.ProcessingConfig{
			TempDir: "/tmp/test-transcoder",
		},
		FFmpeg: config.FFmpegConfig{
			BinaryPath: "ffmpeg",
		},
	}

	transcoder, err := NewTranscoder(cfg)
	if err != nil {
		t.Skipf("Skipping test - FFmpeg not available: %v", err)
	}

	// Create a simple test video file using ffmpeg
	testVideoPath := filepath.Join(cfg.Processing.TempDir, "test-input.mp4")
	if err := os.MkdirAll(cfg.Processing.TempDir, 0755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(cfg.Processing.TempDir)

	// Generate a 5-second test video with ffmpeg
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Skip this test if we can't create a test video
	if err := transcoder.createTestVideo(ctx, testVideoPath); err != nil {
		t.Skipf("Skipping test - cannot create test video: %v", err)
	}

	info, err := transcoder.getVideoInfo(ctx, testVideoPath)
	if err != nil {
		t.Fatalf("Failed to get video info: %v", err)
	}

	if info.Duration <= 0 {
		t.Error("Expected duration to be greater than 0")
	}

	if info.Width <= 0 || info.Height <= 0 {
		t.Errorf("Expected valid dimensions, got %dx%d", info.Width, info.Height)
	}

	if info.Format == "" {
		t.Error("Expected format to be set")
	}

	t.Logf("Video info: %+v", info)
}

func TestParseFrameRate(t *testing.T) {
	tests := []struct {
		input     string
		expected  float64
		tolerance float64
	}{
		{"30/1", 30.0, 0.001},
		{"29.97", 29.97, 0.001},
		{"25/1", 25.0, 0.001},
		{"23.976", 23.976, 0.001},
		{"60000/1001", 59.94, 0.01}, // Allow for floating point precision
		{"", 0.0, 0.001},
		{"invalid", 0.0, 0.001},
	}

	for _, test := range tests {
		result := parseFrameRate(test.input)
		if abs(result-test.expected) > test.tolerance {
			t.Errorf("parseFrameRate(%s) = %f, expected %f (±%f)", test.input, result, test.expected, test.tolerance)
		}
	}
}

// abs returns the absolute value of a float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// createTestVideo creates a simple test video for testing purposes
func (t *Transcoder) createTestVideo(ctx context.Context, outputPath string) error {
	// Create a 5-second test video with a color pattern
	args := []string{
		"-f", "lavfi",
		"-i", "testsrc2=duration=5:size=640x480:rate=30",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-pix_fmt", "yuv420p",
		"-y", outputPath,
	}

	return t.runFFmpegWithProgress(ctx, args, 150, nil) // 5 seconds * 30fps = 150 frames
}
