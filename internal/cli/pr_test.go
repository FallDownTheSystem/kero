package cli

import (
	"testing"

	"kero/internal/proto"
)

// A comment's lines: one, or a range; a path may hold colons, as a
// Windows drive's, and the last one names the lines.
func TestLinesOf(t *testing.T) {
	for _, c := range []struct {
		in          string
		path        string
		start, line int
		bad         bool
	}{
		{in: "a.go:12", path: "a.go", line: 12},
		{in: "src/a.go:10-12", path: "src/a.go", start: 10, line: 12},
		{in: "c:/x/a.go:3", path: "c:/x/a.go", line: 3},
		{in: "a.go", bad: true},
		{in: "a.go:0", bad: true},
		{in: "a.go:5-3", bad: true},
		{in: "a.go:x", bad: true},
	} {
		path, start, line, err := linesOf(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("%q: no error", c.in)
			}
			continue
		}
		if err != nil || path != c.path || start != c.start || line != c.line {
			t.Errorf("%q: %q %d-%d, %v", c.in, path, start, line, err)
		}
	}
	if s := placeOf(proto.PRDraftComment{Path: "a.go", Start: 3, Line: 5, Side: "LEFT"}); s != "a.go:3-5 (base)" {
		t.Errorf("place %q", s)
	}
}
