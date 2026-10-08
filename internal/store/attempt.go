package store

import (
	"context"
	"database/sql"
	"time"
)

type AttemptRepo struct{ db *sql.DB }

func NewAttemptRepo(db *sql.DB) *AttemptRepo { return &AttemptRepo{db: db} }

func (r *AttemptRepo) Create(ctx context.Context, a *Attempt) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO job_attempts(id,job_id,attempt_number,worker_id,status,lease_expires_at,started_at,completed_at,exit_code,failure_reason)
VALUES(?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.JobID, a.AttemptNumber, a.WorkerID, a.Status, a.LeaseExpiresAt, a.StartedAt, a.CompletedAt, a.ExitCode, a.FailureReason)
	return err
}

func (r *AttemptRepo) Get(ctx context.Context, id string) (*Attempt, error) {
	a := &Attempt{}
	err := r.db.QueryRowContext(ctx, `
SELECT id,job_id,attempt_number,worker_id,status,lease_expires_at,started_at,completed_at,exit_code,failure_reason
FROM job_attempts WHERE id=?`, id).Scan(
		&a.ID, &a.JobID, &a.AttemptNumber, &a.WorkerID, &a.Status, &a.LeaseExpiresAt,
		&a.StartedAt, &a.CompletedAt, &a.ExitCode, &a.FailureReason)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (r *AttemptRepo) ListByJob(ctx context.Context, jobID string) ([]*Attempt, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id,job_id,attempt_number,worker_id,status,lease_expires_at,started_at,completed_at,exit_code,failure_reason
FROM job_attempts WHERE job_id=? ORDER BY attempt_number DESC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Attempt
	for rows.Next() {
		a := &Attempt{}
		if err := rows.Scan(&a.ID, &a.JobID, &a.AttemptNumber, &a.WorkerID, &a.Status, &a.LeaseExpiresAt,
			&a.StartedAt, &a.CompletedAt, &a.ExitCode, &a.FailureReason); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *AttemptRepo) NextAttemptNumber(ctx context.Context, jobID string) (int, error) {
	var n sql.NullInt64
	err := r.db.QueryRowContext(ctx,
		`SELECT MAX(attempt_number) FROM job_attempts WHERE job_id=?`, jobID).Scan(&n)
	if err != nil {
		return 0, err
	}
	return int(n.Int64) + 1, nil
}

func (r *AttemptRepo) RenewLease(ctx context.Context, id string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE job_attempts SET lease_expires_at=? WHERE id=?`, expiresAt, id)
	return err
}

func (r *AttemptRepo) Complete(ctx context.Context, id, status string, exitCode int, reason string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE job_attempts SET status=?, completed_at=?, exit_code=?, failure_reason=? WHERE id=?`,
		status, time.Now().UTC(), exitCode, reason, id)
	return err
}

func (r *AttemptRepo) ListExpired(ctx context.Context, now time.Time) ([]*Attempt, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id,job_id,attempt_number,worker_id,status,lease_expires_at,started_at,completed_at,exit_code,failure_reason
FROM job_attempts WHERE status='RUNNING' AND lease_expires_at < ?`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Attempt
	for rows.Next() {
		a := &Attempt{}
		if err := rows.Scan(&a.ID, &a.JobID, &a.AttemptNumber, &a.WorkerID, &a.Status, &a.LeaseExpiresAt,
			&a.StartedAt, &a.CompletedAt, &a.ExitCode, &a.FailureReason); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
