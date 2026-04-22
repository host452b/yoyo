// internal/detector/fuzzy_test.go
package detector_test

import (
	"testing"

	"github.com/host452b/yoyo/internal/detector"
)

func TestFuzzyMatch_Hits(t *testing.T) {
	cases := []string{
		"do you want to proceed (y/n)",
		"are you sure? (Y/n)",
		"delete? (y/N)",
		"[y/n]",
		"[Y/n]",
		"continue y/n?",
		"Yes/No",
		"yes/no",
	}
	for _, c := range cases {
		if !detector.FuzzyMatch(c) {
			t.Errorf("expected fuzzy hit for %q", c)
		}
	}
}

func TestFuzzyMatch_Misses(t *testing.T) {
	// None of these should match — they're prose/logs/code.
	cases := []string{
		"All tests passed: 5 yes, 0 no",
		"function hasYes(input) { return input }",
		"Yes",
		"No",
		"enter",
		"press enter to continue",
		"(y)",      // Cursor already handles this
		"confirm",
	}
	for _, c := range cases {
		if detector.FuzzyMatch(c) {
			t.Errorf("expected fuzzy miss for %q", c)
		}
	}
}

func TestFuzzyMatch_OnlyScansLast15Lines(t *testing.T) {
	// A y/n prompt 20 lines up should NOT match (scrolled off active area).
	var sb []byte
	sb = append(sb, []byte("continue (y/n)\n")...)
	for i := 0; i < 20; i++ {
		sb = append(sb, []byte("filler line\n")...)
	}
	if detector.FuzzyMatch(string(sb)) {
		t.Error("expected fuzzy miss — y/n marker is outside the last 15 lines")
	}
}
