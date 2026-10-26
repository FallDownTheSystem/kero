package main

import (
	"path"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/client"
	"kero/internal/proto"

	"kero/locales"
)

// The Search view searches the text of the files of the folder the Files
// view shows, as kerod finds them (fs.search): as the user types, of any
// case unless Match Case, as a word with Whole Word, and as a pattern with
// Regular Expression. Its matches, a line each under their file, open the
// file at their line in an editor pane.

// sideSearch is the Search view's id, as prefs.Side holds it.
const sideSearch = "search"

// searchState is the Search view's.
type searchState struct {
	query                  string
	matchCase, word, regex bool
	// typed is when the query last changed, which searches once the user
	// stops typing; zero searches at once.
	typed time.Time
	// asked is the search asked for last, and res what came of it, err
	// why nothing did; gen tells the search asked for last from those
	// still under way.
	asked searchAsk
	res   *proto.FSSearch
	resOf searchAsk // the search res came of
	err   string
	busy  bool
	gen   int
	// folded are the files the user folded, by gotoKey of their path.
	folded map[string]bool
	focus  bool // the field takes the keyboard, as the view shows
	list   ui.ListState
}

// searchAsk is a search of a host's folder.
type searchAsk struct {
	host, root, query      string
	matchCase, word, regex bool
	hidden                 bool
}

// searchDelay is how long after the user stops typing the view searches.
const searchDelay = 300 * time.Millisecond

// toggleSearch shows the Search view, its field taking the keyboard, or
// hides the panel when it shows it.
func (a *App) toggleSearch() {
	if a.prefs.Side != sideSearch {
		a.showSearch("")
		return
	}
	a.toggleSide(sideSearch)
}

// showSearch shows the Search view, its field taking the keyboard: to
// search for query at once, unless "".
func (a *App) showSearch(query string) {
	a.showSide(sideSearch)
	ss := &a.side.search
	ss.focus = true
	if query != "" {
		ss.query, ss.typed = query, time.Time{}
	}
}

// search asks kerod for what the view searches.
func (a *App) search(cl *client.Client, ask searchAsk) {
	ss := &a.side.search
	ss.gen++
	ss.asked, ss.busy, ss.err = ask, true, ""
	gen := ss.gen
	go func() {
		var r proto.FSSearch
		err := cl.Call("fs.search", proto.FSSearchParams{Dir: ask.root, Query: ask.query, Case: ask.matchCase, Word: ask.word, Regex: ask.regex, Hidden: ask.hidden}, &r)
		a.update(func() {
			ss := &a.side.search
			if ss.gen != gen {
				return
			}
			ss.busy = false
			if err != nil {
				ss.err, ss.res = err.Error(), nil
				return
			}
			ss.res, ss.resOf = &r, ask
		})
	}()
}

// searchRow is a row of the matches: a file's header, or one of its
// matches.
type searchRow struct {
	file, match int // match is -1 for the header
}

func (a *App) searchView(c *ui.Context, pal *palette, s *proto.Session) {
	ss := &a.side.search
	pane, root := a.filesRoot(s)
	if pane == nil {
		a.sideMessage(c, pal, locales.SearchNoFolder(), locales.SearchNoFolderDetail())
		return
	}
	h := a.hostOf(pane.ID)
	want := searchAsk{host: h.name, root: root, query: ss.query, matchCase: ss.matchCase, word: ss.word, regex: ss.regex, hidden: a.set.HiddenFiles}
	switch {
	case want == ss.asked:
	case ss.query == "":
		ss.gen++
		ss.asked, ss.res, ss.err, ss.busy = want, nil, "", false
	case h.cl == nil:
	case c.Now().Sub(ss.typed) < searchDelay:
		c.After(searchDelay - c.Now().Sub(ss.typed))
	default:
		a.search(h.cl, want)
	}
	// What was found in another folder, as of another session, never shows
	// as this one's, whose paths its matches would open.
	res := ss.res
	if ss.resOf.host != h.name || ss.resOf.root != root {
		res = nil
	}

	ui.Column(c.Key("search-bar")).Shrink(0).Padding(2, 12, 6, 12).Gap(6).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Height(30).Gap(2).AlignItems(ui.Center).Children(func() {
			in := textField(c, "query", &ss.query, locales.SearchTextPlaceholder(), 0, pal).Bind(a.paneRef("search-in"))
			if in.Changed() {
				ss.typed = c.Now()
			}
			in.OnSubmit(func() {
				// Searches again what it found, as the files change.
				ss.typed, ss.asked = time.Time{}, searchAsk{}
			})
			if ss.focus {
				ss.focus = false
				a.paneRef("search-in").Focus()
			}
			toggle := func(key string, svg *ui.SVG, label string, on *bool) {
				b := ui.ButtonBase(c.Key(key)).Size(26, 26).Radius(6).Center().TextColor(pal.textMuted).Label(label).Tooltip(label).KeepFocus()
				switch {
				case *on:
					b.Background(pal.accent.Alpha(0.16)).TextColor(pal.text)
				case b.Hovered() || b.Pressed():
					b.Background(pal.rowHover.Alpha(0.5)).TextColor(pal.text)
				}
				b.Children(func() { ui.Icon(c, svg).Size(15, 15) })
				if b.Clicked() {
					*on, ss.typed = !*on, time.Time{}
				}
			}
			// Apart from the field, which the toggles would hug.
			ui.Box(c).Width(4).Shrink(0)
			toggle("case", caseIcon, locales.SearchMatchCase(), &ss.matchCase)
			toggle("word", wordIcon, locales.SearchWholeWord(), &ss.word)
			toggle("regex", regexIcon, locales.SearchRegex(), &ss.regex)
		})
		ui.Row(c).Height(22).Gap(8).AlignItems(ui.Center).Children(func() {
			switch {
			case ss.err != "":
				ui.Text(c, ss.err).FontSize(12.5).TextColor(pal.danger).SingleLine().Ellipsis("…").MinWidth(0).Tooltip(ss.err)
			case res == nil:
				ui.Text(c, locales.SearchIn(h.shortDir(root))).FontSize(12.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…").MinWidth(0)
			default:
				n := 0
				for _, f := range res.Files {
					n += len(f.Matches)
				}
				label := locales.SearchResults(n, len(res.Files))
				if res.More {
					label = locales.SearchResultsMore(n, len(res.Files))
				}
				ui.Text(c, label).FontSize(12.5).TextColor(pal.textMuted).SingleLine().Ellipsis("…").MinWidth(0)
			}
			ui.Spacer(c)
			if ss.busy {
				ui.Spinner(c).Size(12, 12)
			}
			if res != nil && len(res.Files) > 0 {
				keys := make([]string, len(res.Files))
				for i, f := range res.Files {
					keys[i] = gotoKey(h, joinRel(root, f.Path))
				}
				fold := false
				for _, k := range keys {
					fold = fold || !ss.folded[k]
				}
				icon, tip := unfoldAllIcon, locales.SearchUnfoldAll()
				if fold {
					icon, tip = foldAllIcon, locales.SearchFoldAll()
				}
				iconButton(c, "fold-all", icon, tip, pal, func() {
					if ss.folded == nil {
						ss.folded = map[string]bool{}
					}
					for _, k := range keys {
						ss.folded[k] = fold
					}
				})
			}
		})
	})

	if res == nil || len(res.Files) == 0 {
		if res != nil && !ss.busy {
			a.sideMessage(c, pal, locales.SearchNone(), locales.SearchNoneDetail(ss.asked.query))
		}
		return
	}
	var rows []searchRow
	for i, f := range res.Files {
		rows = append(rows, searchRow{file: i, match: -1})
		if !ss.folded[gotoKey(h, joinRel(root, f.Path))] {
			for j := range f.Matches {
				rows = append(rows, searchRow{file: i, match: j})
			}
		}
	}
	ls := &ss.list
	ls.Header = func(i int) bool { return rows[i].match < 0 }
	ls.Key = func(i int) any { r := rows[i]; return [2]any{res.Files[r.file].Path, r.match} }
	ui.List(c.Key("matches"), ls, len(rows), func(i int) {
		r := rows[i]
		f := res.Files[r.file]
		file := joinRel(root, f.Path)
		if r.match < 0 {
			a.searchFileRow(c, pal, h, f, file)
			return
		}
		m := f.Matches[r.match]
		row := ui.Row(c).FillWidth().Height(24).Padding(0, 12, 0, 12).Gap(8).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if row.Hovered() {
			row.Background(pal.rowHover)
		}
		if row.Clicked() {
			id, line := pane.ID, m.Line
			a.defer_(func() { a.openAt(id, file, line) })
		}
		row.Children(func() {
			ui.Text(c, itoa(m.Line)).FontSize(11.5).FontFeatures("tnum").TextColor(pal.textFaint).SingleLine().Width(34).TextAlign(ui.End).Shrink(0)
			size := float32(12.5)
			font := a.termFamily()
			start, end := min(max(m.Start, 0), len(m.Text)), min(max(m.End, 0), len(m.Text))
			hit := pal.accent
			ui.RichText(c,
				ui.Span{Text: m.Text[:start], Font: font, Size: size, Color: pal.textMuted},
				ui.Span{Text: m.Text[start:max(start, end)], Font: font, Size: size, Color: pal.text, Background: hit.Alpha(0.28), Weight: 600},
				ui.Span{Text: m.Text[max(start, end):], Font: font, Size: size, Color: pal.textMuted},
			).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		})
	}).Grow(1).MinHeight(0).Padding(0, 0, 16, 0).ScrollbarInsets(stickyH, 0, 0, 0)
}

// searchFileRow is a file's header among the matches, which a click folds
// or unfolds.
func (a *App) searchFileRow(c *ui.Context, pal *palette, h *host, f proto.FSSearchFile, file string) {
	ss := &a.side.search
	key := gotoKey(h, file)
	folded := ss.folded[key]
	row := ui.Row(c).FillWidth().Height(stickyH).Padding(0, 12).Gap(7).AlignItems(ui.Center).Background(pal.card).Cursor(ui.CursorPointer).
		Label(f.Path).Tooltip(f.Path)
	if row.Hovered() {
		row.Background(pal.rowHover)
	}
	if row.Clicked() {
		if ss.folded == nil {
			ss.folded = map[string]bool{}
		}
		ss.folded[key] = !folded
	}
	row.Children(func() {
		chev := ui.Icon(c, chevronIcon).Size(10, 10).TextColor(pal.textFaint).Shrink(0)
		chev.Rotate(chev.AnimateTo("turn", 90*(1-b2f(folded)), chevronTurn))
		ui.Icon(c, docIcon).Size(13, 13).TextColor(pal.textFaint).Shrink(0)
		ui.Text(c, path.Base(f.Path)).FontSize(13).FontWeight(600).TextColor(pal.text).SingleLine().Ellipsis("…").Shrink(0).MaxWidth(220)
		if dir := path.Dir(f.Path); dir != "." {
			ui.Text(c, strings.ReplaceAll(dir, "/", " / ")).FontSize(12).TextColor(pal.textFaint).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		}
		ui.Spacer(c)
		ui.Box(c).Height(16).Padding(0, 6).Radius(8).Center().Background(pal.rowHover).Shrink(0).Children(func() {
			ui.Text(c, itoa(len(f.Matches))).FontSize(11).FontFeatures("tnum").TextColor(pal.textMuted).SingleLine()
		})
	})
}
