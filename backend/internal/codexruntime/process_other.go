//go:build !darwin && !linux

package codexruntime

import (
	"os"
	"os/exec"
	"syscall"
)

func configureProcessGroup(_ *exec.Cmd) {}

func signalProcessGroup(pid int, signal syscall.Signal) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if signal == syscall.SIGKILL {
		return process.Kill()
	}
	return process.Signal(os.Interrupt)
}

func processGroupAlive(_ int) bool {
	return false
}
