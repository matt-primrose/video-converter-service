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
│   │   └── worker.go          # Worker pool and job execution
│   ├── storage/               # Storage abstraction layer (placeholder)
│   └── transcoder/            # ffmpeg integration (placeholder)
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

### Next Steps
- [ ] Implement ffmpeg transcoding logic
- [ ] Add storage backends (Azure Blob, S3)
- [ ] WebSocket event listener
- [ ] Metrics and tracing integration
- [ ] Unit and integration tests

## License

See LICENSE file for details.