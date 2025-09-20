package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/matt-primrose/video-converter-service/internal/config"
	"github.com/matt-primrose/video-converter-service/internal/worker"
	"github.com/matt-primrose/video-converter-service/pkg/models"
)

// Router handles routing events from different sources to the worker
type Router struct {
	config *config.Config
	worker *worker.Worker
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

	// Start Azure Event Grid listener if configured
	if r.config.EventSources.AzureEventGrid.Endpoint != "" {
		go r.startAzureEventGridListener(ctx)
	}

	// Start WebSocket listener if configured
	if r.config.EventSources.WebSocket.Endpoint != "" {
		go r.startWebSocketListener(ctx)
	}

	<-ctx.Done()
	slog.Info("Event router stopping")
	return nil
}

// startAzureEventGridListener starts listening for Azure Event Grid events
func (r *Router) startAzureEventGridListener(ctx context.Context) {
	slog.Info("Starting Azure Event Grid listener",
		"endpoint", r.config.EventSources.AzureEventGrid.Endpoint)

	// TODO: Implement Azure Event Grid subscription and webhook handling
	// For now, this is a placeholder that handles webhook validation
	http.HandleFunc("/eventgrid", r.handleEventGridWebhook)

	// This would typically be handled by the main HTTP server
	// Left here as a placeholder for the Event Grid specific logic
}

// startWebSocketListener starts the WebSocket event listener
func (r *Router) startWebSocketListener(ctx context.Context) {
	slog.Info("Starting WebSocket listener",
		"endpoint", r.config.EventSources.WebSocket.Endpoint)

	// TODO: Implement WebSocket client connection to receive events
	// This would connect to the configured WebSocket endpoint
	// and listen for conversion events
}

// handleEventGridWebhook handles incoming Azure Event Grid webhooks
func (r *Router) handleEventGridWebhook(w http.ResponseWriter, req *http.Request) {
	slog.Debug("Received Event Grid webhook", "method", req.Method)

	// Handle subscription validation
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

	url, ok := data["url"].(string)
	if !ok {
		return fmt.Errorf("missing blob URL")
	}

	// Extract videoId from blob name/path
	// This is a simplified implementation - in practice you might
	// derive this from blob metadata or path conventions
	videoId := extractVideoIdFromUrl(url)

	// Create conversion job using default template
	job := &models.ConversionJob{
		JobID:    generateJobID(),
		VideoID:  videoId,
		Template: "default", // Use default template from config
		Source: models.SourceConfig{
			URI:  url,
			Type: "http",
		},
	}

	// Submit job to worker
	if err := r.worker.SubmitJob(job); err != nil {
		return fmt.Errorf("failed to submit job: %w", err)
	}

	slog.Info("Submitted conversion job",
		"jobId", job.JobID,
		"videoId", job.VideoID,
		"sourceUrl", job.Source.URI,
	)

	return nil
}

// extractVideoIdFromUrl extracts a video ID from the blob URL
// This is a placeholder implementation
func extractVideoIdFromUrl(url string) string {
	// TODO: Implement proper video ID extraction logic
	// This might involve parsing the blob path, checking metadata, etc.
	return fmt.Sprintf("video-%d", len(url)%1000)
}

// generateJobID generates a unique job ID
func generateJobID() string {
	// TODO: Implement proper unique ID generation
	// This should use UUIDs or another reliable method
	return fmt.Sprintf("job-%d", len(fmt.Sprintf("%p", &struct{}{})))
}
