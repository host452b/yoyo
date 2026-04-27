# Changelog

All notable changes to yoyo are documented here.
Format based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.4.0] — 2026-04-27

### Added

- **Maintainer-ready detector diagnostics in `Ctrl+Y d` dumps.** Diagnostic
  dumps now include every active detector/rule's match result, fuzzy fallback
  result, safety-guard result, matched response shape, prompt hash, and a
  line-numbered repro copy of the exact vt10x-rendered screen yoyo evaluated.
  This turns "Codex/Claude/Cursor did not auto-approve here" reports into
  copy-pasteable regression-test cases for maintainers.

### Fixed

- Runtime flags in dumps now reflect the current proxy state at the moment
  `Ctrl+Y d` is pressed, including runtime toggles such as `Ctrl+Y 0`,
  `Ctrl+Y a`, `Ctrl+Y f`, and delay changes via `Ctrl+Y 1`–`5`.

## [2.3.0] — 2026-04-23

### Added

- **Diagnostic dumps via `Ctrl+Y  d`.** Freezes everything yoyo knew
  about its state at that moment into a timestamped Markdown file
  under `~/.yoyo/dumps/yoyo-<ts>.md`: yoyo version, Go runtime,
  OS/arch, TERM, tmux flag, runtime flags (delay, afk/fuzzy/safety,
  dry-run, approval count), agent command/kind/PID/PTY geometry,
  current vt10x-rendered screen, config content, last 100 log lines,
  filtered environment. Keys with secret-looking names (TOKEN,
  PASSWORD, KEY, SECRET, API_*, CREDENTIAL, AUTH, BEARER,
  SESSION_ID) have their values replaced with `<redacted>`;
  `response = "…"` config lines are redacted uniformly. The screen
  content is deliberately NOT redacted — that's the entire point.
  Status bar briefly shows `dumped: <path>`.
  - New package `internal/dump`, 4 unit tests for redaction + file
    shape.
  - Two new proxy E2E tests: `TestProxy_E2E_CtrlYD_DumpCallback`
    (callback fires exactly once per keypress) and
    `TestProxy_E2E_CtrlYD_DumpErrorIsNonFatal` (error path keeps
    proxy running).

## [2.2.5] — 2026-04-23

### Fixed

- **Safety guard no longer false-positives on stale scrollback**.
  `ContainsDangerousCommand` previously scanned the full visible
  screen, so a destructive command from earlier in the session that
  the user had manually approved (e.g. `kubectl delete ns staging`)
  would keep blocking every subsequent auto-approval until it
  scrolled off-screen — manifesting as "yoyo silently stopped
  auto-approving for no visible reason." The scan window is now
  clamped to the trailing 20 lines (the active prompt area), matching
  the fuzzy detector's last-15-lines policy. A new
  `ContainsDangerousCommandFull` entry point preserves the
  whole-input behaviour for callers that genuinely need it (tests,
  future uses).

  Added two regression tests in
  `internal/detector/danger_test.go`:
  - `TestContainsDangerousCommand_IgnoresStaleScrollback` — 30 lines
    of benign output between a past `kubectl delete` and the current
    prompt; must not block.
  - `TestContainsDangerousCommand_DangerInTailStillCaught` —
    destructive command inside the scan window must still match.

## [2.2.4] — 2026-04-22

### Added

- **PyPI distribution** — `pip install yoyo-cli` now installs
  pre-built Go binaries via per-platform wheels (linux-amd64,
  linux-arm64, macos-amd64, macos-arm64). Wheels contain only the
  compiled Go binary — no Python runtime dependency. The installed
  command is still just `yoyo`; the PyPI package name is `yoyo-cli`
  because the bare `yoyo` name was already taken on PyPI by an
  unrelated placeholder.
  Build: `python3 python/build_wheels.py vX.Y.Z`. Extended
  `scripts/release.sh` to build wheels automatically; set
  `UPLOAD_PYPI=1` to also upload via `twine`. See
  `python/build_wheels.py` for details.

## [2.2.3] — 2026-04-22

### Fixed

- **yoyo hung after short-lived child exited** (e.g.
  `yoyo cat nonexistent-file`). The parent process kept its own copy
  of the PTY slave fd open, so the master read never received EOF
  when the child exited. Ctrl-C, Ctrl+Y q, and 3× Ctrl-C were all
  no-ops in this state because the child was already dead. Fix:
  close the parent's slave copy immediately after `cmd.Start()` so
  the master sees EOF when the child (the last slave writer) exits.

### Build

- `scripts/release.sh` + `make release TAG=v2.X.Y`: one-command
  manual release — cross-compiles all four platforms, generates
  checksums, extracts release notes from CHANGELOG, and publishes
  via `gh release create`. Replaces the removed GitHub Actions
  workflow.

## [2.2.2] — 2026-04-22

### Fixed

- **3× Ctrl-C window widened from 500ms → 1s**. 500ms was too tight
  for real frustrated-Ctrl-C cadence (~600ms). The sliding-window
  test still protects against accidental triggers from occasional
  solo Ctrl-C presses.

### Build

- Release binaries are built with `-trimpath -ldflags "-s -w -X
  main.version=<tag>"` so `yoyo -v` reports the actual tag instead
  of `dev`, and the binary is reproducible and stripped.

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
