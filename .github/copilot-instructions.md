# Copilot Instructions: Distributed Ethereum Key Scanner (MVP)

## 0. Golden Rules
- **Verify State:** Meticulously verify the current project state (files, branches, builds) before making recommendations to avoid incorrect assumptions.
- **English Only:** All generated content (code, comments, filenames, documentation) and chat responses must be in English.
- **Ask First:** When in doubt, ask simple yes/no questions; wait for the user's answer before proceeding.
- **MVP-First:** Prioritize only what is required for a working MVP. Defer non-essential features.
- **Efficiency:** Prioritize CPU, memory, and storage efficiency above all else. Every byte matters.
- **Minimal Tech Stack:** Keep the stack simple and maintainable.
- **Docs Location:** All project documentation (except the project's root README) must be created inside the `docs/` directory, organized into a sensible subfolder structure to keep content tidy.
- **Workspace Layout (VS Code):** This project is organized as a VS Code workspace. The repository root should be opened as a workspace in VS Code.
    - The `esp32-worker/` folder contains the ESP32 firmware and C++/Arduino/FreeRTOS code.
    - The `cmd/master/` folder contains the Master API server (Go).
    - The `cmd/worker-pc/` folder contains the PC worker implementation (Go).

## 1. Core Tech Stack
- **Backend (Master/PC Worker):** Go (Golang).
- **Router:** `net/http` or `chi`.
- **Database:** SQLite (Embedded). Use `sqlc` for type-safe SQL. **Strictly No CGO** (use `modernc.org/sqlite`).
- **ESP32 Worker:** C++ (Arduino Core) with FreeRTOS.

## 2. Go Code Style & Implementation
- **Simplicity:** Keep it simple and idiomatic. Avoid unnecessary abstractions.
- **Project Structure:** Use the `internal/` folder pattern for core logic.
- **Concurrency:** Use `context.Context` for all cancellations and timeouts.
- **Time Handling:** - Always use **UTC** for all timestamps. 
    - Use `time.Now().UTC()`. 
    - Store all times in the database as UTC. 
    - Never use `time.Local`.
- **Database:** Use `sqlc` to generate code from raw SQL. Avoid heavy ORMs.

## 3. Worker Specifications

### A. ESP32 Worker (C++/FreeRTOS)
- **Multithreading:** Use `xTaskCreatePinnedToCore`.
    - **Core 0:** Networking (WiFi, API communication, Watchdog).
    - **Core 1:** Computation (The "Hot Loop" for key generation).
- **Cryptography:** Use `trezor-crypto` or `micro-ecc` for optimized `secp256k1` and `keccak256`.
- **Memory:** Use static buffers (`char[]`) and avoid `std::string` or `String` class to prevent heap fragmentation.

### B. PC Worker (Go)
- **Paralellism:** Scale using `runtime.NumCPU()` and a pool of worker goroutines.
- **Crypto:** Use `github.com/ethereum/go-ethereum/crypto`.

## 4. Distributed Scanning Logic (Space Partitioning)
- **Private Key:** 32 bytes (256 bits).
- **Master Strategy:** The API manages the **Prefix** (e.g., first 4 to 8 bytes).
- **Worker Strategy:** The worker receives a prefix and iterates through the remaining bytes.
- **Lease System:** The database tracks jobs with `status` (`pending`, `processing`, `completed`) and an `expires_at` (UTC) to handle worker timeouts.

## 5. Implementation Patterns for AI
- **SQL Generation:** "Write a SQL schema for a `jobs` table including `prefix`, `status`, `worker_id`, and `expires_at` (UTC), then provide the `sqlc` query to claim an expired job."
- **ESP32 Task:** "Create a FreeRTOS task pinned to Core 1 that iterates through a 24-byte suffix given an 8-byte prefix."