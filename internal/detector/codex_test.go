// internal/detector/codex_test.go
package detector_test

import (
	"fmt"
	"testing"

	"yoyo/internal/detector"
)

func codexPrompt(action, command string, options []string) string {
	s := "  " + action + "\n\n"
	s += "  $ " + command + "\n\n"
	for i, opt := range options {
		if i == 0 {
			s += "› " + fmt.Sprintf("%d", i+1) + ". " + opt + "\n"
		} else {
			s += "  " + fmt.Sprintf("%d", i+1) + ". " + opt + "\n"
		}
	}
	s += "\n  Press enter to confirm or esc to cancel\n"
	return s
}

func TestCodex_CommandPrompt(t *testing.T) {
	d := detector.Codex{}
	p := "  Would you like to run the following command?\n\n  $ cargo test\n\n" +
		"› 1. Yes, proceed (y)\n  2. No (esc)\n\n" +
		"  Press enter to confirm or esc to cancel\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.RuleName != "Codex" {
		t.Errorf("RuleName = %q, want 'Codex'", r.RuleName)
	}
}

func TestCodex_EditPrompt(t *testing.T) {
	d := detector.Codex{}
	p := "  Would you like to make the following edits?\n\n  file.rs\n\n" +
		"  Press enter to confirm or esc to cancel\n"
	if d.Detect(p) == nil {
		t.Error("should detect edit prompt")
	}
}

func TestCodex_McpPrompt(t *testing.T) {
	d := detector.Codex{}
	p := "  MyServer needs your approval.\n\n" +
		"  Press enter to confirm or esc to cancel\n"
	if d.Detect(p) == nil {
		t.Error("should detect MCP approval prompt")
	}
}

func TestCodex_NoMatch(t *testing.T) {
	d := detector.Codex{}
	if d.Detect("Hello world") != nil {
		t.Error("should not detect random text")
	}
}

func TestCodex_NoFooter(t *testing.T) {
	d := detector.Codex{}
	if d.Detect("Would you like to run the following command?") != nil {
		t.Error("should not detect without footer")
	}
}

func TestCodex_StripsSelectionMarker(t *testing.T) {
	d := detector.Codex{}
	p := "  Would you like to run this?\n› 1. Yes\n  2. No\n" +
		"  Press enter to confirm or esc to cancel\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
}

func TestCodex_DefaultResponse(t *testing.T) {
	d := detector.Codex{}
	p := "  Would you like to run the following command?\n\n  $ go test\n\n" +
		"  Press enter to confirm or esc to cancel\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.Response != "\r" {
		t.Errorf("Response = %q, want \\r", r.Response)
	}
}
