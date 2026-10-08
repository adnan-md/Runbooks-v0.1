package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/runbooks/runbooks/internal/store"
)

type LogHandler struct {
	segments *store.LogSegmentRepo
	jobs     *store.JobRepo
	attempts *store.AttemptRepo
	workers  *store.WorkerRepo
	logsDir  string
}

func NewLogHandler(s *store.LogSegmentRepo, j *store.JobRepo, a *store.AttemptRepo, w *store.WorkerRepo, logsDir string) *LogHandler {
	return &LogHandler{segments: s, jobs: j, attempts: a, workers: w, logsDir: logsDir}
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (h *LogHandler) Upload(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	attemptID := chi.URLParam(r, "attempt_id")
	segStr := r.Header.Get("X-Segment-Number")
	segNum, _ := strconv.Atoi(segStr)
	if segNum < 0 {
		segNum = 0
	}

	workerName := "unknown"
	workerID := r.Header.Get("X-Worker-Id")
	if workerID != "" {
		if wr, err := h.workers.Get(r.Context(), workerID); err == nil {
			workerName = wr.Name
		}
	} else {
		if a, err := h.attempts.Get(r.Context(), attemptID); err == nil && a.WorkerID.Valid {
			if wr, err := h.workers.Get(r.Context(), a.WorkerID.String); err == nil {
				workerName = wr.Name
			}
			workerID = a.WorkerID.String
		}
	}

	attemptDir := filepath.Join(h.logsDir, jobID, fmt.Sprintf("%s-%s", attemptID, workerName))
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	segPath := filepath.Join(attemptDir, fmt.Sprintf("part-%04d.log", segNum))

	if exists, _ := h.segments.ExistsByPath(r.Context(), segPath); exists {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"duplicate"}`))
		return
	}

	f, err := os.Create(segPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer f.Close()
	header := fmt.Sprintf("[System] Executed on worker: %s (id: %s)\n", workerName, workerID)
	_, _ = f.WriteString(header)
	n, err := io.Copy(f, r.Body)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	lines := countLines(segPath)

	seg := &store.LogSegment{
		ID:            newID(),
		JobID:         jobID,
		AttemptID:     attemptID,
		WorkerID:      sql.NullString{String: workerID, Valid: workerID != ""},
		SegmentNumber: segNum,
		StoragePath:   segPath,
		SizeBytes:     int(n) + len(header),
		LineCount:     lines,
		CreatedAt:     time.Now().UTC(),
	}
	if err := h.segments.Create(r.Context(), seg); err != nil {
		slog.Error("seg create", "err", err)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func countLines(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}

func (h *LogHandler) List(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	segs, err := h.segments.ListByJob(r.Context(), jobID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, segs)
}

func (h *LogHandler) Stream(w http.ResponseWriter, r *http.Request) {
}

func (h *LogHandler) SSE(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "no flusher")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	sent := map[string]bool{}
	segs, err := h.segments.ListByJob(r.Context(), jobID)
	if err != nil {
		return
	}
	for _, s := range segs {
		sendSegment(w, flusher, s)
		sent[s.ID] = true
	}
	flusher.Flush()
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			segs, err := h.segments.ListByJob(ctx, jobID)
			if err != nil {
				continue
			}
			newCount := 0
			for _, s := range segs {
				if !sent[s.ID] {
					sendSegment(w, flusher, s)
					sent[s.ID] = true
					newCount++
				}
			}
			if newCount > 0 {
				flusher.Flush()
			}
		}
	}
}

func sendSegment(w http.ResponseWriter, _ http.Flusher, s *store.LogSegment) {
	b, err := os.ReadFile(s.StoragePath)
	if err != nil {
		return
	}
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n\n"))
}

func (h *LogHandler) GetContent(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	segs, err := h.segments.ListByJob(r.Context(), jobID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	for _, s := range segs {
		b, err := os.ReadFile(s.StoragePath)
		if err != nil {
			continue
		}
		_, _ = w.Write(b)
	}
}

type ScheduleHandler struct {
	repo *store.ScheduleRepo
}

func NewScheduleHandler(r *store.ScheduleRepo) *ScheduleHandler {
	return &ScheduleHandler{repo: r}
}

type scheduleReq struct {
	Name           string `json:"name"`
	CronExpression string `json:"cron_expression"`
	JobDefinition  string `json:"job_definition"`
	Enabled        bool   `json:"enabled"`
	PreventOverlap bool   `json:"prevent_overlap"`
}

func (h *ScheduleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req scheduleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Name == "" || req.CronExpression == "" || req.JobDefinition == "" {
		writeErr(w, http.StatusBadRequest, "name, cron_expression, job_definition required")
		return
	}
	s := &store.Schedule{
		ID:             newID(),
		Name:           req.Name,
		CronExpression: req.CronExpression,
		JobDefinition:  req.JobDefinition,
		Enabled:        req.Enabled,
		PreventOverlap: req.PreventOverlap,
		NextRunAt:      sql.NullTime{Time: time.Now().UTC().Add(time.Minute), Valid: true},
	}
	if err := h.repo.Create(r.Context(), s); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, s)
}

func (h *ScheduleHandler) List(w http.ResponseWriter, r *http.Request) {
	ss, err := h.repo.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ss)
}

func (h *ScheduleHandler) Get(w http.ResponseWriter, r *http.Request) {
	s, err := h.repo.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *ScheduleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
