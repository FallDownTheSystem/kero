package convsearch

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kero/internal/proto"
)

func write(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// messages are a Doc's messages, as "u: text" and "a: text".
func messages(d Doc) []string {
	var out []string
	for m := range strings.SplitSeq(d.Text, string(rune(sep))) {
		if m != "" {
			out = append(out, string(m[0])+": "+m[1:])
		}
	}
	return out
}

func TestParseClaude(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "abc-123.jsonl",
		`{"type":"user","entrypoint":"cli","cwd":"/work/kero","timestamp":"2026-10-01T10:00:00Z","message":{"role":"user","content":"Fix the push bug"}}`,
		`{"type":"user","isMeta":true,"timestamp":"2026-10-01T10:00:01Z","message":{"role":"user","content":"Caveat: meta"}}`,
		`{"type":"user","timestamp":"2026-10-01T10:00:02Z","message":{"role":"user","content":"<command-name>/model</command-name>"}}`,
		`{"type":"assistant","timestamp":"2026-10-01T10:00:03Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"I read remote.go."},{"type":"tool_use","name":"Read","input":{"file_path":"remote.go"}}]}}`,
		`{"type":"user","timestamp":"2026-10-01T10:00:04Z","message":{"role":"user","content":[{"type":"tool_result","content":"package git"}]}}`,
		`{"type":"user","timestamp":"2026-10-01T10:05:00Z","message":{"role":"user","content":[{"type":"text","text":"Now test it"}]}}`,
		`{"type":"ai-title","aiTitle":"Push bug"}`,
		`{"type":"custom-title","customTitle":"My push fix"}`,
		`not json`,
	)
	docs, err := parseClaude(p)
	if err != nil || len(docs) != 1 {
		t.Fatalf("parseClaude = %v, %v", docs, err)
	}
	d := docs[0]
	if d.ID != "abc-123" || d.Agent != "claude" || d.Dir != "/work/kero" || d.Title != "My push fix" || d.Turns != 2 {
		t.Errorf("doc = %+v", d)
	}
	want := []string{"u: Fix the push bug", "a: I read remote.go.", "u: Now test it"}
	if got := messages(d); !slices.Equal(got, want) {
		t.Errorf("messages = %q, want %q", got, want)
	}
	if d.Started != time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC).UnixMilli() || d.Updated != time.Date(2026, 10, 1, 10, 5, 0, 0, time.UTC).UnixMilli() {
		t.Errorf("times = %d, %d", d.Started, d.Updated)
	}

	// Run by a program, as claude -p: none.
	p = write(t, dir, "prog.jsonl", `{"type":"user","entrypoint":"sdk-cli","cwd":"/w","message":{"role":"user","content":"Write a commit message"}}`)
	if docs, err := parseClaude(p); err != nil || len(docs) != 0 {
		t.Errorf("a program's = %v, %v", docs, err)
	}
	// Titled by its first message, without a title of Claude's.
	p = write(t, dir, "untitled.jsonl", `{"type":"user","entrypoint":"cli","message":{"role":"user","content":"Line one\nline two"}}`)
	if docs, _ := parseClaude(p); len(docs) != 1 || docs[0].Title != "Line one" {
		t.Errorf("untitled = %+v", docs)
	}
}

func TestParseCodex(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "rollout.jsonl",
		`{"timestamp":"2026-10-01T10:00:00Z","type":"session_meta","payload":{"id":"0199-aaaa","cwd":"C:\\work\\kero","source":"cli"}}`,
		`{"timestamp":"2026-10-01T10:00:00Z","type":"session_meta","payload":{"id":"0198-parent","cwd":"C:\\other","source":"cli"}}`,
		`{"timestamp":"2026-10-01T10:00:01Z","type":"response_item","payload":{"type":"message","role":"developer","content":[{"type":"input_text","text":"rules"}]}}`,
		`{"timestamp":"2026-10-01T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"# AGENTS.md instructions for C:\\work"}]}}`,
		`{"timestamp":"2026-10-01T10:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Search the sidebar"}]}}`,
		`{"timestamp":"2026-10-01T10:00:02Z","type":"event_msg","payload":{"type":"item_completed","item":{"type":"UserMessage","content":[{"type":"text","text":"Search the sidebar"}]}}}`,
		`{"timestamp":"2026-10-01T10:00:03Z","type":"response_item","payload":{"type":"reasoning","summary":[]}}`,
		`{"timestamp":"2026-10-01T10:00:04Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"I add a field."}]}}`,
		`{"timestamp":"2026-10-01T10:09:00Z","type":"event_msg","payload":{"type":"user_message","message":"Ship it"}}`,
	)
	docs, err := parseCodex(p)
	if err != nil || len(docs) != 1 {
		t.Fatalf("parseCodex = %v, %v", docs, err)
	}
	d := docs[0]
	if d.ID != "0199-aaaa" || d.Dir != `C:\work\kero` || d.Turns != 2 || d.Title != "Search the sidebar" {
		t.Errorf("doc = %+v", d)
	}
	want := []string{"u: Search the sidebar", "a: I add a field.", "u: Ship it"}
	if got := messages(d); !slices.Equal(got, want) {
		t.Errorf("messages = %q, want %q", got, want)
	}

	// Without events, what was sent as the user's, but Codex's own.
	p = write(t, dir, "old.jsonl",
		`{"timestamp":"2026-10-01T10:00:00Z","type":"session_meta","payload":{"id":"0197-old","cwd":"/w"}}`,
		`{"timestamp":"2026-10-01T10:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>x</environment_context>"}]}}`,
		`{"timestamp":"2026-10-01T10:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}}`,
	)
	if docs, _ := parseCodex(p); len(docs) != 1 || !slices.Equal(messages(docs[0]), []string{"u: Hello"}) {
		t.Errorf("old = %+v", docs)
	}

	for name, source := range map[string]string{"exec": `"exec"`, "subagent": `{"subagent":{"thread_spawn":{}}}`} {
		p := write(t, dir, name+".jsonl",
			`{"timestamp":"2026-10-01T10:00:00Z","type":"session_meta","payload":{"id":"0196-x","cwd":"/w","source":`+source+`}}`,
			`{"timestamp":"2026-10-01T10:00:02Z","type":"event_msg","payload":{"type":"user_message","message":"Review this"}}`,
		)
		if docs, err := parseCodex(p); err != nil || len(docs) != 0 {
			t.Errorf("%s = %v, %v", name, docs, err)
		}
	}
}

func TestParsePi(t *testing.T) {
	p := write(t, t.TempDir(), "2026_0199.jsonl",
		`{"type":"session","id":"0199-pi","cwd":"/work/pi","timestamp":"2026-10-01T10:00:00Z"}`,
		`{"type":"message","message":{"role":"user","content":"Hi Pi","timestamp":1790848900000}}`,
		`{"type":"message","message":{"role":"assistant","content":[{"type":"thinking","thinking":"x"},{"type":"text","text":"Hello"}],"timestamp":1790848901000}}`,
		`{"type":"message","message":{"role":"toolResult","content":[{"type":"text","text":"output"}]}}`,
		`{"type":"message","message":{"role":"custom","display":false,"content":"hidden"}}`,
		`{"type":"compaction","summary":"Talked about Pi"}`,
		`{"type":"session_info","name":"Pi chat"}`,
	)
	docs, err := parsePi(p)
	if err != nil || len(docs) != 1 {
		t.Fatalf("parsePi = %v, %v", docs, err)
	}
	d := docs[0]
	if d.ID != "0199-pi" || d.Dir != "/work/pi" || d.Title != "Pi chat" || d.Turns != 1 || d.Started != 1790848800000 || d.Updated != 1790848901000 {
		t.Errorf("doc = %+v", d)
	}
	want := []string{"u: Hi Pi", "a: Hello", "a: Talked about Pi"}
	if got := messages(d); !slices.Equal(got, want) {
		t.Errorf("messages = %q, want %q", got, want)
	}
}

// fakeSource lists a file of each of docs, which reads as its docs; err,
// while set, is its listing's.
func fakeSource(t *testing.T, docs map[string][]Doc, err *error) Source {
	t.Helper()
	dir := t.TempDir()
	for name := range docs {
		write(t, dir, name, name)
	}
	return Source{Agent: "claude",
		List: func() ([]string, error) {
			if err != nil && *err != nil {
				return nil, *err
			}
			var out []string
			for name := range docs {
				out = append(out, filepath.Join(dir, name))
			}
			return out, nil
		},
		Parse: func(p string) ([]Doc, error) { return docs[filepath.Base(p)], nil },
	}
}

func mkdoc(agent, id, dir, title string, updated time.Time, msgs ...string) Doc {
	b := &builder{doc: Doc{Agent: agent, ID: id, Dir: dir, Updated: updated.UnixMilli()}}
	for i, m := range msgs {
		b.add(i%2 == 0, m)
	}
	d := b.done(title, "")
	return d[0]
}

func ids(res proto.SearchResult) []string {
	var out []string
	for _, h := range res.Hits {
		out = append(out, h.ID)
	}
	return out
}

func TestSearch(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.Local)
	ix := New(filepath.Join(t.TempDir(), "cache.gob"), []Source{fakeSource(t, map[string][]Doc{
		"a": {mkdoc("claude", "a", "/work/kero", "Sidebar search", now.Add(-time.Hour), "Add a search to the sidebar", "I add a field at its foot.")},
		"b": {mkdoc("codex", "b", "/work/mygo", "Focus fix", now.Add(-40*24*time.Hour), "The dialog's focus is wrong", "The search of the opener is late. The SEARCH again.")},
		"c": {mkdoc("claude", "c", "/work/kero", "Push bug", now.Add(-2*24*time.Hour), "Pushing a branch pushes main", "PushArgs falls back to the upstream.")},
	}, nil)})
	ix.Search(proto.SearchParams{}, now)
	ix.Wait()
	for _, tc := range []struct {
		q    string
		want []string
	}{
		{"search", []string{"a", "b"}},    // the title's first, and the newer
		{"SEARCH sidebar", []string{"a"}}, // every word
		{`"the upstream"`, []string{"c"}},
		{"search -dialog", []string{"a"}},
		{"agent:codex search", []string{"b"}},
		{"agent:!codex search", []string{"a"}},
		{"-agent:cl search", []string{"b"}},
		{"dir:mygo", []string{"b"}},
		{"dir:kero", []string{"a", "c"}}, // by time alone
		{"date:<1d", []string{"a"}},
		{"date:>1w", []string{"b"}},
		{"date:today", []string{"a"}},
		{"kero", []string{"a", "c"}}, // in the folder
		{"nothing-has-this", nil},
	} {
		if got := ids(ix.Search(proto.SearchParams{Query: tc.q}, now)); !slices.Equal(got, tc.want) {
			t.Errorf("%q = %v, want %v", tc.q, got, tc.want)
		}
	}

	res := ix.Search(proto.SearchParams{Query: "search", Preview: true}, now)
	if res.Matched != 2 || res.Indexed != 3 || !slices.Equal(res.Terms, []string{"search"}) {
		t.Errorf("result = %+v", res)
	}
	b := res.Hits[1]
	if len(b.Snippets) != 1 || b.Snippets[0].User {
		t.Fatalf("b's snippets = %+v", b.Snippets)
	}
	sn := b.Snippets[0]
	var marked []string
	for i := 0; i < len(sn.Marks); i += 2 {
		marked = append(marked, sn.Text[sn.Marks[i]:sn.Marks[i+1]])
	}
	if !slices.Equal(marked, []string{"search", "SEARCH"}) {
		t.Errorf("marked %q in %q", marked, sn.Text)
	}
	// A title alone that has it: the first message, unmarked.
	if res := ix.Search(proto.SearchParams{Query: "focus fix"}, now); len(res.Hits) != 1 || res.Hits[0].Snippets[0].Text != "The dialog's focus is wrong" {
		t.Errorf("focus fix = %+v", res.Hits)
	}
	if got := Marks("Sidebar Search", []string{"search"}); !slices.Equal(got, []int{8, 14}) {
		t.Errorf("Marks = %v", got)
	}
}

func TestSnippetAroundTheWord(t *testing.T) {
	long := strings.Repeat("word ", 40) + "needle" + strings.Repeat(" more", 100)
	d := mkdoc("claude", "x", "/w", "t", time.Now(), "line one\nneedle", long)
	sn := snippets(&doc{Doc: &d, title: Fold(d.Title), dir: Fold(d.Dir), text: Fold(d.Text)}, []string{"needle"}, 6)
	if len(sn) != 2 {
		t.Fatalf("snippets = %+v", sn)
	}
	if sn[0].Text != "line one needle" || !slices.Equal(sn[0].Marks, []int{9, 15}) {
		t.Errorf("first = %+v", sn[0])
	}
	s := sn[1]
	if !strings.HasPrefix(s.Text, "…word") || !strings.HasSuffix(s.Text, "…") || s.Text[s.Marks[0]:s.Marks[1]] != "needle" {
		t.Errorf("second = %+v", s)
	}
}

func TestFold(t *testing.T) {
	for in, want := range map[string]string{"Hello": "hello", "ÄÖ äö": "äö äö", "İ": "İ", "plain": "plain"} {
		if got := Fold(in); got != want || len(got) != len(in) {
			t.Errorf("Fold(%q) = %q", in, got)
		}
	}
}

func TestScanKeepsWhatItCannotRead(t *testing.T) {
	now := time.Now()
	var listErr error
	files := map[string][]Doc{
		"one.jsonl": {mkdoc("claude", "one", "/w", "One", now, "first")},
		"two.jsonl": {mkdoc("claude", "two", "/w", "Two", now, "second")},
	}
	src := fakeSource(t, files, &listErr)
	cache := filepath.Join(t.TempDir(), "cache.gob")
	scan := func(ix *Index) proto.SearchResult {
		ix.mu.Lock()
		ix.scanned = time.Time{}
		ix.mu.Unlock()
		ix.Search(proto.SearchParams{}, time.Now())
		ix.Wait()
		return ix.Search(proto.SearchParams{}, time.Now())
	}
	ix := New(cache, []Source{src})
	if res := scan(ix); res.Indexed != 2 {
		t.Fatalf("indexed %d", res.Indexed)
	}
	// A history gone is forgotten.
	delete(files, "two.jsonl")
	if res := scan(ix); res.Indexed != 1 {
		t.Errorf("after a history went: %d", res.Indexed)
	}
	// A source that can't list its histories keeps them.
	listErr = errors.New("away")
	if res := scan(ix); res.Indexed != 1 {
		t.Errorf("while its folder is away: %d", res.Indexed)
	}
	// The next kerod has them from the cache, before any is read.
	parsed := 0
	parse := src.Parse
	src.Parse = func(p string) ([]Doc, error) { parsed++; return parse(p) }
	listErr = nil
	ix = New(cache, []Source{src})
	if res := scan(ix); res.Indexed != 1 || parsed != 0 {
		t.Errorf("from the cache: %d indexed, %d read anew", res.Indexed, parsed)
	}
	if d, ok := ix.Find("claude", "one"); !ok || d.Title != "One" {
		t.Errorf("Find = %+v, %v", d, ok)
	}
}
