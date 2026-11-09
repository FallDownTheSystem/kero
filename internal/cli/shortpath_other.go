//go:build !windows

package cli

func shortPath(path string) string { return path }
