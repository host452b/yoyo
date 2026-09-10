// internal/detector/codex_test.go
package detector_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/host452b/yoyo/internal/detector"
	"github.com/host452b/yoyo/internal/screen"
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
		"› 1. Yes, proceed (y)\n  2. No (esc)\n\n" +
		"  Press enter to confirm or esc to cancel\n"
	if d.Detect(p) == nil {
		t.Error("should detect edit prompt")
	}
}

func TestCodex_McpPrompt(t *testing.T) {
	d := detector.Codex{}
	p := "  MyServer needs your approval.\n\n" +
		"› 1. Yes, provide the requested info (y)\n  2. No, but continue without it (n)\n\n" +
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
	p := "  Would you like to run the following command?\n› 1. Yes, proceed\n  2. No\n" +
		"  Press enter to confirm or esc to cancel\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
}

func TestCodex_DefaultResponse(t *testing.T) {
	d := detector.Codex{}
	p := "  Would you like to run the following command?\n\n  $ go test\n\n" +
		"› 1. Yes, proceed (y)\n  2. No (esc)\n\n" +
		"  Press enter to confirm or esc to cancel\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.Response != "\r" {
		t.Errorf("Response = %q, want \\r", r.Response)
	}
}

func TestCodex_UpstreamSnapshots(t *testing.T) {
	paths, err := filepath.Glob("testdata/codex/*.txt")
	if err != nil || len(paths) != 7 {
		t.Fatalf("expected 7 upstream fixtures: %v, %v", paths, err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, cols := range []int{60, 80, 120} {
				t.Run(fmt.Sprint(cols), func(t *testing.T) {
					scr := screen.New(cols, 60)
					raw := []byte("\x1b[2J\x1b[H\x1b[1m" + strings.ReplaceAll(string(data), "\n", "\r\n") + "\x1b[0m")
					// Split UTF-8 markers and ANSI sequences as well as text.
					for _, b := range raw {
						scr.Feed([]byte{b})
					}
					r := (detector.Codex{}).Detect(scr.Text())
					if r == nil || r.Response != "\r" {
						t.Fatalf("snapshot not approved correctly: %+v\n%s", r, scr.Text())
					}
				})
			}
		})
	}
}

func TestCodex_StructuralMatching(t *testing.T) {
	base := codexPrompt("Would you like to run the following command?", "echo hello", []string{"Yes, proceed (y)", "No (esc)"})
	session := codexPrompt("Would you like to run the following command?", "echo hello", []string{"Yes, proceed (y)", "Yes, and don't ask again for this command in this session (a)", "No (esc)"})
	cases := []struct{ name, text, response string }{
		{"complete", base, "\r"},
		{"rejection selected", strings.ReplaceAll(strings.ReplaceAll(base, "› 1.", "  1."), "  2.", "› 2."), "y"},
		{"custom confirmation", strings.ReplaceAll(base, "enter to confirm", "space to confirm"), "y"},
		{"custom approval", strings.ReplaceAll(strings.ReplaceAll(base, "enter to confirm", "space to confirm"), "(y)", "(v)"), "v"},
		{"wrapped footer", strings.ReplaceAll(base, "or esc to cancel", "or esc to\n  cancel"), "\r"},
		{"wrapped label", strings.ReplaceAll(base, "Yes, proceed", "Yes,\n     proceed"), "\r"},
		{"wrapped title", strings.ReplaceAll(base, "following command", "following\n  command"), "\r"},
		{"session grant selected", strings.ReplaceAll(strings.ReplaceAll(session, "› 1.", "  1."), "  2.", "› 2."), "y"},
		{"enter assigned to rejection", strings.ReplaceAll(base, "(esc)", "(enter)"), "y"},
		{"enter assigned to cancel", strings.ReplaceAll(base, "esc to cancel", "enter to cancel"), "y"},
		{"approval key opens thread", strings.ReplaceAll(base, "enter to confirm", "space to confirm") + " or y to open thread", ""},
		{"off-target with no shortcut", strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(base, "› 1.", "  1."), "  2.", "› 2."), " (y)", ""), ""},
		{"header and footer only", "Would you like to run the following command?\nPress enter to confirm or esc to cancel", ""},
		{"footer ends before cancel binding", strings.Split(base, " or esc")[0], ""},
		{"missing reject", strings.ReplaceAll(base, "  2. No (esc)\n", ""), ""},
		{"missing selection", strings.ReplaceAll(base, "›", " "), ""},
		{"multiple selections", strings.ReplaceAll(base, "  2.", "› 2."), ""},
		{"number gap", strings.ReplaceAll(base, "  2.", "  3."), ""},
		{"unknown yes", strings.ReplaceAll(base, "Yes, proceed", "Yes, delete all records"), ""},
		{"persistent grant only", strings.ReplaceAll(base, "Yes, proceed", "Yes, and allow this host in the future"), ""},
		{"general question", strings.ReplaceAll(base, "Would you like to run the following command?", "Would you like to use dark mode?"), ""},
		{"quoted header", strings.ReplaceAll(base, "Would you like", "The documentation says: Would you like"), ""},
		{"quoted menu", "```text\n" + base + "```\n", ""},
		{"stale menu", base + "Working...\n", ""},
		{"new incomplete menu", base + "Would you like to make the following edits?\n", ""},
		{"old title with unrelated new menu", base + "\n" + strings.ReplaceAll(base, "Would you like to run the following command?", "Choose how to continue"), ""},
		{"unsupported keys", strings.ReplaceAll(strings.ReplaceAll(base, "enter to confirm", "ctrl + x to confirm"), "(y)", "(ctrl + y)"), ""},
		{"conflicting key", strings.ReplaceAll(strings.ReplaceAll(base, "enter to confirm", "space to confirm"), "(esc)", "(y)"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := (detector.Codex{}).Detect(tc.text)
			if tc.response == "" {
				if r != nil {
					t.Fatalf("unexpected match: %+v", r)
				}
			} else if r == nil || r.Response != tc.response {
				t.Fatalf("got %+v, want response %q", r, tc.response)
			}
		})
	}
}

func TestCodex_HashPreservesCommandAndIgnoresSelection(t *testing.T) {
	p := codexPrompt("Would you like to run the following command?", "echo '› keep  two spaces'", []string{"Yes, proceed (y)", "No (esc)"})
	d := detector.Codex{}
	r := d.Detect(p)
	moved := d.Detect(strings.ReplaceAll(strings.ReplaceAll(p, "› 1.", "  1."), "  2.", "› 2."))
	if r == nil || moved == nil {
		t.Fatal("expected complete menus")
	}
	t.Run("selection ignored", func(t *testing.T) {
		if r.Hash != moved.Hash {
			t.Error("selection changed prompt identity")
		}
	})
	for name, changedText := range map[string]string{
		"command marker preserved": strings.Replace(p, "'›", "'", 1),
		"quoted spaces preserved":  strings.Replace(p, "keep  two", "keep two", 1),
	} {
		t.Run(name, func(t *testing.T) {
			changed := d.Detect(changedText)
			if changed == nil {
				t.Fatal("expected complete menu")
			}
			if r.Hash == changed.Hash {
				t.Error("different commands shared an approval identity")
			}
		})
	}
}
