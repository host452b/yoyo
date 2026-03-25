// internal/term/term_windows.go
//go:build windows

package term

// WatchResize is a no-op on Windows. go-pty handles ConPTY resize events.
func (t *Term) WatchResize(onResize func(cols, rows int)) func() {
	return func() {}
}
