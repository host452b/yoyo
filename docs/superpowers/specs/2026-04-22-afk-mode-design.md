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

`Ctrl+Y a` is a pure toggle — the same keystroke both activates and
deactivates AFK depending on its current state:

- If AFK is currently **off** → `Ctrl+Y a` **activates** it; the idle timer
  is armed fresh (as if activity just occurred) using the current
  `-afk-idle` value.
- If AFK is currently **on** → `Ctrl+Y a` **deactivates** it; any pending
  idle timer is stopped and no further nudges will fire.

The initial on/off state at startup is determined by the `-afk` flag (or
the config file), but the toggle key works from either starting state. A
user who did not pass `-afk` can still activate AFK at runtime by pressing
`Ctrl+Y a`; a user who did pass `-afk` can likewise deactivate it with the
same keystroke.

The `-afk-idle` duration is not adjustable at runtime in v1; it is fixed at
the startup value and applied whenever AFK is toggled on.

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

---

# Generic Fuzzy Fallback (`-fuzzy`)

A second, complementary fallback that sits in the same spec because it
addresses the same core failure — "detector didn't match but the agent is
clearly waiting for y/n input." AFK waits 10 minutes then pokes blindly;
`-fuzzy` tries to detect "likely y/n prompt" within a few seconds so the
user doesn't have to wait.

The two are orthogonal and can be enabled together: fuzzy fires faster on
recognisable y/n prompts; AFK catches everything else AFK-idle duration
later.

## Purpose

Permissively recognise a y/n prompt via two signals:

1. The visible screen text has been **stable** (unchanged) for a short
   window — evidence that the agent is waiting, not still rendering.
2. The screen contains at least one **precise** y/n prompt marker from a
   narrow vocabulary — not generic English words that could appear in
   logs or code.

Both signals required → synthesise a `MatchResult{RuleName: "fuzzy",
Response: "\r"}` and route through the normal approval flow (respects
`-delay`, `memory.Seen` dedup, user-cancel on keypress).

## Vocabulary

Matched via a single case-insensitive regexp against the concatenation of
the last 15 lines of `screen.Text()`:

```
\([yY]/[nN]\)       ( y/n ) — any casing
\([nN]/[yY]\)       ( n/y ) — any casing
\[[yY]/[nN]\]       [ y/n ]
\[[nN]/[yY]\]       [ n/y ]
[yY]/[nN]\?         y/n?
[yY]es/[nN]o        yes/no
```

**Deliberately excluded:** bare `Yes`, `No`, `Enter`, `confirm`,
`continue`, `(y)` alone (Cursor detector handles that one), word
`approve`. They appear too often in normal text and logs.

## Stability tracking

A one-shot timer is armed whenever the hash of `screen.Text()` changes:

- Every `outputCh` frame: compute `h = sha256(screen.Text())`. If `h` !=
  `fuzzyLastHash`, update `fuzzyLastHash = h`; stop any pending stability
  timer; start a fresh timer for `-fuzzy-stable` (default 3 s).
- When the stability timer fires: re-read the current text, run the
  vocabulary regex. If matched, synthesise a `MatchResult` and hand it to
  the same approval code path that handles regular detector matches
  (`memory.Seen` → immediate; otherwise start `-delay` countdown).

The stability timer is a separate `*time.Timer` from the approval timer
and the afk idle timer. They coexist in the `select`.

## Configuration

### CLI flags

```
-fuzzy                  enable generic fuzzy fallback (default: off)
-fuzzy-stable duration  screen-stability window before attempting
                        vocabulary match (default: 3s)
```

### Config file

```toml
[defaults]
fuzzy        = true
fuzzy_stable = "3s"

[agents.cursor]
fuzzy = false     # per-agent override
```

## Runtime toggle

`Ctrl+Y f` toggles fuzzy on/off. Same pure-toggle semantics as
`Ctrl+Y a`: works from either starting state, flips current state,
stops any pending stability timer when turning off.

## Status bar

No dedicated segment. When fuzzy matches, the rule slot shows `fuzzy` —
same mechanism the existing detectors use (`[yoyo: on 3s | fuzzy]`).
During the `-delay` countdown after a fuzzy match the label becomes
`[yoyo: on 2s | fuzzy]`, identical to the behaviour of Claude/Codex/Cursor
matches.

## Interactions

- **Chain order**: fuzzy runs **after** all specific detectors (custom
  rules → agent-specific → fuzzy). Specific detectors always win.
- **Auto-approve disabled** (`Ctrl+Y 0`): fuzzy is suppressed along with
  everything else, same as the other detectors.
- **Dry-run**: a fuzzy match logs `fuzzy: would approve` and does not
  send `\r`. Matches the existing dry-run contract.
- **AFK interaction**: fuzzy's `\r` write produces output from the agent,
  which resets the afk idle timer. AFK therefore never fires on a prompt
  that fuzzy successfully handles.

## Code layout (addition)

- `internal/detector/fuzzy.go` (new): a plain function
  `FuzzyMatch(text string) bool` that applies the regexp to the last 15
  lines. No state; no `Detector` interface implementation. Fuzzy is *not*
  a `RuleChain` member — its stability timing lives in `proxy.Run`.
- `internal/detector/fuzzy_test.go` (new): vocabulary positive/negative
  cases, boundary-line counting, case-insensitivity.
- `internal/config/config.go`: `Fuzzy bool`, `FuzzyStable time.Duration`
  (plus raw `Duration`) on `Defaults`; `Fuzzy *bool`, `FuzzyStable
  *time.Duration` on `AgentConfig` — same pattern as the AFK fields.
- `cmd/yoyo/main.go`: `-fuzzy` and `-fuzzy-stable` flags; resolve
  effective values; pass `FuzzyEnabled` / `FuzzyStable` into
  `proxy.Config`.
- `internal/proxy/proxy.go`:
  - `Config` gains `FuzzyEnabled bool`, `FuzzyStable time.Duration`.
  - In `Run`: `fuzzyEnabled`, `fuzzyLastHash`, `fuzzyStableTimer`,
    `fuzzyStableTimerCh` locals.
  - Every `outputCh` frame: recompute `screen.Text()` hash, reset timer
    on change.
  - New `case <-fuzzyStableTimerCh:` re-reads text, runs `FuzzyMatch`,
    synthesises `MatchResult`, funnels through the same approval handler
    as regular detector hits.
  - `handlePrefix` gets a `'f'` case mirroring `'a'`.
- `internal/proxy/proxy_e2e_test.go`: new tests for fuzzy (fires on
  stable y/n screen, does not fire without vocab match, respects
  `-delay`, `Ctrl+Y f` toggles, dry-run suppresses).

## Non-goals (fuzzy v1)

- User-customisable vocabulary via config (custom rules already cover
  this need with more precision).
- Multi-response inference (always `\r`, never `y\r` or `n\r`).
- Per-line scoring / confidence threshold (one bit: hit or miss).
- Remembering "this screen looked like a prompt once, auto-approve next
  time without re-matching" — `memory.Seen` already handles re-approval
  once we've approved once.
