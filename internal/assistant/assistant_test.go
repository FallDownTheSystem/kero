package assistant

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"kero/internal/proto"
)

// A check's due time collapses missed runs into one, waits for its next
// time when added after today's, takes a Last in the future for now, and
// a start check runs once a kerod run, at most every four hours.
func TestDue(t *testing.T) {
	loc := time.FixedZone("EET", 2*3600)
	at := func(day, h, m int) time.Time { return time.Date(2026, 11, day, h, m, 0, 0, loc) } // Nov 23 2026 is a Monday
	ms := func(t time.Time) int64 { return t.UnixMilli() }
	daily := proto.AssistantCheck{Kind: proto.CheckDaily, Time: "09:00", Created: ms(at(1, 8, 0))}
	weekly := proto.AssistantCheck{Kind: proto.CheckWeekly, Time: "09:00", Days: []int{1, 4}, Created: ms(at(1, 8, 0))}
	started := at(23, 8, 0)
	with := func(c proto.AssistantCheck, f func(*proto.AssistantCheck)) proto.AssistantCheck { f(&c); return c }
	for _, tc := range []struct {
		name string
		c    proto.AssistantCheck
		now  time.Time
		want bool
	}{
		{"daily before its time, never ran", daily, at(23, 8, 59), true}, // yesterday's 09:00 is after Created
		{"daily ran yesterday, before today's time", with(daily, func(c *proto.AssistantCheck) { c.Last = ms(at(22, 9, 1)) }), at(23, 8, 59), false},
		{"daily ran yesterday, at today's time", with(daily, func(c *proto.AssistantCheck) { c.Last = ms(at(22, 9, 1)) }), at(23, 9, 0), true},
		{"daily missed three days, ran today already", with(daily, func(c *proto.AssistantCheck) { c.Last = ms(at(23, 12, 0)) }), at(23, 18, 0), false},
		{"daily added after its time today", with(daily, func(c *proto.AssistantCheck) { c.Created = ms(at(23, 14, 0)) }), at(23, 15, 0), false},
		{"daily added after its time, next day", with(daily, func(c *proto.AssistantCheck) { c.Created = ms(at(23, 14, 0)) }), at(24, 9, 0), true},
		{"daily whose Last is in the future", with(daily, func(c *proto.AssistantCheck) { c.Last = ms(at(30, 9, 0)) }), at(24, 10, 0), false},
		{"weekly on Thursday, asked Wednesday after Monday's run", with(weekly, func(c *proto.AssistantCheck) { c.Last = ms(at(23, 9, 5)) }), at(25, 12, 0), false},
		{"weekly on Thursday, at Thursday 09:00", with(weekly, func(c *proto.AssistantCheck) { c.Last = ms(at(23, 9, 5)) }), at(26, 9, 0), true},
		{"weekly missed a week, Sunday", with(weekly, func(c *proto.AssistantCheck) { c.Last = ms(at(12, 9, 0)) }), at(22, 12, 0), true},
		{"start, new kerod run", proto.AssistantCheck{Kind: proto.CheckStart, Created: ms(at(1, 0, 0)), Last: ms(at(22, 9, 0))}, at(23, 8, 1), true},
		{"start, ran this kerod run", proto.AssistantCheck{Kind: proto.CheckStart, Created: ms(at(1, 0, 0)), Last: ms(at(23, 8, 1))}, at(23, 13, 0), false},
		{"start, kerod restarted within four hours", proto.AssistantCheck{Kind: proto.CheckStart, Created: ms(at(1, 0, 0)), Last: ms(at(23, 5, 0))}, at(23, 8, 1), false},
		{"start, added during this run", proto.AssistantCheck{Kind: proto.CheckStart, Created: ms(at(23, 8, 30))}, at(23, 9, 0), false},
		{"start, never ran", proto.AssistantCheck{Kind: proto.CheckStart, Created: ms(at(1, 0, 0))}, at(23, 8, 1), true},
	} {
		if got := Due(tc.c, started, tc.now); got != tc.want {
			t.Errorf("%s: Due = %v, want %v", tc.name, got, tc.want)
		}
	}
	if got, want := Next(weekly, at(23, 9, 0)), at(26, 9, 0); !got.Equal(want) {
		t.Errorf("Next of the weekly check after Monday 09:00 = %v, want Thursday 09:00", got)
	}
	if got, want := DueAt(weekly, at(25, 12, 0)), at(23, 9, 0); !got.Equal(want) {
		t.Errorf("DueAt on Wednesday = %v, want Monday 09:00", got)
	}
}

// Across daylight saving's spring gap, a check at a time the clock skips
// runs once that day, and not again an hour later.
func TestDueAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Helsinki")
	if err != nil {
		t.Skip("no time zone data")
	}
	c := proto.AssistantCheck{Kind: proto.CheckDaily, Time: "03:30", Created: time.Date(2026, 3, 1, 0, 0, 0, 0, loc).UnixMilli()}
	c.Last = time.Date(2026, 3, 28, 3, 31, 0, 0, loc).UnixMilli()
	// 2026-03-29 03:00 jumps to 04:00 in Helsinki.
	if !Due(c, time.Time{}, time.Date(2026, 3, 29, 4, 31, 0, 0, loc)) {
		t.Fatal("the skipped 03:30 never came due")
	}
	c.Last = time.Date(2026, 3, 29, 4, 31, 0, 0, loc).UnixMilli()
	if Due(c, time.Time{}, time.Date(2026, 3, 29, 6, 0, 0, 0, loc)) {
		t.Fatal("the skipped 03:30 came due twice")
	}
}

func TestSlugAndNewID(t *testing.T) {
	for name, want := range map[string]string{
		"Jira Buddy":   "jira-buddy",
		"  Mail. ":     "mail",
		"CON":          "",
		"日本":           "",
		"Über-Helper!": "ber-helper",
	} {
		if got := Slug(name); got != want {
			t.Errorf("Slug(%q) = %q, want %q", name, got, want)
		}
	}
	taken := map[string]bool{"mail": true, "mail-2": true, "assistant-1": true}
	if got := NewID("Mail", func(id string) bool { return taken[id] }); got != "mail-3" {
		t.Errorf("NewID(Mail) = %q, want mail-3", got)
	}
	if got := NewID("nul", func(id string) bool { return taken[id] }); got != "assistant-2" {
		t.Errorf("NewID(nul) = %q, want assistant-2", got)
	}
	long := strings.Repeat("a", 40)
	if got := NewID(long, func(id string) bool { return id == long[:MaxID] }); len(got) > MaxID || got != long[:MaxID-2]+"-2" {
		t.Errorf("NewID(40 a's) = %q", got)
	}
}

func TestParseCheck(t *testing.T) {
	c, err := ParseCheck("weekly@mon,th@09:30=List branches merged a fortnight ago")
	if err != nil || c.Kind != proto.CheckWeekly || c.Time != "09:30" || len(c.Days) != 2 || c.Days[0] != 1 || c.Days[1] != 4 || c.Text != "List branches merged a fortnight ago" {
		t.Errorf("weekly: %+v, %v", c, err)
	}
	for _, bad := range []string{"daily", "daily@25:00", "weekly@@09:00", "weekly@x@09:00", "hourly@1", "start@09:00"} {
		if _, err := ParseCheck(bad); err == nil {
			t.Errorf("ParseCheck(%q) took it", bad)
		}
	}
}

func TestPrepare(t *testing.T) {
	t.Setenv("KERO_CONFIG", t.TempDir()+"/config")
	a := proto.Assistant{ID: "jira", Name: "Jira", Role: "Keeps my tickets moving",
		Checks: []proto.AssistantCheck{{Kind: proto.CheckDaily, Time: "09:00", Text: "What moved?"}}}
	if changed, err := Prepare(a); err != nil || !changed {
		t.Fatalf("Prepare: %v, %v", changed, err)
	}
	if err := writeOwn(MemoryFile("jira"), "mine"); err != nil {
		t.Fatal(err)
	}
	if changed, err := Prepare(a); err != nil || changed {
		t.Fatalf("Prepare again changed %v, %v", changed, err)
	}
	a.Role = "Watches my sprint"
	if changed, _ := Prepare(a); !changed {
		t.Fatal("a new role changed nothing")
	}
	claude := read(t, Dir("jira")+"/CLAUDE.md")
	for _, want := range []string{"Watches my sprint", "- Daily at 09:00: What moved?", "@USER.md", "@MEMORY.md", "kero assistants report"} {
		if !strings.Contains(claude, want) {
			t.Errorf("CLAUDE.md has no %q", want)
		}
	}
	if read(t, MemoryFile("jira")) != "mine" {
		t.Error("Prepare rewrote the assistant's memory")
	}
	if id, ok := Within(Dir("jira") + "/sub"); !ok || id != "jira" {
		t.Errorf("Within = %q, %v", id, ok)
	}
}

func writeOwn(path, text string) error { return os.WriteFile(path, []byte(text), 0o644) }

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestValidID(t *testing.T) {
	for id, want := range map[string]bool{"jira": true, "mail-2": true, "assistant-1": true,
		"": false, "..": false, "a/b": false, `a\b`: false, "Jira": false, "con": false, "-a": false,
		"abcdefghijklmnopqrstuvwxyzabcdefg": false} {
		if got := ValidID(id); got != want {
			t.Errorf("ValidID(%q) = %v", id, got)
		}
	}
}

// Writers of one file at once each finish, the file whole.
func TestWriteFileTogether(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CLAUDE.md")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- WriteFile(path, []byte(strings.Repeat(fmt.Sprint(i), 1000)), 0o644)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if b, _ := os.ReadFile(path); len(b) != 1000 {
		t.Errorf("%d bytes", len(b))
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp")); len(left) > 0 {
		t.Errorf("left %v", left)
	}
}
