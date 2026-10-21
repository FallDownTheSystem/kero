// Package detach starts programs that outlive whoever started them, as
// kerod, which the app, the CLI and a bridge start, and which starts the
// kerod that follows it as it restarts.
package detach

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// Start starts cmd, returning what runs it, without a console and out of
// whatever job holds this process when it may leave it, so that it
// outlives whoever started it.
func Start(cmd *exec.Cmd) (*exec.Cmd, error) {
	const flags = windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags | windows.CREATE_BREAKAWAY_FROM_JOB}
	if err := cmd.Start(); err == nil {
		return cmd, nil
	}
	c := exec.Command(cmd.Path, cmd.Args[1:]...)
	c.Dir, c.Env, c.Stdin, c.Stdout, c.Stderr = cmd.Dir, cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
	return c, c.Start()
}
