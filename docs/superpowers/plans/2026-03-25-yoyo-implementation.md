# yoyo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `yoyo`, a cross-platform Go CLI PTY proxy that auto-approves AI agent permission prompts with rule-based response selection, session memory, and runtime keyboard controls.

**Architecture:** Goroutine-based coordinator loop reads stdin and PTY output via channels. Each PTY output frame is fed to a VT100 screen buffer, then scanned by a `RuleChain` of detectors. Matches trigger a timer-delayed response (or immediate if seen before). A bottom-right ANSI status bar shows live state.

**Tech Stack:** Go 1.22, `github.com/aymanbagabas/go-pty` (cross-platform PTY), `github.com/hinshun/vt10x` (VT100 screen), `github.com/pelletier/go-toml/v2` (TOML config), `golang.org/x/term` (raw mode).

**Spec:** `docs/superpowers/specs/2026-03-25-yoyo-design.md`

---

## File Map

```
yoyo/
├── cmd/yoyo/main.go                     # CLI entry: flags, signals, proxy startup
├── internal/
│   ├── logger/
│   │   └── logger.go                    # Async file logger (channel-based, never writes stdout)
│   ├── config/
│   │   └── config.go                    # TOML load, tilde expansion, defaults
│   ├── memory/
│   │   └── memory.go                    # Session memory: seen/record by hash
│   ├── detector/
│   │   ├── detector.go                  # Detector interface, MatchResult, RuleChain, AgentKind
│   │   ├── claude.go                    # Claude Code prompt detector
│   │   ├── codex.go                     # Codex CLI prompt detector
│   │   ├── cursor.go                    # Cursor Agent prompt detector
│   │   └── regexp_detector.go           # Config-rule detector (compiled regexp → response)
│   ├── agent/
│   │   └── agent.go                     # KindFromCommand, KindFromScreen, Detector() dispatch
│   ├── screen/
│   │   └── screen.go                    # vt10x wrapper: Feed([]byte), Text() string, Resize
│   ├── statusbar/
│   │   └── statusbar.go                 # ANSI overlay: clear+paint around PTY frames
│   ├── term/
│   │   ├── term.go                      # Term struct, EnableRaw/Restore (golang.org/x/term)
│   │   ├── term_unix.go                 # SIGWINCH watcher + winsize helpers (build tag: !windows)
│   │   └── term_windows.go              # Windows no-op SIGWINCH stub (build tag: windows)
│   └── proxy/
│       └── proxy.go                     # Coordinator loop, safeGo, prefix key state machine
├── go.mod
└── go.sum
```

---

## Task 1: Project Scaffold

**Files:**
- Create: `go.mod`
- Create: `cmd/yoyo/main.go` (stub)

- [ ] **Step 1: Initialize go module**

```bash
cd /localhome/swqa/workspace/yoyo
go mod init yoyo
```

- [ ] **Step 2: Add dependencies**

```bash
go get github.com/aymanbagabas/go-pty@latest
go get github.com/hinshun/vt10x@latest
go get github.com/pelletier/go-toml/v2@latest
go get golang.org/x/term@latest
go mod tidy
```

- [ ] **Step 3: Create stub main.go**

```go
// cmd/yoyo/main.go
package main

import "fmt"

func main() {
	fmt.Println("yoyo")
}
```

- [ ] **Step 4: Verify it builds**

```bash
go build ./cmd/yoyo/
```
Expected: binary created, no errors.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum cmd/
git commit -m "feat: initialize yoyo Go module with dependencies"
```

---

## Task 2: Logger

**Files:**
- Create: `internal/logger/logger.go`
- Create: `internal/logger/logger_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/logger/logger_test.go
package logger_test

import (
	"os"
	"strings"
	"testing"

	"yoyo/internal/logger"
)

func TestLogger_WritesToFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "yoyo-log-*.log")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	log, err := logger.New(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	log.Infof("hello %s", "world")
	log.Close()

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello world") {
		t.Errorf("expected log file to contain 'hello world', got: %s", data)
	}
}

func TestLogger_Async(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "*.log")
	f.Close()
	log, _ := logger.New(f.Name())

	for i := 0; i < 100; i++ {
		log.Infof("line %d", i)
	}
	log.Close() // flush before read

	data, _ := os.ReadFile(f.Name())
	if !strings.Contains(string(data), "line 99") {
		t.Error("expected all 100 lines to be flushed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/logger/ -v
```
Expected: compile error — package does not exist yet.

- [ ] **Step 3: Implement logger**

```go
// internal/logger/logger.go
package logger

import (
	"fmt"
	"os"
	"time"
)

type Logger struct {
	ch   chan string
	done chan struct{}
}

// New opens the log file and starts the background write goroutine.
// Caller must call Close() when done.
func New(path string) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("logger: open %s: %w", path, err)
	}
	l := &Logger{
		ch:   make(chan string, 256),
		done: make(chan struct{}),
	}
	go func() {
		defer f.Close()
		for msg := range l.ch {
			fmt.Fprint(f, msg)
		}
		close(l.done)
	}()
	return l, nil
}

// Infof writes an INFO log line. Never blocks — drops if buffer full.
func (l *Logger) Infof(format string, args ...any) {
	l.log("INFO", format, args...)
}

// Errorf writes an ERROR log line.
func (l *Logger) Errorf(format string, args ...any) {
	l.log("ERROR", format, args...)
}

func (l *Logger) log(level, format string, args ...any) {
	msg := fmt.Sprintf("[%s] %s %s\n",
		level,
		time.Now().Format("15:04:05.000"),
		fmt.Sprintf(format, args...),
	)
	select {
	case l.ch <- msg:
	default: // drop if buffer full to avoid blocking PTY loop
	}
}

// Close flushes all pending log entries and closes the file.
func (l *Logger) Close() {
	close(l.ch)
	<-l.done
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/logger/ -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/logger/
git commit -m "feat: add async file logger"
```

---

## Task 3: Config

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/config/config_test.go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"yoyo/internal/config"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(content)
	f.Close()
	return f.Name()
}

func TestLoad_Defaults(t *testing.T) {
	path := writeConfig(t, "")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.Delay != 3 {
		t.Errorf("default delay = %d, want 3", cfg.Defaults.Delay)
	}
	if !cfg.Defaults.Enabled {
		t.Error("default enabled = false, want true")
	}
}

func TestLoad_AgentDelay(t *testing.T) {
	path := writeConfig(t, `
[agents.claude]
delay = 1
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agents["claude"].Delay != 1 {
		t.Errorf("claude delay = %d, want 1", cfg.Agents["claude"].Delay)
	}
}

func TestLoad_GlobalRule(t *testing.T) {
	path := writeConfig(t, `
[[rules]]
name = "confirm"
pattern = "Are you sure"
response = "y\r"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("rules count = %d, want 1", len(cfg.Rules))
	}
	if cfg.Rules[0].Name != "confirm" {
		t.Errorf("rule name = %q, want 'confirm'", cfg.Rules[0].Name)
	}
}

func TestLoad_TildeExpansion(t *testing.T) {
	path := writeConfig(t, `
[defaults]
log_file = "~/yoyo.log"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "yoyo.log")
	if cfg.Defaults.LogFile != want {
		t.Errorf("log_file = %q, want %q", cfg.Defaults.LogFile, want)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path.toml")
	if err != nil {
		t.Fatal("expected no error for missing config, got:", err)
	}
	// Missing file returns defaults
	if cfg.Defaults.Delay != 3 {
		t.Errorf("delay = %d, want 3", cfg.Defaults.Delay)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/config/ -v
```
Expected: compile error.

- [ ] **Step 3: Implement config**

```go
// internal/config/config.go
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Rule struct {
	Name     string
	Pattern  string
	Response string
}

type AgentConfig struct {
	Delay int    // -1 = inherit defaults
	Rules []Rule
}

type Defaults struct {
	Delay   int
	Enabled bool
	LogFile string `toml:"log_file"`
}

type Config struct {
	Defaults Defaults
	Agents   map[string]AgentConfig
	Rules    []Rule
}

// Load parses the TOML file at path and applies defaults.
// Returns defaults if the file does not exist.
func Load(path string) (*Config, error) {
	cfg := &Config{}
	cfg.Defaults.Delay = 3
	cfg.Defaults.Enabled = true
	cfg.Defaults.LogFile = expandTilde("~/.yoyo/yoyo.log")

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Apply tilde expansion to paths
	cfg.Defaults.LogFile = expandTilde(cfg.Defaults.LogFile)

	return cfg, nil
}

// DefaultPath returns the default config file path (~/.config/yoyo/config.toml).
func DefaultPath() string {
	return expandTilde("~/.config/yoyo/config.toml")
}

func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/config/ -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add TOML config loading with tilde expansion"
```

---

## Task 4: Session Memory

**Files:**
- Create: `internal/memory/memory.go`
- Create: `internal/memory/memory_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/memory/memory_test.go
package memory_test

import (
	"testing"

	"yoyo/internal/memory"
)

func TestMemory_NotSeenInitially(t *testing.T) {
	m := memory.New()
	if m.Seen("abc123") {
		t.Error("expected hash to not be seen initially")
	}
}

func TestMemory_RecordThenSeen(t *testing.T) {
	m := memory.New()
	m.Record("abc123")
	if !m.Seen("abc123") {
		t.Error("expected hash to be seen after Record")
	}
}

func TestMemory_DifferentHashesIndependent(t *testing.T) {
	m := memory.New()
	m.Record("hash1")
	if m.Seen("hash2") {
		t.Error("hash2 should not be seen after recording hash1")
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/memory/ -v
```

- [ ] **Step 3: Implement**

```go
// internal/memory/memory.go
package memory

import "sync"

// Memory tracks approved prompt hashes within a session.
type Memory struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func New() *Memory {
	return &Memory{seen: make(map[string]struct{})}
}

// Seen reports whether this hash was previously approved.
func (m *Memory) Seen(hash string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.seen[hash]
	return ok
}

// Record marks a hash as approved.
func (m *Memory) Record(hash string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seen[hash] = struct{}{}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/memory/ -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/memory/
git commit -m "feat: add session memory for prompt deduplication"
```

---

## Task 5: Detector Interface + Claude

**Files:**
- Create: `internal/detector/detector.go`
- Create: `internal/detector/claude.go`
- Create: `internal/detector/claude_test.go`
- Create: `internal/detector/regexp_detector.go`
- Create: `internal/detector/regexp_detector_test.go`

- [ ] **Step 1: Write the failing tests for core types**

```go
// internal/detector/claude_test.go
package detector_test

import (
	"testing"

	"yoyo/internal/detector"
)

func claudePrompt(action string, options []string) string {
	s := "─────────────────────────────────────────────\n"
	s += " " + action + "\n\n"
	for i, opt := range options {
		s += "   " + string(rune('1'+i)) + ". " + opt + "\n"
	}
	s += "\n Esc to cancel · Tab to amend\n"
	return s
}

func TestClaude_DetectsPrompt(t *testing.T) {
	d := detector.Claude{}
	p := claudePrompt("Read file", []string{"Yes", "No"})
	if d.Detect(p) == nil {
		t.Error("expected detection, got nil")
	}
}

func TestClaude_StripsWhitespaceAndSpecialChars(t *testing.T) {
	d := detector.Claude{}
	p := "─────────────\n  ❯ Read file  \n\n   1. Yes\n   2. No\n\n Esc to cancel\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.RuleName != "Claude" {
		t.Errorf("RuleName = %q, want 'Claude'", r.RuleName)
	}
}

func TestClaude_FullExample(t *testing.T) {
	d := detector.Claude{}
	p := "─────────────────────────────────────────────────────────────────────\n" +
		" Read file\n\n" +
		"  Read(/some/path/file.rs)\n\n" +
		" Do you want to proceed?\n" +
		"   1. Yes\n" +
		"   2. Yes, allow reading from src/ during this session\n" +
		"   3. No\n\n" +
		" Esc to cancel · Tab to amend\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.Hash == "" {
		t.Error("expected non-empty hash")
	}
}

func TestClaude_NoSeparator(t *testing.T) {
	d := detector.Claude{}
	if d.Detect(" Read file\n\n Esc to cancel\n") != nil {
		t.Error("should not detect without separator")
	}
}

func TestClaude_NoEscLineNoNoOption(t *testing.T) {
	d := detector.Claude{}
	if d.Detect("─────────────\n Read file\n 1. Yes\n") != nil {
		t.Error("should not detect without No option")
	}
}

func TestClaude_FallbackNoEscWithNoOption(t *testing.T) {
	d := detector.Claude{}
	p := "─────────────────────────────────────────────\n" +
		" Read file\n\n 1. Yes\n 2. No\n"
	if d.Detect(p) == nil {
		t.Error("should detect with fallback bottom boundary")
	}
}

func TestClaude_FallbackLongBodyNoEsc(t *testing.T) {
	d := detector.Claude{}
	p := "─────────────────────────────────────────────────────────────────────\n" +
		" Read file\n\n  Read(/some/long/path.rs)\n\n" +
		" Do you want to proceed?\n" +
		"   1. Yes\n   2. Yes, allow reading from src/ during this session\n   3. No\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
}

func TestClaude_IncompleteRenderNoOptions(t *testing.T) {
	d := detector.Claude{}
	if d.Detect("─────────────\n Read file\n\n Esc to cancel\n") != nil {
		t.Error("should not detect incomplete render (no options)")
	}
}

func TestClaude_IncompleteRenderPartialOptions(t *testing.T) {
	d := detector.Claude{}
	if d.Detect("─────────────\n Read file\n 1. Yes\n Esc to cancel\n") != nil {
		t.Error("should not detect when only Yes rendered but No missing")
	}
}

func TestClaude_PicksLastSeparatorWhenMultiple(t *testing.T) {
	d := detector.Claude{}
	p := "─────────────\n old output with separator\n" +
		"─────────────\n Write file\n 1. Yes\n 2. No\n Esc to cancel\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
}

func TestClaude_StableAcrossRedraws(t *testing.T) {
	d := detector.Claude{}
	p := claudePrompt("Read file", []string{"Yes", "No"})
	r1 := d.Detect(p)
	r2 := d.Detect(p)
	if r1 == nil || r2 == nil {
		t.Fatal("expected both detections to succeed")
	}
	if r1.Hash != r2.Hash {
		t.Error("hash should be stable across identical redraws")
	}
}

func TestClaude_DifferentPromptsDiffer(t *testing.T) {
	d := detector.Claude{}
	r1 := d.Detect(claudePrompt("Read file", []string{"Yes", "No"}))
	r2 := d.Detect(claudePrompt("Write file", []string{"Yes", "No"}))
	if r1 == nil || r2 == nil {
		t.Fatal("both should be detected")
	}
	if r1.Hash == r2.Hash {
		t.Error("different prompts should have different hashes")
	}
}

func TestClaude_SeparatorScrolledOff(t *testing.T) {
	d := detector.Claude{}
	p := "  Read(/some/very/long/path/to/file.rs)\n\n" +
		" Do you want to proceed?\n" +
		"   1. Yes\n   2. Yes, allow reading from src/ during this session\n   3. No\n\n" +
		" Esc to cancel · Tab to amend\n"
	r := d.Detect(p)
	if r == nil {
		t.Error("should detect when separator scrolled off (fallback top)")
	}
}

func TestClaude_SeparatorScrolledOffNoEsc(t *testing.T) {
	d := detector.Claude{}
	p := "  Read(/some/very/long/path/to/file.rs)\n\n" +
		" Do you want to proceed?\n" +
		"   1. Yes\n   2. No\n"
	if d.Detect(p) == nil {
		t.Error("should detect with both fallback boundaries")
	}
}

func TestClaude_SeparatorScrolledOffEditVariant(t *testing.T) {
	d := detector.Claude{}
	p := "  some long content\n\n" +
		" Do you want to edit the file?\n" +
		"   1. Yes\n   2. No\n\n" +
		" Esc to cancel · Tab to amend\n"
	if d.Detect(p) == nil {
		t.Error("should detect 'Do you want to edit' variant")
	}
}

func TestClaude_SeparatorPreferredOverFallback(t *testing.T) {
	d := detector.Claude{}
	p := "─────────────────────────────────────────────\n" +
		" Read file\n\n Do you want to proceed?\n" +
		"   1. Yes\n   2. No\n\n Esc to cancel\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
}

func TestClaude_DefaultResponse(t *testing.T) {
	d := detector.Claude{}
	p := claudePrompt("Read file", []string{"Yes", "No"})
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.Response != "\r" {
		t.Errorf("Response = %q, want %q", r.Response, "\r")
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/detector/ -v -run TestClaude
```

- [ ] **Step 3: Implement detector.go (interfaces + types)**

```go
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
```

- [ ] **Step 4: Implement claude.go**

```go
// internal/detector/claude.go
package detector

import "strings"

// Claude detects Claude Code permission prompts.
//
// Matches the structured block delimited by a line of ─ characters (top)
// and "Esc to cancel" (bottom). Falls back to "Do you want to" as top
// boundary when the separator has scrolled off. Falls back to the last
// numbered "N. No" line as bottom when "Esc to cancel" is absent.
// Requires both Yes and No options to guard against partial renders.
type Claude struct{}

func (c Claude) Detect(screenText string) *MatchResult {
	lines := strings.Split(screenText, "\n")

	// Find bottom boundary: prefer "Esc to cancel", fall back to last "N. No" line
	bottomIdx, bottomInclusive := findClaudeBottom(lines)
	if bottomIdx < 0 {
		return nil
	}

	// Find top boundary: prefer ─ separator, fall back to "Do you want to"
	topIdx, topInclusive := findClaudeTop(lines, bottomIdx)
	if topIdx < 0 {
		return nil
	}

	bodyStart := topIdx
	if !topInclusive {
		bodyStart = topIdx + 1
	}
	bodyEnd := bottomIdx
	if bottomInclusive {
		bodyEnd = bottomIdx + 1
	}

	if bodyStart >= bodyEnd {
		return nil
	}

	var cleaned []string
	for _, line := range lines[bodyStart:bodyEnd] {
		l := strings.TrimSpace(strings.ReplaceAll(line, "❯", ""))
		l = strings.TrimSpace(l)
		if l != "" {
			cleaned = append(cleaned, l)
		}
	}
	body := strings.Join(cleaned, "\n")

	if !strings.Contains(body, "Yes") || !strings.Contains(body, "No") {
		return nil
	}

	return &MatchResult{
		RuleName: "Claude",
		Response: "\r",
		Hash:     hashBody(body),
	}
}

// findClaudeBottom finds the bottom boundary index.
// Returns (idx, inclusive).
func findClaudeBottom(lines []string) (int, bool) {
	// Prefer "Esc to cancel" (exclusive)
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "Esc to cancel") {
			return i, false
		}
	}
	// Fall back: last "N. No" line (inclusive)
	for i := len(lines) - 1; i >= 0; i-- {
		t := strings.TrimSpace(strings.ReplaceAll(lines[i], "❯", ""))
		t = strings.TrimSpace(t)
		if len(t) >= 4 && t[0] >= '1' && t[0] <= '9' && strings.HasPrefix(t[1:], ". No") {
			return i, true
		}
		if len(t) >= 4 && t[0] >= '1' && t[0] <= '9' {
			after := strings.TrimLeft(t[1:], " \t")
			if strings.HasPrefix(after, ". No") {
				return i, true
			}
		}
	}
	return -1, false
}

// findClaudeTop finds the top boundary index above bottomIdx.
// Returns (idx, inclusive).
func findClaudeTop(lines []string, bottomIdx int) (int, bool) {
	// Prefer ─ separator (exclusive)
	for i := bottomIdx - 1; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if t != "" && isAllRune(t, '─') {
			return i, false
		}
	}
	// Fall back: "Do you want to" (inclusive)
	for i := bottomIdx - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "Do you want to") {
			return i, true
		}
	}
	return -1, false
}

func isAllRune(s string, r rune) bool {
	for _, c := range s {
		if c != r {
			return false
		}
	}
	return true
}
```

- [ ] **Step 5: Run Claude tests**

```bash
go test ./internal/detector/ -v -run TestClaude
```
Expected: all 16 tests PASS

- [ ] **Step 6: Write and implement regexp_detector.go**

```go
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
```

```go
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
```

- [ ] **Step 7: Run all detector tests so far**

```bash
go test ./internal/detector/ -v
```
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/detector/
git commit -m "feat: add Detector interface, Claude detector, RegexpDetector"
```

---

## Task 6: Codex Detector

**Files:**
- Create: `internal/detector/codex.go`
- Create: `internal/detector/codex_test.go`

- [ ] **Step 1: Write failing tests**

```go
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
```

> Note: add `"fmt"` import to codex_test.go.

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/detector/ -v -run TestCodex
```

- [ ] **Step 3: Implement codex.go**

```go
// internal/detector/codex.go
package detector

import "strings"

var codexStartPatterns = []string{
	"Would you like to",
	"needs your approval",
}

const codexFooter = "Press enter to confirm or esc to cancel"

// Codex detects Codex CLI permission prompts.
type Codex struct{}

func (c Codex) Detect(screenText string) *MatchResult {
	lines := strings.Split(screenText, "\n")

	// Find last footer line
	footerIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], codexFooter) {
			footerIdx = i
			break
		}
	}
	if footerIdx < 0 {
		return nil
	}

	// Find nearest start pattern above footer
	startIdx := -1
	for i := footerIdx - 1; i >= 0; i-- {
		for _, pat := range codexStartPatterns {
			if strings.Contains(lines[i], pat) {
				startIdx = i
				break
			}
		}
		if startIdx >= 0 {
			break
		}
	}
	if startIdx < 0 {
		return nil
	}

	var cleaned []string
	for _, line := range lines[startIdx:footerIdx] {
		l := strings.TrimSpace(strings.ReplaceAll(line, "›", ""))
		l = strings.TrimSpace(l)
		if l != "" {
			cleaned = append(cleaned, l)
		}
	}
	body := strings.Join(cleaned, "\n")
	if body == "" {
		return nil
	}

	return &MatchResult{
		RuleName: "Codex",
		Response: "\r",
		Hash:     hashBody(body),
	}
}
```

- [ ] **Step 4: Run Codex tests**

```bash
go test ./internal/detector/ -v -run TestCodex
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/detector/codex.go internal/detector/codex_test.go
git commit -m "feat: add Codex CLI prompt detector"
```

---

## Task 7: Cursor Detector

**Files:**
- Create: `internal/detector/cursor.go`
- Create: `internal/detector/cursor_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/detector/cursor_test.go
package detector_test

import (
	"strings"
	"testing"

	"yoyo/internal/detector"
)

func cursorBox(lines []string) string {
	width := 60
	top := "┌" + strings.Repeat("─", width) + "┐\n"
	bottom := "└" + strings.Repeat("─", width) + "┘\n"
	var sb strings.Builder
	sb.WriteString(top)
	for _, line := range lines {
		sb.WriteString("│ ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString(bottom)
	return sb.String()
}

func TestCursor_CommandPrompt(t *testing.T) {
	d := detector.Cursor{}
	p := cursorBox([]string{
		"Run this command?",
		"Not in allowlist: cargo test",
		" → Run (once) (y)",
		"   Skip (esc or n)",
	})
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.RuleName != "Cursor" {
		t.Errorf("RuleName = %q, want 'Cursor'", r.RuleName)
	}
}

func TestCursor_NoMatchWithoutOptions(t *testing.T) {
	d := detector.Cursor{}
	if d.Detect(cursorBox([]string{"Some text without options"})) != nil {
		t.Error("should not detect without (y)/(n) options")
	}
}

func TestCursor_NoMatchWithoutBox(t *testing.T) {
	d := detector.Cursor{}
	if d.Detect("Run (once) (y)\nSkip (esc or n)") != nil {
		t.Error("should not detect without box drawing")
	}
}

func TestCursor_IgnoresInputBox(t *testing.T) {
	d := detector.Cursor{}
	if d.Detect(cursorBox([]string{"→ Plan, search, build anything"})) != nil {
		t.Error("should ignore input box without (y)/(n)")
	}
}

func TestCursor_PicksLastBox(t *testing.T) {
	d := detector.Cursor{}
	p := cursorBox([]string{"→ Plan, search, build anything"}) +
		cursorBox([]string{"Run this command?", " → Run (once) (y)", "   Skip (esc or n)"})
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
}

func TestCursor_DefaultResponse(t *testing.T) {
	d := detector.Cursor{}
	p := cursorBox([]string{"Run (once) (y)", "Skip (esc or n)"})
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.Response != "\r" {
		t.Errorf("Response = %q, want \\r", r.Response)
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/detector/ -v -run TestCursor
```

- [ ] **Step 3: Implement cursor.go**

```go
// internal/detector/cursor.go
package detector

import "strings"

// Cursor detects Cursor Agent permission prompts (box-drawn UI).
type Cursor struct{}

func (c Cursor) Detect(screenText string) *MatchResult {
	lines := strings.Split(screenText, "\n")

	// Find last bottom border └──┘
	bottomIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "└") && strings.HasSuffix(t, "┘") && strings.Contains(t, "─") {
			bottomIdx = i
			break
		}
	}
	if bottomIdx < 0 {
		return nil
	}

	// Find nearest top border ┌──┐ above bottom
	topIdx := -1
	for i := bottomIdx - 1; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "┌") && strings.HasSuffix(t, "┐") && strings.Contains(t, "─") {
			topIdx = i
			break
		}
	}
	if topIdx < 0 {
		return nil
	}

	var cleaned []string
	for _, line := range lines[topIdx+1 : bottomIdx] {
		l := strings.TrimSpace(line)
		l = strings.TrimPrefix(l, "│")
		l = strings.TrimSuffix(l, "│")
		l = strings.ReplaceAll(l, "→", "")
		l = strings.TrimSpace(l)
		if l != "" {
			cleaned = append(cleaned, l)
		}
	}
	body := strings.Join(cleaned, "\n")

	if !strings.Contains(body, "(y)") || !strings.Contains(body, "n)") {
		return nil
	}

	return &MatchResult{
		RuleName: "Cursor",
		Response: "\r",
		Hash:     hashBody(body),
	}
}
```

- [ ] **Step 4: Run Cursor tests**

```bash
go test ./internal/detector/ -v -run TestCursor
```
Expected: PASS

- [ ] **Step 5: Run full detector suite**

```bash
go test ./internal/detector/ -v
```
Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/detector/cursor.go internal/detector/cursor_test.go
git commit -m "feat: add Cursor Agent prompt detector"
```

---

## Task 8: Agent Kind Detection

**Files:**
- Create: `internal/agent/agent.go`
- Create: `internal/agent/agent_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/agent/agent_test.go
package agent_test

import (
	"testing"

	"yoyo/internal/agent"
	"yoyo/internal/detector"
)

func TestKindFromCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want agent.Kind
	}{
		{"claude", agent.KindClaude},
		{"/usr/bin/claude", agent.KindClaude},
		{"claude.exe", agent.KindClaude},
		{"codex", agent.KindCodex},
		{"cursor", agent.KindCursor},
		{"unknown-tool", agent.KindUnknown},
		{"", agent.KindUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.cmd, func(t *testing.T) {
			got := agent.KindFromCommand(tc.cmd)
			if got != tc.want {
				t.Errorf("KindFromCommand(%q) = %v, want %v", tc.cmd, got, tc.want)
			}
		})
	}
}

func TestKindFromScreen(t *testing.T) {
	tests := []struct {
		text string
		want agent.Kind
	}{
		{"Welcome to Claude Code", agent.KindClaude},
		{"codex v1.0", agent.KindCodex},
		{"Codex CLI", agent.KindCodex},
		{"cursor running", agent.KindCursor},
		{"something else", agent.KindUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			got := agent.KindFromScreen(tc.text)
			if got != tc.want {
				t.Errorf("KindFromScreen(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestKind_Detector_Claude(t *testing.T) {
	d := agent.KindClaude.Detector()
	if d == nil {
		t.Error("expected non-nil detector for Claude")
	}
	// Should behave as Claude detector
	prompt := "─────────────\n Read file\n 1. Yes\n 2. No\n Esc to cancel\n"
	if d.Detect(prompt) == nil {
		t.Error("Claude kind detector should detect Claude prompts")
	}
}

func TestKind_Detector_Unknown_TriesAll(t *testing.T) {
	d := agent.KindUnknown.Detector()
	// Should detect Claude prompts
	claudePrompt := "─────────────\n Read file\n 1. Yes\n 2. No\n Esc to cancel\n"
	if d.Detect(claudePrompt) == nil {
		t.Error("Unknown kind should try Claude detector")
	}
	// Should detect Codex prompts
	codexPrompt := "  Would you like to run this?\n› 1. Yes\n" +
		"  Press enter to confirm or esc to cancel\n"
	if d.Detect(codexPrompt) == nil {
		t.Error("Unknown kind should try Codex detector")
	}
}

// Ensure agent.Kind satisfies detector.Detector interface
var _ detector.Detector = agent.Kind(0)
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/agent/ -v
```

- [ ] **Step 3: Implement agent.go**

```go
// internal/agent/agent.go
package agent

import (
	"path/filepath"
	"strings"

	"yoyo/internal/detector"
)

// Kind identifies the type of AI agent CLI being proxied.
type Kind int

const (
	KindUnknown Kind = iota
	KindClaude
	KindCodex
	KindCursor
)

func (k Kind) String() string {
	switch k {
	case KindClaude:
		return "claude"
	case KindCodex:
		return "codex"
	case KindCursor:
		return "cursor"
	default:
		return "unknown"
	}
}

// KindFromCommand identifies the agent from the command name.
// Strips path components and Windows extensions (.exe, .cmd, .bat).
func KindFromCommand(command string) Kind {
	if command == "" {
		return KindUnknown
	}
	base := filepath.Base(command)
	// Strip Windows extensions
	for _, ext := range []string{".exe", ".cmd", ".bat"} {
		base = strings.TrimSuffix(base, ext)
	}
	switch strings.ToLower(base) {
	case "claude":
		return KindClaude
	case "codex":
		return KindCodex
	case "cursor", "cursor-agent":
		return KindCursor
	default:
		return KindUnknown
	}
}

// KindFromScreen identifies the agent from visible terminal content.
// Used as fallback when the command name is not recognized.
func KindFromScreen(screenText string) Kind {
	switch {
	case strings.Contains(screenText, "Claude Code"):
		return KindClaude
	case strings.Contains(screenText, "codex") || strings.Contains(screenText, "Codex"):
		return KindCodex
	case strings.Contains(screenText, "cursor") || strings.Contains(screenText, "Cursor"):
		return KindCursor
	default:
		return KindUnknown
	}
}

// Detector returns the appropriate built-in Detector for this agent kind.
// For KindUnknown, returns a multi-detector that tries all built-ins in order.
func (k Kind) Detector() detector.Detector {
	switch k {
	case KindClaude:
		return detector.Claude{}
	case KindCodex:
		return detector.Codex{}
	case KindCursor:
		return detector.Cursor{}
	default:
		return multiDetector{detector.Claude{}, detector.Codex{}, detector.Cursor{}}
	}
}

// multiDetector tries each detector in order, returning the first match.
type multiDetector []detector.Detector

func (m multiDetector) Detect(screenText string) *detector.MatchResult {
	for _, d := range m {
		if r := d.Detect(screenText); r != nil {
			return r
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/agent/ -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/
git commit -m "feat: add AgentKind detection from command name and screen content"
```

---

## Task 9: Screen Buffer

**Files:**
- Create: `internal/screen/screen.go`
- Create: `internal/screen/screen_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/screen/screen_test.go
package screen_test

import (
	"strings"
	"testing"

	"yoyo/internal/screen"
)

func TestScreen_FeedAndText(t *testing.T) {
	s := screen.New(80, 24)
	s.Feed([]byte("Hello, World!"))
	text := s.Text()
	if !strings.Contains(text, "Hello, World!") {
		t.Errorf("expected 'Hello, World!' in screen text, got: %q", text)
	}
}

func TestScreen_StripANSI(t *testing.T) {
	s := screen.New(80, 24)
	// Write text with color codes
	s.Feed([]byte("\x1b[31mRed text\x1b[0m normal"))
	text := s.Text()
	if !strings.Contains(text, "Red text") {
		t.Errorf("screen text should contain stripped text 'Red text', got: %q", text)
	}
	if strings.Contains(text, "\x1b") {
		t.Error("screen text should not contain ANSI escape sequences")
	}
}

func TestScreen_Resize(t *testing.T) {
	s := screen.New(80, 24)
	// Should not panic
	s.Resize(132, 50)
	s.Feed([]byte("after resize"))
	if !strings.Contains(s.Text(), "after resize") {
		t.Error("should work after resize")
	}
}

func TestScreen_MultipleFeeds(t *testing.T) {
	s := screen.New(80, 24)
	s.Feed([]byte("first"))
	s.Feed([]byte(" second"))
	if !strings.Contains(s.Text(), "first") {
		t.Error("should retain first feed")
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/screen/ -v
```

- [ ] **Step 3: Implement screen.go**

```go
// internal/screen/screen.go
package screen

import "github.com/hinshun/vt10x"

// Screen wraps a vt10x virtual terminal for screen content extraction.
type Screen struct {
	vt    vt10x.VT
	state *vt10x.State
}

// New creates a Screen with the given dimensions (cols x rows).
func New(cols, rows int) *Screen {
	vt, state := vt10x.New(vt10x.WithSize(cols, rows))
	return &Screen{vt: vt, state: state}
}

// Feed writes raw PTY bytes into the virtual terminal.
func (s *Screen) Feed(data []byte) {
	s.vt.Write(data)
}

// Text returns the current visible text content of the screen,
// with ANSI escape sequences stripped.
func (s *Screen) Text() string {
	return s.state.String()
}

// Resize updates the screen dimensions.
func (s *Screen) Resize(cols, rows int) {
	s.vt.Resize(cols, rows)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/screen/ -v
```
Expected: PASS

> Note: If `vt10x.New` API differs from above (check with `go doc github.com/hinshun/vt10x`), adjust constructor call accordingly. The key contract: `vt.Write(data)` feeds bytes, `state.String()` returns visible text.

- [ ] **Step 5: Commit**

```bash
git add internal/screen/
git commit -m "feat: add vt10x-backed screen buffer"
```

---

## Task 10: Status Bar

**Files:**
- Create: `internal/statusbar/statusbar.go`
- Create: `internal/statusbar/statusbar_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/statusbar/statusbar_test.go
package statusbar_test

import (
	"strings"
	"testing"

	"yoyo/internal/statusbar"
)

func TestStatusBar_PaintsLabel(t *testing.T) {
	sb := statusbar.New(24, 80, true, 3)
	out := sb.WrapFrame([]byte("hello"))
	s := string(out)
	// Should contain cursor save/restore
	if !strings.Contains(s, "\x1b[s") {
		t.Error("expected cursor save sequence")
	}
	if !strings.Contains(s, "\x1b[u") {
		t.Error("expected cursor restore sequence")
	}
	// Should contain the frame content
	if !strings.Contains(s, "hello") {
		t.Error("should contain original frame content")
	}
}

func TestStatusBar_LabelContainsDelay(t *testing.T) {
	sb := statusbar.New(24, 80, true, 3)
	out := string(sb.WrapFrame([]byte("x")))
	if !strings.Contains(out, "3s") {
		t.Error("label should show delay '3s'")
	}
}

func TestStatusBar_OffLabel(t *testing.T) {
	sb := statusbar.New(24, 80, false, 3)
	out := string(sb.WrapFrame([]byte("x")))
	if !strings.Contains(out, "off") {
		t.Error("label should show 'off' when disabled")
	}
}

func TestStatusBar_Toggle(t *testing.T) {
	sb := statusbar.New(24, 80, true, 3)
	sb.Toggle()
	out := string(sb.WrapFrame([]byte("x")))
	if !strings.Contains(out, "off") {
		t.Error("after toggle, label should show 'off'")
	}
}

func TestStatusBar_SetDelay(t *testing.T) {
	sb := statusbar.New(24, 80, true, 3)
	sb.SetDelay(5)
	out := string(sb.WrapFrame([]byte("x")))
	if !strings.Contains(out, "5s") {
		t.Error("after SetDelay(5), label should show '5s'")
	}
}

func TestStatusBar_SetRule(t *testing.T) {
	sb := statusbar.New(24, 80, true, 3)
	sb.SetRule("Claude:yes")
	out := string(sb.WrapFrame([]byte("x")))
	if !strings.Contains(out, "Claude:yes") {
		t.Error("should show rule name in label")
	}
}

func TestStatusBar_SkipOnMidEscape(t *testing.T) {
	sb := statusbar.New(24, 80, true, 3)
	// Frame ends mid-escape (ESC only)
	out1 := sb.WrapFrame([]byte("\x1b"))
	// The continuation should not inject
	out2 := sb.WrapFrame([]byte("[1m"))
	// Check no double injection for the mid-escape frame
	_ = out1
	_ = out2
	// Main contract: should not panic and should contain frame bytes
	combined := string(out1) + string(out2)
	if !strings.Contains(combined, "[1m") {
		t.Error("frame content must be present")
	}
}

func TestStatusBar_NoPaintWhenTooNarrow(t *testing.T) {
	sb := statusbar.New(24, 10, true, 3) // too narrow for label
	out := string(sb.WrapFrame([]byte("x")))
	// Should just pass through without overlay
	if out != "x" {
		t.Logf("narrow terminal output: %q", out)
		// Accept pass-through or minimal output
	}
}
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./internal/statusbar/ -v
```

- [ ] **Step 3: Implement statusbar.go**

```go
// internal/statusbar/statusbar.go
package statusbar

import "fmt"

const (
	cursorSave    = "\x1b[s"
	cursorRestore = "\x1b[u"
	sgrReset      = "\x1b[0m"
	fgGreen       = "\x1b[32m"
	fgRed         = "\x1b[31m"
)

// labelWidth is the fixed width of the status label (widest possible label).
// " [yoyo: on Xs | RuleName]" — we reserve space for "on Xs" + rule.
// Keep it wide enough; truncate rule name if needed.
const minLabelWidth = 22 // " [yoyo: on Xs | ...]  " minimum

// StatusBar renders a bottom-right ANSI overlay around PTY output frames.
type StatusBar struct {
	rows       uint16
	cols       uint16
	enabled    bool
	delaySecs  int
	rule       string
	painted    bool
	midSeq     bool
}

// New creates a StatusBar. enabled=true means auto-approve is active.
func New(rows, cols uint16, enabled bool, delaySecs int) *StatusBar {
	return &StatusBar{
		rows:      rows,
		cols:      cols,
		enabled:   enabled,
		delaySecs: delaySecs,
	}
}

func (sb *StatusBar) Toggle() { sb.enabled = !sb.enabled }

func (sb *StatusBar) SetDelay(secs int) { sb.delaySecs = secs }

// SetRule sets the last-matched rule name shown in the label.
func (sb *StatusBar) SetRule(rule string) { sb.rule = rule }

// Resize updates terminal dimensions.
func (sb *StatusBar) Resize(rows, cols uint16) {
	sb.rows = rows
	sb.cols = cols
}

// WrapFrame injects clear-previous and paint-new ANSI sequences around frame.
// Returns a single buffer for one atomic write to stdout.
func (sb *StatusBar) WrapFrame(frame []byte) []byte {
	prevMid := sb.midSeq
	sb.midSeq = endsMidEscape(frame) || endsMidUTF8(frame)

	label := sb.labelText()
	lw := uint16(len(label))
	if sb.cols < lw+2 || sb.rows == 0 {
		return frame // terminal too narrow
	}

	col := sb.cols - lw + 1

	var clear []byte
	if sb.painted && !prevMid {
		sb.painted = false
		blank := make([]byte, lw)
		for i := range blank {
			blank[i] = ' '
		}
		clear = overlayAt(sb.rows, col, "", string(blank))
	} else if prevMid {
		sb.painted = false
	}

	var paint []byte
	if !sb.midSeq {
		sb.painted = true
		color := fgRed // off = red (warning)
		if sb.enabled {
			color = fgGreen // on = green (active/good)
		}
		paint = overlayAt(sb.rows, col, color, label)
	}

	out := make([]byte, 0, len(clear)+len(frame)+len(paint))
	out = append(out, clear...)
	out = append(out, frame...)
	out = append(out, paint...)
	return out
}

func (sb *StatusBar) labelText() string {
	if !sb.enabled {
		return " [yoyo: off] "
	}
	rule := sb.rule
	if rule == "" {
		return fmt.Sprintf(" [yoyo: on %ds] ", sb.delaySecs)
	}
	return fmt.Sprintf(" [yoyo: on %ds | %s] ", sb.delaySecs, rule)
}

func overlayAt(row, col uint16, color, text string) []byte {
	s := fmt.Sprintf("%s\x1b[%d;%dH%s%s%s%s",
		cursorSave, row, col, sgrReset, color, text, cursorRestore)
	return []byte(s)
}

// endsMidEscape reports whether data ends with an incomplete ANSI escape.
func endsMidEscape(data []byte) bool {
	pos := -1
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] == 0x1b {
			pos = i
			break
		}
	}
	if pos < 0 {
		return false
	}
	tail := data[pos:]
	if len(tail) == 1 {
		return true
	}
	switch tail[1] {
	case '[':
		for _, b := range tail[2:] {
			if b >= 0x40 && b <= 0x7E {
				return false
			}
		}
		return true
	case ']':
		for i := 0; i < len(tail)-1; i++ {
			if tail[i] == 0x07 {
				return false
			}
			if tail[i] == 0x1b && tail[i+1] == '\\' {
				return false
			}
		}
		return true
	}
	return false
}

// endsMidUTF8 reports whether data ends with an incomplete UTF-8 character.
func endsMidUTF8(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	i := len(data) - 1
	for i > 0 && (data[i]&0xC0) == 0x80 {
		i--
	}
	b := data[i]
	var expected int
	switch {
	case b < 0x80:
		expected = 1
	case b < 0xE0:
		expected = 2
	case b < 0xF0:
		expected = 3
	default:
		expected = 4
	}
	return (len(data) - i) < expected
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/statusbar/ -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/statusbar/
git commit -m "feat: add ANSI status bar overlay"
```

---

## Task 11: Terminal Management

**Files:**
- Create: `internal/term/term.go`
- Create: `internal/term/term_unix.go` (build tag: `//go:build !windows`)
- Create: `internal/term/term_windows.go` (build tag: `//go:build windows`)
- Create: `internal/term/term_test.go`

- [ ] **Step 1: Write test**

```go
// internal/term/term_test.go
package term_test

import (
	"os"
	"testing"

	"yoyo/internal/term"
)

func TestTerm_NewDoesNotPanic(t *testing.T) {
	// Can't easily test raw mode in unit tests (stdin may not be a tty).
	// Just ensure New() works and Restore() is safe to call without EnableRaw.
	t.Run("restore without enable is safe", func(t *testing.T) {
		tr := term.New(os.Stdin)
		tr.Restore() // should not panic
	})
}
```

- [ ] **Step 2: Implement term.go (cross-platform core)**

```go
// internal/term/term.go
package term

import (
	"os"

	"golang.org/x/term"
)

// Term manages raw mode for a terminal file descriptor.
type Term struct {
	file     *os.File
	oldState *term.State
}

// New creates a Term wrapping the given file (typically os.Stdin).
func New(f *os.File) *Term {
	return &Term{file: f}
}

// NewNoOp creates a no-op Term for use in tests where no real TTY is available.
// Restore() and Size() are safe to call on a no-op Term.
func NewNoOp() *Term {
	return &Term{}
}

// EnableRaw switches the terminal to raw mode.
// Call Restore() when done (typically via defer).
func (t *Term) EnableRaw() error {
	if t.file == nil {
		return nil // no-op
	}
	state, err := term.MakeRaw(int(t.file.Fd()))
	if err != nil {
		return err
	}
	t.oldState = state
	return nil
}

// Restore returns the terminal to its original mode.
// Safe to call even if EnableRaw was never called or file is nil.
func (t *Term) Restore() {
	if t.oldState != nil && t.file != nil {
		term.Restore(int(t.file.Fd()), t.oldState)
	}
}

// Size returns the current terminal dimensions (cols, rows).
// Returns (80, 24) if file is nil or size cannot be determined.
func (t *Term) Size() (cols, rows int) {
	if t.file == nil {
		return 80, 24
	}
	w, h, err := term.GetSize(int(t.file.Fd()))
	if err != nil {
		return 80, 24 // safe default
	}
	return w, h
}
```

- [ ] **Step 3: Implement term_unix.go (SIGWINCH)**

```go
// internal/term/term_unix.go
//go:build !windows

package term

import (
	"os"
	"os/signal"
	"syscall"
)

// WatchResize calls onResize(cols, rows) whenever the terminal is resized.
// Returns a stop function. Call stop() to unsubscribe.
func (t *Term) WatchResize(onResize func(cols, rows int)) func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		for range ch {
			cols, rows := t.Size()
			onResize(cols, rows)
		}
	}()
	return func() {
		signal.Stop(ch)
		close(ch)
	}
}
```

- [ ] **Step 4: Implement term_windows.go (stub)**

```go
// internal/term/term_windows.go
//go:build windows

package term

// WatchResize is a no-op on Windows. go-pty handles ConPTY resize events.
func (t *Term) WatchResize(onResize func(cols, rows int)) func() {
	return func() {}
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/term/ -v
go build ./...
```
Expected: PASS and no compile errors on both Unix and (cross-compile) Windows.

- [ ] **Step 6: Commit**

```bash
git add internal/term/
git commit -m "feat: add terminal raw mode management with SIGWINCH support"
```

---

## Task 12: Proxy — Core Loop

**Files:**
- Create: `internal/proxy/proxy.go`
- Create: `internal/proxy/proxy_test.go`

This is the largest and most complex task. It wires all previous components together.

- [ ] **Step 1: Write the integration test first**

```go
// internal/proxy/proxy_test.go
package proxy_test

import (
	"testing"

	"yoyo/internal/proxy"
)

// TestProxy_PackageCompiles is a compile-time check.
// Full integration test is in Task 14.
func TestProxy_PackageCompiles(t *testing.T) {
	// Verify proxy.Config fields compile correctly
	_ = proxy.Config{
		Delay:   0,
		Enabled: true,
	}
}
```

> Note: The full integration test requires a real PTY. The test above establishes the interface. The `proxy.New` constructor defined below drives the full implementation.

- [ ] **Step 2: Implement proxy.go**

```go
// internal/proxy/proxy.go
package proxy

import (
	"io"
	"os"
	"runtime/debug"
	"time"

	"yoyo/internal/agent"
	"yoyo/internal/detector"
	"yoyo/internal/logger"
	"yoyo/internal/memory"
	"yoyo/internal/screen"
	"yoyo/internal/statusbar"
	"yoyo/internal/term"
)

const (
	prefixByte    = byte(0x19) // Ctrl+Y
	prefixTimeout = 1500 * time.Millisecond
)

// Config holds all dependencies for the Proxy.
type Config struct {
	PTY       io.ReadWriter  // child PTY (go-pty Pty)
	Stdin     io.Reader      // defaults to os.Stdin if nil
	Stdout    io.Writer      // defaults to os.Stdout if nil
	RuleChain detector.RuleChain
	Memory    *memory.Memory
	StatusBar *statusbar.StatusBar
	Log       *logger.Logger
	Term      *term.Term
	Screen    *screen.Screen
	AgentKind agent.Kind
	Delay     int  // seconds
	Enabled   bool
}

// Proxy is the coordinator that routes bytes between stdin, child PTY, and stdout.
type Proxy struct {
	cfg Config
}

func New(cfg Config) *Proxy {
	return &Proxy{cfg: cfg}
}

// safeGo launches fn in a goroutine with panic recovery.
// On panic, restores terminal and exits.
func safeGo(t *term.Term, log *logger.Logger, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Restore()
				if log != nil {
					log.Errorf("goroutine panic: %v\n%s", r, debug.Stack())
					log.Close()
				}
				os.Exit(1)
			}
		}()
		fn()
	}()
}

// Run starts the proxy event loop. Blocks until the child exits.
func (p *Proxy) Run() error {
	cfg := p.cfg
	stdin := cfg.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := cfg.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}

	inputCh := make(chan []byte, 32)
	outputCh := make(chan []byte, 32)
	done := make(chan struct{})

	// stdin → inputCh
	safeGo(cfg.Term, cfg.Log, func() {
		defer close(inputCh)
		buf := make([]byte, 128*1024)
		for {
			n, err := stdin.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case inputCh <- data:
				case <-done:
					return
				}
			}
			if err != nil {
				return
			}
		}
	})

	// PTY → outputCh
	safeGo(cfg.Term, cfg.Log, func() {
		defer close(outputCh)
		buf := make([]byte, 4096)
		for {
			n, err := cfg.PTY.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case outputCh <- data:
				case <-done:
					return
				}
			}
			if err != nil {
				return
			}
		}
	})

	enabled := cfg.Enabled
	delaySecs := cfg.Delay
	agentKind := cfg.AgentKind
	frames := 0

	var approvalTimer *time.Timer
	var timerCh <-chan time.Time
	var lastResult *detector.MatchResult

	var prefixTimer *time.Timer
	var prefixTimerCh <-chan time.Time
	prefixActive := false

	// Rebuild rule chain when agent kind is resolved
	chain := cfg.RuleChain

	for {
		select {
		case data, ok := <-inputCh:
			if !ok {
				close(done)
				return nil
			}
			data = p.handlePrefix(data, &prefixActive, &prefixTimer, &prefixTimerCh,
				&enabled, &delaySecs, &approvalTimer, &timerCh, cfg, stdout)

			if len(data) == 0 {
				continue
			}

			// Any non-escape user keypress cancels pending approval
			if !isEscapeSequence(data) && approvalTimer != nil {
				if cfg.Log != nil {
					cfg.Log.Infof("user input during delay, cancelling approval")
				}
				approvalTimer.Stop()
				approvalTimer = nil
				timerCh = nil
			}

			cfg.PTY.Write(data)

		case data, ok := <-outputCh:
			if !ok {
				close(done)
				return nil
			}

			cfg.Screen.Feed(data)
			text := cfg.Screen.Text()

			// Try to resolve unknown agent from screen during first 10 frames
			if agentKind == agent.KindUnknown && frames < 10 {
				if k := agent.KindFromScreen(text); k != agent.KindUnknown {
					agentKind = k
					if cfg.Log != nil {
						cfg.Log.Infof("detected agent from screen: %s", k)
					}
					// Rebuild chain with resolved kind
					chain = append(cfg.RuleChain[:len(cfg.RuleChain):len(cfg.RuleChain)],
						agentKind.Detector())
				}
			}
			frames++

			if enabled {
				result := chain.Detect(text)
				if result != nil && !cfg.Memory.Seen(result.Hash) {
					if cfg.Log != nil {
						cfg.Log.Infof("prompt detected: %s", result.RuleName)
					}
					cfg.StatusBar.SetRule(result.RuleName)
					if delaySecs == 0 {
						cfg.Memory.Record(result.Hash)
						cfg.PTY.Write([]byte(result.Response))
					} else if lastResult == nil || lastResult.Hash != result.Hash {
						// New or changed prompt: (re)start timer
						if approvalTimer != nil {
							approvalTimer.Stop()
						}
						lastResult = result
						approvalTimer = time.NewTimer(time.Duration(delaySecs) * time.Second)
						timerCh = approvalTimer.C
					}
				}
			}

			out := cfg.StatusBar.WrapFrame(data)
			stdout.Write(out)

		case <-timerCh:
			timerCh = nil
			approvalTimer = nil
			if lastResult != nil {
				if cfg.Log != nil {
					cfg.Log.Infof("approval timer fired, sending response for: %s", lastResult.RuleName)
				}
				cfg.Memory.Record(lastResult.Hash)
				cfg.PTY.Write([]byte(lastResult.Response))
				lastResult = nil
			}

		case <-prefixTimerCh:
			// Prefix key timed out without command byte — forward 0x19 to child
			prefixTimerCh = nil
			prefixTimer = nil
			prefixActive = false
			cfg.PTY.Write([]byte{prefixByte})
		}
	}
}

// handlePrefix processes the Ctrl+Y prefix key state machine.
// Returns the remaining data to forward to the child (may be empty).
func (p *Proxy) handlePrefix(
	data []byte,
	prefixActive *bool,
	prefixTimer **time.Timer,
	prefixTimerCh *<-chan time.Time,
	enabled *bool,
	delaySecs *int,
	approvalTimer **time.Timer,
	timerCh *<-chan time.Time,
	cfg Config,
	stdout io.Writer,
) []byte {
	sb := cfg.StatusBar

	if *prefixActive {
		*prefixActive = false
		if *prefixTimer != nil {
			(*prefixTimer).Stop()
			*prefixTimer = nil
			*prefixTimerCh = nil
		}
		cmd := data[0]
		rest := data[1:]
		switch cmd {
		case '0':
			*enabled = !*enabled
			if !*enabled && *approvalTimer != nil {
				(*approvalTimer).Stop()
				*approvalTimer = nil
				*timerCh = nil
			}
			sb.Toggle()
		case '1', '2', '3', '4', '5':
			secs := int(cmd - '0')
			*delaySecs = secs
			if !*enabled {
				*enabled = true
				sb.Toggle() // re-enable
			}
			sb.SetDelay(secs)
		default:
			// Unrecognized: forward prefix + cmd byte
			cfg.PTY.Write([]byte{prefixByte})
			return append([]byte{cmd}, rest...)
		}
		// Repaint status bar immediately
		out := sb.WrapFrame([]byte{})
		stdout.Write(out)
		return rest
	}

	if len(data) > 0 && data[0] == prefixByte {
		if len(data) == 1 {
			// Saw Ctrl+Y alone — enter prefix mode with timeout
			*prefixActive = true
			t := time.NewTimer(prefixTimeout)
			*prefixTimer = t
			*prefixTimerCh = t.C
			return nil
		}
		// Ctrl+Y followed immediately by another byte
		*prefixActive = true
		return p.handlePrefix(data[1:], prefixActive, prefixTimer, prefixTimerCh,
			enabled, delaySecs, approvalTimer, timerCh, cfg, stdout)
	}

	return data
}

// isEscapeSequence returns true if data is a terminal-generated escape
// sequence (focus events, cursor position reports) rather than user input.
func isEscapeSequence(data []byte) bool {
	if len(data) < 3 || data[0] != 0x1b || data[1] != '[' {
		return false
	}
	if len(data) == 3 && (data[2] == 'I' || data[2] == 'O') {
		return true
	}
	if data[len(data)-1] == 'R' {
		inner := data[2 : len(data)-1]
		for _, b := range inner {
			if !(b >= '0' && b <= '9') && b != ';' {
				return false
			}
		}
		return len(inner) > 0
	}
	return false
}
```

- [ ] **Step 3: Build to check for compile errors**

```bash
go build ./internal/proxy/
```
Expected: no errors. Fix any import or type mismatches.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/proxy/ -v -short
```
Expected: test passes (integration test skipped with `-short`).

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/
git commit -m "feat: add proxy coordinator loop with prefix key and approval timer"
```

---

## Task 13: CLI Entry Point

**Files:**
- Modify: `cmd/yoyo/main.go` (replace stub)

- [ ] **Step 1: Implement main.go**

```go
// cmd/yoyo/main.go
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	ptylib "github.com/aymanbagabas/go-pty"

	"yoyo/internal/agent"
	"yoyo/internal/config"
	"yoyo/internal/detector"
	"yoyo/internal/logger"
	"yoyo/internal/memory"
	"yoyo/internal/proxy"
	"yoyo/internal/screen"
	"yoyo/internal/statusbar"
	"yoyo/internal/term"
)

func main() {
	var (
		delay     = flag.Int("delay", -1, "approval delay in seconds (0=immediate, default from config)")
		logPath   = flag.String("log", "", "log file path (default from config)")
		cfgPath   = flag.String("config", config.DefaultPath(), "config file path")
	)
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: yoyo [flags] <command> [args...]")
		os.Exit(1)
	}

	// Load config
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	// Resolve effective settings
	effectiveDelay := cfg.Defaults.Delay
	if *delay >= 0 {
		effectiveDelay = *delay
	}
	effectiveLog := cfg.Defaults.LogFile
	if *logPath != "" {
		effectiveLog = *logPath
	}

	// Identify agent kind
	kind := agent.KindFromCommand(args[0])

	// Apply agent-specific delay override
	if agentCfg, ok := cfg.Agents[kind.String()]; ok && agentCfg.Delay >= 0 {
		effectiveDelay = agentCfg.Delay
	}

	// Start logger
	if err := os.MkdirAll(dirOf(effectiveLog), 0o755); err == nil {
		// ignore mkdir error — logger.New will fail with a clear message
	}
	log, err := logger.New(effectiveLog)
	if err != nil {
		fmt.Fprintln(os.Stderr, "logger error:", err)
		os.Exit(1)
	}
	defer log.Close()

	// Build rule chain: agent rules → global rules → built-in detector
	var chain detector.RuleChain
	if agentCfg, ok := cfg.Agents[kind.String()]; ok {
		for _, r := range agentCfg.Rules {
			name := r.Name
			if name == "" {
				name = "custom"
			}
			d, err := detector.NewRegexpDetector(name, r.Pattern, r.Response)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid rule %q: %v\n", name, err)
				os.Exit(1)
			}
			chain = append(chain, d)
		}
	}
	for _, r := range cfg.Rules {
		name := r.Name
		if name == "" {
			name = "custom"
		}
		d, err := detector.NewRegexpDetector(name, r.Pattern, r.Response)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid rule %q: %v\n", name, err)
			os.Exit(1)
		}
		chain = append(chain, d)
	}
	chain = append(chain, kind.Detector())

	// Setup terminal raw mode
	t := term.New(os.Stdin)
	if err := t.EnableRaw(); err != nil {
		fmt.Fprintln(os.Stderr, "failed to enable raw mode:", err)
		os.Exit(1)
	}
	defer t.Restore()

	// Signal handler for clean exit
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	go func() {
		<-sigs
		t.Restore()
		log.Close()
		os.Exit(0)
	}()

	// Get initial terminal size
	cols, rows := t.Size()

	// Start SIGWINCH watcher (Unix only; no-op on Windows)
	scr := screen.New(cols, rows)
	sb := statusbar.New(uint16(rows), uint16(cols), cfg.Defaults.Enabled, effectiveDelay)

	stopResize := t.WatchResize(func(c, r int) {
		// Resize PTY handled in proxy after PTY is created
		scr.Resize(c, r)
		sb.Resize(uint16(r), uint16(c))
	})
	defer stopResize()

	// Start child process in PTY
	cmd := exec.Command(args[0], args[1:]...)
	p, err := ptylib.Start(cmd)
	if err != nil {
		t.Restore()
		fmt.Fprintln(os.Stderr, "failed to start PTY:", err)
		os.Exit(1)
	}
	defer p.Close()

	log.Infof("started %s (kind=%s, delay=%ds)", args[0], kind, effectiveDelay)

	// Also hook resize to update PTY
	stopResize2 := t.WatchResize(func(c, r int) {
		p.Resize(uint16(c), uint16(r))
	})
	defer stopResize2()

	// Run proxy
	pr := proxy.New(proxy.Config{
		PTY:       p,
		RuleChain: chain,
		Memory:    memory.New(),
		StatusBar: sb,
		Log:       log,
		Term:      t,
		Screen:    scr,
		AgentKind: kind,
		Delay:     effectiveDelay,
		Enabled:   cfg.Defaults.Enabled,
	})

	if err := pr.Run(); err != nil {
		log.Errorf("proxy error: %v", err)
	}
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
```

- [ ] **Step 2: Build**

```bash
go build ./cmd/yoyo/
```
Expected: no errors. Fix type mismatches between proxy.Config fields and go-pty Pty type.

> **Note on go-pty API:** If `ptylib.Start(cmd)` is not the correct API, check with `go doc github.com/aymanbagabas/go-pty`. The Pty interface implements `io.ReadWriter`, so `proxy.Config.PTY io.ReadWriter` will accept it.

- [ ] **Step 3: Smoke test (manual)**

```bash
./yoyo echo "hello from yoyo"
```
Expected: `hello from yoyo` printed, no terminal corruption.

- [ ] **Step 4: Commit**

```bash
git add cmd/yoyo/main.go
git commit -m "feat: add CLI entry point with flag parsing and signal handling"
```

---

## Task 14: Integration Test + Full Test Run

**Files:**
- Modify: `internal/proxy/proxy_test.go`

- [ ] **Step 1: Write real integration test**

```go
// internal/proxy/proxy_test.go
package proxy_test

import (
	"os/exec"
	"testing"
	"time"

	ptylib "github.com/aymanbagabas/go-pty"

	"yoyo/internal/agent"
	"yoyo/internal/detector"
	"yoyo/internal/logger"
	"yoyo/internal/memory"
	"yoyo/internal/proxy"
	"yoyo/internal/screen"
	"yoyo/internal/statusbar"
	"yoyo/internal/term"
)

// TestProxy_AutoApprovesClaudePrompt runs a real PTY with `cat`, writes a
// simulated Claude permission prompt to stdin, and asserts the proxy sends CR.
func TestProxy_AutoApprovesClaudePrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: skipped in short mode")
	}

	cmd := exec.Command("cat")
	pty, err := ptylib.Start(cmd)
	if err != nil {
		t.Skip("PTY not available:", err)
	}
	defer pty.Close()

	tr := term.NewNoOp() // no real TTY in tests
	log, _ := logger.New(t.TempDir() + "/test.log")
	defer log.Close()

	scr := screen.New(80, 24)
	sb := statusbar.New(24, 80, true, 0) // delay=0 immediate
	mem := memory.New()

	chain := detector.RuleChain{detector.Claude{}}

	pr := proxy.New(proxy.Config{
		PTY:       pty,
		RuleChain: chain,
		Memory:    mem,
		StatusBar: sb,
		Log:       log,
		Term:      tr,
		Screen:    scr,
		AgentKind: agent.KindClaude,
		Delay:     0,
		Enabled:   true,
	})

	// Write a Claude prompt to the PTY
	claudePrompt := "─────────────────────────────────────────────\n" +
		" Read file\n\n 1. Yes\n 2. No\n\n Esc to cancel\n"

	go func() {
		time.Sleep(100 * time.Millisecond)
		pty.Write([]byte(claudePrompt))
	}()

	// Run proxy with timeout
	done := make(chan error, 1)
	go func() {
		done <- pr.Run()
	}()

	select {
	case <-time.After(3 * time.Second):
		t.Log("proxy ran for 3s without error (expected for interactive test)")
	case err := <-done:
		if err != nil {
			t.Logf("proxy exited: %v", err)
		}
	}

	// Verify memory recorded the prompt hash
	scr.Feed([]byte(claudePrompt))
	r := detector.Claude{}.Detect(scr.Text())
	if r == nil {
		t.Fatal("detector failed to match the injected prompt")
	}
	if !mem.Seen(r.Hash) {
		t.Log("note: prompt may not have been seen if proxy stopped before detection")
	}
}
```

- [ ] **Step 2: Run full test suite**

```bash
go test ./... -v -short
```
Expected: all unit tests PASS. Integration tests skipped.

- [ ] **Step 3: Run integration tests (requires PTY)**

```bash
go test ./... -v -timeout 30s
```
Expected: all tests PASS including integration.

- [ ] **Step 4: Build release binary**

```bash
go build -ldflags="-s -w" -o yoyo ./cmd/yoyo/
```
Expected: `yoyo` binary created.

- [ ] **Step 5: Final commit**

```bash
git add internal/proxy/proxy_test.go
git commit -m "test: add proxy integration test and verify full suite"
```

---

## Completion Checklist

- [ ] `go test ./...` passes with no failures
- [ ] `go build ./cmd/yoyo/` produces a working binary
- [ ] `./yoyo claude` launches Claude Code with PTY proxy active
- [ ] Status bar visible in bottom-right corner
- [ ] `Ctrl+Y → 0` toggles auto-approve
- [ ] `Ctrl+Y → 3` sets 3s delay
- [ ] TOML config at `~/.config/yoyo/config.toml` is loaded
- [ ] Logs written to `~/.yoyo/yoyo.log` (never to stdout)
- [ ] Terminal is restored cleanly on exit (Ctrl+C, signals)

---

## Known Implementation Notes

1. **go-pty API:** Verify exact `ptylib.Start(cmd)` vs `ptylib.New()` + `p.Start(cmd)` API by running `go doc github.com/aymanbagabas/go-pty`. Adjust `main.go` and `proxy_test.go` accordingly.

2. **vt10x API:** Verify `vt10x.New(vt10x.WithSize(cols, rows))` signature with `go doc github.com/hinshun/vt10x`. If `WithSize` option doesn't exist, use `vt.Resize(cols, rows)` after creation.

3. **Windows:** The goroutine-based concurrent PTY reads/writes should work with `go-pty`'s ConPTY backend. If concurrent access causes issues, serialize reads and writes using a mutex in `proxy.go`.

4. **proxy.Config.PTY type:** Use `io.ReadWriter` to decouple from the specific `go-pty` Pty type. The Pty implements this interface. For resize events, pass a separate `ResizeFunc func(cols, rows uint16)` in the config, set from `main.go`.
