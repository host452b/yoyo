// internal/detector/fuzz_test.go
//
// Native Go fuzz tests for every built-in detector. The contract under
// test is: Detect/FuzzyMatch must return without panicking, regardless of
// input shape (malformed escape sequences, box-drawing near input bounds,
// extreme sizes, unicode edge cases, etc.). A positive match is fine;
// a panic is the failure mode.
//
// Run with:
//   go test ./internal/detector/ -run=^$ -fuzz=FuzzClaude -fuzztime=30s
package detector_test

import (
	"strings"
	"testing"

	"github.com/host452b/yoyo/internal/detector"
)

func FuzzClaude(f *testing.F) {
	// Seed with known-good prompts + adversarial shapes.
	f.Add("─\n Do you want to X\n 1. Yes\n 2. No\n Esc to cancel\n")
	f.Add("Esc to cancel")
	f.Add("Do you want to\n")
	f.Add("")
	f.Add(strings.Repeat("─", 10000))
	f.Add(strings.Repeat("\n", 10000))
	f.Add("❯ 1. No\nEsc to cancel\n")
	f.Add("\x00\x1b[\x1b[\x1b")
	f.Fuzz(func(t *testing.T, s string) {
		_ = detector.Claude{}.Detect(s)
	})
}

func FuzzCodex(f *testing.F) {
	f.Add("Would you like to X\nPress enter to confirm or esc to cancel\n")
	f.Add("needs your approval\nPress enter to confirm or esc to cancel\n")
	f.Add("Press enter to confirm or esc to cancel\n")
	f.Add("")
	f.Add(strings.Repeat("›", 5000))
	f.Add("Would you like to\n\n\nneeds your approval\n\nPress enter to confirm or esc to cancel\n")
	f.Fuzz(func(t *testing.T, s string) {
		_ = detector.Codex{}.Detect(s)
	})
}

func FuzzCursor(f *testing.F) {
	f.Add("┌─┐\n│ $ ls │\n└─┘\n Run this command?\n → Run (once) (y)\n Skip (esc or n)\n")
	f.Add("┌──┐\n│ (y) n) │\n└──┘\n")
	f.Add("┌\n└")
	f.Add("└┘┌┐")
	f.Add("")
	f.Add(strings.Repeat("┌─┐\n", 500) + strings.Repeat("└─┘\n", 500))
	f.Add("\xff\xfe\xfd(y)n)┌─┐└─┘")
	f.Fuzz(func(t *testing.T, s string) {
		_ = detector.Cursor{}.Detect(s)
	})
}

func FuzzFuzzyMatch(f *testing.F) {
	f.Add("continue (y/n)")
	f.Add("[Y/n] [y/N] [N/y] (y/n)")
	f.Add("yes/no")
	f.Add("")
	f.Add(strings.Repeat("yes/no\n", 1000))
	f.Add(strings.Repeat("(y/n)", 10000)) // regex backtracking check
	f.Add("\x00\n(y/n)\n\x00")
	f.Add(strings.Repeat("\n", 10000) + "(y/n)")
	f.Fuzz(func(t *testing.T, s string) {
		_ = detector.FuzzyMatch(s)
	})
}

// FuzzContainsDangerousCommand exercises the deletion-class pattern set
// against arbitrary input. The contract: ContainsDangerousCommand must
// never panic and must always return a valid (bool, string) where the
// string (if non-empty) is a genuine substring of the input — callers
// slice it into status bars and logs.
func FuzzContainsDangerousCommand(f *testing.F) {
	f.Add("rm -rf /")
	f.Add("git rm -r")
	f.Add("DROP TABLE users")
	f.Add("kubectl delete ns foo")
	f.Add("terraform destroy")
	f.Add("find / -delete")
	f.Add("")
	f.Add(strings.Repeat("rm -rf /\n", 1000))
	f.Add(strings.Repeat("kubectl delete ", 5000))
	f.Add("\x00\x01\x02\x03")
	f.Add("DROP " + strings.Repeat("TABLE ", 2000))
	f.Fuzz(func(t *testing.T, s string) {
		hit, matched := detector.ContainsDangerousCommand(s)
		if hit && !strings.Contains(s, matched) {
			t.Errorf("match snippet %q not a substring of input (len=%d)", matched, len(s))
		}
		if !hit && matched != "" {
			t.Errorf("miss but non-empty snippet %q", matched)
		}
	})
}
