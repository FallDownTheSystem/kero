package daemon

import (
	"os"
	"strconv"
	"strings"
)

func procName(pid int) string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func procArgs(pid int) []string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
}

func procCwd(pid int) string {
	dir, _ := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
	return dir
}
