package worker

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Config holds worker configuration values loaded from environment.
type Config struct {
	APIURL             string
	WorkerID           string
	APIKey             string
	CheckpointInterval time.Duration
	// LeaseGracePeriod is subtracted from lease expiry to create a scanning
	// deadline so the worker can checkpoint and shut down gracefully before
	// the master-side lease actually expires.
	LeaseGracePeriod time.Duration
	// Retry configuration
	RetryMinDelay time.Duration
	RetryMaxDelay time.Duration
	// Adaptive batch sizing
	TargetJobDurationSeconds int64   // seconds, default 3600
	MinBatchSize             uint32  // default 100000
	MaxBatchSize             uint32  // default 10000000
	BatchAdjustAlpha         float64 // smoothing factor 0..1, default 0.5
	InitialBatchSize         uint32  // optional initial batch size; 0 means use calculated default
}

// LoadConfig reads configuration from environment variables and validates them.
// Required env vars:
//
//	WORKER_API_URL
//
// Optional env vars:
//
//	WORKER_ID (auto-generated if empty)
//	WORKER_CHECKPOINT_INTERVAL (default: 5m)
//	WORKER_API_KEY (optional, may be required by Master API depending on configuration)
func LoadConfig() (*Config, error) {
	apiURL := os.Getenv("WORKER_API_URL")
	if apiURL == "" {
		return nil, fmt.Errorf("missing required environment variable WORKER_API_URL")
	}
	// Validate URL
	if err := validateURL(apiURL); err != nil {
		return nil, fmt.Errorf("invalid WORKER_API_URL: %w", err)
	}

	// API key is optional. The Master API may disable header validation; if
	// the key is absent the worker will discover this on first request and
	// should handle an authentication error accordingly.
	apiKey := os.Getenv("WORKER_API_KEY")

	workerID := os.Getenv("WORKER_ID")
	if workerID == "" {
		id, err := autoGenerateWorkerID()
		if err != nil {
			return nil, fmt.Errorf("failed to auto-generate WORKER_ID: %w", err)
		}
		workerID = id
	}

	checkpointInterval := 5 * time.Minute
	if v := os.Getenv("WORKER_CHECKPOINT_INTERVAL"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_CHECKPOINT_INTERVAL: %w", err)
		}
		checkpointInterval = d
	}

	// Adaptive batch sizing environment overrides
	targetSecs := int64(3600)
	if v := os.Getenv("WORKER_TARGET_JOB_DURATION"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_TARGET_JOB_DURATION: %w", err)
		}
		targetSecs = n
	}

	minBatch := uint32(100000)
	if v := os.Getenv("WORKER_MIN_BATCH_SIZE"); v != "" {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_MIN_BATCH_SIZE: %w", err)
		}
		minBatch = uint32(n)
	}

	maxBatch := uint32(10000000)
	if v := os.Getenv("WORKER_MAX_BATCH_SIZE"); v != "" {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_MAX_BATCH_SIZE: %w", err)
		}
		maxBatch = uint32(n)
	}

	alpha := 0.5
	if v := os.Getenv("WORKER_BATCH_ADJUST_ALPHA"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_BATCH_ADJUST_ALPHA: %w", err)
		}
		alpha = f
	}

	initialBatch := uint32(0)
	if v := os.Getenv("WORKER_INITIAL_BATCH_SIZE"); v != "" {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid WORKER_INITIAL_BATCH_SIZE: %w", err)
		}
		initialBatch = uint32(n)
	}

	return &Config{
		APIURL:                   apiURL,
		WorkerID:                 workerID,
		APIKey:                   apiKey,
		CheckpointInterval:       checkpointInterval,
		LeaseGracePeriod:         30 * time.Second,
		RetryMinDelay:            1 * time.Second,
		RetryMaxDelay:            5 * time.Minute,
		TargetJobDurationSeconds: targetSecs,
		MinBatchSize:             minBatch,
		MaxBatchSize:             maxBatch,
		BatchAdjustAlpha:         alpha,
		InitialBatchSize:         initialBatch,
	}, nil
}

func validateURL(raw string) error {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return fmt.Errorf("failed to parse url: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("url must include scheme and host")
	}
	return nil
}

// autoGenerateWorkerID builds an id using hostname and random bytes.
func autoGenerateWorkerID() (string, error) {
	hn, _ := os.Hostname()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return fmt.Sprintf("worker-pc-%s-%s", sanitizeHostname(hn), hex.EncodeToString(b)), nil
}

// sanitizeHostname keeps hostname safe for use in IDs (very small sanitization).
func sanitizeHostname(h string) string {
	if h == "" {
		return "unknown"
	}
	// remove spaces
	out := make([]rune, 0, len(h))
	for _, r := range h {
		if r == ' ' || r == '/' || r == '\\' {
			continue
		}
		out = append(out, r)
	}
	return string(out)
}
