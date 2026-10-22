// Package build tells what build of Kero runs: its version, and whether
// it is the server build (cmd/kero-server), which runs kerod and the CLI
// alone, with no app, and updates itself.
package build

import "github.com/egoist/mygo"

// Server is set by the server build.
var Server bool

// Version is the version of Kero that runs: the app's, or the one the
// server build was made of.
func Version() string { return mygo.App.Version() }
