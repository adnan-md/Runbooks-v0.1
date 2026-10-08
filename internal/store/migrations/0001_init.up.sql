CREATE TABLE IF NOT EXISTS workers (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    capabilities TEXT,
    max_concurrency INTEGER NOT NULL DEFAULT 1,
    last_heartbeat_at DATETIME,
    registered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    command TEXT NOT NULL,
    arguments TEXT,
    environment TEXT,
    status TEXT NOT NULL DEFAULT 'PENDING',
    priority INTEGER NOT NULL DEFAULT 0,
    timeout_seconds INTEGER NOT NULL DEFAULT 3600,
    max_retries INTEGER NOT NULL DEFAULT 0,
    retry_count INTEGER NOT NULL DEFAULT 0,
    idempotency_key TEXT UNIQUE,
    schedule_id TEXT,
    created_by TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at DATETIME,
    completed_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_priority ON jobs(priority DESC);
CREATE INDEX IF NOT EXISTS idx_jobs_idempotency ON jobs(idempotency_key);

CREATE TABLE IF NOT EXISTS job_attempts (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL,
    worker_id TEXT REFERENCES workers(id),
    status TEXT NOT NULL,
    lease_expires_at DATETIME,
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    exit_code INTEGER,
    failure_reason TEXT,
    UNIQUE(job_id, attempt_number)
);

CREATE INDEX IF NOT EXISTS idx_attempts_job ON job_attempts(job_id);
CREATE INDEX IF NOT EXISTS idx_attempts_lease ON job_attempts(lease_expires_at);

CREATE TABLE IF NOT EXISTS schedules (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    cron_expression TEXT NOT NULL,
    job_definition TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    prevent_overlap INTEGER NOT NULL DEFAULT 1,
    last_run_at DATETIME,
    next_run_at DATETIME
);

CREATE TABLE IF NOT EXISTS log_segments (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    worker_id TEXT,
    segment_number INTEGER NOT NULL,
    storage_path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    line_count INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_segments_job ON log_segments(job_id);
CREATE INDEX IF NOT EXISTS idx_segments_attempt ON log_segments(attempt_id);
