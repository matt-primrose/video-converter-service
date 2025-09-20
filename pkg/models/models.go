package models

import "time"

// ConversionJob represents a video conversion job
type ConversionJob struct {
	JobID         string            `json:"jobId"`
	CorrelationID string            `json:"correlationId,omitempty"`
	VideoID       string            `json:"videoId"`
	Template      string            `json:"template"`
	Source        SourceConfig      `json:"source"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	CreatedAt     time.Time         `json:"createdAt"`
	Status        JobStatus         `json:"status"`
}

// SourceConfig represents the source file configuration
type SourceConfig struct {
	URI      string `json:"uri"`
	Type     string `json:"type"` // http, azure-blob, s3, local
	Checksum string `json:"checksum,omitempty"`
}

// JobStatus represents the current status of a job
type JobStatus struct {
	State       JobState  `json:"state"`
	Message     string    `json:"message,omitempty"`
	Progress    float64   `json:"progress"` // 0.0 to 1.0
	StartedAt   time.Time `json:"startedAt,omitempty"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
	Error       string    `json:"error,omitempty"`
}

// JobState represents the possible states of a conversion job
type JobState string

const (
	JobStatePending    JobState = "pending"
	JobStateProcessing JobState = "processing"
	JobStateCompleted  JobState = "completed"
	JobStateFailed     JobState = "failed"
	JobStateCancelled  JobState = "cancelled"
)

// ConversionResult represents the result of a completed conversion
type ConversionResult struct {
	JobID      string               `json:"jobId"`
	VideoID    string               `json:"videoId"`
	Outputs    []ConversionOutput   `json:"outputs"`
	Duration   time.Duration        `json:"duration"`
	Statistics ConversionStatistics `json:"statistics"`
	CreatedAt  time.Time            `json:"createdAt"`
}

// ConversionOutput represents a single output from the conversion process
type ConversionOutput struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"` // hls, progressive
	Profile  string            `json:"profile,omitempty"`
	Files    []OutputFile      `json:"files"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// OutputFile represents a single output file
type OutputFile struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Checksum string `json:"checksum,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// ConversionStatistics contains statistics about the conversion process
type ConversionStatistics struct {
	SourceFileSize    int64         `json:"sourceFileSize"`
	TotalOutputSize   int64         `json:"totalOutputSize"`
	ProcessingTime    time.Duration `json:"processingTime"`
	DownloadTime      time.Duration `json:"downloadTime"`
	UploadTime        time.Duration `json:"uploadTime"`
	FFmpegTime        time.Duration `json:"ffmpegTime"`
	ProfilesProcessed int           `json:"profilesProcessed"`
}

// EventGridEvent represents an Azure Event Grid event
type EventGridEvent struct {
	ID          string                 `json:"id"`
	EventType   string                 `json:"eventType"`
	Subject     string                 `json:"subject"`
	EventTime   time.Time              `json:"eventTime"`
	Data        map[string]interface{} `json:"data"`
	DataVersion string                 `json:"dataVersion"`
}

// WebSocketEvent represents an event received via WebSocket
type WebSocketEvent struct {
	Type          string        `json:"type"`
	CorrelationID string        `json:"correlationId,omitempty"`
	Job           ConversionJob `json:"job,omitempty"`
	Timestamp     time.Time     `json:"timestamp"`
}
