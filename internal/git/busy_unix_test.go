//go:build !windows

package git

import (
	"os"
	"path/filepath"
	"testing"
)

// holdBusy keeps the folder at path from being deleted until release, as
// Windows keeps one that a process works in: its parent, read-only, lets
// git delete what it holds, not the folder itself, which fails as
// Windows' does, with "Permission denied".
func holdBusy(t *testing.T, path string) (release func()) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root deletes from a read-only folder")
	}
	parent := filepath.Dir(path)
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	release = func() { os.Chmod(parent, 0o755) }
	t.Cleanup(release)
	return release
}
