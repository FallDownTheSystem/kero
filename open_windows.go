package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

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

var assocQueryString = windows.NewLazySystemDLL("shlwapi.dll").NewProc("AssocQueryStringW")

// folderEditor is the program the system opens text files with, to open a
// folder in when no editor is set: the system would open the folder in
// Explorer. Notepad, the system's own, opens no folder, so there's none.
func folderEditor() string {
	const assocstrExecutable = 2
	ext, _ := windows.UTF16PtrFromString(".txt")
	buf := make([]uint16, windows.MAX_PATH)
	n := uint32(len(buf))
	if hr, _, _ := assocQueryString.Call(0, assocstrExecutable, uintptr(unsafe.Pointer(ext)), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); hr != 0 {
		return ""
	}
	exe := windows.UTF16ToString(buf)
	if strings.EqualFold(filepath.Base(exe), "notepad.exe") {
		return ""
	}
	return exe
}

// padMenus widens the context menus' items by a space: MyGo takes the
// width of a check, SM_CXMENUCHECK, off each item it draws, for what
// Windows adds, but Windows adds a fixed 24 pixels; past 150% scale the
// check is wider, and the widest item's last letter is cut off (29 at
// 175%: 4 pixels short). A space is wider than what's missing.
func padMenus() { labelPad = " " }

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
