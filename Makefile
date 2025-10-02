.PHONY: build up down logs clean test-videos check-ffmpeg help

# Default target
help:
	@echo "Video Converter Service - Docker Compose Commands"
	@echo ""
	@echo "Available commands:"
	@echo "  build         - Build the video converter service image"
	@echo "  up            - Start the video converter service"
	@echo "  up-monitoring - Start service with Prometheus and Grafana"
	@echo "  up-full       - Start service with all optional components"
	@echo "  down          - Stop and remove all containers"
	@echo "  logs          - Follow service logs"
	@echo "  clean         - Remove all containers, networks, and volumes"
	@echo "  test-videos   - Create video_source directory for test videos"
	@echo "  check-ffmpeg  - Check ffmpeg capabilities in container"
	@echo ""
	@echo "Service URLs (when running):"
	@echo "  Video Converter API:  http://localhost:8080"
	@echo "  Health Checks:        http://localhost:8081"

# Build the service
build:
	docker-compose build video-converter

# Start just the video converter service
up:
	docker-compose up -d video-converter

# Start with monitoring stack (Prometheus + Grafana)
up-monitoring:
	docker-compose --profile monitoring up -d

# Start with all optional components
up-full:
	docker-compose --profile monitoring --profile tracing --profile s3-storage up -d

# Stop all services
down:
	docker-compose --profile monitoring --profile tracing --profile s3-storage down

# Follow logs
logs:
	docker-compose logs -f video-converter

# Clean everything (containers, networks, volumes)
clean:
	docker-compose --profile monitoring --profile tracing --profile s3-storage down -v
	docker system prune -f

# Create test video directory structure
test-videos:
	mkdir -p video_source
	@echo "Created video_source directory. Place your test video files here."
	@echo "They will be mounted to /app/video_source in the container."

# Quick development cycle
dev: build up logs