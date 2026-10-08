package worker

import (
        "bytes"
        "context"
        "crypto/rand"
        "encoding/hex"
        "encoding/json"
        "errors"
        "fmt"
        "io"
        "net/http"
        "os"
        "os/exec"
        "os/signal"
        "path/filepath"
        "strings"
        "sync"
        "time"

        "github.com/runbooks/runbooks/internal/config"
        "github.com/runbooks/runbooks/internal/executor"
)

type Agent struct {
        cfg      *config.Worker
        id       string
        name     string
        client   *http.Client
        exec     *executor.ProcessExecutor
        mu       sync.Mutex
        current  *runningJob
        logRoot  string
        shutdown chan struct{}
}

type runningJob struct {
        AttemptID string
        JobID     string
        Cancel    context.CancelFunc
        Cmd       *exec.Cmd
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

func New(cfg *config.Worker) (*Agent, error) {
        if err := cfg.Validate(); err != nil {
                return nil, err
        }
        id, err := loadOrCreateID(cfg.IDFile)
        if err != nil {
                return nil, err
        }
        name := cfg.WorkerName
        if name == "" {
                h, _ := os.Hostname()
                if h == "" {
                        h = "worker"
                }
                name = h
        }
        if err := os.MkdirAll(cfg.LogsDir, 0o755); err != nil {
                return nil, err
        }
        return &Agent{
                cfg:      cfg,
                id:       id,
                name:     name,
                client:   &http.Client{Timeout: 0},
                exec:     executor.NewProcessExecutor(cfg.AllowedExecutables),
                logRoot:  cfg.LogsDir,
                shutdown: make(chan struct{}),
        }, nil
}

func loadOrCreateID(path string) (string, error) {
        b, err := os.ReadFile(path)
        if err == nil {
                s := strings.TrimSpace(string(b))
                if s != "" {
                        return s, nil
                }
        }
        buf := make([]byte, 16)
        if _, err := rand.Read(buf); err != nil {
                return "", err
        }
        id := hex.EncodeToString(buf)
        if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
                return "", err
        }
        return id, nil
}

func (a *Agent) ID() string { return a.id }
func (a *Agent) Name() string { return a.name }

func (a *Agent) Run(ctx context.Context) error {
        if err := a.register(ctx); err != nil {
                return err
        }
        go a.heartbeatLoop(ctx)
        go a.uploadLoop(ctx)

        sem := make(chan struct{}, a.cfg.Concurrency)
        for i := 0; i < a.cfg.Concurrency; i++ {
                sem <- struct{}{}
        }

        tick := time.NewTicker(500 * time.Millisecond)
        defer tick.Stop()

        for {
                select {
                case <-ctx.Done():
                        return a.gracefulShutdown()
                case <-a.shutdown:
                        return a.gracefulShutdown()
                case <-sem:
                        select {
                        case <-ctx.Done():
                                return a.gracefulShutdown()
                        case <-time.After(200 * time.Millisecond):
                                j, err := a.pullJob(ctx)
                                if err != nil {
                                        sem <- struct{}{}
                                        continue
                                }
                                if j == nil {
                                        sem <- struct{}{}
                                        continue
                                }
                                go func() {
                                        a.execute(ctx, j)
                                        sem <- struct{}{}
                                }()
                        }
                case <-tick.C:
                }
        }
}

func (a *Agent) Stop() {
        close(a.shutdown)
}

func (a *Agent) gracefulShutdown() error {
        a.mu.Lock()
        defer a.mu.Unlock()
        if a.current != nil {
                a.current.Cancel()
                if a.current.Cmd != nil && a.current.Cmd.Process != nil {
                        terminateProcess(a.current.Cmd)
                }
                deadline := time.NewTimer(10 * time.Second)
                <-deadline.C
        }
        a.uploadPendingLogs(context.Background())
        a.sendHeartbeat(context.Background(), "OFFLINE")
        return nil
}

func (a *Agent) register(ctx context.Context) error {
        body, _ := json.Marshal(map[string]any{
                "id":              a.id,
                "name":            a.name,
                "max_concurrency": a.cfg.Concurrency,
        })
        req, _ := http.NewRequestWithContext(ctx, "POST", a.cfg.ServerURL+"/api/v1/workers/register", bytes.NewReader(body))
        req.Header.Set("Content-Type", "application/json")
        if a.cfg.AuthToken != "" {
                req.Header.Set("Authorization", "Bearer "+a.cfg.AuthToken)
        }
        resp, err := a.client.Do(req)
        if err != nil {
                return err
        }
        defer resp.Body.Close()
        if resp.StatusCode >= 300 {
                return fmt.Errorf("register failed: %d", resp.StatusCode)
        }
        return nil
}

func (a *Agent) heartbeatLoop(ctx context.Context) {
        t := time.NewTicker(time.Duration(a.cfg.HeartbeatInterval) * time.Second)
        defer t.Stop()
        for {
                select {
                case <-ctx.Done():
                        return
                case <-t.C:
                        a.sendHeartbeat(ctx, "ACTIVE")
                }
        }
}

func (a *Agent) sendHeartbeat(ctx context.Context, status string) {
        body, _ := json.Marshal(map[string]string{"status": status})
        req, _ := http.NewRequestWithContext(ctx, "POST", a.cfg.ServerURL+"/api/v1/workers/"+a.id+"/heartbeat", bytes.NewReader(body))
        req.Header.Set("Content-Type", "application/json")
        if a.cfg.AuthToken != "" {
                req.Header.Set("Authorization", "Bearer "+a.cfg.AuthToken)
        }
        resp, err := a.client.Do(req)
        if err != nil {
                return
        }
        resp.Body.Close()
}

func (a *Agent) pullJob(ctx context.Context) (*pulledJob, error) {
        url := fmt.Sprintf("%s/api/v1/jobs/pull?worker_id=%s", a.cfg.ServerURL, a.id)
        req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
        if a.cfg.AuthToken != "" {
                req.Header.Set("Authorization", "Bearer "+a.cfg.AuthToken)
        }
        resp, err := a.client.Do(req)
        if err != nil {
                return nil, err
        }
        defer resp.Body.Close()
        if resp.StatusCode == http.StatusNoContent {
                return nil, nil
        }
        if resp.StatusCode >= 300 {
                return nil, fmt.Errorf("pull failed: %d", resp.StatusCode)
        }
        var j pulledJob
        if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
                if errors.Is(err, io.EOF) {
                        return nil, nil
                }
                return nil, err
        }
        return &j, nil
}

type rotatedLogger struct {
        jobDir     string
        segmentNum int
        maxBytes   int
        maxLines   int
        cur        *os.File
        curBytes   int
        curLines   int
        path       string
        segments   []string
        mu         sync.Mutex
}

func newRotatedLogger(jobDir string, maxBytes, maxLines int) *rotatedLogger {
        return &rotatedLogger{jobDir: jobDir, maxBytes: maxBytes, maxLines: maxLines}
}

func (rl *rotatedLogger) Write(p []byte) (int, error) {
        rl.mu.Lock()
        defer rl.mu.Unlock()
        if rl.cur == nil {
                if err := rl.rotate(); err != nil {
                        return 0, err
                }
        }
        if rl.curBytes+len(p) >= rl.maxBytes || rl.curLines >= rl.maxLines {
                if err := rl.rotate(); err != nil {
                        return 0, err
                }
        }
        n, err := rl.cur.Write(p)
        rl.curBytes += n
        rl.curLines += bytes.Count(p, []byte("\n"))
        return n, err
}

func (rl *rotatedLogger) rotate() error {
        if rl.cur != nil {
                _ = rl.cur.Close()
                rl.segments = append(rl.segments, rl.path)
                rl.segmentNum++
                rl.curBytes = 0
                rl.curLines = 0
        }
        if err := os.MkdirAll(rl.jobDir, 0o755); err != nil {
                return err
        }
        rl.path = filepath.Join(rl.jobDir, fmt.Sprintf("part-%04d.log", rl.segmentNum))
        f, err := os.OpenFile(rl.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
        if err != nil {
                return err
        }
        rl.cur = f
        return nil
}

func (rl *rotatedLogger) Close() {
        rl.mu.Lock()
        defer rl.mu.Unlock()
        if rl.cur != nil {
                _ = rl.cur.Close()
                rl.segments = append(rl.segments, rl.path)
                rl.cur = nil
        }
}

func (rl *rotatedLogger) Segments() []string {
        rl.mu.Lock()
        defer rl.mu.Unlock()
        out := make([]string, len(rl.segments))
        copy(out, rl.segments)
        return out
}

func (a *Agent) execute(ctx context.Context, j *pulledJob) {
        jobDir := filepath.Join(a.logRoot, j.JobID, j.AttemptID)
        rl := newRotatedLogger(jobDir, a.cfg.SegmentSizeMB*1024*1024, a.cfg.SegmentLines)
        defer rl.Close()

        timeout := time.Duration(j.TimeoutSeconds) * time.Second
        if timeout <= 0 {
                timeout = time.Hour
        }
        execCtx, cancel := context.WithTimeout(ctx, timeout)
        defer cancel()

        cmdCtx, cmdCancel := context.WithCancel(execCtx)
        defer cmdCancel()

        command, args, cleanup, err := a.resolveCommand(j)
        if err != nil {
                _, _ = rl.Write([]byte(fmt.Sprintf("error: %v\n", err)))
                rl.Close()
                a.uploadSegments(ctx, j, rl.Segments())
                a.complete(ctx, j.AttemptID, 127, err)
                return
        }
        if cleanup != nil {
                defer cleanup()
        }

        cmd := exec.CommandContext(cmdCtx, command, args...)
        cmd.Stdout = rl
        cmd.Stderr = rl
        applySysProcAttr(cmd)

        a.mu.Lock()
        a.current = &runningJob{AttemptID: j.AttemptID, JobID: j.JobID, Cancel: cmdCancel, Cmd: cmd}
        a.mu.Unlock()

        defer func() {
                a.mu.Lock()
                a.current = nil
                a.mu.Unlock()
        }()

        go a.renewLeaseLoop(ctx, j.AttemptID, execCtx)

        exitCode := 0
        var runErr error
        if err := cmd.Start(); err != nil {
                exitCode = -1
                runErr = err
        } else {
                err = cmd.Wait()
                if execCtx.Err() == context.DeadlineExceeded {
                        exitCode = 124
                        runErr = errors.New("timeout")
                } else if err != nil {
                        if exitErr, ok := err.(*exec.ExitError); ok {
                                exitCode = exitErr.ExitCode()
                        } else {
                                exitCode = 1
                                runErr = err
                        }
                }
        }
        if cmdCtx.Err() == context.Canceled && execCtx.Err() != context.DeadlineExceeded {
                forceKillProcess(cmd)
        }
        rl.Close()
        a.uploadSegments(ctx, j, rl.Segments())
        a.complete(ctx, j.AttemptID, exitCode, runErr)
}

func (a *Agent) resolveCommand(j *pulledJob) (string, []string, func(), error) {
        if j.Script == "" {
                if j.Command == "" {
                        return "", nil, nil, fmt.Errorf("job has no command or script")
                }
                return j.Command, parseArgs(j.Arguments), nil, nil
        }
        lang := j.ScriptLanguage
        if lang == "" {
                lang = "bash"
        }
        ext := ".sh"
        switch lang {
        case "bash", "sh":
                ext = ".sh"
        case "python", "python3", "py":
                ext = ".py"
        case "powershell", "ps1":
                ext = ".ps1"
        }
        tmpDir := filepath.Join(a.logRoot, j.JobID, j.AttemptID)
        if err := os.MkdirAll(tmpDir, 0o755); err != nil {
                return "", nil, nil, err
        }
        scriptPath := filepath.Join(tmpDir, "script"+ext)
        if err := os.WriteFile(scriptPath, []byte(j.Script), 0o755); err != nil {
                return "", nil, nil, err
        }
        cleanup := func() { _ = os.Remove(scriptPath) }
        args := parseArgs(j.Arguments)
        switch lang {
        case "bash":
                return "bash", append([]string{scriptPath}, args...), cleanup, nil
        case "sh":
                return "sh", append([]string{scriptPath}, args...), cleanup, nil
        case "python", "python3", "py":
                return "python3", append([]string{scriptPath}, args...), cleanup, nil
        case "powershell", "ps1":
                exe, err := lookPowerShell()
                if err != nil {
                        return "", nil, cleanup, err
                }
                return exe, append([]string{"-NoProfile", "-File", scriptPath}, args...), cleanup, nil
        default:
                return "bash", append([]string{scriptPath}, args...), cleanup, nil
        }
}

func parseArgs(s string) []string {
        s = strings.TrimSpace(s)
        if s == "" {
                return nil
        }
        return strings.Fields(s)
}

func lookPowerShell() (string, error) {
        for _, name := range []string{"pwsh", "powershell"} {
                if p, err := exec.LookPath(name); err == nil {
                        return p, nil
                }
        }
        return "", fmt.Errorf("powershell not found")
}

func (a *Agent) renewLeaseLoop(ctx context.Context, attemptID string, execCtx context.Context) {
        t := time.NewTicker(20 * time.Second)
        defer t.Stop()
        for {
                select {
                case <-execCtx.Done():
                        return
                case <-ctx.Done():
                        return
                case <-t.C:
                        body, _ := json.Marshal(map[string]string{"attempt_id": attemptID})
                        req, _ := http.NewRequestWithContext(ctx, "POST", a.cfg.ServerURL+"/api/v1/jobs/renew-lease", bytes.NewReader(body))
                        req.Header.Set("Content-Type", "application/json")
                        if a.cfg.AuthToken != "" {
                                req.Header.Set("Authorization", "Bearer "+a.cfg.AuthToken)
                        }
                        resp, err := a.client.Do(req)
                        if err == nil {
                                resp.Body.Close()
                        }
                }
        }
}

func (a *Agent) complete(ctx context.Context, attemptID string, exitCode int, runErr error) {
        reason := ""
        if runErr != nil {
                reason = runErr.Error()
        }
        body, _ := json.Marshal(map[string]any{
                "attempt_id": attemptID,
                "exit_code":  exitCode,
                "reason":     reason,
        })
        req, _ := http.NewRequestWithContext(ctx, "POST", a.cfg.ServerURL+"/api/v1/jobs/complete", bytes.NewReader(body))
        req.Header.Set("Content-Type", "application/json")
        if a.cfg.AuthToken != "" {
                req.Header.Set("Authorization", "Bearer "+a.cfg.AuthToken)
        }
        resp, err := a.client.Do(req)
        if err == nil {
                resp.Body.Close()
        }
}

func (a *Agent) uploadSegments(ctx context.Context, j *pulledJob, segments []string) {
        for i, p := range segments {
                if err := a.uploadSegment(ctx, j, p, i); err != nil {
                        continue
                }
                _ = os.Remove(p)
        }
}

func (a *Agent) uploadSegment(ctx context.Context, j *pulledJob, path string, segmentNum int) error {
        f, err := os.Open(path)
        if err != nil {
                return err
        }
        defer f.Close()
        fi, _ := f.Stat()
        url := fmt.Sprintf("%s/api/v1/jobs/%s/attempts/%s/logs", a.cfg.ServerURL, j.JobID, j.AttemptID)
        req, _ := http.NewRequestWithContext(ctx, "POST", url, f)
        req.Header.Set("Content-Type", "application/octet-stream")
        req.Header.Set("X-Segment-Number", fmt.Sprintf("%d", segmentNum))
        req.Header.Set("X-Segment-Size", fmt.Sprintf("%d", fi.Size()))
        if a.cfg.AuthToken != "" {
                req.Header.Set("Authorization", "Bearer "+a.cfg.AuthToken)
        }
        resp, err := a.client.Do(req)
        if err != nil {
                return err
        }
        defer resp.Body.Close()
        if resp.StatusCode >= 300 {
                return fmt.Errorf("upload failed: %d", resp.StatusCode)
        }
        return nil
}

func (a *Agent) uploadLoop(ctx context.Context) {
        t := time.NewTicker(30 * time.Second)
        defer t.Stop()
        for {
                select {
                case <-ctx.Done():
                        return
                case <-t.C:
                        a.uploadPendingLogs(ctx)
                }
        }
}

func (a *Agent) uploadPendingLogs(ctx context.Context) {
        _ = filepath.Walk(a.logRoot, func(path string, info os.FileInfo, err error) error {
                if err != nil || info.IsDir() {
                        return nil
                }
                if !strings.HasSuffix(path, ".log") {
                        return nil
                }
                parts := strings.Split(path, string(os.PathSeparator))
                if len(parts) < 3 {
                        return nil
                }
                attemptIdx := len(parts) - 2
                jobIdx := attemptIdx - 1
                jobID := parts[jobIdx]
                attemptID := parts[attemptIdx]
                _ = a.uploadSegment(ctx, &pulledJob{JobID: jobID, AttemptID: attemptID}, path, 0)
                return nil
        })
}

func HandleSignals(ctx context.Context, cancel context.CancelFunc) {
        ch := make(chan os.Signal, 1)
        signal.Notify(ch, ShutdownSignals()...)
        go func() {
                <-ch
                cancel()
        }()
}
