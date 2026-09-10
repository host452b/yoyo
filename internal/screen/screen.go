package screen

import (
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/hinshun/vt10x"
	"github.com/host452b/yoyo/internal/logger"
)

// Screen wraps a vt10x terminal to provide a simple interface for feeding PTY
// data and retrieving visible text.
// All methods are goroutine-safe (SIGWINCH resize races with event loop writes).
type Screen struct {
	mu          sync.Mutex
	terminal    vt10x.Terminal
	panicCount  int64 // incremented each time Feed recovers a vt10x panic
	log         *logger.Logger
	pendingUTF8 []byte // incomplete trailing rune, at most utf8.UTFMax-1 bytes
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
	// vt10x.Write uses a fresh bytes.Reader per call and does not retain an
	// incomplete UTF-8 rune. PTY read boundaries can split any rune, including
	// the selection marker and box borders used by the detectors.
	if len(s.pendingUTF8) > 0 {
		data = append(s.pendingUTF8, data...)
	}
	end := len(data)
	if end > 0 {
		start := end - 1
		for start > 0 && end-start < utf8.UTFMax && !utf8.RuneStart(data[start]) {
			start--
		}
		if !utf8.FullRune(data[start:]) {
			end = start
		}
	}
	s.pendingUTF8 = append([]byte(nil), data[end:]...)
	s.terminal.Write(data[:end])
}

// Text returns the visible text content of the screen, with ANSI sequences stripped.
func (s *Screen) Text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminal.String()
}

// Snapshot captures text and cursor together so input checks cannot combine
// different terminal states during a resize or redraw.
type Snapshot struct {
	Text             string
	CursorX, CursorY int
	CursorVisible    bool
}

func (s *Screen) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	cursor := s.terminal.Cursor()
	return Snapshot{
		Text: s.terminal.String(), CursorX: cursor.X, CursorY: cursor.Y,
		CursorVisible: s.terminal.CursorVisible() && len(s.pendingUTF8) == 0,
	}
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
