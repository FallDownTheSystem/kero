package git

import (
	"context"
	"os"
	"path/filepath"
)

// HasSubmodules tells that the worktree at dir lists submodules.
func HasSubmodules(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".gitmodules"))
	return err == nil
}

// UpdateSubmodules checks out the submodules of the worktree at dir, and
// theirs, at the commits it records, as a new worktree has none of them.
func UpdateSubmodules(ctx context.Context, dir string) error {
	_, err := run(ctx, dir, nil, "submodule", "update", "--init", "--recursive")
	return err
}
