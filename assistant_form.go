package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"kero/internal/assistant"
	"kero/internal/proto"
	"kero/internal/settings"

	"kero/locales"
)

// The New Assistant and Edit dialog: an assistant's name, color, avatar,
// role, what it keeps track of, group, checks and agent, with its row as
// it will show at the top. kerod checks what it is given and says what is
// wrong, which shows under the fields.

// assistantForm is the dialog's state; id "" for a new assistant.
type assistantForm struct {
	open                             bool
	id                               string
	name, color, avatar, image, role string
	tracks, group, agent, args       string
	checks                           []formCheck
	err                              string
	busy                             bool
	choosing                         bool
	// custom is the color well's: the last custom color, which it keeps
	// showing when a named color is chosen again.
	custom ui.Color
}

// formCheck is a check as the dialog edits it.
type formCheck struct {
	id, kind, time, text string
	days                 [7]bool
}

// defaultAgentChoice is the agent popup's first item, the default
// agent's, which leaves an assistant's Agent empty.
const defaultAgentChoice = ""

// openAssistantForm opens the dialog: for a new assistant with id "",
// else to edit one.
func (a *App) openAssistantForm(id string) {
	f := &assistantForm{open: true, id: id}
	if as := a.assistantByID(id); as != nil {
		f.name, f.color, f.avatar, f.role = as.Name, as.Color, as.Avatar, as.Role
		f.tracks, f.group, f.agent, f.args = as.Tracks, as.Group, as.Agent, as.AgentArgs
		if assistant.IsCustomColor(as.Color) {
			f.custom = ui.Hex(as.Color)
		}
		for _, c := range as.Checks {
			fc := formCheck{id: c.ID, kind: c.Kind, time: c.Time, text: c.Text}
			for _, d := range c.Days {
				if d >= 0 && d < 7 {
					fc.days[d] = true
				}
			}
			f.checks = append(f.checks, fc)
		}
	} else if id != "" {
		return
	} else {
		f.checks = []formCheck{{kind: proto.CheckDaily, time: "09:00"}}
	}
	if !a.assistantsShown() {
		a.prefs.View = viewAssistants
		a.savePrefs()
	}
	a.asstForm = f
}

// preview is the assistant as the dialog would make it, for its row.
func (f *assistantForm) preview() proto.Assistant {
	return proto.Assistant{ID: f.id, Name: strings.TrimSpace(f.name), Color: f.color, Avatar: f.avatar, Role: strings.TrimSpace(f.role)}
}

// edit is what the dialog asks kerod for.
func (f *assistantForm) edit() proto.AssistantEdit {
	trim := func(s string) *string { t := strings.TrimSpace(s); return &t }
	e := proto.AssistantEdit{ID: f.id, Name: trim(f.name), Color: trim(f.color), Group: trim(f.group), Role: trim(f.role),
		Tracks: trim(f.tracks), Agent: trim(f.agent), AgentArgs: trim(f.args), Image: f.image}
	if f.image == "" {
		e.Avatar = trim(f.avatar)
	}
	checks := []proto.AssistantCheck{}
	for _, fc := range f.checks {
		c := proto.AssistantCheck{ID: fc.id, Kind: fc.kind, Text: strings.TrimSpace(fc.text)}
		if fc.kind != proto.CheckStart {
			c.Time = strings.TrimSpace(fc.time)
		}
		if fc.kind == proto.CheckWeekly {
			for d, on := range fc.days {
				if on {
					c.Days = append(c.Days, d)
				}
			}
		}
		checks = append(checks, c)
	}
	e.Checks = &checks
	return e
}

// saveAssistantForm creates or changes the assistant; a new one's session
// starts and shows, for the user to answer its agent's first questions.
func (a *App) saveAssistantForm(f *assistantForm) {
	h := a.local()
	if f.busy || h.cl == nil || h.conn != connReady {
		return
	}
	if strings.TrimSpace(f.name) == "" || strings.TrimSpace(f.role) == "" {
		return
	}
	f.busy, f.err = true, ""
	cl, e, method := h.cl, f.edit(), "assistant.update"
	if f.id == "" {
		method = "assistant.create"
	}
	go func() {
		var as proto.Assistant
		err := cl.Call(method, e, &as)
		a.update(func() {
			f.busy = false
			if err != nil {
				f.err = err.Error()
				return
			}
			f.open = false
			if method == "assistant.create" {
				a.startAssistant(as.ID, false)
			}
		})
	}()
}

// chooseAvatar asks for an image, which kerod copies into the assistant's
// folder as its avatar.
func (a *App) chooseAvatar(f *assistantForm) {
	if f.choosing {
		return
	}
	f.choosing = true
	win := a.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: win, Title: locales.AssistantFormImage(), ButtonLabel: locales.CommonChoose()})
		a.update(func() {
			f.choosing = false
			if err == nil && len(paths) > 0 {
				f.image, f.avatar = paths[0], ""
			}
		})
	}()
}

// assistantFormView is the dialog.
func (a *App) assistantFormView(c *ui.Context, pal *palette) {
	f := a.asstForm
	if f == nil {
		return
	}
	save := false
	ui.DialogBase(c.Key("assistant-form"), &f.open, func(backdrop, panel ui.Element) {
		backdrop.Background(pal.overlay)
		title := locales.AssistantFormNew()
		if f.id != "" {
			title = locales.AssistantFormEdit(f.name)
		}
		_, winH := c.Size()
		floating(panel.Width(560).MaxHeight(winH-40).Padding(22).Gap(8).Radius(14).Background(pal.popover), pal).Label(title)
		if panel.Shortcut(ui.Cmd, ui.KeyEnter) {
			save = true
		}
		// The row as it will show.
		ui.Row(c.Key("preview")).Gap(10).AlignItems(ui.Center).Children(func() {
			p := f.preview()
			if p.Name == "" {
				p.Name = title
			}
			assistantAvatar(c, pal, &p, 30)
			ui.Column(c).Gap(2).MinWidth(0).Children(func() {
				ui.Text(c, p.Name).FontSize(15).FontWeight(650).TextColor(pal.text).SingleLine().Ellipsis("…")
				ui.Text(c, cmpOr(p.Role, locales.AssistantFormRoleHint())).FontSize(12.5).TextColor(pal.textMuted).SingleLine().Ellipsis("…")
			})
		})
		ui.Scroll(c.Key("fields")).Shrink(1).MinHeight(0).Margin(6, -6, 0, -6).Padding(4, 6, 2, 6).Children(func() {
			ui.Column(c).Shrink(0).Gap(12).AlignItems(ui.Stretch).Children(func() {
				formRow(c, pal, "name", locales.AssistantFormName(), func() {
					in := textField(c, "name-in", &f.name, locales.AssistantFormName(), 300, pal)
					if f.id == "" {
						in.AutoFocus()
					}
				})
				formRow(c, pal, "role", locales.AssistantFormRole(), func() {
					textField(c, "role-in", &f.role, locales.AssistantFormRoleHint(), 300, pal)
				})
				formRow(c, pal, "color", locales.AssistantFormColor(), func() { a.colorSwatches(c, pal, f) })
				a.avatarPicker(c, pal, f)
				ui.Column(c.Key("tracks")).Gap(6).Children(func() {
					ui.Text(c, locales.AssistantFormTracks()).FontSize(13).TextColor(pal.text)
					frame := ui.Row(c.Key("tracks-frame")).AlignItems(ui.Start).Cursor(ui.CursorText)
					var in ui.Element
					frame.Children(func() {
						in = ui.TextAreaBase(c.Key("in"), &f.tracks).Grow(1).MinWidth(0).MinHeight(72).MaxHeight(200).Padding(8).FontSize(12.5).
							Label(locales.AssistantFormTracks())
					})
					fieldLook(c, frame, in.Focused(), pal).Padding(0)
				})
				formRow(c, pal, "group", locales.AssistantFormGroup(), func() {
					for _, g := range a.groupsInUse() {
						if g != f.group {
							name := g
							chip(c, pal, pal.chip, pal.textMuted, func() { f.group = name }).Children(func() {
								ui.Text(c, name).FontSize(12).SingleLine()
							})
						}
					}
					textField(c, "group-in", &f.group, locales.AssistantFormGroupHint(), 160, pal)
				})
				a.checksFields(c, pal, f)
				formRow(c, pal, "agent", locales.AssistantFormAgent(), func() {
					agents := append([]string{defaultAgentChoice}, a.set.AgentCommands()...)
					if f.agent != "" && !slices.Contains(agents, f.agent) {
						agents = append(agents, f.agent)
					}
					popup(c, "agent", &f.agent, agents, func(cmd string) string {
						if cmd == defaultAgentChoice {
							return locales.AssistantFormDefaultAgent(a.set.DefaultAgent)
						}
						return cmd
					}, 260, pal)
				})
				formRow(c, pal, "args", locales.AssistantFormArgs(), func() {
					textField(c, "args-in", &f.args, "--model …", 260, pal).Font("monospace")
				})
				ui.Text(c, locales.AssistantFormAgentNote()).FontSize(12).TextColor(pal.textMuted).LineHeight(1.4)
				if f.err != "" {
					ui.Text(c, f.err).FontSize(12.5).TextColor(pal.danger).LineHeight(1.4)
				}
			})
		})
		ok := strings.TrimSpace(f.name) != "" && strings.TrimSpace(f.role) != "" && !f.busy
		dialogButtons(c, func() {
			if f.id != "" {
				if button(c, "delete", locales.AssistantsDelete(), "", false, pal) {
					id, name := f.id, f.name
					f.open = false
					a.defer_(func() { a.askDeleteAssistant(id, name) })
				}
				ui.Spacer(c)
			}
			if button(c, "cancel", locales.CommonCancel(), "esc", false, pal) {
				f.open = false
			}
			label := locales.AssistantFormCreate()
			if f.id != "" {
				label = locales.AssistantFormSave()
			}
			ui.Row(c.Key("save")).Opacity(1 - 0.5*b2f(!ok)).Children(func() {
				if button(c, "ok", label, "", true, pal) {
					save = true
				}
			})
		})
	})
	if f.open && save {
		a.saveAssistantForm(f)
	}
	if !f.open {
		a.asstForm = nil
		a.focusTerminal()
	}
}

func cmpOr(s, or string) string {
	if strings.TrimSpace(s) == "" {
		return or
	}
	return s
}

// groupsInUse are the assistants' groups, for the dialog to offer.
func (a *App) groupsInUse() []string {
	var gs []string
	for _, as := range a.st.Assistants {
		if as.Group != "" && !slices.Contains(gs, as.Group) {
			gs = append(gs, as.Group)
		}
	}
	return gs
}

// colorSwatches chooses the assistant's color: one of the hosts' eight,
// or, in the well after them, a custom one.
func (a *App) colorSwatches(c *ui.Context, pal *palette, f *assistantForm) {
	current := assistantColorName(&proto.Assistant{Name: f.name, Color: f.color})
	ring := func(b ui.Element, col ui.Color) {
		b.InsetShadow(0, 0, 0, 2, pal.popover).Shadow(0, 0, 0, 2, pal.assistantInk(&proto.Assistant{Color: f.color, Name: f.name}, pal.popover))
	}
	for _, name := range settings.HostColors {
		col := hostColors[name][0]
		if pal.dark {
			col = hostColors[name][1]
		}
		b := ui.ButtonBase(c.Key("color-"+name)).Size(20, 20).Radius(10).Background(col).Label(colorLabel(name)).Tooltip(colorLabel(name))
		if current == name {
			ring(b, col)
		}
		if b.Clicked() {
			f.color = name
		}
	}
	ui.Box(c.Key("color-rule")).Size(1, 16).Background(pal.text.Alpha(0.14))
	// The well holds the last custom color; unset, a plus on a neutral
	// face stands over it, which the pointer passes through.
	set := f.custom != (ui.Color{})
	if !set {
		f.custom = ui.Hex("#808080")
	}
	ui.Box(c.Key("custom")).Size(20, 20).Shrink(0).Children(func() {
		w := ui.ColorWell(c.Key("custom-well"), &f.custom).Size(20, 20).Radius(10).Padding(0).Clip().
			Label(locales.AssistantFormCustomColor()).Tooltip(locales.AssistantFormCustomColor())
		if !set {
			w.Background(pal.text.Alpha(0.06)).Border(1, pal.text.Alpha(0.16))
			ui.Box(c.Key("plus")).Absolute().Top(0).Left(0).Size(20, 20).Radius(10).Center().PassThrough().
				Background(pal.popover).Border(1, pal.text.Alpha(0.18)).Children(func() {
				ui.Icon(c, plusIcon).Size(11, 11).TextColor(pal.textMuted)
			})
		} else {
			w.Border(0, ui.Color{})
			if assistant.IsCustomColor(f.color) {
				ring(w, f.custom)
			}
		}
		if w.Changed() {
			col := ui.RGB(f.custom.R, f.custom.G, f.custom.B) // a picker's opacity, ignored
			f.custom = col
			f.color = fmt.Sprintf("#%02x%02x%02x", col.R, col.G, col.B)
		} else if !set {
			f.custom = ui.Color{}
		}
	})
}

// avatarPicker chooses its avatar: its initial, a built-in glyph, or an
// image of the user's.
func (a *App) avatarPicker(c *ui.Context, pal *palette, f *assistantForm) {
	ui.Column(c.Key("avatar")).Gap(6).Children(func() {
		ui.Text(c, locales.AssistantFormAvatar()).FontSize(13).TextColor(pal.text)
		ui.Row(c).Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
			col := pal.assistantInk(&proto.Assistant{Name: f.name, Color: f.color}, pal.popover)
			for _, name := range append([]string{""}, avatarNames...) {
				on := f.image == "" && f.avatar == name
				b := ui.ButtonBase(c.Key("av-"+name)).Size(30, 30).Radius(8).Center().Label(avatarLabel(name)).Tooltip(avatarLabel(name))
				switch {
				case on:
					b.Background(col.Alpha(0.18)).InsetShadow(0, 0, 0, 1.5, col)
				case b.Hovered():
					b.Background(pal.rowHover)
				}
				if b.Clicked() {
					f.avatar, f.image = name, ""
				}
				b.Children(func() {
					if g := avatarGlyphs[name]; g != nil {
						ui.Icon(c, g).Size(16, 16).TextColor(col)
					} else {
						ui.Text(c, initial(cmpOr(f.name, "A"))).FontSize(14).FontWeight(650).TextColor(col)
					}
				})
			}
			label := locales.AssistantFormImage()
			if f.image != "" {
				label = shortName(f.image)
			} else if strings.HasPrefix(f.avatar, "file:") {
				label = locales.AssistantFormImage() + " ✓"
			}
			if button(c, "image", label, "", false, pal) {
				a.chooseAvatar(f)
			}
		})
	})
}

func shortName(path string) string {
	path = strings.ReplaceAll(path, `\`, "/")
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[i+1:]
	}
	return path
}

// checkKinds are the kinds of check, as the dialog offers them.
var checkKinds = []string{proto.CheckDaily, proto.CheckWeekly, proto.CheckStart}

func checkKindLabel(kind string) string {
	switch kind {
	case proto.CheckStart:
		return locales.CheckKindStart()
	case proto.CheckWeekly:
		return locales.CheckKindWeekly()
	}
	return locales.CheckKindDaily()
}

// checksFields are the assistant's checks, a row each: when, the days of a
// weekly one, its time, and what to check; then Add Check.
func (a *App) checksFields(c *ui.Context, pal *palette, f *assistantForm) {
	ui.Column(c.Key("checks")).Gap(8).Children(func() {
		ui.Text(c, locales.AssistantFormChecks()).FontSize(13).TextColor(pal.text)
		days := strings.Split(locales.WeekdaysShort(), ",")
		remove := -1
		for i := range f.checks {
			fc := &f.checks[i]
			ui.Column(c.Key("check-" + strconv.Itoa(i))).Gap(6).Padding(8).Radius(8).Background(pal.text.Alpha(0.035)).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					popup(c, "kind", &fc.kind, checkKinds, checkKindLabel, 170, pal)
					if fc.kind != proto.CheckStart {
						textField(c, "time", &fc.time, "09:00", 64, pal)
					}
					ui.Spacer(c)
					if button(c, "remove", locales.AssistantFormRemoveCheck(), "", false, pal) {
						remove = i
					}
				})
				if fc.kind == proto.CheckWeekly && len(days) == 7 {
					ui.Row(c.Key("days")).Gap(4).Children(func() {
						// Monday first, as most of the world's weeks start.
						for _, d := range []int{1, 2, 3, 4, 5, 6, 0} {
							on := fc.days[d]
							b := ui.ButtonBase(c.Key("day-"+strconv.Itoa(d))).Height(22).Padding(0, 8).Radius(11).Center().Label(days[d])
							fg := pal.textMuted
							switch {
							case on:
								b.Background(pal.accent.Alpha(0.15))
								fg = pal.accent
							case b.Hovered():
								b.Background(pal.rowHover)
							}
							if b.Clicked() {
								fc.days[d] = !on
							}
							b.Children(func() { ui.Text(c, days[d]).FontSize(12).TextColor(fg).SingleLine() })
						}
					})
				}
				ui.Row(c).Children(func() { textField(c, "text", &fc.text, locales.AssistantFormCheckText(), 0, pal) })
			})
		}
		if remove >= 0 {
			f.checks = slices.Delete(f.checks, remove, remove+1)
		}
		ui.Row(c).Children(func() {
			if len(f.checks) < 8 && button(c, "add-check", locales.AssistantFormAddCheck(), "", false, pal) {
				f.checks = append(f.checks, formCheck{kind: proto.CheckDaily, time: "09:00"})
			}
		})
		ui.Text(c, locales.AssistantFormChecksNote()).FontSize(12).TextColor(pal.textMuted).LineHeight(1.4)
	})
}
