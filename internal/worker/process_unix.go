//go:build !windows

package worker

import (
        "os"
        "os/exec"
        "syscall"
)

func applySysProcAttr(cmd *exec.Cmd) {
        cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateProcess(cmd *exec.Cmd) {
        if cmd.Process == nil {
                return
        }
        _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}

func forceKillProcess(cmd *exec.Cmd) {
        if cmd.Process == nil {
                return
        }
        _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

func ShutdownSignals() []os.Signal {
        return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}
