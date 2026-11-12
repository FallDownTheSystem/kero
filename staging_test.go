package main

import (
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

// The lines selected of a diff stage each alone, as git add -p stages
// them: a removed line's removal, an added line's addition, and a line
// edited as an edit with both its lines.
// Both ways a diff compares lines: an editable one without their endings,
// one that is not, as a staged change's, with them.
func TestApplyLines(t *testing.T) {
	for _, editable := range []bool{true, false} {
		t.Run(map[bool]string{true: "editable", false: "read-only"}[editable], func(t *testing.T) { testApplyLines(t, editable) })
	}
}

func testApplyLines(t *testing.T, editable bool) {
	old := "a\nb\nc\nd\n"
	new := "a\nB\nC\nd\ne\n"
	d := &editor.DiffState{Old: editor.NewBuffer(old), New: editor.NewBuffer(new), Editable: editable}
	hunks := d.Hunks()
	if len(hunks) != 2 {
		t.Fatalf("hunks %+v", hunks)
	}
	oldLines := func(a, b int) editor.DiffLineRange { return editor.DiffLineRange{Side: editor.DiffOld, Start: a, End: b} }
	newLines := func(a, b int) editor.DiffLineRange { return editor.DiffLineRange{Side: editor.DiffNew, Start: a, End: b} }
	stage := func(sel ...editor.DiffLineRange) (string, int) {
		chosen, n := chosenLines(hunks, sel)
		return applyLines(d, hunks, withEnding(d.New, chosen), "\n"), n
	}
	unstage := func(sel ...editor.DiffLineRange) string {
		chosen, _ := chosenLines(hunks, sel)
		return applyLines(d, hunks, func(s editor.DiffSide, l int) bool { return !chosen(s, l) }, "\n")
	}
	for _, tc := range []struct {
		name string
		sel  []editor.DiffLineRange
		want string
		n    int
	}{
		{"the first line edited", []editor.DiffLineRange{oldLines(1, 1), newLines(1, 1)}, "a\nB\nc\nd\n", 2},
		{"both lines edited", []editor.DiffLineRange{oldLines(1, 2), newLines(1, 2)}, "a\nB\nC\nd\n", 4},
		{"a line added alone, its old one kept", []editor.DiffLineRange{newLines(1, 1)}, "a\nb\nB\nc\nd\n", 1},
		{"the line added", []editor.DiffLineRange{newLines(4, 4)}, "a\nb\nc\nd\ne\n", 1},
		{"a line removed alone", []editor.DiffLineRange{oldLines(2, 2)}, "a\nb\nd\n", 1},
		{"every line", []editor.DiffLineRange{oldLines(0, 4), newLines(0, 5)}, new, 5},
		{"no changed line", []editor.DiffLineRange{newLines(0, 0)}, old, 0},
	} {
		got, n := stage(tc.sel...)
		if got != tc.want || n != tc.n {
			t.Errorf("%s: staged %q of %d lines, want %q of %d", tc.name, got, n, tc.want, tc.n)
		}
	}
	// Unstaging the line added takes it alone out.
	if got := unstage(newLines(4, 4)); got != "a\nB\nC\nd\n" {
		t.Errorf("unstaged %q", got)
	}

	// A new file's lines, staged one by one.
	d = &editor.DiffState{Old: editor.NewBuffer(""), New: editor.NewBuffer("x\ny\n"), Editable: editable}
	hunks = d.Hunks()
	chosen, n := chosenLines(hunks, []editor.DiffLineRange{newLines(1, 1)})
	if got := applyLines(d, hunks, withEnding(d.New, chosen), "\r\n"); got != "y\r\n" || n != 1 {
		t.Errorf("a new file's second line staged as %q, %d lines; hunks %+v", got, n, hunks)
	}
}

// A text's last line ending is the empty line after it in an editable
// diff: staging, unstaging or discarding it, or a whole change, adds or
// drops that ending alone, and an empty text has no line to keep. A diff
// that is not editable has no such line: each line keeps its ending, and
// a last line without one is a line changed.
func TestApplyLineEndings(t *testing.T) {
	newLine := func(l int) *editor.DiffLineRange { return &editor.DiffLineRange{Side: editor.DiffNew, Start: l, End: l} }
	for _, tc := range []struct {
		name, old, new string
		sel            *editor.DiffLineRange // nil: the whole first change
		unstage        bool
		want           string
		editableOnly   bool // the line after a final ending selected
	}{
		{"a new file staged whole", "", "a\n", nil, false, "a\n", false},
		{"a new file without an ending, whole", "", "a", nil, false, "a", false},
		{"a new file's ending unstaged", "", "one\ntwo\n", newLine(2), true, "one\ntwo", true},
		{"a new file's last line unstaged", "", "one\ntwo\n", newLine(1), true, "one\n", false},
		{"a new file's last line staged", "", "one\ntwo\n", newLine(1), false, "two\n", false},
		{"a new file unstaged whole", "", "one\ntwo\n", nil, true, "", false},
		{"an ending added", "a", "a\n", newLine(1), false, "a\n", true},
		{"an ending added, whole", "a", "a\n", nil, false, "a\n", false},
		{"an ending dropped", "a\n", "a", &editor.DiffLineRange{Side: editor.DiffOld, Start: 1, End: 1}, false, "a", true},
		{"an ending dropped, whole", "a\n", "a", nil, false, "a", false},
		{"a file emptied", "a\n", "", nil, false, "", false},
		{"a line added at the end", "one\ntwo\n", "one\ntwo\nthree\n", nil, false, "one\ntwo\nthree\n", false},
		{"a line added after no ending", "one", "one\ntwo\n", nil, false, "one\ntwo\n", false},
	} {
		for _, editable := range []bool{true, false} {
			if tc.editableOnly && !editable {
				continue
			}
			d := &editor.DiffState{Old: editor.NewBuffer(tc.old), New: editor.NewBuffer(tc.new), Editable: editable}
			hunks := d.Hunks()
			if len(hunks) == 0 {
				t.Fatalf("%s (editable %v): no change", tc.name, editable)
			}
			take := inHunk(hunks[0])
			if tc.sel != nil {
				take, _ = chosenLines(hunks, []editor.DiffLineRange{*tc.sel})
				if !tc.unstage {
					take = withEnding(d.New, take)
				}
			}
			if tc.unstage {
				take = not(take)
			}
			if got := applyLines(d, hunks, take, "\n"); got != tc.want {
				t.Errorf("%s (editable %v): %q, want %q; hunks %+v", tc.name, editable, got, tc.want, hunks)
			}
		}
	}
}
