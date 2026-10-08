package config

import (
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

type Server struct {
	HTTPAddr        string `envconfig:"HTTP_ADDR" default:":8080"`
	DBDriver        string `envconfig:"DB_DRIVER" default:"sqlite"`
	DBDSN           string `envconfig:"DB_DSN" default:"runbook.db"`
	LogsDir         string `envconfig:"LOGS_DIR" default:"./data/logs"`
	LogRetentionDays int   `envconfig:"LOG_RETENTION_DAYS" default:"30"`
	AuthTokens      string `envconfig:"AUTH_TOKENS" default:""`
	LongPollTimeout int    `envconfig:"LONG_POLL_TIMEOUT" default:"30"`
	LeaseTimeout    int    `envconfig:"LEASE_TIMEOUT" default:"60"`
}

type Worker struct {
	ServerURL         string   `envconfig:"SERVER_URL" default:"http://localhost:8080"`
	WorkerName        string   `envconfig:"WORKER_NAME" default:""`
	IDFile            string   `envconfig:"ID_FILE" default:".worker-id"`
	LogsDir           string   `envconfig:"LOGS_DIR" default:"./logs"`
	MaxLocalLogsMB    int      `envconfig:"MAX_LOCAL_LOGS_MB" default:"500"`
	SegmentSizeMB     int      `envconfig:"SEGMENT_SIZE_MB" default:"50"`
	SegmentLines      int      `envconfig:"SEGMENT_LINES" default:"50000"`
	HeartbeatInterval int      `envconfig:"HEARTBEAT_INTERVAL" default:"10"`
	Concurrency       int      `envconfig:"CONCURRENCY" default:"1"`
	AllowedExecutables []string `envconfig:"ALLOWED_EXECUTABLES" default:""`
	AuthToken         string   `envconfig:"AUTH_TOKEN" default:""`
}

func LoadServer() (*Server, error) {
	var c Server
	if err := envconfig.Process("RUNBOOK", &c); err != nil {
		return nil, err
	}
	if c.LogRetentionDays < 1 {
		c.LogRetentionDays = 30
	}
	return &c, nil
}

func LoadWorker() (*Worker, error) {
	var c Worker
	if err := envconfig.Process("RUNBOOK", &c); err != nil {
		return nil, err
	}
	if c.Concurrency < 1 {
		c.Concurrency = 1
	}
	return &c, nil
}

func (w *Worker) Validate() error {
	if w.ServerURL == "" {
		return fmt.Errorf("server url required")
	}
	return nil
}
