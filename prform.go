package main

import (
	"cmp"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"

	"kero/locales"
)

// New Pull Request: a pull request of the branch of the pane the user is
// in, into a branch of one of the repository's remotes on GitHub: its
// title and description, which an agent can write, then the commits and
// the files it brings, each file's diff unfolding. Create pushes what
// isn't pushed, then makes it with the GitHub CLI; Show on GitHub opens
// GitHub's page that makes it, its fields filled in.

// openPRForm shows the form of a pull request of the pane's branch.
func (a *App) openPRForm(pane *proto.Pane) {
	f := &prForm{pane: pane.ID, root: pane.Git.Root, files: map[string]*sideFile{}}
	a.side.pulls.form = f
	a.fetchDraft(f)
}

// fetchDraft asks what the pull request would bring, into the base asked
// for, and the branches of the base's remote.
func (a *App) fetchDraft(f *prForm) {
	cl := a.hostOf(f.pane).cl
	if cl == nil {
		return
	}
	f.gen++
	gen := f.gen
	f.busy, f.err = true, ""
	p := proto.PRDraftParams{Pane: f.pane, Remote: f.remote, Base: f.base}
	go func() {
		var d proto.PRDraft
		err := cl.Call("pr.draft", p, &d)
		var bl proto.GitBranches
		if err == nil {
			cl.Call("git.branches", proto.GitParams{Pane: f.pane}, &bl)
		}
		a.update(func() {
			if f.gen != gen {
				return
			}
			f.busy = false
			if err != nil {
				f.err = err.Error()
				return
			}
			if f.draft == nil || f.draft.MergeBase != d.MergeBase {
				f.files = map[string]*sideFile{}
			}
			f.draft = &d
			f.branches = baseBranches(&bl, &d)
			// One commit's subject is the pull request's title, as GitHub
			// takes it.
			if strings.TrimSpace(f.title) == "" && len(d.Commits) == 1 {
				f.title = d.Commits[0].Subject
			}
		})
	}()
}

func (a *App) prFormView(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, width float32) {
	ps := &a.side.pulls
	f := ps.form
	d := f.draft
	ui.Row(c.Key("form-bar")).Shrink(0).Height(32).Padding(0, 12, 4, 6).Gap(4).AlignItems(ui.Center).Children(func() {
		iconButton(c, "back", chevronLeftIcon, locales.PrsBack(), pal, func() { ps.form = nil })
		ui.Text(c, locales.PrsNew()).FontSize(13.5).FontWeight(650).TextColor(pal.text).SingleLine()
		ui.Spacer(c)
		if f.busy {
			ui.Spinner(c).Size(12, 12).Margin(0, 4, 0, 0)
		}
		iconButton(c, "refresh", reloadIcon, locales.PrsRefresh(), pal, func() { a.fetchDraft(f) })
	})
	switch {
	case f.err != "" && d == nil:
		a.sideMessage(c, pal, locales.PrsCantDraft(), f.err)
		return
	case d == nil:
		ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c) })
		return
	}
	if ps.form == nil {
		return
	}
	ui.Scroll(c.Key("form")).Grow(1).MinHeight(0).TrackScroll(&f.scroll).Children(func() {
		ui.Column(c).Padding(0, 12, 16, 12).Gap(8).AlignItems(ui.Stretch).Children(func() {
			a.prBranches(c, pal, f, d)
			if f.err != "" {
				_, removed, _ := a.diffColors(c, pal)
				ui.Text(c, f.err).FontSize(12).TextColor(removed).LineHeight(1.35).Selectable()
			}
			if e := d.Existing; e != nil {
				a.prExisting(c, pal, e)
			}
			a.prFields(c, pal, pane, f, d)
			a.prCommits(c, pal, d)
		})
		a.prFiles(c, pal, s, pane, f, d, width)
	})
	a.prCreateBar(c, pal, pane, f, d)
}

// baseBranch is a branch of the base's remote, as the base picker lists
// it: its name on the remote, and its commit's time.
type baseBranch struct {
	name string
	time int64
}

// baseBranches are the branches of the draft's remote, as last fetched:
// its default branch first, then the most recent first, as a repository
// of hundreds of branches most often takes one of those.
func baseBranches(bl *proto.GitBranches, d *proto.PRDraft) []baseBranch {
	def := ""
	for _, r := range d.Repos {
		if r.Remote == d.Remote {
			def = r.Default
		}
	}
	var out []baseBranch
	for _, b := range bl.Branches {
		if name, ok := strings.CutPrefix(b.Name, d.Remote+"/"); ok && b.Remote && name != "HEAD" {
			out = append(out, baseBranch{name, b.Time})
		}
	}
	slices.SortStableFunc(out, func(x, y baseBranch) int {
		switch {
		case x.name == def:
			return -1
		case y.name == def:
			return 1
		}
		return cmp.Compare(y.time, x.time)
	})
	return out
}

// basePicker is the base picker's: open, what is typed, the row the
// arrows are on, and where its button was as it opened, as the switcher
// (branchMenu) has them.
type basePicker struct {
	open   bool
	query  string
	hi     int
	reveal bool
	at     ui.Rect
}

// prBranches is the branch the pull request is of, and the base it goes
// into, which the base picker chooses.
func (a *App) prBranches(c *ui.Context, pal *palette, f *prForm, d *proto.PRDraft) {
	ui.Row(c.Key("branches")).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, branchIcon).Size(12, 12).TextColor(pal.textFaint).Shrink(0)
		ui.Text(c, d.Head).FontSize(12.5).FontWeight(600).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		ui.Text(c, "→").FontSize(12.5).TextColor(pal.textFaint).Shrink(0)
		label := d.Remote + "/" + d.Base
		m := &f.picker
		b := ui.ButtonBase(c.Key("base")).Height(24).Padding(0, 8).Gap(5).Radius(6).AlignItems(ui.Center).MinWidth(0).Shrink(1).
			Background(pal.chip).TextColor(pal.text).Label(locales.PrsBaseTip()).Tooltip(locales.PrsBaseTip())
		if b.Hovered() || b.Pressed() || m.open {
			b.Background(pal.rowHover)
		}
		b.Children(func() {
			ui.Text(c, label).FontSize(12.5).FontWeight(600).SingleLine().Ellipsis("…").MinWidth(0)
			ui.Icon(c, chevronDownIcon).Size(9, 9).TextColor(pal.textFaint).Shrink(0)
		})
		if b.Clicked() {
			if m.open {
				m.open = false
			} else {
				*m = basePicker{open: true, reveal: true, at: b.Bounds()}
				for i, br := range f.branches {
					if br.name == d.Base {
						m.hi = i
					}
				}
			}
		}
		a.basePopover(c, pal, f, d, b)
	})
}

// basePopover is the base picker, below its button: the repository's
// remotes on GitHub, when it has more than one, then a field that filters
// the remote's branches, all of them, which the arrows and Return choose
// from.
func (a *App) basePopover(c *ui.Context, pal *palette, f *prForm, d *proto.PRDraft, trig ui.Element) {
	m := &f.picker
	// The form scrolled, or the panel changed: the popover would hang where
	// the button was.
	if b := trig.Bounds(); m.open && (abs(b.X-m.at.X) > 0.5 || abs(b.Y-m.at.Y) > 0.5) {
		m.open = false
	}
	if m.open {
		ui.Overlay(c, func() {
			back := ui.ButtonBase(c.Key("base-back")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Label(locales.CloseTheMenu())
			if back.Clicked() {
				m.open = false
			}
		})
	}
	_, h := c.Size()
	listH := max(menuItemH*4, min(380, h-trig.Bounds().Y-trig.Bounds().H-140))
	ui.PopoverBase(c.Key("base-menu"), trig, &m.open, func(panel ui.Element) {
		menuPanel(panel, pal).Margin(4, 0, 0, 0).Width(300)
		q := strings.ToLower(strings.TrimSpace(m.query))
		var rows []string
		for _, br := range f.branches {
			if q == "" || strings.Contains(strings.ToLower(br.name), q) {
				rows = append(rows, br.name)
			}
		}
		m.hi = min(max(m.hi, 0), max(len(rows)-1, 0))
		pick := func(i int) {
			if i < 0 || i >= len(rows) {
				return
			}
			m.open = false
			if rows[i] != d.Base {
				f.remote, f.base = d.Remote, rows[i]
				a.defer_(func() { a.fetchDraft(f) })
			}
		}
		if len(d.Repos) > 1 {
			ids, labels := make([]string, len(d.Repos)), make([]string, len(d.Repos))
			for i, r := range d.Repos {
				ids[i], labels[i] = r.Remote, r.Remote
			}
			remote := d.Remote
			ui.Row(c.Key("remotes")).Margin(3, 3, 3, 3).Children(func() {
				segmented(c, "remote", &remote, ids, labels)
			})
			if remote != d.Remote {
				m.open = false
				f.remote, f.base = remote, ""
				a.defer_(func() { a.fetchDraft(f) })
			}
		}
		var in ui.Element
		frame := ui.Row(c.Key("search")).Margin(3, 3, menuPad+3, 3).Cursor(ui.CursorText)
		frame.Children(func() {
			ui.Icon(c, searchIcon).Size(12, 12).TextColor(pal.textFaint).Shrink(0)
			in = ui.TextInputBase(c.Key("q"), &m.query).AutoFocus().Grow(1).MinWidth(0).AlignSelf(ui.Stretch).FontSize(13).
				Placeholder(locales.GitBranchFilter()).Label(locales.GitBranchFilter())
		})
		field(c, frame, in.Focused(), pal)
		in.OnChange(func() { m.hi, m.reveal = 0, true })
		in.OnShortcut(0, ui.KeyDown, func() { m.hi, m.reveal = min(m.hi+1, len(rows)-1), true })
		in.OnShortcut(0, ui.KeyUp, func() { m.hi, m.reveal = max(m.hi-1, 0), true })
		in.OnShortcut(0, ui.KeyEscape, func() { m.open = false })
		in.OnSubmit(func() { pick(m.hi) })
		if len(rows) == 0 {
			ui.Text(c, locales.CommonNoResults()).FontSize(13).TextColor(pal.textFaint).Padding(6, 12)
			return
		}
		reveal := m.reveal
		m.reveal = false
		ui.Scroll(c.Key("rows")).MaxHeight(listH).Children(func() {
			for i, name := range rows {
				row := ui.Row(c.Key(name)).Label(name)
				if row.Hovered() && m.hi != i {
					m.hi = i
					c.After(0)
				}
				if row.Clicked() {
					pick(i)
				}
				hi := i == m.hi
				row.Height(menuItemH+2).Padding(0, 10, 0, 6).Gap(6).Radius(7).AlignItems(ui.Center)
				fg := pal.text
				if hi {
					row.Background(pal.accent)
					fg = ui.Hex("#ffffff")
				}
				row.TextColor(fg).Children(func() {
					if name == d.Base {
						ui.Icon(c, checkIcon).Size(12, 12).Shrink(0)
					} else {
						ui.Box(c).Size(12, 12).Shrink(0)
					}
					ui.Text(c, name).FontSize(13).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
				})
				if reveal && hi {
					row.ScrollIntoView()
				}
			}
		})
	})
}

// prExisting says the branch has a pull request open already, which a
// click opens.
func (a *App) prExisting(c *ui.Context, pal *palette, e *proto.PullRequest) {
	row := ui.Row(c.Key("existing")).Padding(8, 10).Gap(8).Radius(8).AlignItems(ui.Center).Background(pal.accent.Alpha(0.1)).Cursor(ui.CursorPointer).Tooltip(e.URL)
	if row.Clicked() {
		openURL(e.URL)
	}
	row.Children(func() {
		ui.Icon(c, prIcon).Size(13, 13).TextColor(pal.accent).Shrink(0)
		ui.Text(c, locales.PrsExisting(e.Number, e.Title)).FontSize(12.5).TextColor(pal.text).Grow(1).MinWidth(0).LineHeight(1.35)
		ui.Icon(c, linkIcon).Size(11, 11).TextColor(pal.textMuted).Shrink(0)
	})
}

// prFields are the pull request's title, with the button that has an
// agent write it and the description, and its description.
func (a *App) prFields(c *ui.Context, pal *palette, pane *proto.Pane, f *prForm, d *proto.PRDraft) {
	frame := ui.Row(c.Key("title")).AlignItems(ui.Center).Cursor(ui.CursorText)
	var in ui.Element
	frame.Children(func() {
		in = ui.TextInputBase(c.Key("in"), &f.title).Grow(1).MinWidth(0).Height(32).Padding(0, 8).FontSize(13.5).FontWeight(600).
			Placeholder(locales.PrsTitle()).Label(locales.PrsTitle())
		a.prWriteButton(c, pal, pane, f, d)
	})
	fieldLook(c, frame, in.Focused(), pal).Padding(0)
	body := ui.Row(c.Key("body")).AlignItems(ui.Start).Cursor(ui.CursorText)
	var area ui.Element
	body.Children(func() {
		area = ui.TextAreaBase(c.Key("in"), &f.body).Grow(1).MinWidth(0).MinHeight(110).MaxHeight(300).Padding(8).FontSize(13).
			Placeholder(locales.PrsBodyPlaceholder()).Label(locales.PrsBody())
	})
	fieldLook(c, body, area.Focused(), pal).Padding(0)
	// As the panel's Escape would take the keyboard from them.
	if in.Focused() || area.Focused() {
		a.side.editing = true
	}
	if f.writeErr != "" {
		_, removed, _ := a.diffColors(c, pal)
		ui.Text(c, f.writeErr).FontSize(12).TextColor(removed).LineHeight(1.35).Selectable()
	}
}

// prWriteButton has an agent write the title and the description, or
// stops it while it writes.
func (a *App) prWriteButton(c *ui.Context, pal *palette, pane *proto.Pane, f *prForm, d *proto.PRDraft) {
	b := ui.ButtonBase(c.Key("write")).Size(26, 26).Margin(0, 3, 0, 0).Radius(6).Center().Shrink(0).TextColor(pal.textMuted)
	if f.writing != "" {
		b.Label(locales.GitStopWriting()).Tooltip(locales.GitStopWriting())
	} else {
		b.Label(locales.PrsWrite()).Tooltip(locales.PrsWriteTip())
	}
	if b.Hovered() || b.Pressed() {
		b.Background(pal.rowHover).TextColor(pal.text)
	}
	b.Children(func() {
		if f.writing != "" {
			ui.Spinner(c).Size(13, 13)
		} else {
			ui.Icon(c, sparklesIcon).Size(14, 14)
		}
	})
	if !b.Clicked() {
		return
	}
	cl := a.hostOf(f.pane).cl
	if cl == nil {
		return
	}
	if f.writing != "" {
		go cl.Call("git.cancel", proto.IDParams{ID: f.writing}, nil)
		f.writing = ""
		return
	}
	id := opID()
	f.writing, f.writeErr = id, ""
	p := proto.PRMessageParams{Pane: f.pane, ID: id, Remote: d.Remote, Base: d.Base, Command: a.set.CommitMessageCommand, Prompt: a.set.PRMessagePrompt}
	go func() {
		var res proto.PRMessage
		err := cl.Call("pr.message", p, &res)
		a.update(func() {
			if f.writing != id {
				return // stopped, or written over by another
			}
			f.writing = ""
			if err != nil {
				if err.Error() != "canceled" {
					f.writeErr = locales.GitWriteFailed(err.Error())
				}
				return
			}
			f.title, f.body = res.Title, res.Body
		})
	}()
}

// prCommits are the commits the pull request brings, the newest first.
func (a *App) prCommits(c *ui.Context, pal *palette, d *proto.PRDraft) {
	ui.Text(c, locales.PrsCommits(len(d.Commits))).FontSize(12).FontWeight(650).TextColor(pal.textMuted).Margin(8, 0, 0, 0)
	if len(d.Commits) == 0 {
		ui.Text(c, locales.PrsNoCommits(d.Remote+"/"+d.Base)).FontSize(12.5).TextColor(pal.textFaint).LineHeight(1.4)
		return
	}
	ui.Column(c.Key("commits")).Gap(2).AlignItems(ui.Stretch).Children(func() {
		for _, cm := range d.Commits {
			ui.Row(c.Key(cm.Hash)).Height(22).Gap(8).AlignItems(ui.Center).Tooltip(cm.Author).Children(func() {
				ui.Text(c, shortHash(cm.Hash)).Font(a.termFamily()).FontSize(11.5).TextColor(pal.textFaint).SingleLine().Shrink(0)
				ui.Text(c, cm.Subject).FontSize(12.5).TextColor(pal.text).SingleLine().Ellipsis("…").MinWidth(0)
			})
		}
	})
}

// prFiles are the files the pull request changes, each of whose headers
// unfolds its diff, which is asked for then.
func (a *App) prFiles(c *ui.Context, pal *palette, s *proto.Session, pane *proto.Pane, f *prForm, d *proto.PRDraft, width float32) {
	if len(d.Files) == 0 {
		return
	}
	added, removed := 0, 0
	for _, fl := range d.Files {
		added, removed = added+fl.Added, removed+fl.Removed
	}
	ui.Row(c.Key("files-title")).Padding(8, 12, 6, 12).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, locales.PrsFiles(len(d.Files))).FontSize(12).FontWeight(650).TextColor(pal.textMuted)
		a.lineCounts(c, pal, added, removed, 12)
	})
	asking := 0
	for _, sf := range f.files {
		if sf.busy {
			asking++
		}
	}
	ui.Column(c.Key("files")).Padding(0, 6, 0, 0).AlignItems(ui.Stretch).Children(func() {
		for _, fl := range d.Files {
			key := "pr|" + d.MergeBase + "|" + fl.Path
			ui.Column(c.Key(key)).AlignItems(ui.Stretch).Padding(0, 0, 2, 0).Children(func() {
				a.fileHeader(c, pal, key, pane.ID, pane.Git.Root, fl, flatList, stickyH, nil, nil, nil)
				if a.fileCollapsed(key) {
					return
				}
				sf := f.files[fl.Path]
				if sf == nil {
					sf = &sideFile{pane: pane.ID, base: baseRev, f: fl}
					f.files[fl.Path] = sf
				}
				if !sf.busy && sf.d == nil && !sf.text.Binary && !sf.text.Large && sf.err == "" && asking < 4 {
					asking++
					a.fetchFile(sf, "")
				}
				a.fileDiff(c, pal, s, sf, width-6)
			})
		}
	})
}

// prCreateBar is what the pull request leaves out or does first, and
// Create Pull Request, whose menu makes it a draft or shows GitHub's page
// that makes it instead.
func (a *App) prCreateBar(c *ui.Context, pal *palette, pane *proto.Pane, f *prForm, d *proto.PRDraft) {
	busy := f.creating || f.busy || a.gitBusy(f.root)
	can := !busy && d.Existing == nil && len(d.Commits) > 0
	canCreate := can && strings.TrimSpace(f.title) != ""
	box := ui.Column(c.Key("create")).Shrink(0).Padding(8, 12, 12, 12).Gap(6).AlignItems(ui.Stretch)
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, pal.text.Alpha(0.08), 0)
	})
	box.Children(func() {
		note := func(text string) {
			ui.Text(c, text).FontSize(12).TextColor(pal.textMuted).LineHeight(1.35)
		}
		if d.Unpushed > 0 {
			note(locales.PrsUnpushed(d.Unpushed, d.PushRemote))
		}
		if d.Uncommitted > 0 {
			note(locales.PrsUncommitted(d.Uncommitted))
		}
		if d.Error != "" {
			note(d.Error)
		}
		ui.Row(c.Key("buttons")).Gap(1).AlignItems(ui.Stretch).Children(func() {
			label := locales.PrsCreate()
			tip := locales.PrsCreateTip()
			switch {
			case d.Existing != nil:
				tip = locales.PrsAlreadyOpen()
			case len(d.Commits) == 0:
				tip = locales.PrsNoCommits(d.Remote + "/" + d.Base)
			case strings.TrimSpace(f.title) == "":
				tip = locales.PrsTitleFirst()
			}
			b := ui.ButtonBase(c.Key("create-btn")).Height(28).Grow(1).Padding(0, 12).Gap(6).Center().Label(label).Tooltip(tip)
			commitFace(b, pal, canCreate, canCreate, 8, 2)
			b.Children(func() {
				if f.creating {
					ui.Spinner(c).Size(12, 12)
				} else {
					ui.Icon(c, prIcon).Size(13, 13)
				}
				ui.Text(c, label).FontSize(13).FontWeight(550).SingleLine().Ellipsis("…").MinWidth(0)
			})
			if b.Clicked() && canCreate {
				a.createPR(pane, f, false, false)
			}
			opts := ui.ButtonBase(c.Key("create-opts")).Width(28).Height(28).Radius(8).Center().Label(locales.PrsCreateOptions()).Tooltip(locales.PrsCreateOptions())
			commitFace(opts, pal, canCreate, can, 2, 8)
			opts.Children(func() { ui.Icon(c, chevronDownIcon).Size(10, 10) })
			opts.Menu(func(m *ui.Menu) {
				if m.Item(locales.PrsCreateDraft()).Disabled(!canCreate).Chosen() {
					a.createPR(pane, f, true, false)
				}
				if m.Item(locales.PrsShow()).Disabled(!can).Chosen() {
					a.createPR(pane, f, false, true)
				}
			})
		})
	})
}

// createPR pushes what the push remote hasn't of the branch, then makes
// the pull request, a draft with draft, and opens it; with show, opens
// GitHub's page that makes it instead.
func (a *App) createPR(pane *proto.Pane, f *prForm, draft, show bool) {
	d := f.draft
	if d == nil {
		return
	}
	make := func() {
		cl := a.hostOf(f.pane).cl
		if cl == nil {
			return
		}
		f.creating, f.err = true, ""
		p := proto.PRCreateParams{Pane: f.pane, Remote: d.Remote, Base: d.Base, Title: f.title, Body: f.body, Draft: draft, Show: show}
		go func() {
			var res proto.PRCreated
			err := cl.Call("pr.create", p, &res)
			a.update(func() {
				f.creating = false
				if err != nil {
					f.err = locales.PrsCreateFailed(err.Error())
					return
				}
				openURL(res.URL)
				if show {
					return
				}
				a.toast(locales.PrsCreated())
				if a.side.pulls.form == f {
					a.side.pulls.form = nil
				}
				a.side.pulls.fetched = a.side.pulls.fetched.Add(-pullsRefresh)
			})
		}()
	}
	if d.Unpushed == 0 {
		make()
		return
	}
	a.runGit(f.pane, f.root, proto.GitOpParams{Op: "push"}, locales.GitPush(), func(string) {
		d.Unpushed = 0
		make()
	})
}
