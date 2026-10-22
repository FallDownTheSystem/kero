package main

import (
	"errors"
	"strings"

	"github.com/egoist/mygo/ui"

	"kero/internal/remote"
	"kero/internal/settings"

	"kero/locales"
)

// hostDraft is the Hosts page's form, which adds a host.
type hostDraft struct {
	name, addr string
	err        string
}

// hostsPage is Settings › Hosts: the machines whose sessions the app
// shows beside this one's, which the configuration's host keys keep, as
// `kero hosts` does.
func (a *App) hostsPage(c *ui.Context, pal *palette) {
	settingsGroup(c, pal, locales.SetHosts(), locales.SetHostsDetail(), func() {
		if len(a.hosts) == 1 {
			settingRow(c, pal, locales.SetHostsNone(), locales.SetHostsNoneDetail(), func() {})
		}
		for _, h := range a.hosts[1:] {
			a.hostRow(c, pal, h)
		}
	})
	settingsGroup(c, pal, locales.SetHostAdd(), locales.SetHostAddDetail(), func() {
		d := &a.ss.host
		ui.Row(c).Padding(10, 0).Gap(8).AlignItems(ui.Center).Children(func() {
			add := func() { a.addHost() }
			textField(c, "host-addr", &d.addr, locales.SetHostAddressPlaceholder(), 0, pal).Font("monospace").OnSubmit(add)
			textField(c, "host-name", &d.name, locales.SetHostNamePlaceholder(defaultHostName(d.addr)), 130, pal).OnSubmit(add)
			if button(c, "host-add", locales.CommonAdd(), "", true, pal) {
				add()
			}
		})
		if d.err != "" {
			ui.Text(c, d.err).FontSize(12).TextColor(pal.danger).Padding(0, 0, 10, 0)
		}
	})
	codeBlock(c, pal, "# on the machine, once: the server build of Kero, kerod as a service\ncurl -fsSL https://github.com/egoist/kero/releases/latest/download/install-server.sh | sh\n\n# from here\nkero hosts add devbox me@devbox\nkero -H devbox ls")
	settingsGroup(c, pal, locales.SetHostSettingsSplit(), locales.SetHostSettingsSplitDetail(), nil)
}

// hostRow is a host in Settings › Hosts: how the app stands with it, and
// what the user may do about it.
func (a *App) hostRow(c *ui.Context, pal *palette, h *host) {
	status := a.hostStatus(h)
	if groupRows++; groupRows > 1 {
		ui.Box(c).Height(1).Shrink(0).Background(pal.text.Alpha(0.07))
	}
	ui.Column(c.Key("host-"+h.name)).Padding(11, 0).Gap(8).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, hostIcon).Size(16, 16).TextColor(pal.textMuted).Shrink(0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, h.name).FontSize(13.5).FontWeight(600).TextColor(pal.text).SingleLine()
					ui.Text(c, h.addr.String()).Font("monospace").FontSize(12).TextColor(pal.textMuted).SingleLine().Ellipsis("…").MinWidth(0).Selectable()
				})
				col := pal.textMuted
				if h.conn == connFailed {
					col = pal.danger
				}
				ui.Text(c, status).FontSize(12).TextColor(col).LineHeight(1.35).Selectable()
			})
			iconButton(c, "reconnect", reloadIcon, locales.HostReconnect(), pal, func() { a.reconnectHost(h) })
			iconButton(c, "remove", closeIcon, locales.HostRemove(), pal, func() { a.confirmRemoveHost(h) })
		})
		ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(0, 0, 0, 26).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, locales.SetHostRemoteControl()).FontSize(12.5).TextColor(pal.text)
				ui.Text(c, locales.SetHostRemoteControlDetail(h.name)).FontSize(12).TextColor(pal.textMuted).LineHeight(1.35)
			})
			// The switch shows the host's setting, which a click writes back.
			on, name := h.cfg.RemoteControl, h.name
			toggle(c.Key("rc"), &on, pal).OnClick(func() {
				s := a.set.Clone()
				if cfg := s.Host(name); cfg != nil {
					cfg.RemoteControl = on
				}
				a.set = s
			})
		})
		// What fixes it, a button of its own size, under the host.
		ui.Row(c).Padding(0, 0, 0, 26).Children(func() {
			switch {
			case h.busy != "":
			case errorsIsNotInstalled(h):
				if button(c, "install", locales.HostInstall(), "", true, pal) {
					a.installOnHost(h)
				}
			case h.conn == connVersion || h.behind():
				if button(c, "update", locales.HostUpdateAction(h.name), "", true, pal) {
					a.offerHostUpdate(h)
				}
			}
		})
	})
}

func errorsIsNotInstalled(h *host) bool { return errors.Is(h.errKind, remote.ErrNotInstalled) }

// hostStatus says how the app stands with a host.
func (a *App) hostStatus(h *host) string {
	switch {
	case h.busy == "install":
		return locales.HostInstalling(h.name)
	case h.busy == "update":
		return locales.HostUpdating(h.name)
	case h.busy == "restart":
		return locales.HostRestarting(h.name)
	case h.conn == connConnecting && h.loaded:
		return locales.HostReconnecting()
	case h.conn == connConnecting:
		return locales.StatusConnecting()
	case h.conn == connFailed && errors.Is(h.errKind, remote.ErrNotInstalled):
		return a.hostProblem(h)
	case h.conn == connFailed:
		return a.hostProblem(h) + ": " + h.err
	case h.conn == connVersion:
		return a.hostVersionDetail(h)
	}
	i := h.info
	s := locales.HostConnected(i.Kero, i.OS+"/"+i.Arch, i.Hostname, i.User)
	if n := len(a.sessionsOf(h)); n > 0 {
		s += " · " + locales.HostSessions(n)
	}
	return s
}

// defaultHostName is the name a host of an address takes when the user
// gives none.
func defaultHostName(addr string) string {
	a, err := remote.ParseAddr(addr)
	if err != nil {
		return "devbox"
	}
	return a.Label()
}

// addHost adds the host of the Hosts page's form.
func (a *App) addHost() {
	d := &a.ss.host
	addr := strings.TrimSpace(d.addr)
	name := strings.TrimSpace(d.name)
	if addr == "" {
		d.err = locales.SetHostNeedsAddress()
		return
	}
	if name == "" {
		name = defaultHostName(addr)
	}
	h, err := settings.ParseHost(name + "=" + addr)
	if err != nil {
		d.err = err.Error()
		return
	}
	if a.set.Host(h.Name) != nil {
		d.err = locales.SetHostNameTaken(h.Name)
		return
	}
	s := a.set.Clone()
	s.Hosts = append(s.Hosts, h)
	a.set = s
	*d = hostDraft{}
}
