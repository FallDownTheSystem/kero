package cli

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"kero/internal/client"
	"kero/internal/proto"
)

// The browser commands of the page's DevTools: its console, its network,
// and the device and network it is shown as.
var devlogCommands = []string{"console", "network", "emulate"}

// devlogFlags are the flags only they take.
var devlogFlags = map[string][]string{
	"console": {"errors", "clear"},
	"network": {"failed", "url", "clear", "out"},
	"emulate": {"scale", "mobile", "touch"},
}

// devlogFlag tells of a flag of theirs given to another command.
func devlogFlag(f *flags, op string) error {
	for cmd, names := range devlogFlags {
		for _, k := range names {
			if k != "out" && cmd != op && (f.set[k] || f.vals[k] != "") {
				return usageErr("--%s is %s's", k, cmd)
			}
		}
	}
	return nil
}

// devlogCmd does kero browser console, network and emulate, whose
// flags it checks itself, then prints what the app answered: as text, or
// with --json, as JSON.
func devlogCmd(c *client.Client, p proto.BrowserParams, op string, pos []string, f *flags) error {
	if err := devlogFlag(f, op); err != nil {
		return err
	}
	for _, k := range []string{"text", "domain"} {
		if f.vals[k] != "" {
			return usageErr("--%s is not %s's", k, op)
		}
	}
	if f.set["enter"] || p.Ref != "" || len(f.rest) > 0 && op != "emulate" {
		return usageErr("kero browser %s takes no --enter, --ref or --", op)
	}
	if f.vals["out"] != "" && (op != "network" || len(pos) == 0 || pos[0] != "body") {
		return usageErr("-o is screenshot's and network body's")
	}
	switch op {
	case "console":
		if len(pos) > 0 {
			return usageErr("kero browser [target] console [--errors] [--clear] [--json]")
		}
		p.Clear = f.set["clear"]
		var res proto.BrowserResult
		if err := c.Call("browser", p, &res); err != nil {
			return err
		}
		list := res.Console[:0:0]
		for _, e := range res.Console {
			if !f.set["errors"] || e.Level == "error" {
				list = append(list, e)
			}
		}
		if f.set["json"] {
			return printJSON(list)
		}
		printConsole(os.Stdout, list)
		return nil
	case "network":
		if len(pos) > 0 {
			return networkBody(c, p, pos, f)
		}
		p.Clear = f.set["clear"]
		var res proto.BrowserResult
		if err := c.Call("browser", p, &res); err != nil {
			return err
		}
		list := res.Network[:0:0]
		for _, r := range res.Network {
			if f.set["failed"] && r.Error == "" && r.Status < 400 {
				continue
			}
			if u := f.vals["url"]; u != "" && !strings.Contains(strings.ToLower(r.URL), strings.ToLower(u)) {
				continue
			}
			list = append(list, r)
		}
		if f.set["json"] {
			return printJSON(list)
		}
		printNetwork(os.Stdout, list)
		return nil
	}
	return emulateCmd(c, p, pos, f)
}

// networkBody prints the body of a response of the log, or writes it to
// -o's file.
func networkBody(c *client.Client, p proto.BrowserParams, pos []string, f *flags) error {
	const use = "kero browser [target] network body <id> [-o file] [--json]"
	if pos[0] != "body" || len(pos) != 2 || f.set["failed"] || f.set["clear"] || f.vals["url"] != "" {
		return usageErr("%s", use)
	}
	id, err := strconv.Atoi(strings.TrimPrefix(pos[1], "#"))
	if err != nil || id <= 0 {
		return usageErr("%s: the id is the log's number of the request, as #12", use)
	}
	p.Text, p.Request = "body", id
	var res proto.BrowserResult
	if err := c.Call("browser", p, &res); err != nil {
		return err
	}
	if f.set["json"] {
		out := struct {
			proto.NetworkRequest
			Body          string `json:"body"`
			Base64Encoded bool   `json:"base64Encoded"`
		}{Body: string(res.Body)}
		if len(res.Network) == 1 {
			out.NetworkRequest = res.Network[0]
		}
		if !utf8.Valid(res.Body) {
			out.Body, out.Base64Encoded = base64.StdEncoding.EncodeToString(res.Body), true
		}
		return printJSON(out)
	}
	switch out := f.vals["out"]; out {
	case "", "-":
		_, err := os.Stdout.Write(res.Body)
		return err
	default:
		if err := os.WriteFile(out, res.Body, 0o644); err != nil {
			return err
		}
		fmt.Println(out)
		return nil
	}
}

// device is a phone's or a tablet's screen, as emulate shows a page on
// it.
type device struct {
	name          string
	width, height int
	scale         float64
	ua            string
}

const (
	iosUA     = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1"
	ipadUA    = "Mozilla/5.0 (iPad; CPU OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1"
	androidUA = "Mozilla/5.0 (Linux; Android 14; %s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36"
)

var devices = []device{
	{"iphone-se", 375, 667, 2, iosUA},
	{"iphone-15", 393, 852, 3, iosUA},
	{"iphone-15-pro-max", 430, 932, 3, iosUA},
	{"pixel-8", 412, 915, 2.625, fmt.Sprintf(androidUA, "Pixel 8")},
	{"galaxy-s24", 360, 780, 3, fmt.Sprintf(androidUA, "SM-S921B")},
	{"ipad", 820, 1180, 2, ipadUA},
	{"ipad-pro", 1024, 1366, 2, ipadUA},
}

// networks are the presets of a slow network, as Chrome's DevTools
// throttle: latency in ms, throughput in bytes a second.
var networks = []proto.Emulation{
	{Network: "offline"},
	{Network: "slow-3g", Latency: 2000, Download: 50000, Upload: 50000},
	{Network: "3g", Latency: 562.5, Download: 180000, Upload: 84375},
	{Network: "4g", Latency: 165, Download: 1012500, Upload: 168750},
	{Network: "none"},
}

// emulateCmd shows the page as a device or on a network would, or as it
// is again, and prints what it is shown as.
func emulateCmd(c *client.Client, p proto.BrowserParams, pos []string, f *flags) error {
	const use = "kero browser [target] emulate [device <name> | viewport <width>x<height> [--scale n] [--mobile] [--touch] | user-agent <ua…> | network <offline|slow-3g|3g|4g|none> | reset] [--json]"
	what, args := "", pos
	if len(pos) > 0 {
		what, args = pos[0], pos[1:]
	}
	if what != "viewport" && (f.vals["scale"] != "" || f.set["mobile"] || f.set["touch"]) {
		return usageErr("--scale, --mobile and --touch are emulate viewport's")
	}
	if what != "user-agent" && len(f.rest) > 0 {
		return usageErr("%s", use)
	}
	to := &proto.Emulation{}
	switch what {
	case "", "reset":
		if len(args) > 0 {
			return usageErr("%s", use)
		}
	case "device":
		if len(args) == 0 {
			// The devices, to pick one of.
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			for _, d := range devices {
				fmt.Fprintf(tw, "%s\t%d×%d\t%g×\n", d.name, d.width, d.height, d.scale)
			}
			return tw.Flush()
		}
		if len(args) > 1 {
			return usageErr("%s", use)
		}
		if args[0] != "none" {
			i := slices.IndexFunc(devices, func(d device) bool { return d.name == strings.ToLower(args[0]) })
			if i < 0 {
				return usageErr("no device %q: kero browser emulate device lists them", args[0])
			}
			d := devices[i]
			to = &proto.Emulation{Device: d.name, Width: d.width, Height: d.height, Scale: d.scale, Mobile: true, Touch: true, UserAgent: d.ua}
		}
	case "viewport":
		if len(args) != 1 {
			return usageErr("%s", use)
		}
		if args[0] != "none" {
			w, h, ok := parseViewport(args[0])
			if !ok {
				return usageErr("a viewport is <width>x<height>, as 390x844, or none")
			}
			to.Width, to.Height, to.Mobile, to.Touch = w, h, f.set["mobile"], f.set["touch"]
			if s := f.vals["scale"]; s != "" {
				v, err := strconv.ParseFloat(s, 64)
				if err != nil || v <= 0 || v > 10 {
					return usageErr("--scale is a device's pixels to a CSS pixel, as 2")
				}
				to.Scale = v
			}
		}
	case "user-agent":
		ua := strings.TrimSpace(strings.Join(append(args, f.rest...), " "))
		if ua == "" {
			return usageErr("%s", use)
		}
		if ua != "none" {
			to.UserAgent = ua
		}
	case "network":
		if len(args) != 1 {
			return usageErr("%s", use)
		}
		i := slices.IndexFunc(networks, func(n proto.Emulation) bool { return n.Network == strings.ToLower(args[0]) })
		if i < 0 {
			return usageErr("a network is offline, slow-3g, 3g, 4g or none")
		}
		n := networks[i]
		to = &n
	default:
		return usageErr("%s", use)
	}
	p.Text, p.Emulate = what, to
	var res proto.BrowserResult
	if err := c.Call("browser", p, &res); err != nil {
		return err
	}
	emu := proto.Emulation{}
	if res.Emulation != nil {
		emu = *res.Emulation
	}
	if f.set["json"] {
		return printJSON(emu)
	}
	printEmulation(os.Stdout, emu)
	if what == "device" || what == "user-agent" {
		fmt.Println("(the page sees the user agent now; its server, once it loads again: kero browser reload)")
	}
	return nil
}

// parseViewport reads a viewport's size, as 390x844.
func parseViewport(s string) (w, h int, ok bool) {
	a, b, cut := strings.Cut(strings.ReplaceAll(strings.ToLower(s), "×", "x"), "x")
	if !cut {
		return 0, 0, false
	}
	w, err1 := strconv.Atoi(a)
	h, err2 := strconv.Atoi(b)
	if err1 != nil || err2 != nil || w < 1 || h < 1 || w > 10000 || h > 10000 {
		return 0, 0, false
	}
	return w, h, true
}

// printConsole prints a console's entries as a console shows them: the
// level, or what threw, then the text and where it came from, and the
// stack of an error.
func printConsole(w io.Writer, list []proto.ConsoleEntry) {
	if len(list) == 0 {
		fmt.Fprintln(w, "no console messages")
		return
	}
	for _, e := range list {
		tag := e.Level
		switch e.Source {
		case "exception", "rejection":
			tag = e.Source
		case "console", "":
		default:
			tag += " " + e.Source
		}
		// Where it came from goes after its first line, before the rest.
		first, more, _ := strings.Cut(strings.TrimRight(e.Text, "\n"), "\n")
		if at := where(e.URL, e.Line, e.Column); at != "" {
			first += "  (" + at + ")"
		}
		fmt.Fprintf(w, "[%s] %s\n", tag, first)
		if more != "" {
			fmt.Fprintf(w, "    %s\n", strings.ReplaceAll(more, "\n", "\n    "))
		}
		for _, f := range e.Stack {
			fmt.Fprintf(w, "    at %s\n", f)
		}
	}
}

func where(url string, line, col int) string {
	switch {
	case url == "":
		return ""
	case line == 0:
		return url
	case col == 0:
		return fmt.Sprintf("%s:%d", url, line)
	}
	return fmt.Sprintf("%s:%d:%d", url, line, col)
}

// printNetwork prints the requests of a network's log, a line each: its
// number, method, status, type, size, time and URL, then why it failed.
func printNetwork(w io.Writer, list []proto.NetworkRequest) {
	if len(list) == 0 {
		fmt.Fprintln(w, "no requests")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range list {
		status, size, took := strconv.Itoa(r.Status), byteSize(r.Size), duration(r.Duration)
		switch {
		case r.Pending:
			status, size, took = "…", "", "pending"
		case r.Error != "" && r.Status == 0:
			status, size = "failed", ""
		}
		if r.FromCache && !r.Pending {
			size = "cache"
		}
		line := fmt.Sprintf("#%d\t%s\t%s\t%s\t%s\t%s\t%s", r.ID, r.Method, status, r.Type, size, took, r.URL)
		if r.Error != "" {
			line += "  " + r.Error
		}
		fmt.Fprintln(tw, line)
	}
	tw.Flush()
}

func byteSize(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d B", n)
	case n < 1000*1000:
		return fmt.Sprintf("%.1f kB", float64(n)/1000)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/1e6)
}

func duration(ms float64) string {
	if ms < 1000 {
		return fmt.Sprintf("%.0f ms", ms)
	}
	return fmt.Sprintf("%.2f s", ms/1000)
}

// printEmulation prints what a page is shown as.
func printEmulation(w io.Writer, e proto.Emulation) {
	viewport := "the pane's"
	if e.Width > 0 || e.Height > 0 {
		viewport = fmt.Sprintf("%d×%d", e.Width, e.Height)
		if e.Scale > 0 {
			viewport += fmt.Sprintf(" at %g×", e.Scale)
		}
		if e.Mobile {
			viewport += ", mobile"
		}
		if e.Touch {
			viewport += ", touch"
		}
		if e.Device != "" {
			viewport += " (" + e.Device + ")"
		}
	}
	ua := e.UserAgent
	if ua == "" {
		ua = "the browser's"
	}
	network := "as it is"
	switch e.Network {
	case "":
	case "offline":
		network = "offline"
	default:
		network = fmt.Sprintf("%s: %g ms latency, %s/s down, %s/s up", e.Network, e.Latency, byteSize(int64(e.Download)), byteSize(int64(e.Upload)))
	}
	fmt.Fprintf(w, "viewport    %s\nuser agent  %s\nnetwork     %s\n", viewport, ua, network)
}
