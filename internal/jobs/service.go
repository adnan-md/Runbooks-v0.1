package jobs

import (
        "context"
        "crypto/rand"
        "encoding/hex"
        "database/sql"
        "time"

        "github.com/runbooks/runbooks/internal/store"
)

type Service struct {
        jobs     *store.JobRepo
        attempts *store.AttemptRepo
        sm       *StateMachine
}

func NewService(j *store.JobRepo, a *store.AttemptRepo) *Service {
        return &Service{jobs: j, attempts: a, sm: NewStateMachine()}
}

func NewID() string {
        b := make([]byte, 16)
        _, _ = rand.Read(b)
        return hex.EncodeToString(b)
}

type CreateJob struct {
        Name           string
        Command        string
        Arguments      string
        Environment    string
        Priority       int
        TimeoutSeconds int
        MaxRetries     int
        IdempotencyKey string
        ScheduleID     string
        CreatedBy      string
        Script         string
        ScriptLanguage string
        TargetWorkerID string
}

func (s *Service) Create(ctx context.Context, in *CreateJob) (*store.Job, error) {
        now := time.Now().UTC()
        j := &store.Job{
                ID:             NewID(),
                Name:           in.Name,
                Command:        in.Command,
                Arguments:      in.Arguments,
                Environment:    in.Environment,
                Status:         StateQueued,
                Priority:       in.Priority,
                TimeoutSeconds: in.TimeoutSeconds,
                MaxRetries:     in.MaxRetries,
                CreatedAt:      now,
                UpdatedAt:      now,
        }
        if in.IdempotencyKey != "" {
                j.IdempotencyKey = sql.NullString{String: in.IdempotencyKey, Valid: true}
        }
        if in.ScheduleID != "" {
                j.ScheduleID = sql.NullString{String: in.ScheduleID, Valid: true}
        }
        if in.CreatedBy != "" {
                j.CreatedBy = sql.NullString{String: in.CreatedBy, Valid: true}
        }
        if in.Script != "" {
                j.Script = sql.NullString{String: in.Script, Valid: true}
                lang := in.ScriptLanguage
                if lang == "" {
                        lang = "bash"
                }
                j.ScriptLanguage = sql.NullString{String: lang, Valid: true}
        }
        if in.TargetWorkerID != "" {
                j.TargetWorkerID = sql.NullString{String: in.TargetWorkerID, Valid: true}
        }
        if j.TimeoutSeconds <= 0 {
                j.TimeoutSeconds = 3600
        }
        if err := s.jobs.Create(ctx, j); err != nil {
                return nil, err
        }
        return j, nil
}

func (s *Service) Get(ctx context.Context, id string) (*store.Job, error) {
        return s.jobs.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, limit, offset int) ([]*store.Job, error) {
        return s.jobs.List(ctx, limit, offset)
}

func (s *Service) ListFiltered(ctx context.Context, statuses []string, limit, offset int) ([]*store.Job, error) {
        return s.jobs.ListFiltered(ctx, statuses, limit, offset)
}

func (s *Service) Count(ctx context.Context) (int, error) {
        return s.jobs.Count(ctx)
}

func (s *Service) Transition(ctx context.Context, jobID, to string) error {
        j, err := s.jobs.Get(ctx, jobID)
        if err != nil {
                return err
        }
        if err := s.sm.Validate(j.Status, to); err != nil {
                return err
        }
        if to == StateRunning {
                if err := s.jobs.MarkStarted(ctx, jobID, to, time.Now().UTC()); err != nil {
                        return err
                }
                return nil
        }
        if to == StateSucceeded || to == StateFailed {
                if err := s.jobs.MarkCompleted(ctx, jobID, to, time.Now().UTC()); err != nil {
                        return err
                }
                return nil
        }
        return s.jobs.UpdateStatus(ctx, jobID, to)
}

func (s *Service) StartAttempt(ctx context.Context, jobID, workerID string, leaseDuration time.Duration) (*store.Attempt, error) {
        n, err := s.attempts.NextAttemptNumber(ctx, jobID)
        if err != nil {
                return nil, err
        }
        now := time.Now().UTC()
        a := &store.Attempt{
                ID:             NewID(),
                JobID:          jobID,
                AttemptNumber:  n,
                WorkerID:       sql.NullString{String: workerID, Valid: workerID != ""},
                Status:         "RUNNING",
                StartedAt:      now,
                LeaseExpiresAt: sql.NullTime{Time: now.Add(leaseDuration), Valid: true},
        }
        if err := s.attempts.Create(ctx, a); err != nil {
                return nil, err
        }
        if err := s.Transition(ctx, jobID, StateRunning); err != nil {
                return nil, err
        }
        return a, nil
}

func (s *Service) CompleteAttempt(ctx context.Context, attemptID string, exitCode int, reason string) error {
        a, err := s.attempts.Get(ctx, attemptID)
        if err != nil {
                return err
        }
        status := "SUCCEEDED"
        if exitCode != 0 {
                status = "FAILED"
        }
        if err := s.attempts.Complete(ctx, attemptID, status, exitCode, reason); err != nil {
                return err
        }
        j, err := s.jobs.Get(ctx, a.JobID)
        if err != nil {
                return err
        }
        if status == "SUCCEEDED" {
                return s.Transition(ctx, j.ID, StateSucceeded)
        }
        if j.RetryCount < j.MaxRetries {
                if err := s.jobs.IncrementRetry(ctx, j.ID); err != nil {
                        return err
                }
                return s.Transition(ctx, j.ID, StateRetrying)
        }
        return s.Transition(ctx, j.ID, StateFailed)
}

func (s *Service) MarkLost(ctx context.Context, jobID string) error {
        j, err := s.jobs.Get(ctx, jobID)
        if err != nil {
                return err
        }
        if j.Status != StateRunning && j.Status != StateRetrying {
                return nil
        }
        if err := s.sm.Validate(j.Status, StateLost); err != nil {
                return nil
        }
        if j.RetryCount < j.MaxRetries {
                if err := s.jobs.IncrementRetry(ctx, jobID); err != nil {
                        return err
                }
                _ = s.jobs.UpdateStatus(ctx, jobID, StateLost)
                return s.jobs.UpdateStatus(ctx, jobID, StateRetrying)
        }
        return s.jobs.UpdateStatus(ctx, jobID, StateLost)
}

func (s *Service) RetryQueued(ctx context.Context, jobID string) error {
        j, err := s.jobs.Get(ctx, jobID)
        if err != nil {
                return err
        }
        if j.Status != StateFailed && j.Status != StateLost && j.Status != StateRetrying {
                return ErrInvalidTransition
        }
        return s.Transition(ctx, jobID, StateQueued)
}

func (s *Service) RenewLease(ctx context.Context, attemptID string, leaseDuration time.Duration) error {
        return s.attempts.RenewLease(ctx, attemptID, time.Now().UTC().Add(leaseDuration))
}

func (s *Service) ListAttempts(ctx context.Context, jobID string) ([]*store.Attempt, error) {
        return s.attempts.ListByJob(ctx, jobID)
}

func (s *Service) ListByStatus(ctx context.Context, statuses []string) ([]*store.Job, error) {
        return s.jobs.ListByStatus(ctx, statuses)
}
