package proto

import (
	"encoding/binary"
	"errors"
	"io"
)

// Frames of an attach connection: a kind, the length of the payload as a
// big-endian uint32, and the payload.
const (
	// FrameOutput carries what the program wrote (daemon → client). The
	// first frames after the Welcome are the session's snapshot.
	FrameOutput byte = 'o'
	// FrameInput carries what the user typed (client → daemon).
	FrameInput byte = 'i'
	// FrameResize carries the terminal's size: cols and rows as big-endian
	// uint16s (client → daemon).
	FrameResize byte = 'r'
	// FrameClosed tells the client the session is gone (daemon → client).
	FrameClosed byte = 'c'
	// FrameResized tells the client where in the output the session's
	// terminal took a size, as FrameResize carries it (daemon → client):
	// the client's terminal takes it there too, so that both lay out each
	// byte at one size.
	FrameResized byte = 'z'
)

// maxFrame bounds a frame's payload, against a corrupt stream.
const maxFrame = 16 << 20

// WriteFrame writes one frame. Callers serialize writes to a connection.
func WriteFrame(w io.Writer, kind byte, payload []byte) error {
	buf := make([]byte, 5+len(payload))
	buf[0] = kind
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(payload)))
	copy(buf[5:], payload)
	_, err := w.Write(buf)
	return err
}

// ReadFrame reads one frame.
func ReadFrame(r io.Reader) (kind byte, payload []byte, err error) {
	var hdr [5]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(hdr[1:])
	if n > maxFrame {
		return 0, nil, errors.New("proto: frame too large")
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return hdr[0], payload, nil
}

// ResizePayload encodes a size for FrameResize.
func ResizePayload(cols, rows int) []byte {
	var b [4]byte
	binary.BigEndian.PutUint16(b[0:], uint16(max(1, min(cols, 0xFFFF))))
	binary.BigEndian.PutUint16(b[2:], uint16(max(1, min(rows, 0xFFFF))))
	return b[:]
}

// ParseResize decodes a FrameResize payload.
func ParseResize(p []byte) (cols, rows int, ok bool) {
	if len(p) != 4 {
		return 0, 0, false
	}
	return int(binary.BigEndian.Uint16(p[0:])), int(binary.BigEndian.Uint16(p[2:])), true
}
