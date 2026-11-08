package main

import (
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A development build may tell a line of its own output, as React 18's
// _debugSource does under @vitejs/plugin-react 4.7 (25 for a file of 11
// lines): the tag the element's strings are by is the line the agent gets.
func TestPickSourceLine(t *testing.T) {
	src := `export default function PricingCard({ name, price }) {
  return (
    <div className="card rounded-lg p-4 shadow">
      <h3>{name}</h3>
      <p className="price text-2xl font-bold">{price}</p>
      <button className="btn btn-primary px-4 py-2" data-testid="buy">
        Buy now
      </button>
      <button className="btn ghost">Details</button>
    </div>
  )
}
`
	dir := t.TempDir()
	file := filepath.Join(dir, "src", "PricingCard.jsx")
	os.MkdirAll(filepath.Dir(file), 0o755)
	os.WriteFile(file, []byte(src), 0o644)
	el := &pickedElem{Tag: "button", Hints: []pickHint{
		{Kind: "class", Value: "btn btn-primary px-4 py-2", Count: 3},
		{Kind: "text", Value: "Buy now", Count: 3},
	}}
	for _, tc := range []struct {
		name string
		f    pickFramework
		want string
	}{
		// Past the file's end, and on a line of another tag: the button
		// its strings are by, not the other button.
		{"wrong line", pickFramework{Framework: "react", File: file, Line: 25}, "src/PricingCard.jsx:6 (React _debugSource, which told line 25"},
		{"other tag's line", pickFramework{Framework: "react", File: file, Line: 5}, "src/PricingCard.jsx:6 "},
		// A line a source map told, from the dev server's root.
		{"mapped", pickFramework{Framework: "react", File: "/src/PricingCard.jsx", Line: 6, Mapped: true}, "src/PricingCard.jsx:6 (React, from the source map of its stack)"},
	} {
		el.Frameworks = []pickFramework{tc.f}
		got := pickSources(el, "http://localhost:5173/", []string{dir})
		if len(got) == 0 || !strings.HasPrefix(got[0], tc.want) {
			t.Errorf("%s: %q, want it to start %q", tc.name, got, tc.want)
		}
	}

	// Vue tells the components' files, not lines: the tag is looked for
	// from the innermost out, past a component that only slots it in.
	inner := filepath.Join(dir, "Card.vue")
	os.WriteFile(inner, []byte("<template>\n  <div class=\"card\"><slot /></div>\n</template>\n"), 0o644)
	app := filepath.Join(dir, "App.vue")
	os.WriteFile(app, []byte("<template>\n  <Card>\n    <h2>Plans</h2>\n    <button class=\"buy\">Buy</button>\n  </Card>\n</template>\n"), 0o644)
	vue := pickFramework{Framework: "vue"}
	vue.Chain = append(vue.Chain, struct {
		Name string `json:"name"`
		File string `json:"file"`
	}{"Card", inner}, struct {
		Name string `json:"name"`
		File string `json:"file"`
	}{"App", app})
	btn := &pickedElem{Tag: "button", Frameworks: []pickFramework{vue}, Hints: []pickHint{{Kind: "class", Value: "buy", Count: 1}}}
	if got := pickSources(btn, "http://localhost:5173/", []string{dir}); len(got) == 0 || !strings.HasPrefix(got[0], "App.vue:4 (Vue component App") {
		t.Errorf("slotted element: %q", got)
	}
}

// A page of a file gets its element's line in it, counted by start tags
// as the page has them: not those of comments, scripts or titles.
func TestPickHTMLLine(t *testing.T) {
	page := `<!doctype html>
<html><head><title>a <button> in a title</title>
<script>document.write("<button>")</script>
</head>
<body>
  <!-- <button>old</button> -->
  <button class="pick">Pick</button>
  <div>
    <button class="pick">Pick</button>
  </div>
</body></html>
`
	dir := t.TempDir()
	path := filepath.Join(dir, "page.html")
	os.WriteFile(path, []byte(page), 0o644)
	el := &pickedElem{Tag: "button", Index: 1, Count: 2}
	got := pickSources(el, "file://"+filepath.ToSlash(path), []string{dir})
	if len(got) != 1 || got[0] != "page.html:9 (its start tag in the page's file)" {
		t.Errorf("%q", got)
	}
	// A script added a button: the count is off, and the line a guess by
	// its strings, told as one.
	el = &pickedElem{Tag: "button", Index: 0, Count: 3, Hints: []pickHint{{Kind: "class", Value: "pick", Count: 2}}}
	if line, exact := htmlLine(page, el); line != 7 || exact {
		t.Errorf("changed page: line %d, exact %v", line, exact)
	}
}

// The agent reads where the element is in the source and what to grep
// for before the page's selector, and which hints are the element's alone.
func TestPickMessage(t *testing.T) {
	res := &pickResult{URL: "http://localhost:5173/"}
	res.Viewport.W, res.Viewport.H = 1280, 720
	res.Picks = []pickedElem{{
		Note: "make it green", Tag: "button", Text: "Buy now", Selector: "div.card:nth-of-type(2) > button",
		Hints:   []pickHint{{Kind: "class", Value: "btn px-4", Count: 3}, {Kind: "aria-label", Value: "Buy Pro", Count: 1}},
		Context: "in <main>, the 2nd of 3 div.card",
		sources: []string{"src/Card.vue:8 (Vue component Card)"},
	}}
	res.pageShot = "/tmp/kero-pick-p1-page.png"
	msg := pickMessage(res, nil)
	order := []string{"open them to see", "Page screenshot (each element boxed in blue and numbered as below): /tmp/kero-pick-p1-page.png", "Note: make it green", "Source: src/Card.vue:8", "Search hints", `aria-label="Buy Pro" ★`, "Where: in <main>", "Selector: div.card", "Page: http://localhost:5173/, viewport 1280×720"}
	at := 0
	for _, s := range order {
		i := strings.Index(msg[at:], s)
		if i < 0 {
			t.Fatalf("%q missing or out of order in:\n%s", s, msg)
		}
		at += i
	}
	if !strings.Contains(msg, `class="btn px-4" (3 on the page)`) || strings.Contains(msg, `px-4" ★`) {
		t.Errorf("a class three elements share is marked unique:\n%s", msg)
	}
	if strings.Contains(msg, "Source location: none") {
		t.Errorf("an element with a source location is told it has none:\n%s", msg)
	}

	// With no source location, the agent is told so, before the hints,
	// to grep them rather than look for one; of a host's page, that the
	// source was not looked in.
	res.Picks[0].sources = nil
	msg = pickMessage(res, nil)
	if i, j := strings.Index(msg, "Source location: none"), strings.Index(msg, "Search hints"); i < 0 || j < i {
		t.Errorf("no source location, untold or told after the hints:\n%s", msg)
	}
	res.remote = true
	if msg = pickMessage(res, nil); !strings.Contains(msg, "Source location: not looked up") {
		t.Errorf("a host's page:\n%s", msg)
	}
}

// The picker's screenshots go a day after they were taken, as do those
// older builds left in the temporary directory itself; newer ones, and
// what else is there, stay.
func TestPrunePickShots(t *testing.T) {
	tmp := t.TempDir()
	now := time.Now()
	os.MkdirAll(filepath.Join(tmp, pickShotDir), 0o755)
	file := func(name string, age time.Duration) string {
		p := filepath.Join(tmp, name)
		os.WriteFile(p, []byte("png"), 0o644)
		os.Chtimes(p, now.Add(-age), now.Add(-age))
		return p
	}
	old := file(filepath.Join(pickShotDir, "p1-20261101-page.png"), 25*time.Hour)
	legacy := file("kero-pick-p1-20261101-1.png", 48*time.Hour)
	fresh := file(filepath.Join(pickShotDir, "p1-20261108-1.png"), 23*time.Hour)
	other := file("someone-else.png", 72*time.Hour)
	prunePickShots(tmp, now)
	for p, want := range map[string]bool{old: false, legacy: false, fresh: true, other: true} {
		if _, err := os.Stat(p); (err == nil) != want {
			t.Errorf("%s: kept %v, want %v", filepath.Base(p), err == nil, want)
		}
	}
}

// An element's picture is the page's around it, boxed where it is, in
// the picture's pixels, not the page's.
func TestPickHighlight(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	// 20×10 at 100,50 in CSS pixels, at 2x: 200,100 to 240,120.
	out := highlight(img, pickRect{X: 100, Y: 50, W: 20, H: 10}, 2)
	if out == nil {
		t.Fatal("no picture")
	}
	// 24 CSS pixels of margin: the crop starts at 152,52.
	if b := out.Bounds(); b.Dx() != 40+96 || b.Dy() != 20+96 {
		t.Errorf("crop %v", b)
	}
	if c := out.RGBAAt(48-2, 48+5); c != pickAccent {
		t.Errorf("left edge %v, want the box", c)
	}
	if c := out.RGBAAt(48+5, 48+5); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("inside %v, want the page", c)
	}
	if highlight(img, pickRect{X: 500, Y: 50, W: 20, H: 10}, 2) != nil {
		t.Error("an element out of sight has a picture")
	}

	// The page's picture boxes the elements in sight, numbered; one the
	// page scrolled away from is scrolled to for its own picture.
	picks := []pickedElem{{Rect: pickRect{X: 100, Y: 50, W: 20, H: 10}}, {Rect: pickRect{X: 10, Y: -400, W: 20, H: 10}}}
	page := pagePicture(img, picks, 2)
	if c := page.RGBAAt(200-2, 110); c != pickAccent {
		t.Errorf("the page's picture: element 1's edge %v", c)
	}
	// The badge "1" sits above the box, a white digit on blue, where
	// nothing else is drawn: its 15 font pixels at 2x, 30 tall.
	blue, white := 0, 0
	for y := 66; y < 95; y++ {
		for x := 196; x < 220; x++ {
			switch page.RGBAAt(x, y) {
			case pickAccent:
				blue++
			case color.RGBA{255, 255, 255, 255}:
				white++
			}
		}
	}
	if blue < 200 || white == 0 {
		t.Errorf("no badge above element 1: %d blue, %d white pixels", blue, white)
	}
	if img.RGBAAt(198, 96) == pickAccent {
		t.Error("the page's picture drew on the capture itself")
	}
	for _, tc := range []struct {
		r    pickRect
		want bool
	}{
		{picks[0].Rect, true},
		{picks[1].Rect, false},
		{pickRect{X: 10, Y: 290, W: 20, H: 20}, false},  // cut off at the bottom
		{pickRect{X: 0, Y: -100, W: 200, H: 900}, true}, // taller than the viewport
	} {
		if got := inSight(tc.r, 200, 300); got != tc.want {
			t.Errorf("inSight(%v) = %v", tc.r, got)
		}
	}
}
