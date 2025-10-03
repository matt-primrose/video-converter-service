package events

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/matt-primrose/video-converter-service/internal/config"
	"github.com/matt-primrose/video-converter-service/internal/worker"
	"github.com/matt-primrose/video-converter-service/pkg/models"
)

// Router handles routing events from different sources to the worker
type Router struct {
	config *config.Config
	worker *worker.Worker
	server *http.Server
}

// NewRouter creates a new event router
func NewRouter(cfg *config.Config, w *worker.Worker) *Router {
	return &Router{
		config: cfg,
		worker: w,
	}
}

// Start starts the event listeners
func (r *Router) Start(ctx context.Context) error {
	slog.Info("Starting event router")

	// Set up HTTP mux for event handlers
	mux := http.NewServeMux()
	hasEventSources := false

	// Start Azure Event Grid listener if configured
	if r.config.EventSources.AzureEventGrid.Endpoint != "" && r.config.EventSources.AzureEventGrid.Key != "" {
		slog.Info("Configuring Azure Event Grid webhook handler", "endpoint", r.config.EventSources.AzureEventGrid.Endpoint)
		mux.HandleFunc("/webhook/eventgrid", r.handleEventGridWebhook)
		hasEventSources = true
	}

	// Start WebSocket listener if configured
	if r.config.EventSources.WebSocket.Endpoint != "" {
		go r.startWebSocketListener(ctx)
		hasEventSources = true
	}

	// Only start HTTP server if we have event sources that need it
	if hasEventSources {
		// Start webhook server on a different port (8082) to avoid conflicts
		r.server = &http.Server{
			Addr:    ":8082",
			Handler: mux,
		}

		go func() {
			slog.Info("Starting event webhook server", "addr", r.server.Addr)
			if err := r.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("Event webhook server error", "error", err)
			}
		}()
	}

	<-ctx.Done()
	slog.Info("Event router stopping")

	// Shutdown webhook server
	if r.server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := r.server.Shutdown(shutdownCtx); err != nil {
			slog.Error("Error shutting down event webhook server", "error", err)
		}
	}

	return nil
}

// validateEventGridKey validates the Event Grid access key from request headers
func (r *Router) validateEventGridKey(req *http.Request) bool {
	// Event Grid sends the key in the aeg-sas-key header
	key := req.Header.Get("aeg-sas-key")
	if key == "" {
		// Also check for the key in aeg-sas-token header (alternative)
		key = req.Header.Get("aeg-sas-token")
	}

	return key != "" && key == r.config.EventSources.AzureEventGrid.Key
}

// startWebSocketListener starts the WebSocket event listener
func (r *Router) startWebSocketListener(ctx context.Context) {
	slog.Info("Starting WebSocket listener", "endpoint", r.config.EventSources.WebSocket.Endpoint)

	// Implement reconnection logic with exponential backoff
	reconnectDelay := time.Second
	maxReconnectDelay := time.Minute * 5

	for {
		select {
		case <-ctx.Done():
			slog.Info("WebSocket listener stopping")
			return
		default:
			if err := r.connectWebSocket(ctx); err != nil {
				slog.Error("WebSocket connection failed", "error", err, "retry_in", reconnectDelay)

				// Wait before reconnecting, but respect context cancellation
				select {
				case <-ctx.Done():
					return
				case <-time.After(reconnectDelay):
					// Exponential backoff with maximum delay
					reconnectDelay *= 2
					if reconnectDelay > maxReconnectDelay {
						reconnectDelay = maxReconnectDelay
					}
				}
			} else {
				// Reset reconnect delay on successful connection
				reconnectDelay = time.Second
			}
		}
	}
}

// connectWebSocket establishes a WebSocket connection and listens for events
func (r *Router) connectWebSocket(ctx context.Context) error {
	slog.Info("Connecting to WebSocket", "endpoint", r.config.EventSources.WebSocket.Endpoint)

	// For now, this is a placeholder that simulates WebSocket connection
	// In a real implementation, you would use a WebSocket library like gorilla/websocket
	// and implement the actual WebSocket client protocol

	// Example structure:
	// 1. Create WebSocket connection with authentication
	// 2. Send authentication token if required
	// 3. Listen for messages in a loop
	// 4. Parse and process events
	// 5. Handle connection errors and reconnection

	// Placeholder: simulate connection for 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Simulate receiving an event
			slog.Debug("WebSocket connection active (placeholder)")
		}
	}
}

// handleEventGridWebhook handles incoming Azure Event Grid webhooks
func (r *Router) handleEventGridWebhook(w http.ResponseWriter, req *http.Request) {
	slog.Debug("Received Event Grid webhook", "method", req.Method, "remote_addr", req.RemoteAddr)

	// Validate authentication if key is configured
	if r.config.EventSources.AzureEventGrid.Key != "" {
		if !r.validateEventGridKey(req) {
			slog.Warn("Event Grid webhook authentication failed", "remote_addr", req.RemoteAddr)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Handle subscription validation and events
	if req.Method == "POST" {
		var events []map[string]interface{}
		if err := json.NewDecoder(req.Body).Decode(&events); err != nil {
			slog.Error("Failed to decode Event Grid payload", "error", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		for _, event := range events {
			// Handle subscription validation events
			if eventType, ok := event["eventType"].(string); ok && eventType == "Microsoft.EventGrid.SubscriptionValidationEvent" {
				if data, ok := event["data"].(map[string]interface{}); ok {
					if validationCode, ok := data["validationCode"].(string); ok {
						response := map[string]string{
							"validationResponse": validationCode,
						}
						w.Header().Set("Content-Type", "application/json")
						json.NewEncoder(w).Encode(response)
						return
					}
				}
			}

			// Process actual blob events
			if err := r.processEventGridEvent(event); err != nil {
				slog.Error("Failed to process Event Grid event", "error", err)
			}
		}

		w.WriteHeader(http.StatusOK)
	}
}

// processEventGridEvent processes a single Event Grid event
func (r *Router) processEventGridEvent(event map[string]interface{}) error {
	eventType, ok := event["eventType"].(string)
	if !ok {
		return fmt.Errorf("missing eventType")
	}

	// Handle blob created events
	if eventType == "Microsoft.Storage.BlobCreated" {
		return r.handleBlobCreatedEvent(event)
	}

	slog.Debug("Ignoring unsupported event type", "eventType", eventType)
	return nil
}

// handleBlobCreatedEvent handles blob created events and converts them to conversion jobs
func (r *Router) handleBlobCreatedEvent(event map[string]interface{}) error {
	data, ok := event["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing event data")
	}

	blobUrl, ok := data["url"].(string)
	if !ok {
		return fmt.Errorf("missing blob URL")
	}

	// Check if this is a video file based on the URL
	if !r.isVideoFile(blobUrl) {
		slog.Debug("Ignoring non-video file", "url", blobUrl)
		return nil
	}

	// Extract additional metadata from the event
	contentType, _ := data["contentType"].(string)
	contentLength, _ := data["contentLength"].(float64)

	// Extract videoId from blob name/path
	videoId := extractVideoIdFromUrl(blobUrl)

	// Detect source type from URL
	sourceType := r.detectSourceType(blobUrl)

	// Create conversion job using default template
	job := &models.ConversionJob{
		JobID:    generateJobID(),
		VideoID:  videoId,
		Template: "default", // Use default template from config
		Source: models.SourceConfig{
			URI:  blobUrl,
			Type: sourceType,
		},
	}

	// Initialize job status
	job.Status.State = models.JobStatePending
	job.Status.Progress = 0.0
	job.CreatedAt = time.Now()

	// Submit job to worker
	if err := r.worker.SubmitJob(job); err != nil {
		return fmt.Errorf("failed to submit job: %w", err)
	}

	slog.Info("Submitted conversion job from Event Grid",
		"jobId", job.JobID,
		"videoId", job.VideoID,
		"sourceUrl", job.Source.URI,
		"contentType", contentType,
		"contentLength", int64(contentLength),
	)

	return nil
}

// isVideoFile checks if the given URL points to a video file based on file extension
func (r *Router) isVideoFile(fileUrl string) bool {
	parsedUrl, err := url.Parse(fileUrl)
	if err != nil {
		return false
	}

	ext := strings.ToLower(path.Ext(parsedUrl.Path))
	videoExtensions := []string{".mp4", ".avi", ".mov", ".mkv", ".wmv", ".flv", ".webm", ".m4v", ".3gp", ".ts", ".mts"}

	for _, videoExt := range videoExtensions {
		if ext == videoExt {
			return true
		}
	}

	return false
}

// detectSourceType determines the appropriate source type based on the URL
func (r *Router) detectSourceType(fileUrl string) string {
	parsedUrl, err := url.Parse(fileUrl)
	if err != nil {
		return "http" // Default fallback
	}

	host := strings.ToLower(parsedUrl.Host)

	// Check for Azure Blob Storage
	if strings.Contains(host, ".blob.core.windows.net") {
		return "azure-blob"
	}

	// Check for AWS S3
	if strings.Contains(host, ".s3.") || strings.Contains(host, "s3.") || strings.Contains(host, ".amazonaws.com") {
		return "s3"
	}

	// Check for local file paths
	if parsedUrl.Scheme == "file" || parsedUrl.Scheme == "" {
		return "local"
	}

	// Default to HTTP for all other URLs
	return "http"
} // extractVideoIdFromUrl extracts a video ID from the blob URL
func extractVideoIdFromUrl(blobUrl string) string {
	parsedUrl, err := url.Parse(blobUrl)
	if err != nil {
		slog.Warn("Failed to parse blob URL", "url", blobUrl, "error", err)
		return fmt.Sprintf("video-%d", time.Now().Unix())
	}

	// Extract filename without extension from the path
	filename := path.Base(parsedUrl.Path)
	ext := path.Ext(filename)
	if ext != "" {
		filename = strings.TrimSuffix(filename, ext)
	}

	// Clean the filename to create a valid video ID
	videoId := strings.ReplaceAll(filename, " ", "-")
	videoId = strings.ToLower(videoId)

	if videoId == "" {
		videoId = fmt.Sprintf("video-%d", time.Now().Unix())
	}

	return videoId
}

// generateJobID generates a unique job ID using timestamp and random bytes
func generateJobID() string {
	// Generate 4 random bytes for uniqueness
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to timestamp-based ID if random generation fails
		return fmt.Sprintf("job-%d", time.Now().UnixNano())
	}

	// Combine timestamp with random bytes for uniqueness
	timestamp := time.Now().Unix()
	return fmt.Sprintf("job-%d-%x", timestamp, randomBytes)
}
