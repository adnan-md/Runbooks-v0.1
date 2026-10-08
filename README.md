# Runbooks

> **Status: v0.1 alpha** — actively developed, not hardened. Suitable for internal tooling by lean IT/DevOps teams or power users homelabs.

A lightweight, in-house job runner for lean IT and DevOps teams and homelab admins. Two standalone binaries — a control plane (`runbooks-server`) and a worker agent (`runbooks-worker`) — backed by embedded SQLite. No Docker, no Postgres, no Redis, no Java, no Node toolchain required. Designed for teams that want scheduled and ad-hoc script execution across a small fleet without standing up Rundeck, Jenkins, or a full DevOps platform.

## What it does

- Submit jobs via the web UI or REST API
- Workers pull jobs via HTTP long-poll and execute them
- Three execution modes: **existing command**, **inline script** (bash / sh / Python / PowerShell), or **uploaded script file** (.sh / .ps1 / .py)
- Target specific workers by ID 
- Schedules with cron expressions and overlap prevention
- Live log streaming via SSE
- Worker attribution on every attempt and log file
- Lease-based execution with automatic retry, backoff, and recovery sweep on server restart
- Process groups ensure timed-out jobs kill their entire subprocess tree
- Fleet status dashboard: online workers, running jobs, 24h failure count
- Job status filter, pagination, automatic retention
- Cross-platform binaries for Linux and Windows

## Quick Start

### 1. Start the server

```bash
./bin/runbooks-server
# 2026/09/07 INFO listening addr=:8080 db=sqlite
```

On first start it creates `runbook.db`, runs migrations, seeds a disabled `runbooks-hello-world` schedule so you can see what a schedule looks like, and starts the background monitor (lease sweeps, log cleanup, schedule cron, job retention).

Open <http://localhost:8080> in your browser.

### 2. Start a worker

```bash
./bin/runbooks-worker
# 2026/09/07 INFO worker started id=e548e552... name=host-01 server=http://localhost:8080
# NOTE!: YOU NEED TO CHANGE THE DEFAULT 'RUNBOOK_SERVER_URL' VARIABLE TO YOUR SERVER URL BEFORE YOU BUILD THE WORKER 
```

The worker creates a persistent `.worker-id` file, registers with the server, and begins long-polling `GET /api/v1/jobs/pull`.

### 3. Submit a job

From the UI (`/jobs/new`) or via the API:

```bash
# Existing command
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "hello-command",
    "command": "echo",
    "arguments": "hello world",
    "timeout_seconds": 30
  }'

# Inline bash script
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "hello-script",
    "script": "#!/bin/bash\necho hello from inline\ndate",
    "script_language": "bash",
    "timeout_seconds": 30
  }'

# Targeted to a specific worker
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "name": "db-backup",
    "command": "/usr/local/bin/backup.sh",
    "target_worker_id": "<worker-id>",
    "timeout_seconds": 3600,
    "max_retries": 2
  }'
```

## Keeping the Process Alive

Both binaries run in the foreground — closing the terminal stops them. For a lean-team setup without systemd/Windows Service, use one of these approaches:

### Linux (nohup or tmux)

```bash
# nohup 
nohup ./bin/runbooks-server > server.log 2>&1 &
nohup ./bin/runbooks-worker > worker.log 2>&1 &

# tmux 
tmux new -s runbooks-server  './bin/runbooks-server'
tmux new -s runbooks-worker  './bin/runbooks-worker'
# attach: tmux attach -t runbooks-server
```

### Windows 

```powershell
# Start in background, log to file
Start-Process -FilePath ".\bin\runbooks-server.exe" -RedirectStandardOutput "server.log" -RedirectStandardError "server.err" -NoNewWindow
Start-Process -FilePath ".\bin\runbooks-worker.exe" -RedirectStandardOutput "worker.log" -RedirectStandardError "worker.err" -NoNewWindow

# Or just run in a separate PowerShell window 
```

### Optional: write your own systemd unit or Windows Service

If you want auto-restart on crash and boot time startup, you can wrap the binary in a service unit yourself. Sample systemd unit:

```ini
# /etc/systemd/system/runbooks-worker.service
[Unit]
Description=Runbooks Worker
After=network-online.target

[Service]
Type=simple
WorkingDirectory=/var/lib/runbooks
ExecStart=/usr/local/bin/runbooks-worker
Restart=always
RestartSec=5
Environment=RUNBOOK_ID_FILE=/var/lib/runbooks/.worker-id
Environment=RUNBOOK_LOGS_DIR=/var/lib/runbooks/logs

[Install]
WantedBy=multi-user.target
```

Sample Windows Service registration (run as Administrator):

```powershell
sc.exe create runbooks-worker binPath= "C:\runbooks\runbooks-worker.exe" start= auto
sc.exe failure runbooks-worker reset= 86400 actions= restart/5000/restart/10000/restart/30000
sc.exe start runbooks-worker
```

Note: the binary doesn't register as a Windows SCM service handler itself, so `sc.exe stop` will kill it without graceful shutdown. For graceful shutdown use `Ctrl+C` in foreground mode or `Stop-Process` after sending a graceful signal. A future release may add native SCM integration.

## Build from Source

Requires Go 1.25+.

```bash
make            # builds bin/runbooks-server and bin/runbooks-worker
make test
```

The `Makefile` automatically appends `.exe` on Windows.

### Windows cross-compile from Linux/macOS

```bash
GOOS=windows GOARCH=amd64 go build -o bin/runbooks-server.exe ./cmd/runbooks-server
GOOS=windows GOARCH=amd64 go build -o bin/runbooks-worker.exe ./cmd/runbooks-worker
```

Pre-built Windows binaries are shipped alongside the source zip.

## Configuration

All configuration via environment variables with the prefix `RUNBOOK_`.

### Server

| Variable | Default | Description |
|---|---|---|
| `RUNBOOK_HTTP_ADDR` | `:8080` | Listen address |
| `RUNBOOK_DB_DRIVER` | `sqlite` | `sqlite` or `postgres` |
| `RUNBOOK_DB_DSN` | `runbook.db` | SQLite path or Postgres DSN |
| `RUNBOOK_LOGS_DIR` | `./data/logs` | Where uploaded log segments live |
| `RUNBOOK_LOG_RETENTION_DAYS` | `30` | Server-side log retention |
| `RUNBOOK_AUTH_TOKENS` | _empty_ | Comma-separated bearer tokens; if empty, auth disabled |
| `RUNBOOK_LONG_POLL_TIMEOUT` | `30` | Seconds before pull returns 204 |
| `RUNBOOK_LEASE_TIMEOUT` | `60` | Seconds before attempt lease expires |

### Worker

| Variable | Default | Description |
|---|---|---|
| `RUNBOOK_SERVER_URL` | `http://localhost:8080` | Control plane URL |
| `RUNBOOK_WORKER_NAME` | _hostname_ | Display name in UI |
| `RUNBOOK_ID_FILE` | `.worker-id` | Persistent UUID location |
| `RUNBOOK_LOGS_DIR` | `./logs` | Local log buffer |
| `RUNBOOK_MAX_LOCAL_LOGS_MB` | `500` | Refuse new jobs above this |
| `RUNBOOK_SEGMENT_SIZE_MB` | `50` | Local file rotation threshold |
| `RUNBOOK_SEGMENT_LINES` | `50000` | Local file rotation threshold |
| `RUNBOOK_HEARTBEAT_INTERVAL` | `10` | Seconds between heartbeats |
| `RUNBOOK_CONCURRENCY` | `1` | Concurrent job executions |
| `RUNBOOK_ALLOWED_EXECUTABLES` | _empty (allow all)_ | Comma-separated binaries |
| `RUNBOOK_AUTH_TOKEN` | _empty_ | Bearer token |

### Execution Modes

1. **Command** — `command: "/usr/local/bin/backup.sh"`, `arguments: "--verbose"`. Worker runs the binary directly with the argument array. No shell interpolation.
2. **Inline Script** — `script: "#!/bin/bash\n..."`, `script_language: "bash"`. Worker writes the script to a temp file under `logs/{job_id}/{attempt_id}/script.{ext}`, makes it executable, then runs `bash script.sh`, `python3 script.py`, or `pwsh -File script.ps1`.
3. **Upload** — UI form posts a `.sh` / `.ps1` / `.py` file. Server stores the file contents in the `script` column and behaves identically to inline script mode.

### Worker Targeting

By default, any worker can pull any queued job. Set `target_worker_id` on a job to restrict it to a specific worker. The dispatch query filters by `target_worker_id IS NULL OR target_worker_id = ?`.

### Recovery Sweep

On server startup and every 10 seconds, the monitor:
1. Finds attempts with `lease_expires_at < now()`, transitions the job `RUNNING → LOST → RETRYING` if retries remain.
2. Re-queues `RETRYING` jobs after backoff expires.
3. Prunes log segments older than `LOG_RETENTION_DAYS`.
4. Prunes `SUCCEEDED` jobs older than 7 days, `FAILED` / `LOST` jobs older than 30 days.

### Process Groups

Every spawned child runs in its own process group (`Setpgid: true` on Linux, `CREATE_NEW_PROCESS_GROUP` on Windows). On timeout or shutdown, the worker kills the entire group — no orphaned bash subshells, no zombie Python interpreters.

### Schedule Overlap Prevention

When `prevent_overlap` is true on a schedule, the monitor checks for any active job with the same `schedule_id` (`RUNNING`, `QUEUED`, `RETRYING`) before spawning. If any exists, the run is skipped and `schedule.skipped_overlap` is logged.

## API Reference (prefix `/api/v1`)

### Workers
- `POST /workers/register` — upsert worker by ID
- `POST /workers/{id}/heartbeat` — refresh last_heartbeat_at + status
- `GET /workers` — list all workers

### Jobs
- `POST /jobs` — submit a job (command or script + optional target_worker_id)
- `GET /jobs?limit=&offset=&status=RUNNING,FAILED` — list with pagination + status filter
- `GET /jobs/{id}` — full job record
- `GET /jobs/{id}/attempts` — attempt history (includes worker_id)
- `POST /jobs/{id}/retry` — manual retry from `FAILED` / `LOST`
- `GET /jobs/pull?worker_id=<id>` — long-poll for next claim (204 if nothing in 30s)
- `POST /jobs/complete` — completion with exit code + reason
- `POST /jobs/renew-lease` — extend current attempt lease

### Logs
- `GET /jobs/{id}/logs` — segment metadata
- `GET /jobs/{id}/logs/content` — concatenated plaintext
- `GET /jobs/{id}/logs/sse` — Server-Sent Events stream (appends segments as they arrive)
- `POST /jobs/{id}/attempts/{attempt_id}/logs` — upload one segment (binary body)

### Schedules
- `POST /schedules` — create
- `GET /schedules` — list
- `GET /schedules/{id}` — fetch
- `DELETE /schedules/{id}` — remove

## What's Not Implemented (Honest Gaps)

- **No native service install** — the binary runs in foreground only. Use `nohup`, `tmux`, `Start-Process`, or write your own systemd unit / Windows Service wrapper.
- **No RBAC** — single shared bearer token. No per-user identity, no audit log of who clicked run.
- **No TLS** — workers register over HTTP. Use a reverse proxy (Caddy, nginx) for production.
- **No notifications** — no Slack/email/webhook on failure. Check the dashboard.
- **No Postgres `FOR UPDATE SKIP LOCKED`** — the SQLite `BEGIN TRANSACTION` claim path works; the Postgres path is stubbed but untested.
- **Single-node server** — server is not HA. If it crashes, workers can't pull jobs until it restarts (existing running jobs continue to completion).
- **No artifact handling** — if a job produces a 5 GB tarball, Runbooks can't collect it.
- the gaps are actively being fixed , such as adding TLS,more robust auth,notifications,etc.. , this version was meant to be a prototype 

## Development

```bash
go test ./...
make clean && make
```

Migrations live in `internal/store/migrations/*.up.sql` and are applied idempotently on server startup.
