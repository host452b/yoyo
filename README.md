# yoyo

**you only yes once** — a PTY proxy that auto-approves AI agent permission prompts.

yoyo sits between your terminal and an AI agent CLI (Claude Code, Codex, Cursor, …).
It watches the agent's output, detects permission prompts, and automatically sends the
confirmation keystroke after a configurable delay — so you don't have to babysit it.

---

## Quick Start

```bash
# Install
go install github.com/host452b/yoyo/cmd/yoyo@latest

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
[yoyo: on 3s | Claude]  prompt detected — countdown started
[yoyo: on 0s | seen: X] already approved this session — sent immediately
[yoyo: off]             auto-approve disabled (manual mode)
```

- **Green** = auto-approve on
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

Run `yoyo -h` for the full built-in reference.

### Runtime Controls

The prefix key is **Ctrl+Y**. Press Ctrl+Y, then:

| Key | Action |
|-----|--------|
| `0` | Toggle auto-approve on/off |
| `1`–`5` | Set delay to N seconds (also re-enables if currently off) |

**Cancel pending approval:** press any non-escape key while the countdown is running.

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

---

## Building from Source

```bash
git clone https://github.com/host452b/yoyo.git
cd yoyo
go build ./cmd/yoyo
./yoyo -h
```

Requirements: Go 1.21+, Linux/macOS (Windows: no-op PTY resize).

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
