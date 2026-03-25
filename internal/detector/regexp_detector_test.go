// internal/detector/regexp_detector_test.go
package detector_test

import (
	"testing"

	"yoyo/internal/detector"
)

func TestRegexpDetector_Matches(t *testing.T) {
	d, err := detector.NewRegexpDetector("confirm", "Are you sure", "y\r")
	if err != nil {
		t.Fatal(err)
	}
	r := d.Detect("some text\nAre you sure you want to proceed?\nmore text")
	if r == nil {
		t.Fatal("expected match")
	}
	if r.RuleName != "confirm" {
		t.Errorf("RuleName = %q, want 'confirm'", r.RuleName)
	}
	if r.Response != "y\r" {
		t.Errorf("Response = %q, want 'y\\r'", r.Response)
	}
}

func TestRegexpDetector_NoMatch(t *testing.T) {
	d, _ := detector.NewRegexpDetector("confirm", "Are you sure", "y\r")
	if d.Detect("nothing relevant here") != nil {
		t.Error("expected no match")
	}
}

func TestRegexpDetector_HashStable(t *testing.T) {
	d, _ := detector.NewRegexpDetector("test", "pattern", "\r")
	text := "found pattern in text"
	r1 := d.Detect(text)
	r2 := d.Detect(text)
	if r1.Hash != r2.Hash {
		t.Error("hash must be stable for same input")
	}
}

func TestRegexpDetector_InvalidPattern(t *testing.T) {
	_, err := detector.NewRegexpDetector("bad", "[invalid", "\r")
	if err == nil {
		t.Error("expected error for invalid regexp")
	}
}
