package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"kero/internal/ama"
	"kero/internal/proto"
)

// waitFile waits for a pane's program to write a file.
func waitFile(t *testing.T, path string) string {
	t.Helper()
	for range 200 {
		if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
			return strings.TrimSpace(string(b))
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("no %s", path)
	return ""
}

// Ama's session is of no project, in Ama's folder, which it writes; its
// panes alone get the secret that kero do passes on, which a forged one,
// kerod's own environment's, and a closed session's don't pass for. A new
// Ama replaces the one there was, and a restore keeps it Ama's.
func TestAmaSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the panes run sh")
	}
	d := testDaemon(t)
	t.Setenv("KERO_AMA", "forged")
	out := t.TempDir()
	envOf := func(name string) []string {
		return []string{"sh", "-c", `printf '%s' "$KERO_AMA-" > ` + filepath.Join(out, name)}
	}
	s, err := d.create(proto.CreateParams{Ama: true, Name: "Ama", Dir: t.TempDir(), Project: "repo", Command: envOf("ama")})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Ama || s.Project != "" || s.Panes[0].Dir != ama.Dir() {
		t.Fatalf("Ama's session: ama %v, project %q, dir %s", s.Ama, s.Project, s.Panes[0].Dir)
	}
	for _, f := range []string{"CLAUDE.md", "AGENTS.md", "USER.md"} {
		if _, err := os.Stat(filepath.Join(ama.Dir(), f)); err != nil {
			t.Errorf("Ama's folder: %v", err)
		}
	}
	token := strings.TrimSuffix(waitFile(t, filepath.Join(out, "ama")), "-")
	if token == "" || token == "forged" || !d.fromAma(token) {
		t.Fatalf("Ama's pane's secret %q", token)
	}
	if d.fromAma("forged") || d.fromAma("") {
		t.Error("a forged secret passed")
	}

	other, err := d.create(proto.CreateParams{Command: envOf("other")})
	if err != nil {
		t.Fatal(err)
	}
	if got := waitFile(t, filepath.Join(out, "other")); got != "-" {
		t.Errorf("another session's pane has $KERO_AMA %q", got)
	}
	if other.Ama {
		t.Error("another session is Ama's")
	}

	// A split of Ama's session is Ama's too.
	if _, err := d.create(proto.CreateParams{Split: s.ID, Command: envOf("split")}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSuffix(waitFile(t, filepath.Join(out, "split")), "-"); got != token {
		t.Errorf("a split of Ama's has %q", got)
	}

	next, err := d.create(proto.CreateParams{Ama: true, Command: []string{"true"}})
	if err != nil {
		t.Fatal(err)
	}
	if d.findSessionLocked(s.ID) != nil {
		t.Error("the first Ama stayed beside the next")
	}
	if d.fromAma(token) {
		t.Error("a closed Ama's secret passed")
	}
	if d.move(proto.MoveParams{ID: next.ID, Project: "repo"}) == nil {
		t.Error("moved Ama into a project")
	}

	d.saveState()
	r := testDaemon(t)
	r.dir = d.dir
	r.restore()
	var restored *session
	for _, rs := range r.sessions {
		if rs.id == next.ID {
			restored = rs
		}
	}
	if restored == nil || !restored.ama {
		t.Fatal("the restore lost Ama")
	}
	for _, dd := range []*Daemon{r, d} {
		for _, s := range slices.Clone(dd.sessions) {
			dd.removeSession(s.id)
		}
	}
}
