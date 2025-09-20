package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/matt-primrose/video-converter-service/internal/config"
	"github.com/matt-primrose/video-converter-service/internal/events"
	"github.com/matt-primrose/video-converter-service/internal/worker"
)

const (
	serviceName    = "video-converter-service"
	serviceVersion = "0.1.0"
)

func main() {
	// Initialize logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting video converter service",
		"service", serviceName,
		"version", serviceVersion,
	)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Set log level from config
	setLogLevel(cfg.Observability.LogLevel)

	slog.Info("Configuration loaded successfully",
		"storage_type", cfg.Storage.Type,
		"max_concurrent_jobs", cfg.Processing.MaxConcurrentJobs,
		"server_port", cfg.Server.Port,
	)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize components
	worker := worker.New(cfg)
	eventRouter := events.NewRouter(cfg, worker)

	// Start HTTP server for health checks
	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler: setupHTTPRoutes(cfg),
	}

	// Start health check server
	healthServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.HealthCheckPort),
		Handler: setupHealthRoutes(),
	}

	var wg sync.WaitGroup

	// Start main HTTP server
	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("Starting HTTP server", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	// Start health check server
	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("Starting health check server", "addr", healthServer.Addr)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Health check server error", "error", err)
		}
	}()

	// Start event listeners
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := eventRouter.Start(ctx); err != nil {
			slog.Error("Event router error", "error", err)
		}
	}()

	// Start worker pool
	wg.Add(1)
	go func() {
		defer wg.Done()
		worker.Start(ctx)
	}()

	// Start metrics server if enabled
	if cfg.Observability.MetricsPort > 0 {
		metricsServer := &http.Server{
			Addr:    fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Observability.MetricsPort),
			Handler: setupMetricsRoutes(),
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			slog.Info("Starting metrics server", "addr", metricsServer.Addr)
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("Metrics server error", "error", err)
			}
		}()
	}

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	slog.Info("Shutting down service...")

	// Cancel context to stop all goroutines
	cancel()

	// Shutdown HTTP servers gracefully
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Error shutting down HTTP server", "error", err)
	}

	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("Error shutting down health server", "error", err)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	slog.Info("Service shutdown complete")
}

// setLogLevel configures the global log level based on config
func setLogLevel(level string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)
}

// setupHTTPRoutes creates the main HTTP server routes
func setupHTTPRoutes(cfg *config.Config) http.Handler {
	mux := http.NewServeMux()

	// WebSocket endpoint for events (if enabled)
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement WebSocket handler
		http.Error(w, "WebSocket handler not implemented", http.StatusNotImplemented)
	})

	// Status endpoint
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"service":"%s","version":"%s","status":"running"}`, serviceName, serviceVersion)
	})

	return mux
}

// setupHealthRoutes creates health check routes
func setupHealthRoutes() http.Handler {
	mux := http.NewServeMux()

	// Liveness probe
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	})

	// Readiness probe
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Check if service is ready (dependencies available, etc.)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Ready")
	})

	return mux
}

// setupMetricsRoutes creates metrics endpoint
func setupMetricsRoutes() http.Handler {
	mux := http.NewServeMux()

	// Prometheus metrics endpoint
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement Prometheus metrics handler
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "# TODO: Implement metrics\n")
	})

	return mux
}
