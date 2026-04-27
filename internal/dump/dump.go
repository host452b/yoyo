// Package dump collects a point-in-time diagnostic snapshot of a running
// yoyo and serialises it as Markdown. Users trigger it at runtime via
// `Ctrl+Y d`; the resulting file is self-contained (everything yoyo knew
// about its state at that moment) and safe-ish to share — secret-looking
// environment variables and custom-rule response fields are redacted
// before write. The "Current screen" section is *not* redacted, because
// that's the entire point of the dump. Users should eyeball a dump
// before pasting it to strangers.
package dump

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/host452b/yoyo/internal/agent"
	"github.com/host452b/yoyo/internal/detector"
)

// DetectorProbe names a detector from the active rule chain so dumps can show
// how each rule evaluated the exact screen yoyo was seeing.
type DetectorProbe struct {
	Label    string
	Detector detector.Detector
}

// DetectorDiagnostic is one detector's point-in-time evaluation result.
type DetectorDiagnostic struct {
	Label    string
	Matched  bool
	RuleName string
	Response string
	Hash     string
}

// Diagnostics captures detector/fallback/safety decisions for a screen.
type Diagnostics struct {
	Detectors     []DetectorDiagnostic
	FuzzyMatched  bool
	SafetyBlocked bool
	SafetySnippet string
}

// Snapshot is the subset of yoyo's runtime state that a dump captures.
// All fields are optional; zero values render as "n/a" or are omitted.
type Snapshot struct {
	Version string // yoyo version string (e.g. "v2.2.5" or "dev")

	AgentCommand string     // first argv element passed to yoyo (the wrapped command)
	AgentKind    agent.Kind // resolved agent type
	AgentPID     int        // child process pid (0 = unknown / already exited)

	PTYCols int
	PTYRows int

	ScreenText  string // current vt10x-rendered visible screen
	Diagnostics *Diagnostics

	// ConfigPath is read during Write to include the raw TOML (with
	// response fields redacted). Empty string = no config loaded.
	ConfigPath string

	// LogPath is read during Write to include the last 100 lines.
	// Empty string = no log.
	LogPath string

	ApprovalCount int64
	Delay         int
	Enabled       bool
	DryRun        bool
	AfkEnabled    bool
	AfkIdle       time.Duration
	FuzzyEnabled  bool
	FuzzyStable   time.Duration
	SafetyEnabled bool
}

// NewDiagnostics evaluates probes, fuzzy fallback, and safety guard against the
// exact screen text used in a dump.
func NewDiagnostics(screenText string, probes []DetectorProbe) Diagnostics {
	out := Diagnostics{
		Detectors:    make([]DetectorDiagnostic, 0, len(probes)),
		FuzzyMatched: detector.FuzzyMatch(screenText),
	}
	for i, probe := range probes {
		label := probe.Label
		if label == "" {
			label = fmt.Sprintf("detector %d", i+1)
		}
		diag := DetectorDiagnostic{Label: label}
		if probe.Detector != nil {
			if match := probe.Detector.Detect(screenText); match != nil {
				diag.Matched = true
				diag.RuleName = match.RuleName
				diag.Response = describeResponse(match.Response)
				diag.Hash = match.Hash
			}
		}
		out.Detectors = append(out.Detectors, diag)
	}
	out.SafetyBlocked, out.SafetySnippet = detector.ContainsDangerousCommand(screenText)
	return out
}

// Write serialises s as Markdown into a timestamped file under dir.
// Returns the absolute file path. Creates dir with mode 0700 if it
// doesn't exist. The file itself is created with mode 0600.
func Write(s Snapshot, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("dump: mkdir %s: %w", dir, err)
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	path := filepath.Join(dir, "yoyo-"+ts+".md")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := writeMarkdown(f, s); err != nil {
		return "", err
	}
	return path, nil
}

func writeMarkdown(w io.Writer, s Snapshot) error {
	bw := bufio.NewWriter(w)

	fmt.Fprintf(bw, "# yoyo diagnostic dump — %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintln(bw, "> ⚠️  **This dump contains your current screen content.** It is safe to keep")
	fmt.Fprintln(bw, "> locally but **review it before sharing** — the screen may include paths,")
	fmt.Fprintln(bw, "> command output, or anything else the wrapped agent had visible.")
	fmt.Fprintln(bw, "> Environment variables that look like secrets (TOKEN / PASSWORD / KEY /")
	fmt.Fprintln(bw, "> SECRET / API* / CREDENTIAL / AUTH) and `response =` fields from your")
	fmt.Fprintln(bw, "> config are auto-redacted below.")
	fmt.Fprintln(bw)

	// --- Version ----------------------------------------------------
	fmt.Fprintln(bw, "## Version")
	fmt.Fprintln(bw)
	ver := s.Version
	if ver == "" {
		ver = "(unknown)"
	}
	fmt.Fprintf(bw, "- yoyo: %s\n", ver)
	fmt.Fprintf(bw, "- go: %s  %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	if v := os.Getenv("TERM"); v != "" {
		fmt.Fprintf(bw, "- TERM: %s\n", v)
	}
	if _, ok := os.LookupEnv("TMUX"); ok {
		fmt.Fprintln(bw, "- tmux: yes")
	} else {
		fmt.Fprintln(bw, "- tmux: no")
	}
	fmt.Fprintln(bw)

	// --- Runtime ----------------------------------------------------
	fmt.Fprintln(bw, "## Runtime")
	fmt.Fprintln(bw)
	fmt.Fprintf(bw, "- enabled: %v\n", s.Enabled)
	fmt.Fprintf(bw, "- delay: %ds\n", s.Delay)
	fmt.Fprintf(bw, "- dry-run: %v\n", s.DryRun)
	fmt.Fprintf(bw, "- afk: enabled=%v  idle=%s\n", s.AfkEnabled, s.AfkIdle)
	fmt.Fprintf(bw, "- fuzzy: enabled=%v  stable=%s\n", s.FuzzyEnabled, s.FuzzyStable)
	fmt.Fprintf(bw, "- safety: %v\n", s.SafetyEnabled)
	fmt.Fprintf(bw, "- approvals this session: %d\n", s.ApprovalCount)
	fmt.Fprintln(bw)

	// --- Agent ------------------------------------------------------
	fmt.Fprintln(bw, "## Agent")
	fmt.Fprintln(bw)
	cmd := s.AgentCommand
	if cmd == "" {
		cmd = "(unknown)"
	}
	fmt.Fprintf(bw, "- command: `%s`\n", cmd)
	fmt.Fprintf(bw, "- kind: %s\n", s.AgentKind)
	fmt.Fprintf(bw, "- pid: %d\n", s.AgentPID)
	fmt.Fprintf(bw, "- pty: %dx%d\n", s.PTYCols, s.PTYRows)
	fmt.Fprintln(bw)

	// --- Current screen ---------------------------------------------
	fmt.Fprintln(bw, "## Current screen (vt10x-rendered — what yoyo sees)")
	fmt.Fprintln(bw)
	fmt.Fprintln(bw, "```text")
	fmt.Fprintln(bw, strings.TrimRight(s.ScreenText, "\n"))
	fmt.Fprintln(bw, "```")
	fmt.Fprintln(bw)

	if s.Diagnostics != nil {
		writeDetectorDiagnostics(bw, *s.Diagnostics)
	}
	writeLineNumberedScreen(bw, s.ScreenText)

	// --- Config -----------------------------------------------------
	if s.ConfigPath != "" {
		if data, err := os.ReadFile(s.ConfigPath); err == nil {
			fmt.Fprintf(bw, "## Config (`%s`, response fields redacted)\n\n", s.ConfigPath)
			fmt.Fprintln(bw, "```toml")
			fmt.Fprintln(bw, strings.TrimRight(RedactConfig(string(data)), "\n"))
			fmt.Fprintln(bw, "```")
		} else {
			fmt.Fprintf(bw, "## Config (`%s`)\n\n(read error: %v)\n", s.ConfigPath, err)
		}
		fmt.Fprintln(bw)
	}

	// --- Log tail ---------------------------------------------------
	if s.LogPath != "" {
		fmt.Fprintf(bw, "## Last 100 log lines (`%s`)\n\n", s.LogPath)
		if tail, err := readLogTail(s.LogPath, 100); err == nil {
			fmt.Fprintln(bw, "```text")
			fmt.Fprintln(bw, tail)
			fmt.Fprintln(bw, "```")
		} else {
			fmt.Fprintf(bw, "(read error: %v)\n", err)
		}
		fmt.Fprintln(bw)
	}

	// --- Environment ------------------------------------------------
	fmt.Fprintln(bw, "## Environment (filtered)")
	fmt.Fprintln(bw)
	fmt.Fprintln(bw, "```")
	fmt.Fprintln(bw, FilteredEnv())
	fmt.Fprintln(bw, "```")

	return bw.Flush()
}

func writeDetectorDiagnostics(w io.Writer, d Diagnostics) {
	fmt.Fprintln(w, "## Detector diagnostics")
	fmt.Fprintln(w)
	if len(d.Detectors) == 0 {
		fmt.Fprintln(w, "- detectors: none")
	} else {
		for _, probe := range d.Detectors {
			if !probe.Matched {
				fmt.Fprintf(w, "- %s: no match\n", probe.Label)
				continue
			}
			fmt.Fprintf(w, "- %s: matched\n", probe.Label)
			fmt.Fprintf(w, "  - rule: %s\n", probe.RuleName)
			fmt.Fprintf(w, "  - response: %s\n", probe.Response)
			fmt.Fprintf(w, "  - hash: `%s`\n", probe.Hash)
		}
	}
	fmt.Fprintf(w, "- fuzzy: matched=%v\n", d.FuzzyMatched)
	fmt.Fprintf(w, "- safety: blocked=%v\n", d.SafetyBlocked)
	if d.SafetyBlocked {
		fmt.Fprintf(w, "  - snippet: `%s`\n", d.SafetySnippet)
	}
	fmt.Fprintln(w)
}

func writeLineNumberedScreen(w io.Writer, screenText string) {
	fmt.Fprintln(w, "## Repro screen (line-numbered)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "```text")
	fmt.Fprintln(w, numberedScreen(screenText))
	fmt.Fprintln(w, "```")
	fmt.Fprintln(w)
}

func numberedScreen(screenText string) string {
	text := strings.TrimRight(screenText, "\n")
	if text == "" {
		return "   1 | "
	}
	lines := strings.Split(text, "\n")
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%4d | %s", i+1, line)
	}
	return b.String()
}

func describeResponse(response string) string {
	switch response {
	case "\r":
		return "enter"
	case "y\r":
		return "y + enter"
	case "n\r":
		return "n + enter"
	case "p\r":
		return "p + enter"
	default:
		return fmt.Sprintf("<%d bytes, redacted>", len(response))
	}
}

// --- exported helpers for testing ------------------------------------------

// responseLineRe matches a `response = "…"` TOML assignment at any indent.
// The capture group preserves whatever preceded the value so replacement is
// idempotent and minimally invasive.
var responseLineRe = regexp.MustCompile(`(?m)^(\s*response\s*=\s*).*$`)

// RedactConfig replaces every `response = "…"` line with
// `response = "<redacted>"`. Rule patterns are kept — they're matched
// against the screen text and aren't secrets on their own.
func RedactConfig(toml string) string {
	return responseLineRe.ReplaceAllString(toml, `${1}"<redacted>"`)
}

var secretKeyRe = regexp.MustCompile(`(?i)(token|password|passwd|secret|credential|auth[^a-z]|^auth$|api[_-]?key|private[_-]?key|access[_-]?key|bearer|session[_-]?id|^key$)`)

// FilteredEnv returns the process environment as a sorted "KEY=VALUE"
// text block. Variables whose keys look like secrets have their values
// replaced by "<redacted>" (the key is kept so the reader can see it
// was present without leaking the value).
func FilteredEnv() string {
	raw := os.Environ()
	out := make([]string, 0, len(raw))
	for _, kv := range raw {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		k, v := kv[:eq], kv[eq+1:]
		if secretKeyRe.MatchString(k) {
			out = append(out, k+"=<redacted>")
			continue
		}
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return strings.Join(out, "\n")
}

func readLogTail(path string, n int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n"), nil
}
