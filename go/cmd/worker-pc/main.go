package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/garnizeh/eth-scanner/internal/worker"
)

func main() {
	// Setup logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("EthScanner PC Worker starting...")

	// Load configuration
	cfg, err := worker.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	log.Printf("Configuration loaded:")
	log.Printf("  API URL: %s", cfg.APIURL)
	log.Printf("  Worker ID: %s", cfg.WorkerID)
	log.Printf("  Checkpoint Interval: %v", cfg.CheckpointInterval)

	// Create worker
	w := worker.NewWorker(cfg)

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, initiating graceful shutdown...", sig)
		cancel()
	}()

	// Run worker
	log.Println("Worker started, waiting for jobs...")
	if err := w.Run(ctx); err != nil {
		// Treat context cancellation / deadline as graceful shutdown.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Println("Worker stopped gracefully")
			os.Exit(0)
		}
		log.Fatalf("Worker failed: %v", err)
	}

	log.Println("Worker stopped gracefully")
}
