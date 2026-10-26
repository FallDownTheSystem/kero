//go:build !windows

package github

import "os/exec"

func hide(*exec.Cmd) {}
