package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/runbooks/runbooks/internal/config"
	"github.com/runbooks/runbooks/internal/worker"
)

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			printUsage()
			return
		}
	}

	cfg, err := config.LoadWorker()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	a, err := worker.New(cfg)
	if err != nil {
		logger.Error("worker init", "err", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, worker.ShutdownSignals()...)
	go func() {
		<-ch
		logger.Info("signal received, shutting down")
		a.Stop()
	}()

	logger.Info("worker started", "id", a.ID(), "name", a.Name(), "server", cfg.ServerURL)
	if err := a.Run(ctx); err != nil {
		logger.Error("worker run", "err", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`runbooks-worker - distributed job execution agent

Usage:
  runbooks-worker [run]       Run in foreground (default)
  runbooks-worker help        Show this help

Run in foreground with tmux/screen/nohup to keep alive across disconnects:
  nohup ./runbooks-worker > worker.log 2>&1 &    (Linux/macOS)
  Start-Process .\runbooks-worker.exe            (Windows PowerShell)

Environment variables (prefix RUNBOOK_):
  RUNBOOK_SERVER_URL           Control plane URL (default: http://localhost:8080)
  RUNBOOK_WORKER_NAME          Display name (default: hostname)
  RUNBOOK_CONCURRENCY          Concurrent job executions (default: 1)
  RUNBOOK_ALLOWED_EXECUTABLES  Comma-separated binary paths (empty = allow all)
  RUNBOOK_AUTH_TOKEN           Bearer token for API auth`)
}
