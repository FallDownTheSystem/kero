package build

import "strings"

// Fork reports whether v is a version of this fork's own releases, as
// 2.1.18-fork.1 (FORK.md): the server builds that hosts install are
// egoist/kero's, which have no such version, and which update.Compare
// takes for newer than it, a prerelease of 2.1.18.
func Fork(v string) bool { return strings.Contains(v, "-fork.") }
