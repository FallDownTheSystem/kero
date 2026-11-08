package proto

// DevTools is the DevTools port of the app's browser panes, as kero
// browser devtools prints it: the endpoint that Playwright's
// connectOverCDP, chrome-devtools-mcp's --browserUrl and Lighthouse's
// --port connect to, and each pane's page there. Every pane is a page of
// the one browser, which tools tell apart by its URL or title.
type DevTools struct {
	Endpoint string `json:"endpoint"` // http://127.0.0.1:<port>
	Port     int    `json:"port"`
	// Browser is the browser's WebSocket URL, for clients that take one.
	Browser string         `json:"browser,omitempty"`
	Panes   []DevToolsPane `json:"panes"`
}

// DevToolsPane is a browser pane's page on the DevTools port: Target is
// the page's target id, and WebSocket the URL that drives it alone; both
// empty while the port shows no page of the pane's URL and title.
type DevToolsPane struct {
	Pane      string `json:"pane"`
	Session   string `json:"session"`
	URL       string `json:"url"`
	Title     string `json:"title"`
	Target    string `json:"target,omitempty"`
	WebSocket string `json:"webSocketDebuggerUrl,omitempty"`
}
