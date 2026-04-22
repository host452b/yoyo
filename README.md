```
 ██    ██  ██████  ██    ██  ██████
  ██  ██  ██    ██  ██  ██  ██    ██
   ████   ██    ██   ████   ██    ██
    ██    ██    ██    ██    ██    ██
    ██     ██████     ██     ██████
```

# yoyo

**you only yes once** — a PTY proxy that auto-approves AI agent permission prompts.

yoyo sits between your terminal and an AI agent CLI (Claude Code, Codex, Cursor, …).
It watches the agent's output, detects permission prompts, and automatically sends the
confirmation keystroke after a configurable delay — so you don't have to babysit it.

---

## Installation

### One-line install (Linux & macOS)

```bash
curl -fsSL https://github.com/host452b/yoyo/releases/latest/download/install.sh | sh
```

Auto-detects your OS and architecture (linux/darwin × amd64/arm64). No Go required.

---

<details>
<summary>Manual options</summary>

### Pre-built binary

```bash
# Linux (amd64)
curl -L https://github.com/host452b/yoyo/releases/latest/download/yoyo-linux-amd64 -o yoyo
chmod +x yoyo && sudo mv yoyo /usr/local/bin/

# Linux (arm64)
curl -L https://github.com/host452b/yoyo/releases/latest/download/yoyo-linux-arm64 -o yoyo
chmod +x yoyo && sudo mv yoyo /usr/local/bin/

# macOS (Apple Silicon)
curl -L https://github.com/host452b/yoyo/releases/latest/download/yoyo-darwin-arm64 -o yoyo
chmod +x yoyo && sudo mv yoyo /usr/local/bin/

# macOS (Intel)
curl -L https://github.com/host452b/yoyo/releases/latest/download/yoyo-darwin-amd64 -o yoyo
chmod +x yoyo && sudo mv yoyo /usr/local/bin/
```

### go install (requires Go 1.21+)

```bash
go install github.com/host452b/yoyo/cmd/yoyo@latest
```

> Run `go version` to check. If you have Go < 1.21, use the one-line install above.

### Build from source

```bash
git clone https://github.com/host452b/yoyo.git
cd yoyo
go build -o yoyo ./cmd/yoyo
sudo mv yoyo /usr/local/bin/
```

</details>

### Verify

```bash
yoyo -h
```

---

## Quick Start

```bash
# Wrap claude with default settings (3-second delay before auto-approve)
yoyo claude

# Approve immediately (no delay)
yoyo -delay 0 claude

# Wrap codex with a 5-second review window
yoyo -delay 5 codex
```

---

## How It Works

```
┌──────────┐   stdin    ┌───────┐   stdin    ┌───────────┐
│ Terminal │ ─────────► │ yoyo  │ ─────────► │ AI agent  │
│          │ ◄───────── │ proxy │ ◄───────── │  (PTY)    │
└──────────┘  stdout    └───────┘  stdout    └───────────┘
                            │
                    detect prompt?
                    wait delay secs
                    send "yes" key
```

1. yoyo spawns the agent inside a PTY (pseudo-terminal) so the agent behaves exactly as if a human were typing.
2. Every frame of PTY output is fed through a VT100 screen buffer to get the visible text.
3. The text is matched against a rule chain (custom rules → built-in detectors).
4. On a match, yoyo waits `delay` seconds, then sends the approval keystroke.
5. Any real keypress during the countdown cancels the pending auto-approve.
6. A status bar in the bottom-right corner shows the current state at all times.

---

## Status Bar

```
[yoyo: on 3s]           enabled, 3-second delay, no prompt detected yet
[yoyo: on 2s | Claude]  prompt detected — countdown active (2s remaining)
[yoyo: on 0s | seen: X] already approved this session — sent immediately
[yoyo: off]             auto-approve disabled (manual mode)
[yoyo: ^Y …]           waiting for Ctrl+Y command key
[yoyo: dry 3s]          dry-run mode — detects but does not approve
```

- **Green** = auto-approve active
- **Yellow** = countdown in progress, dry-run, or waiting for Ctrl+Y command
- **Red** = auto-approve off

---

## Supported Agents

| Agent | Command | Detection method |
|-------|---------|-----------------|
| Claude Code | `claude` | `───` bordered permission box + Yes/No options |
| OpenAI Codex | `codex` | "Would you like to" / "needs your approval" + "Press enter to confirm" |
| Cursor | `cursor`, `cursor-agent` | Box-drawn `┌─┐` prompt with `(y)` / `n)` options |
| Unknown | any command | All three detectors run in parallel; agent auto-identified from screen within first 10 frames |

---

## CLI Reference

```
yoyo [flags] <command> [args...]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-delay int` | `3` (from config) | Seconds to wait before auto-approving. `0` = approve immediately. `-1` = use config value. Explicit flag always takes priority over per-agent config. |
| `-config string` | `~/.config/yoyo/config.toml` | Path to TOML config file. Supports `~/`. |
| `-log string` | `~/.yoyo/yoyo.log` | Path to log file. Supports `~/`. |
| `-dry-run` | off | Detect prompts but do not send approval keystrokes. The status bar shows `dry` instead of `on`. Useful for testing custom rules. |
| `-v` | | Print version and exit. |

Run `yoyo -h` for the full built-in reference.

### Runtime Controls

The prefix key is **Ctrl+Y**. Press Ctrl+Y, then:

| Key | Action |
|-----|--------|
| `0` | Toggle auto-approve on/off |
| `1`–`5` | Set delay to N seconds (also re-enables if currently off) |

**Cancel pending approval:** press any non-escape key while the countdown is running.

### AFK mode

Some agent prompts don't match yoyo's detectors and the agent blocks on
`read()` forever. `-afk` sets a dumb idle timer that nudges the agent
after a configured silence:

```
yoyo -afk -afk-idle 10m claude
```

Every time the terminal sees no output *and* no input for the idle window,
yoyo injects `y` + Enter, pauses briefly, then sends
`continue, Choose based on your project understanding.` + Enter, and
rearms. Toggle at runtime with `Ctrl+Y a`.

---

## Config File

Default location: `~/.config/yoyo/config.toml`

```toml
[defaults]
delay    = 3              # approval delay in seconds (0 = immediate)
enabled  = true           # start with auto-approve on
log_file = "~/.yoyo/yoyo.log"

# Per-agent delay overrides (overridden by explicit -delay flag)
[agents.claude]
delay = 0                 # approve Claude prompts immediately

[agents.codex]
delay = 5                 # give codex prompts 5 seconds to review

# Global custom rules — checked before built-in detectors, in order
[[rules]]
name     = "my-tool-confirm"
pattern  = "Continue\\? \\[y/N\\]"   # Go regexp matched against full screen text
response = "y\r"                      # keystrokes sent to the agent on match

# Agent-specific custom rules — checked before global rules for that agent
[[agents.claude.rules]]
name     = "claude-custom"
pattern  = "Are you sure you want to"
response = "y\r"
```

### Rule Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | no | Label shown in status bar on match (default: `"custom"`) |
| `pattern` | yes | Go regular expression matched against the full visible screen text |
| `response` | yes | Keystrokes sent to the agent. Use `\r` for Enter, `\t` for Tab, etc. |

### Rule Chain Order

For a given agent, rules are evaluated in this priority order — first match wins:

1. Agent-specific custom rules (`agents.<name>.rules`)
2. Global custom rules (`rules`)
3. Built-in detector for the agent kind

---

## Session Memory

yoyo remembers every prompt it has approved within the current session (keyed by a SHA-256 hash of the prompt body). If the same prompt appears again, it is approved **immediately** regardless of the delay setting, and the status bar shows `seen: <rule>`. Memory is in-process only — it resets when yoyo exits.

---

## Exit Behavior

- yoyo exits when the child process exits.
- `SIGINT`, `SIGTERM`, `SIGHUP`, `SIGQUIT` restore the terminal and exit cleanly.
- The terminal is always restored even if yoyo crashes internally (panic recovery).
- On exit, yoyo prints a summary to stderr: `yoyo: 42 prompt(s) auto-approved`.

---

## Logging

yoyo writes a log file at `~/.yoyo/yoyo.log` (configurable via `-log` flag or `defaults.log_file` in config).

```
[INFO]  2026-04-02 14:32:15.123 started claude (kind=claude, delay=3s)
[INFO]  2026-04-02 14:32:20.456 prompt detected: Claude
[INFO]  2026-04-02 14:32:23.456 approval timer fired, sending response for: Claude
[ERROR] 2026-04-02 14:32:24.789 vt10x panic recovered: index out of range
```

Watch the log in real time:

```bash
tail -f ~/.yoyo/yoyo.log
```

---

## Troubleshooting

**My prompt isn't being detected**

1. Run with `-dry-run` to see if yoyo recognizes the prompt without sending approvals:
   ```bash
   yoyo -dry-run my-agent
   ```
   If the status bar shows the rule name, detection works — check the delay or enable setting.

2. Watch the log for detection events:
   ```bash
   tail -f ~/.yoyo/yoyo.log
   ```

3. For custom agents, add a `[[rules]]` entry with a regex that matches the prompt text visible on screen. Test your pattern against the visible text (not the raw ANSI output).

**My terminal looks broken after yoyo exits**

Run `reset` to restore terminal state. This can happen if yoyo is killed with `SIGKILL` (which cannot be caught).

**Status bar flickers or doesn't appear**

- Ensure your terminal supports ANSI escape sequences (most do).
- If the terminal is too narrow (< 24 columns), the status bar is hidden automatically.
- Resize events are tracked; the status bar repositions when the terminal is resized.

---

## Platform Support

| Platform | Status |
|----------|--------|
| Linux | Fully supported |
| macOS | Fully supported |
| Windows | Builds and runs; PTY resize is a no-op |

Requirements: Go 1.21+ to build from source.

---

## Security

### Prompt injection

yoyo auto-approves prompts that match its detector rules. A malicious program or file processed by the wrapped agent could deliberately emit output that looks like a permission prompt, causing yoyo to approve an action you did not intend.

**Mitigations:**

- Use the `-delay` option to give yourself time to review before approval is sent. The default is 3 seconds; press any key to cancel.
- Disable yoyo (`Ctrl+Y 0`) when the agent is about to process untrusted input.
- Keep custom `pattern` rules in your config as specific as possible — overly broad patterns increase the injection surface.

yoyo is designed for development workflows where you trust the agent and its environment. It is not designed to be safe in adversarial environments.

---

## License

MIT
