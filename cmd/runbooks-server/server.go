package main

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/runbooks/runbooks/internal/api"
	"github.com/runbooks/runbooks/internal/config"
	"github.com/runbooks/runbooks/internal/jobs"
	"github.com/runbooks/runbooks/internal/scheduler"
	"github.com/runbooks/runbooks/internal/store"
	"github.com/runbooks/runbooks/internal/ui"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			printUsage()
			return
		}
	}

	cfg, err := config.LoadServer()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	db, err := store.Open(cfg.DBDriver, cfg.DBDSN)
	if err != nil {
		logger.Error("db open", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := store.Migrate(db, cfg.DBDriver); err != nil {
		logger.Error("migrate", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobRepo := store.NewJobRepo(db)
	attemptRepo := store.NewAttemptRepo(db)
	workerRepo := store.NewWorkerRepo(db)
	scheduleRepo := store.NewScheduleRepo(db)
	segRepo := store.NewLogSegmentRepo(db)

	svc := jobs.NewService(jobRepo, attemptRepo)

	if err := seedOnboarding(ctx, scheduleRepo); err != nil {
		logger.Error("onboarding seed", "err", err)
	}

	leaseDur := time.Duration(cfg.LeaseTimeout) * time.Second
	pollDur := time.Duration(cfg.LongPollTimeout) * time.Second

	dispatch := api.NewDispatchHandler(db, jobRepo, attemptRepo, svc, leaseDur, pollDur)
	jobH := api.NewJobHandler(svc)
	workerH := api.NewWorkerHandler(workerRepo)
	logH := api.NewLogHandler(segRepo, jobRepo, attemptRepo, workerRepo, cfg.LogsDir)
	scheduleH := api.NewScheduleHandler(scheduleRepo)

	var tokens []string
	if cfg.AuthTokens != "" {
		for _, t := range strings.Split(cfg.AuthTokens, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tokens = append(tokens, t)
			}
		}
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(120 * time.Second))

	auth := api.TokenAuth(tokens)
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(auth)
			r.Post("/workers/register", workerH.Register)
			r.Post("/workers/{id}/heartbeat", workerH.Heartbeat)
			r.Get("/workers", workerH.List)
			r.Post("/jobs", jobH.Create)
			r.Get("/jobs", jobH.List)
			r.Get("/jobs/{id}", jobH.Get)
			r.Post("/jobs/{id}/retry", jobH.Retry)
			r.Get("/jobs/{id}/attempts", jobH.ListAttempts)
			r.Get("/jobs/{id}/logs", logH.List)
			r.Get("/jobs/{id}/logs/content", logH.GetContent)
			r.Get("/jobs/{id}/logs/sse", logH.SSE)
			r.Post("/jobs/{id}/attempts/{attempt_id}/logs", logH.Upload)
			r.Get("/jobs/pull", dispatch.Pull)
			r.Post("/jobs/complete", dispatch.Complete)
			r.Post("/jobs/renew-lease", dispatch.RenewLease)
			r.Post("/schedules", scheduleH.Create)
			r.Get("/schedules", scheduleH.List)
			r.Get("/schedules/{id}", scheduleH.Get)
			r.Delete("/schedules/{id}", scheduleH.Delete)
		})
	})

	uiH := ui.NewHandler(jobRepo, attemptRepo, workerRepo, scheduleRepo, segRepo)
	r.Get("/", uiH.Dashboard)
	r.Get("/jobs", uiH.JobsList)
	r.Get("/jobs/{id}", uiH.JobDetail)
	r.Post("/jobs/{id}/retry", uiH.RetryJob)
	r.Get("/workers", uiH.WorkersList)
	r.Get("/schedules", uiH.SchedulesList)
	r.Post("/schedules", uiH.CreateSchedule)
	r.Get("/schedules/new", uiH.NewSchedule)
	r.Get("/jobs/new", uiH.NewJob)
	r.Post("/jobs", uiH.CreateJob)
	staticFS, err := fs.Sub(ui.Static, "static")
	if err != nil {
		logger.Error("static sub", "err", err)
		os.Exit(1)
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	if err := os.MkdirAll(cfg.LogsDir, 0o755); err != nil {
		logger.Error("logs dir", "err", err)
		os.Exit(1)
	}

	monitor := scheduler.NewMonitor(db, jobRepo, attemptRepo, workerRepo, scheduleRepo, segRepo, svc, leaseDur, cfg.LogsDir, cfg.LogRetentionDays)
	go monitor.Run(context.Background())

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: r}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down")
		ctx2, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = srv.Shutdown(ctx2)
	}()

	logger.Info("listening", "addr", cfg.HTTPAddr, "db", cfg.DBDriver)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server", "err", err)
		os.Exit(1)
	}
	_ = ctx
}

func seedOnboarding(ctx context.Context, sr *store.ScheduleRepo) error {
	existing, err := sr.List(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	def := `{"name":"runbooks-hello-world","command":"echo","arguments":"hello from runbooks","timeout_seconds":10}`
	s := &store.Schedule{
		ID:             jobs.NewID(),
		Name:           "runbooks-hello-world",
		CronExpression: "0 * * * *",
		JobDefinition:  def,
		Enabled:        false,
		PreventOverlap: true,
		NextRunAt:      sql.NullTime{Time: time.Now().UTC().Add(time.Hour), Valid: true},
	}
	return sr.Create(ctx, s)
}

func printUsage() {
	fmt.Println(`runbooks-server - distributed job execution control plane

Usage:
  runbooks-server [run]       Run in foreground (default)
  runbooks-server help        Show this help

Run in foreground with tmux/screen/nohup to keep alive across disconnects:
  nohup ./runbooks-server > server.log 2>&1 &    (Linux/macOS)
  Start-Process .\runbooks-server.exe            (Windows PowerShell)

Environment variables (prefix RUNBOOK_):
  RUNBOOK_HTTP_ADDR            Listen address (default: :8080)
  RUNBOOK_DB_DRIVER            Database driver: sqlite|postgres (default: sqlite)
  RUNBOOK_DB_DSN               Database DSN (default: runbook.db)
  RUNBOOK_LOGS_DIR             Server-side log storage (default: ./data/logs)
  RUNBOOK_LOG_RETENTION_DAYS  Log retention days (default: 30)
  RUNBOOK_AUTH_TOKENS          Comma-separated bearer tokens (default: empty = no auth)`)
}
