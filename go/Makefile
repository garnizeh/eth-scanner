# EthScanner Distributed - Makefile
# Provides convenient shortcuts for common development tasks

.PHONY: help build test clean sqlc run-master run-worker fmt lint clean-branches

# Default target: show help
help:
	@echo "EthScanner Distributed - Available Make Targets"
	@echo ""
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
$(MASTER_BINARY): cmd/master/*.go
	@mkdir -p $(BINARY_DIR)
	@echo "Building master..."
	@go build $(BUILD_FLAGS) -o $(MASTER_BINARY) ./cmd/master
	@echo "  → $(MASTER_BINARY)"

# Build worker binary
$(WORKER_BINARY): cmd/worker-pc/*.go
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
	@go run ./cmd/master

# Run PC worker
run-worker:
	@echo "Starting PC Worker..."
	@go run ./cmd/worker-pc

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
	@echo "Cleaning merged local branches..."
	@git branch --merged | grep -v main | xargs git branch -d
	@echo "✓ Branches cleaned"
