//go:build !windows

package client

import (
	"os"
	"os/exec"
	"syscall"
)

// startDetached starts cmd, returning what runs it, in a session of its own, so that it outlives
// whoever started it.
func startDetached(cmd *exec.Cmd) (*exec.Cmd, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd, cmd.Start()
}

// Interrupt asks the daemon of a pid to save and stop.
func Interrupt(p *os.Process) error { return p.Signal(os.Interrupt) }
