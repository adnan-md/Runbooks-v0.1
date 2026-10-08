package store

import (
        "context"
        "database/sql"
        "time"
)

type Worker struct {
        ID              string
        Name            string
        Status          string
        Capabilities    string
        MaxConcurrency  int
        LastHeartbeatAt sql.NullTime
        RegisteredAt    time.Time
}

type Job struct {
        ID             string
        Name           string
        Command        string
        Arguments      string
        Environment    string
        Status         string
        Priority       int
        TimeoutSeconds int
        MaxRetries     int
        RetryCount     int
        IdempotencyKey sql.NullString
        ScheduleID     sql.NullString
        CreatedBy      sql.NullString
        CreatedAt      time.Time
        UpdatedAt      time.Time
        StartedAt      sql.NullTime
        CompletedAt    sql.NullTime
        Script         sql.NullString
        ScriptLanguage sql.NullString
        TargetWorkerID sql.NullString
}

type Attempt struct {
        ID              string
        JobID           string
        AttemptNumber   int
        WorkerID        sql.NullString
        Status          string
        LeaseExpiresAt  sql.NullTime
        StartedAt       time.Time
        CompletedAt     sql.NullTime
        ExitCode        sql.NullInt64
        FailureReason   sql.NullString
}

type Schedule struct {
        ID              string
        Name            string
        CronExpression  string
        JobDefinition   string
        Enabled         bool
        PreventOverlap  bool
        LastRunAt       sql.NullTime
        NextRunAt       sql.NullTime
}

type LogSegment struct {
        ID            string
        JobID         string
        AttemptID     string
        WorkerID      sql.NullString
        SegmentNumber int
        StoragePath   string
        SizeBytes     int
        LineCount     int
        CreatedAt     time.Time
}

type WorkerRepo struct{ db *sql.DB }

func NewWorkerRepo(db *sql.DB) *WorkerRepo { return &WorkerRepo{db: db} }

func (r *WorkerRepo) Upsert(ctx context.Context, w *Worker) error {
        _, err := r.db.ExecContext(ctx, `
INSERT INTO workers(id,name,status,capabilities,max_concurrency,last_heartbeat_at,registered_at)
VALUES(?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
  name=excluded.name,
  status=excluded.status,
  capabilities=excluded.capabilities,
  max_concurrency=excluded.max_concurrency,
  last_heartbeat_at=excluded.last_heartbeat_at`,
                w.ID, w.Name, w.Status, w.Capabilities, w.MaxConcurrency, w.LastHeartbeatAt, w.RegisteredAt)
        return err
}

func (r *WorkerRepo) Heartbeat(ctx context.Context, id, status string) error {
        _, err := r.db.ExecContext(ctx,
                `UPDATE workers SET last_heartbeat_at=?, status=? WHERE id=?`,
                time.Now().UTC(), status, id)
        return err
}

func (r *WorkerRepo) MarkStale(ctx context.Context, cutoff time.Time) (int64, error) {
        res, err := r.db.ExecContext(ctx,
                `UPDATE workers SET status='UNHEALTHY' WHERE status='ACTIVE' AND last_heartbeat_at < ?`,
                cutoff)
        if err != nil {
                return 0, err
        }
        return res.RowsAffected()
}

func (r *WorkerRepo) List(ctx context.Context) ([]*Worker, error) {
        rows, err := r.db.QueryContext(ctx,
                `SELECT id,name,status,capabilities,max_concurrency,last_heartbeat_at,registered_at FROM workers ORDER BY registered_at DESC`)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        var out []*Worker
        for rows.Next() {
                w := &Worker{}
                if err := rows.Scan(&w.ID, &w.Name, &w.Status, &w.Capabilities, &w.MaxConcurrency, &w.LastHeartbeatAt, &w.RegisteredAt); err != nil {
                        return nil, err
                }
                out = append(out, w)
        }
        return out, rows.Err()
}

func (r *WorkerRepo) Get(ctx context.Context, id string) (*Worker, error) {
        w := &Worker{}
        err := r.db.QueryRowContext(ctx,
                `SELECT id,name,status,capabilities,max_concurrency,last_heartbeat_at,registered_at FROM workers WHERE id=?`, id).
                Scan(&w.ID, &w.Name, &w.Status, &w.Capabilities, &w.MaxConcurrency, &w.LastHeartbeatAt, &w.RegisteredAt)
        if err != nil {
                return nil, err
        }
        return w, nil
}

func (r *WorkerRepo) CountActive(ctx context.Context) (int, error) {
        var n int
        err := r.db.QueryRowContext(ctx,
                `SELECT COUNT(*) FROM workers WHERE status='ACTIVE'`).Scan(&n)
        return n, err
}

func (r *WorkerRepo) ListActive(ctx context.Context) ([]*Worker, error) {
        rows, err := r.db.QueryContext(ctx,
                `SELECT id,name,status,capabilities,max_concurrency,last_heartbeat_at,registered_at FROM workers WHERE status='ACTIVE' ORDER BY name`)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        var out []*Worker
        for rows.Next() {
                w := &Worker{}
                if err := rows.Scan(&w.ID, &w.Name, &w.Status, &w.Capabilities, &w.MaxConcurrency, &w.LastHeartbeatAt, &w.RegisteredAt); err != nil {
                        return nil, err
                }
                out = append(out, w)
        }
        return out, rows.Err()
}
