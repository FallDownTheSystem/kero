// Package assistant is the folders of the user's assistants, as package
// ama is Ama's: one for each, under assistants beside Kero's
// configuration, which holds its instructions, Kero's, as CLAUDE.md and
// AGENTS.md, which Kero writes anew from the assistant's definition; the
// user's, USER.md, and the assistant's own notes, MEMORY.md, which Kero
// makes once and never touches again. It also tells when a check is due
// (Due, Next), which kerod's schedule asks.
package assistant

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"kero/internal/commitmsg"
	"kero/internal/proto"
	"kero/internal/settings"
)

// Root is the assistants' folder: assistants beside the configuration
// file, as ~/.config/kero/assistants.
func Root() string {
	return filepath.Join(filepath.Dir(settings.ConfigPath()), "assistants")
}

// Dir is the folder of assistant id.
func Dir(id string) string { return filepath.Join(Root(), id) }

// Within reports whether dir is an assistant's folder, or in one, and
// whose: a conversation there is that assistant's.
func Within(dir string) (id string, ok bool) {
	if dir == "" {
		return "", false
	}
	rel, err := filepath.Rel(Root(), filepath.Clean(dir))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	id, _, _ = strings.Cut(filepath.ToSlash(rel), "/")
	return id, id != ""
}

// MaxID is the longest an id is.
const MaxID = 32

var (
	notSlug = regexp.MustCompile(`[^a-z0-9]+`)
	// reserved are the names Windows gives devices, which no folder may
	// take, of any extension.
	reserved = regexp.MustCompile(`^(con|prn|aux|nul|com[0-9]|lpt[0-9])$`)
)

// Slug is the id a name gives: lowercase letters, digits and dashes, at
// most MaxID long, "" for a name that has none or names a device on
// Windows.
func Slug(name string) string {
	s := notSlug.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(s, "-")
	if len(s) > MaxID {
		s = strings.TrimRight(s[:MaxID], "-")
	}
	if reserved.MatchString(s) {
		return ""
	}
	return s
}

// NewID is the id for a new assistant of name, which taken tells is in
// use: its Slug, with -2, -3… after it on a clash, else assistant-<n>.
func NewID(name string, taken func(string) bool) string {
	base := Slug(name)
	if base == "" {
		for n := 1; ; n++ {
			if id := fmt.Sprintf("assistant-%d", n); !taken(id) {
				return id
			}
		}
	}
	if !taken(base) {
		return base
	}
	for n := 2; ; n++ {
		suffix := fmt.Sprintf("-%d", n)
		b := base
		if len(b)+len(suffix) > MaxID {
			b = strings.TrimRight(b[:MaxID-len(suffix)], "-")
		}
		if id := b + suffix; !taken(id) {
			return id
		}
	}
}

// Command is the agent an assistant runs: its own, else the default
// agent's, as words, with its arguments after it.
func Command(a proto.Assistant, s settings.Settings) []string {
	agent := strings.TrimSpace(a.Agent)
	if agent == "" {
		agent = s.DefaultAgent
	}
	w := commitmsg.Words(agent)
	if len(w) == 0 {
		w = []string{settings.DefaultAgentCommand}
	}
	return append(w, commitmsg.Words(a.AgentArgs)...)
}

// UserFile, MemoryFile and the instructions' files of assistant id.
// customColor is an assistant's own color, as Color holds it: #rrggbb,
// lowercase and opaque.
var customColor = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// ColorValid reports whether c is an assistant's color: "" for the one
// its name picks, one of the hosts' eight, or a custom #rrggbb.
func ColorValid(c string) bool {
	return c == "" || slices.Contains(settings.HostColors, c) || customColor.MatchString(c)
}

// IsCustomColor reports whether c is a custom #rrggbb, not a named color.
func IsCustomColor(c string) bool { return customColor.MatchString(c) }

func UserFile(id string) string   { return filepath.Join(Dir(id), "USER.md") }
func MemoryFile(id string) string { return filepath.Join(Dir(id), "MEMORY.md") }

// Prepare makes an assistant's folder, with Kero's instructions as its
// definition gives them, and the user's file and the assistant's memory,
// once. changed tells that the instructions changed, which an assistant
// running is told of.
func Prepare(a proto.Assistant) (changed bool, err error) {
	dir := Dir(a.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	prompt := Prompt(a)
	for name, text := range map[string]string{"CLAUDE.md": claudeFile(prompt), "AGENTS.md": agentsFile(prompt)} {
		c, err := writeIfChanged(filepath.Join(dir, name), text)
		if err != nil {
			return changed, err
		}
		changed = changed || c
	}
	for path, text := range map[string]string{UserFile(a.ID): userTemplate, MemoryFile(a.ID): memoryTemplate} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				return changed, err
			}
		}
	}
	return changed, nil
}

// writeIfChanged writes text to path unless it holds it already, through
// a file beside it renamed over it: on Windows a rename fails while
// another program has the file open, as an agent reading it or a virus
// scan, so it tries a few times before it gives up, and leaves the file
// whole either way. changed tells it wrote.
func writeIfChanged(path, text string) (changed bool, err error) {
	if b, err := os.ReadFile(path); err == nil && bytes.Equal(b, []byte(text)) {
		return false, nil
	}
	return true, WriteFile(path, []byte(text), 0o644)
}

// WriteFile writes data to path through a file beside it, renamed over
// it, retrying the rename a few times (writeIfChanged).
func WriteFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	var err error
	for i := 0; i < 5; i++ {
		if err = os.Rename(tmp, path); err == nil {
			return nil
		}
		time.Sleep(time.Duration(50<<i) * time.Millisecond)
	}
	os.Remove(tmp)
	return err
}

// keroHeader opens the files Kero writes.
const keroHeader = "<!-- Written by Kero, which rewrites this file from the assistant's settings (its role, what it keeps track of and its checks): put your own instructions in USER.md, beside it. MEMORY.md is the assistant's own notes. -->\n\n"

// claudeFile is CLAUDE.md: Claude Code imports USER.md and MEMORY.md
// itself.
func claudeFile(prompt string) string {
	return keroHeader + prompt + "\n@USER.md\n\n@MEMORY.md\n"
}

// agentsFile is AGENTS.md, for Codex and the other agents, which import
// nothing: the instructions tell them to read both.
func agentsFile(prompt string) string { return keroHeader + prompt }

const userTemplate = `# My instructions for this assistant

<!-- Kero made this file once and never changes it. What you write here
goes on top of the assistant's instructions (CLAUDE.md, AGENTS.md), which
Kero rewrites from its settings. For example: which Jira project or mail
folder to look at, what counts as urgent, what never to touch. -->
`

const memoryTemplate = `# Memory

<!-- The assistant's own notes, which it keeps between its checks and its chats. Kero made this file once and never changes it. -->
`

// Prompt is an assistant's role prompt, which CLAUDE.md and AGENTS.md
// hold: Kero's, with its definition filled in.
func Prompt(a proto.Assistant) string {
	var b strings.Builder
	name := strings.TrimSpace(a.Name)
	fmt.Fprintf(&b, "# You are %s\n\n", name)
	fmt.Fprintf(&b, "You are %s, one of the user's assistants in Kero, the terminal app they run coding agents in. Your role: %s\n\n",
		name, strings.TrimSpace(a.Role))
	b.WriteString("Kero wakes you for the checks below while it runs. The user reads your status line and your reports, and seldom your screen. " +
		"Ama, Kero's helper agent, relays your reports when the user asks for news.\n\n")
	b.WriteString("## What you do and keep track of\n\n")
	if t := strings.TrimSpace(a.Tracks); t != "" {
		b.WriteString(t + "\n\n")
	} else {
		b.WriteString("Your role says it all; USER.md may say more.\n\n")
	}
	b.WriteString("## Your checks\n\n")
	if len(a.Checks) == 0 {
		b.WriteString("None: you work when the user writes to you.\n")
	}
	for _, c := range a.Checks {
		fmt.Fprintf(&b, "- %s: %s\n", CheckWhen(c), CheckText(c))
	}
	b.WriteString("\nA check reaches you as \"Kero: message from your schedule\". One that was due while Kero was not running comes once, late, and says when it was due.\n\n")
	b.WriteString("## How you work\n\n" +
		"- Before each check, read USER.md, the user's own instructions, which come before these where they disagree, and MEMORY.md, your notes.\n" +
		"- Use what this machine has for the work: its MCP servers, CLIs and skills. When a check needs one that is missing or not signed in, that is what you wait on.\n" +
		"- This folder is yours and belongs to no project. Do short work here yourself. For work in a repository, or work that runs long, start a child session with kero new, naming its project (-p) or folder (-d). " +
		"Read reference/assistants.md of the kero skill first (`kero skill show assistants`).\n" +
		"- Keep your status line current: kero report working|done|blocked, seven words or fewer.\n" +
		"- End every check with a report: `kero assistants report -` with Result, Not done, Where and Next, as reference/reporting.md describes. " +
		"When nothing changed, say so in one line. When you need the user, finish what you can first, then start the report with \"Waiting on you:\" and the question.\n" +
		"- After each check, write in MEMORY.md what the next check needs: what you checked and when, what you told the user, what is open. " +
		"Date each entry, and keep the file under 200 lines by removing what is settled.\n" +
		"- Ask before what can't be undone: sending mail or messages in the user's name, closing or changing tickets, deleting branches, merging.\n")
	return b.String()
}

// CheckWhen says when a check runs, in English, as "Daily at 09:00".
func CheckWhen(c proto.AssistantCheck) string {
	switch c.Kind {
	case proto.CheckStart:
		return "When Kero starts"
	case proto.CheckDaily:
		return "Daily at " + c.Time
	case proto.CheckWeekly:
		var days []string
		for _, d := range c.Days {
			if d >= 0 && d < 7 {
				days = append(days, time.Weekday(d).String())
			}
		}
		return "Weekly on " + strings.Join(days, ", ") + " at " + c.Time
	}
	return c.Kind
}

// CheckText is what a check asks.
func CheckText(c proto.AssistantCheck) string {
	if t := strings.TrimSpace(c.Text); t != "" {
		return t
	}
	return "Do your usual check."
}

// ParseCheck reads a check as kero assistants takes it: start,
// daily@09:00 or weekly@mon,thu@09:00, with =<what to check> after it.
func ParseCheck(spec string) (proto.AssistantCheck, error) {
	head, text, _ := strings.Cut(spec, "=")
	c := proto.AssistantCheck{Text: strings.TrimSpace(text)}
	parts := strings.Split(strings.TrimSpace(head), "@")
	c.Kind = strings.ToLower(parts[0])
	bad := fmt.Errorf("check %q: write start, daily@09:00 or weekly@mon,thu@09:00, with =<what to check> after it", spec)
	switch {
	case c.Kind == proto.CheckStart && len(parts) == 1:
	case c.Kind == proto.CheckDaily && len(parts) == 2:
		c.Time = parts[1]
	case c.Kind == proto.CheckWeekly && len(parts) == 3:
		for _, d := range strings.Split(parts[1], ",") {
			i := indexDay(d)
			if i < 0 {
				return c, bad
			}
			c.Days = append(c.Days, i)
		}
		c.Time = parts[2]
	default:
		return c, bad
	}
	return c, CheckValid(c)
}

func indexDay(d string) int {
	// "mon", "monday", or at least two letters of one: "tu", "th".
	d = strings.ToLower(strings.TrimSpace(d))
	if len(d) < 2 {
		return -1
	}
	for i := range 7 {
		if strings.HasPrefix(strings.ToLower(time.Weekday(i).String()), d) {
			return i
		}
	}
	return -1
}

// CheckValid tells whether a check is one Kero can run.
func CheckValid(c proto.AssistantCheck) error {
	switch c.Kind {
	case proto.CheckStart:
		return nil
	case proto.CheckDaily, proto.CheckWeekly:
		if _, _, ok := clock(c.Time); !ok {
			return fmt.Errorf("check time %q: write it as 09:00", c.Time)
		}
		if c.Kind == proto.CheckWeekly {
			if len(c.Days) == 0 {
				return fmt.Errorf("a weekly check needs a day")
			}
			for _, d := range c.Days {
				if d < 0 || d > 6 {
					return fmt.Errorf("no weekday %d", d)
				}
			}
		}
		return nil
	}
	return fmt.Errorf("no kind of check %q: start, daily or weekly", c.Kind)
}

// clock reads "09:00" or "9:00".
func clock(s string) (h, m int, ok bool) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}

// StartGap is the least time between two runs of a check of kind start:
// a kerod restarted for an update doesn't run the morning's check again.
const StartGap = 4 * time.Hour

// Due tells whether check c is due at now, in now's location, of a kerod
// started at started. A daily or weekly check is due once its latest time
// at or before now is after both when it last ran and when it was made:
// missed times come to one run, late, and a check added after its time
// today first runs at its next. A start check is due once for each run of
// kerod, at most every StartGap. A Last in the future, as after the clock
// was set back, counts as now.
func Due(c proto.AssistantCheck, started, now time.Time) bool {
	last := time.UnixMilli(c.Last)
	if c.Last > 0 && last.After(now) {
		last = now
	}
	switch c.Kind {
	case proto.CheckStart:
		if c.Created >= started.UnixMilli() || c.Last >= started.UnixMilli() {
			return false
		}
		return c.Last == 0 || now.Sub(last) >= StartGap
	case proto.CheckDaily, proto.CheckWeekly:
		occ, ok := latest(c, now)
		if !ok {
			return false
		}
		return occ.UnixMilli() > c.Created && (c.Last == 0 || occ.After(last))
	}
	return false
}

// DueAt is the time a due daily or weekly check was due at, the latest
// at or before now; zero for a start check.
func DueAt(c proto.AssistantCheck, now time.Time) time.Time {
	t, _ := latest(c, now)
	return t
}

// latest is the latest time of a daily or weekly check at or before now.
func latest(c proto.AssistantCheck, now time.Time) (time.Time, bool) {
	h, m, ok := clock(c.Time)
	if !ok {
		return time.Time{}, false
	}
	for back := 0; back <= 7; back++ {
		day := now.AddDate(0, 0, -back)
		t := time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, now.Location())
		if t.After(now) || !onDay(c, t) {
			continue
		}
		return t, true
	}
	return time.Time{}, false
}

// Next is the next time a daily or weekly check runs after now; zero for
// a start check.
func Next(c proto.AssistantCheck, now time.Time) time.Time {
	h, m, ok := clock(c.Time)
	if !ok || c.Kind == proto.CheckStart {
		return time.Time{}
	}
	for ahead := 0; ahead <= 7; ahead++ {
		day := now.AddDate(0, 0, ahead)
		t := time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, now.Location())
		if t.After(now) && onDay(c, t) {
			return t
		}
	}
	return time.Time{}
}

func onDay(c proto.AssistantCheck, t time.Time) bool {
	if c.Kind != proto.CheckWeekly {
		return true
	}
	for _, d := range c.Days {
		if time.Weekday(d) == t.Weekday() {
			return true
		}
	}
	return false
}
