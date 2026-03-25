package screen

import (
	"sync"
	"sync/atomic"

	"github.com/hinshun/vt10x"
)

// Screen wraps a vt10x terminal to provide a simple interface for feeding PTY
// data and retrieving visible text.
// All methods are goroutine-safe (SIGWINCH resize races with event loop writes).
type Screen struct {
	mu         sync.Mutex
	terminal   vt10x.Terminal
	panicCount int64 // incremented each time Feed recovers a vt10x panic
}

// New creates a new Screen with the specified dimensions (cols, rows).
func New(cols, rows int) *Screen {
	terminal := vt10x.New(vt10x.WithSize(cols, rows))
	return &Screen{terminal: terminal}
}

// Feed writes raw PTY data into the terminal emulator.
// Recovers from panics in the vt10x library (e.g. cursor out-of-bounds).
func (s *Screen) Feed(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if recover() != nil {
			atomic.AddInt64(&s.panicCount, 1)
		}
	}()
	s.terminal.Write(data)
}

// Text returns the visible text content of the screen, with ANSI sequences stripped.
func (s *Screen) Text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminal.String()
}

// Resize changes the screen dimensions.
func (s *Screen) Resize(cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.terminal.Resize(cols, rows)
}

// PanicCount returns the number of times Feed recovered from a vt10x panic.
func (s *Screen) PanicCount() int64 {
	return atomic.LoadInt64(&s.panicCount)
}
