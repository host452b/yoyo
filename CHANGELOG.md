# Changelog

All notable changes to yoyo are documented here.
Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.1.0] — 2026-04-22

Two opt-in fallback layers for prompts the built-in detectors miss, plus a Cursor
detector fix for the newer "command-box + question-below" layout.

### Added

- **AFK mode** (`-afk`, `-afk-idle 10m`). After the configured idle window
  (default 10 minutes) with no PTY output *and* no user input, yoyo injects
  `y` + Enter, pauses 200 ms, then sends
  `continue, Choose based on your project understanding.` + Enter, and rearms.
  The loop runs forever until toggled off. Status bar shows an
  `afk mm:ss` countdown and flashes `afk nudged` for 2 s after each fire.
- **Fuzzy fallback** (`-fuzzy`, `-fuzzy-stable 3s`). A generic y/n detector
  that fires when (1) the screen has been stable for the configured window
  and (2) the last 15 lines contain a precise vocabulary marker
  (`(y/n)`, `[Y/n]`, `y/n?`, `yes/no`, any casing; bare `Yes`/`No`/`enter`/
  `confirm` intentionally excluded). Routes through the normal approval
  flow, respects `-delay` and memory-based dedup.
- **Runtime toggles**: `Ctrl+Y a` toggles AFK on/off; `Ctrl+Y f` toggles
  fuzzy on/off. Both are pure toggles — same keystroke both activates and
  deactivates based on current state; works regardless of startup flags.
- **Per-agent config overrides** for both features:
  ```toml
  [agents.claude]
  afk          = false
  afk_idle     = "5m"
  fuzzy        = true
  fuzzy_stable = "2s"
  ```
- 18 atomic commits, 30+ new tests covering defaults, explicit values,
  per-agent overrides, timer arm/fire/rearm, reset on output and input,
  dry-run suppression, runtime toggle, deadline tracking, vocabulary
  hits/misses, stability resets mid-window, delay interaction, and
  specific-detector precedence.

### Fixed

- **Cursor detector** now matches the newer layout where the command sits
  inside a `┌──┐ ... └──┘` box and the approval question / `(y)` / `n)`
  options render outside (below) the box. Previously only content inside
  the box was inspected, so yoyo stopped auto-approving for that layout.
- **Fuzzy defers to specific detectors**: when a specific detector
  currently matches a screen, fuzzy no longer double-fires on stability
  expiry. Fixes a regression where a single screen could produce two
  approval writes.

### Design docs

- `docs/superpowers/specs/2026-04-22-afk-mode-design.md`
- `docs/superpowers/plans/2026-04-22-afk-mode.md`

## [2.0.7] — 2026-03-27

### Fixed

- **tmux compatibility**: strip `TMUX` / `TMUX_PANE` from the child's
  environment and force `TERM=xterm-256color` so the child doesn't think
  it is running inside tmux, which caused rendering issues when yoyo
  itself was invoked from a tmux pane.

## [2.0.5] — 2026-03-25

### Added

- **Approval debounce** (`approvedHash` tracking): yoyo no longer re-sends
  `\r` while the same prompt is still visible on screen.
- **Countdown display** in the status bar during the approval delay.
- **Dry-run mode** (`-dry-run`): detect prompts and log the intent without
  writing approvals to the child PTY.
- Misc UX improvements around status-bar rendering and prompt detection.

## [2.0.2] — 2026-03-25

### Added

- 14 E2E scenario tests for the proxy event loop using fake PTY and stdin
  (immediate approval for Claude / Codex / Cursor, delayed approval,
  cancellation on keypress, memory replay, dry-run, prefix key state).

## [2.0.1] — 2026-03-25

### Fixed

- Stop pending timers (`approvalTimer`, `prefixTimer`) on both
  `inputCh`-closed and `outputCh`-closed paths to prevent resource leaks
  when the child exits or stdin closes.

<!-- Earlier releases predate this changelog. -->
