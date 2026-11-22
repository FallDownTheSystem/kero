package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"kero/internal/assistant"
	"kero/internal/client"
	"kero/internal/proto"
	"kero/internal/settings"
)

// assistantsUsage is kero assistants' usage, which kero help shows too.
const assistantsUsage = `kero assistants [ls] [--json]
kero assistants show <assistant> [--json]
kero assistants news [--json]
kero assistants reports <assistant> [-n 5] [--json]
kero assistants create <name> --role <text> [--tracks <text> | --tracks-file <file>] [--group <g>]
      [--color <name|#rrggbb>] [--avatar <glyph|image file>] [--agent <command>] [--args <args>] [--check <spec>]… [--no-start]
kero assistants edit <assistant> [--name <n>] [the flags of create] [--remove-check <id>]…
kero assistants pause|resume <assistant>
kero assistants run <assistant> [<check id>]
kero assistants start <assistant> [--new]
kero assistants remove <assistant> [--delete-folder]
kero assistants report [--assistant <id>] <text…|->`

// assistantsCmd is kero assistants: the user's assistants, which this
// machine's kerod keeps.
func assistantsCmd(c *client.Client, st proto.State, args []string) error {
	sub := "ls"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "help", "--help", "-h":
		fmt.Println(assistantsUsage)
		return nil
	case "ls", "list":
		f, err := parse(args, nil, map[string]string{"json": "json"})
		if err != nil {
			return err
		}
		if f.set["json"] {
			return printJSON(assistantRows(st))
		}
		listAssistants(st)
		return nil
	case "show":
		f, err := parse(args, nil, map[string]string{"json": "json"})
		if err != nil {
			return err
		}
		a, err := findAssistant(st, f.pos)
		if err != nil {
			return err
		}
		if f.set["json"] {
			return printJSON(a)
		}
		showAssistant(a)
		return nil
	case "news":
		f, err := parse(args, nil, map[string]string{"json": "json"})
		if err != nil {
			return err
		}
		news := assistantNews(c, st)
		if f.set["json"] {
			return printJSON(news)
		}
		printNews(news)
		return nil
	case "reports":
		f, err := parse(args, map[string]string{"n": "n"}, map[string]string{"json": "json"})
		if err != nil {
			return err
		}
		a, err := findAssistant(st, f.pos)
		if err != nil {
			return err
		}
		n := 5
		if v := f.vals["n"]; v != "" {
			if _, err := fmt.Sscan(v, &n); err != nil || n < 1 {
				return usageErr("-n %q: a number from 1", v)
			}
		}
		var rs []proto.AssistantReport
		if err := c.Call("assistant.reports", proto.AssistantParams{ID: a.ID, Limit: n}, &rs); err != nil {
			return err
		}
		if f.set["json"] {
			return printJSON(rs)
		}
		for _, r := range rs {
			fmt.Printf("── %s\n%s\n\n", time.UnixMilli(r.At).Format("Mon Jan 2 15:04"), r.Text)
		}
		if len(rs) == 0 {
			fmt.Println("no reports yet")
		}
		return nil
	case "create", "edit":
		return editAssistant(c, st, sub, args)
	case "pause", "resume":
		f, err := parse(args, nil, nil)
		if err != nil {
			return err
		}
		a, err := findAssistant(st, f.pos)
		if err != nil {
			return err
		}
		paused := sub == "pause"
		return c.Call("assistant.update", proto.AssistantEdit{ID: a.ID, Paused: &paused}, nil)
	case "run":
		f, err := parse(args, nil, nil)
		if err != nil {
			return err
		}
		a, err := findAssistant(st, f.pos)
		if err != nil {
			return err
		}
		check := ""
		if len(f.pos) > 1 {
			check = f.pos[1]
		}
		if err := c.Call("assistant.run", proto.AssistantParams{ID: a.ID, Check: check}, nil); err != nil {
			return err
		}
		fmt.Printf("queued for %s: its agent gets the checks once it is at its prompt\n", a.Name)
		return nil
	case "start":
		f, err := parse(args, nil, map[string]string{"new": "new", "json": "json"})
		if err != nil {
			return err
		}
		a, err := findAssistant(st, f.pos)
		if err != nil {
			return err
		}
		var s proto.Session
		if err := c.Call("assistant.start", proto.AssistantParams{ID: a.ID, New: f.set["new"]}, &s); err != nil {
			return err
		}
		if f.set["json"] {
			return printJSON(s)
		}
		fmt.Println(s.ID)
		return nil
	case "remove", "rm":
		f, err := parse(args, nil, map[string]string{"delete-folder": "folder"})
		if err != nil {
			return err
		}
		a, err := findAssistant(st, f.pos)
		if err != nil {
			return err
		}
		return c.Call("assistant.remove", proto.AssistantParams{ID: a.ID, Folder: f.set["folder"]}, nil)
	case "report":
		f, err := parse(args, map[string]string{"assistant": "assistant"}, nil)
		if err != nil {
			return err
		}
		text, err := textArg(append(f.pos, f.rest...))
		if err != nil {
			return err
		}
		if strings.TrimSpace(text) == "" {
			return usageErr("kero assistants report [--assistant <id>] <text…|->")
		}
		id, from := f.vals["assistant"], current()
		if id == "" {
			id = os.Getenv("KERO_ASSISTANT")
		}
		if id == "" && from == "" {
			return usageErr("kero assistants report: whose? Run it in an assistant's session, or give --assistant")
		}
		return c.Call("assistant.report", proto.AssistantReportParams{Assistant: id, From: from, Text: text}, nil)
	}
	return usageErr("%s", assistantsUsage)
}

// textArg is a text given as words, or with -, read from stdin.
func textArg(words []string) (string, error) {
	text := strings.Join(words, " ")
	if text == "-" {
		b, err := io.ReadAll(os.Stdin)
		return string(b), err
	}
	return text, nil
}

// findAssistant finds the assistant pos[0] names: its id, a prefix of it,
// or its name, of any case.
func findAssistant(st proto.State, pos []string) (proto.Assistant, error) {
	if len(pos) == 0 {
		return proto.Assistant{}, usageErr("which assistant?")
	}
	ref := pos[0]
	var found []proto.Assistant
	for _, a := range st.Assistants {
		if a.ID == ref || strings.EqualFold(a.Name, ref) {
			return a, nil
		}
		if strings.HasPrefix(a.ID, strings.ToLower(ref)) {
			found = append(found, a)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return proto.Assistant{}, fmt.Errorf("%w: no assistant %q", errNotFound, ref)
	}
	return proto.Assistant{}, fmt.Errorf("%w: %q names several assistants", errNotFound, ref)
}

// editAssistant is kero assistants create and edit.
func editAssistant(c *client.Client, st proto.State, sub string, args []string) error {
	valued := map[string]string{"name": "name", "role": "role", "tracks": "tracks", "tracks-file": "tracks-file", "group": "group",
		"color": "color", "avatar": "avatar", "agent": "agent", "args": "args"}
	// --check and --remove-check repeat: take them out first.
	var specs, removes []string
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, val, has := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if strings.HasPrefix(a, "--") && (name == "check" || name == "remove-check") {
			if !has {
				if i+1 >= len(args) {
					return usageErr("%s needs a value", a)
				}
				i++
				val = args[i]
			}
			if name == "check" {
				specs = append(specs, val)
			} else {
				removes = append(removes, val)
			}
			continue
		}
		rest = append(rest, a)
	}
	f, err := parse(rest, valued, map[string]string{"no-start": "no-start", "json": "json"})
	if err != nil {
		return err
	}
	var e proto.AssistantEdit
	var cur proto.Assistant
	if sub == "create" {
		if len(f.pos) == 0 {
			return usageErr("kero assistants create <name> --role <text> …")
		}
		name := strings.Join(f.pos, " ")
		e.Name, e.Start = &name, !f.set["no-start"]
	} else {
		if cur, err = findAssistant(st, f.pos); err != nil {
			return err
		}
		e.ID = cur.ID
	}
	for key, dst := range map[string]**string{"name": &e.Name, "role": &e.Role, "tracks": &e.Tracks, "group": &e.Group,
		"color": &e.Color, "agent": &e.Agent, "args": &e.AgentArgs} {
		if v, ok := f.vals[key]; ok {
			*dst = &v
		}
	}
	if e.Color != nil {
		// A named color or #rrggbb, of any case: kerod keeps hex lowercase.
		c := strings.ToLower(strings.TrimSpace(*e.Color))
		e.Color = &c
	}
	if p := f.vals["tracks-file"]; p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		t := string(b)
		e.Tracks = &t
	}
	if v, ok := f.vals["avatar"]; ok {
		if _, err := os.Stat(v); err == nil && filepath.Ext(v) != "" {
			// A file: kerod copies it into the assistant's folder.
			abs, _ := filepath.Abs(v)
			e.Image = abs
		} else {
			e.Avatar = &v
		}
	}
	if len(specs) > 0 || len(removes) > 0 {
		checks := cur.Checks
		for _, id := range removes {
			n := len(checks)
			checks = deleteCheck(checks, id)
			if len(checks) == n {
				return fmt.Errorf("%s has no check %q", cur.Name, id)
			}
		}
		for _, spec := range specs {
			ch, err := assistant.ParseCheck(spec)
			if err != nil {
				return usageErr("%v", err)
			}
			checks = append(checks, ch)
		}
		e.Checks = &checks
	}
	method := "assistant.create"
	if sub == "edit" {
		method = "assistant.update"
	}
	var a proto.Assistant
	if err := c.Call(method, e, &a); err != nil {
		return err
	}
	if f.set["json"] {
		return printJSON(a)
	}
	fmt.Println(a.ID)
	return nil
}

func deleteCheck(checks []proto.AssistantCheck, id string) []proto.AssistantCheck {
	out := make([]proto.AssistantCheck, 0, len(checks))
	for _, c := range checks {
		if c.ID != id {
			out = append(out, c)
		}
	}
	return out
}

// assistantRow is an assistant as kero assistants --json lists it.
type assistantRow struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Group      string `json:"group,omitempty"`
	Session    string `json:"session,omitempty"`
	Status     string `json:"status"`
	StatusLine string `json:"statusLine,omitempty"`
	NeedsYou   bool   `json:"needsYou"`
	Paused     bool   `json:"paused,omitempty"`
	NextCheck  string `json:"nextCheck,omitempty"`
	Problem    string `json:"problem,omitempty"`
}

// assistantSession is assistant a's session in st, nil for none.
func assistantSession(st proto.State, a proto.Assistant) *proto.Session {
	for i := range st.Sessions {
		if s := &st.Sessions[i]; s.Assistant == a.ID {
			return s
		}
	}
	return nil
}

// needsYou tells an assistant's session that needs the user: waiting or
// failed, as the sidebar and notifications tell it.
func needsYou(s *proto.Session) bool {
	return s != nil && (s.Status == proto.Waiting || s.Status == proto.Failed)
}

func statusOf(s *proto.Session) string {
	if s == nil {
		return "not running"
	}
	return string(s.Status)
}

func nextCheck(a proto.Assistant) string {
	if a.NextCheck == 0 {
		return ""
	}
	return time.UnixMilli(a.NextCheck).Format("Mon Jan 2 15:04")
}

func assistantRows(st proto.State) []assistantRow {
	rows := make([]assistantRow, 0, len(st.Assistants))
	for _, a := range st.Assistants {
		s := assistantSession(st, a)
		r := assistantRow{ID: a.ID, Name: a.Name, Group: a.Group, Session: a.Session, Status: statusOf(s), NeedsYou: needsYou(s),
			Paused: a.Paused, NextCheck: nextCheck(a), Problem: a.Problem}
		if s != nil {
			r.StatusLine = s.Message
		}
		rows = append(rows, r)
	}
	return rows
}

func listAssistants(st proto.State) {
	if len(st.Assistants) == 0 {
		fmt.Println("no assistants: kero assistants create <name> --role <text> makes one")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tGROUP\tSTATUS\tNEXT CHECK\tSTATUS LINE")
	for _, r := range assistantRows(st) {
		status := r.Status
		switch {
		case r.NeedsYou:
			status += " (needs you)"
		case r.Paused:
			status += " (paused)"
		}
		line := r.StatusLine
		if r.Problem != "" {
			line = "problem: " + r.Problem
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", r.ID, r.Name, orDash(r.Group), status, orDash(r.NextCheck), line)
	}
	w.Flush()
}

func showAssistant(a proto.Assistant) {
	fmt.Printf("%s (%s)\n  role: %s\n", a.Name, a.ID, a.Role)
	if a.Group != "" {
		fmt.Printf("  group: %s\n", a.Group)
	}
	agent := a.Agent
	if agent == "" {
		agent = settings.Load().DefaultAgent + " (default-agent)"
	}
	fmt.Printf("  agent: %s %s\n  folder: %s\n  session: %s\n", agent, a.AgentArgs, a.Dir, orDash(a.Session))
	if a.Paused {
		fmt.Println("  paused: its checks don't run")
	}
	if a.Problem != "" {
		fmt.Printf("  problem: %s\n", a.Problem)
	}
	if t := strings.TrimSpace(a.Tracks); t != "" {
		fmt.Printf("  keeps track of:\n    %s\n", strings.ReplaceAll(t, "\n", "\n    "))
	}
	fmt.Println("  checks:")
	if len(a.Checks) == 0 {
		fmt.Println("    none")
	}
	for _, c := range a.Checks {
		last := "never"
		if c.Last > 0 {
			last = time.UnixMilli(c.Last).Format("Mon Jan 2 15:04")
		}
		fmt.Printf("    %s  %s: %s (last ran %s)\n", c.ID, assistant.CheckWhen(c), assistant.CheckText(c), last)
	}
}

// newsItem is what kero assistants news tells of an assistant, for Ama.
type newsItem struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Role       string                 `json:"role"`
	Status     string                 `json:"status"`
	StatusLine string                 `json:"statusLine,omitempty"`
	NeedsYou   bool                   `json:"needsYou"`
	Question   string                 `json:"question,omitempty"`
	LastReport *proto.AssistantReport `json:"lastReport,omitempty"`
	Unread     bool                   `json:"unread,omitempty"`
	Waiting    []string               `json:"childrenWaiting,omitempty"`
	NextCheck  string                 `json:"nextCheck,omitempty"`
	Paused     bool                   `json:"paused,omitempty"`
	Problem    string                 `json:"problem,omitempty"`
}

func assistantNews(c *client.Client, st proto.State) []newsItem {
	news := make([]newsItem, 0, len(st.Assistants))
	for _, a := range st.Assistants {
		s := assistantSession(st, a)
		n := newsItem{ID: a.ID, Name: a.Name, Role: a.Role, Status: statusOf(s), NeedsYou: needsYou(s), NextCheck: nextCheck(a),
			Paused: a.Paused, Problem: a.Problem}
		var rs []proto.AssistantReport
		if c.Call("assistant.reports", proto.AssistantParams{ID: a.ID, Limit: 1}, &rs) == nil && len(rs) > 0 {
			n.LastReport = &rs[0]
		}
		if s != nil {
			n.StatusLine, n.Unread = s.Message, s.Unread
			for _, k := range st.Sessions {
				if k.Parent == s.ID && k.Status == proto.Waiting {
					n.Waiting = append(n.Waiting, fmt.Sprintf("%s %s", k.ID, proto.Quoted(k.DisplayName(proto.NameLead))))
				}
			}
		}
		if n.NeedsYou {
			n.Question = n.StatusLine
			if n.LastReport != nil && n.LastReport.Waiting {
				n.Question = n.LastReport.Text
			}
		}
		news = append(news, n)
	}
	return news
}

func printNews(news []newsItem) {
	if len(news) == 0 {
		fmt.Println("no assistants")
		return
	}
	for _, n := range news {
		head := n.Name + ": " + n.Status
		if n.NeedsYou {
			head += ", needs you"
		}
		fmt.Println(head)
		if n.Question != "" {
			fmt.Printf("  asks: %s\n", n.Question)
		}
		if n.LastReport != nil && !n.NeedsYou {
			fmt.Printf("  reported %s:\n    %s\n", time.UnixMilli(n.LastReport.At).Format("Mon Jan 2 15:04"),
				strings.ReplaceAll(strings.TrimSpace(n.LastReport.Text), "\n", "\n    "))
		}
		for _, k := range n.Waiting {
			fmt.Printf("  child waiting: %s\n", k)
		}
		if n.Problem != "" {
			fmt.Printf("  problem: %s\n", n.Problem)
		}
		if n.NextCheck != "" {
			fmt.Printf("  next check: %s\n", n.NextCheck)
		}
	}
}
