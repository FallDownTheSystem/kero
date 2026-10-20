package client

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"

	"kero/internal/proto"
)

// startDetached starts cmd, returning what runs it, without a console
// and out of whatever job holds this process when it may leave it, so
// that it outlives whoever started it.
func startDetached(cmd *exec.Cmd) (*exec.Cmd, error) {
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

// Interrupt asks the daemon of a pid to save and stop, through its event
// (proto.StopEvent): Windows has no signal for a process without a console.
func Interrupt(p *os.Process) error {
	name, err := windows.UTF16PtrFromString(proto.StopEvent(p.Pid))
	if err != nil {
		return err
	}
	ev, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ev)
	return windows.SetEvent(ev)
}
