//go:build windows

package worker

import (
        "os"
        "os/exec"
        "syscall"
)

func applySysProcAttr(cmd *exec.Cmd) {
        cmd.SysProcAttr = &syscall.SysProcAttr{
                CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
        }
}

func terminateProcess(cmd *exec.Cmd) {
        if cmd.Process == nil {
                return
        }
        _ = cmd.Process.Kill()
}

func forceKillProcess(cmd *exec.Cmd) {
        if cmd.Process == nil {
                return
        }
        _ = cmd.Process.Kill()
}

func ShutdownSignals() []os.Signal {
        return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}
