package main

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui/markdown/parser"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// GitHub renders the HTML of pull requests' text, which bots such as
// Dependabot write whole: release notes as headings and lists, in
// <details> that show only their summary until opened. The markdown view
// shows HTML blocks as they are, so the page splits the text at its
// <details> to show them folded, and turns the HTML of the rest into the
// markdown it means.

// mdPart is a run of markdown, or a <details> of its summary and parts.
type mdPart struct {
	text    string
	details bool
	summary string
	open    bool
	parts   []mdPart
}

var (
	detailsTag  = regexp.MustCompile(`(?i)<(/?)details\b[^>]*>`)
	summaryTag  = regexp.MustCompile(`(?is)^\s*<summary\b[^>]*>(.*?)</summary\s*>`)
	openAttr    = regexp.MustCompile(`(?i)\sopen\b`)
	fenceLine   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
	detailsLine = regexp.MustCompile(`(?i)^ {0,3}<details\b`)
)

// mdParts splits markdown at the <details> that start its lines, as
// HTML blocks do, but not those of code blocks. A <details> not closed
// holds the rest of the text, as a browser reads it.
func mdParts(src string) []mdPart {
	var out []mdPart
	text := func(s string) {
		if strings.TrimSpace(s) != "" {
			out = append(out, mdPart{text: s})
		}
	}
	start, at := 0, 0
	fence := ""
	for at < len(src) {
		end := strings.IndexByte(src[at:], '\n') + 1
		if end == 0 {
			end = len(src) - at
		}
		line := src[at : at+end]
		if fence != "" {
			if strings.HasPrefix(strings.TrimLeft(line, " "), fence) {
				fence = ""
			}
			at += end
			continue
		}
		if m := fenceLine.FindStringSubmatch(line); m != nil {
			fence = m[1][:3]
			at += end
			continue
		}
		if !detailsLine.MatchString(line) {
			at += end
			continue
		}
		text(src[start:at])
		d, next := detailsAt(src, at)
		out = append(out, d)
		start, at = next, next
	}
	text(src[start:])
	return out
}

// detailsAt reads the <details> at i of src, and returns it with where
// the text after it starts.
func detailsAt(src string, i int) (mdPart, int) {
	open := detailsTag.FindStringIndex(src[i:])
	tag := src[i+open[0] : i+open[1]]
	body, next := src[i+open[1]:], len(src)
	depth := 1
	for _, m := range detailsTag.FindAllStringSubmatchIndex(body, -1) {
		if m[3] > m[2] {
			depth--
		} else {
			depth++
		}
		if depth == 0 {
			next = i + open[1] + m[1]
			body = body[:m[0]]
			break
		}
	}
	d := mdPart{details: true, open: openAttr.MatchString(tag)}
	if m := summaryTag.FindStringSubmatchIndex(body); m != nil {
		d.summary = strings.Join(strings.Fields(htmlText(body[m[2]:m[3]])), " ")
		body = body[m[1]:]
	}
	d.parts = mdParts(body)
	return d, next
}

// htmlMarkdown is the markdown of an HTML block: its headings, paragraphs,
// lists, quotes, code, links and emphasis as markdown writes them.
func htmlMarkdown(h string) string {
	nodes, err := html.ParseFragment(strings.NewReader(h), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return htmlText(h)
	}
	return mdBlocks(nodes)
}

// mdBlocks is the markdown of nodes as blocks, a paragraph of each run of
// text and inline elements between the others.
func mdBlocks(nodes []*html.Node) string {
	var blocks []string
	var run []*html.Node
	flush := func() {
		if p := mdParagraph(run); p != "" {
			blocks = append(blocks, p)
		}
		run = nil
	}
	for _, n := range nodes {
		if !isBlock(n) {
			run = append(run, n)
			continue
		}
		flush()
		if b := mdBlock(n); strings.TrimSpace(b) != "" {
			blocks = append(blocks, b)
		}
	}
	flush()
	return strings.Join(blocks, "\n\n")
}

func isBlock(n *html.Node) bool {
	if n.Type != html.ElementNode {
		return false
	}
	switch n.DataAtom {
	case atom.P, atom.Div, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6,
		atom.Ul, atom.Ol, atom.Li, atom.Blockquote, atom.Pre, atom.Hr, atom.Table,
		atom.Details, atom.Summary, atom.Section, atom.Article, atom.Dl, atom.Dt, atom.Dd:
		return true
	}
	return false
}

func children(n *html.Node) []*html.Node {
	var out []*html.Node
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		out = append(out, k)
	}
	return out
}

func mdBlock(n *html.Node) string {
	switch n.DataAtom {
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		t := mdParagraph(children(n))
		if t == "" {
			return ""
		}
		return strings.Repeat("#", int(n.Data[1]-'0')) + " " + strings.ReplaceAll(t, "\\\n", " ")
	case atom.Ul, atom.Ol:
		return mdList(n)
	case atom.Blockquote:
		return prefixLines(mdBlocks(children(n)), "> ", "> ")
	case atom.Pre:
		return mdCode(n)
	case atom.Hr:
		return "---"
	case atom.Summary, atom.Dt:
		if t := mdParagraph(children(n)); t != "" {
			return "**" + t + "**"
		}
		return ""
	case atom.Table:
		return mdTable(n)
	}
	return mdBlocks(children(n))
}

// mdList is a list, each item's lines after its first under its marker.
func mdList(n *html.Node) string {
	num := 1
	if s, err := strconv.Atoi(attr(n, "start")); err == nil {
		num = s
	}
	var items []string
	for _, k := range children(n) {
		if k.Type != html.ElementNode || k.DataAtom != atom.Li {
			continue
		}
		marker := "- "
		if n.DataAtom == atom.Ol {
			marker = strconv.Itoa(num) + ". "
			num++
		}
		body := mdBlocks(children(k))
		items = append(items, prefixLines(body, marker, strings.Repeat(" ", len(marker))))
	}
	return strings.Join(items, "\n")
}

// prefixLines puts first before s's first line and rest before the
// others, but blank ones.
func prefixLines(s, first, rest string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		switch {
		case i == 0:
			lines[i] = first + l
		case l == "":
			lines[i] = strings.TrimRight(rest, " ")
		default:
			lines[i] = rest + l
		}
	}
	return strings.Join(lines, "\n")
}

func mdCode(n *html.Node) string {
	lang := ""
	for _, e := range append([]*html.Node{n}, children(n)...) {
		for _, c := range strings.Fields(attr(e, "class")) {
			if l, ok := strings.CutPrefix(c, "language-"); ok {
				lang = l
			}
		}
	}
	code := strings.TrimSuffix(textOf(n), "\n")
	fence := "```"
	for strings.Contains(code, fence) {
		fence += "`"
	}
	return fence + lang + "\n" + code + "\n" + fence
}

// mdTable is a table as markdown's, its first row its head.
func mdTable(n *html.Node) string {
	var rows [][]string
	var walk func(*html.Node)
	walk = func(e *html.Node) {
		for _, k := range children(e) {
			if k.Type != html.ElementNode {
				continue
			}
			if k.DataAtom != atom.Tr {
				walk(k)
				continue
			}
			var row []string
			for _, cell := range children(k) {
				if cell.DataAtom == atom.Td || cell.DataAtom == atom.Th {
					row = append(row, strings.ReplaceAll(mdParagraph(children(cell)), "\\\n", " "))
				}
			}
			rows = append(rows, row)
		}
	}
	walk(n)
	if len(rows) == 0 {
		return ""
	}
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	line := func(r []string) string {
		r = append(r, make([]string, cols-len(r))...)
		return "| " + strings.Join(r, " | ") + " |"
	}
	out := []string{line(rows[0]), line(slices.Repeat([]string{"---"}, cols))}
	for _, r := range rows[1:] {
		out = append(out, line(r))
	}
	return strings.Join(out, "\n")
}

// lineBreak stands for a <br> until the paragraph's spaces are one.
const lineBreak = "\x00"

// mdParagraph is the markdown of inline nodes, on one line but where
// they break it.
func mdParagraph(nodes []*html.Node) string {
	var b strings.Builder
	for _, n := range nodes {
		mdInline(&b, n)
	}
	p := spaces.ReplaceAllString(b.String(), " ")
	p = strings.ReplaceAll(strings.ReplaceAll(p, " "+lineBreak, lineBreak), lineBreak+" ", lineBreak)
	p = strings.Trim(p, " "+lineBreak)
	return strings.ReplaceAll(p, lineBreak, "\\\n")
}

var (
	spaces      = regexp.MustCompile(`[ \t\r\n]+`)
	mdSpecial   = strings.NewReplacer(`\`, `\\`, "`", "\\`", `*`, `\*`, `_`, `\_`, `[`, `\[`, `]`, `\]`, `<`, `\<`, `>`, `\>`, `~`, `\~`, `|`, `\|`, `&`, `\&`, `#`, `\#`, `!`, `\!`)
	listLike    = regexp.MustCompile(`^(\d+)([.)])`)
	inlineTags  = map[atom.Atom]bool{atom.Kbd: true, atom.Sub: true, atom.Sup: true, atom.Mark: true, atom.Small: true, atom.U: true, atom.Ins: true, atom.Samp: true}
	emphasisFor = map[atom.Atom]string{atom.Em: "*", atom.I: "*", atom.Strong: "**", atom.B: "**", atom.Del: "~~", atom.S: "~~", atom.Strike: "~~"}
)

// mdInline writes the markdown of an inline node: text escaped, as HTML
// had it as text, and the elements markdown has as markdown, others as
// their text or the inline HTML the view applies.
func mdInline(b *strings.Builder, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		t := mdSpecial.Replace(n.Data)
		if strings.TrimSpace(b.String()) == "" {
			t = escapeLead(t)
		}
		b.WriteString(t)
		return
	case html.ElementNode:
	default:
		return
	}
	inner := func() string {
		var ib strings.Builder
		for _, k := range children(n) {
			mdInline(&ib, k)
		}
		return ib.String()
	}
	switch {
	case n.DataAtom == atom.Br:
		b.WriteString(lineBreak)
	case n.DataAtom == atom.Img:
		b.WriteString(mdSpecial.Replace(attr(n, "alt")))
	case n.DataAtom == atom.Code || n.DataAtom == atom.Tt:
		b.WriteString(codeSpan(spaces.ReplaceAllString(textOf(n), " ")))
	case n.DataAtom == atom.A:
		t, href := strings.TrimSpace(inner()), attr(n, "href")
		switch {
		case href == "":
			b.WriteString(t)
		case t == "":
			b.WriteString("<" + href + ">")
		default:
			if strings.ContainsAny(href, " ()<>") {
				href = "<" + strings.NewReplacer("<", "%3C", ">", "%3E").Replace(href) + ">"
			}
			b.WriteString("[" + t + "](" + href + ")")
		}
	case emphasisFor[n.DataAtom] != "":
		t := inner()
		if s := strings.TrimSpace(t); s != "" {
			m := emphasisFor[n.DataAtom]
			lead, trail := t[:len(t)-len(strings.TrimLeft(t, " \t\n"))], t[len(strings.TrimRight(t, " \t\n")):]
			b.WriteString(lead + m + s + m + trail)
		} else {
			b.WriteString(t)
		}
	case inlineTags[n.DataAtom]:
		b.WriteString("<" + n.Data + ">" + inner() + "</" + n.Data + ">")
	case isBlock(n):
		b.WriteString(" " + inner() + " ")
	default:
		b.WriteString(inner())
	}
}

// escapeLead escapes what would start a block at a paragraph's start.
func escapeLead(t string) string {
	s := strings.TrimLeft(t, " \t\n")
	lead := t[:len(t)-len(s)]
	if m := listLike.FindStringSubmatch(s); m != nil {
		return lead + m[1] + `\` + s[len(m[1]):]
	}
	if s != "" && strings.ContainsRune("-+=", rune(s[0])) {
		return lead + `\` + s
	}
	return t
}

// codeSpan is a code span of t, its backticks more than t has in a row.
func codeSpan(t string) string {
	n, run := 0, 0
	for _, r := range t {
		if r == '`' {
			run++
			n = max(n, run)
		} else {
			run = 0
		}
	}
	ticks := strings.Repeat("`", n+1)
	if strings.HasPrefix(t, "`") || strings.HasSuffix(t, "`") {
		t = " " + t + " "
	}
	return ticks + t + ticks
}

func textOf(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for _, k := range children(n) {
		if k.DataAtom == atom.Br {
			b.WriteString("\n")
			continue
		}
		b.WriteString(textOf(k))
	}
	return b.String()
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

var htmlTagName = regexp.MustCompile(`^<(/?)([a-zA-Z][a-zA-Z0-9-]*)`)

// viewTags are the inline tags the markdown view applies, as <b> or <br>,
// and those the page reads itself (mdParts).
var viewTags = map[string]bool{"b": true, "strong": true, "i": true, "em": true, "u": true, "ins": true, "s": true, "del": true,
	"strike": true, "code": true, "kbd": true, "samp": true, "mark": true, "sub": true, "sup": true, "small": true, "br": true,
	"details": true, "summary": true}

// inlineHTML rewrites the HTML within the text of markdown src, which
// the markdown view shows as its source, as GitHub's bots write it: an
// image, as a <picture> of a severity's badge, as its description in
// brackets, as the view shows an image within text; a link as markdown's;
// other tags of HTML's gone, their text kept. Code stays as it is. With plain, for
// text shown as it is, links and the view's tags go too, <br> a line.
func inlineHTML(src string, plain bool) string {
	if !strings.Contains(src, "<") {
		return src
	}
	type edit struct {
		start, end int
		text       string
	}
	var edits []edit
	d := parser.Parse(src)
	for i := range d.Len() {
		in := d.Inlines(i)
		var links []int // the edits of the <a> open
		var hrefs []string
		for _, n := range in {
			if n.Kind != parser.HTML {
				continue
			}
			e := edit{start: n.Start, end: n.End}
			m := htmlTagName.FindStringSubmatch(n.Text)
			name := ""
			if m != nil {
				name = strings.ToLower(m[2])
			}
			closing := m != nil && m[1] == "/"
			switch {
			case m != nil && atom.Lookup([]byte(name)) == 0:
				continue // no HTML's, as Vec<String> in prose
			case plain && name == "br":
				e.text = "\n"
			case !plain && viewTags[name]:
				continue
			case name == "img" && !closing:
				if alt := strings.TrimSpace(attr(htmlElement(n.Text), "alt")); alt != "" {
					if plain {
						e.text = "[" + alt + "]"
					} else {
						// Not \[ \], which is math.
						e.text = "&#91;" + mdSpecial.Replace(alt) + "&#93;"
					}
				}
			case name == "a" && !plain:
				if !closing {
					if href := attr(htmlElement(n.Text), "href"); href != "" {
						links, hrefs = append(links, len(edits)), append(hrefs, href)
						e.text = "["
					}
				} else if len(links) > 0 {
					href := hrefs[len(hrefs)-1]
					if strings.ContainsAny(href, " ()<>") {
						href = "<" + strings.NewReplacer("<", "%3C", ">", "%3E").Replace(href) + ">"
					}
					e.text = "](" + href + ")"
					links, hrefs = links[:len(links)-1], hrefs[:len(hrefs)-1]
				}
			}
			edits = append(edits, e)
		}
		// A link not closed in its block is no link.
		for _, j := range links {
			edits[j].text = ""
		}
	}
	if len(edits) == 0 {
		return src
	}
	slices.SortFunc(edits, func(a, b edit) int { return a.start - b.start })
	var b strings.Builder
	at := 0
	for _, e := range edits {
		if e.start < at {
			continue
		}
		b.WriteString(src[at:e.start])
		b.WriteString(e.text)
		at = e.end
	}
	b.WriteString(src[at:])
	return b.String()
}

// htmlElement is the element of a tag, as <img alt="…">, for its
// attributes, decoded.
func htmlElement(tag string) *html.Node {
	nodes, err := html.ParseFragment(strings.NewReader(tag), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil || len(nodes) == 0 {
		return &html.Node{}
	}
	return nodes[0]
}
