package store

import (
        "context"
        "database/sql"
        "time"
)

type JobRepo struct{ db *sql.DB }

func NewJobRepo(db *sql.DB) *JobRepo { return &JobRepo{db: db} }

const jobColumns = `id,name,command,arguments,environment,status,priority,timeout_seconds,max_retries,retry_count,idempotency_key,schedule_id,created_by,created_at,updated_at,started_at,completed_at,script,script_language,target_worker_id`

func (r *JobRepo) Create(ctx context.Context, j *Job) error {
        _, err := r.db.ExecContext(ctx, `
INSERT INTO jobs(id,name,command,arguments,environment,status,priority,timeout_seconds,max_retries,retry_count,idempotency_key,schedule_id,created_by,created_at,updated_at,script,script_language,target_worker_id)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
                j.ID, j.Name, j.Command, j.Arguments, j.Environment, j.Status, j.Priority,
                j.TimeoutSeconds, j.MaxRetries, j.RetryCount, j.IdempotencyKey, j.ScheduleID,
                j.CreatedBy, j.CreatedAt, j.UpdatedAt, j.Script, j.ScriptLanguage, j.TargetWorkerID)
        return err
}

func (r *JobRepo) Get(ctx context.Context, id string) (*Job, error) {
        j := &Job{}
        err := r.db.QueryRowContext(ctx,
                `SELECT `+jobColumns+` FROM jobs WHERE id=?`, id).Scan(
                &j.ID, &j.Name, &j.Command, &j.Arguments, &j.Environment, &j.Status, &j.Priority,
                &j.TimeoutSeconds, &j.MaxRetries, &j.RetryCount, &j.IdempotencyKey, &j.ScheduleID,
                &j.CreatedBy, &j.CreatedAt, &j.UpdatedAt, &j.StartedAt, &j.CompletedAt,
                &j.Script, &j.ScriptLanguage, &j.TargetWorkerID)
        if err != nil {
                return nil, err
        }
        return j, nil
}

func (r *JobRepo) List(ctx context.Context, limit, offset int) ([]*Job, error) {
        if limit <= 0 || limit > 500 {
                limit = 100
        }
        if offset < 0 {
                offset = 0
        }
        rows, err := r.db.QueryContext(ctx,
                `SELECT `+jobColumns+` FROM jobs ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanJobs(rows)
}

func (r *JobRepo) Count(ctx context.Context) (int, error) {
        var n int
        err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs`).Scan(&n)
        return n, err
}

func (r *JobRepo) ListByStatus(ctx context.Context, statuses []string) ([]*Job, error) {
        if len(statuses) == 0 {
                return nil, nil
        }
        q := `SELECT ` + jobColumns + ` FROM jobs WHERE status IN (`
        args := make([]any, 0, len(statuses))
        for i, s := range statuses {
                if i > 0 {
                        q += ","
                }
                q += "?"
                args = append(args, s)
        }
        q += `) ORDER BY priority DESC, created_at ASC`
        rows, err := r.db.QueryContext(ctx, q, args...)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanJobs(rows)
}

func (r *JobRepo) ListFiltered(ctx context.Context, statuses []string, limit, offset int) ([]*Job, error) {
        if limit <= 0 || limit > 500 {
                limit = 100
        }
        if offset < 0 {
                offset = 0
        }
        q := `SELECT ` + jobColumns + ` FROM jobs`
        args := make([]any, 0, len(statuses)+2)
        if len(statuses) > 0 {
                q += ` WHERE status IN (`
                for i, s := range statuses {
                        if i > 0 {
                                q += ","
                        }
                        q += "?"
                        args = append(args, s)
                }
                q += `)`
        }
        q += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
        args = append(args, limit, offset)
        rows, err := r.db.QueryContext(ctx, q, args...)
        if err != nil {
                return nil, err
        }
        defer rows.Close()
        return scanJobs(rows)
}

func (r *JobRepo) UpdateStatus(ctx context.Context, id, status string) error {
        _, err := r.db.ExecContext(ctx,
                `UPDATE jobs SET status=?, updated_at=? WHERE id=?`, status, time.Now().UTC(), id)
        return err
}

func (r *JobRepo) MarkStarted(ctx context.Context, id, status string, startedAt time.Time) error {
        _, err := r.db.ExecContext(ctx,
                `UPDATE jobs SET status=?, started_at=?, updated_at=? WHERE id=?`,
                status, startedAt, time.Now().UTC(), id)
        return err
}

func (r *JobRepo) MarkCompleted(ctx context.Context, id, status string, completedAt time.Time) error {
        _, err := r.db.ExecContext(ctx,
                `UPDATE jobs SET status=?, completed_at=?, updated_at=? WHERE id=?`,
                status, completedAt, time.Now().UTC(), id)
        return err
}

func (r *JobRepo) IncrementRetry(ctx context.Context, id string) error {
        _, err := r.db.ExecContext(ctx,
                `UPDATE jobs SET retry_count=retry_count+1, updated_at=? WHERE id=?`,
                time.Now().UTC(), id)
        return err
}

func (r *JobRepo) HasActiveForSchedule(ctx context.Context, scheduleID string) (bool, error) {
        var n int
        err := r.db.QueryRowContext(ctx,
                `SELECT COUNT(*) FROM jobs WHERE schedule_id=? AND status IN ('RUNNING','QUEUED','RETRYING')`,
                scheduleID).Scan(&n)
        if err != nil {
                return false, err
        }
        return n > 0, nil
}

func (r *JobRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time, statuses []string) (int64, error) {
        if len(statuses) == 0 {
                return 0, nil
        }
        q := `DELETE FROM jobs WHERE completed_at < ? AND status IN (`
        args := []any{cutoff}
        for i, s := range statuses {
                if i > 0 {
                        q += ","
                }
                q += "?"
                args = append(args, s)
        }
        q += `)`
        res, err := r.db.ExecContext(ctx, q, args...)
        if err != nil {
                return 0, err
        }
        return res.RowsAffected()
}

func scanJobs(rows *sql.Rows) ([]*Job, error) {
        var out []*Job
        for rows.Next() {
                j := &Job{}
                if err := rows.Scan(
                        &j.ID, &j.Name, &j.Command, &j.Arguments, &j.Environment, &j.Status, &j.Priority,
                        &j.TimeoutSeconds, &j.MaxRetries, &j.RetryCount, &j.IdempotencyKey, &j.ScheduleID,
                        &j.CreatedBy, &j.CreatedAt, &j.UpdatedAt, &j.StartedAt, &j.CompletedAt,
                        &j.Script, &j.ScriptLanguage, &j.TargetWorkerID); err != nil {
                        return nil, err
                }
                out = append(out, j)
        }
        return out, rows.Err()
}
