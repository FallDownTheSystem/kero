package remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Errors a dial tells apart, for the app to say what to do.
var (
	// ErrNotInstalled: the host answered, but has no kero to run.
	ErrNotInstalled = errors.New("Kero is not installed on the host")
	// ErrAuth: the host refused us, or its key is not one ssh trusts yet.
	ErrAuth = errors.New("the host refused the connection")
)

// MuxPath is where kerod's network listeners take remote connections, as
// a WebSocket.
const MuxPath = "/kero/mux"

// ClientHeader is the header a client sends with its WebSocket handshake:
// browsers cannot send one, so that no web page reaches kerod through a
// browser on the tailnet.
const ClientHeader = "Kero-Client"

// BridgeLocations are where the bridge command looks for kero on a host,
// in order: a server install, the command line tool a desktop install
// links, the PATH, the apps.
var BridgeLocations = []string{
	`$HOME/.local/bin/kero`,
	`$HOME/.config/kero/bin/kero`,
	`$HOME/Library/Application Support/kero/bin/kero`,
	`kero`,
	`/usr/local/bin/kero`,
	`/usr/bin/kero`,
	`/Applications/Kero.app/Contents/MacOS/Kero`,
}

// bridgeNotFound is how the bridge command says there is no kero, as its
// exit status: a shell's for a command not found.
const bridgeNotFound = 127

// BridgeCommand is the shell command that runs kero's bridge on the host,
// for via ("ssh" or "wsl"): with sh, whatever the user's shell, so that
// fish and csh take it too.
func BridgeCommand(a Addr, via string) string { return "sh -c " + shellQuote(bridgeScript(a, via)) }

// bridgeScript is the script of sh that finds kero and runs its bridge.
func bridgeScript(a Addr, via string) string {
	run := " __bridge --via " + via
	var script string
	if a.Kero != "" {
		script = "exec " + shellQuote(a.Kero) + run
	} else {
		var locs []string
		for _, l := range BridgeLocations {
			locs = append(locs, `"`+l+`"`)
		}
		script = `for k in ` + strings.Join(locs, " ") + `; do if command -v "$k" >/dev/null 2>&1; then exec "$k"` + run + `; fi; done; ` +
			`echo "kero is not installed on $(uname -n)" >&2; exit 127`
	}
	return script
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// sshArgs are the arguments of ssh(1) that run the bridge on a host. It
// never asks anything, as there is no one to answer in the app: a key
// with a passphrase needs the agent, and a host's key needs trusting in a
// terminal first. It notices a dead link itself, as the mux does.
func sshArgs(a Addr) []string {
	args := []string{"-T",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=15",
		"-o", "ServerAliveInterval=10",
		"-o", "ServerAliveCountMax=3",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ClearAllForwardings=yes",
	}
	if a.Port != "" {
		args = append(args, "-p", a.Port)
	}
	if a.User != "" {
		args = append(args, "-l", a.User)
	}
	return append(args, "--", a.Host, BridgeCommand(a, "ssh"))
}

// SSHCommand is the program that runs ssh: $KERO_SSH, else ssh.
func SSHCommand() string {
	if s := os.Getenv("KERO_SSH"); s != "" {
		return s
	}
	return "ssh"
}

// transport opens the byte stream to a host that a remote connection runs
// over.
func transport(ctx context.Context, a Addr, client string) (io.ReadWriteCloser, error) {
	switch a.Scheme {
	case "ssh":
		return startBridge(exec.Command(SSHCommand(), sshArgs(a)...))
	case "wsl":
		if runtime.GOOS != "windows" {
			return nil, errors.New("wsl:// addresses are of Windows")
		}
		args := []string{}
		if a.Host != "" {
			args = append(args, "-d", a.Host)
		}
		// --exec runs sh itself, not through the distribution's shell.
		args = append(args, "--exec", "sh", "-c", bridgeScript(a, "wsl"))
		return startBridge(exec.Command("wsl.exe", args...))
	case "http", "https":
		scheme := "ws"
		if a.Scheme == "https" {
			scheme = "wss"
		}
		h := http.Header{}
		h.Set(ClientHeader, client)
		ws, resp, err := websocket.Dial(ctx, scheme+"://"+a.hostPort()+MuxPath, &websocket.DialOptions{
			HTTPHeader:      h,
			CompressionMode: websocket.CompressionDisabled, // ours is across the mux
		})
		if err != nil {
			if resp != nil && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized) {
				return nil, fmt.Errorf("%w: %s", ErrAuth, httpError(resp, err))
			}
			return nil, err
		}
		ws.SetReadLimit(64 << 20)
		return websocket.NetConn(context.Background(), ws, websocket.MessageBinary), nil
	}
	return nil, fmt.Errorf("no way to reach %s", a)
}

func httpError(resp *http.Response, err error) string {
	if resp.Body != nil {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if t := strings.TrimSpace(string(b)); t != "" {
			return t
		}
	}
	return err.Error()
}

// cmdConn is a program's standard input and output: ssh's or wsl.exe's,
// running the bridge.
type cmdConn struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr tail
	done   chan struct{}
	once   sync.Once
	err    error // the program's end, once done
}

func startBridge(cmd *exec.Cmd) (*cmdConn, error) {
	c := &cmdConn{cmd: cmd, done: make(chan struct{})}
	hideWindow(cmd)
	// Never a prompt: there is no one to answer.
	cmd.Env = append(os.Environ(), "SSH_ASKPASS_REQUIRE=never")
	var err error
	if c.stdin, err = cmd.StdinPipe(); err != nil {
		return nil, err
	}
	if c.stdout, err = cmd.StdoutPipe(); err != nil {
		return nil, err
	}
	cmd.Stderr = &c.stderr
	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%s is not installed: Kero reaches the host with it", cmd.Path)
		}
		return nil, err
	}
	go func() {
		c.err = cmd.Wait()
		close(c.done)
	}()
	return c, nil
}

func (c *cmdConn) Read(p []byte) (int, error)  { return c.stdout.Read(p) }
func (c *cmdConn) Write(p []byte) (int, error) { return c.stdin.Write(p) }

func (c *cmdConn) Close() error {
	c.once.Do(func() {
		c.stdin.Close()
		select {
		case <-c.done:
		case <-time.After(500 * time.Millisecond):
			c.cmd.Process.Kill()
		}
	})
	return nil
}

// failure explains why the program ended before the connection was up:
// what it said last, as ssh's "Permission denied", and how it exited.
func (c *cmdConn) failure() error {
	select {
	case <-c.done:
	case <-time.After(3 * time.Second):
		c.cmd.Process.Kill()
		<-c.done
	}
	msg := c.stderr.last()
	code := -1
	var ee *exec.ExitError
	if errors.As(c.err, &ee) {
		code = ee.ExitCode()
	}
	switch {
	case code == bridgeNotFound || strings.Contains(msg, "kero is not installed"):
		if msg == "" {
			msg = "kero not found"
		}
		return fmt.Errorf("%w: %s", ErrNotInstalled, msg)
	case code == 255 && authFailure(msg):
		return fmt.Errorf("%w: %s", ErrAuth, msg)
	case msg != "":
		return errors.New(msg)
	case c.err != nil:
		return fmt.Errorf("%s: %v", c.cmd.Path, c.err)
	}
	return fmt.Errorf("%s ended", c.cmd.Path)
}

func authFailure(msg string) bool {
	m := strings.ToLower(msg)
	for _, s := range []string{"permission denied", "host key verification failed", "authentication", "too many authentication failures", "no matching host key"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}

// tail keeps the end of what a program writes to its standard error.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > 4096 {
		t.buf = t.buf[len(t.buf)-4096:]
	}
	return len(p), nil
}

// last is the last line that says something.
func (t *tail) last() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := bytes.Split(bytes.TrimSpace(t.buf), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(string(lines[i])); l != "" {
			return l
		}
	}
	return ""
}
