package screen

import "github.com/hinshun/vt10x"

// Screen wraps a vt10x terminal to provide a simple interface for feeding PTY
// data and retrieving visible text.
type Screen struct {
	terminal vt10x.Terminal
}

// New creates a new Screen with the specified dimensions (cols, rows).
func New(cols, rows int) *Screen {
	terminal := vt10x.New(vt10x.WithSize(cols, rows))
	return &Screen{terminal: terminal}
}

// Feed writes raw PTY data into the terminal emulator.
func (s *Screen) Feed(data []byte) {
	s.terminal.Write(data)
}

// Text returns the visible text content of the screen, with ANSI sequences stripped.
func (s *Screen) Text() string {
	return s.terminal.String()
}

// Resize changes the screen dimensions.
func (s *Screen) Resize(cols, rows int) {
	s.terminal.Resize(cols, rows)
}
