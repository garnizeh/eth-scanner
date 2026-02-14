package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"time"
)

// middleware.go implements common HTTP middleware for the Master API.
// It provides Logger, CORS, and RequestID middleware following standard
// http.Handler middleware patterns.

// requestIDKey is an unexported type for context keys in this package.
type requestIDKey struct{}

// RequestIDContextKey is the context key used to store the request id.
var RequestIDContextKey = requestIDKey{}

// GetRequestID extracts the request id from the context or returns empty string.
func GetRequestID(ctx context.Context) string {
	if v := ctx.Value(RequestIDContextKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Logger middleware logs request method, path, duration, and response status.
// Logged timestamp uses UTC as required by project guidelines.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now().UTC()

		// wrap ResponseWriter to capture status code
		rw := &statusCapturingResponseWriter{ResponseWriter: w}

		next.ServeHTTP(rw, r)

		// If no status was written, default to 200
		status := rw.status
		if status == 0 {
			status = http.StatusOK
		}

		duration := time.Since(start)

		log.Printf("%s method=%s path=%s status=%d duration=%s",
			start.Format(time.RFC3339), r.Method, r.URL.Path, status, duration)
	})
}

// statusCapturingResponseWriter wraps http.ResponseWriter to capture status code.
type statusCapturingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCapturingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusCapturingResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		// implicitly assume 200 if Write is called without WriteHeader
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// CORS sets permissive CORS headers for development and handles preflight OPTIONS.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")

		if r.Method == http.MethodOptions {
			// Preflight request — respond immediately
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// RequestID middleware generates a unique request id, adds it to the request
// context and response headers as X-Request-ID.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := generateRequestID()
		if err != nil {
			// Fallback to timestamp-based id (very unlikely). Do not stop the request.
			id = time.Now().UTC().Format("20060102T150405.000000000Z07:00")
		}

		// Add to context
		ctx := context.WithValue(r.Context(), RequestIDContextKey, id)

		// Add response header
		w.Header().Set("X-Request-ID", id)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// generateRequestID creates a 16-byte random hex string.
func generateRequestID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
