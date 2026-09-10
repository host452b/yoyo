// internal/screen/screen_test.go
package screen_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/host452b/yoyo/internal/screen"
)

func TestScreen_FeedAndText(t *testing.T) {
	s := screen.New(80, 24)
	s.Feed([]byte("Hello, World!"))
	text := s.Text()
	if !strings.Contains(text, "Hello, World!") {
		t.Errorf("expected 'Hello, World!' in screen text, got: %q", text)
	}
}

func TestScreen_StripANSI(t *testing.T) {
	s := screen.New(80, 24)
	// Write text with color codes
	s.Feed([]byte("\x1b[31mRed text\x1b[0m normal"))
	text := s.Text()
	if !strings.Contains(text, "Red text") {
		t.Errorf("screen text should contain stripped text 'Red text', got: %q", text)
	}
	if strings.Contains(text, "\x1b") {
		t.Error("screen text should not contain ANSI escape sequences")
	}
}

func TestScreen_Resize(t *testing.T) {
	s := screen.New(80, 24)
	// Should not panic
	s.Resize(132, 50)
	s.Feed([]byte("after resize"))
	if !strings.Contains(s.Text(), "after resize") {
		t.Error("should work after resize")
	}
}

func TestScreen_MultipleFeeds(t *testing.T) {
	s := screen.New(80, 24)
	s.Feed([]byte("first"))
	s.Feed([]byte(" second"))
	if !strings.Contains(s.Text(), "first") {
		t.Error("should retain first feed")
	}
}

func TestScreen_SplitUTF8AndANSI(t *testing.T) {
	raw := []byte("\x1b[2J\x1b[H\x1b[32m› 1. Yes 确认 ❯ ─ 😀\x1b[0m\r\n  2. No")
	whole := screen.New(80, 24)
	whole.Feed(raw)
	for split := 1; split < len(raw); split++ {
		s := screen.New(80, 24)
		s.Feed(raw[:split])
		s.Feed(raw[split:])
		if s.Text() != whole.Text() {
			t.Fatalf("split at byte %d changed rendered text:\n%q\nwant\n%q", split, s.Text(), whole.Text())
		}
	}
}

func TestScreen_StreamEquivalence(t *testing.T) {
	// Representative existing agent layouts, adapted to harmless sample bodies.
	frames := map[string]string{
		"codex":  "Would you like to run the following command?\r\n\r\n$ echo 确认\r\n\r\n› 1. Yes, proceed (y)\r\n  2. No (esc)\r\n\r\nPress enter to confirm or esc to cancel",
		"claude": "────────────────────\r\nRead /tmp/check\r\n  1. Yes\r\n❯ 2. Yes, don't ask again for: Read *\r\n  3. No\r\nEsc to cancel",
		"cursor": "┌──────────────────┐\r\n│ Run: echo 确认   │\r\n└──────────────────┘\r\n→ Run (once) (y)\r\nSkip (esc or n)",
	}
	for name, text := range frames {
		raw := []byte("\x1b[2J\x1b[H\x1b[32m" + text + "\x1b[0m")
		whole := screen.New(120, 30)
		whole.Feed(raw)
		for _, chunk := range []int{1, 2, 3, 7, 4096} {
			t.Run(fmt.Sprintf("%s/chunk_%d", name, chunk), func(t *testing.T) {
				split := screen.New(120, 30)
				for start := 0; start < len(raw); start += chunk {
					end := start + chunk
					if end > len(raw) {
						end = len(raw)
					}
					split.Feed(raw[start:end])
				}
				if split.Text() != whole.Text() {
					t.Fatal("PTY read boundaries changed visible characters")
				}
			})
		}
	}
}
