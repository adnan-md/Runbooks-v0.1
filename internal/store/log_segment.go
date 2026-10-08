package store

import (
	"context"
	"database/sql"
	"time"
)

type LogSegmentRepo struct{ db *sql.DB }

func NewLogSegmentRepo(db *sql.DB) *LogSegmentRepo { return &LogSegmentRepo{db: db} }

func (r *LogSegmentRepo) Create(ctx context.Context, s *LogSegment) error {
	_, err := r.db.ExecContext(ctx, `
INSERT INTO log_segments(id,job_id,attempt_id,worker_id,segment_number,storage_path,size_bytes,line_count,created_at)
VALUES(?,?,?,?,?,?,?,?,?)`,
		s.ID, s.JobID, s.AttemptID, s.WorkerID, s.SegmentNumber, s.StoragePath, s.SizeBytes, s.LineCount, s.CreatedAt)
	return err
}

func (r *LogSegmentRepo) ListByJob(ctx context.Context, jobID string) ([]*LogSegment, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id,job_id,attempt_id,worker_id,segment_number,storage_path,size_bytes,line_count,created_at FROM log_segments WHERE job_id=? ORDER BY segment_number ASC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSegments(rows)
}

func (r *LogSegmentRepo) ListByAttempt(ctx context.Context, attemptID string) ([]*LogSegment, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id,job_id,attempt_id,worker_id,segment_number,storage_path,size_bytes,line_count,created_at FROM log_segments WHERE attempt_id=? ORDER BY segment_number ASC`, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSegments(rows)
}

func (r *LogSegmentRepo) ExistsByPath(ctx context.Context, path string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM log_segments WHERE storage_path=?`, path).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *LogSegmentRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM log_segments WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func scanSegments(rows *sql.Rows) ([]*LogSegment, error) {
	var out []*LogSegment
	for rows.Next() {
		s := &LogSegment{}
		if err := rows.Scan(&s.ID, &s.JobID, &s.AttemptID, &s.WorkerID, &s.SegmentNumber, &s.StoragePath, &s.SizeBytes, &s.LineCount, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
