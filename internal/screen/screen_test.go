// internal/screen/screen_test.go
package screen_test

import (
	"strings"
	"testing"

	"yoyo/internal/screen"
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
