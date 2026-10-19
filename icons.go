package main

import "github.com/egoist/mygo/ui"

func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.35" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

var (
	sidebarIcon  = icon(`<rect x="1.75" y="2.75" width="12.5" height="10.5" rx="2.25"/><path d="M6.25 2.75v10.5"/>`)
	plusIcon     = icon(`<path d="M8 3v10M3 8h10"/>`)
	chevronIcon  = icon(`<path d="M6 4l4 4-4 4"/>`)
	folderIcon   = icon(`<path d="M2 4.75A1.75 1.75 0 0 1 3.75 3h2.6l1.5 1.5h4.4A1.75 1.75 0 0 1 14 6.25v5A1.75 1.75 0 0 1 12.25 13h-8.5A1.75 1.75 0 0 1 2 11.25z"/>`)
	searchIcon   = icon(`<circle cx="7" cy="7" r="4.25"/><path d="M10.25 10.25L13.5 13.5"/>`)
	terminalIcon = icon(`<path d="M3 4.5l3.5 3.5L3 11.5M8.5 12h4.5"/>`)
	returnIcon   = icon(`<path d="M12.5 3.5v4.25a1.75 1.75 0 0 1-1.75 1.75H3.5M6 6.5l-2.75 3 2.75 3"/>`)
	boltIcon     = icon(`<path d="M8.75 1.75L3.5 9h4l-.75 5.25L12 7H8z"/>`)
	moonIcon     = icon(`<path d="M13 9.5A5.5 5.5 0 1 1 6.5 3a4.5 4.5 0 0 0 6.5 6.5z"/>`)
)
