#!/bin/bash
set -e

# Configuration
API_URL=${API_URL:-"http://localhost:8080"}
API_KEY=${API_KEY:-"test-api-key"}
WORKER_ID="test-worker-01"

echo "=== Starting API Validation ==="

# 1. Health Check
echo "Testing GET /health..."
curl -s -f -H "X-API-KEY: $API_KEY" "$API_URL/health" | grep -i "ok"
echo "✓ Health Check OK"

# 2. Stats Check (initially empty)
echo "Testing GET /api/v1/stats..."
curl -s -f -H "X-API-KEY: $API_KEY" "$API_URL/api/v1/stats" | grep "total_jobs"
echo "✓ Stats OK"

# 3. Lease Job
echo "Testing POST /api/v1/jobs/lease..."
LEASE_RESP=$(curl -s -f -X POST -H "X-API-KEY: $API_KEY" -H "Content-Type: application/json" \
  -d "{\"worker_id\":\"$WORKER_ID\",\"requested_batch_size\":1000000}" \
  "$API_URL/api/v1/jobs/lease")

JOB_ID=$(echo "$LEASE_RESP" | grep -o '"job_id":[0-9]*' | cut -d: -f2)
NONCE_START=$(echo "$LEASE_RESP" | grep -o '"nonce_start":[0-9]*' | cut -d: -f2)
NONCE_END=$(echo "$LEASE_RESP" | grep -o '"nonce_end":[0-9]*' | cut -d: -f2)

if [ -z "$JOB_ID" ]; then
  echo "FAILED: Could not lease job"
  exit 1
fi

echo "✓ Leased Job ID: $JOB_ID (Nonces: $NONCE_START to $NONCE_END)"

# 4. Checkpoint Job
echo "Testing PATCH /api/v1/jobs/$JOB_ID/checkpoint..."
CURRENT_NONCE=$((NONCE_START + 500000))
curl -s -f -X PATCH -H "X-API-KEY: $API_KEY" -H "Content-Type: application/json" \
  -d "{\"worker_id\":\"$WORKER_ID\",\"current_nonce\":$CURRENT_NONCE,\"keys_scanned\":500000,\"started_at\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"duration_ms\":5000}" \
  "$API_URL/api/v1/jobs/$JOB_ID/checkpoint"
echo "✓ Checkpoint OK"

# 5. Submit Result
echo "Testing POST /api/v1/results..."
# Mock a found result
PRIV_KEY="0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
ADDRESS="0x000000000000000000000000000000000000dEaD" # default target address
curl -s -f -X POST -H "X-API-KEY: $API_KEY" -H "Content-Type: application/json" \
  -d "{\"worker_id\":\"$WORKER_ID\",\"job_id\":$JOB_ID,\"private_key\":\"$PRIV_KEY\",\"address\":\"$ADDRESS\",\"nonce\":$CURRENT_NONCE}" \
  "$API_URL/api/v1/results"
echo "✓ Submit Result OK"

# 6. Complete Job
echo "Testing POST /api/v1/jobs/$JOB_ID/complete..."
curl -s -f -X POST -H "X-API-KEY: $API_KEY" -H "Content-Type: application/json" \
  -d "{\"worker_id\":\"$WORKER_ID\",\"final_nonce\":$NONCE_END,\"keys_scanned\":1000000,\"started_at\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"duration_ms\":10000}" \
  "$API_URL/api/v1/jobs/$JOB_ID/complete"
echo "✓ Complete Job OK"

# 7. Final Stats Check
echo "Testing final GET /api/v1/stats..."
FINAL_STATS=$(curl -s -f -H "X-API-KEY: $API_KEY" "$API_URL/api/v1/stats")
echo "$FINAL_STATS" | grep '"completed":1'
echo "$FINAL_STATS" | grep '"results_found":1'
echo "✓ Final Stats OK"

echo "=== API Validation Successful ==="
