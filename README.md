# Video Converter Service

A containerized Go service that converts videos using ffmpeg, designed for cloud deployment with Azure Event Grid and WebSocket event support.

## Features

- 🎬 **Video Transcoding**: Converts videos using ffmpeg with configurable quality profiles
- 🌐 **Event-Driven**: Listens to Azure Event Grid and WebSocket events
- 📦 **Containerized**: Docker-ready with health checks and observability
- ⚡ **High Performance**: Concurrent job processing with configurable limits
- 📊 **Observable**: Structured logging, metrics, and distributed tracing support
- 🔧 **Configurable**: Environment variables + YAML configuration

## Quick Start

### Prerequisites

- Go 1.24+
- ffmpeg installed (for local testing)
- Docker (for containerized deployment)

### Local Development

1. **Clone and build:**
   ```bash
   git clone https://github.com/matt-primrose/video-converter-service.git
   cd video-converter-service
   go build -o bin/video-converter ./cmd/video-converter
   ```

2. **Create local config:**
   ```bash
   cp config.yaml.example config.yaml
   # Edit config.yaml as needed
   ```

3. **Run the service:**
   ```bash
   ./bin/video-converter
   ```

The service will start with:
- Main server: `http://localhost:8080`
- Health checks: `http://localhost:8081`
- Metrics: `http://localhost:9090/metrics`

### Local Testing & Development

The service includes built-in test modes for development and debugging:

#### Test Command Usage
```bash
# Run in test mode
go run cmd/video-converter/main.go -test [flags]

# Available test types:
# - direct: Direct FFmpeg transcoding test
# - worker: Full worker pipeline test
# - upload: Copy files from temp to outputs
# - create-video: Generate test video files
```

#### Test Examples

**Direct Transcoding Test:**
```bash
# Test direct 720p transcoding
go run cmd/video-converter/main.go -test -test-type direct -input "./test-videos/sample.mp4"

# Custom output location
go run cmd/video-converter/main.go -test -test-type direct -input "./test-videos/sample.mp4" -output "./outputs/test.mp4"
```

**Worker Pipeline Test:**
```bash
# Test full worker processing (default 30s wait)
go run cmd/video-converter/main.go -test -test-type worker

# Custom job file and wait time
go run cmd/video-converter/main.go -test -test-type worker -job "examples/job.json" -wait 45s
```

**File Upload Test:**
```bash
# Copy transcoded files from temp to outputs directory
go run cmd/video-converter/main.go -test -test-type upload
```

**Create Test Videos:**
```bash
# Create 4K test video (30 seconds)
go run cmd/video-converter/main.go -test -test-type create-video -input "./test-videos/4k-sample.mp4"

# Create custom test video
go run cmd/video-converter/main.go -test -test-type create-video \
  -input "./test-videos/small.mp4" \
  -video-length 10s \
  -video-res 1280x720
```

#### Test Flags
- `-test`: Enable test mode
- `-test-type`: Test type (direct, worker, upload, create-video)
- `-input`: Input video file path
- `-output`: Output file path (direct mode only)
- `-job`: Job configuration file (default: examples/job.json)
- `-wait`: Wait time for worker jobs (default: 30s)
- `-video-length`: Duration for created test videos (default: 30s)  
- `-video-res`: Resolution for test videos (default: 3840x2160)
- `-log-level`: Override log level (debug, info, warn, error)

#### Test Video Files

The service expects test videos in the `test-videos/` directory. The example job configuration references `test-videos/sample.mp4`.

You can use any MP4, MOV, AVI, or other video format that FFmpeg supports. If you don't have test videos, you can create them using the built-in test mode:

```bash
# Create a 4K test video (30 seconds)
go run cmd/video-converter/main.go -test -test-type create-video -input "./test-videos/sample.mp4"

# Create a smaller test video for faster testing
go run cmd/video-converter/main.go -test -test-type create-video \
  -input "./test-videos/small-test.mp4" \
  -video-length 10s \
  -video-res 1280x720
```

The generated test videos include:
- Colorful test patterns with moving elements
- 30fps frame rate
- High quality encoding for good source material
- Perfect for testing all transcoding profiles

### Docker Deployment

#### Quick Start with Docker Compose
```bash
# Build and start the service
make build
make up

# View logs
make logs

# Stop the service
make down
```

#### Development with Live Config
```bash
# Copy example config and customize
cp config.yaml.example config.yaml
# Edit config.yaml as needed

# Start with local config mounted
docker-compose -f docker-compose.yml -f docker-compose.dev.yml up -d
```

#### Full Stack with Monitoring
```bash
# Start with Prometheus and Grafana
make up-monitoring

# Or start everything (includes Jaeger tracing and MinIO S3)
make up-full
```

**Service URLs:**
- Video Converter API: http://localhost:8080
- Health Checks: http://localhost:8081
- Metrics: http://localhost:9090/metrics
- Prometheus: http://localhost:9091 (monitoring profile)
- Grafana: http://localhost:3000 (monitoring profile, admin/admin)
- Jaeger UI: http://localhost:16686 (tracing profile)
- MinIO Console: http://localhost:9001 (s3-storage profile)

#### Manual Docker Commands
```bash
# Build image
docker build -t video-converter-service .

# Run container
docker run -d \
  --name video-converter \
  -p 8080:8080 \
  -p 8081:8081 \
  -p 9090:9090 \
  -e STORAGE_TYPE=local \
  -e STORAGE_LOCAL_PATH=/app/outputs \
  video-converter-service
```

## Configuration

The service is configured via environment variables (production) with `config.yaml` fallback for local development.

### Key Environment Variables

```bash
# Server
SERVER_PORT=8080
SERVER_HOST=0.0.0.0
SERVER_HEALTH_CHECK_PORT=8081

# Storage
STORAGE_TYPE=local  # local|azure-blob|s3
STORAGE_LOCAL_PATH=/app/outputs

# Processing
PROCESSING_MAX_CONCURRENT_JOBS=2
PROCESSING_JOB_TIMEOUT_MINUTES=30

# Observability
OBSERVABILITY_LOG_LEVEL=info
OBSERVABILITY_METRICS_PORT=9090
```

See `config.yaml.example` for the complete configuration structure.

## API Endpoints

### Health Checks
- `GET /healthz` - Liveness probe
- `GET /ready` - Readiness probe
- `GET /status` - Service status

### Events
- `POST /eventgrid` - Azure Event Grid webhook endpoint
- `WS /events` - WebSocket events (TODO)

### Metrics
- `GET /metrics` - Prometheus metrics

## Job Templates

The service uses pre-configured job templates defined in configuration. Example templates:

- **`default`**: Standard ABR ladder (240p-4K) + HLS + progressive fallback
- **`social_media`**: Social media optimized (480p, 720p progressive)
- **`premium`**: High-quality encoding with premium bitrates

## Event Processing

### Azure Event Grid
Listens for `Microsoft.Storage.BlobCreated` events and automatically converts uploaded videos using the configured job template.

### WebSocket (Coming Soon)
Real-time event processing for low-latency scenarios.

## Development

### Make Commands

The project includes a `Makefile` with common Docker operations:

```bash
make help           # Show all available commands
make build          # Build the video converter service image
make up             # Start the video converter service
make up-monitoring  # Start service with Prometheus and Grafana
make up-full        # Start service with all optional components
make down           # Stop and remove all containers
make logs           # Follow service logs
make clean          # Remove all containers, networks, and volumes
make test-videos    # Create test video directory structure
make dev            # Quick development cycle (build + up + logs)
```

### Testing Video Conversion

1. **Prepare test videos:**
   ```bash
   make test-videos
   # Place your test video files in ./test-videos/
   ```

2. **Submit a conversion job via API** (when WebSocket is implemented):
   ```bash
   curl -X POST http://localhost:8080/jobs \
     -H "Content-Type: application/json" \
     -d '{
       "jobId": "test-001",
       "videoId": "sample-video",
       "template": "default",
       "source": {
         "uri": "/app/test-videos/sample.mp4",
         "type": "local"
       }
     }'
   ```

### Project Structure
```
video-converter-service/
├── cmd/
│   └── video-converter/        # Main application entry point
│       └── main.go            # Entry point and service initialization
├── internal/                  # Private application packages
│   ├── config/                # Configuration loading and validation
│   │   └── config.go          # Config structs and loading logic
│   ├── events/                # Event routing (Event Grid, WebSocket)
│   │   └── router.go          # Azure Event Grid and WebSocket routing
│   ├── worker/                # Job processing and worker pool
│   │   ├── worker.go          # Worker pool and job execution
│   │   └── helpers.go         # Download, upload, and notification helpers
│   ├── storage/               # Storage abstraction layer (placeholder)
│   └── transcoder/            # ffmpeg integration and video processing
│       ├── transcoder.go      # Main transcoder interface and job orchestration
│       ├── video_info.go      # Video analysis and metadata extraction
│       ├── hls.go             # HLS adaptive bitrate streaming output
│       ├── progressive.go     # Progressive MP4 download output
│       └── utils.go           # File utilities and checksum calculation
├── pkg/                       # Public packages
│   └── models/                # Shared data models
│       └── models.go          # Job, event, and result types
├── monitoring/                # Observability configuration
│   ├── grafana/               # Grafana dashboards and config
│   └── prometheus.yml         # Prometheus scraping configuration
├── bin/                       # Compiled binaries (generated)
├── test-videos/               # Sample test files
├── config.yaml.example        # Example configuration
├── config.yaml               # Local configuration (gitignored)
├── docker-compose.yml         # Production container setup
├── docker-compose.dev.yml     # Development environment with monitoring
├── Dockerfile                 # Multi-stage container build
├── Makefile                   # Development and build commands
├── go.mod                     # Go module definition
├── go.sum                     # Go module checksums
├── .gitignore                 # Git ignore rules
├── REQUIREMENTS.md            # Detailed project requirements
└── README.md                  # This file
```

## Video Transcoding Implementation

The service implements a comprehensive video transcoding pipeline using FFmpeg with support for multiple output formats and adaptive bitrate streaming.

### Supported Output Formats

#### HLS (HTTP Live Streaming)
- **Adaptive Bitrate**: Multiple quality profiles with automatic switching
- **Segmented Output**: Configurable segment duration (default 6 seconds)
- **Master Playlist**: Automatic generation for multi-bitrate streams
- **Web Optimized**: Ready for HTML5 video players and CDN delivery

#### Progressive MP4
- **Universal Compatibility**: Works with all modern browsers and devices
- **Fast Start**: Optimized for progressive download with `faststart` flag
- **Multi-Profile**: Generate multiple quality variants simultaneously
- **Container Support**: MP4, WebM, MOV, AVI, MKV formats

### Transcoding Features

#### Video Processing
- **Resolution Scaling**: Automatic scaling with aspect ratio preservation
- **Bitrate Control**: CBR, VBR, and CRF encoding modes
- **Quality Profiles**: Predefined profiles (240p to 4K) with optimal settings
- **Hardware Acceleration**: Support for NVIDIA NVENC, Intel QSV, AMD VCE
- **Advanced Encoding**: H.264 High profile, HEVC/H.265 support planned

#### Audio Processing
- **AAC Encoding**: High-quality AAC audio with configurable bitrates
- **Multi-Channel**: Stereo and surround sound support
- **Audio Normalization**: Consistent audio levels across outputs
- **Format Conversion**: Automatic audio format conversion when needed

#### Progress Monitoring
- **Real-time Progress**: Frame-by-frame progress reporting with speed metrics
- **Status Updates**: Live progress updates via callback functions
- **Error Handling**: Detailed error reporting with context
- **Resource Monitoring**: Memory and disk usage tracking

### Configuration Examples

#### Basic Web Streaming (HLS + Progressive)
```yaml
job_templates:
  web_streaming:
    outputs:
      - name: "hls_adaptive"
        package: "hls"
        segment_length_s: 6
        profiles:
          - name: "480p"
            width: 854
            height: 480
            video_bitrate_kbps: 1200
            audio_bitrate_kbps: 128
          - name: "720p"
            width: 1280
            height: 720
            video_bitrate_kbps: 2500
            audio_bitrate_kbps: 128
          - name: "1080p"
            width: 1920
            height: 1080
            video_bitrate_kbps: 5000
            audio_bitrate_kbps: 192
      - name: "mp4_fallback"
        package: "progressive"
        container: "mp4"
        profile: "720p"
    ffmpeg:
      preset: "medium"
      hwaccel: "nvenc"
      extra_args: ["-g", "60", "-keyint_min", "60"]
```

#### Social Media Optimization
```yaml
job_templates:
  social_media:
    outputs:
      - name: "instagram_story"
        package: "progressive"
        container: "mp4"
        profiles:
          - name: "vertical_hd"
            width: 1080
            height: 1920  # 9:16 aspect ratio
            video_bitrate_kbps: 3000
            audio_bitrate_kbps: 128
    ffmpeg:
      preset: "fast"
      extra_args: ["-profile:v", "high", "-level", "4.0"]
```

#### 4K/UHD Processing
```yaml
job_templates:
  uhd_processing:
    outputs:
      - name: "4k_hls"
        package: "hls"
        profiles:
          - name: "4k"
            width: 3840
            height: 2160
            video_bitrate_kbps: 18000
            audio_bitrate_kbps: 256
    ffmpeg:
      preset: "slow"
      hwaccel: "nvenc"
      extra_args: ["-cq:v", "19", "-rc:v", "vbr"]
```

### Processing Pipeline

1. **Job Initialization**: Parse job configuration and validate templates
2. **Source Download**: Download from HTTP, local file, Azure Blob, or S3
3. **Video Analysis**: Extract metadata (resolution, duration, codecs, bitrate)
4. **Profile Generation**: Create encoding profiles based on templates
5. **Transcoding**: Execute FFmpeg with progress monitoring
6. **Output Validation**: Verify output files and calculate checksums
7. **Upload**: Upload processed files to configured storage
8. **Notification**: Send completion/failure webhooks if configured
9. **Cleanup**: Remove temporary files and free resources

### Performance Optimizations

#### Hardware Acceleration
- **NVIDIA NVENC**: GPU-accelerated encoding for NVIDIA cards
- **Intel Quick Sync**: Hardware encoding on Intel CPUs with iGPU
- **AMD VCE**: AMD GPU acceleration support
- **Apple VideoToolbox**: Hardware encoding on macOS (planned)

#### Encoding Settings
- **Preset Optimization**: Fast/medium/slow presets for speed vs quality balance
- **Two-Pass Encoding**: Optional two-pass encoding for optimal bitrate control
- **Scene Detection**: Automatic keyframe insertion on scene changes
- **Rate Control**: CBR for streaming, VBR for file delivery

#### Resource Management
- **Concurrent Processing**: Multiple jobs with configurable limits
- **Memory Management**: Automatic cleanup and garbage collection
- **Disk Usage**: Temporary file management with size limits
- **Process Isolation**: Each job runs in isolated context with timeouts

### Error Handling

#### Input Validation
- **Format Support**: Comprehensive format detection and validation
- **Codec Compatibility**: Automatic codec detection and conversion
- **Resolution Limits**: Configurable maximum input/output resolutions
- **File Size Limits**: Protection against oversized inputs

#### Processing Errors
- **FFmpeg Error Parsing**: Detailed error message extraction
- **Retry Logic**: Configurable retry attempts for transient failures
- **Graceful Degradation**: Fallback to lower quality on resource constraints
- **Progress Recovery**: Resume interrupted jobs when possible

### Testing and Validation

The transcoder includes comprehensive test coverage:

```bash
# Run transcoder tests
go test ./internal/transcoder -v

# Test with sample video files
make test-videos
./bin/video-converter &
curl -X POST http://localhost:8080/jobs -H "Content-Type: application/json" -d @examples/job.json
```

See `config-examples.yaml` for complete configuration examples and `internal/transcoder/transcoder_test.go` for test cases.

### Next Steps
- [x] ~~Implement ffmpeg transcoding logic~~ ✅ **Complete**
- [ ] Add storage backends (Azure Blob, S3)
- [ ] WebSocket event listener
- [ ] Metrics and tracing integration
- [ ] Unit and integration tests
