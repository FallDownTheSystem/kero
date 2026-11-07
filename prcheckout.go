package main

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"
	"kero/locales"
)

// Checking a pull request out (pr.checkout): in the pane's worktree, or in
// a new session in a worktree of its own, from the pull requests' list
// and a pull request's page.

// checkoutItems are the items of a menu that checks a pull request out:
// in the pane's worktree, or in a new one of its own.
func checkoutItems(m *ui.Menu, checkout func(here bool)) {
	if m.Item(locales.PrsCheckoutHere()).Chosen() {
		checkout(true)
	}
	if m.Item(locales.PrsCheckoutWorktree()).Chosen() {
		checkout(false)
	}
}

// checkoutPR checks a pull request out on its branch: in the pane's
// worktree when here, which kerod refuses when it has changes, else in a
// new session in a worktree of its own. In a worktree that has it
// already, the user goes there instead: to the session working in it, or
// a new one.
func (a *App) checkoutPR(s *proto.Session, pane *proto.Pane, remote string, pr proto.PullRequest, here bool) {
	if s == nil || pane == nil || pane.Git == nil {
		return
	}
	h := a.hostOf(pane.ID)
	cl := h.cl
	if cl == nil {
		return
	}
	p := proto.PRCheckoutParams{Pane: pane.ID, Remote: remote, Number: pr.Number, Head: pr.Head, CrossRepo: pr.CrossRepo, Here: here}
	ns := proto.CreateParams{Project: s.Project, After: s.ID, Dir: pane.Git.Root, Cols: s.Cols, Rows: s.Rows, Name: "#" + strconv.Itoa(pr.Number)}
	a.toast(locales.PrsFetching(pr.Number))
	go func() {
		var got proto.PRCheckedOut
		err := cl.Call("pr.checkout", p, &got)
		a.update(func() {
			switch {
			case err != nil:
				a.toast(locales.PrsCheckoutFailed(err.Error()))
			case got.Worktree != "":
				if id := a.worktreeSession(h, got.Worktree); id != "" {
					a.selectSession(id)
				} else {
					ns.Dir = got.Worktree
					a.createThen(h, ns, ns.Project, true, nil)
				}
				a.toast(locales.PrsCheckedOutIn(pr.Number, h.shortDir(got.Worktree)))
			case here:
				a.toast(locales.PrsCheckedOutBranch(pr.Number, got.Branch))
			default:
				ns.Worktree, ns.Base, ns.PushRemote, ns.PushBranch = got.Branch, got.Base, got.PushRemote, got.PushBranch
				a.createThen(h, ns, ns.Project, true, nil)
			}
		})
	}()
}

// worktreeSession is a session of h that works in the worktree at path,
// its own first; "" for none.
func (a *App) worktreeSession(h *host, path string) string {
	in := ""
	for i := range a.st.Sessions {
		s := &a.st.Sessions[i]
		if a.hostOf(s.ID) != h {
			continue
		}
		if s.Worktree != nil && samePath(s.Worktree.Path, path) {
			return s.ID
		}
		for _, p := range s.Panes {
			// By its worktree's root: the main checkout holds the others.
			if in == "" && p.Git != nil && samePath(p.Git.Root, path) {
				in = s.ID
			}
		}
	}
	return in
}

// checkoutButton is Check Out, a menu of where to check the pull request
// out (checkoutItems).
func (a *App) checkoutButton(c *ui.Context, pal *palette, checkout func(here bool)) {
	b := ui.ButtonBase(c.Key("checkout")).Size(28, 26).Radius(6).Center().TextColor(pal.textMuted).
		Label(locales.PrsCheckout()).Tooltip(locales.PrsCheckout())
	if b.Hovered() || b.Pressed() {
		b.Background(pal.rowHover).TextColor(pal.text)
	}
	b.Children(func() { ui.Icon(c, branchIcon).Size(15, 15) })
	b.Menu(func(m *ui.Menu) { checkoutItems(m, checkout) })
}
