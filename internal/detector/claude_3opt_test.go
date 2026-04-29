package detector_test

import (
	"testing"

	"github.com/host452b/yoyo/internal/detector"
	"github.com/host452b/yoyo/internal/screen"
)

// Regression probe for the newer 3-option Claude Code UI:
//
//   ❯ 1. Yes
//     2. Yes, and don't ask again for: <pattern>
//     3. No
//
// Earlier UI had only "1. Yes / 2. No" (locked in by Claude_test.go). The
// detector must still fire for the 3-option variant — body still contains
// both "Yes" and "No".
func TestClaude_3OptionPrompt_Bash(t *testing.T) {
	prompt := "──────────────────────────────────────────\r\n" +
		" Bash command\r\n" +
		"   pip3 show python-dotenv 2>/dev/null || echo \"dotenv not installed\"\r\n" +
		"   Check if python-dotenv is installed\r\n" +
		"\r\n" +
		" Do you want to proceed?\r\n" +
		" ❯ 1. Yes\r\n" +
		"   2. Yes, and don't ask again for: pip3 show *\r\n" +
		"   3. No\r\n" +
		"\r\n" +
		" Esc to cancel · Tab to amend · ctrl+e to explain\r\n"
	scr := screen.New(120, 40)
	scr.Feed([]byte(prompt))
	text := scr.Text()

	r := (detector.Claude{}).Detect(text)
	if r == nil {
		t.Fatalf("expected Claude detector to match 3-option UI; got nil.\n\nScreen text was:\n%s", text)
	}
	if r.Response != "\x1b[B\r" {
		t.Errorf("expected Response=\"\\x1b[B\\r\" (↓+Enter to select don't-ask-again), got %q", r.Response)
	}
}
