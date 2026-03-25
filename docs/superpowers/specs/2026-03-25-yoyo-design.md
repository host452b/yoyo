# yoyo — Design Spec

**Date:** 2026-03-25
**Status:** Approved
**Summary:** Go CLI tool (`yoyo`) — cross-platform PTY proxy that auto-approves AI agent permission prompts with rule-based response selection, session memory, and runtime controls.

---

## 1. Overview

`yoyo` (you only yes once) is a PTY proxy that sits between the user's terminal and an AI agent CLI (Claude Code, Codex, Cursor, etc.). It:

1. Detects permission/confirmation prompts on the virtual terminal screen
2. Matches them against built-in presets or user-defined rules
3. Sends the configured response (Enter, `1\r`, `y\r`, etc.) after an optional delay
4. Remembers previously approved prompts within the session (same prompt → skip delay)
5. Exposes a bottom-right status bar and `Ctrl+Y` prefix keyboard controls

---

## 2. Usage

```bash
yoyo claude                        # launch Claude Code with auto-approve
yoyo codex                         # launch Codex CLI
yoyo cursor-agent                  # launch Cursor Agent
yoyo --delay 5 claude              # 5s delay before approval
yoyo --log /tmp/yoyo.log claude    # override log file path
yoyo --config ~/.yoyo/config.toml claude
```

### CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--delay <n>` | `3` | Integer seconds to wait before auto-approving (0 = immediate) |
| `--log <path>` | `~/.yoyo/yoyo.log` | Log file path (never writes to stdout) |
| `--config <path>` | `~/.config/yoyo/config.toml` | Config file path |

Delay is **integer seconds only**. Sub-second precision is not supported; the keyboard shortcuts (`0`–`5`) reinforce this constraint.

### Keyboard Shortcuts (Ctrl+Y prefix)

Prefix byte: `0x19` (`Ctrl+Y`). After `Ctrl+Y` is received, the proxy waits up to **1500ms** for the command byte:

| Keys | Action |
|------|--------|
| `Ctrl+Y → 0` | Toggle auto-approve on/off |
| `Ctrl+Y → 1` | Set delay to 1s (re-enables if toggled off) |
| `Ctrl+Y → 2` | Set delay to 2s (re-enables if toggled off) |
| `Ctrl+Y → 3` | Set delay to 3s (re-enables if toggled off) |
| `Ctrl+Y → 4` | Set delay to 4s (re-enables if toggled off) |
| `Ctrl+Y → 5` | Set delay to 5s (re-enables if toggled off) |

**Prefix timeout:** If no command byte arrives within 1500ms, the buffered `0x19` byte is forwarded to the child process unchanged.

**Unrecognized command byte:** Forward the original `0x19` prefix byte plus the unrecognized byte to the child.

**Delay keys while toggled off:** Pressing `Ctrl+Y → 1`–`5` re-enables auto-approve and sets the delay. Only `Ctrl+Y → 0` acts as a pure toggle.

`Ctrl+Y` is the prefix key (Y = Yoyo). On macOS, `Ctrl+1–5` conflicts with Mission Control virtual desktop shortcuts at the OS level; the prefix approach is cross-platform safe.

---

## 3. Architecture

### Module Structure

```
yoyo/
├── cmd/yoyo/
│   └── main.go             # CLI entry, flag parsing, signal handling
├── internal/
│   ├── config/             # TOML config loading (go-toml/v2), tilde expansion
│   ├── detector/           # Prompt detector interface + per-agent impls
│   │   ├── detector.go     # Detector interface, MatchResult, AgentKind, RuleChain
│   │   ├── claude.go       # Claude Code prompt detector
│   │   ├── codex.go        # Codex CLI prompt detector
│   │   └── cursor.go       # Cursor Agent prompt detector
│   ├── agent/              # AgentKind identification (CLI name + screen heuristic)
│   ├── memory/             # Session-scoped approval memory (hash map)
│   ├── screen/             # vt10x wrapper (feed bytes → extract text)
│   ├── statusbar/          # Bottom-right ANSI overlay injection
│   ├── term/               # Raw mode enable/restore + terminal utils
│   ├── logger/             # Async file logger (never writes to stdout)
│   └── proxy/              # Main goroutine coordinator loop
├── go.mod
└── go.sum
```

### Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/aymanbagabas/go-pty` | Cross-platform PTY (Unix + Windows ConPTY) |
| `github.com/hinshun/vt10x` | VT100 virtual terminal screen buffer |
| `github.com/pelletier/go-toml/v2` | TOML 1.0 config parsing |

---

## 4. Concurrency Model

```
┌─────────────────────────────────────────────────────────────┐
│  main goroutine (proxy coordinator)                          │
│                                                              │
│  safeGo → stdinReader() ─────► inputCh  chan []byte         │
│  safeGo → ptyReader()   ─────► outputCh chan []byte         │
│                                                              │
│  var approvalTimer *time.Timer   // nil when no pending      │
│  var timerCh <-chan time.Time    // set from approvalTimer.C │
│  var lastResult *MatchResult    // saved when timer started  │
│                                                              │
│  for { select {                                              │
│    case data := <-inputCh:                                   │
│        handleInput(data)                                     │
│        // if user key (non-escape): cancel pending timer     │
│        //   approvalTimer.Stop(); timerCh = nil              │
│        // if Ctrl+Y prefix: handle shortcut                  │
│        // else: forward to child PTY                         │
│                                                              │
│    case data := <-outputCh:                                  │
│        screen.Feed(data)                                     │
│        result = ruleChain.Detect(screen.Text())              │
│        if result != nil && !memory.Seen(result.Hash):        │
│            if delay == 0: writeToPTY("\r")                   │
│            else: startTimer(delay)                           │
│              // lastResult = result                          │
│              // approvalTimer = time.NewTimer(delay)         │
│              // timerCh = approvalTimer.C                    │
│        statusbar.Wrap(data) → write stdout                   │
│                                                              │
│    case <-timerCh:                                           │
│        timerCh = nil                                         │
│        memory.Record(lastResult.Hash)                        │
│        writeToPTY("\r" or result.Response)                   │
│                                                              │
│    case <-done:                                              │
│        return                                                │
│  }}                                                          │
└─────────────────────────────────────────────────────────────┘
```

**Timer lifecycle:**
- Created with `time.NewTimer(duration)` when a new prompt is detected (and delay > 0)
- `approvalTimer.Stop()` + `timerCh = nil` when user types a key (aborting auto-approval)
- `approvalTimer.Stop()` + reset when a new distinct prompt is detected while one is pending
- `timerCh = nil` after firing to prevent double-send

`safeGo` is a utility wrapper:
```go
func safeGo(t *term.Term, fn func()) {
    go func() {
        defer func() {
            if r := recover(); r != nil {
                t.Restore() // os.Exit bypasses defers; explicit restore here is the only safe path
                logger.Errorf("goroutine panic: %v\n%s", r, debug.Stack())
                os.Exit(1)
            }
        }()
        fn()
    }()
}
```

**Note:** `os.Exit(1)` inside `safeGo` bypasses `main`'s `defer t.Restore()`. The explicit `t.Restore()` call in `safeGo` before `os.Exit` is therefore the sole terminal-restore path for background goroutine panics.

---

## 5. Core Interfaces

```go
// detector/detector.go

// MatchResult is returned by a Detector when a permission prompt is found.
// Hash is computed by the detector from the prompt body (sha256 hex string)
// and used by the memory subsystem for deduplication.
type MatchResult struct {
    RuleName string // displayed in status bar, e.g. "Claude:yes-session"
    Response string // sent to child PTY, e.g. "\r", "2\r", "y\r"
    Hash     string // sha256(prompt body), computed inside Detect()
}

type Detector interface {
    Detect(screenText string) *MatchResult
}

// RuleChain evaluates a prioritized list of Detectors, returning the first match.
// It is constructed from: agent-specific config rules → global config rules → built-in detectors.
type RuleChain []Detector

func (rc RuleChain) Detect(screenText string) *MatchResult {
    for _, d := range rc {
        if m := d.Detect(screenText); m != nil {
            return m
        }
    }
    return nil
}

type AgentKind int
const (
    AgentUnknown AgentKind = iota
    AgentClaude
    AgentCodex
    AgentCursor
)
```

```go
// memory/memory.go

type Memory interface {
    Seen(hash string) bool   // true if this prompt was already approved this session
    Record(hash string)      // mark as approved
}
```

```go
// config/config.go

type Rule struct {
    Name     string // optional, shown in status bar; defaults to "custom"
    Pattern  string // regexp
    Response string // "\r" | "1\r" | "y\r" | custom
}

type AgentConfig struct {
    Delay int    // integer seconds, 0 = immediate; -1 = inherit from defaults
    Rules []Rule
}

type Config struct {
    Defaults struct {
        Delay   int    // integer seconds, default 3
        Enabled bool   // default true
        LogFile string // tilde-expanded before use
    }
    Agents map[string]AgentConfig // "claude", "codex", "cursor"
    Rules  []Rule                 // global custom rules, applied after agent-specific rules
}
```

**Tilde expansion:** All path-valued config fields (`LogFile`, `--log`, `--config`) are expanded via `os.UserHomeDir()` before use. `go-toml` does not perform this automatically.

---

## 6. Rule Matching Priority

`RuleChain` is assembled at startup in this order (first match wins):

1. Agent-specific rules from config file (`agents.<name>.rules`) — compiled to `RegexpDetector`
2. Global custom rules from config file (`[[rules]]`) — compiled to `RegexpDetector`
3. Built-in agent preset detector (e.g. `detector.Claude{}`)
4. If `AgentKind` is `Unknown`: try all built-in detectors in order (Claude → Codex → Cursor)

---

## 7. AgentKind Detection

### From CLI command name

```go
// agent/agent.go
func KindFromCommand(command string) AgentKind {
    // strip path and Windows extensions (.exe, .cmd, .bat)
    // match stem: "claude" → AgentClaude, "codex" → AgentCodex, "cursor" → AgentCursor
    // else → AgentUnknown
}
```

### From screen content (fallback for unknown agents)

When `AgentKind` is `AgentUnknown`, the proxy inspects screen content during the first **10 output frames** to identify the agent:

```go
// agent/agent.go
func KindFromScreen(screenText string) AgentKind {
    switch {
    case strings.Contains(screenText, "Claude Code"):
        return AgentClaude
    case strings.Contains(screenText, "codex"), strings.Contains(screenText, "Codex"):
        return AgentCodex
    case strings.Contains(screenText, "cursor"), strings.Contains(screenText, "Cursor"):
        return AgentCursor
    }
    return AgentUnknown
}
```

After 10 frames, if still `AgentUnknown`, the proxy falls back to trying all built-in detectors on every frame (see §6, item 4). The `RuleChain` is rebuilt when `AgentKind` is resolved.

---

## 8. Built-in Detectors

### Claude Code
Matches the structured block delimited by `─` separator (top) and `Esc to cancel` (bottom). Falls back to `Do you want to` as top boundary if separator scrolled off. Requires both `Yes` and `No` options to be present (guards against partial renders).

Default response: `"\r"` (sends Enter, selects highlighted option 1).

### Codex CLI
Matches prompts starting with `Would you like to` or `needs your approval`, ending with `Press enter to confirm or esc to cancel`.

Default response: `"\r"`.

### Cursor Agent
Matches box-drawn prompts (`┌─┐` / `└─┘`) containing `(y)` and `n)` options.

Default response: `"\r"`.

---

## 9. Session Memory

After a prompt response is sent, its `Hash` (sha256 of the prompt body, computed in `Detect()`) is recorded. On the next occurrence:
- Delay is skipped
- Response is sent immediately
- Status bar shows `[yoyo: on | seen: <RuleName>]`

Memory is an in-memory map scoped to the process lifetime — not persisted across sessions (security: re-evaluate on new runs).

---

## 10. Status Bar

Bottom-right overlay, injected around each PTY output frame without corrupting terminal content.

```
[yoyo: on 3s | Claude:yes-session]   ← rule fired, waiting delay
[yoyo: on 3s | seen: Claude:yes]     ← memory hit, immediate send
[yoyo: off]                          ← toggled off
```

Implementation mirrors `aaa`'s `StatusBar`: cursor save (`\x1b[s`) → move to row/col (`\x1b[R;CH`) → paint → cursor restore (`\x1b[u`), prepended as clear-previous and appended as paint-new around each PTY output frame. Skips injection when the frame ends mid-escape or mid-UTF-8 sequence.

---

## 11. TOML Configuration

Default location: `~/.config/yoyo/config.toml` (tilde expanded at runtime).

```toml
[defaults]
delay = 3        # integer seconds, 0 = immediate
enabled = true
log_file = "~/.yoyo/yoyo.log"  # tilde expanded at runtime

[agents.claude]
delay = 1

  [[agents.claude.rules]]
  name = "yes-session"
  pattern = "allow.*session"
  response = "2\r"   # choose option 2 (session-wide permission)

[agents.codex]
delay = 0  # immediate

[[rules]]
name = "generic-confirm"
pattern = "Are you sure"
response = "y\r"
```

---

## 12. Error Handling & Safety

### Terminal Restoration
```go
func main() {
    t := term.New(os.Stdin)
    if err := t.EnableRaw(); err != nil {
        fmt.Fprintln(os.Stderr, "failed to enable raw mode:", err)
        os.Exit(1)
    }
    defer t.Restore() // covers normal return and panics in main goroutine only

    // Note: os.Exit called from background goroutines (safeGo) bypasses this defer.
    // safeGo calls t.Restore() explicitly before os.Exit(1).

    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
    go func() { <-sig; t.Restore(); os.Exit(0) }()
}
```

Signals covered: `SIGINT`, `SIGTERM`, `SIGHUP`, `SIGQUIT`.

**`SIGTSTP` (Ctrl+Z):** Out of scope for v1. The raw `0x1a` byte from stdin is forwarded to the child process unchanged (the child handles suspend itself). A full suspend/resume cycle (drain PTY, restore raw mode, re-raise `SIGTSTP`, re-enter raw mode on `SIGCONT`) may be added in a later version.

### Goroutine Safety
All background goroutines use `safeGo(t, fn)` (see §4). `os.Exit` in `safeGo` bypasses `main`'s `defer` — the explicit `t.Restore()` before `os.Exit` in `safeGo` is the sole restore path for goroutine panics.

### PTY Errors
- `io.EOF` / `EIO` from PTY reader → child exited → close `done` channel → coordinator exits cleanly
- PTY creation failure → fatal before raw mode is enabled; `os.Stderr` is safe to write

### Logging
All runtime logging goes to file only (never stdout/stderr while PTY is active). Tilde in log path is expanded via `os.UserHomeDir()`.

---

## 13. Platform Support

| Platform | Architecture | PTY Backend |
|----------|-------------|-------------|
| Linux | x86_64, aarch64 | Unix PTY (`go-pty`) |
| macOS | x86_64, aarch64 | Unix PTY (`go-pty`) |
| Windows | x86_64 | ConPTY (`go-pty`) |

**Unix:** handles `SIGWINCH` (terminal resize via `go-pty`'s `Resize` method).
**Windows:** `go-pty`'s ConPTY backend abstracts the Win32 API. Goroutine-based concurrent reads/writes on Windows ConPTY should be validated during implementation; a serialized I/O loop (similar to `aaa`'s separate `win_proxy.rs`) may be needed if concurrent access is unsafe.
**Ctrl+Z (SIGTSTP):** forwarded to child as-is in v1 (see §12).

---

## 14. Testing Strategy

| Layer | Approach |
|-------|---------|
| `detector/` | Pure unit tests, screen text in → MatchResult out. Port all 14 test cases from `aaa`'s `claude.rs`. |
| `memory/` | Pure unit tests |
| `config/` | Table-driven TOML parsing tests including tilde expansion |
| `statusbar/` | Unit tests verifying ANSI escape sequence output |
| `proxy/` | Integration tests: spawn real subprocess (e.g. `cat`), write simulated prompts via PTY, assert correct response sent |
| `agent/` | Unit tests for `KindFromCommand` and `KindFromScreen` |
