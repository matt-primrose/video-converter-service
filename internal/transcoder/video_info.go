package transcoder

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// VideoInfo contains information about a video file
type VideoInfo struct {
	Duration    time.Duration `json:"duration"`
	Width       int           `json:"width"`
	Height      int           `json:"height"`
	FrameRate   float64       `json:"frameRate"`
	Bitrate     int64         `json:"bitrate"`
	Size        int64         `json:"size"`
	Format      string        `json:"format"`
	VideoCodec  string        `json:"videoCodec"`
	AudioCodec  string        `json:"audioCodec"`
	TotalFrames int           `json:"totalFrames"`
}

// FFprobeOutput represents the structure of ffprobe JSON output
type FFprobeOutput struct {
	Streams []struct {
		Index      int    `json:"index"`
		CodecName  string `json:"codec_name"`
		CodecType  string `json:"codec_type"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		RFrameRate string `json:"r_frame_rate"`
		Duration   string `json:"duration"`
		BitRate    string `json:"bit_rate"`
		Tags       struct {
			Duration string `json:"DURATION"`
		} `json:"tags"`
	} `json:"streams"`
	Format struct {
		Filename   string `json:"filename"`
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		Size       string `json:"size"`
		BitRate    string `json:"bit_rate"`
		Tags       struct {
			Title string `json:"title"`
		} `json:"tags"`
	} `json:"format"`
}

// getVideoInfo retrieves detailed information about a video file using ffprobe
func (t *Transcoder) getVideoInfo(ctx context.Context, inputPath string) (*VideoInfo, error) {
	// Use ffprobe to get detailed video information
	cmd := exec.CommandContext(ctx, t.ffprobeBin,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		inputPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run ffprobe: %w", err)
	}

	var probe FFprobeOutput
	if err := json.Unmarshal(output, &probe); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	info := &VideoInfo{}

	// Parse format information
	if probe.Format.Duration != "" {
		if duration, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
			info.Duration = time.Duration(duration * float64(time.Second))
		}
	}

	if probe.Format.Size != "" {
		if size, err := strconv.ParseInt(probe.Format.Size, 10, 64); err == nil {
			info.Size = size
		}
	}

	if probe.Format.BitRate != "" {
		if bitrate, err := strconv.ParseInt(probe.Format.BitRate, 10, 64); err == nil {
			info.Bitrate = bitrate
		}
	}

	info.Format = probe.Format.FormatName

	// Parse video stream information
	for _, stream := range probe.Streams {
		if stream.CodecType == "video" {
			info.Width = stream.Width
			info.Height = stream.Height
			info.VideoCodec = stream.CodecName

			// Parse frame rate
			if stream.RFrameRate != "" {
				if frameRate := parseFrameRate(stream.RFrameRate); frameRate > 0 {
					info.FrameRate = frameRate
				}
			}

			// Calculate total frames if we have duration and frame rate
			if info.Duration > 0 && info.FrameRate > 0 {
				info.TotalFrames = int(info.Duration.Seconds() * info.FrameRate)
			}
			break
		}
	}

	// Parse audio stream information
	for _, stream := range probe.Streams {
		if stream.CodecType == "audio" {
			info.AudioCodec = stream.CodecName
			break
		}
	}

	return info, nil
}

// parseFrameRate parses frame rate string like "30/1" or "29.97"
func parseFrameRate(frameRateStr string) float64 {
	if strings.Contains(frameRateStr, "/") {
		parts := strings.Split(frameRateStr, "/")
		if len(parts) == 2 {
			num, err1 := strconv.ParseFloat(parts[0], 64)
			den, err2 := strconv.ParseFloat(parts[1], 64)
			if err1 == nil && err2 == nil && den != 0 {
				return num / den
			}
		}
	} else {
		if rate, err := strconv.ParseFloat(frameRateStr, 64); err == nil {
			return rate
		}
	}
	return 0
}

// ProgressInfo contains progress information from FFmpeg
type ProgressInfo struct {
	Frame    int     `json:"frame"`
	FPS      float64 `json:"fps"`
	Bitrate  string  `json:"bitrate"`
	Size     string  `json:"size"`
	Time     string  `json:"time"`
	Speed    float64 `json:"speed"`
	Progress string  `json:"progress"`
}

// parseProgress parses FFmpeg progress output
func parseProgress(line string) *ProgressInfo {
	// FFmpeg progress format: frame=  123 fps= 25 q=28.0 size=    1024kB time=00:00:05.12 bitrate= 164.2kbits/s speed=1.02x
	re := regexp.MustCompile(`frame=\s*(\d+).*fps=\s*([\d.]+).*size=\s*(\S+).*time=(\S+).*bitrate=\s*(\S+).*speed=\s*([\d.]+)x`)
	matches := re.FindStringSubmatch(line)

	if len(matches) < 7 {
		return nil
	}

	info := &ProgressInfo{
		Bitrate: matches[5],
		Size:    matches[3],
		Time:    matches[4],
	}

	if frame, err := strconv.Atoi(matches[1]); err == nil {
		info.Frame = frame
	}

	if fps, err := strconv.ParseFloat(matches[2], 64); err == nil {
		info.FPS = fps
	}

	if speed, err := strconv.ParseFloat(matches[6], 64); err == nil {
		info.Speed = speed
	}

	return info
}

// runFFmpegWithProgress runs FFmpeg command and monitors progress
func (t *Transcoder) runFFmpegWithProgress(ctx context.Context, args []string,
	totalFrames int, progressCallback ProgressCallback) error {

	cmd := exec.CommandContext(ctx, t.ffmpegBin, args...)

	// Get stderr pipe to read progress
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Monitor progress
	scanner := bufio.NewScanner(stderr)
	go func() {
		for scanner.Scan() {
			line := scanner.Text()

			// Parse progress information
			if progress := parseProgress(line); progress != nil && progressCallback != nil {
				var progressPercent float64
				if totalFrames > 0 {
					progressPercent = float64(progress.Frame) / float64(totalFrames)
				}
				progressCallback(progressPercent, progress.Frame, totalFrames, progress.Speed)
			}
		}
	}()

	return cmd.Wait()
}
