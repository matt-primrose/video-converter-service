# Multi-stage build for Go application with ffmpeg
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o video-converter ./cmd/video-converter

# Final stage - minimal runtime image with ffmpeg
FROM alpine:latest

# Install ffmpeg and other runtime dependencies
RUN apk add --no-cache \
    ffmpeg \
    ca-certificates \
    tzdata

# Create non-root user
RUN adduser -D -s /bin/sh appuser

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/video-converter .

# Create directories for temporary files
RUN mkdir -p /tmp/video-converter && \
    chown -R appuser:appuser /app /tmp/video-converter

# Switch to non-root user
USER appuser

# Expose ports
EXPOSE 8080 8081 9090

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:8081/healthz || exit 1

# Set default command
CMD ["./video-converter"]