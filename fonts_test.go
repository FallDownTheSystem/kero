package main

import (
	"path/filepath"
	"slices"
	"testing"
)

// Every family of kero's own text it offers is in resources/fonts/app,
// and registers.
func TestAppFonts(t *testing.T) {
	appFonts = nil
	loadAppFonts(filepath.Join("resources", "fonts", "app"))
	if !slices.Equal(appFonts, appFontFamilies) {
		t.Errorf("registered %q, want %q", appFonts, appFontFamilies)
	}
}
