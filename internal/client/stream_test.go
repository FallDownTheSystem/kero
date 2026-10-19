package client

import (
	"bufio"
	"bytes"
	"io"
	"testing"

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
