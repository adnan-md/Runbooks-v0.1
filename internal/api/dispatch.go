package api

import (
        "context"
        "database/sql"
        "encoding/json"
        "net/http"
        "time"

        "github.com/runbooks/runbooks/internal/jobs"
        "github.com/runbooks/runbooks/internal/store"
)

type DispatchHandler struct {
        jobs     *store.JobRepo
        attempts *store.AttemptRepo
        svc      *jobs.Service
        db       *sql.DB
        leaseDur time.Duration
        pollDur  time.Duration
}

func NewDispatchHandler(db *sql.DB, j *store.JobRepo, a *store.AttemptRepo, svc *jobs.Service, leaseDur, pollDur time.Duration) *DispatchHandler {
        return &DispatchHandler{db: db, jobs: j, attempts: a, svc: svc, leaseDur: leaseDur, pollDur: pollDur}
}

type pulledJob struct {
        JobID          string `json:"job_id"`
        AttemptID      string `json:"attempt_id"`
        Name           string `json:"name"`
        Command        string `json:"command"`
        Arguments      string `json:"arguments"`
        Environment    string `json:"environment"`
        TimeoutSeconds int    `json:"timeout_seconds"`
        AttemptNumber  int    `json:"attempt_number"`
        Script         string `json:"script"`
        ScriptLanguage string `json:"script_language"`
}

func (h *DispatchHandler) Pull(w http.ResponseWriter, r *http.Request) {
        workerID := r.URL.Query().Get("worker_id")
        if workerID == "" {
                writeErr(w, http.StatusBadRequest, "worker_id required")
                return
        }
        deadline := time.Now().Add(h.pollDur)
        for {
                j, err := h.claimNext(r.Context(), workerID)
                if err == nil && j != nil {
                        writeJSON(w, http.StatusOK, j)
                        return
                }
                if time.Now().After(deadline) {
                        writeJSON(w, http.StatusNoContent, nil)
                        return
                }
                select {
                case <-r.Context().Done():
                        return
                case <-time.After(500 * time.Millisecond):
                }
        }
}

func (h *DispatchHandler) claimNext(ctx context.Context, workerID string) (*pulledJob, error) {
        tx, err := h.db.BeginTx(ctx, nil)
        if err != nil {
                return nil, err
        }
        defer tx.Rollback()

        row := tx.QueryRowContext(ctx, `
SELECT id,name,command,arguments,environment,timeout_seconds,priority,script,script_language
FROM jobs WHERE status='QUEUED' AND (target_worker_id IS NULL OR target_worker_id='' OR target_worker_id=?)
ORDER BY priority DESC, created_at ASC LIMIT 1`, workerID)

        var j store.Job
        var script, scriptLang sql.NullString
        err = row.Scan(&j.ID, &j.Name, &j.Command, &j.Arguments, &j.Environment, &j.TimeoutSeconds, &j.Priority, &script, &scriptLang)
        if err == sql.ErrNoRows {
                return nil, nil
        }
        if err != nil {
                return nil, err
        }

        var n int
        if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(attempt_number),0)+1 FROM job_attempts WHERE job_id=?`, j.ID).Scan(&n); err != nil {
                return nil, err
        }
        now := time.Now().UTC()
        attemptID := jobs.NewID()
        lease := now.Add(h.leaseDur)
        _, err = tx.ExecContext(ctx, `
INSERT INTO job_attempts(id,job_id,attempt_number,worker_id,status,lease_expires_at,started_at)
VALUES(?,?,?,?,?,?,?)`,
                attemptID, j.ID, n, workerID, "RUNNING", lease, now)
        if err != nil {
                return nil, err
        }
        _, err = tx.ExecContext(ctx, `UPDATE jobs SET status='RUNNING', started_at=?, updated_at=? WHERE id=?`, now, now, j.ID)
        if err != nil {
                return nil, err
        }
        if err := tx.Commit(); err != nil {
                return nil, err
        }
        pj := &pulledJob{
                JobID:          j.ID,
                AttemptID:      attemptID,
                Name:           j.Name,
                Command:        j.Command,
                Arguments:      j.Arguments,
                Environment:    j.Environment,
                TimeoutSeconds: j.TimeoutSeconds,
                AttemptNumber:  n,
        }
        if script.Valid {
                pj.Script = script.String
        }
        if scriptLang.Valid {
                pj.ScriptLanguage = scriptLang.String
        }
        return pj, nil
}

type completeReq struct {
        AttemptID string `json:"attempt_id"`
        ExitCode  int    `json:"exit_code"`
        Reason    string `json:"reason"`
}

func (h *DispatchHandler) Complete(w http.ResponseWriter, r *http.Request) {
        var req completeReq
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                writeErr(w, http.StatusBadRequest, "invalid json")
                return
        }
        if err := h.svc.CompleteAttempt(r.Context(), req.AttemptID, req.ExitCode, req.Reason); err != nil {
                writeErr(w, http.StatusInternalServerError, err.Error())
                return
        }
        writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type leaseReq struct {
        AttemptID string `json:"attempt_id"`
}

func (h *DispatchHandler) RenewLease(w http.ResponseWriter, r *http.Request) {
        var req leaseReq
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                writeErr(w, http.StatusBadRequest, "invalid json")
                return
        }
        if err := h.svc.RenewLease(r.Context(), req.AttemptID, h.leaseDur); err != nil {
                writeErr(w, http.StatusInternalServerError, err.Error())
                return
        }
        writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
