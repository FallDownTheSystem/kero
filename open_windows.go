package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

// hideConsole starts a program without a console of its own: the app has
// none, and a script's would flash up.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

var allowSetForegroundWindow = windows.NewLazySystemDLL("user32.dll").NewProc("AllowSetForegroundWindow")

// allowForeground lets the app a file opens in come to the front: the
// app is in front as the user asks, and an editor already running, as
// VS Code, which is not let, presses Alt to be, which opens Kero's menus.
func allowForeground() {
	const asfwAny = ^uintptr(0)
	allowSetForegroundWindow.Call(asfwAny)
}

// canOpenWith tells that the system can ask which app opens a file.
const canOpenWith = true

// openWith asks which app opens a file, in the dialog Explorer's Open
// With shows.
func openWith(path string) error {
	rundll := filepath.Join(os.Getenv("SystemRoot"), "System32", "rundll32.exe")
	cmd := exec.Command(rundll)
	// OpenAs_RunDLL takes the rest of the command line as the path, as it
	// is: quoted, as Go would quote one with spaces, it finds no file.
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: syscall.EscapeArg(rundll) + " shell32.dll,OpenAs_RunDLL " + path}
	allowForeground()
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
