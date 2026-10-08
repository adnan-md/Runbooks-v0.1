package ui

import (
        "context"
        "crypto/rand"
        "database/sql"
        "encoding/hex"
        "encoding/json"
        "fmt"
        "html/template"
        "io"
        "net/http"
        "os"
        "strconv"
        "strings"
        "time"

        "github.com/go-chi/chi/v5"
        "github.com/runbooks/runbooks/internal/store"
)

type Handler struct {
        jobs      *store.JobRepo
        attempts  *store.AttemptRepo
        workers   *store.WorkerRepo
        schedules *store.ScheduleRepo
        segments  *store.LogSegmentRepo
        tmpl      *template.Template
}

func NewHandler(j *store.JobRepo, a *store.AttemptRepo, w *store.WorkerRepo, s *store.ScheduleRepo, seg *store.LogSegmentRepo) *Handler {
        t := template.Must(template.New("").Funcs(template.FuncMap{
                "badge":     badgeClass,
                "formatTime": formatTime,
                "shortID":   shortID,
                "json":      func(v any) string { b, _ := json.Marshal(v); return string(b) },
        }).Parse(tmpl))
        return &Handler{jobs: j, attempts: a, workers: w, schedules: s, segments: seg, tmpl: t}
}

func badgeClass(status string) string {
        return "badge badge-" + strings.ToLower(status)
}

func formatTime(t any) string {
        switch v := t.(type) {
        case time.Time:
                if v.IsZero() {
                        return "-"
                }
                return v.Format("Jan 02 15:04:05")
        case nil:
                return "-"
        default:
                return fmt.Sprintf("%v", v)
        }
}

func shortID(s string) string {
        if len(s) <= 8 {
                return s
        }
        return s[:8]
}

func newID() string {
        b := make([]byte, 16)
        _, _ = rand.Read(b)
        return hex.EncodeToString(b)
}

type baseData struct {
        Title    string
        Active   string
        Content  template.HTML
}

func (h *Handler) render(w http.ResponseWriter, title, active string, content string) {
        base := baseData{Title: title, Active: active, Content: template.HTML(content)}
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        if err := h.tmpl.ExecuteTemplate(w, "base", base); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
        }
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
        jobs, _ := h.jobs.List(r.Context(), 10, 0)
        workers, _ := h.workers.List(r.Context())
        schedules, _ := h.schedules.List(r.Context())
        activeWorkers, _ := h.workers.ListActive(r.Context())
        runningJobs, _ := h.jobs.ListByStatus(r.Context(), []string{"RUNNING"})
        cutoff := time.Now().UTC().Add(-24 * time.Hour)
        failedJobs, _ := h.jobs.ListFiltered(r.Context(), []string{"FAILED", "LOST"}, 100, 0)
        var failed24h int
        for _, j := range failedJobs {
                if j.CompletedAt.Valid && j.CompletedAt.Time.After(cutoff) {
                        failed24h++
                }
        }
        stats := computeStats(jobs, workers, schedules)
        c := dashboardContent(stats, jobs, activeWorkers, runningJobs, failed24h)
        h.render(w, "Dashboard", "dashboard", c)
}

func (h *Handler) JobsList(w http.ResponseWriter, r *http.Request) {
        limit, offset := parseUIPagination(r)
        statuses := parseUIStatusFilter(r)
        var jobs []*store.Job
        if len(statuses) > 0 {
                jobs, _ = h.jobs.ListFiltered(r.Context(), statuses, limit, offset)
        } else {
                jobs, _ = h.jobs.List(r.Context(), limit, offset)
        }
        total, _ := h.jobs.Count(r.Context())
        c := jobsListContent(jobs, statuses, limit, offset, total)
        h.render(w, "Jobs", "jobs", c)
}

func (h *Handler) NewJob(w http.ResponseWriter, r *http.Request) {
        workers, _ := h.workers.List(r.Context())
        c := newJobContent(workers)
        h.render(w, "New Job", "jobs", c)
}

func (h *Handler) CreateJob(w http.ResponseWriter, r *http.Request) {
        if err := r.ParseMultipartForm(10 << 20); err != nil {
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
        }
        now := time.Now().UTC()
        maxRetries := atoiDefault(r.FormValue("max_retries"), 0)
        timeout := atoiDefault(r.FormValue("timeout_seconds"), 3600)
        priority := atoiDefault(r.FormValue("priority"), 0)
        mode := r.FormValue("mode")
        envStr := buildEnvJSON(r.FormValue("environment"))

        script := ""
        scriptLang := ""
        if mode == "inline" {
                script = r.FormValue("script")
                scriptLang = r.FormValue("script_language")
        } else if mode == "upload" {
                scriptLang = r.FormValue("script_language")
                f, _, err := r.FormFile("script_file")
                if err == nil {
                        defer f.Close()
                        b, _ := io.ReadAll(f)
                        script = string(b)
                }
        }

        j := &store.Job{
                ID:             newID(),
                Name:           r.FormValue("name"),
                Command:        r.FormValue("command"),
                Arguments:      r.FormValue("arguments"),
                Environment:    envStr,
                Status:         "QUEUED",
                Priority:       priority,
                TimeoutSeconds: timeout,
                MaxRetries:     maxRetries,
                IdempotencyKey: toNullString(r.FormValue("idempotency_key")),
                CreatedBy:      toNullString(r.FormValue("created_by")),
                CreatedAt:      now,
                UpdatedAt:      now,
        }
        if script != "" {
                j.Script = sql.NullString{String: script, Valid: true}
                if scriptLang == "" {
                        scriptLang = "bash"
                }
                j.ScriptLanguage = sql.NullString{String: scriptLang, Valid: true}
        }
        target := r.FormValue("target_worker_id")
        if target != "" {
                j.TargetWorkerID = sql.NullString{String: target, Valid: true}
        }
        if err := h.jobs.Create(r.Context(), j); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        http.Redirect(w, r, "/jobs/"+j.ID, http.StatusSeeOther)
}

func buildEnvJSON(raw string) string {
        raw = strings.TrimSpace(raw)
        if raw == "" {
                return ""
        }
        envMap := map[string]string{}
        for _, line := range strings.Split(raw, "\n") {
                line = strings.TrimSpace(line)
                if line == "" {
                        continue
                }
                parts := strings.SplitN(line, "=", 2)
                if len(parts) == 2 {
                        envMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
                }
        }
        if len(envMap) == 0 {
                return ""
        }
        b, _ := json.Marshal(envMap)
        return string(b)
}

func (h *Handler) JobDetail(w http.ResponseWriter, r *http.Request) {
        id := chi.URLParam(r, "id")
        j, err := h.jobs.Get(r.Context(), id)
        if err != nil {
                http.NotFound(w, r)
                return
        }
        attempts, _ := h.attempts.ListByJob(r.Context(), id)
        workerNames := map[string]string{}
        for _, a := range attempts {
                if a.WorkerID.Valid {
                        if wr, err := h.workers.Get(r.Context(), a.WorkerID.String); err == nil {
                                workerNames[a.WorkerID.String] = wr.Name
                        }
                }
        }
        segments, _ := h.segments.ListByJob(r.Context(), id)
        c := jobDetailContent(j, attempts, workerNames, segments)
        h.render(w, "Job "+shortID(id), "jobs", c)
}

func (h *Handler) RetryJob(w http.ResponseWriter, r *http.Request) {
        id := chi.URLParam(r, "id")
        j, err := h.jobs.Get(r.Context(), id)
        if err != nil {
                http.NotFound(w, r)
                return
        }
        if j.Status != "FAILED" && j.Status != "LOST" {
                http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
                return
        }
        _ = h.jobs.UpdateStatus(r.Context(), id, "QUEUED")
        http.Redirect(w, r, "/jobs/"+id, http.StatusSeeOther)
}

func (h *Handler) WorkersList(w http.ResponseWriter, r *http.Request) {
        workers, _ := h.workers.List(r.Context())
        c := workersListContent(workers)
        h.render(w, "Workers", "workers", c)
}

func (h *Handler) SchedulesList(w http.ResponseWriter, r *http.Request) {
        schedules, _ := h.schedules.List(r.Context())
        c := schedulesListContent(schedules)
        h.render(w, "Schedules", "schedules", c)
}

func (h *Handler) NewSchedule(w http.ResponseWriter, r *http.Request) {
        c := newScheduleContent()
        h.render(w, "New Schedule", "schedules", c)
}

func (h *Handler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
        if err := r.ParseForm(); err != nil {
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
        }
        def := map[string]any{
                "name":            r.FormValue("name"),
                "command":         r.FormValue("command"),
                "arguments":       r.FormValue("arguments"),
                "timeout_seconds": atoiDefault(r.FormValue("timeout_seconds"), 3600),
                "max_retries":     atoiDefault(r.FormValue("max_retries"), 0),
        }
        b, _ := json.Marshal(def)
        s := &store.Schedule{
                ID:             newID(),
                Name:           r.FormValue("schedule_name"),
                CronExpression: r.FormValue("cron_expression"),
                JobDefinition:  string(b),
                Enabled:        r.FormValue("enabled") == "on",
                PreventOverlap: r.FormValue("prevent_overlap") == "on",
                NextRunAt:      toNullTime(time.Now().UTC().Add(time.Minute)),
        }
        if err := h.schedules.Create(r.Context(), s); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
        }
        http.Redirect(w, r, "/schedules", http.StatusSeeOther)
}

type stats struct {
        JobsTotal    int
        JobsRunning  int
        JobsQueued   int
        JobsFailed   int
        WorkersActive int
        WorkersTotal  int
        Schedules     int
}

func computeStats(jobs []*store.Job, workers []*store.Worker, schedules []*store.Schedule) stats {
        s := stats{JobsTotal: len(jobs), WorkersTotal: len(workers), Schedules: len(schedules)}
        for _, j := range jobs {
                switch j.Status {
                case "RUNNING":
                        s.JobsRunning++
                case "QUEUED":
                        s.JobsQueued++
                case "FAILED":
                        s.JobsFailed++
                }
        }
        for _, w := range workers {
                if w.Status == "ACTIVE" {
                        s.WorkersActive++
                }
        }
        return s
}

func atoiDefault(s string, def int) int {
        if s == "" {
                return def
        }
        var n int
        if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
                return def
        }
        return n
}

func toNullString(s string) sql.NullString {
        if s == "" {
                return sql.NullString{}
        }
        return sql.NullString{String: s, Valid: true}
}

func toNullTime(t time.Time) sql.NullTime {
        return sql.NullTime{Time: t, Valid: true}
}

func parseUIPagination(r *http.Request) (int, int) {
        limit := 50
        offset := 0
        if s := r.URL.Query().Get("limit"); s != "" {
                if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 500 {
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

func parseUIStatusFilter(r *http.Request) []string {
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

var _ = context.Background
var _ = os.Getenv
