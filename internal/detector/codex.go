// internal/detector/codex.go
package detector

import "strings"

var codexStartPatterns = []string{
	"Would you like to",
	"needs your approval",
}

const codexFooter = "Press enter to confirm or esc to cancel"

// Codex detects Codex CLI permission prompts.
type Codex struct{}

func (c Codex) Detect(screenText string) *MatchResult {
	lines := strings.Split(screenText, "\n")

	// Find last footer line
	footerIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], codexFooter) {
			footerIdx = i
			break
		}
	}
	if footerIdx < 0 {
		return nil
	}

	// Find nearest start pattern above footer
	startIdx := -1
	for i := footerIdx - 1; i >= 0; i-- {
		for _, pat := range codexStartPatterns {
			if strings.Contains(lines[i], pat) {
				startIdx = i
				break
			}
		}
		if startIdx >= 0 {
			break
		}
	}
	if startIdx < 0 {
		return nil
	}

	var cleaned []string
	for _, line := range lines[startIdx:footerIdx] {
		l := strings.TrimSpace(strings.ReplaceAll(line, "›", ""))
		l = strings.TrimSpace(l)
		if l != "" {
			cleaned = append(cleaned, l)
		}
	}
	body := strings.Join(cleaned, "\n")
	if body == "" {
		return nil
	}

	return &MatchResult{
		RuleName: "Codex",
		Response: "\r",
		Hash:     hashBody(body),
	}
}
