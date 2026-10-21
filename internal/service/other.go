//go:build !linux && !darwin

package service

func file() string { return "" }

// Install, and the rest, are of systemd and launchd: on Windows, kerod
// starts with the app or a bridge.
func Install(string, func() error) error { return ErrUnsupported }
func EnableLinger() error                { return ErrUnsupported }
func Uninstall() error                   { return nil }
func Start() error                       { return ErrUnsupported }
func Restart() error                     { return ErrUnsupported }
func Stop() error                        { return ErrUnsupported }
func Get() Status                        { return Status{} }
