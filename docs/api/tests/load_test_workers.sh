#!/bin/bash
set -e

# Configuration
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
REPO_ROOT="$( cd "$SCRIPT_DIR/../../.." && pwd )"
TEST_TMP_DIR="$REPO_ROOT/docs/api/tests/tmp"
API_URL="http://localhost:8080"
API_KEY="stress-test-key"
NUM_WORKERS=10
TEST_DURATION=60 # seconds

echo "=== Starting Load Test (10+ Workers) ==="
mkdir -p "$TEST_TMP_DIR"
cd "$REPO_ROOT"

# 1. Start Master API if not running
if ! curl -s "$API_URL/health" > /dev/null; then
    echo "Starting Master API..."
    MASTER_API_KEY="$API_KEY" \
    MASTER_DB_PATH="$TEST_TMP_DIR/stress-test.db" \
    MASTER_PORT=8080 \
    "$REPO_ROOT/go/bin/master" > "$TEST_TMP_DIR/master.log" 2>&1 &
    MASTER_PID=$!
    
    # Wait for server to be ready
    echo "Waiting for server to start..."
    for i in {1..10}; do
        if curl -s -H "X-API-KEY: $API_KEY" "$API_URL/health" | grep -q "ok"; then
            echo "✓ Master API is ready"
            break
        fi
        sleep 1
    done
else
    echo "✓ Master API already running"
fi

# 2. Spawn Workers
echo "Spawning $NUM_WORKERS PC Workers..."
WORKER_PIDS=()
for i in $(seq 1 $NUM_WORKERS); do
    WORKER_ID="load-worker-$i"
    echo "  → Starting $WORKER_ID"
    
    WORKER_API_URL="$API_URL" \
    WORKER_API_KEY="$API_KEY" \
    WORKER_ID="$WORKER_ID" \
    WORKER_TARGET_JOB_DURATION=5 \
    WORKER_CHECKPOINT_INTERVAL=2s \
    WORKER_MAX_BATCH_SIZE=100000 \
    "$REPO_ROOT/go/bin/worker-pc" > "$TEST_TMP_DIR/worker-$i.log" 2>&1 &
    
    WORKER_PIDS+=($!)
done

echo "✓ All workers spawned. Testing for $TEST_DURATION seconds..."

# 3. Monitor
# Check stats periodically
for i in $(seq 1 $((TEST_DURATION / 10))); do
    sleep 10
    echo "--- Status @ $((i * 10))s ---"
    curl -s -H "X-API-KEY: $API_KEY" "$API_URL/api/v1/stats" || echo "Error fetching stats"
    echo ""
done

# 4. Cleanup
echo "=== Cleaning up ==="
for pid in "${WORKER_PIDS[@]}"; do
    kill $pid 2>/dev/null || true
done

if [ ! -z "$MASTER_PID" ]; then
    kill $MASTER_PID 2>/dev/null || true
fi

echo "=== Load Test Complete ==="
echo "Check $TEST_TMP_DIR for logs and the test database."
