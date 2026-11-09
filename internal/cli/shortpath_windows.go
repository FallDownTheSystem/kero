package cli

import "golang.org/x/sys/windows"

// shortPath is path's short (8.3) form, with no spaces, which every shell
// takes bare; path itself when the volume keeps no short names.
func shortPath(path string) string {
	long, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return path
	}
	buf := make([]uint16, 1024)
	n, err := windows.GetShortPathName(long, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		return path
	}
	return windows.UTF16ToString(buf[:n])
}
