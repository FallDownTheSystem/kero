package remote

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"kero/internal/proto"
)

func TestParseAddr(t *testing.T) {
	for _, c := range []struct{ in, out, scheme, user, host, port string }{
		{"devbox", "devbox", "ssh", "", "devbox", ""},
		{"me@devbox", "me@devbox", "ssh", "me", "devbox", ""},
		{"me@devbox:2222", "me@devbox:2222", "ssh", "me", "devbox", "2222"},
		{"ssh://me@build.example.com", "me@build.example.com", "ssh", "me", "build.example.com", ""},
		{"ssh://[::1]:22", "[::1]:22", "ssh", "", "::1", "22"},
		{"wsl://", "wsl://", "wsl", "", "", ""},
		{"wsl://Ubuntu-24.04", "wsl://Ubuntu-24.04", "wsl", "", "Ubuntu-24.04", ""},
		{"http://build:7373", "http://build:7373", "http", "", "build", "7373"},
		{"https://build.tail1234.ts.net", "https://build.tail1234.ts.net", "https", "", "build.tail1234.ts.net", ""},
		{"devbox?kero=/opt/kero/kero", "devbox?kero=%2Fopt%2Fkero%2Fkero", "ssh", "", "devbox", ""},
	} {
		a, err := ParseAddr(c.in)
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if a.Scheme != c.scheme || a.User != c.user || a.Host != c.host || a.Port != c.port {
			t.Errorf("%q: %+v", c.in, a)
		}
		if a.String() != c.out {
			t.Errorf("%q: written %q, want %q", c.in, a.String(), c.out)
		}
		if b, err := ParseAddr(a.String()); err != nil || b != a {
			t.Errorf("%q: read back %+v, %v", c.in, b, err)
		}
	}
	for _, bad := range []string{"", "ftp://x", "-oProxyCommand=evil", "ssh://-x", "me:secret@host", "http://", "a b", "devbox/path", "http://x?kero=y", "host:port"} {
		if a, err := ParseAddr(bad); err == nil {
			t.Errorf("%q read as %+v", bad, a)
		}
	}
}

func TestBridgeCommandQuotes(t *testing.T) {
	cmd := BridgeCommand(Addr{Scheme: "ssh", Host: "x", Kero: "/opt/it's/kero"}, "ssh")
	if !strings.HasPrefix(cmd, "sh -c '") || !strings.Contains(cmd, `'\''`) {
		t.Errorf("%s", cmd)
	}
	if args := sshArgs(Addr{Scheme: "ssh", Host: "devbox", User: "me", Port: "2222"}); args[len(args)-3] != "--" || args[len(args)-2] != "devbox" {
		t.Errorf("ssh %q: the host must follow --", args)
	}
}

func TestValidName(t *testing.T) {
	for name, ok := range map[string]bool{"build": true, "build-2.lab": true, "a_b": true, "local": false, "LOCAL": false,
		"": false, "-x": false, "a b": false, ".x": false, "x/y": false} {
		if ValidName(name) != ok {
			t.Errorf("ValidName(%q) = %v", name, !ok)
		}
	}
}

func TestParseListen(t *testing.T) {
	for in, want := range map[string]Listen{
		"tailnet://":                     {Kind: "tailnet", Port: DefaultPort},
		"tailnet://:7400":                {Kind: "tailnet", Port: "7400"},
		"tailnet-serve://":               {Kind: "tailnet-serve"},
		"tailnet-serve:///run/kero.sock": {Kind: "tailnet-serve", Path: "/run/kero.sock"},
	} {
		got, err := ParseListen(in)
		if err != nil || got != want {
			t.Errorf("%q: %+v, %v", in, got, err)
		}
	}
	for _, bad := range []string{"tcp://:80", "tailnet://0.0.0.0:7373", "tailnet-serve://host/x", ":7373"} {
		if l, err := ParseListen(bad); err == nil {
			t.Errorf("%q read as %+v", bad, l)
		}
	}
}

// pipes are the ends of a connection the test's client and server share:
// TCP's on loopback, which buffer what is written, as the pipes of ssh do.
func pipes() (client, server io.ReadWriteCloser) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer ln.Close()
	ch := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		ch <- c
	}()
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		panic(err)
	}
	return c, <-ch
}

// echoServer answers each stream's Hello with a Welcome naming its kind,
// then echoes what comes.
func echoServer(t *testing.T) *Server {
	return &Server{
		Info: func() proto.HostInfo {
			return proto.HostInfo{OS: "linux", Hostname: "box", Kero: "9.9.9", Version: proto.Version}
		},
		Stream: func(s net.Conn, hello []byte, mux proto.MuxHello) {
			defer s.Close()
			var h proto.Hello
			if err := json.Unmarshal(hello, &h); err != nil {
				t.Errorf("hello %q: %v", hello, err)
				return
			}
			b, _ := json.Marshal(proto.Welcome{Version: proto.Version, App: h.Kind + "/" + mux.Client})
			s.Write(append(b, '\n'))
			io.Copy(s, s)
		},
		Restart: func() error { return nil },
		Update: func(app string) error {
			if app == "bad" {
				return errors.New("no such version")
			}
			return nil
		},
	}
}

func dialPipe(t *testing.T, s *Server, junk string) (*Conn, chan error) {
	t.Helper()
	cl, sv := pipes()
	done := make(chan error, 1)
	go func() {
		if junk != "" {
			// What a shell's startup files print before the bridge runs.
			io.WriteString(sv, junk)
		}
		done <- s.Serve(sv)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := handshake(ctx, cl, Addr{Scheme: "ssh", Host: "box"}, Options{App: "1.0", Client: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return c, done
}

func TestMuxStreams(t *testing.T) {
	c, done := dialPipe(t, echoServer(t), "Welcome to the box!\nlast login: today")
	if c.Welcome.Host.Hostname != "box" || c.Welcome.Compress != deflate {
		t.Fatalf("welcome %+v", c.Welcome)
	}
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := c.Dial()
			if err != nil {
				t.Error(err)
				return
			}
			defer s.Close()
			kind := "control"
			if i%2 == 1 {
				kind = "attach"
			}
			b, _ := json.Marshal(proto.Hello{Kind: kind, Version: proto.Version})
			s.Write(append(b, '\n'))
			r := bufio.NewReader(s)
			line, err := r.ReadBytes('\n')
			if err != nil {
				t.Error(err)
				return
			}
			var w proto.Welcome
			json.Unmarshal(line, &w)
			if w.App != kind+"/test" {
				t.Errorf("stream %d: welcome %+v", i, w)
			}
			// Much output, as a screen's first frame: it compresses, and
			// comes back whole.
			payload := bytes.Repeat([]byte(fmt.Sprintf("\x1b[38;5;%dmline %d of stream %d\r\n", i, i, i)), 20000)
			go s.Write(payload)
			got := make([]byte, len(payload))
			if _, err := io.ReadFull(r, got); err != nil || !bytes.Equal(got, payload) {
				t.Errorf("stream %d: echoed %d bytes, %v", i, len(got), err)
			}
		}()
	}
	wg.Wait()
	if _, err := c.Ping(); err != nil {
		t.Error(err)
	}
	ctx := context.Background()
	if err := c.Host(ctx, proto.HostRequest{Kind: proto.KindRestart}); err != nil {
		t.Error(err)
	}
	if err := c.Host(ctx, proto.HostRequest{Kind: proto.KindUpdate, App: "2.0"}); err != nil {
		t.Error(err)
	}
	if err := c.Host(ctx, proto.HostRequest{Kind: proto.KindUpdate, App: "bad"}); err == nil || !strings.Contains(err.Error(), "no such version") {
		t.Errorf("update to a bad version: %v", err)
	}
	c.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the server did not end with the connection")
	}
	select {
	case <-c.Done():
	default:
		t.Error("Done is not closed")
	}
}

func TestMuxRefusesHostRequests(t *testing.T) {
	s := echoServer(t)
	s.Restart, s.Update = nil, nil
	c, _ := dialPipe(t, s, "")
	defer c.Close()
	if err := c.Host(context.Background(), proto.HostRequest{Kind: proto.KindUpdate, App: "2.0"}); err == nil || !strings.Contains(err.Error(), "desktop app") {
		t.Errorf("update: %v", err)
	}
}

func TestMuxOtherVersion(t *testing.T) {
	cl, sv := pipes()
	go func() {
		br := bufio.NewReader(sv)
		br.ReadBytes('\n')
		b, _ := json.Marshal(proto.MuxWelcome{Mux: proto.MuxVersion + 1, Host: proto.HostInfo{Kero: "9.0"}})
		sv.Write(append(b, '\n'))
	}()
	_, err := handshake(context.Background(), cl, Addr{Scheme: "ssh", Host: "x"}, Options{})
	if err == nil || !strings.Contains(err.Error(), "Kero 9.0") {
		t.Errorf("a host of another mux version: %v", err)
	}
	cl, sv = pipes()
	go func() {
		// Not Kero at all: a shell that says nothing of it.
		bufio.NewReader(sv).ReadBytes('\n')
		io.WriteString(sv, strings.Repeat("hello\n", 200))
	}()
	if _, err := handshake(context.Background(), cl, Addr{Scheme: "ssh", Host: "x"}, Options{}); err == nil || !strings.Contains(err.Error(), "not Kero") {
		t.Errorf("not Kero: %v", err)
	}
}

func TestMuxHandshakeTimesOut(t *testing.T) {
	cl, _ := pipes()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := handshake(ctx, cl, Addr{Scheme: "ssh", Host: "x"}, Options{})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 2*time.Second {
		t.Errorf("a host that never answers: %v after %s", err, time.Since(start))
	}
}

func TestFlateConn(t *testing.T) {
	a, b := pipes()
	fa, fb := newFlateConn(a), newFlateConn(b)
	msgs := []string{"x", strings.Repeat("abc", 100000), "\x00\x01", "end"}
	go func() {
		for _, m := range msgs {
			fa.Write([]byte(m))
		}
	}()
	for _, m := range msgs {
		got := make([]byte, len(m))
		if _, err := io.ReadFull(fb, got); err != nil || string(got) != m {
			t.Fatalf("read %d bytes of %d: %v", len(got), len(m), err)
		}
	}
}
