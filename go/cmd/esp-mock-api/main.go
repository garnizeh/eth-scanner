package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/jobs/lease", handleLease)
	mux.HandleFunc("/api/v1/jobs/", handleJobUpdate) // matches /checkpoint and /complete

	// Logging middleware
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[MOCK] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		mux.ServeHTTP(w, r)
	})

	port := "8080"
	log.Printf("ESP32 Mock API starting on :%s (listening on all interfaces)", port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}

func handleLease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	scenario := r.Header.Get("X-Test-Scenario")
	log.Printf("Lease request received. Scenario: %s", scenario)

	switch scenario {
	case "500":
		http.Error(w, "internal server error", http.StatusInternalServerError)
	case "404":
		http.Error(w, "no jobs", http.StatusNotFound)
	case "malformed":
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"job_id": 123, "nonce_start": "not-a-number"}`) // invalid type
	default:
		// Success case
		resp := map[string]any{
			"job_id":         42,
			"prefix_28":      "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHA==", // bytes 1-28 (correct base64)
			"nonce_start":    1000,
			"nonce_end":      2000,
			"target_address": "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func handleJobUpdate(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	scenario := r.Header.Get("X-Test-Scenario")
	log.Printf("Update request (%s) received. Scenario: %s", path, scenario)

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
