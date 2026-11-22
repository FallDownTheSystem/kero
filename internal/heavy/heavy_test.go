package heavy

import "testing"

// Builds and tests are heavy, and what looks like them but keeps running
// (a server, a watcher) or only asks something is not.
func TestHeavy(t *testing.T) {
	cases := []struct {
		cmd   string
		heavy bool
	}{
		{"go test ./...", true},
		{"go build ./cmd/kero", true},
		{"go run ./cmd/server", false},
		{"go version", false},
		{"go test -run TestX ./internal/... 2>&1 | tail -20", true},
		{"cd app && npm test", true},
		{"npm run build", true},
		{"npm run dev", false},
		{"npm run test:watch", false},
		{"npm install", false},
		{"pnpm typecheck", true},
		{"pnpm dev", false},
		{"yarn lint", true},
		{"bun test", true},
		{"npx vitest run", true},
		{"npx vitest --watch", false},
		{"NODE_ENV=test npx jest", true},
		{"cargo t", true},
		{"cargo run --release", false},
		{`C:\Go\bin\go.exe test ./...`, true},
		{"./gradlew.bat build", true},
		{"make help", false},
		{"make -j8", true},
		{"python -m pytest -x", true},
		{"python -m http.server", false},
		{"uv run pytest", true},
		{"dotnet test", true},
		{"dotnet watch test", false},
		{"cmake --build build", true},
		{"cmake -S . -B build", false},
		{"git status && git diff", false},
		{"echo 'go test ./...'", false},
		{"rg 'npm test' docs", false},
		{"kero lock heavy -- go test ./...", false},
		{"time go vet ./...", true},
		{"nice -n 10 make", true},
		{"tsc --noEmit", true},
		{"tsc --watch", false},
	}
	for _, c := range cases {
		if got := Heavy(c.cmd, nil); got != c.heavy {
			t.Errorf("Heavy(%q) = %v, want %v", c.cmd, got, c.heavy)
		}
	}
}

// The setting's own commands add to the built-in ones, and a ! one takes
// a command out, whichever comes first.
func TestHeavyExtra(t *testing.T) {
	extra := []string{"just build", "!make help-all", "!go test", "./scripts/ci.sh"}
	cases := []struct {
		cmd   string
		heavy bool
	}{
		{"just build --release", true},
		{"just fmt", false},
		{"make help-all", false},
		{"make all", true},
		{"go test ./...", false},
		{"go build ./...", true},
		{"scripts/ci.sh", true},
	}
	for _, c := range cases {
		if got := Heavy(c.cmd, extra); got != c.heavy {
			t.Errorf("Heavy(%q, extra) = %v, want %v", c.cmd, got, c.heavy)
		}
	}
}
