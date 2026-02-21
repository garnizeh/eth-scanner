package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/garnizeh/eth-scanner/internal/config"
	"github.com/garnizeh/eth-scanner/internal/database"
	"github.com/garnizeh/eth-scanner/internal/server"
)

func main() {
	// Use a background context for initialization steps
	ctx := context.Background()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("%s - failed to load config: %v", time.Now().UTC().Format(time.RFC3339), err)
	}

	// Initialize database connection
	db, err := database.InitDB(ctx, cfg.DBPath)
	if err != nil {
		log.Fatalf("%s - failed to initialize database: %v", time.Now().UTC().Format(time.RFC3339), err)
	}
	// Ensure DB is closed on exit
	defer func() {
		if err := database.CloseDB(db); err != nil {
			log.Printf("%s - warning: failed to close database: %v", time.Now().UTC().Format(time.RFC3339), err)
		}
	}()

	// Create server and register routes
	srv, err := server.New(cfg, db)
	if err != nil {
		log.Fatalf("%s - error creating server: %v", time.Now().UTC().Format(time.RFC3339), err)
	}
	srv.RegisterRoutes()

	log.Printf("%s - starting server on :%s", time.Now().UTC().Format(time.RFC3339), cfg.Port)

	// Setup signal handling for graceful shutdown
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start server (blocks until context canceled or server error)
	if err := srv.Start(sigCtx); err != nil {
		log.Printf("%s - server stopped: %v", time.Now().UTC().Format(time.RFC3339), err)
		os.Exit(1)
	}

	log.Printf("%s - server exited cleanly", time.Now().UTC().Format(time.RFC3339))
}
