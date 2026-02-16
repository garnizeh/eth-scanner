EthScanner Distributed — distributed key-scanning research prototype

Short description

A minimal, efficient distributed system that demonstrates scalable job leasing, checkpointing, and multi-tier performance analytics for high-throughput key-scanning workloads. Built as an educational research project (Master API in Go, PC and ESP32 workers).

Why this project exists

- Explore fault-tolerant job leasing and dynamic batch allocation at scale.
- Demonstrate storage-efficient checkpointing and long-term analytics via a multi-tier stats architecture (raw → daily → monthly → lifetime).
- Provide a reproducible platform for benchmarking worker throughput and adaptive batch sizing.

Quick facts

- Language: Go (Master API & PC worker), C++/Arduino (ESP32 firmware)
- Database: SQLite (pure-Go driver, modernc.org/sqlite)
- Key features: long-lived macro jobs, checkpointing, configurable retention, automatic aggregation, dashboard-ready statistics
- Ethics: Educational only — do not target real wallets with funds.

Get started

See README.md for quick start, configuration, and development commands:

- README: ./README.md
- Design & schema: ./docs/architecture/db-optimization-proposal.md
- Tasks & roadmap: ./docs/tasks/OVERVIEW.md