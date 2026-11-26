//go:build !unix

package devcmd

import (
	"os/exec"
	"syscall"
)

func setProcessGroup(*exec.Cmd) {}

func signalGroup(cmd *exec.Cmd, _ syscall.Signal) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
