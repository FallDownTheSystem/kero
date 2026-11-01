package main

import (
	"errors"
	"syscall"
)

// connReset tells a connection the other end reset: on Windows, Winsock's
// error, which syscall.ECONNRESET, an errno of Go's own there, is not.
func connReset(err error) bool {
	return errors.Is(err, syscall.WSAECONNRESET) || errors.Is(err, syscall.ECONNRESET)
}
