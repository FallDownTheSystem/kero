//go:build !windows

package commitmsg

import "os/exec"

func hide(*exec.Cmd) {}
