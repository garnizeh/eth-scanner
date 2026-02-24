# Build stage
FROM golang:1.26-alpine AS builder

# Install build dependencies for CGO (required by webp library)
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled
RUN CGO_ENABLED=1 GOOS=linux go build -o bin/master ./cmd/master

# Final stage
FROM alpine:latest

# Install CA certificates for HTTPS and timezone data
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/master /app/master

# Create necessary directories (image build-time)
RUN mkdir -p /app/data

# Set environment variables
ENV MASTER_PORT=":8081" \
	MASTER_DB_PATH="/app/data/eth-scanner.db" \
	MASTER_LOG_LEVEL="info" \
	MASTER_SHUTDOWN_TIMEOUT="30s" \
	MASTER_API_KEY="fd650fd2be06da65234a6ff116288866" \
	MASTER_TARGET_ADDRESSES="0x000000000000000000000000000000000000dEaD, 0xe968927068902222A7b3173257B476bA935Eff67, 0x9b0c45d46D386cEdD98873168C36efd0DcBa8d46" \
	MASTER_STALE_JOB_THRESHOLD=604800 \
	MASTER_CLEANUP_INTERVAL=21600 \
	WORKER_HISTORY_LIMIT=10000 \
	WORKER_DAILY_STATS_LIMIT=1000 \
	WORKER_MONTHLY_STATS_LIMIT=1000 \
	DASHBOARD_PASSWORD="3a9a7e4d194bd481bf17a09df804f3ca"

# Expose port
EXPOSE 8081

# Entrypoint will prepare runtime dirs and exec the binary
ENTRYPOINT ["/app/master"]