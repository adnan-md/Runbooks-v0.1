package store

import (
	"context"
	"database/sql"
	"time"
)

type ScheduleRepo struct{ db *sql.DB }

func NewScheduleRepo(db *sql.DB) *ScheduleRepo { return &ScheduleRepo{db: db} }

func (r *ScheduleRepo) Create(ctx context.Context, s *Schedule) error {
	enabled := 0
	if s.Enabled {
		enabled = 1
	}
	overlap := 0
	if s.PreventOverlap {
		overlap = 1
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO schedules(id,name,cron_expression,job_definition,enabled,prevent_overlap,last_run_at,next_run_at)
VALUES(?,?,?,?,?,?,?,?)`,
		s.ID, s.Name, s.CronExpression, s.JobDefinition, enabled, overlap, s.LastRunAt, s.NextRunAt)
	return err
}

func (r *ScheduleRepo) Update(ctx context.Context, s *Schedule) error {
	enabled := 0
	if s.Enabled {
		enabled = 1
	}
	overlap := 0
	if s.PreventOverlap {
		overlap = 1
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE schedules SET name=?,cron_expression=?,job_definition=?,enabled=?,prevent_overlap=?,next_run_at=? WHERE id=?`,
		s.Name, s.CronExpression, s.JobDefinition, enabled, overlap, s.NextRunAt, s.ID)
	return err
}

func (r *ScheduleRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM schedules WHERE id=?`, id)
	return err
}

func (r *ScheduleRepo) Get(ctx context.Context, id string) (*Schedule, error) {
	s := &Schedule{}
	var enabled, overlap int
	err := r.db.QueryRowContext(ctx,
		`SELECT id,name,cron_expression,job_definition,enabled,prevent_overlap,last_run_at,next_run_at FROM schedules WHERE id=?`, id).
		Scan(&s.ID, &s.Name, &s.CronExpression, &s.JobDefinition, &enabled, &overlap, &s.LastRunAt, &s.NextRunAt)
	if err != nil {
		return nil, err
	}
	s.Enabled = enabled == 1
	s.PreventOverlap = overlap == 1
	return s, nil
}

func (r *ScheduleRepo) List(ctx context.Context) ([]*Schedule, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id,name,cron_expression,job_definition,enabled,prevent_overlap,last_run_at,next_run_at FROM schedules ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Schedule
	for rows.Next() {
		s := &Schedule{}
		var enabled, overlap int
		if err := rows.Scan(&s.ID, &s.Name, &s.CronExpression, &s.JobDefinition, &enabled, &overlap, &s.LastRunAt, &s.NextRunAt); err != nil {
			return nil, err
		}
		s.Enabled = enabled == 1
		s.PreventOverlap = overlap == 1
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *ScheduleRepo) ListDue(ctx context.Context, now time.Time) ([]*Schedule, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id,name,cron_expression,job_definition,enabled,prevent_overlap,last_run_at,next_run_at
FROM schedules WHERE enabled=1 AND next_run_at IS NOT NULL AND next_run_at <= ?`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSchedules(rows)
}

func (r *ScheduleRepo) MarkRun(ctx context.Context, id string, lastRun, nextRun time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE schedules SET last_run_at=?, next_run_at=? WHERE id=?`, lastRun, nextRun, id)
	return err
}

func scanSchedules(rows *sql.Rows) ([]*Schedule, error) {
	var out []*Schedule
	for rows.Next() {
		s := &Schedule{}
		var enabled, overlap int
		if err := rows.Scan(&s.ID, &s.Name, &s.CronExpression, &s.JobDefinition, &enabled, &overlap, &s.LastRunAt, &s.NextRunAt); err != nil {
			return nil, err
		}
		s.Enabled = enabled == 1
		s.PreventOverlap = overlap == 1
		out = append(out, s)
	}
	return out, rows.Err()
}
