package screen

import (
	"sync"
	"sync/atomic"

	"github.com/hinshun/vt10x"
	"github.com/host452b/yoyo/internal/logger"
)

// Screen wraps a vt10x terminal to provide a simple interface for feeding PTY
// data and retrieving visible text.
// All methods are goroutine-safe (SIGWINCH resize races with event loop writes).
type Screen struct {
	mu         sync.Mutex
	terminal   vt10x.Terminal
	panicCount int64 // incremented each time Feed recovers a vt10x panic
	log        *logger.Logger
}

// New creates a new Screen with the specified dimensions (cols, rows).
func New(cols, rows int) *Screen {
	terminal := vt10x.New(vt10x.WithSize(cols, rows))
	return &Screen{terminal: terminal}
}

// SetLogger attaches a logger for reporting recovered vt10x panics.
func (s *Screen) SetLogger(log *logger.Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log = log
}

// Feed writes raw PTY data into the terminal emulator.
// Recovers from panics in the vt10x library (e.g. cursor out-of-bounds).
func (s *Screen) Feed(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			atomic.AddInt64(&s.panicCount, 1)
			if s.log != nil {
				s.log.Errorf("vt10x panic recovered: %v", r)
			}
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
