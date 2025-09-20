# Video Converter Service - Requirements

This document captures the functional and non-functional requirements, configuration schema, and event contracts for the containerized Video Converter Service (Go + ffmpeg). Use this as the single-source-of-truth as we implement the service.

## 1. Project Overview

**Goal**: Build a containerized service written in Go that listens for upload/convert events, fetches source video files, and uses `ffmpeg` to produce web-optimized output packages (HLS + progressive MP4).

**Key Capabilities**:
- Event-driven architecture supporting Azure Event Grid and WebSocket connections
- Configurable transcoding profiles with adaptive bitrate (ABR) ladder generation
- Container-native deployment with health checks and observability
- Storage-agnostic outputs (Azure Blob, S3, local filesystem)

## 2. Functional Requirements

### 2.1 Event Intake
- **Primary**: Listen for conversion requests from Azure Event Grid
- **Secondary**: Accept events over WebSocket connections for low-latency scenarios
- **Processing**: Validate, deduplicate, and ensure idempotent event handling
- **Correlation**: Support `correlationId` for distributed tracing

### 2.2 Service Configuration
- **Primary**: Environment variables for production deployment
- **Fallback**: `config.yaml` file for local development and testing
- **Helper Library**: Dedicated config library to read and merge environment variables with YAML config
- **Precedence**: Environment variables override YAML file values when both are present

### 2.3 Job Configuration  
- **Format**: Static job templates defined in service configuration (environment variables/YAML)
- **Templates**: Pre-configured conversion profiles with output specifications, ffmpeg settings, and storage destinations
- **Event Mapping**: Events specify source file and reference a job template by name/ID
- **Validation**: Job template validation at service startup

### 2.3 Transcoding Engine
- **Core Engine**: ffmpeg with H.264 (x264) video + AAC audio encoding
- **Output Formats**: HLS with 4-6s segments + progressive MP4 fallback
- **Quality Control**: Limit output resolution to ≤ source resolution
- **Optimization**: Keyframe alignment with segment boundaries

### 2.4 Output Profiles (ABR Ladder)
- **240p**: 426x240 @ 300-400 kbps
- **360p**: 640x360 @ 600-800 kbps  
- **480p**: 854x480 @ ~1.2 Mbps
- **720p**: 1280x720 @ ~2-3 Mbps
- **1080p+**: 1920x1080 and 4K (only if source ≥ target resolution)
- **Packaging**: HLS master playlist at `vod/{videoId}/master.m3u8`
- **Fallback**: Progressive MP4 (720p.mp4) for social sharing

### 2.5 Storage & Output Management
- **Destinations**: Configurable storage backends (Azure Blob, S3-compatible, local)
- **Path Templates**: Support variable substitution (e.g., `{videoId}`, `{profile}`)
- **Cleanup**: Temporary file management and disk space monitoring

## 3. Configuration Schema Reference

### 3.1 Service Configuration (Environment Variables + config.yaml)

The service itself is configured via environment variables (production) with a `config.yaml` fallback for local development. A helper config library handles reading and merging these sources.

**Environment Variables**:
```bash
# Server Configuration
SERVER_PORT=8080
SERVER_HOST=0.0.0.0
SERVER_HEALTH_CHECK_PORT=8081

# Event Sources
EVENT_SOURCES_AZURE_EVENTGRID_ENDPOINT=https://topic.eventgrid.azure.net/
EVENT_SOURCES_AZURE_EVENTGRID_KEY=base64-encoded-key
EVENT_SOURCES_WEBSOCKET_ENDPOINT=wss://events.example.com/converter
EVENT_SOURCES_WEBSOCKET_TOKEN=bearer-token

# Storage Configuration  
STORAGE_TYPE=azure-blob  # azure-blob|s3|local
STORAGE_LOCAL_PATH=/var/lib/video-converter/outputs
STORAGE_AZURE_BLOB_ACCOUNT=myaccount
STORAGE_AZURE_BLOB_CONTAINER=video-outputs
STORAGE_S3_BUCKET=video-outputs
STORAGE_S3_REGION=us-east-1

# Processing Configuration
PROCESSING_MAX_CONCURRENT_JOBS=2
PROCESSING_JOB_TIMEOUT_MINUTES=30
PROCESSING_TEMP_DIR=/tmp/video-converter
PROCESSING_MAX_TEMP_DISK_GB=10

# ffmpeg Configuration
FFMPEG_BINARY_PATH=/usr/bin/ffmpeg
FFMPEG_DEFAULT_PRESET=fast
FFMPEG_HARDWARE_ACCEL=  # vaapi|nvenc|empty

# Job Templates (JSON-encoded - matches config.yaml structure)
JOB_TEMPLATES='{"default":{"outputs":[{"name":"hls-adaptive","package":"hls","profiles":[{"name":"240p","width":426,"height":240,"video_bitrate_kbps":350,"audio_bitrate_kbps":64},{"name":"360p","width":640,"height":360,"video_bitrate_kbps":700,"audio_bitrate_kbps":96},{"name":"720p","width":1280,"height":720,"video_bitrate_kbps":2500,"audio_bitrate_kbps":128},{"name":"1080p","width":1920,"height":1080,"video_bitrate_kbps":4000,"audio_bitrate_kbps":128},{"name":"4k","width":3840,"height":2160,"video_bitrate_kbps":8000,"audio_bitrate_kbps":128}],"segment_length_s":6,"container":"fmp4","destination":"vod/{videoId}/hls/"},{"name":"progressive-fallback","package":"progressive","profile":"720p","destination":"vod/{videoId}/progressive/720p.mp4"},{"name":"progressive-hd","package":"progressive","profile":"1080p","destination":"vod/{videoId}/progressive/1080p.mp4"}],"ffmpeg":{"preset":"fast","hwaccel":"","extra_args":["-movflags","+faststart"]},"notifications":{"webhook_url":"https://api.example.com/conversion-complete","on_complete":true,"on_failure":true}},"social_media":{"outputs":[{"name":"social-optimized","package":"progressive","profiles":[{"name":"480p","width":854,"height":480,"video_bitrate_kbps":1200,"audio_bitrate_kbps":96},{"name":"720p","width":1280,"height":720,"video_bitrate_kbps":2000,"audio_bitrate_kbps":128}],"destination":"social/{videoId}/"}],"ffmpeg":{"preset":"medium","extra_args":["-movflags","+faststart","-pix_fmt","yuv420p"]}},"premium":{"outputs":[{"name":"premium-hls","package":"hls","profiles":[{"name":"480p","width":854,"height":480,"video_bitrate_kbps":1200,"audio_bitrate_kbps":96},{"name":"720p","width":1280,"height":720,"video_bitrate_kbps":3000,"audio_bitrate_kbps":128},{"name":"1080p","width":1920,"height":1080,"video_bitrate_kbps":5000,"audio_bitrate_kbps":128},{"name":"4k","width":3840,"height":2160,"video_bitrate_kbps":12000,"audio_bitrate_kbps":192}],"segment_length_s":4,"container":"fmp4","destination":"premium/{videoId}/hls/"},{"name":"premium-progressive","package":"progressive","profile":"1080p","destination":"premium/{videoId}/progressive/1080p.mp4"}],"ffmpeg":{"preset":"slow","extra_args":["-movflags","+faststart","-tune","film"]}}}'

# Observability
OBSERVABILITY_LOG_LEVEL=info  # debug|info|warn|error
OBSERVABILITY_METRICS_PORT=9090
OBSERVABILITY_ENABLE_TRACING=true
OBSERVABILITY_JAEGER_ENDPOINT=http://jaeger:14268/api/traces
```

**config.yaml (Local Development)**:
```yaml
server:
  port: 8080
  host: "0.0.0.0"
  health_check_port: 8081

event_sources:
  azure_eventgrid:
    endpoint: "https://topic.eventgrid.azure.net/"
    key: "base64-encoded-key"
  websocket:
    endpoint: "ws://localhost:8080/events"
    token: "dev-token"

storage:
  type: "local"  # azure-blob|s3|local
  local:
    path: "./outputs"
  azure_blob:
    account: "devstorageaccount1"
    container: "video-outputs"
  s3:
    bucket: "video-outputs"
    region: "us-east-1"

processing:
  max_concurrent_jobs: 2
  job_timeout_minutes: 30
  temp_dir: "./temp"
  max_temp_disk_gb: 5

ffmpeg:
  binary_path: "ffmpeg"
  default_preset: "fast"
  hardware_accel: ""

observability:
  log_level: "debug"
  metrics_port: 9090
  enable_tracing: false

# Job Templates - Static conversion configurations
job_templates:
  default:
    outputs:
      - name: "hls-adaptive"
        package: "hls"
        profiles:
          - name: "240p"
            width: 426
            height: 240
            video_bitrate_kbps: 350
            audio_bitrate_kbps: 64
          - name: "360p" 
            width: 640
            height: 360
            video_bitrate_kbps: 700
            audio_bitrate_kbps: 96
          - name: "720p"
            width: 1280
            height: 720
            video_bitrate_kbps: 2500
            audio_bitrate_kbps: 128
          - name: "1080p"
            width: 1920
            height: 1080
            video_bitrate_kbps: 4000
            audio_bitrate_kbps: 128
          - name: "4k"
            width: 3840
            height: 2160
            video_bitrate_kbps: 8000
            audio_bitrate_kbps: 128
        segment_length_s: 6
        container: "fmp4"  # fmp4|ts
        destination: "vod/{videoId}/hls/"

      - name: "progressive-fallback"
        package: "progressive" 
        profile: "720p"
        destination: "vod/{videoId}/progressive/720p.mp4"

      - name: "progressive-hd"
        package: "progressive"
        profile: "1080p" 
        destination: "vod/{videoId}/progressive/1080p.mp4"

    ffmpeg:
      preset: "fast"
      hwaccel: ""  # optional: vaapi, nvenc
      extra_args: ["-movflags", "+faststart"]

    notifications:
      webhook_url: "https://api.example.com/conversion-complete"
      on_complete: true
      on_failure: true

  # Additional job templates can be defined
  social_media:
    outputs:
      - name: "social-optimized"
        package: "progressive"
        profiles:
          - name: "480p"
            width: 854
            height: 480
            video_bitrate_kbps: 1200
            audio_bitrate_kbps: 96
          - name: "720p"
            width: 1280
            height: 720
            video_bitrate_kbps: 2000
            audio_bitrate_kbps: 128
        destination: "social/{videoId}/"
    ffmpeg:
      preset: "medium"
      extra_args: ["-movflags", "+faststart", "-pix_fmt", "yuv420p"]

  # High-quality template for premium content
  premium:
    outputs:
      - name: "premium-hls"
        package: "hls"
        profiles:
          - name: "480p"
            width: 854
            height: 480
            video_bitrate_kbps: 1200
            audio_bitrate_kbps: 96
          - name: "720p"
            width: 1280
            height: 720
            video_bitrate_kbps: 3000
            audio_bitrate_kbps: 128
          - name: "1080p"
            width: 1920
            height: 1080
            video_bitrate_kbps: 5000
            audio_bitrate_kbps: 128
          - name: "4k"
            width: 3840
            height: 2160
            video_bitrate_kbps: 12000
            audio_bitrate_kbps: 192
        segment_length_s: 4
        container: "fmp4"
        destination: "premium/{videoId}/hls/"
      
      - name: "premium-progressive"
        package: "progressive"
        profile: "1080p"
        destination: "premium/{videoId}/progressive/1080p.mp4"
    
    ffmpeg:
      preset: "slow"  # Higher quality encoding
      extra_args: ["-movflags", "+faststart", "-tune", "film"]
```

**Config Library Requirements**:
- Load `config.yaml` if present in working directory
- Override with environment variables (precedence: ENV > YAML > defaults)
- Validate required fields and provide clear error messages
- Support nested configuration via environment variable naming (e.g., `STORAGE_TYPE`, `EVENT_SOURCES_AZURE_EVENTGRID_ENDPOINT`, `OBSERVABILITY_LOG_LEVEL`)
- Parse and validate job templates at startup (schema validation, required fields)
- Provide typed configuration struct for Go code consumption

### 3.2 Job Configuration (Event Payloads)

### 3.2 Event Payload Schema

Events specify the source file and reference a predefined job template. The service looks up the template and processes the conversion accordingly.

```yaml
# Simplified event payload - references static job template
jobId: "job-1234"
correlationId: "trace-abc-def"
videoId: "video-abc"
template: "default"  # References job_templates.default from service config

source:
  uri: "https://account.blob.core.windows.net/uploads/source.mp4"
  type: "http"  # http|azure-blob|s3|local
  checksum: "sha256:..."  # optional validation

# Optional metadata (merged with template defaults)
metadata:
  title: "Sample Video"
  description: "User uploaded content"
  tags: ["user-content"]
```

**Processing Flow**:
1. Event received with source URI and template reference
2. Service loads job template from static configuration
3. Template variables (e.g., `{videoId}`) are substituted with event values
4. Conversion proceeds using template-defined outputs, profiles, and ffmpeg settings

## 4. Event Contracts

### 4.1 Azure Event Grid (Blob Created Event)
```json
{
  "id": "event-123",
  "eventType": "Microsoft.Storage.BlobCreated", 
  "subject": "/blobServices/default/containers/uploads/blobs/video.mp4",
  "eventTime": "2025-09-20T12:34:56.789Z",
  "data": {
    "api": "PutBlob",
    "contentType": "video/mp4",
    "url": "https://account.blob.core.windows.net/uploads/video.mp4",
    "eTag": "0x8D..."
  }
}
```

**Mapping Strategy**: Extract `data.url` as source URI, derive `videoId` from blob name or metadata, use default job template (configurable which template to use for blob events).

### 4.2 WebSocket Event Format
```json
{
  "type": "convert-request",
  "correlationId": "trace-456", 
  "jobId": "job-456",
  "videoId": "video-def",
  "template": "default",
  "source": {
    "uri": "https://storage.example.com/uploads/video.mp4",
    "type": "http"
  },
  "metadata": {
    "title": "User Upload",
    "tags": ["user-content"]
  }
}
```

**WebSocket Requirements**:
- TLS encryption mandatory
- Token-based authentication
- Connection keep-alive and reconnection logic

## 5. Technical Architecture

### 5.1 Non-Functional Requirements
- **Concurrency**: Configurable max concurrent ffmpeg processes (default: 2 per container)
- **Resource Limits**: Memory/CPU constraints, per-job timeouts, disk quotas
- **Retry Logic**: Exponential backoff for transient failures, max 3 attempts
- **Observability**: Structured logging (JSON), Prometheus metrics, distributed tracing

### 5.2 Container Requirements
- **Base Image**: Multi-stage build with Go binary + pinned ffmpeg version
- **Health Checks**: `/healthz` (liveness), `/ready` (readiness) endpoints
- **Graceful Shutdown**: SIGTERM handling with job completion timeout
- **Security**: Non-root user, minimal attack surface

### 5.3 ffmpeg Integration
- **Encoding**: x264 for video, AAC for audio (libfdk_aac preferred)
- **GOP Alignment**: Keyframe interval = 2x segment length
- **Quality Control**: Two-pass encoding or CRF-based bitrate targeting
- **Error Handling**: Parse ffmpeg stderr, categorize transient vs permanent failures

## 6. Security & Authentication

### 6.1 Event Grid Integration
- **Subscription Validation**: Handle validation handshake
- **Authentication**: Shared Access Signature or Managed Identity
- **Input Validation**: Schema validation, suspicious payload detection

### 6.2 Storage Access
- **Credentials**: Environment variables or managed identity (never hardcoded)
- **Access Patterns**: Read-only for source, write-only for destination
- **Encryption**: Support for encryption at rest and in transit

## 7. Testing Strategy

### 7.1 Unit Tests
- Configuration parsing and validation
- Event mapping logic
- ffmpeg command generation
- Error handling and retry mechanisms

### 7.2 Integration Tests  
- End-to-end ffmpeg execution with sample video
- HLS playlist and segment validation
- Storage backend interaction
- Event Grid webhook simulation

### 7.3 Acceptance Criteria
- ✅ Process Azure Event Grid blob-created event
- ✅ Generate valid HLS adaptive stream with multiple bitrates
- ✅ Create progressive MP4 fallback
- ✅ Store outputs to configured destination
- ✅ Emit completion notification with job status
- ✅ Handle duplicate events idempotently
- ✅ Gracefully handle invalid/corrupt source files

## 8. Error Handling & Edge Cases

### 8.1 Input Validation Failures
- **Invalid source URI**: Immediate job failure, no retries
- **Unsupported format**: Log error, attempt conversion with warnings
- **Corrupt/damaged files**: ffmpeg error, mark job failed with diagnostics

### 8.2 Resource Constraints
- **Disk space**: Pre-flight checks, fail fast if insufficient
- **Memory limits**: Monitor ffmpeg memory usage, kill if exceeded
- **Network issues**: Retry with exponential backoff for transient failures

### 8.3 Idempotency
- **Duplicate events**: Use `jobId` or source `eTag` for deduplication
- **Partial failures**: Resume from last successful step where possible
- **Output conflicts**: Overwrite policy configurable per job

## 9. Deployment & Operations

### 9.1 Container Orchestration
- **Health checks**: Kubernetes-ready liveness/readiness probes  
- **Resource requests**: CPU/memory requests and limits
- **Scaling**: Horizontal pod autoscaling based on queue depth
- **Monitoring**: Prometheus metrics export on `/metrics`

### 9.2 Observability
- **Metrics**: Job duration, success/failure rates, queue depth, resource utilization
- **Logging**: Structured JSON logs with correlation IDs
- **Tracing**: OpenTelemetry integration for distributed traces
