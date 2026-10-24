//go:build !windows

package main

import (
	"errors"
	"os/exec"
)

func hideConsole(*exec.Cmd) {}

func allowForeground() {}

// canOpenWith tells that the system can ask which app opens a file: macOS
// and Linux have no dialog of it for another app to show.
const canOpenWith = false

func openWith(string) error { return errors.ErrUnsupported }
