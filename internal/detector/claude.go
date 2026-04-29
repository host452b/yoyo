// internal/detector/claude.go
package detector

import "strings"

// Claude detects Claude Code permission prompts.
//
// Matches the structured block delimited by a line of ─ characters (top)
// and "Esc to cancel" (bottom). Falls back to "Do you want to" as top
// boundary when the separator has scrolled off. Falls back to the last
// numbered "N. No" line as bottom when "Esc to cancel" is absent.
// Requires both Yes and No options to guard against partial renders.
type Claude struct{}

func (c Claude) Detect(screenText string) *MatchResult {
	lines := strings.Split(screenText, "\n")

	// Find bottom boundary: prefer "Esc to cancel", fall back to last "N. No" line
	bottomIdx, bottomInclusive := findClaudeBottom(lines)
	if bottomIdx < 0 {
		return nil
	}

	// Find top boundary: prefer ─ separator, fall back to "Do you want to"
	topIdx, topInclusive := findClaudeTop(lines, bottomIdx)
	if topIdx < 0 {
		return nil
	}

	bodyStart := topIdx
	if !topInclusive {
		bodyStart = topIdx + 1
	}
	bodyEnd := bottomIdx
	if bottomInclusive {
		bodyEnd = bottomIdx + 1
	}

	if bodyStart >= bodyEnd {
		return nil
	}

	var cleaned []string
	for _, line := range lines[bodyStart:bodyEnd] {
		l := strings.TrimSpace(strings.ReplaceAll(line, "❯", ""))
		l = strings.TrimSpace(l)
		if l != "" {
			cleaned = append(cleaned, l)
		}
	}
	body := strings.Join(cleaned, "\n")

	if !strings.Contains(body, "Yes") || !strings.Contains(body, "No") {
		return nil
	}

	// When the dialog offers "don't ask again", navigate to option 2 (↓↵) so
	// the command pattern is added to Claude's allowlist permanently, avoiding
	// repeated prompts for the same command in future sessions.
	response := "\r"
	if containsDontAskAgain(body) {
		response = "\x1b[B\r"
	}

	return &MatchResult{
		RuleName: "Claude",
		Response: response,
		Hash:     hashBody(body),
	}
}

// containsDontAskAgain reports whether the dialog body contains a
// "don’t ask again for: <pattern>" option (Claude Code 3-option UI).
// Matches by the unique substring "ask again for" to sidestep apostrophe
// encoding variants (ASCII ‘ vs Unicode U+2019).
func containsDontAskAgain(body string) bool {
	return strings.Contains(strings.ToLower(body), "ask again for")
}

// findClaudeBottom finds the bottom boundary index.
// Returns (idx, inclusive).
func findClaudeBottom(lines []string) (int, bool) {
	// Prefer "Esc to cancel" (exclusive)
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "Esc to cancel") {
			return i, false
		}
	}
	// Fall back: last "N. No" line (inclusive)
	for i := len(lines) - 1; i >= 0; i-- {
		t := strings.TrimSpace(strings.ReplaceAll(lines[i], "❯", ""))
		t = strings.TrimSpace(t)
		if len(t) >= 4 && t[0] >= '1' && t[0] <= '9' && strings.HasPrefix(t[1:], ". No") {
			return i, true
		}
	}
	return -1, false
}

// findClaudeTop finds the top boundary index above bottomIdx.
// Returns (idx, inclusive).
func findClaudeTop(lines []string, bottomIdx int) (int, bool) {
	// Prefer ─ separator (exclusive)
	for i := bottomIdx - 1; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if t != "" && isAllRune(t, '─') {
			return i, false
		}
	}
	// Fall back: "Do you want to" (inclusive)
	for i := bottomIdx - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "Do you want to") {
			return i, true
		}
	}
	return -1, false
}

func isAllRune(s string, r rune) bool {
	for _, c := range s {
		if c != r {
			return false
		}
	}
	return true
}
