// internal/detector/regexp_detector.go
package detector

import (
	"fmt"
	"regexp"
)

// RegexpDetector matches screen text against a compiled regexp.
type RegexpDetector struct {
	name     string
	pattern  *regexp.Regexp
	response string
}

func NewRegexpDetector(name, pattern, response string) (*RegexpDetector, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}
	return &RegexpDetector{name: name, pattern: re, response: response}, nil
}

func (d *RegexpDetector) Detect(screenText string) *MatchResult {
	match := d.pattern.FindString(screenText)
	if match == "" {
		return nil
	}
	return &MatchResult{
		RuleName: d.name,
		Response: d.response,
		Hash:     hashBody(match),
	}
}
