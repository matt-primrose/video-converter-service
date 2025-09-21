package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the complete service configuration
type Config struct {
	Server        ServerConfig        `yaml:"server" json:"server"`
	EventSources  EventSourcesConfig  `yaml:"event_sources" json:"event_sources"`
	Storage       StorageConfig       `yaml:"storage" json:"storage"`
	Processing    ProcessingConfig    `yaml:"processing" json:"processing"`
	FFmpeg        FFmpegConfig        `yaml:"ffmpeg" json:"ffmpeg"`
	JobTemplates  JobTemplatesConfig  `yaml:"job_templates" json:"job_templates"`
	Observability ObservabilityConfig `yaml:"observability" json:"observability"`
}

type ServerConfig struct {
	Port            int    `yaml:"port" json:"port"`
	Host            string `yaml:"host" json:"host"`
	HealthCheckPort int    `yaml:"health_check_port" json:"health_check_port"`
}

type EventSourcesConfig struct {
	AzureEventGrid AzureEventGridConfig `yaml:"azure_eventgrid" json:"azure_eventgrid"`
	WebSocket      WebSocketConfig      `yaml:"websocket" json:"websocket"`
}

type AzureEventGridConfig struct {
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	Key      string `yaml:"key" json:"key"`
}

type WebSocketConfig struct {
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	Token    string `yaml:"token" json:"token"`
}

type StorageConfig struct {
	Type      string           `yaml:"type" json:"type"`
	Local     LocalStorage     `yaml:"local" json:"local"`
	AzureBlob AzureBlobStorage `yaml:"azure_blob" json:"azure_blob"`
	S3        S3Storage        `yaml:"s3" json:"s3"`
}

type LocalStorage struct {
	Path string `yaml:"path" json:"path"`
}

type AzureBlobStorage struct {
	Account   string `yaml:"account" json:"account"`
	Container string `yaml:"container" json:"container"`
}

type S3Storage struct {
	Bucket string `yaml:"bucket" json:"bucket"`
	Region string `yaml:"region" json:"region"`
}

type ProcessingConfig struct {
	MaxConcurrentJobs int    `yaml:"max_concurrent_jobs" json:"max_concurrent_jobs"`
	JobTimeoutMinutes int    `yaml:"job_timeout_minutes" json:"job_timeout_minutes"`
	TempDir           string `yaml:"temp_dir" json:"temp_dir"`
	MaxTempDiskGB     int    `yaml:"max_temp_disk_gb" json:"max_temp_disk_gb"`
}

type FFmpegConfig struct {
	BinaryPath    string `yaml:"binary_path" json:"binary_path"`
	ProbePath     string `yaml:"probe_path" json:"probe_path"`
	DefaultPreset string `yaml:"default_preset" json:"default_preset"`
	HardwareAccel string `yaml:"hardware_accel" json:"hardware_accel"`
}

type ObservabilityConfig struct {
	LogLevel       string `yaml:"log_level" json:"log_level"`
	MetricsPort    int    `yaml:"metrics_port" json:"metrics_port"`
	EnableTracing  bool   `yaml:"enable_tracing" json:"enable_tracing"`
	JaegerEndpoint string `yaml:"jaeger_endpoint" json:"jaeger_endpoint"`
}

// JobTemplatesConfig holds the job templates
type JobTemplatesConfig map[string]JobTemplate

type JobTemplate struct {
	Outputs       []OutputConfig     `yaml:"outputs" json:"outputs"`
	FFmpeg        JobFFmpegConfig    `yaml:"ffmpeg" json:"ffmpeg"`
	Notifications NotificationConfig `yaml:"notifications" json:"notifications"`
}

type OutputConfig struct {
	Name           string          `yaml:"name" json:"name"`
	Package        string          `yaml:"package" json:"package"`
	Profiles       []ProfileConfig `yaml:"profiles" json:"profiles"`
	Profile        string          `yaml:"profile" json:"profile"` // For single profile outputs
	SegmentLengthS int             `yaml:"segment_length_s" json:"segment_length_s"`
	Container      string          `yaml:"container" json:"container"`
	Destination    string          `yaml:"destination" json:"destination"`
}

type ProfileConfig struct {
	Name             string `yaml:"name" json:"name"`
	Width            int    `yaml:"width" json:"width"`
	Height           int    `yaml:"height" json:"height"`
	VideoBitrateKbps int    `yaml:"video_bitrate_kbps" json:"video_bitrate_kbps"`
	AudioBitrateKbps int    `yaml:"audio_bitrate_kbps" json:"audio_bitrate_kbps"`
}

type JobFFmpegConfig struct {
	Preset    string   `yaml:"preset" json:"preset"`
	HWAccel   string   `yaml:"hwaccel" json:"hwaccel"`
	ExtraArgs []string `yaml:"extra_args" json:"extra_args"`
}

type NotificationConfig struct {
	WebhookURL string `yaml:"webhook_url" json:"webhook_url"`
	OnComplete bool   `yaml:"on_complete" json:"on_complete"`
	OnFailure  bool   `yaml:"on_failure" json:"on_failure"`
}

// Load loads configuration from environment variables and config.yaml file
func Load() (*Config, error) {
	// Default configuration
	cfg := &Config{
		Server: ServerConfig{
			Port:            8080,
			Host:            "0.0.0.0",
			HealthCheckPort: 8081,
		},
		Processing: ProcessingConfig{
			MaxConcurrentJobs: 2,
			JobTimeoutMinutes: 60, // Increased default for longer video processing
			TempDir:           "/tmp/video-converter",
			MaxTempDiskGB:     10,
		},
		FFmpeg: FFmpegConfig{
			BinaryPath:    "ffmpeg",
			ProbePath:     "ffprobe",
			DefaultPreset: "fast",
		},
		Observability: ObservabilityConfig{
			LogLevel:    "info",
			MetricsPort: 9090,
		},
	}

	// Load from config.yaml if present
	if _, err := os.Stat("config.yaml"); err == nil {
		yamlFile, err := os.ReadFile("config.yaml")
		if err != nil {
			return nil, fmt.Errorf("error reading config.yaml: %w", err)
		}

		if err := yaml.Unmarshal(yamlFile, cfg); err != nil {
			return nil, fmt.Errorf("error parsing config.yaml: %w", err)
		}
	}

	// Override with environment variables
	loadFromEnv(cfg)

	// Validate configuration
	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// loadFromEnv loads configuration from environment variables
func loadFromEnv(cfg *Config) {
	// Server config
	if val := os.Getenv("SERVER_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			cfg.Server.Port = port
		}
	}
	if val := os.Getenv("SERVER_HOST"); val != "" {
		cfg.Server.Host = val
	}
	if val := os.Getenv("SERVER_HEALTH_CHECK_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			cfg.Server.HealthCheckPort = port
		}
	}

	// Event sources
	if val := os.Getenv("EVENT_SOURCES_AZURE_EVENTGRID_ENDPOINT"); val != "" {
		cfg.EventSources.AzureEventGrid.Endpoint = val
	}
	if val := os.Getenv("EVENT_SOURCES_AZURE_EVENTGRID_KEY"); val != "" {
		cfg.EventSources.AzureEventGrid.Key = val
	}
	if val := os.Getenv("EVENT_SOURCES_WEBSOCKET_ENDPOINT"); val != "" {
		cfg.EventSources.WebSocket.Endpoint = val
	}
	if val := os.Getenv("EVENT_SOURCES_WEBSOCKET_TOKEN"); val != "" {
		cfg.EventSources.WebSocket.Token = val
	}

	// Storage config
	if val := os.Getenv("STORAGE_TYPE"); val != "" {
		cfg.Storage.Type = val
	}
	if val := os.Getenv("STORAGE_LOCAL_PATH"); val != "" {
		cfg.Storage.Local.Path = val
	}
	if val := os.Getenv("STORAGE_AZURE_BLOB_ACCOUNT"); val != "" {
		cfg.Storage.AzureBlob.Account = val
	}
	if val := os.Getenv("STORAGE_AZURE_BLOB_CONTAINER"); val != "" {
		cfg.Storage.AzureBlob.Container = val
	}
	if val := os.Getenv("STORAGE_S3_BUCKET"); val != "" {
		cfg.Storage.S3.Bucket = val
	}
	if val := os.Getenv("STORAGE_S3_REGION"); val != "" {
		cfg.Storage.S3.Region = val
	}

	// Processing config
	if val := os.Getenv("PROCESSING_MAX_CONCURRENT_JOBS"); val != "" {
		if jobs, err := strconv.Atoi(val); err == nil {
			cfg.Processing.MaxConcurrentJobs = jobs
		}
	}
	if val := os.Getenv("PROCESSING_JOB_TIMEOUT_MINUTES"); val != "" {
		if timeout, err := strconv.Atoi(val); err == nil {
			cfg.Processing.JobTimeoutMinutes = timeout
		}
	}
	if val := os.Getenv("PROCESSING_TEMP_DIR"); val != "" {
		cfg.Processing.TempDir = val
	}
	if val := os.Getenv("PROCESSING_MAX_TEMP_DISK_GB"); val != "" {
		if size, err := strconv.Atoi(val); err == nil {
			cfg.Processing.MaxTempDiskGB = size
		}
	}

	// FFmpeg config
	if val := os.Getenv("FFMPEG_BINARY_PATH"); val != "" {
		cfg.FFmpeg.BinaryPath = val
	}
	if val := os.Getenv("FFMPEG_PROBE_PATH"); val != "" {
		cfg.FFmpeg.ProbePath = val
	}
	if val := os.Getenv("FFMPEG_DEFAULT_PRESET"); val != "" {
		cfg.FFmpeg.DefaultPreset = val
	}
	if val := os.Getenv("FFMPEG_HARDWARE_ACCEL"); val != "" {
		cfg.FFmpeg.HardwareAccel = val
	}

	// Job templates (JSON)
	if val := os.Getenv("JOB_TEMPLATES"); val != "" {
		var templates JobTemplatesConfig
		if err := json.Unmarshal([]byte(val), &templates); err == nil {
			cfg.JobTemplates = templates
		}
	}

	// Observability config
	if val := os.Getenv("OBSERVABILITY_LOG_LEVEL"); val != "" {
		cfg.Observability.LogLevel = strings.ToLower(val)
	}
	if val := os.Getenv("OBSERVABILITY_METRICS_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			cfg.Observability.MetricsPort = port
		}
	}
	if val := os.Getenv("OBSERVABILITY_ENABLE_TRACING"); val != "" {
		cfg.Observability.EnableTracing = strings.ToLower(val) == "true"
	}
	if val := os.Getenv("OBSERVABILITY_JAEGER_ENDPOINT"); val != "" {
		cfg.Observability.JaegerEndpoint = val
	}
}

// validate performs basic configuration validation
func validate(cfg *Config) error {
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", cfg.Server.Port)
	}

	if cfg.Server.HealthCheckPort <= 0 || cfg.Server.HealthCheckPort > 65535 {
		return fmt.Errorf("invalid health check port: %d", cfg.Server.HealthCheckPort)
	}

	if cfg.Processing.MaxConcurrentJobs <= 0 {
		return fmt.Errorf("max concurrent jobs must be positive: %d", cfg.Processing.MaxConcurrentJobs)
	}

	if cfg.Storage.Type == "" {
		return fmt.Errorf("storage type is required")
	}

	validStorageTypes := []string{"local", "azure-blob", "s3"}
	valid := false
	for _, t := range validStorageTypes {
		if cfg.Storage.Type == t {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid storage type: %s", cfg.Storage.Type)
	}

	validLogLevels := []string{"debug", "info", "warn", "error"}
	valid = false
	for _, l := range validLogLevels {
		if cfg.Observability.LogLevel == l {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid log level: %s", cfg.Observability.LogLevel)
	}

	return nil
}
