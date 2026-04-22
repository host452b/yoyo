// internal/detector/fuzzy.go
package detector

import (
	"regexp"
	"strings"
)

// fuzzyPattern matches precise y/n prompt markers. Intentionally narrow:
// bare "Yes" / "No" / "enter" / "confirm" are excluded because they
// appear too often in normal prose and code.
var fuzzyPattern = regexp.MustCompile(`(?i)\([yn]/[yn]\)|\[[yn]/[yn]\]|[yn]/[yn]\?|yes/no`)

// FuzzyMatch reports whether the last 15 lines of text contain a precise
// y/n prompt marker. The last-15-lines window keeps the detector focused
// on the currently visible prompt area. Trailing blank lines (common in
// padded screen buffers) are stripped before the window is applied.
func FuzzyMatch(text string) bool {
	lines := strings.Split(text, "\n")
	// Drop trailing blank/whitespace-only lines so the window covers real content.
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	start := 0
	if end > 15 {
		start = end - 15
	}
	return fuzzyPattern.MatchString(strings.Join(lines[start:end], "\n"))
}
