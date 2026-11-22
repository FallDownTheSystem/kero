package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"kero/internal/proto"
)

// The ways in of the agents that take a message from another program,
// through their own session, rather than from code of Kero's running in
// them (inbox.go): Codex's queue, OpenCode's background service. Either
// gives the agent the message as a turn of its own, after the turn it may
// be in, and leaves its prompt as the user left it.

// nativeWait is how long a way in may take. One cut short may have taken
// the message all the same, which is then typed in too, so it is long:
// the pane's other messages wait for it meanwhile (deliverNative), not the
// other panes'.
const nativeWait = 30 * time.Second

// nativeSession is the session of pane pn's agent, when the agent has a
// way in of its own here.
func nativeSession(pn *pane) (id string, as proto.AgentSession, ok bool) {
	pn.mu.Lock()
	defer pn.mu.Unlock()
	s := pn.info.AgentSession
	if s == nil || s.ID == "" || s.Agent != pn.info.Agent || s.Agent != "codex" && s.Agent != "opencode" {
		return "", proto.AgentSession{}, false
	}
	return pn.info.ID, *s, true
}

// sendNative gives text to the agent of pane pn through its own way in,
// reporting whether it took it; a failure is logged, and the message is
// typed instead.
func (d *Daemon) sendNative(pn *pane, text string) bool {
	id, as, ok := nativeSession(pn)
	if !ok {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), nativeWait)
	defer cancel()
	var err error
	if as.Agent == "codex" {
		err = codexQueue(ctx, as.ID, text)
	} else {
		err = openCodePrompt(ctx, as.ID, text)
	}
	if err != nil {
		d.log.Printf("message to %s's %s: %v; typed into its prompt instead", id, as.Agent, err)
		return false
	}
	return true
}

// codexQueue queues text for Codex's session thread (codex queue), which
// the session runs as soon as its turn is over: at once when it has none.
// After the user stops a turn, the session holds it until the user's next
// prompt.
var codexQueue = func(ctx context.Context, thread, text string) error {
	exe, err := exec.LookPath("codex")
	if err != nil {
		return err
	}
	// npm's codex.cmd runs in cmd.exe, which reads the message for its own
	// syntax: a line break ends it, and & or | in it starts a command. The
	// program the package ships beside it reads it as it is.
	if runtime.GOOS == "windows" && !strings.EqualFold(filepath.Ext(exe), ".exe") {
		native := codexNative(exe, runtime.GOARCH)
		if native == "" {
			return fmt.Errorf("%s is a script, which would read the message as commands, and no codex.exe of its package was found", exe)
		}
		exe = native
	}
	cmd := exec.CommandContext(ctx, exe, "queue", "--thread", thread, "--message", text)
	hideWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("codex queue: %v: %s", err, proto.OneLine(string(out)))
	}
	return nil
}

// codexNative is the codex.exe of the npm package whose launcher script
// is script, for arch (GOARCH), "" for none: the package's own, or that of
// the package of its platform, which newer versions ship it in, nested or
// beside it.
func codexNative(script, arch string) string {
	triple := map[string]string{"amd64": "x86_64-pc-windows-msvc", "arm64": "aarch64-pc-windows-msvc"}[arch]
	if triple == "" {
		return ""
	}
	modules := filepath.Join(filepath.Dir(script), "node_modules")
	exe := filepath.Join("vendor", triple, "codex", "codex.exe")
	for _, pattern := range []string{
		filepath.Join(modules, "@openai", "codex", exe),
		filepath.Join(modules, "@openai", "codex", "node_modules", "@openai", "*", exe),
		filepath.Join(modules, "@openai", "*", exe),
	} {
		if found, _ := filepath.Glob(pattern); len(found) > 0 {
			return found[0]
		}
	}
	return ""
}

// openCodeService is the file where OpenCode's background service, which
// every OpenCode terminal is a client of, tells where it listens.
var openCodeService = func() string {
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(proto.Home(), ".local", "state")
	}
	return filepath.Join(state, "opencode", "service.json")
}

// openCodePrompt gives text to OpenCode's session as a prompt of the user,
// queued after the turn it may be in.
func openCodePrompt(ctx context.Context, session, text string) error {
	b, err := os.ReadFile(openCodeService())
	if err != nil {
		return err
	}
	var svc struct{ URL, Password string }
	if err := json.Unmarshal(b, &svc); err != nil {
		return err
	}
	if svc.URL == "" {
		return errors.New("OpenCode's service tells no address")
	}
	body, _ := json.Marshal(map[string]any{"text": text, "delivery": "queue", "metadata": map[string]string{"source": "kero"}})
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(svc.URL, "/")+"/api/session/"+url.PathEscape(session)+"/prompt", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth("opencode", svc.Password)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var msg bytes.Buffer
		msg.ReadFrom(http.MaxBytesReader(nil, resp.Body, 512))
		return fmt.Errorf("OpenCode's service: %s: %s", resp.Status, proto.OneLine(msg.String()))
	}
	return nil
}
