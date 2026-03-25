// internal/term/term.go
package term

import (
	"os"

	"golang.org/x/term"
)

// Term manages raw mode for a terminal file descriptor.
type Term struct {
	file     *os.File
	oldState *term.State
}

// New creates a Term wrapping the given file (typically os.Stdin).
func New(f *os.File) *Term {
	return &Term{file: f}
}

// NewNoOp creates a no-op Term for use in tests where no real TTY is available.
// Restore() and Size() are safe to call on a no-op Term.
func NewNoOp() *Term {
	return &Term{}
}

// EnableRaw switches the terminal to raw mode.
// Call Restore() when done (typically via defer).
func (t *Term) EnableRaw() error {
	if t.file == nil {
		return nil // no-op
	}
	state, err := term.MakeRaw(int(t.file.Fd()))
	if err != nil {
		return err
	}
	t.oldState = state
	return nil
}

// Restore returns the terminal to its original mode.
// Safe to call even if EnableRaw was never called or file is nil.
func (t *Term) Restore() {
	if t.oldState != nil && t.file != nil {
		term.Restore(int(t.file.Fd()), t.oldState)
	}
}

// Size returns the current terminal dimensions (cols, rows).
// Returns (80, 24) if file is nil or size cannot be determined.
func (t *Term) Size() (cols, rows int) {
	if t.file == nil {
		return 80, 24
	}
	w, h, err := term.GetSize(int(t.file.Fd()))
	if err != nil {
		return 80, 24 // safe default
	}
	return w, h
}
