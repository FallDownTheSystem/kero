//go:build !windows

package main

import (
	"errors"
	"os/exec"
)

func hideConsole(*exec.Cmd) {}

func allowForeground() {}

func padMenus() {}

// folderEditor is the program to open a folder in when no editor is set:
// none, as the system has no app for text files to ask of here.
func folderEditor() string { return "" }

// canOpenWith tells that the system can ask which app opens a file: macOS
// and Linux have no dialog of it for another app to show.
const canOpenWith = false

func openWith(string) error { return errors.ErrUnsupported }
