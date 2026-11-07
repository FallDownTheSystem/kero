package daemon

import (
	"bytes"
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

// procParent returns a process's parent, 0 when it is gone: the field
// of /proc/pid/stat after its state, past the name in parentheses.
func procParent(pid int) int {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0
	}
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return 0
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 2 {
		return 0
	}
	ppid, _ := strconv.Atoi(f[1])
	return ppid
}

// sessionProcs returns the processes of the session that sid leads,
// which those a pane's program starts are in, in process groups of their
// own too: those whose /proc/pid/stat has sid as its session, the fourth
// field after the name.
func sessionProcs(sid int) []int {
	entries, _ := os.ReadDir("/proc")
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		i := bytes.LastIndexByte(b, ')')
		if i < 0 {
			continue
		}
		// A zombie, its parent yet to wait for it, holds nothing.
		if f := strings.Fields(string(b[i+1:])); len(f) > 3 && f[0] != "Z" && f[3] == strconv.Itoa(sid) {
			pids = append(pids, pid)
		}
	}
	return pids
}
