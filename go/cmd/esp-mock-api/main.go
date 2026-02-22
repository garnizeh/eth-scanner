package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

var (
	winScenario bool
	won         bool
)

func main() {
	flag.BoolVar(&winScenario, "win", false, "Always return a winning job scenario (Key 0x1)")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobs/lease", handleLease)
	mux.HandleFunc("/api/v1/jobs/", handleJobUpdate) // matches /checkpoint and /complete
	mux.HandleFunc("/api/v1/results", handleResults)

	// Logging middleware — sanitize tainted values before logging
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//nolint:gosec // false positive: Log injection via taint analysis in mock server is not a security risk
		log.Printf("[MOCK] %q %q from %q", r.Method, r.URL.Path, r.RemoteAddr)
		mux.ServeHTTP(w, r)
	})

	port := "8080"
	log.Printf("ESP32 Mock API starting on :%s (listening on all interfaces)", port)
	if winScenario {
		log.Printf("Win scenario active: returning nonce 1 as a winner.")
	}

	// Use an http.Server with timeouts to satisfy security linters
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
		// reasonable defaults for a mock server
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

func handleLease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	//nolint:gosec // false positive: Log injection via taint analysis in mock server is not a security risk
	scenario := r.Header.Get("X-Test-Scenario")

	// If the global flag is set, override the default scenario to "win"
	if winScenario && scenario == "" {
		if won {
			log.Printf("Win already achieved. Returning 404 No Jobs.")
			http.Error(w, "no jobs available", http.StatusNotFound)
			return
		}
		scenario = "win"
	}

	//nolint:gosec // false positive: Log injection via taint analysis in mock server is not a security risk
	log.Printf("Lease request received. Scenario: %q", scenario)

	switch scenario {
	case "500":
		http.Error(w, "internal server error", http.StatusInternalServerError)
	case "404":
		http.Error(w, "no jobs", http.StatusNotFound)
	case "malformed":
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"job_id": 123, "nonce_start": "not-a-number"}`) // invalid type
	case "win":
		// Winning case: private key 0x1 (nonce 1 + prefix 28 zero bytes)
		// which hashes to address 0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf.
		// A worker starting at nonce 0 will find it at the second iteration (nonce=1).
		resp := map[string]any{
			"job_id":           777,
			"prefix_28":        "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==", // 28 bytes of zeros
			"nonce_start":      0,
			"nonce_end":        100, // Small range
			"target_addresses": []string{"0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf"},
			"expires_at":       time.Now().Add(time.Hour).Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("failed to encode winning lease response: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	default:
		// Success case
		resp := map[string]any{
			"job_id":      42,
			"prefix_28":   "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHA==", // bytes 1-28 (correct base64)
			"nonce_start": 1000,
			"nonce_end":   2000,
			"target_addresses": []string{
				"0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
			},
			"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Printf("failed to encode lease response: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}
}

func handleJobUpdate(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	scenario := r.Header.Get("X-Test-Scenario")
	//nolint:gosec // false positive: Log injection via taint analysis in mock server is not a security risk
	log.Printf("Update request (%q) received. Scenario: %q", path, scenario)

	if scenario == "500" {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if strings.HasSuffix(path, "/checkpoint") {
		if r.Method != http.MethodPatch {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
	} else if strings.HasSuffix(path, "/complete") {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
	} else {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Verify request body
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Printf("Error decoding body: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	log.Printf("Received body: %+v", body)

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok"}`)
}

func handleResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	log.Printf("[MOCK] Result submitted successfully! STOPPING WIN SCENARIO.")
	if winScenario {
		won = true
	}
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"status":"created"}`)
}
