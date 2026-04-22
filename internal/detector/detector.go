// internal/detector/detector.go
package detector

import (
	"crypto/sha256"
	"fmt"
)

// MatchResult is returned when a permission prompt is detected.
type MatchResult struct {
	RuleName string // shown in status bar, e.g. "Claude"
	Response string // sent to child PTY, e.g. "\r", "2\r"
	Hash     string // sha256(prompt body) for memory deduplication
}

// Detector scans terminal screen text and returns a match if a prompt is found.
type Detector interface {
	Detect(screenText string) *MatchResult
}

// RuleChain tries each Detector in order, returning the first match.
type RuleChain []Detector

func (rc RuleChain) Detect(screenText string) *MatchResult {
	for _, d := range rc {
		if m := d.Detect(screenText); m != nil {
			return m
		}
	}
	return nil
}

// hashBody computes sha256 of text and returns the hex string.
func hashBody(body string) string {
	sum := sha256.Sum256([]byte(body))
	return fmt.Sprintf("%x", sum)
}

// HashBody is exported for callers outside this package (e.g. the proxy's
// fuzzy stability check) that need a stable identifier for deduped text.
func HashBody(body string) string { return hashBody(body) }
