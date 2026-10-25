package main

import (
	"path"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"kero/internal/gitgraph"
	"kero/internal/proto"

	"kero/locales"
)

// The Explorer's History view: the worktree's commits as a graph, newest
// first, of its branch, its upstream and the default branch, or of every
// branch; the stash above them; and the changes not committed at the top.
// A commit opens below its row, its message and files, each with its
// diff; its menu checks it out, branches from it, and picks, reverts or
// resets to it. A filter finds commits by message, author or hash, and a
// file's history lists the commits that changed it.

// sideHistory is the History view's id, as prefs.Side holds it.
const sideHistory = "history"

// histPage is how many commits a page of the log has.
const histPage = 200

// The graph's lanes: their width, and how many show before the rest go
// into the last.
const (
	laneW    = 12
	maxLanes = 6
)

// historyState is the History view's.
type historyState struct {
	// key is what the list is of: the worktree, its scope and the filter;
	// stamp the worktree's status it was fetched at.
	key, stamp string
	pane       string
	// scope "branch" shows the branch, its upstream and the default
	// branch alone, rather than every branch. It is a field, not a value
	// made each frame, because segmented writes the choice after the
	// frame is built.
	scope string
	// first shows the first parents alone.
	first bool
	// query is what the filter has, asked what was fetched for it, and
	// typed when it last changed; path the file whose history it is.
	query, asked string
	typed        time.Time
	path         string
	commits      []proto.GitCommit
	graph        []gitgraph.Row
	layout       gitgraph.Layout
	more         bool
	unpushed     map[string]bool
	remotes      []string
	busy         bool
	err          string
	// opened are the commits shown open, by hash, and their contents.
	opened  map[string]*commitView
	stashes []proto.GitStash
	stashOn bool // the stash's section is open
	stashAt string
	list    ui.ListState
}

func (hs *historyState) branchOnly() bool { return hs.scope == "branch" }

// commitView is an open commit's: what kerod told of it, and its files'
// diffs, by path.
type commitView struct {
	info  *proto.GitCommitInfo
	busy  bool
	err   string
	files map[string]*sideFile
}

// historyDir is the worktree the History view shows.
func (a *App) historyDir(s *proto.Session) string {
	if here, _ := a.changesPanes(s); here != nil && here.Git != nil {
		return here.Git.Root
	}
	return ""
}

// histRow is a row of the History view's list.
type histRow struct {
	kind   int
	commit int // the commit's, of hs.commits, for rowCommit and the rows it opens
	stash  int // the stash's, of hs.stashes
	f      proto.GitFile
	hash   string // the open commit's or stash's whose detail or file the row is
}

const (
	rowStashes = iota
	rowStash
	rowUncommitted
	rowCommit
	rowDetail
	rowFile
	rowDiff
	rowMore
)

func (a *App) historyView(c *ui.Context, pal *palette, s *proto.Session, width float32) {
	hs := &a.side.git.hist
	here, _ := a.changesPanes(s)
	if !a.set.GitStatus {
		a.sideMessage(c, pal, locales.ChangesGitOff(), locales.ChangesGitOffDetail())
		return
	}
	if here == nil || here.Git == nil {
		a.sideMessage(c, pal, locales.ChangesNoRepo(), locales.ChangesNoRepoDetail())
		return
	}
	g := here.Git
	a.gitBar(c, pal, s, here)
	a.historyFilter(c, pal, g.Root)

	// The list follows the worktree, the scope and the filter.
	if q := strings.TrimSpace(hs.query); q != hs.asked && time.Since(hs.typed) > 250*time.Millisecond {
		hs.asked = q
	} else if q != hs.asked {
		c.After(260 * time.Millisecond)
	}
	key := strings.Join([]string{g.Root, b2s(hs.branchOnly()), b2s(hs.first), hs.asked, hs.path}, "|")
	stamp := gitStamp(g) + "|" + itoa(a.side.git.done)
	if key != hs.key {
		hs.key, hs.stamp, hs.commits, hs.graph, hs.layout, hs.more, hs.err = key, "", nil, nil, gitgraph.Layout{}, false, ""
		hs.list = ui.ListState{}
	}
	if stamp != hs.stamp && !hs.busy {
		a.fetchHistory(here.ID, key, stamp, 0)
	}
	if hs.stashAt != stamp {
		a.fetchStashes(here.ID, stamp)
	}

	filtered := hs.asked != "" || hs.path != ""
	var rows []histRow
	if !filtered {
		if len(hs.stashes) > 0 {
			rows = append(rows, histRow{kind: rowStashes})
			if hs.stashOn {
				for i, st := range hs.stashes {
					rows = append(rows, histRow{kind: rowStash, stash: i, hash: st.Hash})
					rows = a.openRows(rows, st.Hash, -1)
				}
			}
		}
		if g.Files > 0 {
			rows = append(rows, histRow{kind: rowUncommitted})
		}
	}
	for i, cm := range hs.commits {
		rows = append(rows, histRow{kind: rowCommit, commit: i, hash: cm.Hash})
		rows = a.openRows(rows, cm.Hash, i)
	}
	if hs.more || hs.busy && len(hs.commits) > 0 {
		rows = append(rows, histRow{kind: rowMore})
	}

	switch {
	case len(hs.commits) == 0 && hs.err != "":
		a.sideMessage(c, pal, locales.HistoryCantRead(), hs.err)
		return
	case len(hs.commits) == 0 && hs.busy:
		ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c) })
		return
	case len(rows) == 0 && filtered:
		a.sideMessage(c, pal, locales.HistoryNoMatch(), "")
		return
	case len(rows) == 0:
		a.sideMessage(c, pal, locales.HistoryNone(), locales.HistoryNoneDetail())
		return
	}
	gw := float32(0)
	if !filtered {
		for _, r := range hs.graph {
			gw = max(gw, float32(min(r.Width, maxLanes))*laneW)
		}
	}
	// The list leaves room at its right, past the rows' times, for the
	// scroll bar.
	ls := &hs.list
	ls.Key =func(i int) any { r := rows[i]; return [3]any{r.kind, r.hash, r.f.Path} }
	ui.List(c.Key("history-"+hs.key), ls, len(rows), func(i int) {
		r := rows[i]
		switch r.kind {
		case rowStashes:
			a.stashesRow(c, pal)
		case rowStash:
			a.stashRow(c, pal, here, hs.stashes[r.stash])
		case rowUncommitted:
			a.uncommittedRow(c, pal, g, gw)
		case rowCommit:
			a.commitRow(c, pal, s, here, r.commit, gw, filtered)
			// The next page, as the list comes near the end of this one.
			if hs.more && !hs.busy && i >= len(rows)-30 && !filtered {
				a.fetchHistory(here.ID, hs.key, hs.stamp, len(hs.commits))
			}
		case rowDetail:
			a.commitDetail(c, pal, r, gw, filtered)
		case rowFile:
			a.histFile(c, pal, here, r, gw, filtered)
		case rowDiff:
			if cv := hs.opened[r.hash]; cv != nil && cv.files[r.f.Path] != nil {
				a.histDiff(c, pal, s, r, cv.files[r.f.Path], gw, filtered, width)
			}
		case rowMore:
			ui.Row(c.Key("more")).Height(40).Center().Children(func() {
				if hs.busy {
					ui.Spinner(c)
				} else if filtered {
					ui.Text(c, locales.HistoryMoreFound()).FontSize(12).TextColor(pal.textFaint)
				}
			})
		}
	}).Grow(1).MinHeight(0).Padding(0, 6, 16, 0)
	a.syncHistoryFiles(here)
}

func b2s(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// openRows adds the rows an open commit or stash shows below its own: its
// message, then its files, each with its diff while unfolded.
func (a *App) openRows(rows []histRow, hash string, commit int) []histRow {
	cv := a.side.git.hist.opened[hash]
	if cv == nil {
		return rows
	}
	rows = append(rows, histRow{kind: rowDetail, commit: commit, hash: hash})
	if cv.info == nil {
		return rows
	}
	for _, f := range cv.info.Files {
		rows = append(rows, histRow{kind: rowFile, commit: commit, hash: hash, f: f})
		if !a.fileCollapsed(histKey(hash, f.Path), f) {
			rows = append(rows, histRow{kind: rowDiff, commit: commit, hash: hash, f: f})
		}
	}
	return rows
}

// histKey is the key an open commit's file's fold is kept by, with the
// Changes view's (fileCollapsed).
func histKey(hash, path string) string { return "commit|" + hash + "|" + path }

// historyFilter is the row under the bar: the scope, the filter, and the
// first-parent switch; a file's history in place of the scope.
func (a *App) historyFilter(c *ui.Context, pal *palette, root string) {
	hs := &a.side.git.hist
	ui.Row(c.Key("hist-filter")).Shrink(0).Padding(2, 10, 6, 12).Gap(6).AlignItems(ui.Center).Children(func() {
		if hs.path != "" {
			chip := ui.Row(c.Key("path")).Height(26).Padding(0, 4, 0, 9).Gap(4).Radius(7).AlignItems(ui.Center).Background(pal.chip).Shrink(1).MinWidth(0).Tooltip(hs.path)
			chip.Children(func() {
				ui.Text(c, locales.HistoryOf(path.Base(hs.path))).FontSize(12.5).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0)
				iconButton(c, "clear-path", closeIcon, locales.HistoryAllFiles(), pal, func() { hs.path = "" })
			})
		} else {
			segmented(c, "hist-scope", &hs.scope, []string{"all", "branch"}, []string{locales.HistoryAll(), locales.HistoryBranch()})
		}
		in := textField(c, "hist-q", &hs.query, locales.HistoryFilter(), 0, pal).Tooltip(locales.HistoryFilterTip())
		in.OnChange(func() { hs.typed = time.Now() })
		in.OnShortcut(0, ui.KeyEscape, func() { hs.query, hs.asked = "", "" })
		tip := locales.HistoryFirstParent()
		b := ui.ButtonBase(c.Key("first")).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).Label(tip).Tooltip(tip)
		if hs.first {
			b.Background(pal.rowHover).TextColor(pal.text)
		} else if b.Hovered() {
			b.Background(pal.rowHover.Alpha(0.5))
		}
		// A line of commits, as the history shows with one parent each.
		b.Children(func() { ui.Icon(c, commitIcon).Size(14, 14).Rotate(90) })
		if b.Clicked() {
			hs.first = !hs.first
		}
	})
}

// fetchHistory asks for a page of the log, from skip, for the list key
// names; a first page takes the list's place as it comes.
func (a *App) fetchHistory(pane, key, stamp string, skip int) {
	hs := &a.side.git.hist
	cl := a.hostOf(pane).cl
	if cl == nil {
		return
	}
	hs.busy, hs.pane = true, pane
	p := proto.GitLogParams{Pane: pane, All: !hs.branchOnly(), FirstParent: hs.first, Skip: skip, Max: histPage, Query: hs.asked, Path: hs.path}
	go func() {
		var res proto.GitLog
		err := cl.Call("git.log", p, &res)
		a.update(func() {
			hs.busy = false
			if hs.key != key {
				return
			}
			hs.stamp = stamp
			if err != nil {
				hs.err = err.Error()
				return
			}
			hs.err = ""
			if skip == 0 {
				hs.commits, hs.graph, hs.layout = nil, nil, gitgraph.Layout{}
				hs.unpushed = map[string]bool{}
				for _, h := range res.Unpushed {
					hs.unpushed[h] = true
				}
				hs.remotes = res.Remotes
			} else if skip != len(hs.commits) {
				return // a page of a list since fetched anew
			}
			for _, cm := range res.Commits {
				hs.commits = append(hs.commits, cm)
				hs.graph = append(hs.graph, hs.layout.Add(gitgraph.Commit{Hash: cm.Hash, Parents: cm.Parents}))
			}
			hs.more = res.More
		})
	}()
}

// fetchStashes asks for the stash anew.
func (a *App) fetchStashes(pane, stamp string) {
	hs := &a.side.git.hist
	cl := a.hostOf(pane).cl
	if cl == nil {
		return
	}
	hs.stashAt = stamp
	go func() {
		var res proto.GitStashes
		err := cl.Call("git.stashes", proto.GitParams{Pane: pane}, &res)
		a.update(func() {
			if err == nil && hs.stashAt == stamp {
				hs.stashes = res.Stashes
			}
		})
	}()
}

// laneColors are the graph's lanes' colors: the terminal's, as the diffs
// take theirs, but black and white.
func (a *App) laneColors(c *ui.Context, pal *palette) []ui.Color {
	t := a.codeOf(c, pal)
	return []ui.Color{pal.accent, t.Palette[2], t.Palette[5], t.Palette[3], t.Palette[6], t.Palette[1], t.Palette[4], t.Palette[10]}
}

// laneX is where a lane's line runs in a graph's box.
func laneX(r ui.Rect, lane int) float32 {
	return r.X + laneW/2 + float32(min(lane, maxLanes-1))*laneW
}

// drawGraph draws a commit's row of the graph; node is how its dot shows:
// filled for a commit not pushed, a ring for one pushed, HEAD's ringed
// besides.
func drawGraph(p *ui.Painter, r ui.Rect, row gitgraph.Row, colors []ui.Color, bg ui.Color, unpushed, head bool) {
	top, mid, bot := r.Y, r.Y+r.H/2, r.Y+r.H
	col := func(i int) ui.Color { return colors[i%len(colors)] }
	for _, l := range row.Pass {
		var path ui.Path
		path.MoveTo(laneX(r, l.From), top).LineTo(laneX(r, l.To), bot)
		p.StrokePath(&path, 1.5, col(l.Color))
	}
	nx := laneX(r, row.Node)
	for _, l := range row.In {
		var path ui.Path
		x := laneX(r, l.From)
		path.MoveTo(x, top).CubeTo(x, (top+mid)/2, nx, (top+mid)/2, nx, mid)
		p.StrokePath(&path, 1.5, col(l.Color))
	}
	for _, l := range row.Out {
		var path ui.Path
		x := laneX(r, l.To)
		path.MoveTo(nx, mid).CubeTo(nx, (mid+bot)/2, x, (mid+bot)/2, x, bot)
		p.StrokePath(&path, 1.5, col(l.Color))
	}
	var dot ui.Path
	dot.Circle(nx, mid, 3.5)
	if head {
		var ring ui.Path
		ring.Circle(nx, mid, 6)
		p.StrokePath(&ring, 1.5, col(row.Color).Alpha(0.5))
	}
	if unpushed {
		p.FillPath(&dot, col(row.Color))
	} else {
		p.FillPath(&dot, bg)
		p.StrokePath(&dot, 1.5, col(row.Color))
	}
}

// drawLanes draws the lanes that go on past an open commit's row, by the
// rows it opens below it.
func drawLanes(p *ui.Painter, r ui.Rect, row gitgraph.Row, colors []ui.Color) {
	for _, ls := range [][]gitgraph.Line{row.Pass, row.Out} {
		for _, l := range ls {
			var path ui.Path
			x := laneX(r, l.To)
			path.MoveTo(x, r.Y).LineTo(x, r.Y+r.H)
			p.StrokePath(&path, 1.5, colors[l.Color%len(colors)])
		}
	}
}

// graphBox is the box of a row's graph, gw wide, drawn by draw.
func graphBox(c *ui.Context, gw float32, draw func(p *ui.Painter, r ui.Rect)) {
	if gw == 0 {
		return
	}
	ui.Box(c.Key("graph")).Width(gw).Shrink(0).AlignSelf(ui.Stretch).Draw(draw)
}

// commitRow is a commit's row: its graph, subject, refs and time, which a
// click opens or closes.
func (a *App) commitRow(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, i int, gw float32, filtered bool) {
	hs := &a.side.git.hist
	cm := hs.commits[i]
	open := hs.opened[cm.Hash] != nil
	row := ui.Row(c.Key("row")).Height(26).Padding(0, 10, 0, 6).Gap(7).AlignItems(ui.Center).Radius(6)
	if open {
		row.Background(pal.rowHover.Alpha(0.6))
	} else if row.Hovered() {
		row.Background(pal.rowHover.Alpha(0.5))
	}
	when := time.Unix(cm.Time, 0)
	row.Tooltip(cm.Author + " <" + cm.Email + ">\n" + when.Format("2006-01-02 15:04") + "\n" + shortHash(cm.Hash))
	if row.Clicked() {
		a.toggleCommit(pane.ID, cm.Hash)
	}
	row.ContextMenu(func(m *ui.Menu) { a.commitMenu(c, m, s, pane, cm) })
	refs := parseRefs(cm.Refs, hs.remotes)
	head := slices.ContainsFunc(refs, func(r gitRef) bool { return r.head })
	colors := a.laneColors(c, pal)
	row.Children(func() {
		if filtered {
			ui.Box(c).Size(6, 6).Radius(3).Background(pal.textFaint).Shrink(0).Margin(0, 2, 0, 4)
		} else {
			gr := hs.graph[i]
			graphBox(c, gw, func(p *ui.Painter, r ui.Rect) {
				drawGraph(p, r, gr, colors, pal.card, hs.unpushed[cm.Hash], head)
			})
		}
		fg := pal.text
		if !hs.unpushed[cm.Hash] && !filtered {
			fg = pal.text.Alpha(0.85)
		}
		ui.Text(c, cm.Subject).FontSize(13).TextColor(fg).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		a.refChips(c, pal, refs)
		ui.Text(c, ago(cm.Time*1000, time.Now())).FontSize(11.5).FontFeatures("tnum").TextColor(pal.textFaint).SingleLine().Shrink(0)
	})
}

// gitRef is a ref a commit's row shows: a local branch, a remote's, or a
// tag.
type gitRef struct {
	name   string
	remote bool
	tag    bool
	head   bool // HEAD is on it, or at it, detached
}

// parseRefs reads %D's refs: "HEAD -> main, origin/main, tag: v1".
func parseRefs(s string, remotes []string) []gitRef {
	var refs []gitRef
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ", ")
	isRemote := func(name string) bool {
		r, _, ok := strings.Cut(name, "/")
		return ok && slices.Contains(remotes, r)
	}
	for _, p := range parts {
		switch {
		case strings.HasPrefix(p, "HEAD -> "):
			refs = append(refs, gitRef{name: p[len("HEAD -> "):], head: true})
		case p == "HEAD":
			refs = append(refs, gitRef{name: "HEAD", head: true})
		case strings.HasPrefix(p, "tag: "):
			refs = append(refs, gitRef{name: p[len("tag: "):], tag: true})
		case strings.HasSuffix(p, "/HEAD") && isRemote(p):
		case isRemote(p):
			refs = append(refs, gitRef{name: p, remote: true})
		default:
			refs = append(refs, gitRef{name: p})
		}
	}
	return refs
}

// branchRefs are the names of a commit's branches, local and remote.
func branchRefs(refs []gitRef) []string {
	var names []string
	for _, r := range refs {
		if !r.tag && r.name != "HEAD" {
			names = append(names, r.name)
		}
	}
	return names
}

// refChips shows a commit's refs, three at most, then how many more: a
// local branch filled, with the branch's icon, and a remote's outlined,
// with a globe, each apart from the other, so that a branch and its
// remote's at the same commit show both.
func (a *App) refChips(c *ui.Context, pal *palette, refs []gitRef) {
	const shown = 3
	for i, r := range refs {
		if i == shown {
			more := make([]string, 0, len(refs)-shown)
			for _, o := range refs[shown:] {
				more = append(more, o.name)
			}
			ui.Text(c, "+"+itoa(len(refs)-shown)).FontSize(11).TextColor(pal.textFaint).SingleLine().Shrink(0).Tooltip(strings.Join(more, "\n"))
			return
		}
		bg, fg, icon := pal.chip, pal.textMuted, branchIcon
		switch {
		case r.tag:
			icon = tagIcon
		case r.remote:
			bg, fg, icon = ui.Color{}, pal.textFaint, globeIcon
		case r.name == "HEAD":
			icon = nil
		}
		if r.head {
			bg, fg = pal.accent.Alpha(0.16), pal.accent
		}
		chip := ui.Row(c.Key("ref-"+r.name)).Height(18).Padding(0, 6, 0, 5).Gap(3).Radius(5).AlignItems(ui.Center).Background(bg).Shrink(1).MinWidth(0).MaxWidth(130)
		if r.remote {
			chip.Border(1, pal.text.Alpha(0.14))
		}
		chip.Tooltip(r.name).Children(func() {
			if icon != nil {
				ui.Icon(c, icon).Size(9, 9).TextColor(fg).Shrink(0)
			}
			ui.Text(c, r.name).FontSize(11).FontWeight(550).TextColor(fg).SingleLine().Ellipsis("…").MinWidth(0)
		})
	}
}

// shortHash is a commit's hash as git abbreviates it.
func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

// toggleCommit opens a commit, or a stash, below its row, asking kerod
// for it, or closes it.
func (a *App) toggleCommit(pane, hash string) {
	hs := &a.side.git.hist
	if hs.opened == nil {
		hs.opened = map[string]*commitView{}
	}
	if hs.opened[hash] != nil {
		delete(hs.opened, hash)
		return
	}
	cv := &commitView{busy: true, files: map[string]*sideFile{}}
	hs.opened[hash] = cv
	cl := a.hostOf(pane).cl
	if cl == nil {
		return
	}
	go func() {
		var info proto.GitCommitInfo
		err := cl.Call("git.show", proto.GitShowParams{Pane: pane, Rev: hash}, &info)
		a.update(func() {
			cv.busy = false
			if err != nil {
				cv.err = err.Error()
				return
			}
			cv.info = &info
			ch := &a.side.changes
			if ch.collapsed == nil {
				ch.collapsed = map[string]bool{}
			}
			// A commit's files open folded, a list of what it changed, each
			// of which a click unfolds.
			for _, f := range info.Files {
				if _, ok := ch.collapsed[histKey(hash, f.Path)]; !ok {
					ch.collapsed[histKey(hash, f.Path)] = true
				}
			}
		})
	}()
}

// syncHistoryFiles asks for the text of the open commits' files unfolded
// that have none yet, a few at a time.
func (a *App) syncHistoryFiles(pane *proto.Pane) {
	hs := &a.side.git.hist
	asking := 0
	for _, cv := range hs.opened {
		for _, sf := range cv.files {
			if sf.busy {
				asking++
			}
		}
	}
	for hash, cv := range hs.opened {
		if cv.info == nil {
			continue
		}
		for _, f := range cv.info.Files {
			if a.fileCollapsed(histKey(hash, f.Path), f) {
				continue
			}
			sf := cv.files[f.Path]
			if sf == nil {
				sf = &sideFile{pane: pane.ID, base: baseRev, f: f}
				cv.files[f.Path] = sf
			}
			if sf.busy || sf.d != nil || sf.text.Binary || sf.text.Large || sf.err != "" || asking >= 4 {
				continue
			}
			asking++
			a.fetchFile(sf, "")
		}
	}
}

// commitDetail is an open commit's message, author and hash.
func (a *App) commitDetail(c *ui.Context, pal *palette, r histRow, gw float32, filtered bool) {
	hs := &a.side.git.hist
	cv := hs.opened[r.hash]
	colors := a.laneColors(c, pal)
	ui.Row(c.Key("detail")).Padding(0, 10, 0, 6).Gap(7).AlignItems(ui.Stretch).Children(func() {
		if !filtered && r.commit >= 0 {
			gr := hs.graph[r.commit]
			graphBox(c, gw, func(p *ui.Painter, rr ui.Rect) { drawLanes(p, rr, gr, colors) })
		}
		ui.Column(c).Grow(1).MinWidth(0).Padding(4, 0, 8, 0).Gap(4).Children(func() {
			switch {
			case cv == nil:
			case cv.err != "":
				ui.Text(c, cv.err).FontSize(12.5).TextColor(pal.textMuted)
			case cv.info == nil:
				ui.Spinner(c).Size(14, 14)
			default:
				info := cv.info
				if body := strings.TrimSpace(strings.TrimPrefix(info.Message, info.Subject)); body != "" {
					ui.Text(c, body).FontSize(12.5).TextColor(pal.textMuted).LineHeight(1.4).Selectable()
				}
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					meta := info.Author + " · " + time.Unix(info.Time, 0).Format("2006-01-02 15:04")
					ui.Text(c, meta).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
					hash := ui.ButtonBase(c.Key("hash")).Height(18).Padding(0, 5).Gap(4).Radius(5).AlignItems(ui.Center).Background(pal.chip).
						Label(locales.HistoryCopyHash()).Tooltip(locales.HistoryCopyHash())
					hash.Children(func() {
						ui.Text(c, shortHash(info.Hash)).Font(a.termFamily()).FontSize(11).TextColor(pal.textMuted).SingleLine()
						ui.Icon(c, copyIcon).Size(10, 10).TextColor(pal.textFaint)
					})
					if hash.Clicked() {
						a.copyText = info.Hash
					}
				})
			}
		})
	})
}

// histFile is an open commit's file's header, which a click folds or
// unfolds.
func (a *App) histFile(c *ui.Context, pal *palette, pane *proto.Pane, r histRow, gw float32, filtered bool) {
	hs := &a.side.git.hist
	colors := a.laneColors(c, pal)
	ui.Row(c.Key("file")).AlignItems(ui.Stretch).Padding(0, 0, 0, 6).Children(func() {
		if !filtered && r.commit >= 0 {
			gr := hs.graph[r.commit]
			graphBox(c, gw, func(p *ui.Painter, rr ui.Rect) { drawLanes(p, rr, gr, colors) })
		}
		ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			f := r.f
			tools := func() {
				iconButton(c, "file-history", historyIcon, locales.HistoryOfFile(), pal, func() {
					hs.path, hs.query, hs.asked = f.Path, "", ""
				})
			}
			a.fileHeader(c, pal, histKey(r.hash, f.Path), pane.Git.Root, f, nil, tools)
		})
	})
}

// histDiff is an open commit's file's diff, right of the graph, as its
// header is, so that the lanes going on past the commit run unbroken by
// it.
func (a *App) histDiff(c *ui.Context, pal *palette, s *proto.Session, r histRow, sf *sideFile, gw float32, filtered bool, width float32) {
	hs := &a.side.git.hist
	colors := a.laneColors(c, pal)
	ui.Row(c.Key("diff")).AlignItems(ui.Stretch).Padding(0, 0, 0, 6).Children(func() {
		if !filtered && r.commit >= 0 {
			gr := hs.graph[r.commit]
			graphBox(c, gw, func(p *ui.Painter, rr ui.Rect) { drawLanes(p, rr, gr, colors) })
		}
		ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			a.fileDiff(c, pal, s, sf, width-gw-6)
		})
	})
}

// stashesRow is the stash's title, which a click opens or closes.
func (a *App) stashesRow(c *ui.Context, pal *palette) {
	hs := &a.side.git.hist
	row := ui.Row(c.Key("stashes")).Height(28).Padding(0, 10, 0, 12).Gap(7).AlignItems(ui.Center)
	if row.Hovered() {
		row.Background(pal.rowHover.Alpha(0.5))
	}
	if row.Clicked() {
		hs.stashOn = !hs.stashOn
	}
	row.Children(func() {
		chev := ui.Icon(c, chevronIcon).Size(10, 10).TextColor(pal.textFaint).Shrink(0)
		chev.Rotate(chev.AnimateTo("turn", 90*b2f(hs.stashOn), ui.Snappy))
		ui.Icon(c, archiveIcon).Size(13, 13).TextColor(pal.textMuted).Shrink(0)
		ui.Text(c, locales.HistoryStashes(len(hs.stashes))).FontSize(12.5).FontWeight(600).TextColor(pal.textMuted).SingleLine()
	})
}

// stashRow is an entry of the stash, with Apply, Pop and Drop; a click
// opens it as a commit's.
func (a *App) stashRow(c *ui.Context, pal *palette, pane *proto.Pane, st proto.GitStash) {
	id, root := pane.ID, pane.Git.Root
	row := ui.Row(c.Key("stash")).Height(28).Padding(0, 6, 0, 30).Gap(6).AlignItems(ui.Center)
	if a.side.git.hist.opened[st.Hash] != nil {
		row.Background(pal.rowHover.Alpha(0.6))
	} else if row.Hovered() {
		row.Background(pal.rowHover.Alpha(0.5))
	}
	if row.Clicked() {
		a.toggleCommit(id, st.Hash)
	}
	p := proto.GitOpParams{Index: st.Index, Hash: st.Hash}
	row.Children(func() {
		ui.Text(c, st.Message).FontSize(12.5).TextColor(pal.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0).Tooltip(st.Message)
		ui.Text(c, ago(st.Time*1000, time.Now())).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Shrink(0)
		iconButton(c, "apply", checkIcon, locales.HistoryStashApply(), pal, func() {
			p.Op = "stashApply"
			a.guarded(id, root, locales.HistoryStashApply(), func() { a.runGit(id, root, p, locales.HistoryStashApply(), nil) })
		})
		iconButton(c, "pop", pullIcon, locales.HistoryStashPop(), pal, func() {
			p.Op = "stashPop"
			a.guarded(id, root, locales.HistoryStashPop(), func() { a.runGit(id, root, p, locales.HistoryStashApply(), nil) })
		})
		iconButton(c, "drop", closeIcon, locales.HistoryStashDrop(), pal, func() {
			p.Op = "stashDrop"
			a.confirm = &confirmation{open: true, title: locales.HistoryDropTitle(), detail: locales.HistoryDropDetail(st.Message),
				action: locales.HistoryStashDrop(), destroy: true, run: func() { a.runGit(id, root, p, locales.HistoryStashDrop(), nil) }}
		})
	})
}

// uncommittedRow stands for the changes not committed, atop the graph:
// a click shows them in the Changes view.
func (a *App) uncommittedRow(c *ui.Context, pal *palette, g *proto.GitInfo, gw float32) {
	row := ui.Row(c.Key("uncommitted")).Height(26).Padding(0, 10, 0, 6).Gap(7).AlignItems(ui.Center).Radius(6).
		Tooltip(locales.HistoryShowUncommitted())
	if row.Hovered() {
		row.Background(pal.rowHover.Alpha(0.5))
	}
	if row.Clicked() {
		a.defer_(func() {
			a.side.changes.base = "head"
			a.showSide(sideChanges)
		})
	}
	row.Children(func() {
		if gw > 0 {
			graphBox(c, gw, func(p *ui.Painter, r ui.Rect) {
				x, mid := laneX(r, 0), r.Y+r.H/2
				var path ui.Path
				path.MoveTo(x, mid).LineTo(x, r.Y+r.H)
				p.StrokePath(&path, 1.5, pal.textFaint.Alpha(0.6))
				var dot ui.Path
				dot.Circle(x, mid, 3.5)
				p.FillPath(&dot, pal.card)
				p.StrokePath(&dot, 1.5, pal.textFaint)
			})
		}
		ui.Text(c, locales.HistoryUncommitted()).FontSize(13).TextColor(pal.textMuted).SingleLine().Shrink(0)
		ui.Text(c, locales.ChangesFiles(g.Files)).FontSize(12).TextColor(pal.textFaint).SingleLine().Ellipsis("…").MinWidth(0)
		ui.Spacer(c)
		a.lineCounts(c, pal, g.Added, g.Removed, 11.5)
	})
}

// commitMenu is a commit's menu.
func (a *App) commitMenu(c *ui.Context, m *ui.Menu, s *proto.Session, pane *proto.Pane, cm proto.GitCommit) {
	id, root := pane.ID, pane.Git.Root
	short := shortHash(cm.Hash)
	if padItem(m, locales.HistoryCopyHash()).Chosen() {
		c.WriteClipboard(cm.Hash)
	}
	// Its branches' names, the one it has, or each of those it has.
	switch names := branchRefs(parseRefs(cm.Refs, a.side.git.hist.remotes)); len(names) {
	case 0:
	case 1:
		if padItem(m, locales.HistoryCopyBranch()).Chosen() {
			c.WriteClipboard(names[0])
		}
	default:
		padSub(m, locales.HistoryCopyBranch(), func(sm *ui.Menu) {
			for _, name := range names {
				if padItem(sm, name).Chosen() {
					c.WriteClipboard(name)
				}
			}
		})
	}
	m.Separator()
	// A commit a branch is at checks the branch out, a remote's as the
	// local branch of its name; one with none, the commit, detached.
	refs := parseRefs(cm.Refs, a.side.git.hist.remotes)
	var targets []proto.GitBranch
	local := map[string]bool{}
	for _, r := range refs {
		if !r.tag && !r.remote && r.name != "HEAD" {
			local[r.name] = true
			targets = append(targets, proto.GitBranch{Name: r.name, Current: r.name == pane.Git.Branch})
		}
	}
	for _, r := range refs {
		if _, name, ok := strings.Cut(r.name, "/"); r.remote && ok && !local[name] {
			targets = append(targets, proto.GitBranch{Name: r.name, Remote: true})
		}
	}
	switch len(targets) {
	case 0:
		if padItem(m, locales.HistoryCheckout()).Chosen() {
			a.guarded(id, root, locales.HistoryCheckout(), func() {
				a.runGit(id, root, proto.GitOpParams{Op: "checkout", Rev: cm.Hash}, locales.GitSwitchTo(short), nil)
			})
		}
	case 1:
		if b := targets[0]; padItem(m, locales.HistoryCheckoutBranch(b.Name)).Disabled(b.Current).Chosen() {
			a.switchBranch(s, pane, b, cm.Hash)
		}
	default:
		padSub(m, locales.HistoryCheckout(), func(sm *ui.Menu) {
			for _, b := range targets {
				if padItem(sm, b.Name).Disabled(b.Current).Chosen() {
					a.switchBranch(s, pane, b, cm.Hash)
				}
			}
		})
	}
	if padItem(m, locales.GitNewBranchHere()).Chosen() {
		a.askNewBranch(id, root, cm.Hash, short)
	}
	if padItem(m, locales.GitSessionHere()).Chosen() {
		h := a.hostOf(id)
		a.askName(locales.GitSessionHereTitle(), locales.GitNewBranchAt(short), locales.GitBranchName(), "", locales.WorktreeCreate(), func(name string) {
			a.create(h, proto.CreateParams{Project: s.Project, After: s.ID, Dir: root, Cols: s.Cols, Rows: s.Rows, Worktree: name, Base: cm.Hash}, s.Project)
		})
	}
	m.Separator()
	// A commit's branch, merged or rebased onto, as itself, the one it is
	// at: else the commit, by its hash.
	rev, name := cm.Hash, short
	if names := branchRefs(parseRefs(cm.Refs, a.side.git.hist.remotes)); len(names) > 0 {
		rev, name = names[0], names[0]
	}
	a.mergeItems(m, pane, rev, name, cm.Hash == pane.Git.Head)
	m.Separator()
	if padItem(m, locales.HistoryCherryPick()).Chosen() {
		a.guarded(id, root, locales.HistoryCherryPick(), func() {
			a.runGit(id, root, proto.GitOpParams{Op: "cherryPick", Rev: cm.Hash}, locales.HistoryCherryPickOf(short), nil)
		})
	}
	if padItem(m, locales.HistoryRevert()).Chosen() {
		a.guarded(id, root, locales.HistoryRevert(), func() {
			a.runGit(id, root, proto.GitOpParams{Op: "revert", Rev: cm.Hash}, locales.HistoryRevertOf(short), nil)
		})
	}
	padSub(m, locales.HistoryReset(), func(sm *ui.Menu) {
		for _, r := range []struct{ mode, label, detail string }{
			{"soft", locales.HistoryResetSoft(), locales.HistoryResetSoftDetail()},
			{"mixed", locales.HistoryResetMixed(), locales.HistoryResetMixedDetail()},
			{"hard", locales.HistoryResetHard(), locales.HistoryResetHardDetail()},
		} {
			if padItem(sm, r.label).Chosen() {
				mode := r.mode
				a.confirm = &confirmation{open: true, title: locales.HistoryResetTitle(branchName(pane.Git), short), detail: r.detail,
					action: r.label, destroy: mode == "hard", run: func() {
						a.guarded(id, root, locales.HistoryReset(), func() {
							a.runGit(id, root, proto.GitOpParams{Op: "reset", Rev: cm.Hash, Mode: mode}, locales.HistoryResetTo(short), nil)
						})
					}}
			}
		}
	})
}

// showHistory shows a file's history, as the Files view's menu asks.
func (a *App) showHistory(relPath string) {
	hs := &a.side.git.hist
	hs.path, hs.query, hs.asked = relPath, "", ""
	a.showSide(sideHistory)
}
