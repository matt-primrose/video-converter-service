.PHONY: build up down logs clean test-videos help

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
	@echo "  test-videos   - Create test video directory structure"
	@echo ""
	@echo "Service URLs (when running):"
	@echo "  Video Converter API:  http://localhost:8080"
	@echo "  Health Checks:        http://localhost:8081"
	@echo "  Metrics:              http://localhost:9090/metrics"
	@echo "  Prometheus:           http://localhost:9091 (with monitoring)"
	@echo "  Grafana:              http://localhost:3000 (with monitoring, admin/admin)"
	@echo "  Jaeger:               http://localhost:16686 (with tracing)"
	@echo "  MinIO Console:        http://localhost:9001 (with s3-storage)"

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
	mkdir -p test-videos
	@echo "Created test-videos directory. Place your test video files here."
	@echo "They will be mounted to /app/test-videos in the container."

# Quick development cycle
dev: build up logs