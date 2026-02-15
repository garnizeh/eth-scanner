package worker

import (
	"bytes"
	"context"
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
