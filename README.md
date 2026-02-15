# EthScanner Distributed (MVP)

EthScanner Distributed is an educational research project demonstrating a distributed key-scanning architecture (Master API + heterogeneous workers: PC and ESP32).

**Project Status:** 🟢 Phase 4 Complete - Master API is fully functional with Job Management, Dynamic Batching, and Checkpointing. PC and ESP32 workers are currently in development (Phase 5+).

IMPORTANT: This project is for research/educational purposes only. Do NOT use it against real wallets with funds. Brute-forcing private keys is computationally infeasible and unethical when targeting active addresses.

## Quick Links
- **Documentation:** [docs/architecture/system-design-document.md](docs/architecture/system-design-document.md)
- **API Reference:** Detailed in [System Design Document](docs/architecture/system-design-document.md#api-endpoints).
- **Tasks (Backlog):** [docs/tasks/backlog](docs/tasks/backlog)
- **Tasks (Done):** [docs/tasks/done](docs/tasks/done)
- **Tasks Overview:** [docs/tasks/OVERVIEW.md](docs/tasks/OVERVIEW.md)

## Getting Started (Linux)

### Prerequisites
- **Go 1.26+**
- **Git**

### Configuration
The Master API is configured via environment variables. You can set them in your shell or use the defaults provided in the `Makefile`.

| Variable | Description | Default |
|----------|-------------|---------|
| `MASTER_DB_PATH` | Path to the SQLite database file (Required) | `./data/eth-scanner.db` |
| `MASTER_PORT` | TCP port for the API server | `8080` |
| `MASTER_API_KEY` | Secret key for API authentication | (Disabled if empty) |
| `MASTER_LOG_LEVEL`| Logging verbosity (`debug`, `info`, `warn`, `error`) | `info` |

### Running the Master API
The database is initialized automatically with all necessary migrations on the first run. Ensure the directory for your database file exists.

```bash
cd go
# Optional: Create data directory
mkdir -p data

# Option A: Using Makefile (uses defaults)
make run-master

# Option B: Manual run with env vars
MASTER_DB_PATH=./data/eth-scanner.db go run ./cmd/master
```

### Authentication
Endpoints (except `/health`) require an `X-API-KEY` header if `MASTER_API_KEY` is configured.

```bash
curl -H "X-API-KEY: your-secret-key-here" http://localhost:8080/api/v1/jobs/lease
```

## Repository Layout
```
eth-scanner/
├── README.md
├── docs/                       # Comprehensive documentation
│   ├── architecture/           # System design and API contracts
│   ├── database/               # SQL schema and queries
│   └── tasks/                  # Task board (Backlog/Done)
├── go/                         # Master API & PC Worker (Go)
│   ├── cmd/                    # Entry points (master, worker-pc)
│   ├── internal/               # Core logic (database, config, server, jobs)
│   └── Makefile                # Development shortcuts
└── esp32/                      # ESP32 firmware (C++/Arduino) - PLANNED
```

## Project Progress
- [x] **Phase 1: Foundation** - Repository structure and tooling.
- [x] **Phase 2: Database Layer** - Type-safe SQL with `sqlc` and pure Go SQLite.
- [x] **Phase 3: Core Infrastructure** - HTTP server, logging, and configuration.
- [x] **Phase 4: Job Management** - Lease, Checkpoint, and Complete logic.
- [ ] **Phase 5-6: PC Worker** - (Planned) High-throughput scanning implementation.
- [ ] **Phase 7-8: ESP32 Worker** - (Planned) Resource-constrained device support.
- [ ] **Phase 9: Integration, Testing & Validation** - (Planned) integration tests, benchmarks, and hardware validation.
- [ ] **Phase 10: Documentation, Deployment & Monitoring** - (Planned) API docs, deployment guides, and observability.

## Development Commands (Go)
Within the `go/` directory:
- `make build`: Build binaries for master and worker.
- `make test`: Run all unit tests.
- `make fmt`: Format Go code.
- `make sqlc`: Re-generate database code from SQL definitions.

## Ethics & Disclaimer
- Use only on synthetic or "dead" addresses (e.g., `0x000...dEaD`) for demos.
- This project demonstrates architectural patterns and fault-tolerance; it is not intended to be used for malicious activity.

## License
This project is licensed under the MIT License — see the [LICENSE](LICENSE) file.
