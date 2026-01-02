package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (
	defaultPort    = "8080"
	defaultVersion = "1.0.0"
	// SECURITY: ReadHeaderTimeout prevents slow loris attacks by limiting how long
	// the server waits for request headers. 10s is generous for legitimate clients.
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 60 * time.Second
	idleTimeout       = 120 * time.Second
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	version := os.Getenv("VERSION")
	if version == "" {
		version = defaultVersion
	}

	// Configure structured JSON logging for production
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	handler := NewHandler(version, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handler.HandleHealth)
	mux.HandleFunc("/parse", handler.HandleParse)

	// SECURITY: Chain security headers middleware before logging middleware.
	// Order: SecurityHeaders -> Logging -> Handler
	wrappedHandler := SecurityHeadersMiddleware(handler.LoggingMiddleware(mux))

	server := &http.Server{
		Addr:    ":" + port,
		Handler: wrappedHandler,
		// SECURITY: ReadHeaderTimeout prevents slow loris attacks.
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		// SECURITY: MaxHeaderBytes limits header size to prevent memory exhaustion.
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// Graceful shutdown handling
	done := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh

		log.Println("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
		close(done)
	}()

	log.Printf("Parser service starting on :%s (version %s)", port, version)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	<-done
	log.Println("Server stopped")
}
