package main

import (
	"cmp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"kero/internal/convsearch"
	"kero/internal/proto"
	"kero/locales"
)

// The sidebar's search of the coding agents' conversations, as
// fast-resume's: a field at its foot finds them by what was said in them,
// in every host's agents' histories, whether Kero ran them or not, and
// the sidebar shows what it found in place of the sessions: each to open
// for a look at the messages that have the words, and to resume.

// convSearch is the search's state.
type convSearch struct {
	query string
	input ui.Handle
	focus bool // the field takes the keyboard, its text selected
	// typed is when the query last changed: it goes to the hosts once
	// typing pauses (searchPause). asked is the query that went, gen
	// counts those that did, and again is when to ask once more, while a
	// host reads its histories.
	typed time.Time
	asked string
	gen   int
	again time.Time
	// res is each host's answer to the query of resGen; hits are theirs,
	// the best first.
	res    map[*host]proto.SearchResult
	resGen int
	hits   []searchHit
	// chosen is the key of the hit the arrows chose, "" for the first;
	// open the one that shows its messages; reveal scrolls chosen into
	// view, once.
	chosen string
	open   string
	reveal bool
}

// searchHit is a conversation a host's search found.
type searchHit struct {
	h *host
	proto.SearchHit
}

func (s *searchHit) key() string { return s.h.name + "/" + s.Agent + "/" + s.ID }

const (
	searchPause = 120 * time.Millisecond
	searchLimit = 50
)

// searching tells that the sidebar shows what the search found.
func (a *App) searching() bool { return strings.TrimSpace(a.srch.query) != "" }

// focusSearch shows the sidebar, and puts the keyboard in the search.
func (a *App) focusSearch() {
	if a.prefs.SidebarHidden {
		a.toggleSidebar()
	}
	a.srch.focus = true
}

// endSearch clears the search, and gives the keyboard back to the
// terminal.
func (a *App) endSearch() {
	a.srch = convSearch{gen: a.srch.gen}
	a.focusTerminal()
}

// askSearch sends the query to the hosts once typing pauses, and again
// while one reads its histories, as what it read so far may answer more.
func (a *App) askSearch(c *ui.Context) {
	s := &a.srch
	q := strings.TrimSpace(s.query)
	now := c.Now()
	if q == "" {
		if s.asked != "" {
			s.asked, s.res, s.hits, s.open, s.chosen, s.again = "", nil, nil, "", "", time.Time{}
			s.gen++
		}
		return
	}
	switch {
	case q != s.asked:
		if wait := searchPause - now.Sub(s.typed); wait > 0 {
			c.After(wait)
			return
		}
	case !s.again.IsZero() && now.Before(s.again):
		c.After(s.again.Sub(now))
		return
	case s.again.IsZero():
		return
	}
	s.gen++
	gen := s.gen
	if q != s.asked {
		s.chosen, s.reveal = "", true
	}
	s.asked, s.again = q, time.Time{}
	for _, h := range a.hosts {
		cl := h.cl
		if cl == nil {
			continue
		}
		go func() {
			var r proto.SearchResult
			err := cl.Call("conversation.search", proto.SearchParams{Query: q, Limit: searchLimit, Preview: true}, &r)
			a.update(func() {
				if s.gen != gen {
					return
				}
				if s.resGen != gen {
					s.res, s.resGen = map[*host]proto.SearchResult{}, gen
				}
				// An older kerod has no search: it finds nothing.
				if err == nil {
					s.res[h] = r
					if r.Indexing && s.again.IsZero() {
						s.again = time.Now().Add(700 * time.Millisecond)
					}
				}
				a.mergeHits()
			})
		}()
	}
}

// mergeHits puts the hosts' hits in one list, the best first.
func (a *App) mergeHits() {
	s := &a.srch
	s.hits = s.hits[:0]
	for _, h := range a.hosts {
		for _, hit := range s.res[h].Hits {
			s.hits = append(s.hits, searchHit{h, hit})
		}
	}
	slices.SortStableFunc(s.hits, func(x, y searchHit) int {
		if c := cmp.Compare(y.Score, x.Score); c != 0 {
			return c
		}
		return cmp.Compare(y.Updated, x.Updated)
	})
}

// searchState is how the hosts' searches stand: how many they found, and
// how much of their histories those still reading them have read.
func (a *App) searchState() (matched int, indexing bool, done, todo int) {
	for _, r := range a.srch.res {
		matched += r.Matched
		if r.Indexing {
			indexing = true
			done += r.Done
			todo += r.Todo
		}
	}
	return
}

// chosenHit is the index of the hit the arrows chose, -1 for none.
func (a *App) chosenHit() int {
	s := &a.srch
	if len(s.hits) == 0 {
		return -1
	}
	if i := slices.IndexFunc(s.hits, func(h searchHit) bool { return h.key() == s.chosen }); i >= 0 {
		return i
	}
	return 0
}

// moveHit chooses the hit by, after or before the one chosen.
func (a *App) moveHit(by int) {
	s := &a.srch
	if i := a.chosenHit(); i >= 0 {
		i = min(max(i+by, 0), len(s.hits)-1)
		s.chosen, s.reveal = s.hits[i].key(), true
	}
}

// resumeHit shows the session that runs a conversation found, or starts
// one that resumes it; the search is done, as the palette is with what
// it picks, and the sidebar shows the session.
func (a *App) resumeHit(hit searchHit) {
	a.srch = convSearch{gen: a.srch.gen}
	if hit.Session != "" && a.session(hit.Session) != nil {
		a.selectSession(hit.Session)
		a.focusTerminal()
		return
	}
	a.resumeConversation(hit.h, hit.Conversation)
}

// searchField is the search's field, over the sidebar's foot: the arrows
// choose among what it found, Enter resumes the one chosen, and Escape
// clears it.
func (a *App) searchField(c *ui.Context, pal *palette) {
	s := &a.srch
	ui.Row(c.Key("search")).Height(34).Shrink(0).Padding(6, 12, 0, 12).Gap(2).AlignItems(ui.Center).Children(func() {
		in := textField(c, "q", &s.query, locales.SearchPlaceholder(), 0, pal).Bind(&s.input)
		in.Tooltip(locales.SearchTip())
		if s.focus {
			s.focus = false
			in.Focus().SetTextSelection(0, utf8.RuneCountInString(s.query))
		}
		if in.Changed() {
			s.typed = c.Now()
		}
		in.OnShortcut(0, ui.KeyEscape, func() { a.defer_(a.endSearch) })
		in.OnShortcut(0, ui.KeyDown, func() { a.moveHit(1) })
		in.OnShortcut(0, ui.KeyUp, func() { a.moveHit(-1) })
		in.OnSubmit(func() {
			if i := a.chosenHit(); i >= 0 {
				hit := s.hits[i]
				a.defer_(func() { a.resumeHit(hit) })
			}
		})
		if _, indexing, _, _ := a.searchState(); indexing && a.searching() {
			ui.Spinner(c).Size(12, 12).Margin(0, 4)
		}
		if s.query != "" {
			iconButton(c, "clear", closeIcon, locales.SearchClear(), pal, func() { a.defer_(a.endSearch) })
		}
	})
	a.askSearch(c)
}

// searchResults are what the search found, in place of the sessions.
func (a *App) searchResults(c *ui.Context, pal *palette) {
	s := &a.srch
	matched, indexing, done, todo := a.searchState()
	note := ""
	switch {
	case indexing && todo > 0:
		note = locales.SearchIndexing(done, todo)
	case s.res == nil:
	case matched == 0:
		note = locales.SearchNoMatches()
	default:
		note = locales.SearchCount(matched)
	}
	ui.Text(c, note).FontSize(12).TextColor(pal.textFaint).FontFeatures("tnum").Padding(4, 6, 6, rowInset).SingleLine().Ellipsis("…")
	chosen := a.chosenHit()
	now := c.Now()
	for i := range s.hits {
		a.hitRow(c, pal, &s.hits[i], i == chosen, now)
	}
	s.reveal = false
}

// hitRow is a conversation found: its agent, folder, the times the user
// wrote in it and when it last ran, then its title; open, the messages
// that have the query's words.
func (a *App) hitRow(c *ui.Context, pal *palette, hit *searchHit, chosenRow bool, now time.Time) {
	s := &a.srch
	key := hit.key()
	open := s.open == key
	card := ui.Column(c.Key("hit-"+key)).Radius(8).Margin(0, 0, 1, 0)
	if pal.dark {
		card.Border(1, ui.Transparent)
	}
	bg := ui.Transparent
	switch {
	case open || chosenRow:
		bg = pal.rowSelected
		chosen(card, pal)
	case card.Hovered():
		bg = pal.rowHover
	}
	card.Background(card.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	if hit.h.remote() && hit.h.conn != connReady {
		card.Opacity(0.55)
	}
	if chosenRow && s.reveal {
		card.ScrollIntoView()
	}
	card.Children(func() {
		// The head alone opens and closes it: the messages below are to
		// read, and select.
		head := ui.Column(c.Key("head")).Padding(6, 8, 6, rowInset).Gap(3)
		if head.Clicked() {
			s.chosen = key
			if open {
				s.open = ""
			} else {
				s.open = key
			}
		}
		if head.DoubleClicked() {
			h := *hit
			a.defer_(func() { a.resumeHit(h) })
		}
		head.ContextMenu(func(m *ui.Menu) { a.hitMenu(m, *hit) })
		head.Children(func() {
			ui.Row(c).Height(18).Gap(6).AlignItems(ui.Center).Children(func() {
				icon := programIcons[hit.Agent]
				if icon == nil {
					icon = terminalIcon
				}
				ui.Icon(c, icon).Size(13, 13).TextColor(pal.textMuted).Shrink(0)
				ui.Text(c, agentShort(hit.Agent)).FontSize(12).FontWeight(500).TextColor(pal.textMuted).SingleLine().Shrink(0)
				if dir := a.folderShown(c, "", hit.h.shortDir(hit.Dir), proto.DirName(hit.Dir), key, 12, 6, 0); dir != "" {
					ui.Text(c, dir).FontSize(12).TextColor(pal.textFaint).SingleLine().Ellipsis("…").Shrink(2).MinWidth(0).Bind(a.paneRef(key + "-dir"))
				}
				ui.Box(c).Grow(1).AlignSelf(ui.Stretch).Bind(a.paneRef(key + "-gap"))
				if hit.h.remote() {
					a.hostTag(c, pal, hit.h)
				}
				turns := locales.SearchTurns(hit.Turns)
				ui.Row(c.Key("turns")).Gap(3).AlignItems(ui.Center).Shrink(0).Tooltip(turns).Description(turns).Children(func() {
					ui.Icon(c, turnsIcon).Size(11, 11).TextColor(pal.textFaint)
					ui.Textf(c, "%d", hit.Turns).FontSize(12).TextColor(pal.textFaint).FontFeatures("tnum").SingleLine()
				})
				when := ago(hit.Updated, now)
				if hit.Session != "" {
					when = locales.PaletteConversationRunning()
				}
				ui.Text(c, when).FontSize(12).TextColor(pal.textFaint).FontFeatures("tnum").SingleLine().Shrink(0)
			})
			title := cmp.Or(hit.Name, hit.Title, proto.DirName(hit.Dir))
			t := ui.RichText(c, markedSpans(title, convsearch.Marks(title, s.res[hit.h].Terms), 13, 0, pal.text, pal)...)
			if open {
				t.LineHeight(1.3)
			} else {
				t.SingleLine().Ellipsis("…")
			}
		})
		if open {
			a.hitPreview(c, pal, hit)
		}
	})
}

// hitPreview is an open hit's messages that have the query's words, each
// the user's or its agent's, and its full folder, to resume it from.
func (a *App) hitPreview(c *ui.Context, pal *palette, hit *searchHit) {
	ui.Column(c.Key("preview")).Padding(2, 8, 8, rowInset).Gap(8).Children(func() {
		for i, sn := range hit.Snippets {
			who := cmp.Or(proto.AgentNames[hit.Agent], hit.Agent)
			if sn.User {
				who = locales.SearchYou()
			}
			ui.Column(c.Key(itoa(i))).Gap(2).Children(func() {
				ui.Text(c, who).FontSize(11).FontWeight(600).TextColor(pal.textFaint).SingleLine()
				ui.RichText(c, markedSpans(sn.Text, sn.Marks, 12.5, 0, pal.textMuted, pal)...).LineHeight(1.35).Selectable()
			})
		}
		ui.Text(c, hit.h.shortDir(hit.Dir)).FontSize(11.5).TextColor(pal.textFaint).Selectable()
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			label := locales.EndedResume()
			if hit.Session != "" {
				label = locales.SearchShowSession()
			}
			if smallButton(c, "resume", label, true, pal) {
				h := *hit
				a.defer_(func() { a.resumeHit(h) })
			}
		})
	})
}

func (a *App) hitMenu(m *ui.Menu, hit searchHit) {
	label := locales.EndedResume()
	if hit.Session != "" {
		label = locales.SearchShowSession()
	}
	if m.Item(label).Chosen() {
		a.defer_(func() { a.resumeHit(hit) })
	}
	m.Separator()
	if m.Item(locales.SessionCopyId()).Chosen() {
		a.copyText = hit.ID
	}
	if m.Item(locales.SessionCopyDirectory()).Disabled(hit.Dir == "").Chosen() {
		a.copyText = hit.Dir
	}
	if m.Item(locales.SessionRevealIn(fileManager())).Disabled(hit.Dir == "" || hit.h.remote()).Chosen() {
		openPath(hit.Dir)
	}
}

// agentShort is an agent's name, short: Claude, not Claude Code.
func agentShort(agent string) string {
	name := cmp.Or(proto.AgentNames[agent], agent)
	if first, _, ok := strings.Cut(name, " "); ok {
		return first
	}
	return name
}

// markedSpans are text's spans, those of marks, a start and an end in
// turn, marked as a search's matches are.
func markedSpans(text string, marks []int, size float32, weight int, fg ui.Color, pal *palette) []ui.Span {
	var spans []ui.Span
	at := 0
	for i := 0; i+1 < len(marks); i += 2 {
		from, to := marks[i], marks[i+1]
		if from < at || to > len(text) || from >= to {
			continue
		}
		if from > at {
			spans = append(spans, ui.Span{Text: text[at:from], Size: size, Weight: weight, Color: fg})
		}
		spans = append(spans, ui.Span{Text: text[from:to], Size: size, Weight: weight, Color: pal.text, Background: pal.accent.Alpha(0.24), Radius: 3})
		at = to
	}
	if at < len(text) || len(spans) == 0 {
		spans = append(spans, ui.Span{Text: text[at:], Size: size, Weight: weight, Color: fg})
	}
	return spans
}
