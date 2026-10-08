package executor

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type Result struct {
	ExitCode int
	Output   []byte
	Err      error
}

type ProcessExecutor struct {
	allowed map[string]bool
}

func NewProcessExecutor(allowedBinaries []string) *ProcessExecutor {
	set := make(map[string]bool)
	for _, b := range allowedBinaries {
		set[b] = true
	}
	return &ProcessExecutor{allowed: set}
}

func (e *ProcessExecutor) IsAllowed(cmd string) bool {
	if len(e.allowed) == 0 {
		return true
	}
	return e.allowed[cmd]
}

func (e *ProcessExecutor) Run(ctx context.Context, command, args, env string, stdout, stderr io.Writer) (int, error) {
	if !e.IsAllowed(command) {
		return -1, fmt.Errorf("executable %q not in allowlist", command)
	}
	argSlice := parseArgs(args)
	cmd := exec.CommandContext(ctx, command, argSlice...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	applySysProcAttr(cmd)
	if env != "" {
		cmd.Env = parseEnv(env)
	}
	if err := cmd.Start(); err != nil {
		return -1, err
	}
	err := cmd.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		killProcessGroup(cmd)
		return -1, ctx.Err()
	}
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return -1, err
		}
	}
	return exitCode, nil
}

func parseArgs(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

func parseEnv(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Fields(s)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.Contains(p, "=") {
			out = append(out, p)
		}
	}
	return out
}
