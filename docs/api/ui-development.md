# Dashboard & UI Development Guide

This document outlines the architecture and development patterns for the EthScanner monitoring dashboard (Phase 10).

## Tech Stack
- **Go `html/template`**: Server-side rendering for type-safety and performance.
- **Tailwind CSS**: Utility-first CSS via a standalone CLI or CDN.
- **HTMX**: High-performance AJAX interaction with zero-JavaScript boilerplate.
- **WebSockets via `github.com/coder/websocket`**: Used for real-time updates of throughput and worker stats.

## Directory Structure
- `go/internal/server/ui/templates/`: HTML templates.
- `go/internal/server/ui/static/`: Static assets (images, CSS, frontend libraries).
- `go/internal/server/ui_handlers.go`: Backend Go handlers that serve the dashboard pages.
- `go/internal/server/hub.go`: WebSocket message hub for broadcasting events.

## Key HTMX Patterns

### 1. Polling & Partial Swaps
The dashboard uses HTMX to refresh specific fragments of the page (like a worker's last seen time) without a full page reload.

```html
<div hx-get="/dashboard/widgets/worker-list" hx-trigger="every 5s" hx-swap="innerHTML">
  <!-- Worker list rendered here -->
</div>
```

### 2. Real-time Updates via WebSockets (OOB Swaps)
For truly real-time updates (e.g., "live throughput"), we use HTMX Out-of-Band (OOB) swaps triggered by WebSocket messages.
When a worker submits a checkpoint, the `hub.go` broadcasts a fragment for the "Cumulative Throughput" widget.

```html
<!-- Sent via WebSocket -->
<div id="live-throughput" hx-swap-oob="innerHTML">
  1,234,567 keys/sec
</div>
```

## How to Add a New Statistic or Component

### Step 1: Add the SQL Query
Add the necessary aggregation logic in `go/internal/database/sql/queries.sql`.
```sql
-- name: GetTopWorkers :many
SELECT worker_id, total_keys FROM worker_stats_lifetime ORDER BY total_keys DESC LIMIT 5;
```
Run `make sqlc` in the `go/` directory to generate the Go code.

### Step 2: Fetch Data in the Handler
Update `go/internal/server/ui_handlers.go` to fetch the data and include it in the template context.

### Step 3: Create a Template Component
Add a new template fragment in `go/internal/server/ui/templates/fragments.html` or a new file.

### Step 4: Add to the Dashboard
Include the component in `go/internal/server/ui/templates/index.html` (the main dashboard page).

## Troubleshooting

### WebSocket/CORS Issues
- Ensure the `MASTER_PORT` environment variable is correctly set; the frontend calculates the `ws://` URL based on `location.host`.
- If running behind a reverse proxy (like Nginx), ensure `Upgrade` and `Connection` headers are forwarded correctly.

### Template Formatting
- Changes to templates require the Go server to be restarted for the changes to take effect (unless using a development auto-reloader).
- Ensure all Go variables being passed to the templates are capitalized (exported).

## CSS Development (Tailwind)
The dashboard uses Tailwind CSS. For rapid development, the project includes a script in `go/Makefile` to monitor and rebuild the CSS bundle.

```bash
cd go
make tailwind-watch
```

This watches the `go/internal/server/ui/templates/` folder and generates a minified `go/internal/server/ui/static/css/output.css`.
