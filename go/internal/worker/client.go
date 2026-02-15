package worker

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"
)

// APIError represents a non-2xx response from Master API.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("api error %d: %s", e.StatusCode, e.Message)
}

// Client is a small HTTP client for Master API used by workers.
type Client struct {
	httpClient *http.Client
	baseURL    string
	workerID   string
	apiKey     string
}

// ErrUnauthorized is returned when the Master API responds with 401 Unauthorized.
// This indicates the worker must stop because authentication is required/invalid.
var ErrUnauthorized = errors.New("unauthorized: API key required or invalid")

// NewClient constructs a Client from the worker Config.
func NewClient(cfg *Config) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    cfg.APIURL,
		workerID:   cfg.WorkerID,
		apiKey:     cfg.APIKey,
	}
}

// doRequestWithContext performs an HTTP request, marshaling reqBody (if not nil)
// and unmarshaling response into respBody (if not nil). Returns *APIError for
// non-2xx responses.
//
// nolint // ctx parameter is reserved for future use when we need to support request cancellation.
func (c *Client) doRequestWithContext(ctx context.Context, method, p string, reqBody, respBody any) error {
	// Build URL
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("invalid base url: %w", err)
	}
	// join path
	base.Path = path.Join(base.Path, p)

	var body io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, base.String(), body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read body
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized {
			// Immediate fatal condition for the worker: authentication failed.
			return ErrUnauthorized
		}
		// Try to parse error JSON {"error":"...","message":"..."}
		var apiErr struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(respBytes, &apiErr)
		msg := apiErr.Message
		if msg == "" {
			msg = apiErr.Error
		}
		if msg == "" {
			msg = string(respBytes)
		}
		return &APIError{StatusCode: resp.StatusCode, Message: msg}
	}

	if respBody != nil && len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, respBody); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}

// ErrNoJobsAvailable is returned when the API reports no available jobs (HTTP 404).
var ErrNoJobsAvailable = errors.New("no jobs available")

type JobLease struct {
	JobID      string
	Prefix28   []byte
	NonceStart uint32
	NonceEnd   uint32
	ExpiresAt  time.Time
}

// LeaseBatch requests a job lease from the Master API.
func (c *Client) LeaseBatch(ctx context.Context, requestedBatchSize uint32) (*JobLease, error) {
	req := leaseRequest{
		WorkerID:           c.workerID,
		RequestedBatchSize: requestedBatchSize,
	}

	var resp leaseResponse
	err := c.doRequestWithContext(ctx, http.MethodPost, "/api/v1/jobs/lease", req, &resp)
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return nil, ErrUnauthorized
		}
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil, ErrNoJobsAvailable
		}
		return nil, fmt.Errorf("lease request failed: %w", err)
	}

	// Decode prefix_28 from hex
	prefix28, decErr := hex.DecodeString(resp.Prefix28)
	if decErr != nil {
		return nil, fmt.Errorf("invalid prefix_28 hex: %w", decErr)
	}
	if len(prefix28) != 28 {
		return nil, fmt.Errorf("invalid prefix_28 length: got %d, want 28", len(prefix28))
	}

	// Parse expires_at as UTC
	expiresAt, perr := time.Parse(time.RFC3339, resp.ExpiresAt)
	if perr != nil {
		return nil, fmt.Errorf("invalid expires_at: %w", perr)
	}

	return &JobLease{
		JobID:      resp.JobID,
		Prefix28:   prefix28,
		NonceStart: resp.NonceStart,
		NonceEnd:   resp.NonceEnd,
		ExpiresAt:  expiresAt.UTC(),
	}, nil
}

// Internal request/response types
type leaseRequest struct {
	WorkerID           string `json:"worker_id"`
	RequestedBatchSize uint32 `json:"requested_batch_size"`
}

type leaseResponse struct {
	JobID      string `json:"job_id"`
	Prefix28   string `json:"prefix_28"` // hex-encoded
	NonceStart uint32 `json:"nonce_start"`
	NonceEnd   uint32 `json:"nonce_end"`
	ExpiresAt  string `json:"expires_at"` // RFC3339
}

// checkpointRequest is the payload sent to update a job's checkpoint.
type checkpointRequest struct {
	WorkerID     string `json:"worker_id"`
	CurrentNonce uint32 `json:"current_nonce"`
	KeysScanned  uint64 `json:"keys_scanned"`
}

// UpdateCheckpoint reports progress for a job to the Master API.
func (c *Client) UpdateCheckpoint(ctx context.Context, jobID string, currentNonce uint32, keysScanned uint64) error {
	req := checkpointRequest{
		WorkerID:     c.workerID,
		CurrentNonce: currentNonce,
		KeysScanned:  keysScanned,
	}

	path := fmt.Sprintf("/api/v1/jobs/%s/checkpoint", jobID)

	if err := c.doRequestWithContext(ctx, http.MethodPatch, path, req, nil); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return ErrUnauthorized
		}
		return fmt.Errorf("checkpoint update failed: %w", err)
	}
	return nil
}

// completeRequest is the payload sent to mark a job as completed.
type completeRequest struct {
	WorkerID    string `json:"worker_id"`
	FinalNonce  uint32 `json:"final_nonce"`
	KeysScanned uint64 `json:"keys_scanned"`
}

// CompleteBatch marks a job as completed on the Master API.
func (c *Client) CompleteBatch(ctx context.Context, jobID string, finalNonce uint32, totalKeysScanned uint64) error {
	req := completeRequest{
		WorkerID:    c.workerID,
		FinalNonce:  finalNonce,
		KeysScanned: totalKeysScanned,
	}

	path := fmt.Sprintf("/api/v1/jobs/%s/complete", jobID)

	if err := c.doRequestWithContext(ctx, http.MethodPost, path, req, nil); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return ErrUnauthorized
		}
		return fmt.Errorf("complete batch failed: %w", err)
	}
	return nil
}

// resultRequest is the payload sent to submit a found private key match.
type resultRequest struct {
	WorkerID        string `json:"worker_id"`
	PrivateKey      string `json:"private_key"`      // hex-encoded 32-byte private key
	EthereumAddress string `json:"ethereum_address"` // checksummed
	FoundAt         string `json:"found_at"`         // RFC3339 UTC
}

// SubmitResult submits a found private key result to the Master API.
func (c *Client) SubmitResult(ctx context.Context, privateKey []byte, address string) error {
	if len(privateKey) != 32 {
		return fmt.Errorf("invalid private key length: expected 32 bytes, got %d", len(privateKey))
	}

	req := resultRequest{
		WorkerID:        c.workerID,
		PrivateKey:      hex.EncodeToString(privateKey),
		EthereumAddress: address,
		FoundAt:         time.Now().UTC().Format(time.RFC3339),
	}

	if err := c.doRequestWithContext(ctx, http.MethodPost, "/api/v1/results", req, nil); err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return ErrUnauthorized
		}
		return fmt.Errorf("result submission failed: %w", err)
	}
	return nil
}
