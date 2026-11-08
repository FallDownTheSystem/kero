package main

import (
	"embed"

	"github.com/egoist/mygo/ui"
	"kero/internal/proto"
)

// The icons are Framework7's, drawn after SF Symbols so that they feel at
// home on macOS, a few of other sets where it has none, and programs'
// logos from Simple Icons, as Iconify serves them: see icons/README.md.
//
//go:embed icons/*.svg
var iconFiles embed.FS

// glyph returns an icon of the set, as "f7-gear-alt".
func glyph(name string) *ui.SVG {
	b, err := iconFiles.ReadFile("icons/" + name + ".svg")
	if err != nil {
		panic("kero: no icon " + name)
	}
	return ui.MustParseSVG(b)
}

// icon draws an icon of kero's own, on 16, as a status glyph.
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.35" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	sidebarIcon      = glyph("f7-sidebar-left")
	sidebarRightIcon = glyph("f7-sidebar-right")
	fileIcon         = glyph("f7-doc")
	newFileIcon      = glyph("f7-doc-badge-plus")
	newFolderIcon    = glyph("f7-folder-badge-plus")
	plusIcon         = glyph("f7-plus")
	chevronIcon      = glyph("f7-chevron-right")
	chevronUpIcon    = glyph("f7-chevron-up")
	chevronDownIcon  = glyph("f7-chevron-down")
	folderIcon       = glyph("f7-folder")
	searchIcon       = glyph("f7-search")
	terminalIcon     = glyph("meteor-terminal")
	returnIcon       = glyph("f7-return")
	gearIcon         = glyph("f7-gear-alt")
	splitRightIcon   = glyph("f7-square-split-2x1")
	splitDownIcon    = glyph("f7-square-split-1x2")
	zoomIcon         = glyph("f7-arrow-up-left-arrow-down-right")
	unzoomIcon       = glyph("f7-arrow-down-right-arrow-up-left")
	closeIcon        = glyph("f7-xmark")
	swapIcon         = glyph("f7-arrow-right-arrow-left")
	branchIcon       = glyph("f7-arrow-branch")
	worktreeIcon     = glyph("lucide-folder-git-2")
	diffIcon         = glyph("octicon-file-diff-16")
	wrapIcon         = glyph("octicon-wrap-16")
	unwrapIcon       = glyph("octicon-unwrap-16")
	treeIcon         = glyph("f7-list-bullet-indent")
	collapseIcon     = glyph("f7-rectangle-compress-vertical")
	expandIcon       = glyph("f7-rectangle-expand-vertical")
	revertIcon       = glyph("f7-arrow-uturn-left")
	openFileIcon     = glyph("f7-arrow-up-right-square")
	globeIcon        = glyph("f7-globe")
	chevronLeftIcon  = glyph("f7-chevron-left")
	reloadIcon       = glyph("f7-arrow-clockwise")
	copyIcon         = glyph("f7-doc-on-doc")
	linkIcon         = glyph("f7-arrow-up-right")
	editIcon         = glyph("f7-pencil")
	archiveIcon      = icon(`<rect x="2" y="3" width="12" height="3" rx="1"/><path d="M3 6v6.5a1 1 0 0 0 1 1h8a1 1 0 0 0 1-1V6M6.5 9h3"/>`)
	panelIcon        = sidebarRightIcon
	docIcon          = fileIcon
	unfoldAllIcon    = glyph("lucide-list-chevrons-up-down")
	biggerIcon       = glyph("lucide-a-arrow-up")
	smallerIcon      = glyph("lucide-a-arrow-down")
	foldAllIcon      = glyph("lucide-list-chevrons-down-up")
	previewIcon      = glyph("f7-eye")
	moreIcon         = glyph("f7-ellipsis")
	caseIcon         = glyph("lucide-case-sensitive")
	wordIcon         = glyph("lucide-whole-word")
	regexIcon        = glyph("lucide-regex")
	commentIcon      = glyph("f7-text-bubble")
	minusIcon        = glyph("f7-minus")
	syncIcon         = glyph("f7-arrow-up-arrow-down")
	pushIcon         = glyph("f7-arrow-up")
	pullIcon         = glyph("f7-arrow-down")
	tagIcon          = glyph("f7-tag")
	historyIcon      = glyph("lucide-git-graph")
	commitIcon       = glyph("lucide-git-commit-horizontal")
	mergeIcon        = glyph("lucide-git-merge")
	prIcon           = glyph("lucide-git-pull-request")
	draftPRIcon      = glyph("lucide-git-pull-request-draft")
	warningIcon      = glyph("f7-exclamationmark-triangle")
	clockIcon        = glyph("f7-clock")
	sparklesIcon     = glyph("f7-sparkles")
	// pickIcon is Pick Elements': a pointer on a dashed box, as browsers'
	// inspectors draw it.
	pickIcon  = icon(`<path d="M6 13.5H3.5a1 1 0 0 1-1-1V3.5a1 1 0 0 1 1-1h9a1 1 0 0 1 1 1V6"/><path d="M8 8l6 2.2-2.6 1.1L10.2 14z"/>`)
	turnsIcon = icon(`<path d="M3 3h10a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H7.5L4.5 13.5V11H3a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1z"/>`)
)

// programIcons are the logos of programs a pane may run, by the name kerod
// gives the foreground program, its agent or its script's runtime.
var programIcons = map[string]*ui.SVG{}

func init() {
	for name, logo := range map[string]string{
		"claude": "claude", "codex": "openai", "gemini": "googlegemini", "opencode": "opencode",
		"amp": "amp", "cursor-agent": "cursor", "copilot": "githubcopilot", "qwen": "qwen",
		"kimi": "kimi", "cline": "cline",
		"node": "nodedotjs", "bun": "bun", "deno": "deno", "python": "python", "python3": "python",
		"vim": "vim", "nvim": "neovim", "git": "git", "docker": "docker",
	} {
		programIcons[name] = glyph("si-" + logo)
	}
}

// programIcon is the logo of the pane's foreground program, as Claude's
// for Claude Code and Node.js's for a script node runs; the terminal's
// while the shell is in the foreground or the program has none; a globe
// for a browser.
func programIcon(p *proto.Pane) *ui.SVG {
	if p.Browser() {
		return globeIcon
	}
	if p.Editor() {
		return docIcon
	}
	for _, name := range []string{p.Agent, p.Process, p.Runtime} {
		if i := programIcons[name]; i != nil {
			return i
		}
	}
	return terminalIcon
}

// agentColors are the colors of coding agents whose logos have one of
// their own, as Claude's orange; the others take the text's.
var agentColors = map[string]ui.Color{
	"claude": ui.Hex("#d97757"),
	"gemini": ui.Hex("#3186ff"),
}

// programColor is the color of a pane's program icon in the sidebar: a
// coding agent's own while it runs, the text's for another program, and
// faint once the shell is back in the foreground or the pane ended, as
// the Ended list shows the agents that stopped.
func programColor(pal *palette, p *proto.Pane) ui.Color {
	switch {
	case p.Status == proto.Exited:
		return pal.textFaint
	case p.Agent != "":
		if col, ok := agentColors[p.Agent]; ok {
			return col
		}
		return pal.text
	case p.Process != "", p.Browser():
		return pal.textMuted
	}
	return pal.textFaint
}
