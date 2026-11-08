package main

import (
	"runtime"
	"strconv"

	"kero/internal/settings"
)

// devToolsPlatform tells whether the browser panes can open a DevTools
// port: WebView2's browser does, as it starts with the first page.
var devToolsPlatform = runtime.GOOS == "windows"

// devToolsArguments are the arguments of WebView2's browser that open the
// DevTools port browser-devtools asks for, on 127.0.0.1 alone, as
// Chromium opens it; "" with it off. The browser starts once, so a change
// takes effect when Kero starts again.
func devToolsArguments(s settings.Settings) string {
	if !s.DevTools {
		return ""
	}
	return "--remote-debugging-port=" + strconv.Itoa(s.DevToolsPort)
}
