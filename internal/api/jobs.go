package api

import (
        "encoding/json"
        "net/http"
        "strconv"
        "strings"

        "github.com/go-chi/chi/v5"
        "github.com/runbooks/runbooks/internal/jobs"
        "github.com/runbooks/runbooks/internal/store"
)

type JobHandler struct {
        svc *jobs.Service
}

func NewJobHandler(svc *jobs.Service) *JobHandler {
        return &JobHandler{svc: svc}
}

type createJobReq struct {
        Name           string            `json:"name"`
        Command        string            `json:"command"`
        Arguments      string            `json:"arguments"`
        Environment    map[string]string `json:"environment"`
        Priority       int               `json:"priority"`
        TimeoutSeconds int               `json:"timeout_seconds"`
        MaxRetries     int               `json:"max_retries"`
        IdempotencyKey string            `json:"idempotency_key"`
        CreatedBy      string            `json:"created_by"`
        Script         string            `json:"script"`
        ScriptLanguage string            `json:"script_language"`
        TargetWorkerID string            `json:"target_worker_id"`
}

func (h *JobHandler) Create(w http.ResponseWriter, r *http.Request) {
        var req createJobReq
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                writeErr(w, http.StatusBadRequest, "invalid json")
                return
        }
        if req.Name == "" {
                writeErr(w, http.StatusBadRequest, "name required")
                return
        }
        if req.Command == "" && req.Script == "" {
                writeErr(w, http.StatusBadRequest, "either command or script required")
                return
        }
        var envStr string
        if len(req.Environment) > 0 {
                b, _ := json.Marshal(req.Environment)
                envStr = string(b)
        }
        j, err := h.svc.Create(r.Context(), &jobs.CreateJob{
                Name:           req.Name,
                Command:        req.Command,
                Arguments:      req.Arguments,
                Environment:    envStr,
                Priority:       req.Priority,
                TimeoutSeconds: req.TimeoutSeconds,
                MaxRetries:     req.MaxRetries,
                IdempotencyKey: req.IdempotencyKey,
                CreatedBy:      req.CreatedBy,
                Script:         req.Script,
                ScriptLanguage: req.ScriptLanguage,
                TargetWorkerID: req.TargetWorkerID,
        })
        if err != nil {
                writeErr(w, http.StatusInternalServerError, err.Error())
                return
        }
        writeJSON(w, http.StatusCreated, j)
}

func (h *JobHandler) Get(w http.ResponseWriter, r *http.Request) {
        j, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
        if err != nil {
                writeErr(w, http.StatusNotFound, err.Error())
                return
        }
        writeJSON(w, http.StatusOK, j)
}

func (h *JobHandler) List(w http.ResponseWriter, r *http.Request) {
        limit, offset := parsePagination(r)
        statuses := parseStatusFilter(r)
        var jobList []*store.Job
        var err error
        if len(statuses) > 0 {
                jobList, err = h.svc.ListFiltered(r.Context(), statuses, limit, offset)
        } else {
                jobList, err = h.svc.List(r.Context(), limit, offset)
        }
        if err != nil {
                writeErr(w, http.StatusInternalServerError, err.Error())
                return
        }
        writeJSON(w, http.StatusOK, jobList)
}

func parsePagination(r *http.Request) (int, int) {
        limit := 50
        offset := 0
        if s := r.URL.Query().Get("limit"); s != "" {
                if n, err := strconv.Atoi(s); err == nil && n > 0 {
                        limit = n
                }
        }
        if s := r.URL.Query().Get("offset"); s != "" {
                if n, err := strconv.Atoi(s); err == nil && n >= 0 {
                        offset = n
                }
        }
        return limit, offset
}

func parseStatusFilter(r *http.Request) []string {
        s := r.URL.Query().Get("status")
        if s == "" {
                return nil
        }
        var out []string
        for _, p := range strings.Split(s, ",") {
                p = strings.TrimSpace(strings.ToUpper(p))
                if p != "" {
                        out = append(out, p)
                }
        }
        return out
}

func (h *JobHandler) Retry(w http.ResponseWriter, r *http.Request) {
        if err := h.svc.RetryQueued(r.Context(), chi.URLParam(r, "id")); err != nil {
                writeErr(w, http.StatusBadRequest, err.Error())
                return
        }
        writeJSON(w, http.StatusOK, map[string]string{"status": "QUEUED"})
}

func (h *JobHandler) ListAttempts(w http.ResponseWriter, r *http.Request) {
        a, err := h.svc.ListAttempts(r.Context(), chi.URLParam(r, "id"))
        if err != nil {
                writeErr(w, http.StatusInternalServerError, err.Error())
                return
        }
        writeJSON(w, http.StatusOK, a)
}
