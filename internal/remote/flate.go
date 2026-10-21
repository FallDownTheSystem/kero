package remote

import (
	"compress/flate"
	"io"
	"sync"
)

// deflate is the compression both sides of a remote connection take:
// DEFLATE across everything it carries, flushed with every write so that
// nothing waits. A terminal's frames and kerod's state, sent whole on
// every change, are text that repeats itself, which it makes several
// times smaller.
const deflate = "deflate"

// flateConn compresses what goes through rwc both ways.
type flateConn struct {
	rwc io.ReadWriteCloser
	r   io.ReadCloser
	wmu sync.Mutex
	w   *flate.Writer
}

func newFlateConn(rwc io.ReadWriteCloser) *flateConn {
	// BestSpeed: the link is what is slow, not the machines.
	w, _ := flate.NewWriter(rwc, flate.BestSpeed)
	return &flateConn{rwc: rwc, r: flate.NewReader(rwc), w: w}
}

func (c *flateConn) Read(p []byte) (int, error) { return c.r.Read(p) }

func (c *flateConn) Write(p []byte) (int, error) {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	n, err := c.w.Write(p)
	if err != nil {
		return n, err
	}
	return n, c.w.Flush()
}

func (c *flateConn) Close() error {
	c.r.Close()
	return c.rwc.Close()
}
