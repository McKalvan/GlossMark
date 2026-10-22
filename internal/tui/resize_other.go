//go:build windows

package tui

// onResize: Windows has no SIGWINCH; the frame takes the new size on the
// next key.
func onResize(func()) (stop func()) { return func() {} }
