# EthScanner Distributed (MVP)

EthScanner Distributed is an educational research project demonstrating a distributed key-scanning architecture (Master API + heterogeneous workers: PC and ESP32). This repository contains the Master API (Go), a PC worker (Go), and ESP32 firmware (C++/Arduino).

IMPORTANT: This project is for research/educational purposes only. Do NOT use it against real wallets with funds. Brute-forcing private keys is computationally infeasible and unethical when targeting active addresses.

Quick links
- Documentation: `docs/architecture/system-design-document.md`
- Go code: `go/`
- ESP32 firmware: `esp32/`
- Scripts: `scripts/` (e.g., `scripts/init-db.sh`)
- Tasks (backlog): `docs/tasks/backlog` (pending / in-progress tasks — sequentially numbered; work is performed in numeric order)
- Tasks (done): `docs/tasks/done` (completed tasks)
- Tasks overview: `docs/tasks/OVERVIEW.md` (phases, task naming, workflow and templates)

Getting started (Linux)

Prerequisites
- Go 1.26+ (or latest stable)
- Git
- For ESP32: Arduino IDE or PlatformIO with ESP32 toolchain

Initialize database (SQLite)

```
# from project root
./scripts/init-db.sh
```

Run Master API (development)

```
# from project root
cd go
# run the master API (example entrypoint)
go run ./cmd/master
```

Run PC worker (development)

```
# in a separate terminal
cd go
go run ./cmd/worker-pc
```

ESP32 firmware

- Open `esp32/esp32-worker.ino` in Arduino IDE or import the folder into PlatformIO.
- Configure WiFi and API URL in `esp32/config.h` (if present).
- Compile and flash to your ESP32 board.

Repository layout (high level)

```
eth-scanner/
├── README.md
├── docs/                       # documentation
├── go/                         # Go server and PC worker
│   ├── cmd/
│   └── internal/
├── esp32/                      # ESP32 firmware (Arduino)
├── scripts/                    # helper scripts (init-db, populate jobs)
└── .github/
```

Ethics & disclaimer
- Use only on synthetic or "dead" addresses (e.g., `0x000...dEaD`) for demos.
- This project demonstrates architectural patterns and fault-tolerance; it is not intended to be used for malicious activity.

Contributing
- Open issues or PRs for documentation improvements, tooling, or small fixes.

License
- This project is licensed under the MIT License — see the `LICENSE` file.

Quick setup notes

Go (Master API & PC worker)

```
# Ensure Go is installed (1.26+ recommended)
go version

# from project root: initialize DB (creates SQLite DB)
./scripts/init-db.sh

# run the Master API (development)
cd go
go run ./cmd/master

# run the PC worker in a separate terminal
go run ./cmd/worker-pc
```

ESP32 firmware (Arduino / PlatformIO)

- Open `esp32/esp32-worker.ino` in Arduino IDE or import the folder into PlatformIO.
- Edit `esp32/config.h` to set your WiFi credentials and the Master API URL.
- Build and flash to an ESP32 board using your toolchain (Arduino IDE or `pio run --target upload`).

Notes
- This README is a minimal starter; see `docs/architecture/system-design-document.md` for design details and API contracts.
- If you want a different license owner/year in `LICENSE`, update the file accordingly.
