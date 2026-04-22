# AFK Mode — Design

Status: draft
Date: 2026-04-22

## Purpose

Some AI agent CLIs display custom y/n or confirmation prompts that yoyo's
built-in detectors do not match. In those cases the agent blocks on `read()`
forever, waiting for the user, and the session stalls. AFK mode is a fallback
that proactively unblocks the agent after an extended period of silence.

The model is deliberately dumb: no pattern matching, no screen analysis. It
only watches whether the terminal has seen *any* activity in the past N
minutes, and if not, injects a generic "just pick something and continue"
nudge into the child. The expectation is that AFK recovers from unmatched
prompts at the cost of occasionally nudging an agent that did not strictly
need it.

## Behaviour

### Trigger

- An idle timer is armed on session start (if AFK is enabled) and fires when
  `-afk-idle` passes without any activity.
- "Activity" is either (a) a byte arriving on the child PTY → yoyo output
  path, or (b) a byte arriving on yoyo's stdin (user input).
- Any activity resets the timer. This mirrors macOS display-sleep semantics:
  either keyboard input or machine output keeps the session awake.
- yoyo's own writes to the child PTY (approval keystrokes, AFK nudges) do
  not directly reset the timer. The child's response to those writes will
  appear on outputCh and reset it naturally.

### Fire

When the timer fires, yoyo writes two payloads to the child PTY, in order:

1. `y\r`
2. (200 ms gap)
3. `continue, Choose based on your project understanding.\r`

The 200 ms gap gives the child's TUI time to process the `y` (often a
single-keystroke y/n submit) before the follow-up message arrives as a fresh
input line.

`-dry-run` mode logs the intent but does not write either payload, matching
the existing approval dry-run contract.

### Rearm and loop

Immediately after firing, the idle timer is reset and starts counting down
again. AFK loops forever. If the agent remains unresponsive for another
`-afk-idle` period, the nudge fires again.

**The loop only stops when the user toggles AFK off via `Ctrl+Y a`.** It
does not stop on its own after N fires, does not back off, and does not
self-disable if the agent appears unresponsive to the nudge. This is
intentional: the user explicitly opted into AFK, and any automatic
"give-up" logic would re-introduce the original stuck-session failure mode.

### Runtime toggle

- `Ctrl+Y a` toggles AFK on/off.
- When toggled off, any pending idle timer is stopped.
- When toggled on, the idle timer is armed fresh (as if activity just
  occurred).
- The `-afk-idle` value is not adjustable at runtime in v1; it is fixed at
  the startup value.

### Status bar

The AFK segment appears at the end of the existing status bar, after the
approval rule slot.

| State | Rendering |
|---|---|
| AFK off | (no segment) |
| AFK on, counting down | `afk 9:58` — always `mm:ss` of time remaining, updated each output frame |
| AFK just fired | `afk nudged` for 2 s, then returns to `mm:ss` countdown |

## Configuration

### CLI flags

```
-afk                 enable AFK mode (default: off)
-afk-idle duration   idle threshold before nudging (default: 10m)
```

`-afk-idle` accepts standard Go duration strings (`30m`, `1h`, `90s`).

### Config file

```toml
[defaults]
afk      = true
afk_idle = "10m"

[agents.claude]
afk      = false     # per-agent override, same mechanism as `delay`
```

CLI flags take priority over config, matching existing `-delay` behaviour.

## Interactions

- **Auto-approve**: AFK is independent. A detected prompt goes through the
  normal approval flow; AFK only fires when yoyo's detectors never matched
  and no output has arrived.
- **Auto-approve disabled** (`Ctrl+Y 0`): AFK can still fire. They are
  orthogonal toggles.
- **Approval countdown running**: if output is still arriving from the
  child, the AFK timer is being reset constantly, so it will not fire while
  a live prompt is being processed.
- **Dry-run**: AFK suppresses its PTY writes and logs
  `afk: would send y + continue`.

## Code layout

- `internal/config/config.go`
  - Add `Afk *bool` and `AfkIdle *Duration` to `Defaults` and per-agent
    `AgentConfig` (pointer types so absence means "inherit", same pattern as
    existing `Delay`).
- `cmd/yoyo/main.go`
  - Add `-afk` bool flag and `-afk-idle` duration flag.
  - Resolve effective values (flag → per-agent config → defaults) and pass
    into `proxy.Config`.
- `internal/proxy/proxy.go`
  - `Config` gains `AfkEnabled bool` and `AfkIdle time.Duration`.
  - `Run` loop gains `afkTimer *time.Timer` and `afkTimerCh <-chan time.Time`.
  - Reset afk timer in both the inputCh case and the outputCh case.
  - New `case <-afkTimerCh:` performs the two-phase write (y, sleep, continue).
  - `handlePrefix` switch gains `case 'a'` → toggle AFK, stop/rearm timer,
    update status bar.
- `internal/statusbar/statusbar.go`
  - New `SetAfk(enabled bool, remainingSecs int, nudgedFlash bool)`.
  - Render AFK segment after the rule slot.

## Tests

- `internal/proxy/proxy_e2e_test.go`
  - `TestProxy_E2E_AfkFires`: set `AfkIdle = 300ms`, no output, verify both
    `y\r` and the continue message are written to fakePTY in order.
  - `TestProxy_E2E_AfkRearmsAndFiresTwice`: after first fire, stay idle,
    verify a second fire occurs after another `AfkIdle`.
  - `TestProxy_E2E_AfkResetOnOutput`: send PTY output every 100 ms, verify
    no fire within 500 ms when `AfkIdle = 300ms`.
  - `TestProxy_E2E_AfkResetOnUserInput`: send stdin every 100 ms, verify no
    fire within 500 ms.
  - `TestProxy_E2E_AfkDryRun`: dry-run on, verify nothing written to
    fakePTY, but log contains the dry-run message.
  - `TestProxy_E2E_AfkToggleViaPrefix`: send `Ctrl+Y a`, verify no fire
    after idle; send again, verify fire resumes.
- `internal/config/config_test.go`: round-trip `afk` and `afk_idle` fields
  from TOML, including per-agent overrides.

## Non-goals (v1)

- Adjusting the idle duration at runtime (would require a two-level prefix
  state machine, low user value).
- Content-aware nudging (checking the screen text before firing).
- Customisable nudge payload via config (users who need different behaviour
  can already author custom rules).
- Auto-disable after N consecutive fires / exponential backoff.
