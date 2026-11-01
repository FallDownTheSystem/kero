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

// Files returns the files in dir that git tracks, and those it would track
// as they are not ignored, from dir, with slashes; not those deleted since,
// which git still tracks. An error for a dir in no repository.
func Files(ctx context.Context, dir string) ([]string, error) {
	out, err := run(ctx, dir, nil, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	gone, err := run(ctx, dir, nil, "ls-files", "-z", "--deleted")
	if err != nil {
		return nil, err
	}
	skip := map[string]bool{"": true}
	for _, p := range strings.Split(string(gone), "\x00") {
		skip[p] = true
	}
	var files []string
	for _, p := range strings.Split(string(out), "\x00") {
		// A conflict is listed once by each of its stages.
		if !skip[p] {
			skip[p] = true
			files = append(files, p)
		}
	}
	return files, nil
}
