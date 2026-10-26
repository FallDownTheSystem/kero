//go:build !windows

package main

import (
	"errors"
	"syscall"
)

// connReset tells a connection the other end reset.
func connReset(err error) bool { return errors.Is(err, syscall.ECONNRESET) }
