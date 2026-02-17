# EthScanner Distributed - Makefile
# Provides convenient shortcuts for common development tasks

.PHONY: help all tidy vuln build test clean sqlc run-master run-worker fmt lint clean-branches

# Git configuration for clean-branches
REMOTE = origin
DEFAULT_BRANCH = main

# Default runtime environment variables (can be overridden in environment)
MASTER_PORT ?= 8080
MASTER_DB_PATH ?= ./data/eth-scanner.db
MASTER_LOG_LEVEL ?= info
MASTER_SHUTDOWN_TIMEOUT ?= 30s
MASTER_API_KEY ?=
MASTER_TARGET_ADDRESS ?= 0x000000000000000000000000000000000000dEaD
MASTER_STALE_JOB_THRESHOLD ?= 604800
MASTER_CLEANUP_INTERVAL ?= 21600

# Worker runtime defaults (can be overridden in environment)
WORKER_API_URL ?= http://localhost:8080
WORKER_ID ?= worker-pc-local
WORKER_API_KEY ?=
WORKER_CHECKPOINT_INTERVAL ?= 5m
WORKER_LEASE_GRACE_PERIOD ?= 30s
WORKER_TARGET_JOB_DURATION ?= 3600
WORKER_MIN_BATCH_SIZE ?= 100000
WORKER_MAX_BATCH_SIZE ?= 10000000
WORKER_BATCH_ADJUST_ALPHA ?= 0.5
WORKER_INITIAL_BATCH_SIZE ?= 0
WORKER_INTERNAL_BATCH_SIZE ?= 1000000
WORKER_HISTORY_LIMIT ?= 10000
WORKER_DAILY_STATS_LIMIT ?= 1000
WORKER_MONTHLY_STATS_LIMIT ?= 1000

# Default target: show help
help:
	@echo "EthScanner Distributed - Available Make Targets"
	@echo ""
	@echo "  make all          - Run full CI pipeline (tidy, fmt, lint, vuln, sqlc, build, test)"
	@echo "  make vuln         - Check for vulnerabilities in dependencies"
	@echo "  make build        - Build master and worker binaries"
	@echo "  make test         - Run all unit tests"
	@echo "  make sqlc         - Generate database code from SQL"
	@echo "  make run-master   - Run the Master API server"
	@echo "  make run-worker   - Run the PC Worker"
	@echo "  make fmt          - Format Go code"
	@echo "  make lint         - Run linter (requires golangci-lint)"
	@echo "  make clean        - Remove build artifacts"
	@echo "  make clean-branches - Clean merged local branches"
	@echo ""

# Run full CI pipeline
all: tidy fmt lint vuln sqlc build test
	@echo "✓ All checks passed"

# Tidy Go modules
tidy:
	@echo "Tidying Go modules..."
	@go mod tidy
	@echo "✓ Modules tidied"

# Check for vulnerabilities
vuln:
	@echo "Checking for vulnerabilities..."
	@go tool govulncheck ./...
	@echo "✓ Vulnerability check complete"

# Build configuration
BINARY_DIR = bin
MASTER_BINARY = $(BINARY_DIR)/master
WORKER_BINARY = $(BINARY_DIR)/worker-pc

# Ensure CGO is disabled for all builds
export CGO_ENABLED = 0

# Go build flags
BUILD_FLAGS = -ldflags="-s -w"

# Build both master and worker
build: $(MASTER_BINARY) $(WORKER_BINARY)
	@echo "✓ Build complete"

# Build master binary
$(MASTER_BINARY):
	@mkdir -p $(BINARY_DIR)
	@echo "Building master..."
	@go build $(BUILD_FLAGS) -o $(MASTER_BINARY) ./cmd/master
	@echo "  → $(MASTER_BINARY)"

# Build worker binary
$(WORKER_BINARY):
	@mkdir -p $(BINARY_DIR)
	@echo "Building worker..."
	@go build $(BUILD_FLAGS) -o $(WORKER_BINARY) ./cmd/worker-pc
	@echo "  → $(WORKER_BINARY)"

# Run all tests
test:
	@echo "Running tests..."
	@CGO_ENABLED=1 go test -race -v -cover ./...

# Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	@CGO_ENABLED=1 go -race test -v -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"

# Generate sqlc code
sqlc:
	@echo "Generating sqlc code..."
	@sqlc generate
	@echo "✓ Database code generated"

# Run master API server
run-master:
	@echo "Starting Master API server..."
	@MASTER_PORT=$(MASTER_PORT) \
	MASTER_DB_PATH=$(MASTER_DB_PATH) \
	MASTER_LOG_LEVEL=$(MASTER_LOG_LEVEL) \
	MASTER_SHUTDOWN_TIMEOUT=$(MASTER_SHUTDOWN_TIMEOUT) \
	MASTER_API_KEY=$(MASTER_API_KEY) \
	MASTER_TARGET_ADDRESS=$(MASTER_TARGET_ADDRESS) \
	MASTER_STALE_JOB_THRESHOLD=$(MASTER_STALE_JOB_THRESHOLD) \
	MASTER_CLEANUP_INTERVAL=$(MASTER_CLEANUP_INTERVAL) \
	WORKER_HISTORY_LIMIT=$(WORKER_HISTORY_LIMIT) \
	WORKER_DAILY_STATS_LIMIT=$(WORKER_DAILY_STATS_LIMIT) \
	WORKER_MONTHLY_STATS_LIMIT=$(WORKER_MONTHLY_STATS_LIMIT) \
	MASTER_PORT=$(MASTER_PORT) \
	go run ./cmd/master

# Run PC worker
run-worker:
	@echo "Starting PC Worker..."
	@WORKER_API_URL=$(WORKER_API_URL) \
	WORKER_ID=$(WORKER_ID) \
	WORKER_API_KEY=$(WORKER_API_KEY) \
	WORKER_CHECKPOINT_INTERVAL=$(WORKER_CHECKPOINT_INTERVAL) \
	WORKER_LEASE_GRACE_PERIOD=$(WORKER_LEASE_GRACE_PERIOD) \
	WORKER_TARGET_JOB_DURATION=$(WORKER_TARGET_JOB_DURATION) \
	WORKER_MIN_BATCH_SIZE=$(WORKER_MIN_BATCH_SIZE) \
	WORKER_MAX_BATCH_SIZE=$(WORKER_MAX_BATCH_SIZE) \
	WORKER_BATCH_ADJUST_ALPHA=$(WORKER_BATCH_ADJUST_ALPHA) \
	WORKER_INITIAL_BATCH_SIZE=$(WORKER_INITIAL_BATCH_SIZE) \
	WORKER_INTERNAL_BATCH_SIZE=$(WORKER_INTERNAL_BATCH_SIZE) \
	WORKER_HISTORY_LIMIT=$(WORKER_HISTORY_LIMIT) \
	WORKER_DAILY_STATS_LIMIT=$(WORKER_DAILY_STATS_LIMIT) \
	WORKER_MONTHLY_STATS_LIMIT=$(WORKER_MONTHLY_STATS_LIMIT) \
	go run ./cmd/worker-pc

# Format Go code
fmt:
	@echo "Formatting Go code..."
	@go fmt ./...
	@echo "✓ Code formatted"

# Run linter (requires golangci-lint)
lint:
	@echo "Running linter..."
	@golangci-lint run ./...

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BINARY_DIR)
	@rm -f coverage.out coverage.html
	@rm -f ../data/*.db
	@echo "✓ Clean complete"

# Development: build and run master
dev-master: build
	@$(MASTER_BINARY)

# Development: build and run worker
dev-worker: build
	@$(WORKER_BINARY)

# Install development dependencies
deps:
	@echo "Installing development dependencies..."
	@go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	@echo "✓ Dependencies installed"

# Verify Go environment (no CGO)
verify:
	@echo "Verifying Go environment..."
	@echo "  Go version: $$(go version)"
	@echo "  CGO_ENABLED: $(CGO_ENABLED)"
	@go list -m modernc.org/sqlite > /dev/null 2>&1 && echo "  ✓ modernc.org/sqlite available" || echo "  ✗ modernc.org/sqlite not found"
	@echo "✓ Environment verified"

# Clean merged local branches
clean-branches:
	@echo "Fetching latest refs from $(REMOTE)..."
	@git fetch --prune $(REMOTE)
	@for b in `git branch --merged $(REMOTE)/$(DEFAULT_BRANCH) | grep -vE "(^\*|\b$(DEFAULT_BRANCH)\b|\bmain\b|\bdevelop\b)" | sed 's/^..//'`; do \
		printf "Deleting local branch '%s'\n" "$$b"; \
		git branch -d "$$b"; \
	done
