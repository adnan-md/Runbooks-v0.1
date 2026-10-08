package api

import (
        "database/sql"
        "encoding/json"
        "net/http"
        "time"

        "github.com/go-chi/chi/v5"
        "github.com/runbooks/runbooks/internal/store"
)

type WorkerHandler struct {
        repo *store.WorkerRepo
}

func NewWorkerHandler(r *store.WorkerRepo) *WorkerHandler {
        return &WorkerHandler{repo: r}
}

type registerReq struct {
        ID             string `json:"id"`
        Name           string `json:"name"`
        Capabilities   string `json:"capabilities"`
        MaxConcurrency int    `json:"max_concurrency"`
}

func (h *WorkerHandler) Register(w http.ResponseWriter, r *http.Request) {
        var req registerReq
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                writeErr(w, http.StatusBadRequest, "invalid json")
                return
        }
        if req.ID == "" || req.Name == "" {
                writeErr(w, http.StatusBadRequest, "id and name required")
                return
        }
        if req.MaxConcurrency < 1 {
                req.MaxConcurrency = 1
        }
        w_ := &store.Worker{
                ID:             req.ID,
                Name:           req.Name,
                Status:         "ACTIVE",
                Capabilities:   req.Capabilities,
                MaxConcurrency:  req.MaxConcurrency,
                RegisteredAt:    time.Now().UTC(),
                LastHeartbeatAt: sql.NullTime{Time: time.Now().UTC(), Valid: true},
        }
        if err := h.repo.Upsert(r.Context(), w_); err != nil {
                writeErr(w, http.StatusInternalServerError, err.Error())
                return
        }
        writeJSON(w, http.StatusOK, w_)
}

func (h *WorkerHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
        id := chi.URLParam(r, "id")
        var req struct {
                Status string `json:"status"`
        }
        _ = json.NewDecoder(r.Body).Decode(&req)
        if req.Status == "" {
                req.Status = "ACTIVE"
        }
        if err := h.repo.Heartbeat(r.Context(), id, req.Status); err != nil {
                writeErr(w, http.StatusInternalServerError, err.Error())
                return
        }
        writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *WorkerHandler) List(w http.ResponseWriter, r *http.Request) {
        ws, err := h.repo.List(r.Context())
        if err != nil {
                writeErr(w, http.StatusInternalServerError, err.Error())
                return
        }
        writeJSON(w, http.StatusOK, ws)
}
