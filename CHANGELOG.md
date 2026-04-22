# Changelog

All notable changes to yoyo are documented here.
Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.2.2] — 2026-04-22

### Build / CI

- Add GitHub Actions **release** workflow (`.github/workflows/release.yml`).
  On any `v*` tag push, cross-compiles binaries for linux/amd64,
  linux/arm64, darwin/amd64, darwin/arm64, generates a
  `checksums.txt`, extracts release notes from the matching
  CHANGELOG section, and publishes everything (including `install.sh`)
  to the GitHub Release. Also supports `workflow_dispatch` for
  manually re-releasing an existing tag.
- Add GitHub Actions **CI** workflow (`.github/workflows/ci.yml`):
  `go vet`, `go test ./... -race`, and `govulncheck` on every push to
  main and every pull request.
- Release binaries are now built with `-trimpath -s -w -X
  main.version=<tag>` so `yoyo -v` reports the actual tag instead of
  `dev`, and the binary is reproducible and stripped.

### Fixed

- **3× Ctrl-C window widened from 500ms → 1s**. 500ms was too tight
  for real frustrated-Ctrl-C cadence (~600ms). The sliding-window
  test still protects against accidental triggers from occasional
  solo Ctrl-C presses.

## [2.2.1] — 2026-04-22

### Added

- **Force-kill escape hatch** for wedged agents. When the agent's TUI
  jams its own Ctrl-C handling (e.g. Claude Code showing "Press Ctrl-C
  again to exit" but not responding to subsequent presses), yoyo now
  offers two ways to kill the child from outside the byte stream:
  - `Ctrl+Y q` — deliberate keyboard command.
  - **3× Ctrl-C within 500 ms** — muscle-memory path for users who
    just bang on Ctrl-C.

  Both call `cmd.Process.Kill()` (SIGKILL) directly; yoyo exits
  cleanly afterward. The sliding-window detector resets on any
  non-Ctrl-C byte, so occasional Ctrl-C presses spaced more than 500
  ms apart don't accidentally terminate the agent.

### Docs

- Supported Agents section now calls out that Cursor CLI also ships
  as `agent`. Command-based detection stays on `cursor` /
  `cursor-agent` only (too much collision risk with `ssh-agent` et
  al.); bare `agent` is auto-identified from Cursor's banner text in
  the first 10 output frames.

## [2.2.0] — 2026-04-22

### Added

- **Deletion-command safety guard** (`-no-safety` to opt out). yoyo
  refuses to auto-approve when the visible screen contains a
  deletion-class command: `rm -rf` (top-level/glob), `git rm -r`, `git
  clean -f…`, `find … -delete` / `-exec rm`, SQL `DROP` / `TRUNCATE` /
  naked `DELETE FROM`, `kubectl delete`, `terraform destroy`, and
  `docker`/`podman volume rm` / `system prune`. Scope is deliberately
  narrow (no mkfs/dd/chmod/curl|sh heuristics) to stay
  container-dev-friendly. Status bar shows `danger: <snippet>` when
  the guard trips; user can still approve manually. The AFK fire
  path is guarded too — it's the highest-risk approval path.
- **Config file permission warning**. `yoyo` prints a stderr warning
  when `config.toml` is group- or world-writable, because a writable
  config is a privilege-escalation vector (an attacker can inject
  `[[rules]]` with `pattern=".*" response="y\r"`). Advisory only —
  the config still loads.

### Testing

- **Fuzz targets** for every detector (Claude/Codex/Cursor + FuzzyMatch)
  and `Screen.Feed`, using Go's native `testing.F`. Collectively
  10M+ random inputs survive a 40-second fuzz pass with zero panics.
- **Race-detector clean** across the full suite. Fixed a data race in
  the AFK reset tests (sender goroutine vs. channel close).
- **cmd/yoyo coverage** raised from 14.8% → 33.3% by extracting
  flag-resolution priority logic into a testable
  `resolveEffective()` function with a 9-test priority matrix.
- **Fuzzy defers to specific detectors** (bug fix): previously a
  screen that matched both Claude and fuzzy could produce two
  approval writes due to distinct hash domains. Fuzzy now re-runs
  `chain.Detect` at fire time and yields on match.
- New **rigorous edge tests**: AFK toggle-off cancels pending fire,
  fuzzy stability resets on content change, fuzzy respects `-delay`,
  specific detector wins over fuzzy, FuzzyMatch ignores trailing
  blank padding from vt10x.

### Build

- **Makefile** with `test` / `test-race` / `test-cover` /
  `fuzz-quick` / `fuzz-long` / `build` / `install` / `tidy` targets.

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
