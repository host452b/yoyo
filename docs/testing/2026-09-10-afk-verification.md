# AFK continuation verification — 2026-09-10

Implemented explicit continuation-question and composer checks, atomic terminal
text/cursor snapshots, bracketed paste followed by verified separate submission,
and per-session attempt deduplication. AFK stays off by default with a 10-minute
idle threshold. Unknown or ambiguous states abstain.

## Results

- Full `go test -count=1 -json ./...`: **502 test/subtest pass events across
  11 packages**, no failures or skips on the final run (including tmux tests).
- AFK/screen race run: **114 test/subtest pass events across 3 packages**,
  no failures, skips, or race reports; includes **86 AFK leaf scenarios**.
- `go vet ./...` and `git diff --check`: passed.
- The first full run had 2 tmux startup/render timeouts, with 500 pass events.
  Both failed cases passed individually without code changes, then the complete
  suite passed on a second run. The initial timeout cause is not established.

## Regression evidence

Before the fix, the new paste/echo and no-repeat tests observed repeated blind
`y` and continuation writes. A separate test demonstrated Fuzzy pressing Enter
before AFK draft verification; another demonstrated changed question identity
when context wrapped. All now pass.

Coverage includes both agent layouts, English/Chinese continuation questions,
empty and nonempty composers, visible cursor location, unknown footers, busy
screens, approvals, business choices, completed answers, quoted/code examples,
late/missing echo, changed questions, edits/navigation, AFK-off, deletion guards,
failed/partial writes, question-context changes and Fuzzy coordination.

## Commands and evidence boundary

```sh
TMPDIR=/private/tmp GOCACHE=/private/tmp/yoyo-go-cache go test -count=1 -json ./...
GOCACHE=/private/tmp/yoyo-go-cache go test -race -count=1 -json ./internal/agent ./internal/screen ./internal/proxy -run 'TestAFK|TestProxy_.*Afk|TestScreen_'
GOCACHE=/private/tmp/yoyo-go-cache go vet ./...
git diff --check
```

AFK tests use simulated PTYs and real VT100 screen rendering. Codex's composer
placeholder/footer are grounded in the local upstream snapshot; Claude idle
layouts are constructed. Existing tmux tests exercise terminal integration,
not a complete AFK conversation with live Codex or Claude. Installed-CLI
canaries, custom keymaps/status lines and other layouts remain unverified.

The earlier matching-comparison report is a historical snapshot from before
these AFK changes; its candidate fingerprint does not identify this worktree.

## v2.6.0 push verification

The full race run repeated a tmux size-check timeout while using the default
personal shell. The test harness now uses an isolated tmux configuration and
plain `/bin/sh`, with shell startup environment hooks removed. Both previously
unstable cases passed three consecutive race runs. The final full
`go test -race -count=1 -json ./...` passed: **502 test/subtest pass events,
11 packages, no failures, skips, or race reports**. `go vet ./...` also passed.
Four linux/darwin amd64/arm64 release-build checks succeeded, and the native
binary reported `yoyo v2.6.0`. These were build checks; artifacts were not published.
