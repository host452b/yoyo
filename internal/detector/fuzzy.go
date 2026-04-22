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
// on the currently visible prompt area.
func FuzzyMatch(text string) bool {
	lines := strings.Split(text, "\n")
	start := 0
	if len(lines) > 15 {
		start = len(lines) - 15
	}
	return fuzzyPattern.MatchString(strings.Join(lines[start:], "\n"))
}
