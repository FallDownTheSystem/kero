package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"

	"kero/internal/proto"

	"kero/locales"
)

// Pick mode of a browser pane: the user clicks elements of the page,
// writes a note on each, and sends them to the agent that opened the page,
// as CSS Studio does, without its editing. What the agent most needs is
// where each element is in the source, which a CSS selector does not tell:
// the message leads with the source's file and line where the page or its
// file can tell them, then the strings to grep for, then where the
// element is on the page, its selector last.

// pickScript is the picker the page runs (browserpick.js): it shows the
// user what they point at, takes their picks and notes, and ends with
// what each element is, or null when they stop.
//
//go:embed browserpick.js
var pickScript string

// pickLabel names a picker's message to the agent: kerod adds the page.
const pickLabel = "element picks"

// pickResult is what the picker ends with, as browserpick.js's collect.
type pickResult struct {
	URL      string `json:"url"`
	Title    string `json:"title"`
	Viewport struct {
		W   float64 `json:"w"`
		H   float64 `json:"h"`
		DPR float64 `json:"dpr"`
	} `json:"viewport"`
	// Screenshots asks for a picture of each element, PageShot for one
	// of the page as the user saw it; Scroll is where the page was.
	Screenshots bool                   `json:"screenshots"`
	PageShot    bool                   `json:"pageShot"`
	Scroll      struct{ X, Y float64 } `json:"scroll"`
	Picks       []pickedElem           `json:"picks"`

	// pageShot is the file of the page's picture; remote, that the page's
	// session runs on a host, whose source the app does not look in.
	pageShot string
	remote   bool
}

type pickedElem struct {
	Note       string          `json:"note"`
	Tag        string          `json:"tag"`
	Text       string          `json:"text"`
	Frameworks []pickFramework `json:"frameworks"`
	Attrs      []pickAttr      `json:"attrs"`
	// Index is which of the page's elements of its tag it is, of Count.
	Index    int               `json:"index"`
	Count    int               `json:"count"`
	ID       string            `json:"id"`
	Class    string            `json:"class"`
	Hints    []pickHint        `json:"hints"`
	Context  string            `json:"context"`
	Skeleton string            `json:"skeleton"`
	HTML     string            `json:"html"`
	Selector string            `json:"selector"`
	Path     string            `json:"path"`
	Rect     pickRect          `json:"rect"`
	Styles   map[string]string `json:"styles"`

	// What the app finds of it: where it is in the source, and the
	// file of its picture.
	sources []string
	shot    string
}

// pickFramework is what a framework's development build tells of the
// element: the components it is in, innermost first, and, where it
// knows, the file and line it was written at.
type pickFramework struct {
	Framework string `json:"framework"`
	Chain     []struct {
		Name string `json:"name"`
		File string `json:"file"`
	} `json:"chain"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	// Mapped tells the line came of a source map (React 19's stack).
	Mapped bool `json:"mapped"`
	// Of is the ancestor's tag the place is of, "" for the element's own.
	Of string `json:"of"`
}

// pickAttr is an attribute a build plugin adds to tell the source, as
// data-v-inspector, of the element or the ancestor Of.
type pickAttr struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Of    string `json:"of"`
}

// pickHint is a string likely written in the source as it is: a
// class attribute, an id, a label, the element's text; Count is how many
// of the page's elements have it.
type pickHint struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Count int    `json:"count"`
}

type pickRect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// pick turns a browser pane's Pick mode on, or off when it is, from the
// pane's button, the menus or the palette.
func (a *App) pick(id string) {
	if id == "" {
		a.toast(locales.PickWhichBrowser())
		return
	}
	if err := a.togglePick(id); err != nil {
		a.toast(locales.PickFailed(err.Error()))
	}
}

// togglePick turns a browser pane's Pick mode on, or off when it is.
func (a *App) togglePick(id string) error {
	e := a.webs[id]
	if e == nil || e.view == nil {
		return fmt.Errorf("pane %s shows no page", id)
	}
	if e.url == proto.BlankURL {
		return errors.New("the pane shows no page yet")
	}
	page := e.view.Page()
	if e.picking {
		e.picking = false
		e.pickSeq++
		go page.Eval("window.__keroPick && window.__keroPick.stop()")
		return nil
	}
	e.picking = true
	e.pickSeq++
	seq := e.pickSeq
	// The pictures are taken as the user last chose.
	opts, _ := json.Marshal(map[string]any{"screenshots": !a.pickNoShots, "page": !a.pickNoPage})
	script := "(" + pickScript + ")(" + string(opts) + ")"
	go func() {
		// The picker waits for the user: as long as they take.
		v, err := page.EvalContext(context.Background(), script)
		a.update(func() {
			if a.webs[id] != e || e.pickSeq != seq {
				return // stopped, or the page went on
			}
			e.picking = false
			if err != nil {
				var js *mygo.EvalError
				if errors.As(err, &js) {
					a.toast(locales.PickFailed(js.Message))
				}
				return
			}
			if v == nil {
				return // the user stopped
			}
			var res pickResult
			b, _ := json.Marshal(v)
			if json.Unmarshal(b, &res) != nil || len(res.Picks) == 0 {
				return
			}
			a.pickNoShots, a.pickNoPage = !res.Screenshots, !res.PageShot
			a.sendPicks(id, e, &res)
		})
	}()
	return nil
}

// pickPane is the browser pane Pick Elements is for: the one the user is
// in, else the session's only one.
func (a *App) pickPane() string {
	s := a.session(a.selected)
	if s == nil {
		return ""
	}
	if id := a.focusOf(s); a.isBrowser(id) {
		return id
	}
	found := ""
	for _, p := range s.Panes {
		if p.Browser() {
			if found != "" {
				return ""
			}
			found = p.ID
		}
	}
	return found
}

// sendPicks finds the picks in the source, takes their pictures, and
// sends them to the page's agent as the page bridge sends its messages
// (pageSend): to the agent that opened the page, else the one the user
// chose for it, else one they choose now. Off the main thread.
func (a *App) sendPicks(id string, e *webEntry, res *pickResult) {
	// The source is on the disk of the machine the session runs on, and
	// the agent reads its pictures there: a host's sessions get neither.
	res.remote = a.hostOf(id).remote()
	dirs := a.pickDirs(id)
	view := e.view
	go func() {
		if !res.remote {
			for i := range res.Picks {
				res.Picks[i].sources = pickSources(&res.Picks[i], res.URL, dirs)
			}
			if view != nil {
				pickShots(view, res, id)
			}
		}
		text := pickMessage(res, dirs)
		sent, err := a.pageSend(context.Background(), id, PageMessage{Text: text, Label: pickLabel})
		a.update(func() {
			n := len(res.Picks)
			switch {
			case err != nil && sent.State == "" && strings.Contains(err.Error(), "no agent chosen"):
				// The user chose no agent: they paste the picks where
				// they will.
				if mygo.Clipboard.Write(transfer.TextData(text)) == nil {
					a.toast(locales.PickCopied(n))
				} else {
					a.toast(locales.PickNoAgent())
				}
			case err != nil:
				a.toast(locales.PickFailed(err.Error()))
			case sent.State == "waiting":
				a.toast(locales.PickKept(n))
			case sent.State == "queued":
				a.toast(locales.PickQueued(n, a.agentName(sent.To)))
			default:
				a.toast(locales.PickSent(n, a.agentName(sent.To)))
			}
		})
	}()
}

// agentName is the name of the agent in a pane, as the user knows it.
func (a *App) agentName(id string) string {
	if p := a.paneByID(id); p != nil && proto.AgentNames[p.Agent] != "" {
		return proto.AgentNames[p.Agent]
	}
	return locales.GitAnAgent()
}

// pickDirs are where a page's source may be, to find a path a dev server
// tells from its root in: the opener's folder, the session's agents',
// then its project's.
func (a *App) pickDirs(id string) []string {
	var dirs []string
	add := func(d string) {
		if d != "" && !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	s := a.sessionOfPane(id)
	if s == nil {
		return nil
	}
	if p := s.PaneByID(id); p != nil && p.Opener != "" {
		if o := a.paneByID(p.Opener); o != nil {
			add(o.WorkDir())
		}
	}
	for _, p := range s.Panes {
		if p.Agent != "" {
			add(p.WorkDir())
		}
	}
	for _, p := range s.Panes {
		if p.Terminal() {
			add(p.WorkDir())
		}
	}
	for _, pr := range a.st.Projects {
		if pr.ID == s.Project {
			add(pr.Dir)
		}
	}
	return dirs
}

// pickSources are the places of the element in the source, strongest
// first, as "file:line (how it is known)": what a framework's
// development build tells, checked against the file, the attributes an
// inspector plugin adds, and for a page of a file, its line in it.
func pickSources(el *pickedElem, pageURL string, dirs []string) []string {
	var out []string
	for _, f := range el.Frameworks {
		name := frameworkName(f.Framework)
		if f.File != "" && f.Line > 0 {
			how := name + " dev build"
			switch {
			case f.Mapped:
				how = name + ", from the source map of its stack"
			case f.Framework == "react":
				how = "React _debugSource"
			}
			if f.Of != "" {
				how += ", of the <" + f.Of + "> around it"
			}
			path, ok := resolveSource(f.File, dirs)
			line := f.Line
			if ok && f.Of == "" {
				if text, err := os.ReadFile(path); err == nil {
					// A build may tell a line of its own output: the file
					// says where the tag is.
					switch fixed, sure := findTag(string(text), el.Tag, el.Hints, f.Line); {
					case fixed == f.Line:
					case fixed > 0 && sure:
						how += fmt.Sprintf(", which told line %d; the tag is on this one in the file", f.Line)
						line = fixed
					default:
						how += ", unchecked: the file has no such tag on that line"
					}
				}
			}
			out = append(out, fmt.Sprintf("%s:%d (%s)", showPath(path, dirs), line, how))
			continue
		}
		// A component's file with no line (Vue): the tag is found in it,
		// in the innermost component's file that has it.
		for _, c := range f.Chain {
			if c.File == "" {
				continue
			}
			path, ok := resolveSource(c.File, dirs)
			if !ok {
				out = append(out, fmt.Sprintf("%s (%s component %s)", showPath(path, dirs), name, c.Name))
				break
			}
			text, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if line, sure := findTag(string(text), el.Tag, el.Hints, 0); line > 0 && sure {
				out = append(out, fmt.Sprintf("%s:%d (%s component %s; the line found by the tag and its attributes)", showPath(path, dirs), line, name, c.Name))
				break
			}
			// Not written in this component's own template: a slot's,
			// written in the one around it.
		}
	}
	for _, at := range el.Attrs {
		s := at.Name + "=" + at.Value
		if at.Of != "" {
			s += " (on the <" + at.Of + "> around it)"
		}
		out = append(out, s)
	}
	if path, ok := proto.URLFile(pageURL); ok {
		if text, err := os.ReadFile(path); err == nil {
			if line, exact := htmlLine(string(text), el); line > 0 {
				how := "its start tag in the page's file"
				if !exact {
					how = "likely; the page's scripts changed it since it loaded"
				}
				out = append(out, fmt.Sprintf("%s:%d (%s)", showPath(path, dirs), line, how))
			}
		}
	}
	return out
}

func frameworkName(f string) string {
	switch f {
	case "vue":
		return "Vue"
	case "react":
		return "React"
	case "svelte":
		return "Svelte"
	}
	return f
}

// resolveSource finds a file a page tells on the disk: as it is, or, as
// a dev server tells it from its root (/src/App.jsx), in one of dirs.
func resolveSource(file string, dirs []string) (string, bool) {
	file, _, _ = strings.Cut(file, "?")
	file = strings.TrimPrefix(file, "file://")
	file = strings.TrimPrefix(file, "/@fs")
	if len(file) >= 3 && file[0] == '/' && file[2] == ':' {
		file = file[1:] // /C:/x, Windows' C:/x
	}
	if filepath.IsAbs(filepath.FromSlash(file)) {
		if st, err := os.Stat(file); err == nil && !st.IsDir() {
			return filepath.Clean(filepath.FromSlash(file)), true
		}
	}
	rel := strings.TrimPrefix(filepath.FromSlash(file), string(filepath.Separator))
	for _, d := range dirs {
		p := filepath.Join(d, rel)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, true
		}
	}
	return file, false
}

// showPath writes a path from the first of dirs, the agent's folder, when
// it is in it, as the agent knows its files.
func showPath(path string, dirs []string) string {
	if len(dirs) > 0 && filepath.IsAbs(path) {
		if rel, err := filepath.Rel(dirs[0], path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return path
}

// findTag finds the line of an element's start tag in a source file:
// near's, when the tag is on it; else that of the tag the element's
// strings are written by, the one nearest near among those alike. sure
// tells the tag was there, or its strings were by it, or it is the
// file's only one; 0 for no such tag.
func findTag(text, tag string, hints []pickHint, near int) (line int, sure bool) {
	lines := strings.Split(text, "\n")
	open := regexp.MustCompile(`(?i)<` + regexp.QuoteMeta(tag) + `([\s/>]|$)`)
	if near > 0 && near <= len(lines) && open.MatchString(lines[near-1]) {
		return near, true
	}
	var tags []int
	for i, l := range lines {
		if open.MatchString(l) {
			tags = append(tags, i+1)
		}
	}
	if len(tags) == 0 {
		return 0, false
	}
	best, bestScore := 0, -1
	for _, n := range tags {
		// The tag's attributes and text may run over the lines after it.
		window := strings.Join(lines[n-1:min(n+4, len(lines))], "\n")
		score := 0
		for _, h := range hints {
			if h.Value != "" && strings.Contains(window, h.Value) {
				score++
				if h.Count == 1 {
					score += 2 // a string of it alone
				}
			}
		}
		if score > bestScore || score == bestScore && near > 0 && lineDist(n, near) < lineDist(best, near) {
			best, bestScore = n, score
		}
	}
	return best, bestScore > 0 || len(tags) == 1
}

func lineDist(a, b int) int {
	if a < b {
		return b - a
	}
	return a - b
}

// htmlLine is the line of a page's element in its file: the start tag of
// its tag that is its index among them, as the page has as many of them
// as the file does; else, its scripts having changed the page, the one
// its strings are by. exact tells it was found by its index.
func htmlLine(text string, el *pickedElem) (line int, exact bool) {
	var at []int
	for _, t := range startTags(text) {
		if t.name == el.Tag {
			at = append(at, t.line)
		}
	}
	if len(at) == el.Count && el.Index >= 0 && el.Index < len(at) {
		return at[el.Index], true
	}
	if len(at) == 0 {
		return 0, false
	}
	// Among the file's tags alike, the one its strings are by.
	lines := strings.Split(text, "\n")
	best, bestScore := 0, 0
	for _, n := range at {
		window := strings.Join(lines[n-1:min(n+4, len(lines))], "\n")
		score := 0
		for _, h := range el.Hints {
			if h.Value != "" && strings.Contains(window, h.Value) {
				score++
				if h.Count == 1 {
					score += 2
				}
			}
		}
		if score > bestScore {
			best, bestScore = n, score
		}
	}
	return best, false
}

type startTag struct {
	name string
	line int
}

var tagName = regexp.MustCompile(`^<([a-zA-Z][a-zA-Z0-9-]*)`)

// startTags are an HTML file's start tags, in order, with their lines:
// not those in comments, nor the text of script, style, textarea and
// title, which the page has no elements of.
func startTags(text string) []startTag {
	var out []startTag
	line := 1
	for i := 0; i < len(text); {
		j := strings.IndexByte(text[i:], '<')
		if j < 0 {
			break
		}
		line += strings.Count(text[i:i+j], "\n")
		i += j
		rest := text[i:]
		if strings.HasPrefix(rest, "<!--") {
			end := strings.Index(rest, "-->")
			if end < 0 {
				break
			}
			line += strings.Count(rest[:end], "\n")
			i += end + 3
			continue
		}
		m := tagName.FindStringSubmatch(rest)
		if m == nil {
			i++
			continue
		}
		name := strings.ToLower(m[1])
		out = append(out, startTag{name, line})
		i += len(m[0])
		switch name {
		case "script", "style", "textarea", "title":
			end := strings.Index(strings.ToLower(text[i:]), "</"+name)
			if end < 0 {
				return out
			}
			line += strings.Count(text[i:i+end], "\n")
			i += end
		}
	}
	return out
}

// pickShots takes the pictures the user asked for, to files of the
// temporary directory the agent opens by their paths: the page as they
// saw it, each element boxed and numbered, and each element with a
// margin around it, boxed. An element out of sight is scrolled into it
// for its picture, and the page scrolled back after.
func pickShots(view *mygo.WebView, res *pickResult, pane string) {
	page := view.Page()
	moved := false
	defer func() {
		restore := ""
		if moved {
			restore = fmt.Sprintf("scrollTo({left: %g, top: %g, behavior: \"instant\"});", res.Scroll.X, res.Scroll.Y)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		page.EvalContext(ctx, restore+"delete window.__keroPicked")
	}()
	if !res.Screenshots && !res.PageShot || res.Viewport.W <= 0 {
		return
	}
	dir := filepath.Join(os.TempDir(), pickShotDir)
	prunePickShots(os.TempDir(), time.Now())
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	// The picker's boxes are gone from the page by its next frame.
	time.Sleep(150 * time.Millisecond)
	img, err := capture(view)
	if err != nil {
		return
	}
	scale := float64(img.Bounds().Dx()) / res.Viewport.W
	stamp := time.Now().Format("20060102-150405")
	save := func(img image.Image, name string) string {
		var buf bytes.Buffer
		if png.Encode(&buf, img) != nil {
			return ""
		}
		path := filepath.Join(dir, fmt.Sprintf("%s-%s-%s.png", pane, stamp, name))
		if os.WriteFile(path, buf.Bytes(), 0o644) != nil {
			return ""
		}
		return path
	}
	if res.PageShot {
		res.pageShot = save(pagePicture(img, res.Picks, scale), "page")
	}
	if !res.Screenshots {
		return
	}
	for i := range res.Picks {
		el := &res.Picks[i]
		src, at := img, el.Rect
		if !inSight(at, res.Viewport.W, res.Viewport.H) {
			r, ok := scrollToPick(page, i)
			if !ok {
				continue
			}
			moved = true
			time.Sleep(200 * time.Millisecond) // for the page to paint
			if src, err = capture(view); err != nil {
				continue
			}
			at = r
		}
		if out := highlight(src, at, scale); out != nil {
			el.shot = save(out, strconv.Itoa(i+1))
		}
	}
}

// pickShotDir is the folder of the temporary directory the picker's
// screenshots go to, and pickShotAge how long they stay there: long
// enough for an agent to read them, from a message it got late.
const (
	pickShotDir = "kero-picks"
	pickShotAge = 24 * time.Hour
)

// prunePickShots deletes the picker's screenshots older than pickShotAge,
// of tmp's pickShotDir, and those older Kero builds left in tmp itself
// (kero-pick-*.png), as a pick takes new ones and the app starts.
func prunePickShots(tmp string, now time.Time) {
	old, _ := filepath.Glob(filepath.Join(tmp, pickShotDir, "*.png"))
	legacy, _ := filepath.Glob(filepath.Join(tmp, "kero-pick-*.png"))
	for _, f := range append(old, legacy...) {
		if st, err := os.Stat(f); err == nil && now.Sub(st.ModTime()) > pickShotAge {
			os.Remove(f)
		}
	}
}

// capture is the picture of the page the view shows.
func capture(view *mygo.WebView) (image.Image, error) {
	b, err := view.CapturePage()
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return nil, errors.New("no picture")
	}
	return png.Decode(bytes.NewReader(b))
}

// inSight tells that an element's box is in the viewport, as much of it
// as fits there.
func inSight(r pickRect, vw, vh float64) bool {
	fitsX := r.X >= -1 && r.X+r.W <= vw+1 || r.W > vw && r.X < vw && r.X+r.W > 0
	fitsY := r.Y >= -1 && r.Y+r.H <= vh+1 || r.H > vh && r.Y < vh && r.Y+r.H > 0
	return r.W > 0 && r.H > 0 && fitsX && fitsY
}

// scrollToPick scrolls the page's i'th picked element into sight and
// returns where it is now.
func scrollToPick(page *mygo.Page, i int) (pickRect, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	v, err := page.EvalContext(ctx, fmt.Sprintf(`(() => {
  const el = (window.__keroPicked || [])[%d];
  if (!el || !el.isConnected) return null;
  el.scrollIntoView({ block: "center", inline: "center", behavior: "instant" });
  const r = el.getBoundingClientRect();
  return { x: r.left, y: r.top, w: r.width, h: r.height };
})()`, i))
	if err != nil || v == nil {
		return pickRect{}, false
	}
	var r pickRect
	b, _ := json.Marshal(v)
	return r, json.Unmarshal(b, &r) == nil && r.W > 0 && r.H > 0
}

// pickAccent is the picker's blue, as browserpick.js draws its boxes.
var pickAccent = color.RGBA{0x2f, 0x80, 0xed, 0xff}

// highlight is the part of the page's picture around an element's box
// (r, in CSS pixels, scale picture pixels each), with the box drawn: nil
// for an element out of sight.
func highlight(img image.Image, r pickRect, scale float64) *image.RGBA {
	b := img.Bounds()
	box := scaled(r, scale).Add(b.Min)
	pad := int(24 * scale)
	area := image.Rect(box.Min.X-pad, box.Min.Y-pad, box.Max.X+pad, box.Max.Y+pad).Intersect(b)
	if area.Empty() || box.Intersect(b).Empty() {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, area.Dx(), area.Dy()))
	draw.Draw(out, out.Bounds(), img, area.Min, draw.Src)
	drawBox(out, box.Sub(area.Min), scale)
	return out
}

// pagePicture is the page's picture with each picked element in it boxed
// and numbered, as the message numbers them.
func pagePicture(img image.Image, picks []pickedElem, scale float64) *image.RGBA {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	for i, el := range picks {
		box := scaled(el.Rect, scale)
		if box.Intersect(out.Bounds()).Empty() {
			continue
		}
		drawBox(out, box, scale)
		badge(out, i+1, box, scale)
	}
	return out
}

// scaled is a box of CSS pixels in the picture's.
func scaled(r pickRect, scale float64) image.Rectangle {
	return image.Rect(int(r.X*scale), int(r.Y*scale), int((r.X+r.W)*scale+0.5), int((r.Y+r.H)*scale+0.5))
}

// drawBox draws a box around box, 2 CSS pixels wide: outside it where the
// picture has room, inside it on a side that touches the picture's edge,
// as an element at the page's top or side does, so that every side shows.
// A side past the picture's edge, of an element that goes on beyond it,
// has no line.
func drawBox(out *image.RGBA, box image.Rectangle, scale float64) {
	w := max(int(2*scale), 2)
	b := out.Bounds()
	// The box's outer edge on each side, and whether the side is drawn.
	o := box
	side := func(edge, limit int, outward int) (int, bool) {
		room := (limit - edge) * outward
		switch {
		case room >= w:
			return edge + w*outward, true
		case room > -w: // on the picture's edge: inside, from it
			if outward < 0 {
				return max(edge, limit), true
			}
			return min(edge, limit), true
		}
		return edge, false
	}
	var top, bottom, left, right bool
	o.Min.Y, top = side(box.Min.Y, b.Min.Y, -1)
	o.Max.Y, bottom = side(box.Max.Y, b.Max.Y, 1)
	o.Min.X, left = side(box.Min.X, b.Min.X, -1)
	o.Max.X, right = side(box.Max.X, b.Max.X, 1)
	edge := image.NewUniform(pickAccent)
	for _, s := range []struct {
		r  image.Rectangle
		on bool
	}{
		{image.Rect(o.Min.X, o.Min.Y, o.Max.X, o.Min.Y+w), top},
		{image.Rect(o.Min.X, o.Max.Y-w, o.Max.X, o.Max.Y), bottom},
		{image.Rect(o.Min.X, o.Min.Y, o.Min.X+w, o.Max.Y), left},
		{image.Rect(o.Max.X-w, o.Min.Y, o.Max.X, o.Max.Y), right},
	} {
		if s.on {
			draw.Draw(out, s.r.Intersect(b), edge, image.Point{}, draw.Src)
		}
	}
}

// badge writes an element's number, white on the picker's blue, at its
// box's top left corner, inside the picture.
func badge(out *image.RGBA, n int, box image.Rectangle, scale float64) {
	text := strconv.Itoa(n)
	face := basicfont.Face7x13
	// The font's pixels drawn k times as big: about 13 CSS pixels tall.
	k := max(int(scale+0.5), 1)
	tw := font.MeasureString(face, text).Ceil()
	small := image.NewRGBA(image.Rect(0, 0, tw+6, 15))
	draw.Draw(small, small.Bounds(), image.NewUniform(pickAccent), image.Point{}, draw.Src)
	d := font.Drawer{Dst: small, Src: image.NewUniform(color.White), Face: face, Dot: fixed.P(3, 12)}
	d.DrawString(text)
	sz := image.Pt(small.Bounds().Dx()*k, small.Bounds().Dy()*k)
	at := image.Pt(box.Min.X-2*k, box.Min.Y-sz.Y-2*k)
	if at.Y < 0 {
		at.Y = box.Min.Y + 2*k // no room above: inside the box
	}
	at.X = max(0, min(at.X, out.Bounds().Dx()-sz.X))
	for y := range sz.Y {
		for x := range sz.X {
			if p := at.Add(image.Pt(x, y)); p.In(out.Bounds()) {
				out.SetRGBA(p.X, p.Y, small.RGBAAt(x/k, y/k))
			}
		}
	}
}

// pickStyleOrder is the order the computed styles are told in.
var pickStyleOrder = []string{"display", "position", "color", "background-color", "background-image", "font", "margin", "padding",
	"border", "border-radius", "box-shadow", "flex-direction", "gap", "align-items", "justify-content", "grid-template-columns", "opacity", "z-index"}

// pickMessage is the picks as the agent reads them: each element's note,
// then where it is in the source, the strings to grep for, where it is on
// the page and its HTML, its selector and styles last; the page's URL
// and viewport at the end.
func pickMessage(res *pickResult, dirs []string) string {
	var b strings.Builder
	n := len(res.Picks)
	if n == 1 {
		b.WriteString("The user picked an element on the page and wrote what they want of it. Find it")
	} else {
		fmt.Fprintf(&b, "The user picked %d elements on the page and wrote what they want of each. Find each", n)
	}
	b.WriteString(" in the source by its source location, else by grepping for its search hints; the selector is the page's, not the source's.")
	if res.pageShot != "" || slices.ContainsFunc(res.Picks, func(el pickedElem) bool { return el.shot != "" }) {
		b.WriteString(" The screenshots are image files: open them to see what the user saw.")
	}
	b.WriteString("\n")
	if res.pageShot != "" {
		b.WriteString("Page screenshot (each element boxed in blue and numbered as below): " + res.pageShot + "\n")
	}
	for i := range res.Picks {
		el := &res.Picks[i]
		title := "<" + el.Tag + ">"
		if el.Text != "" {
			title += " " + quoteText(el.Text)
		}
		fmt.Fprintf(&b, "\n## %d. %s\n", i+1, title)
		if el.Note != "" {
			b.WriteString("Note: " + strings.ReplaceAll(el.Note, "\n", "\n  ") + "\n")
		} else {
			b.WriteString("Note: (none)\n")
		}
		switch {
		case len(el.sources) > 0:
		case res.remote:
			b.WriteString("Source location: not looked up (the page's session runs on another machine); grep the search hints\n")
		default:
			b.WriteString("Source location: none (not a development build Kero reads, nor a local file); grep the search hints\n")
		}
		for j, s := range el.sources {
			if j == 0 {
				b.WriteString("Source: " + s + "\n")
			} else {
				b.WriteString("  also: " + s + "\n")
			}
		}
		if chain := componentChain(el, dirs); chain != "" {
			b.WriteString("Components: " + chain + "\n")
		}
		if len(el.Hints) > 0 {
			b.WriteString("Search hints (★ unique on the page):\n")
			for _, h := range el.Hints {
				mark := ""
				if h.Count == 1 {
					mark = " ★"
				} else if h.Count > 1 {
					mark = fmt.Sprintf(" (%d on the page)", h.Count)
				}
				if h.Kind == "text" {
					fmt.Fprintf(&b, "  - text %s%s\n", quoteText(h.Value), mark)
				} else {
					fmt.Fprintf(&b, "  - %s=%s%s\n", h.Kind, quoteText(h.Value), mark)
				}
			}
		}
		if el.Context != "" {
			b.WriteString("Where: " + el.Context + "\n")
		}
		if el.Skeleton != "" {
			b.WriteString("In its parent:\n" + indent(el.Skeleton) + "\n")
		}
		if el.HTML != "" {
			b.WriteString("HTML:\n" + indent(el.HTML) + "\n")
		}
		b.WriteString("Selector: " + el.Selector + "\n")
		b.WriteString("DOM path: " + el.Path + "\n")
		fmt.Fprintf(&b, "Box: %.0f×%.0f at %.0f,%.0f\n", el.Rect.W, el.Rect.H, el.Rect.X, el.Rect.Y)
		if len(el.Styles) > 0 {
			var st []string
			seen := map[string]bool{}
			for _, k := range pickStyleOrder {
				if v, ok := el.Styles[k]; ok {
					st, seen[k] = append(st, k+": "+v), true
				}
			}
			var rest []string
			for k := range el.Styles {
				if !seen[k] {
					rest = append(rest, k)
				}
			}
			sort.Strings(rest)
			for _, k := range rest {
				st = append(st, k+": "+el.Styles[k])
			}
			b.WriteString("Styles: " + strings.Join(st, "; ") + "\n")
		}
		if el.shot != "" {
			b.WriteString("Screenshot (the element boxed in blue): " + el.shot + "\n")
		}
	}
	fmt.Fprintf(&b, "\nPage: %s, viewport %.0f×%.0f", res.URL, res.Viewport.W, res.Viewport.H)
	if res.Viewport.DPR > 0 && res.Viewport.DPR != 1 {
		fmt.Fprintf(&b, " at %gx", res.Viewport.DPR)
	}
	b.WriteString("\n")
	return b.String()
}

// componentChain is the components an element is in, innermost first,
// with their files.
func componentChain(el *pickedElem, dirs []string) string {
	var parts []string
	for _, f := range el.Frameworks {
		for _, c := range f.Chain {
			s := c.Name
			if c.File != "" {
				path, _ := resolveSource(c.File, dirs)
				s += " (" + showPath(path, dirs) + ")"
			}
			if s != "" {
				parts = append(parts, s)
			}
			if len(parts) == 8 {
				break
			}
		}
	}
	return strings.Join(parts, " ← ")
}

func quoteText(s string) string {
	b, _ := json.Marshal(s)
	// The agent reads it: <, > and & as they are.
	b = bytes.ReplaceAll(b, []byte(`\u003c`), []byte("<"))
	b = bytes.ReplaceAll(b, []byte(`\u003e`), []byte(">"))
	b = bytes.ReplaceAll(b, []byte(`\u0026`), []byte("&"))
	return string(b)
}

func indent(s string) string {
	return "    " + strings.ReplaceAll(s, "\n", "\n    ")
}
