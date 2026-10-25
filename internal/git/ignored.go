package git

import (
	"context"
	"strings"
)

// IgnoredDirs returns the folders in dir that git ignores, from dir, with
// slashes: only those ignored as a whole, not the folders in them.
func IgnoredDirs(ctx context.Context, dir string) ([]string, error) {
	out, err := run(ctx, dir, nil, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, p := range strings.Split(string(out), "\x00") {
		if d, ok := strings.CutSuffix(p, "/"); ok {
			dirs = append(dirs, d)
		}
	}
	return dirs, nil
}
