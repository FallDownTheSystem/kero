package convsearch

import (
	"math"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"kero/internal/proto"
)

// query is what a search asks for: words, or quoted phrases, every one
// of which a conversation has in its title, folder or messages, none of
// not, and as filters say, as fast-resume's queries do:
//
//	agent:claude,codex  -agent:pi  agent:!pi
//	dir:kero  dir:!test
//	date:today  date:yesterday  date:week  date:month  date:<2d  date:>1w
type query struct {
	terms, not        []string
	agents, notAgents []string
	dirs, notDirs     []string
	after, before     int64 // unix ms; 0 for none
}

func parseQuery(s string, now time.Time) query {
	var q query
	for _, tok := range tokens(s) {
		neg := false
		if len(tok) > 1 && tok[0] == '-' {
			neg, tok = true, tok[1:]
		}
		key, val, ok := strings.Cut(tok, ":")
		key = strings.ToLower(key)
		if ok && val != "" && (key == "agent" || key == "dir" || key == "date") {
			for v := range strings.SplitSeq(val, ",") {
				n := neg
				if strings.HasPrefix(v, "!") {
					n, v = !n, v[1:]
				}
				v = Fold(strings.Trim(v, `"`))
				if v == "" {
					continue
				}
				switch key {
				case "agent":
					if n {
						q.notAgents = append(q.notAgents, v)
					} else {
						q.agents = append(q.agents, v)
					}
				case "dir":
					if n {
						q.notDirs = append(q.notDirs, v)
					} else {
						q.dirs = append(q.dirs, v)
					}
				case "date":
					q.date(v, n, now)
				}
			}
			continue
		}
		tok = Fold(strings.Trim(tok, `"`))
		if tok == "" {
			continue
		}
		if neg {
			q.not = append(q.not, tok)
		} else {
			q.terms = append(q.terms, tok)
		}
	}
	return q
}

// tokens splits a query at its spaces, but in quotes.
func tokens(s string) []string {
	var out []string
	var b strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			b.WriteRune(r)
		case unicode.IsSpace(r) && !quoted:
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// date narrows the query to a time: since then, or, with older, before.
func (q *query) date(v string, older bool, now time.Time) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var from, to time.Time
	switch v {
	case "today":
		from = day
	case "yesterday":
		from, to = day.AddDate(0, 0, -1), day
	case "week":
		from = now.AddDate(0, 0, -7)
	case "month":
		from = now.AddDate(0, 0, -30)
	default:
		if strings.HasPrefix(v, ">") {
			older = !older
		}
		d, ok := ago(strings.TrimLeft(v, "<>"))
		if !ok {
			return
		}
		from = now.Add(-d)
	}
	if older {
		// Before the time, rather than since it.
		q.before = from.UnixMilli()
		return
	}
	q.after = from.UnixMilli()
	if !to.IsZero() {
		q.before = to.UnixMilli()
	}
}

// ago reads "30m", "2h", "3d", "1w", "2mo" or "1y".
func ago(s string) (time.Duration, bool) {
	units := []struct {
		name string
		d    time.Duration
	}{{"mo", 30 * 24 * time.Hour}, {"m", time.Minute}, {"h", time.Hour}, {"d", 24 * time.Hour}, {"w", 7 * 24 * time.Hour}, {"y", 365 * 24 * time.Hour}}
	for _, u := range units {
		if n, ok := strings.CutSuffix(s, u.name); ok {
			if v, err := strconv.Atoi(n); err == nil && v >= 0 {
				return time.Duration(v) * u.d, true
			}
			return 0, false
		}
	}
	return 0, false
}

func (q *query) wants(d *doc) bool {
	if d.Updated < q.after || (q.before != 0 && d.Updated >= q.before) {
		return false
	}
	if len(q.agents) > 0 && !slices.ContainsFunc(q.agents, func(a string) bool { return agentIs(d.Agent, a) }) {
		return false
	}
	if slices.ContainsFunc(q.notAgents, func(a string) bool { return agentIs(d.Agent, a) }) {
		return false
	}
	if len(q.dirs) > 0 && !slices.ContainsFunc(q.dirs, func(s string) bool { return strings.Contains(d.dir, s) }) {
		return false
	}
	if slices.ContainsFunc(q.notDirs, func(s string) bool { return strings.Contains(d.dir, s) }) {
		return false
	}
	for _, t := range q.not {
		if strings.Contains(d.title, t) || strings.Contains(d.text, t) {
			return false
		}
	}
	return true
}

// agentIs tells an agent by its id or name, or the start of either: cl,
// claude, Claude Code.
func agentIs(agent, v string) bool {
	return strings.HasPrefix(agent, v) || strings.HasPrefix(Fold(proto.AgentNames[agent]), v)
}

// score is how well a conversation answers the query's words, 0 for
// not at all: a word in its title counts most, then each time its
// messages have it, fewer the more, and its folder; the whole query as
// written counts again. Newer conversations count more, as fast-resume's
// do: twice and a half today, falling by half each fortnight to once.
func (q *query) score(d *doc, now time.Time) float64 {
	s := 0.0
	for _, t := range q.terms {
		inTitle := strings.Contains(d.title, t)
		n := count(d.text, t, 20)
		inDir := strings.Contains(d.dir, t)
		if !inTitle && n == 0 && !inDir {
			return 0
		}
		if inTitle {
			s += 4
		}
		if n > 0 {
			s += 1 + math.Log1p(float64(n))
		}
		if inDir {
			s++
		}
	}
	if len(q.terms) > 1 {
		if all := strings.Join(q.terms, " "); strings.Contains(d.title, all) || strings.Contains(d.text, all) {
			s += 3
		}
	}
	if len(q.terms) == 0 {
		s = 1
	}
	age := now.Sub(time.UnixMilli(d.Updated)).Hours() / 24
	return s * (1 + 1.5*math.Exp2(-max(age, 0)/14))
}

// count counts s in text, up to most times.
func count(text, s string, most int) int {
	n := 0
	for n < most {
		i := strings.Index(text, s)
		if i < 0 {
			break
		}
		n++
		text = text[i+len(s):]
	}
	return n
}

// Search finds the conversations a query asks for, the limit best,
// starting a scan of the histories when the last is old: while it runs,
// it finds among those read so far.
func (ix *Index) Search(p proto.SearchParams, now time.Time) proto.SearchResult {
	ix.kick(now)
	q := parseQuery(p.Query, now)
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	ix.mu.Lock()
	docs := ix.snapshotLocked()
	res := proto.SearchResult{Indexed: len(docs), Indexing: ix.scanning, Done: ix.done, Todo: ix.todo, Terms: q.terms}
	ix.mu.Unlock()

	type scored struct {
		d *doc
		s float64
	}
	// The docs in shares, a share for each processor, each its own matches.
	shares := max(runtime.GOMAXPROCS(0), 1)
	found := make([][]scored, shares)
	var wg sync.WaitGroup
	for k := range shares {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := k; i < len(docs); i += shares {
				d := docs[i]
				if !q.wants(d) {
					continue
				}
				if s := q.score(d, now); s > 0 {
					found[k] = append(found[k], scored{d, s})
				}
			}
		}()
	}
	wg.Wait()
	all := slices.Concat(found...)
	slices.SortFunc(all, func(x, y scored) int {
		switch {
		case x.s > y.s:
			return -1
		case x.s < y.s:
			return 1
		case x.d.Updated > y.d.Updated:
			return -1
		case x.d.Updated < y.d.Updated:
			return 1
		}
		return strings.Compare(x.d.ID, y.d.ID)
	})
	res.Matched = len(all)
	if len(all) > limit {
		all = all[:limit]
	}
	most := 1
	if p.Preview {
		most = 6
	}
	res.Hits = make([]proto.SearchHit, 0, len(all))
	for _, f := range all {
		d := f.d
		res.Hits = append(res.Hits, proto.SearchHit{
			Conversation: proto.Conversation{Agent: d.Agent, ID: d.ID, Dir: d.Dir, Title: d.Title, Started: d.Started, Updated: d.Updated},
			Turns:        d.Turns,
			Score:        f.s,
			Snippets:     snippets(d, q.terms, most),
		})
	}
	return res
}

// snippets are the parts of up to most of a conversation's messages that
// have the query's words, in the order said; or, when only its title or
// folder has them, its first message, the user's.
func snippets(d *doc, terms []string, most int) []proto.Snippet {
	type at struct{ msg, hit int }
	var hits []at
	for _, t := range terms {
		off := 0
		for range 8 {
			i := strings.Index(d.text[off:], t)
			if i < 0 {
				break
			}
			i += off
			off = i + len(t)
			m := strings.LastIndexByte(d.text[:i], sep)
			if m < 0 {
				continue
			}
			if !slices.ContainsFunc(hits, func(h at) bool { return h.msg == m }) {
				hits = append(hits, at{m, i})
			}
		}
	}
	slices.SortFunc(hits, func(x, y at) int { return x.msg - y.msg })
	if len(hits) == 0 {
		if m := strings.IndexByte(d.text, sep); m >= 0 {
			hits = append(hits, at{m, m + 2})
		}
	}
	if len(hits) > most {
		hits = hits[:most]
	}
	out := make([]proto.Snippet, 0, len(hits))
	for _, h := range hits {
		start := h.msg + 2 // past sep and the role
		end := len(d.text)
		if j := strings.IndexByte(d.text[start:], sep); j >= 0 {
			end = start + j
		}
		out = append(out, snippet(d, start, end, h.hit, terms))
	}
	return out
}

// snippetBefore and snippetAfter are how much of a message a snippet
// shows around the word it found: some before, to read it in, more
// after.
const (
	snippetBefore = 60
	snippetAfter  = 240
)

// snippet is the message of d.Text from start to end around at, with
// the query's words marked; its lines are spaces, to read as one.
func snippet(d *doc, start, end, at int, terms []string) proto.Snippet {
	from, to := start, end
	if at-snippetBefore > from {
		from = at - snippetBefore
		for from < at && !utf8.RuneStart(d.Text[from]) {
			from++
		}
		// From a word's start.
		if i := strings.IndexAny(d.Text[from:at], " \n\t"); i >= 0 && i < 20 {
			from += i + 1
		}
	}
	if at+snippetAfter < to {
		to = at + snippetAfter
		for to > at && !utf8.RuneStart(d.Text[to]) {
			to--
		}
	}
	text := d.Text[from:to]
	low := d.text[from:to]
	var b strings.Builder
	lead := 0
	if from > start {
		b.WriteString("…")
		lead = b.Len()
	}
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		b.WriteRune(r)
	}
	if to < end {
		b.WriteString("…")
	}
	sn := proto.Snippet{User: d.Text[start-1] == roleUser, Text: b.String()}
	sn.Marks = marks(low, terms, lead)
	return sn
}

// Marks are the byte ranges of s that have terms, as a search's Terms
// are, in lower case: a start and an end in turn.
func Marks(s string, terms []string) []int { return marks(Fold(s), terms, 0) }

// marks are the byte ranges of low that have terms, merged, moved by
// lead.
func marks(low string, terms []string, lead int) []int {
	var spans [][2]int
	for _, t := range terms {
		for off := 0; ; {
			i := strings.Index(low[off:], t)
			if i < 0 {
				break
			}
			i += off
			spans = append(spans, [2]int{i, i + len(t)})
			off = i + len(t)
		}
	}
	slices.SortFunc(spans, func(x, y [2]int) int { return x[0] - y[0] })
	var out []int
	for _, s := range spans {
		s[0], s[1] = s[0]+lead, s[1]+lead
		if n := len(out); n > 0 && s[0] <= out[n-1] {
			out[n-1] = max(out[n-1], s[1])
			continue
		}
		out = append(out, s[0], s[1])
	}
	return out
}

// Fold is s in lower case, of the same length in bytes, so that where a
// word is found in it, it is in s: a letter whose lower case is longer,
// or shorter, stays as it is.
func Fold(s string) string {
	i := 0
	for ; i < len(s); i++ {
		if c := s[i]; c >= utf8.RuneSelf || ('A' <= c && c <= 'Z') {
			break
		}
	}
	if i == len(s) {
		return s
	}
	b := []byte(s)
	for i < len(b) {
		c := b[i]
		if c < utf8.RuneSelf {
			if 'A' <= c && c <= 'Z' {
				b[i] = c + 'a' - 'A'
			}
			i++
			continue
		}
		r, n := utf8.DecodeRune(b[i:])
		if l := unicode.ToLower(r); l != r && utf8.RuneLen(l) == n {
			utf8.EncodeRune(b[i:], l)
		}
		i += n
	}
	return string(b)
}
