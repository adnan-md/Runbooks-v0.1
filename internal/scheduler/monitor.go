package scheduler

import (
        "context"
        "database/sql"
        "fmt"
        "log/slog"
        "os"
        "path/filepath"
        "time"

        "github.com/runbooks/runbooks/internal/jobs"
        "github.com/runbooks/runbooks/internal/store"
)

type Monitor struct {
        db            *sql.DB
        jobs          *store.JobRepo
        attempts      *store.AttemptRepo
        workers       *store.WorkerRepo
        schedules     *store.ScheduleRepo
        segments      *store.LogSegmentRepo
        svc           *jobs.Service
        leaseDur      time.Duration
        logsDir       string
        retentionDays int
}

func NewMonitor(
        db *sql.DB,
        j *store.JobRepo,
        a *store.AttemptRepo,
        w *store.WorkerRepo,
        s *store.ScheduleRepo,
        seg *store.LogSegmentRepo,
        svc *jobs.Service,
        leaseDur time.Duration,
        logsDir string,
        retentionDays int,
) *Monitor {
        return &Monitor{
                db: db, jobs: j, attempts: a, workers: w, schedules: s, segments: seg,
                svc: svc, leaseDur: leaseDur, logsDir: logsDir, retentionDays: retentionDays,
        }
}

func (m *Monitor) Run(ctx context.Context) {
        m.recoverySweep(ctx)
        t := time.NewTicker(10 * time.Second)
        defer t.Stop()
        for {
                select {
                case <-ctx.Done():
                        return
                case <-t.C:
                        m.checkStaleWorkers(ctx)
                        m.checkExpiredLeases(ctx)
                        m.retryBackoff(ctx)
                        m.cleanupOldLogs(ctx)
                        m.runSchedules(ctx)
                }
        }
}

func (m *Monitor) recoverySweep(ctx context.Context) {
        now := time.Now().UTC()
        expired, err := m.attempts.ListExpired(ctx, now)
        if err != nil {
                slog.Error("recovery sweep expired", "err", err)
        }
        for _, a := range expired {
                if err := m.svc.MarkLost(ctx, a.JobID); err != nil {
                        slog.Error("mark lost", "err", err)
                }
        }
        queued, err := m.jobs.ListByStatus(ctx, []string{jobs.StateQueued})
        if err != nil {
                slog.Error("recovery queued list", "err", err)
                return
        }
        slog.Info("recovery sweep complete", "expired", len(expired), "queued", len(queued))
}

func (m *Monitor) checkStaleWorkers(ctx context.Context) {
        cutoff := time.Now().UTC().Add(-30 * time.Second)
        if n, err := m.workers.MarkStale(ctx, cutoff); err != nil {
                slog.Error("stale workers", "err", err)
        } else if n > 0 {
                slog.Info("workers marked unhealthy", "count", n)
        }
}

func (m *Monitor) checkExpiredLeases(ctx context.Context) {
        now := time.Now().UTC()
        expired, err := m.attempts.ListExpired(ctx, now)
        if err != nil {
                slog.Error("expired leases", "err", err)
                return
        }
        for _, a := range expired {
                if err := m.svc.MarkLost(ctx, a.JobID); err != nil {
                        slog.Error("mark lost expired", "job", a.JobID, "err", err)
                }
        }
}

func (m *Monitor) retryBackoff(ctx context.Context) {
        retrying, err := m.jobs.ListByStatus(ctx, []string{jobs.StateRetrying})
        if err != nil {
                slog.Error("retrying list", "err", err)
                return
        }
        for _, j := range retrying {
                backoff := time.Duration(j.RetryCount) * 30 * time.Second
                if j.RetryCount > 0 && time.Since(j.UpdatedAt) > backoff {
                        if err := m.svc.Transition(ctx, j.ID, jobs.StateQueued); err != nil {
                                slog.Error("retry transition", "err", err)
                        }
                }
        }
}

func (m *Monitor) cleanupOldLogs(ctx context.Context) {
        cutoff := time.Now().UTC().AddDate(0, 0, -m.retentionDays)
        segRows, err := m.segments.DeleteOlderThan(ctx, cutoff)
        if err != nil {
                slog.Error("log segment cleanup", "err", err)
                return
        }
        if segRows > 0 {
                slog.Info("deleted log segments", "count", segRows)
        }
        m.pruneLogFiles(ctx, cutoff)
        m.cleanupOldJobs(ctx)
}

func (m *Monitor) cleanupOldJobs(ctx context.Context) {
        succeededCutoff := time.Now().UTC().AddDate(0, 0, -7)
        failedCutoff := time.Now().UTC().AddDate(0, 0, -30)
        n1, err := m.jobs.DeleteOlderThan(ctx, succeededCutoff, []string{"SUCCEEDED"})
        if err != nil {
                slog.Error("cleanup succeeded jobs", "err", err)
        }
        n2, err := m.jobs.DeleteOlderThan(ctx, failedCutoff, []string{"FAILED", "LOST"})
        if err != nil {
                slog.Error("cleanup failed jobs", "err", err)
        }
        if n1+n2 > 0 {
                slog.Info("deleted old jobs", "succeeded", n1, "failed", n2)
        }
}

func (m *Monitor) pruneLogFiles(_ context.Context, cutoff time.Time) {
        if m.logsDir == "" {
                return
        }
        _ = filepath.Walk(m.logsDir, func(path string, info os.FileInfo, err error) error {
                if err != nil || info.IsDir() {
                        return nil
                }
                if info.ModTime().Before(cutoff) {
                        _ = os.Remove(path)
                }
                return nil
        })
}

func (m *Monitor) runSchedules(ctx context.Context) {
        now := time.Now().UTC()
        due, err := m.schedules.ListDue(ctx, now)
        if err != nil {
                slog.Error("schedules due", "err", err)
                return
        }
        for _, s := range due {
                if s.PreventOverlap {
                        active, err := m.jobs.HasActiveForSchedule(ctx, s.ID)
                        if err == nil && active {
                                slog.Info("schedule.skipped_overlap", "schedule", s.Name)
                                m.advanceSchedule(ctx, s, now)
                                continue
                        }
                }
                if err := m.spawnJob(ctx, s); err != nil {
                        slog.Error("schedule spawn", "schedule", s.Name, "err", err)
                }
                m.advanceSchedule(ctx, s, now)
        }
}

func (m *Monitor) advanceSchedule(ctx context.Context, s *store.Schedule, lastRun time.Time) {
        next, err := nextRun(s.CronExpression, lastRun)
        if err != nil {
                slog.Error("compute next", "schedule", s.Name, "err", err)
                return
        }
        _ = m.schedules.MarkRun(ctx, s.ID, lastRun, next)
}

func (m *Monitor) spawnJob(ctx context.Context, s *store.Schedule) error {
        def := s.JobDefinition
        if def == "" {
                return fmt.Errorf("empty job definition")
        }
        j, err := jobs.ParseDefinition(def)
        if err != nil {
                return err
        }
        j.ScheduleID = s.ID
        _, err = m.svc.Create(ctx, j)
        return err
}
