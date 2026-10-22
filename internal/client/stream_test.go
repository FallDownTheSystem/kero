package client

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"kero/internal/proto"
)

// A stream hands over FrameResized where it is in the output: after the
// output before it is read, before the output after it.
func TestStreamResizedInOrder(t *testing.T) {
	var wire bytes.Buffer
	proto.WriteFrame(&wire, proto.FrameOutput, []byte("before"))
	proto.WriteFrame(&wire, proto.FrameResized, proto.ResizePayload(100, 30))
	proto.WriteFrame(&wire, proto.FrameOutput, []byte("after"))
	s := &Stream{r: bufio.NewReader(&wire), Gone: make(chan struct{})}
	var log []string
	s.OnResized = func(cols, rows int) {
		if cols != 100 || rows != 30 {
			t.Errorf("resized to %dx%d", cols, rows)
		}
		log = append(log, "resized")
	}
	buf := make([]byte, 64)
	for {
		n, err := s.Read(buf)
		if n > 0 {
			log = append(log, string(buf[:n]))
		}
		if err == io.EOF {
			break
		}
	}
	if got := len(log); got != 3 || log[0] != "before" || log[1] != "resized" || log[2] != "after" {
		t.Errorf("read %q, want before, resized, after", log)
	}
}

// Output past what a frame carries, as a terminal's frame with the pixels
// of an image, goes in frames of its own, which a stream reads as one.
func TestStreamLargeOutput(t *testing.T) {
	out := bytes.Repeat([]byte("0123456789abcdef"), 5<<20/16) // 5 MiB
	wire := bytes.NewReader(proto.AppendFrames(nil, proto.FrameOutput, out))
	frames := 0
	for {
		kind, payload, err := proto.ReadFrame(wire)
		if err != nil {
			break
		}
		if kind != proto.FrameOutput || len(payload) > 1<<20 {
			t.Fatalf("a frame of kind %q and %d bytes", kind, len(payload))
		}
		frames++
	}
	if frames < 5 {
		t.Errorf("%d frames", frames)
	}
	s := &Stream{r: bufio.NewReader(bytes.NewReader(proto.AppendFrames(nil, proto.FrameOutput, out))), Gone: make(chan struct{})}
	got, _ := io.ReadAll(s)
	if !bytes.Equal(got, out) {
		t.Errorf("read %d bytes, not the %d written", len(got), len(out))
	}
}

// Input takes the session's size back from another client that resized
// it, before it goes; a focus-out report does not, nor input at the size
// asked.
func TestStreamClaimsSize(t *testing.T) {
	var wire bytes.Buffer
	proto.WriteFrame(&wire, proto.FrameResized, proto.ResizePayload(100, 30))
	var sent bytes.Buffer
	s := &Stream{conn: writeConn{&sent}, r: bufio.NewReader(&wire), size: [2]int{80, 24}, Gone: make(chan struct{})}
	s.ResizePixels(80, 24, 9, 18)
	s.Write([]byte("a")) // at the size asked: no resize
	io.ReadAll(s)        // another client's size, 100x30
	s.Write([]byte("\x1b[O"))
	s.Write([]byte("b"))
	var got []string
	for {
		kind, payload, err := proto.ReadFrame(&sent)
		if err != nil {
			break
		}
		if kind == proto.FrameResize {
			c, r, cw, ch, _ := proto.ParseResizePixels(payload)
			got = append(got, fmt.Sprintf("resize %dx%d %dx%d", c, r, cw, ch))
		} else {
			got = append(got, fmt.Sprintf("%c %q", kind, payload))
		}
	}
	want := []string{"resize 80x24 9x18", fmt.Sprintf("%c %q", proto.FrameInput, "a"), fmt.Sprintf("%c %q", proto.FrameInput, "\x1b[O"), "resize 80x24 9x18", fmt.Sprintf("%c %q", proto.FrameInput, "b")}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("sent %q, want %q", got, want)
	}
}

// writeConn is a net.Conn that writes to a buffer.
type writeConn struct{ w *bytes.Buffer }

func (c writeConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c writeConn) Write(p []byte) (int, error)      { return c.w.Write(p) }
func (c writeConn) Close() error                     { return nil }
func (c writeConn) LocalAddr() net.Addr              { return nil }
func (c writeConn) RemoteAddr() net.Addr             { return nil }
func (c writeConn) SetDeadline(time.Time) error      { return nil }
func (c writeConn) SetReadDeadline(time.Time) error  { return nil }
func (c writeConn) SetWriteDeadline(time.Time) error { return nil }
