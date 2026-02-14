.PHONY: help sqlc-generate sqlc-verify clean tidy test build

# Default target
help:
	@echo "Available targets:"
	@echo "  sqlc-generate   - Generate Go code from SQL queries using sqlc"
	@echo "  sqlc-verify     - Verify sqlc configuration and generated code"
	@echo "  tidy            - Run go mod tidy"
	@echo "  test            - Run all tests"
	@echo "  build           - Build all binaries"
	@echo "  clean           - Clean generated files and build artifacts"

# Generate code from SQL using sqlc
sqlc-generate:
	@echo "Generating Go code from SQL..."
	go run github.com/sqlc-dev/sqlc/cmd/sqlc generate
	@echo "Code generation complete!"

# Run go mod tidy
tidy:
	@echo "Running go mod tidy..."
	go mod tidy
	@echo "Done!"

# Run all tests
test:
	@echo "Running tests..."
	go test -v ./...

# Build all binaries
build:
	@echo "Building master API..."
	go build -o bin/master ./cmd/master
	@echo "Building PC worker..."
	go build -o bin/worker-pc ./cmd/worker-pc
	@echo "Build complete!"

# Clean generated files and build artifacts
clean:
	@echo "Cleaning generated files..."
	rm -rf bin/
	rm -f internal/database/db.go internal/database/models.go internal/database/querier.go
	@echo "Clean complete!"
