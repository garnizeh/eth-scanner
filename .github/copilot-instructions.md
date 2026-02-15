# Copilot Instructions: Distributed Ethereum Key Scanner (MVP)

## 0. Golden Rules
- **Verify State:** Meticulously verify the current project state (files, branches, builds) before making recommendations to avoid incorrect assumptions.
- **English Only:** All generated content (code, comments, filenames, documentation) and chat responses must be in English.
- **Ask First:** When in doubt, ask simple yes/no questions; wait for the user's answer before proceeding.
- **MVP-First:** Prioritize only what is required for a working MVP. Defer non-essential features.
- **Efficiency:** Prioritize CPU, memory, and storage efficiency above all else. Every byte matters.
- **Test Failure Cases:** Be meticulous when creating tests: always include unit tests for failure and edge cases in addition to success paths. For any function that performs validation, parsing, HTTP requests, or error wrapping, add negative tests that assert proper handling of invalid inputs, expected error messages, and error unwrapping. Examples: invalid base URL parsing, malformed JSON responses, HTTP 4xx/5xx handling (including 401 Unauthorized), and verifying custom error types' Error() output.
  
    Additional testing rules to avoid common linter failures:
    - Always check returned errors from writers/encoders/closers in tests. For example, check the error returned by `json.NewEncoder(w).Encode(...)` and fail the test if it returns an error. This avoids `errcheck` failures.
    - When asserting specific error types or sentinel errors, use `errors.As` and `errors.Is` respectively to support wrapped errors; do not use direct type assertions or `==` comparisons on `error` values. This avoids `errorlint` failures.
    - In test HTTP handlers, if a parameter is unused, name it `_` (for example `func(w http.ResponseWriter, _ *http.Request)`) to avoid unused-parameter warnings from revive.
    - If a function legitimately triggers an `unparam` false positive (e.g., widely used API but tests pass a constant), add a localized `//nolint:unparam` comment above the function and document why in a nearby code comment.
- **Minimal Tech Stack:** Keep the stack simple and maintainable.
- **Docs Location:** All project documentation (except the project's root README) must be created inside the `docs/` directory, organized into a sensible subfolder structure to keep content tidy.
- **Consult System Design Document:** Always consult `docs/architecture/system-design-document.md` whenever you need a project-wide reference or to align expectations with ongoing development.
- **Task Tracking:** Use `docs/tasks/backlog` for pending and in-progress tasks and `docs/tasks/done` for completed tasks. Tasks in `docs/tasks/backlog` must be sequentially numbered and worked on in numeric order; these folders are the single source of truth for project state and task workflow.
    - **Follow Tasks Overview:** When creating, updating, or executing tasks, strictly follow the definitions, workflow, and naming conventions in `docs/tasks/OVERVIEW.md`. Treat files in `docs/tasks/backlog/` and `docs/tasks/done/` as the authoritative task state; update task files and move them between folders to reflect status changes.
        - **Git Workflow for Tasks:** When starting a new task, always switch to the `main` branch, update it from origin, and create a dedicated local branch for the task. Example workflow:
            1. `git checkout main`
            2. `git fetch origin --prune`
            3. `git pull origin main`
            4. `git checkout -b task/P01-T010-short-description`
            Work on the branch and update the corresponding task file in `docs/tasks/backlog/`.
        - **Automation / Agent checklist:** When an automated agent (Copilot) runs tasks, it MUST perform the following checks and actions automatically before making edits or commits:
            1. Inspect the current Git branch. If the branch is not `main` or not the intended task branch, do NOT proceed silently — switch to `main`, update it, then create the task branch.
            2. Locate the task file in `docs/tasks/backlog/` corresponding to the task ID (e.g., `P01-T020.md`) and confirm the task is in `Not Started` or `In Progress` state. If the task file is missing or status is not appropriate, abort and report.
            3. Verify prerequisite tasks (e.g., `P01-T010`) are completed when required — for example check presence of `go/go.mod` to confirm `P01-T010`. If prerequisites are missing, abort and report the missing dependency.
            4. Create the task branch using the naming convention `task/<TASK-ID>-short-description` from an up-to-date `main` branch. Do not create branches off feature or unrelated branches.
            5. After changes are made, update the task file status and move it to `docs/tasks/done/` only when acceptance criteria are met. Commit messages MUST include the task ID (e.g., `P01-T020: Create internal folder structure — in progress`).
            6. If the agent cannot verify any of the above automatically, stop and ask the user; do not proceed.
        - **Commit & Push After Completion:** After a task is finished (all acceptance criteria met, tests passing where applicable, and the task document updated), commit your changes on the task branch and push to origin. Use a clear commit message referencing the task ID (e.g., `P01-T010: Initialize Go module — complete`). Optionally open a pull request for review.
- **Workspace Layout (VS Code):** This project is organized as a VS Code workspace. The repository root should be opened as a workspace in VS Code.
    - The `esp32/` folder contains the ESP32 firmware and C++/Arduino/FreeRTOS code.
    - The `go/cmd/master/` folder contains the Master API server (Go).
    - The `go/cmd/worker-pc/` folder contains the PC worker implementation (Go).
- **Absolute file paths:** When invoking tools or providing file paths (especially in automated edits or terminal commands), always use the absolute path to the file in the workspace (for example: `/home/user/code/garnizeh/eth-scanner/go/test-sqlite.go`). This repository uses the VS Code workspace feature; do not rely on relative paths or assumptions about the current working directory.

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
- **Testing:** Use `t.TempDir()` for temporary directories in all tests that need to create files.
- **Error Handling:** Never ignore or discard errors returned by functions; the linter will complain. Always add context to errors using `fmt.Errorf` with `%w` for wrapping, e.g., `return fmt.Errorf("failed to set goose dialect: %w", err)`.

## 3. Worker Specifications

### A. ESP32 Worker (C++/FreeRTOS)
- **Multithreading:** Use `xTaskCreatePinnedToCore`.
    - **Core 0:** Networking (WiFi, API communication, Watchdog).
    - **Core 1:** Computation (The "Hot Loop" for key generation).
- **Cryptography:** Use `trezor-crypto` or `micro-ecc` for optimized `secp256k1` and `keccak256`.
- **Memory:** Use static buffers (`char[]`) and avoid `std::string` or `String` class to prevent heap fragmentation.

### B. PC Worker (Go)
- **Parallelism:** Scale using `runtime.NumCPU()` and a pool of worker goroutines.
- **Crypto:** Use `github.com/ethereum/go-ethereum/crypto`.
- **Dynamic Batch Sizing:** Request batch size based on measured/estimated throughput (targeting approximately 1 hour of work per lease).

## 4. Distributed Scanning Logic (Space Partitioning)
- **Private Key:** 32 bytes (256 bits).
- **Master Strategy:** The API manages the **global 28-byte prefix** and allocates worker nonce ranges.
- **Worker Strategy:** The worker receives `prefix_28` and scans a **4-byte nonce range** (`nonce_start` to `nonce_end`).
- **Dynamic Batching:** Workers request `requested_batch_size` according to device capacity (PC and ESP32 should receive different batch sizes).
- **Lease System:** The database tracks jobs with `status` (`pending`, `processing`, `completed`), `worker_id`, `current_nonce`, and `expires_at` (UTC) to handle worker timeouts and resume.
- **Checkpointing:** Workers must periodically report progress (`current_nonce`, `keys_scanned`) to minimize rework after failures.

## 5. Implementation Patterns for AI
- **SQL Generation:** "Write a SQL schema for a `jobs` table including `prefix_28`, `nonce_start`, `nonce_end`, `current_nonce`, `status`, `worker_id`, and `expires_at` (UTC), then provide `sqlc` queries to lease a pending/expired job and update checkpoints."
- **API Flow:** "Implement `POST /api/v1/jobs/lease`, `PATCH /api/v1/jobs/{job_id}/checkpoint`, and `POST /api/v1/jobs/{job_id}/complete` following UTC lease expiration semantics."
- **ESP32 Task:** "Create a FreeRTOS task pinned to Core 1 that iterates nonce values over a 4-byte range while reusing static 32-byte key buffers (`prefix_28 + nonce`)."