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

// The graph's lanes: their width, and the narrowest they get when there
// are too many for graphShare of the view.
const (
	laneW      = 12
	minLaneW   = 7
	graphShare = 0.35
)

// historyState is the History view's.
type historyState struct {
	// laneStep is the distance between the graph's lanes (graphWidth).
	laneStep float32
	// worktrees are the branches checked out in other worktrees, by
	// name, at their worktrees' paths.
	worktrees map[string]string
	// chipMax is the widest a commit's refs get: a share of the view, so
	// that a wide view shows whole names (commitRow).
	chipMax float32
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
	rowCommitDetail
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
		lanes := 0
		for _, r := range hs.graph {
			lanes = max(lanes, r.Width)
		}
		gw, hs.laneStep = graphWidth(lanes, width)
	}
	// What the graph, the subject's least, the time and the gaps leave.
	hs.chipMax = max(130, min(width*0.75, width-gw-140))
	// The list leaves room at its right, past the rows' times, for the
	// scroll bar.
	ls := &hs.list
	ls.Key = func(i int) any { r := rows[i]; return [3]any{r.kind, r.hash, r.f.Path} }
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
		case rowCommitDetail:
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
	rows = append(rows, histRow{kind: rowCommitDetail, commit: commit, hash: hash})
	if cv.info == nil {
		return rows
	}
	for _, f := range cv.info.Files {
		rows = append(rows, histRow{kind: rowFile, commit: commit, hash: hash, f: f})
		if !a.fileCollapsed(histKey(hash, f.Path)) {
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
				hs.remotes, hs.worktrees = res.Remotes, res.Worktrees
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

// graphWidth returns the width of a graph of lanes in a view width wide,
// and the step between its lanes: each lane has a line of its own, closer
// together when they would take more than graphShare of the view.
func graphWidth(lanes int, width float32) (gw, step float32) {
	if lanes == 0 {
		return 0, laneW
	}
	step = laneW
	if n := float32(lanes); n*laneW > max(6*laneW, width*graphShare) {
		step = max(minLaneW, width*graphShare/n)
	}
	return float32(lanes) * step, step
}

// laneX is where a lane's line runs in a graph's box, lanes step apart.
func laneX(r ui.Rect, lane int, step float32) float32 {
	return r.X + step/2 + float32(lane)*step
}

// drawGraph draws a commit's row of the graph; node is how its dot shows:
// filled for a commit not pushed, a ring for one pushed, HEAD's ringed
// besides.
func drawGraph(p *ui.Painter, r ui.Rect, step float32, row gitgraph.Row, colors []ui.Color, bg ui.Color, unpushed, head bool) {
	top, mid, bot := r.Y, r.Y+r.H/2, r.Y+r.H
	col := func(i int) ui.Color { return colors[i%len(colors)] }
	for _, l := range row.Pass {
		var path ui.Path
		path.MoveTo(laneX(r, l.From, step), top).LineTo(laneX(r, l.To, step), bot)
		p.StrokePath(&path, 1.5, col(l.Color))
	}
	nx := laneX(r, row.Node, step)
	for _, l := range row.In {
		var path ui.Path
		x := laneX(r, l.From, step)
		path.MoveTo(x, top).CubeTo(x, (top+mid)/2, nx, (top+mid)/2, nx, mid)
		p.StrokePath(&path, 1.5, col(l.Color))
	}
	for _, l := range row.Out {
		var path ui.Path
		x := laneX(r, l.To, step)
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
func drawLanes(p *ui.Painter, r ui.Rect, step float32, row gitgraph.Row, colors []ui.Color) {
	for _, ls := range [][]gitgraph.Line{row.Pass, row.Out} {
		for _, l := range ls {
			var path ui.Path
			x := laneX(r, l.To, step)
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

// commitRowH is the height of a commit's row, and of its first line open.
const commitRowH = 26

// commitRow is a commit's row: its graph, subject, refs and time, which a
// click opens or closes.
func (a *App) commitRow(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, i int, gw float32, filtered bool) {
	hs := &a.side.git.hist
	cm := hs.commits[i]
	open := hs.opened[cm.Hash] != nil
	row := ui.Row(c.Key("row")).Padding(0, 10, 0, 6).Gap(7).Radius(6)
	if open {
		// Open, the subject wraps, the rest of the row on its first line.
		row.AlignItems(ui.Start)
	} else {
		row.Height(commitRowH).AlignItems(ui.Center)
	}
	if open {
		row.Background(pal.rowHover.Alpha(0.6))
	} else if row.Hovered() {
		row.Background(pal.rowHover.Alpha(0.5))
	}
	when := time.Unix(cm.Time, 0)
	wideTooltip(c, row, cm.Author+"\n"+cm.Email+"\n"+when.Format("2006-01-02 15:04")+"\n"+shortHash(cm.Hash))
	if row.Clicked() {
		a.toggleCommit(pane.ID, cm.Hash)
	}
	row.ContextMenu(func(m *ui.Menu) { a.commitMenu(c, m, s, pane, cm) })
	refs := parseRefs(cm.Refs, hs.remotes)
	for i, r := range refs {
		if !r.remote && !r.tag {
			refs[i].worktree = hs.worktrees[r.name]
		}
	}
	head := slices.ContainsFunc(refs, func(r gitRef) bool { return r.head })
	colors := a.laneColors(c, pal)
	row.Children(func() {
		if filtered {
			ui.Row(c).Height(commitRowH).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(pal.textFaint).Margin(0, 2, 0, 4)
			})
		} else {
			gr := hs.graph[i]
			graphBox(c, gw, func(p *ui.Painter, r ui.Rect) {
				// The commit's dot by the first line; below it, as the
				// subject wraps, its lanes go on as the detail's.
				top := r
				top.H = min(r.H, commitRowH)
				drawGraph(p, top, hs.laneStep, gr, colors, pal.card, hs.unpushed[cm.Hash], head)
				if rest := r.H - top.H; rest > 0 {
					drawLanes(p, ui.Rect{X: r.X, Y: r.Y + top.H, W: r.W, H: rest}, hs.laneStep, gr, colors)
				}
			})
		}
		fg := pal.text
		if !hs.unpushed[cm.Hash] && !filtered {
			fg = pal.text.Alpha(0.85)
		}
		subject := ui.Text(c, cm.Subject).FontSize(13).TextColor(fg).Grow(1).MinWidth(0)
		if open {
			// Its first line centered in the row's height, as when closed.
			subject.LineHeight(1.3).Padding(4.5, 0, 4.5, 0).Selectable()
			// Selectable, the subject takes the row's clicks: one that
			// selects nothing closes the commit, as the row's would.
			if subject.Clicked() {
				if start, end := subject.TextSelection(); start == end {
					a.toggleCommit(pane.ID, cm.Hash)
				}
			}
		} else {
			// The refs are what one looks for in the graph: the subject
			// gives way to them first, down to a few words (below its
			// ellipsis, it drew it over them).
			subject.SingleLine().Ellipsis("…").MinWidth(60)
		}
		ui.Row(c).Height(commitRowH).Gap(7).AlignItems(ui.Center).Shrink(0).MaxWidth(hs.chipMax).Children(func() {
			// Clipped, so that refs too many to fit are cut, not drawn
			// over the subject.
			ui.Row(c).Gap(7).AlignItems(ui.Center).MinWidth(0).Shrink(1).ClipX().Children(func() {
				// Colored as the commit's lane, to match a branch to its line.
				var lane *ui.Color
				if !filtered {
					lc := colors[hs.graph[i].Color%len(colors)]
					lane = &lc
				}
				a.refChips(c, pal, pane, refs, lane, cm.Hash)
			})
			ui.Text(c, ago(cm.Time*1000, time.Now())).FontSize(11.5).FontFeatures("tnum").TextColor(pal.textFaint).SingleLine().Shrink(0)
		})
	})
}

// gitRef is a ref a commit's row shows: a local branch, a remote's, or a
// tag.
type gitRef struct {
	name   string
	remote bool
	tag    bool
	head   bool // HEAD is on it, or at it, detached
	// worktree is the path of the other worktree a local branch is
	// checked out in; stash the stash a chip stands for, made on the
	// commit.
	worktree string
	stash    *proto.GitStash
	// tracked are the remotes' branches of a local branch's name at the
	// same commit, which its chip shows with it (chipRefs).
	tracked []string
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

// chipRefs are the refs a commit's row shows: a remote's branch of a
// local branch's name, at the same commit, goes into the local branch's
// chip, as its tracked, since the two names say the same.
func chipRefs(refs []gitRef) []gitRef {
	out := make([]gitRef, 0, len(refs))
	local := map[string]int{}
	for _, r := range refs {
		if !r.tag && !r.remote && r.name != "HEAD" {
			local[r.name] = len(out)
		}
		out = append(out, r)
	}
	merged := make([]bool, len(out))
	for j, r := range out {
		if r.remote {
			_, branch, _ := strings.Cut(r.name, "/")
			if i, ok := local[branch]; ok {
				out[i].tracked = append(out[i].tracked, r.name)
				merged[j] = true
			}
		}
	}
	n := 0
	for j, r := range out {
		if !merged[j] {
			out[n] = r
			n++
		}
	}
	return out[:n]
}

// refChips shows a commit's refs, three at most, then how many more: a
// local branch filled, with the branch's icon, and a remote's outlined,
// with a globe; a local branch with its remote's at the same commit shows
// both icons in its chip. With a lane's color, the chips take it. The
// branch checked out has a check, bold text and a border of its color,
// not a color of its own, which would read as another lane's.
func (a *App) refChips(c *ui.Context, pal *palette, pane *proto.Pane, refs []gitRef, lane *ui.Color, hash string) {
	const shown = 3
	refs = chipRefs(refs)
	// The stashes made on the commit, after its refs.
	for i := range a.side.git.hist.stashes {
		if st := &a.side.git.hist.stashes[i]; st.Base == hash {
			refs = append(refs, gitRef{name: "stash@{" + itoa(st.Index) + "}", stash: st})
		}
	}
	for i, r := range refs {
		if i == shown {
			more := make([]string, 0, len(refs)-shown)
			for _, o := range refs[shown:] {
				more = append(more, o.name)
				more = append(more, o.tracked...)
			}
			ui.Text(c, "+"+itoa(len(refs)-shown)).FontSize(11).TextColor(pal.textFaint).SingleLine().Shrink(0).Tooltip(strings.Join(more, "\n"))
			return
		}
		// One style for every kind, its icon telling them apart: a
		// branch's, a remote's globe, a worktree's folder, a tag's, a
		// stash's box.
		bg, fg, icon := pal.chip, pal.textMuted, branchIcon
		switch {
		case r.stash != nil:
			icon = archiveIcon
		case r.tag:
			icon = tagIcon
		case r.remote:
			icon = globeIcon
		case r.worktree != "":
			icon = folderIcon
		case r.name == "HEAD":
			icon = nil
		}
		switch {
		case r.stash != nil: // not a lane's
		case lane != nil:
			bg, fg = lane.Alpha(0.14), *lane
		case r.head:
			bg, fg = pal.accent.Alpha(0.16), pal.accent
		}
		weight := 550
		if r.head {
			weight = 750
		}
		chip := ui.Row(c.Key("ref-"+r.name)).Height(18).Padding(0, 6, 0, 5).Gap(3).Radius(5).AlignItems(ui.Center).Background(bg).Shrink(1).MinWidth(0)
		if r.head {
			chip.Border(1, fg)
		}
		tip := strings.Join(append([]string{r.name}, r.tracked...), "\n")
		switch {
		case r.stash != nil:
			tip = r.stash.Message
			st := *r.stash
			chip.ContextMenu(func(m *ui.Menu) { a.stashMenu(m, pane, st) })
		case r.worktree != "":
			tip += "\n" + r.worktree
		}
		chip.Tooltip(tip).Children(func() {
			if r.head {
				ui.Icon(c, checkIcon).Size(11, 11).TextColor(fg).Shrink(0)
			}
			if icon != nil {
				ui.Icon(c, icon).Size(11, 11).TextColor(fg).Shrink(0)
			}
			if len(r.tracked) > 0 {
				ui.Icon(c, globeIcon).Size(11, 11).TextColor(fg).Shrink(0)
			}
			ui.Text(c, r.name).FontSize(11).FontWeight(weight).TextColor(fg).SingleLine().Ellipsis("…").MinWidth(0)
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
			if a.fileCollapsed(histKey(hash, f.Path)) {
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
			graphBox(c, gw, func(p *ui.Painter, rr ui.Rect) { drawLanes(p, rr, hs.laneStep, gr, colors) })
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
			graphBox(c, gw, func(p *ui.Painter, rr ui.Rect) { drawLanes(p, rr, hs.laneStep, gr, colors) })
		}
		ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			f := r.f
			menu := func(m *ui.Menu) {
				if m.Item(locales.HistoryOfFile()).Chosen() {
					hs.path, hs.query, hs.asked = f.Path, "", ""
				}
			}
			a.fileHeader(c, pal, histKey(r.hash, f.Path), pane.ID, pane.Git.Root, f, flatList, stickyH, nil, nil, menu)
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
			graphBox(c, gw, func(p *ui.Painter, rr ui.Rect) { drawLanes(p, rr, hs.laneStep, gr, colors) })
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
		chev.Rotate(chev.AnimateTo("turn", 90*b2f(hs.stashOn), chevronTurn))
		ui.Icon(c, archiveIcon).Size(13, 13).TextColor(pal.textMuted).Shrink(0)
		ui.Text(c, locales.HistoryStashes(len(hs.stashes))).FontSize(12.5).FontWeight(600).TextColor(pal.textMuted).SingleLine()
	})
}

// stashRow is an entry of the stash, with Apply, Pop and Drop; a click
// opens it as a commit's.
func (a *App) stashRow(c *ui.Context, pal *palette, pane *proto.Pane, st proto.GitStash) {
	id := pane.ID
	row := ui.Row(c.Key("stash")).Height(28).Padding(0, 6, 0, 30).Gap(6).AlignItems(ui.Center)
	if a.side.git.hist.opened[st.Hash] != nil {
		row.Background(pal.rowHover.Alpha(0.6))
	} else if row.Hovered() {
		row.Background(pal.rowHover.Alpha(0.5))
	}
	if row.Clicked() {
		a.toggleCommit(id, st.Hash)
	}
	apply, pop, drop := a.stashOps(pane, st)
	row.ContextMenu(func(m *ui.Menu) { a.stashMenu(m, pane, st) })
	row.Children(func() {
		ui.Text(c, st.Message).FontSize(12.5).TextColor(pal.text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0).Tooltip(st.Message)
		ui.Text(c, ago(st.Time*1000, time.Now())).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Shrink(0)
		iconButton(c, "apply", checkIcon, locales.HistoryStashApply(), pal, apply)
		iconButton(c, "pop", pullIcon, locales.HistoryStashPop(), pal, pop)
		iconButton(c, "drop", closeIcon, locales.HistoryStashDrop(), pal, drop)
	})
}

// stashOps are what a stash's row and its chip on the graph do with it:
// apply it, pop it, and drop it once the user confirms.
func (a *App) stashOps(pane *proto.Pane, st proto.GitStash) (apply, pop, drop func()) {
	id, root := pane.ID, pane.Git.Root
	p := proto.GitOpParams{Index: st.Index, Hash: st.Hash}
	apply = func() {
		p.Op = "stashApply"
		a.guarded(id, root, locales.HistoryStashApply(), func() { a.runGit(id, root, p, locales.HistoryStashApply(), nil) })
	}
	pop = func() {
		p.Op = "stashPop"
		a.guarded(id, root, locales.HistoryStashPop(), func() { a.runGit(id, root, p, locales.HistoryStashApply(), nil) })
	}
	drop = func() {
		p.Op = "stashDrop"
		a.confirm = &confirmation{open: true, title: locales.HistoryDropTitle(), detail: locales.HistoryDropDetail(st.Message),
			action: locales.HistoryStashDrop(), destroy: true, run: func() { a.runGit(id, root, p, locales.HistoryStashDrop(), nil) }}
	}
	return apply, pop, drop
}

// stashMenu is a stash's menu, of its row and its chip.
func (a *App) stashMenu(m *ui.Menu, pane *proto.Pane, st proto.GitStash) {
	apply, pop, drop := a.stashOps(pane, st)
	if m.Item(locales.HistoryStashApply()).Chosen() {
		apply()
	}
	if m.Item(locales.HistoryStashPop()).Chosen() {
		pop()
	}
	m.Separator()
	if m.Item(locales.HistoryStashDrop()).Chosen() {
		drop()
	}
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
				x, mid := laneX(r, 0, a.side.git.hist.laneStep), r.Y+r.H/2
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
	if m.Item(locales.HistoryCopyHash()).Chosen() {
		c.WriteClipboard(cm.Hash)
	}
	// Its branches' names, the one it has, or each of those it has.
	switch names := branchRefs(parseRefs(cm.Refs, a.side.git.hist.remotes)); len(names) {
	case 0:
	case 1:
		if m.Item(locales.HistoryCopyBranch()).Chosen() {
			c.WriteClipboard(names[0])
		}
	default:
		m.Submenu(locales.HistoryCopyBranch(), func(sm *ui.Menu) {
			for _, name := range names {
				if sm.Item(name).Chosen() {
					c.WriteClipboard(name)
				}
			}
		})
	}
	m.Separator()
	// A commit a branch is at checks the branch out, a remote's as the
	// local branch of its name; one with none, the commit, detached.
	// A branch checked out in another worktree, which git won't check
	// out here, opens that worktree's session instead.
	refs := parseRefs(cm.Refs, a.side.git.hist.remotes)
	var targets []proto.GitBranch
	var worktrees []string
	local := map[string]bool{}
	for _, r := range refs {
		if !r.tag && !r.remote && r.name != "HEAD" {
			local[r.name] = true
			if a.side.git.hist.worktrees[r.name] != "" {
				worktrees = append(worktrees, r.name)
				continue
			}
			targets = append(targets, proto.GitBranch{Name: r.name, Current: r.name == pane.Git.Branch})
		}
	}
	for _, r := range refs {
		if _, name, ok := strings.Cut(r.name, "/"); r.remote && ok && !local[name] {
			targets = append(targets, proto.GitBranch{Name: r.name, Remote: true})
		}
	}
	for _, name := range worktrees {
		path := a.side.git.hist.worktrees[name]
		if m.Item(locales.HistoryOpenWorktree(name)).Chosen() {
			a.openWorktree(id, path)
		}
		if m.Item(locales.HistoryCopyWorktreePath(name)).Chosen() {
			c.WriteClipboard(path)
		}
	}
	switch len(targets) {
	case 0:
		if len(worktrees) > 0 {
			break
		}
		if m.Item(locales.HistoryCheckout()).Chosen() {
			a.guarded(id, root, locales.HistoryCheckout(), func() {
				a.runGit(id, root, proto.GitOpParams{Op: "checkout", Rev: cm.Hash}, locales.GitSwitchTo(short), nil)
			})
		}
	case 1:
		if b := targets[0]; m.Item(locales.HistoryCheckoutBranch(b.Name)).Disabled(b.Current).Chosen() {
			a.switchBranch(s, pane, b, cm.Hash)
		}
	default:
		m.Submenu(locales.HistoryCheckout(), func(sm *ui.Menu) {
			for _, b := range targets {
				if sm.Item(b.Name).Disabled(b.Current).Chosen() {
					a.switchBranch(s, pane, b, cm.Hash)
				}
			}
		})
	}
	if m.Item(locales.GitNewBranchHere()).Chosen() {
		a.askNewBranch(id, root, cm.Hash, short)
	}
	if m.Item(locales.GitSessionHere()).Chosen() {
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
	if m.Item(locales.HistoryCherryPick()).Chosen() {
		a.guarded(id, root, locales.HistoryCherryPick(), func() {
			a.runGit(id, root, proto.GitOpParams{Op: "cherryPick", Rev: cm.Hash}, locales.HistoryCherryPickOf(short), nil)
		})
	}
	if m.Item(locales.HistoryRevert()).Chosen() {
		a.guarded(id, root, locales.HistoryRevert(), func() {
			a.runGit(id, root, proto.GitOpParams{Op: "revert", Rev: cm.Hash}, locales.HistoryRevertOf(short), nil)
		})
	}
	m.Submenu(locales.HistoryReset(), func(sm *ui.Menu) {
		for _, r := range []struct{ mode, label, detail string }{
			{"soft", locales.HistoryResetSoft(), locales.HistoryResetSoftDetail()},
			{"mixed", locales.HistoryResetMixed(), locales.HistoryResetMixedDetail()},
			{"hard", locales.HistoryResetHard(), locales.HistoryResetHardDetail()},
		} {
			if sm.Item(r.label).Chosen() {
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

// openWorktree shows the session working in the worktree at path, on the
// host of the pane, or opens one there.
func (a *App) openWorktree(pane, path string) {
	h := a.hostOf(pane)
	for _, s := range a.st.Sessions {
		if a.hostOf(s.ID) != h {
			continue
		}
		for _, p := range s.Panes {
			if p.Git != nil && p.Git.Root == path {
				a.selectSession(s.ID)
				return
			}
		}
	}
	a.newSessionIn(h, path)
}
