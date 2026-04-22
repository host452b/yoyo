// internal/screen/fuzz_test.go
//
// Contract under test: Screen.Feed must never propagate a panic out,
// regardless of the raw byte sequence fed in. vt10x has occasional cursor
// out-of-bounds / malformed-DEC panics; Screen wraps each Feed in
// recover() so callers don't have to care. This fuzz target hammers that
// guarantee with arbitrary byte streams.
//
// Run:
//   go test ./internal/screen/ -run=^$ -fuzz=FuzzScreenFeed -fuzztime=30s
package screen_test

import (
	"testing"

	"github.com/host452b/yoyo/internal/screen"
)

func FuzzScreenFeed(f *testing.F) {
	// Seed with classic problem shapes: cursor moves, unterminated OSC,
	// huge parameter counts, DECALN, set-mode sequences, malformed CSI.
	f.Add([]byte("\x1b[2J\x1b[H"))
	f.Add([]byte("\x1b[999;999H"))
	f.Add([]byte("\x1b]0;title\x07"))
	f.Add([]byte("\x1b[?1049h"))
	f.Add([]byte("\x1b[\x1b["))
	f.Add([]byte("\x1b#8"))
	f.Add([]byte("\x1b[6n"))
	f.Add([]byte(""))
	f.Add([]byte("\x00\x01\x02"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		s := screen.New(80, 24)
		// Must not panic out of Feed even with adversarial bytes.
		s.Feed(data)
		// Text() must always return a string (it may be empty, but never panic).
		_ = s.Text()
	})
}
