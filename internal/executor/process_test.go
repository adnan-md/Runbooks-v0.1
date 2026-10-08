package executor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProcessExecutorBasic(t *testing.T) {
	e := NewProcessExecutor(nil)
	var stdout, stderr bytes.Buffer
	code, err := e.Run(context.Background(), "echo", "hello world", "", &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit=%d", code)
	}
	if stdout.String() != "hello world\n" {
		t.Errorf("stdout=%q", stdout.String())
	}
}

func TestProcessExecutorAllowlist(t *testing.T) {
	e := NewProcessExecutor([]string{"/bin/echo"})
	if !e.IsAllowed("/bin/echo") {
		t.Error("expected /bin/echo to be allowed")
	}
	if e.IsAllowed("/bin/rm") {
		t.Error("expected /bin/rm to be denied")
	}
}

func TestProcessExecutorTimeout(t *testing.T) {
	if _, err := os.Stat("/bin/sleep"); err != nil {
		t.Skip("/bin/sleep missing")
	}
	e := NewProcessExecutor(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	var b bytes.Buffer
	_, err := e.Run(ctx, "/bin/sleep", "10", "", &b, &b)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestRedactingWriter(t *testing.T) {
	var b bytes.Buffer
	w := NewRedactingWriter(&b, []string{"supersecret"})
	_, _ = w.Write([]byte("token=supersecret here\n"))
	if b.String() != "token=[REDACTED] here\n" {
		t.Errorf("got %q", b.String())
	}
}

func TestProcessExecutorFailingExitCode(t *testing.T) {
	e := NewProcessExecutor(nil)
	shPath := filepath.Join(os.TempDir(), "fail.sh")
	_ = os.WriteFile(shPath, []byte("#!/bin/sh\nexit 7\n"), 0o755)
	defer os.Remove(shPath)
	var b bytes.Buffer
	code, err := e.Run(context.Background(), shPath, "", "", &b, &b)
	if err != nil {
		t.Fatal(err)
	}
	if code != 7 {
		t.Errorf("exit=%d", code)
	}
}
