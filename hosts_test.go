package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"kero/internal/proto"
	"kero/internal/remote"
	"kero/internal/settings"
)

// withHost adds a remote host to a test app, connected, with a session of
// its own.
func withHost(t *testing.T, a *App, name string) *host {
	t.Helper()
	addr, _ := remote.ParseAddr("me@" + name)
	h := &host{name: name, cfg: settings.Host{Name: name, Addr: addr.String()}, addr: addr, conn: connReady,
		info: proto.HostInfo{Hostname: name, OS: "linux", Arch: "amd64", Home: "/home/me", Kero: "2.1.0", Server: true}}
	a.hosts = append(a.hosts, h)
	a.set.Hosts = append(a.set.Hosts, h.cfg)
	a.applied = a.set.Clone()
	h.loaded = true
	// As setState takes it, but for the Dock's badge, which needs the app running.
	a.mergeState(h, proto.State{
		Projects: []proto.Project{{ID: "rp-" + name, Name: "api", Dir: "/home/me/api"}},
		Sessions: []proto.Session{{
			ID: "rs-" + name, Project: "rp-" + name, Name: "agent on " + name,
			Pane:  proto.Pane{ID: "rpa-" + name, Dir: "/home/me/api", Status: proto.Waiting, Progress: -1},
			Panes: []proto.Pane{{ID: "rpa-" + name, Dir: "/home/me/api", Status: proto.Waiting, Progress: -1}},
		}},
	})
	return h
}

// Each host's state replaces its own part of what the app shows, in the
// hosts' order, and leaves the others', with what the app changed in them
// before their kerods' states come.
func TestHostsMergeState(t *testing.T) {
	a := testApp(t)
	local := a.local()
	a.mergeState(local, a.st)
	build := withHost(t, a, "build")
	if got := len(a.st.Sessions); got != 2 {
		t.Fatalf("%d sessions", got)
	}
	if a.st.Sessions[0].ID != "s1" || a.st.Sessions[1].ID != "rs-build" {
		t.Errorf("order %s, %s", a.st.Sessions[0].ID, a.st.Sessions[1].ID)
	}
	for id, want := range map[string]*host{"s1": local, "a": local, "p1": local, "rs-build": build, "rpa-build": build, "rp-build": build, "unknown": local} {
		if a.hostOf(id) != want {
			t.Errorf("hostOf(%s) = %q", id, a.hostOf(id).name)
		}
	}
	// A rename the app shows at once stays as this machine's state comes.
	a.session("rs-build").Name = "renamed"
	a.mergeState(local, proto.State{Projects: a.projectsOf(local), Sessions: a.sessionsOf(local)})
	if s := a.session("rs-build"); s == nil || s.Name != "renamed" {
		t.Errorf("the remote session after a local state: %+v", s)
	}
	// A session the app made on a host shows before the host says so.
	a.addPending(build, proto.Session{ID: "new", Project: "rp-build", Panes: []proto.Pane{{ID: "np"}}})
	a.mergeState(local, proto.State{Projects: a.projectsOf(local), Sessions: a.sessionsOf(local)})
	if a.session("new") == nil || a.hostOf("np") != build {
		t.Error("a pending session of a host went with this machine's state")
	}
	// A host let go of takes its sessions along.
	a.closeHost(build)
	if a.session("rs-build") != nil || a.session("s1") == nil {
		t.Errorf("after closing the host: %d sessions", len(a.st.Sessions))
	}
}

// The sidebar shows this machine's projects, unlabeled, then each host's
// under its name; a session of a host keeps its host's name in the
// window's title and its notifications.
func TestSidebarHosts(t *testing.T) {
	a := testApp(t)
	a.mergeState(a.local(), a.st)
	withHost(t, a, "build")
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	tt.Frame()
	for _, want := range []string{"demo", "build", "api", "agent on build"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in the sidebar: %q", want, tt.Texts())
		}
	}
	shot(t, tt, "sidebar-hosts")
	ids := a.visibleOrder()
	if len(ids) != 2 || ids[0] != "s1" || ids[1] != "rs-build" {
		t.Errorf("order %q", ids)
	}
	s := a.session("rs-build")
	if got := a.hostedName(s); got != "agent on build · build" {
		t.Errorf("hosted name %q", got)
	}
	if got := a.hostedName(a.session("s1")); got != "demo" {
		t.Errorf("this machine's session is named %q", got)
	}
	// Folded, the host shows its selected session alone.
	a.toggleProject(hostKey(a.hosts[1]))
	if ids := a.visibleOrder(); len(ids) != 1 {
		t.Errorf("folded: %q", ids)
	}
}

// A host the app can't reach says why, and offers what fixes it: Kero
// installed there, for one that has none.
func TestSidebarHostProblems(t *testing.T) {
	a := testApp(t)
	a.mergeState(a.local(), a.st)
	h := withHost(t, a, "build")
	h.conn, h.err, h.errKind = connFailed, "kero is not installed on build", remote.ErrNotInstalled
	tt := ui.NewTester(a.view, 1100, 700)
	tt.Frame()
	if !tt.HasText("Kero isn't installed on build") || !tt.HasText("Install Kero…") {
		t.Errorf("no word of the missing Kero: %q", tt.Texts())
	}
	h.errKind = remote.ErrAuth
	h.err = "Permission denied (publickey)."
	tt.Frame()
	if !tt.HasText("build refused the connection") {
		t.Errorf("no word of the refusal: %q", tt.Texts())
	}
	shot(t, tt, "sidebar-host-refused")
	// Its sessions show as they were, faded, and their panes say why.
	a.selectSession("rs-build")
	tt.Frame()
	if !tt.HasText("Permission denied (publickey).") {
		t.Errorf("the pane of a lost host: %q", tt.Texts())
	}
	h.conn, h.errKind = connVersion, nil
	h.daemonApp = "1.0.0"
	tt.Frame()
	if !tt.HasText("Kero on build is another version") {
		t.Errorf("no word of the version: %q", tt.Texts())
	}
}

// Programs of a host drive the app only as the host's Remote Control
// lets them, whatever this machine's.
func TestRemoteControlPerHost(t *testing.T) {
	a := testApp(t)
	a.mergeState(a.local(), a.st)
	h := withHost(t, a, "build")
	a.set.RemoteControl = true
	if a.remoteControl(h) {
		t.Error("a host drives the app unasked")
	}
	if !a.remoteControl(a.local()) {
		t.Error("this machine's remote control is off")
	}
	h.cfg.RemoteControl = true
	if !a.remoteControl(h) {
		t.Error("a host the user trusts does not drive the app")
	}
}

func TestHostShortDir(t *testing.T) {
	h := &host{name: "b", info: proto.HostInfo{Home: "/home/me"}}
	for in, want := range map[string]string{"/home/me": "~", "/home/me/api": "~/api", "/home/meg": "/home/meg", "/srv": "/srv"} {
		if got := h.shortDir(in); got != want {
			t.Errorf("shortDir(%q) = %q", in, got)
		}
	}
}

func TestShellEscape(t *testing.T) {
	for in, want := range map[string]string{
		"/tmp/kero-1000/paste/x/pasted-image.png": "/tmp/kero-1000/paste/x/pasted-image.png",
		"/tmp/a b/it's (1).png":                   `/tmp/a\ b/it\'s\ \(1\).png`,
		"/tmp/$HOME;rm":                           `/tmp/\$HOME\;rm`,
	} {
		if got := shellEscape(in); got != want {
			t.Errorf("shellEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPickerItems(t *testing.T) {
	a := testApp(t)
	a.picker = folderPicker{open: true, host: a.local(), list: &proto.FSList{Dir: "/home/me", Parent: "/home", Entries: []proto.FSEntry{
		{Name: "api", Dir: true, Repo: true}, {Name: "notes.txt"}, {Name: "web", Dir: true},
	}}}
	var got []string
	for _, it := range a.pickerItems() {
		got = append(got, it.name)
	}
	if strings.Join(got, ",") != "..,api,web" {
		t.Errorf("items %q", got)
	}
	a.picker.query = "wb"
	if items := a.pickerItems(); len(items) != 1 || items[0].path != "/home/me/web" {
		t.Errorf("filtered %+v", items)
	}
	for q, want := range map[string]bool{"~/x": true, "/srv": true, "~": true, "src": false, "~x": false} {
		if isPathQuery(q) != want {
			t.Errorf("isPathQuery(%q)", q)
		}
	}
	_ = fmt.Sprint()
}
