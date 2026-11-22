// Package heavy tells a heavy command, a build or a test run, from the
// others an agent runs, from its command line alone: kero hook holds a
// heavy one for a free slot of kerod's (heavy-slots), so that the agents
// of one machine don't build and test all at once.
//
// It reads a command as a shell does, roughly: segments between &&, ||,
// ;, | and &, outside quotes, each a program and its arguments after
// variables and wrappers such as env, time and npx. A command is heavy
// when one of its segments is. It guesses: what it misses runs at once,
// as without Kero, and the heavy-command setting adds or takes out
// commands of the user's.
package heavy

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Heavy reports whether command builds or tests. extra are the user's
// own, the heavy-command setting's: a command's start, as "just build",
// which makes a segment that starts so heavy, or one after a !, as
// "!make help", which makes it light.
func Heavy(command string, extra []string) bool {
	segs := segments(command)
	// kero lock holds a slot of its own: one of the hook's around it
	// would wait on itself, with one slot.
	for _, words := range segs {
		if len(words) > 1 && program(words[0]) == "kero" && words[1] == "lock" {
			return false
		}
	}
	return slices.ContainsFunc(segs, func(words []string) bool { return heavySegment(words, extra) })
}

func heavySegment(words []string, extra []string) bool {
	words = unwrap(words)
	if len(words) == 0 {
		return false
	}
	verdict, known := false, false
	for _, e := range extra {
		light := strings.HasPrefix(e, "!")
		pat := strings.Fields(strings.TrimPrefix(e, "!"))
		if len(pat) == 0 || !startsWith(words, pat) {
			continue
		}
		// A light one wins over a heavy one, as the narrower exception.
		if light {
			return false
		}
		verdict, known = true, true
	}
	if known {
		return verdict
	}
	if watching(words) {
		return false
	}
	prog, args := program(words[0]), words[1:]
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch prog {
	case "go":
		return slices.Contains([]string{"build", "test", "vet", "install"}, sub)
	case "cargo":
		return slices.Contains([]string{"build", "b", "test", "t", "check", "c", "clippy", "bench", "nextest", "install", "doc"}, sub)
	case "dotnet":
		return slices.Contains([]string{"build", "test", "publish", "pack", "msbuild", "vstest"}, sub)
	case "swift":
		return sub == "build" || sub == "test"
	case "bazel", "bazelisk":
		return sub == "build" || sub == "test" || sub == "coverage"
	case "meson":
		return sub == "compile" || sub == "test"
	case "cmake":
		return slices.Contains(args, "--build")
	case "deno":
		return sub == "test" || sub == "compile" || sub == "check"
	case "vite", "next", "nuxt", "astro", "remix":
		return sub == "build"
	case "playwright", "cypress":
		return sub == "test" || sub == "run"
	case "turbo":
		return sub != "" && sub != "dev" && sub != "login" && sub != "link"
	case "nx":
		return slices.Contains([]string{"build", "test", "lint", "e2e", "run", "run-many", "affected"}, sub)
	case "make", "gmake", "mingw32-make", "nmake", "ninja", "msbuild", "gradle", "gradlew", "mvn", "mvnw", "ant",
		"xcodebuild", "ctest", "tsc", "vue-tsc", "vitest", "jest", "mocha", "ava", "pytest", "py.test", "tox", "nox",
		"rspec", "phpunit", "webpack", "rollup", "esbuild", "eslint", "mypy", "pyright", "golangci-lint", "tsup":
		// make help, gradle --version: a question, not a build.
		return !slices.Contains(args, "--version") && !slices.Contains(args, "--help") && sub != "help" && sub != "-h"
	case "python", "python3", "py":
		for i, a := range args {
			if a == "-m" && i+1 < len(args) {
				return slices.Contains([]string{"pytest", "unittest", "tox", "nox", "mypy", "build"}, args[i+1])
			}
		}
		return false
	case "npm", "pnpm", "yarn", "bun":
		return script(prog, args)
	}
	return false
}

// script tells a package manager's heavy script: npm test, npm run build,
// pnpm typecheck, bun test, as their names say.
func script(prog string, args []string) bool {
	if len(args) == 0 {
		return false
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "test", "t", "tst":
		return true
	case "run", "run-script", "rs":
		if len(rest) == 0 {
			return false
		}
		return heavyScript.MatchString(rest[0])
	case "build":
		return prog != "npm" // npm build is gone; the others run the script, or bun its bundler
	}
	// pnpm and yarn run a script by its name alone; npm and bun only
	// their own commands.
	if prog == "pnpm" || prog == "yarn" {
		return heavyScript.MatchString(sub)
	}
	return false
}

// heavyScript is a script's name that builds or tests: build, test:unit,
// typecheck, lint, e2e.
var heavyScript = regexp.MustCompile(`(?i)(^|[:_-])(build|test|tests|e2e|lint|typecheck|type-check|tsc|check|compile|ci|verify)($|[:_-])`)

// watching tells a command that keeps running, for a server or a
// watcher, which would hold a slot for as long as it runs.
func watching(words []string) bool {
	prog := program(words[0])
	for i, w := range words[1:] {
		switch w {
		case "--watch", "--watchAll", "-w", "watch", "--serve", "serve", "dev", "start", "preview", "--ui":
			return true
		}
		// go run and cargo run start what they built, which may be a
		// server.
		if i == 0 && w == "run" && (prog == "go" || prog == "cargo" || prog == "dotnet") {
			return true
		}
		if strings.HasPrefix(w, "--watch=") {
			return true
		}
		if (prog == "npm" || prog == "pnpm" || prog == "yarn" || prog == "bun") && longScript.MatchString(w) {
			return true
		}
	}
	return false
}

// longScript is a script's name that keeps running: dev, serve, start,
// watch, preview.
var longScript = regexp.MustCompile(`(?i)(^|[:_-])(dev|serve|start|watch|preview)($|[:_-])`)

// unwrap drops what comes before a segment's program: variables, and
// programs that run another, as env, time and npx.
func unwrap(words []string) []string {
	for len(words) > 0 {
		w := words[0]
		switch {
		case strings.Contains(w, "=") && !strings.HasPrefix(w, "-") && !strings.HasPrefix(w, "="):
			words = words[1:]
		case slices.Contains([]string{"env", "time", "nice", "nohup", "command", "exec", "npx", "bunx", "pnpx", "sudo"}, program(w)):
			words = words[1:]
			// nice -n 10, npx --yes: their own flags too.
			for len(words) > 0 && strings.HasPrefix(words[0], "-") {
				if words[0] == "-n" && len(words) > 1 {
					words = words[1:]
				}
				words = words[1:]
			}
		case (program(w) == "uv" || program(w) == "poetry" || program(w) == "pipenv") && len(words) > 1 && words[1] == "run",
			(program(w) == "pnpm" || program(w) == "yarn") && len(words) > 1 && (words[1] == "exec" || words[1] == "dlx"):
			words = words[2:]
		default:
			return words
		}
	}
	return words
}

// startsWith tells a segment that starts with pat's words, its program by
// its name, as program reads it.
func startsWith(words, pat []string) bool {
	words = unwrap(words)
	if len(words) < len(pat) || program(words[0]) != program(pat[0]) {
		return false
	}
	return slices.Equal(words[1:len(pat)], pat[1:])
}

// program is a program's name, as a command names it: the file's, in
// lower case, without a path or Windows' extensions, so that
// C:\Go\bin\go.exe and ./gradlew.bat read as go and gradlew.
func program(word string) string {
	word = strings.ToLower(strings.ReplaceAll(word, `\`, "/"))
	word = filepath.Base(word)
	for _, ext := range []string{".exe", ".cmd", ".bat", ".ps1", ".sh"} {
		word = strings.TrimSuffix(word, ext)
	}
	return word
}

// segments splits a command line into its commands' words: at &&, ||, ;,
// |, & and line ends outside quotes, and at spaces. A quote's words keep
// together, without the quotes; $(…) and backticks are read as words, as
// what they run is rarely the build.
func segments(line string) [][]string {
	var out [][]string
	var words []string
	var word strings.Builder
	inWord := false
	endWord := func() {
		if inWord {
			words = append(words, word.String())
			word.Reset()
			inWord = false
		}
	}
	endSeg := func() {
		endWord()
		if len(words) > 0 {
			out = append(out, words)
			words = nil
		}
	}
	var quote rune
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			endWord()
		case r == ';' || r == '&' || r == '|' || r == '\n' || r == '\r' || r == '(' || r == ')' || r == '{' || r == '}':
			endSeg()
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	endSeg()
	return out
}
