package main

import (
	"strings"
	"testing"
)

func TestMDParts(t *testing.T) {
	src := "Bumps x.\n<details>\n<summary>Release <b>notes</b></summary>\n<p>Inner</p>\n<details open><summary>Nested</summary>\n\nDeep\n</details>\n</details>\n\nAfter.\n\n```html\n<details>not this</details>\n```\n\n<details><summary>Left open</summary>\nrest"
	parts := mdParts(src)
	if len(parts) != 4 {
		t.Fatalf("parts = %#v", parts)
	}
	if parts[0].text != "Bumps x.\n" {
		t.Errorf("first = %q", parts[0].text)
	}
	d := parts[1]
	if !d.details || d.summary != "Release notes" || d.open {
		t.Errorf("details = %#v", d)
	}
	if len(d.parts) != 2 || d.parts[0].text != "\n<p>Inner</p>\n" || !d.parts[1].details || !d.parts[1].open || d.parts[1].summary != "Nested" {
		t.Errorf("details' parts = %#v", d.parts)
	}
	if !strings.Contains(parts[2].text, "After.") || !strings.Contains(parts[2].text, "<details>not this</details>") {
		t.Errorf("text after = %q", parts[2].text)
	}
	if !parts[3].details || parts[3].summary != "Left open" || len(parts[3].parts) != 1 || parts[3].parts[0].text != "\nrest" {
		t.Errorf("unclosed = %#v", parts[3])
	}
	if p := mdParts("<details>\n\nNo summary\n\n</details>"); len(p) != 1 || p[0].summary != "" || len(p[0].parts) != 1 {
		t.Errorf("no summary = %#v", p)
	}
}

func TestHTMLMarkdown(t *testing.T) {
	for _, tc := range []struct{ html, md string }{
		{`<h2>typescript - v3.0.0</h2>`, `## typescript - v3.0.0`},
		{`<p><em>Sourced from <a href="https://x.dev/r">@mistralai's releases</a>.</em></p>`, `*Sourced from [@mistralai's releases](https://x.dev/r).*`},
		{"<ul>\n<li><code>Runs</code></li>\n<li><code>a`b</code> c</li>\n</ul>", "- `Runs`\n- ``a`b`` c"},
		{`<ol start="3"><li>three<ul><li>sub</li></ul></li><li>four</li></ol>`, "3. three\n\n   - sub\n4. four"},
		{`<blockquote><p>one</p><p>two</p></blockquote>`, "> one\n>\n> two"},
		{`<p>EventStream&lt;T&gt; * not_em</p>`, `EventStream\<T\> \* not\_em`},
		{`<p>a<br>b</p>`, "a\\\nb"},
		{"<pre><code class=\"language-go\">x := 1\n</code></pre>", "```go\nx := 1\n```"},
		{`<p>1. not a list</p>`, `1\. not a list`},
		{`<table><tr><th>A</th><th>B</th></tr><tr><td>1</td><td>x|y</td></tr></table>`, "| A | B |\n| --- | --- |\n| 1 | x\\|y |"},
		{`<p><!-- raw HTML omitted -->kept <kbd>K</kbd></p>`, "kept <kbd>K</kbd>"},
	} {
		if got := htmlMarkdown(tc.html); got != tc.md {
			t.Errorf("htmlMarkdown(%q)\n got %q\nwant %q", tc.html, got, tc.md)
		}
	}
}
