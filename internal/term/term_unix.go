// internal/term/term_unix.go
//go:build !windows

package term

import (
	"os"
	"os/signal"
	"syscall"
)

// WatchResize calls onResize(cols, rows) whenever the terminal is resized.
// Returns a stop function. Call stop() to unsubscribe.
func (t *Term) WatchResize(onResize func(cols, rows int)) func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			cols, rows := t.Size()
			onResize(cols, rows)
		}
	}()
	return func() {
		signal.Stop(ch)
		close(ch)
	}
}
