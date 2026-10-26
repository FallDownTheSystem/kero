package main

import (
	"path"
	"slices"
	"strings"

	"kero/internal/proto"
)

// treeEntry is a row of a list of changes shown as a tree: a folder, by
// its path, named as the folders it stands for, which hold nothing else
// but each other, as a/b/c; or a file. depth is how deep in the tree it
// is, from 0.
type treeEntry struct {
	folder bool
	dir    string
	label  string
	count  int // a folder's files, in all of it
	f      proto.GitFile
	depth  int
}

type treeNode struct {
	name, path string
	dirs       map[string]*treeNode
	files      []proto.GitFile
	count      int
}

// changeTree lays files out in their folders, folders before files, each
// by name; open tells whether a folder, by its path, shows what it holds.
func changeTree(files []proto.GitFile, open func(dir string) bool) []treeEntry {
	root := &treeNode{dirs: map[string]*treeNode{}}
	for _, f := range files {
		n := root
		n.count++
		dir := path.Dir(f.Path)
		if dir != "." {
			for i, part := range strings.Split(dir, "/") {
				child := n.dirs[part]
				if child == nil {
					child = &treeNode{name: part, path: strings.Join(strings.Split(dir, "/")[:i+1], "/"), dirs: map[string]*treeNode{}}
					n.dirs[part] = child
				}
				n = child
				n.count++
			}
		}
		n.files = append(n.files, f)
	}
	var out []treeEntry
	var emit func(n *treeNode, depth int)
	emit = func(n *treeNode, depth int) {
		dirs := make([]*treeNode, 0, len(n.dirs))
		for _, d := range n.dirs {
			// A folder of one folder alone is one row with it.
			for len(d.dirs) == 1 && len(d.files) == 0 {
				for _, only := range d.dirs {
					d = &treeNode{name: d.name + "/" + only.name, path: only.path, dirs: only.dirs, files: only.files, count: only.count}
				}
			}
			dirs = append(dirs, d)
		}
		slices.SortFunc(dirs, func(a, b *treeNode) int { return compareNames(a.name, b.name) })
		for _, d := range dirs {
			out = append(out, treeEntry{folder: true, dir: d.path, label: d.name, count: d.count, depth: depth})
			if open(d.path) {
				emit(d, depth+1)
			}
		}
		fs := slices.Clone(n.files)
		slices.SortFunc(fs, func(a, b proto.GitFile) int { return compareNames(path.Base(a.Path), path.Base(b.Path)) })
		for _, f := range fs {
			out = append(out, treeEntry{f: f, depth: depth})
		}
	}
	emit(root, 0)
	return out
}

// compareNames orders names as a file browser does: case aside, then by
// case.
func compareNames(a, b string) int {
	if c := strings.Compare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}
