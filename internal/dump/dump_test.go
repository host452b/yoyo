package dump_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/host452b/yoyo/internal/agent"
	"github.com/host452b/yoyo/internal/dump"
)

func TestRedactConfig_ReplacesResponseButKeepsPattern(t *testing.T) {
	in := `[defaults]
delay = 3

[[rules]]
name     = "confirm"
pattern  = "Are you sure"
response = "y\r"

[[rules]]
  name     = "deploy"
  pattern  = "Deploy to prod"
  response = "super-secret-api-key-masquerading-as-keystrokes\r"
`
	got := dump.RedactConfig(in)

	if strings.Contains(got, "super-secret-api-key") {
		t.Errorf("RedactConfig leaked a response value:\n%s", got)
	}
	if strings.Contains(got, `response = "y\r"`) {
		t.Errorf("RedactConfig should redact even innocuous responses uniformly:\n%s", got)
	}
	// Patterns must survive.
	if !strings.Contains(got, `pattern  = "Are you sure"`) {
		t.Errorf("RedactConfig damaged pattern field:\n%s", got)
	}
	// Exactly two response lines should be redacted.
	if c := strings.Count(got, `"<redacted>"`); c != 2 {
		t.Errorf("expected 2 redactions, got %d:\n%s", c, got)
	}
}

func TestFilteredEnv_RedactsSecrets(t *testing.T) {
	t.Setenv("YOYO_TEST_TOKEN", "tok123")
	t.Setenv("YOYO_TEST_PASSWORD", "pw")
	t.Setenv("YOYO_TEST_API_KEY", "ak")
	t.Setenv("YOYO_TEST_BEARER", "bb")
	t.Setenv("YOYO_TEST_HARMLESS", "hello-world")

	got := dump.FilteredEnv()

	for _, leak := range []string{"tok123", "pw", "ak", "bb"} {
		if strings.Contains(got, leak) {
			t.Errorf("FilteredEnv leaked secret value %q\n---\n%s", leak, got)
		}
	}
	// Keys should remain so the reader can see the variable existed.
	for _, k := range []string{"YOYO_TEST_TOKEN", "YOYO_TEST_PASSWORD",
		"YOYO_TEST_API_KEY", "YOYO_TEST_BEARER"} {
		if !strings.Contains(got, k+"=<redacted>") {
			t.Errorf("expected %s=<redacted>, missing from output", k)
		}
	}
	// Harmless var should be present verbatim.
	if !strings.Contains(got, "YOYO_TEST_HARMLESS=hello-world") {
		t.Error("harmless env var missing from output")
	}
}

func TestWrite_ProducesReadableMarkdownWithKeySections(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte(`[defaults]
delay = 2

[[rules]]
name     = "demo"
pattern  = "X"
response = "SHOULDNOTAPPEAR\r"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(t.TempDir(), "yoyo.log")
	if err := os.WriteFile(logPath, []byte("[INFO] hello\n[INFO] world\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(t.TempDir(), "dumps")
	path, err := dump.Write(dump.Snapshot{
		Version:       "v2.3.0-test",
		AgentCommand:  "claude --model opus",
		AgentKind:     agent.KindClaude,
		AgentPID:      12345,
		PTYCols:       120,
		PTYRows:       40,
		ScreenText:    "some stable screen\ncontinue (y/n)\n",
		ConfigPath:    cfgPath,
		LogPath:       logPath,
		ApprovalCount: 7,
		Delay:         3,
		Enabled:       true,
		AfkEnabled:    true,
		AfkIdle:       10 * time.Minute,
		FuzzyEnabled:  false,
		FuzzyStable:   3 * time.Second,
		SafetyEnabled: true,
	}, dir)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)

	// Required section headers.
	for _, must := range []string{
		"# yoyo diagnostic dump",
		"## Version",
		"## Runtime",
		"## Agent",
		"## Current screen",
		"## Config",
		"## Last 100 log lines",
		"## Environment",
	} {
		if !strings.Contains(body, must) {
			t.Errorf("dump missing section %q", must)
		}
	}

	// Version field populated.
	if !strings.Contains(body, "v2.3.0-test") {
		t.Error("dump missing version string")
	}

	// Screen content present verbatim.
	if !strings.Contains(body, "continue (y/n)") {
		t.Error("dump missing screen text")
	}

	// Config-response redacted.
	if strings.Contains(body, "SHOULDNOTAPPEAR") {
		t.Errorf("dump leaked config response value:\n%s", body)
	}
	if !strings.Contains(body, `response = "<redacted>"`) {
		t.Error("expected redacted response line in dump")
	}

	// Log tail present.
	if !strings.Contains(body, "[INFO] hello") || !strings.Contains(body, "[INFO] world") {
		t.Error("dump missing log tail")
	}

	// File permissions.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("dump file mode %o, want 0600", mode)
	}
}

func TestWrite_CreatesDirectory(t *testing.T) {
	// Nested non-existent path — Write should create it.
	dir := filepath.Join(t.TempDir(), "nested", "dumps")
	path, err := dump.Write(dump.Snapshot{Version: "v"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("dump file not found: %v", err)
	}
}
