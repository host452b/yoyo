# Codex interaction matching

## Evidence and scope

Reference checkout: `codex/`, commit `ddea03ad049142943bdbf13e937b1d67e8c1ba0c`.
Yoyo baseline: `08c9aca`. The reference checkout is read-only input.

Codex separates approval requests (execution, network, stdin, patch, permissions,
MCP elicitation) from questions and other selection menus. See
`codex-rs/tui/src/bottom_pane/approval_overlay.rs` (`build_options`,
`exec_options`, `permissions_options`, `try_handle_shortcut`),
`list_selection_view.rs` (`handle_key_event`), and `popup_consts.rs`
(`accept_cancel_hint_line`). Approval labels and shortcuts depend on the
request and configured keymap. Enter accepts the highlighted row.

The Yoyo baseline accepts a header/footer pair without verifying options, misses
the network header, and always sends Enter. Its proxy caches a response until
the prompt hash changes, even though the hash deliberately excludes selection.

Byte-by-byte replay additionally exposed a screen-layer defect: vt10x's `Write`
creates a new rune reader per call, while `Screen.Feed` discarded incomplete
UTF-8 suffixes. Buffering at most three trailing bytes preserves selection
markers and borders for Codex, Claude and Cursor alike.

## Approach

Options considered:

1. Add more header strings: small change, but incomplete renders and wrong
   selected options still trigger incorrect responses.
2. Parse the current approval menu: retain the PTY architecture, require
   structural evidence, and derive the response from the visible options.
3. Use Codex app-server requests: provides typed request IDs and decisions, but
   requires a separate transport and session integration and does not improve
   existing Claude/Cursor PTY sessions by itself.

Implement option 2 within the existing `Detector` interface.

## Detection and response contract

- Require a recognized approval title, complete numbered options, exactly one
  selection marker, and a confirmation/cancellation footer at the end of the
  visible content. Title, option, and footer wrapping may span physical lines.
- Do not combine an older dialog's title with a newer menu across its footer.
- Recognize execution, network, stdin, patch, permission, and MCP approval
  titles. General questions, onboarding, prose and incomplete menus abstain.
- Select a known single-request affirmative label. Do not silently substitute a
  session/persistent grant when the single-request option is unavailable.
- Enter is valid only when that option is highlighted and the footer advertises
  Enter as confirmation, with no visible Enter cancellation/rejection binding.
  Otherwise use its displayed plain letter shortcut.
  Unsupported or conflicting shortcuts abstain; never append Enter to a
  shortcut that already submits the decision.
- Preserve command text in the hash. Remove selection markers only from option
  rows, so moving the highlight does not create a new permission identity.
  Wrapped command text may conservatively produce a different hash.
- Before a delayed send, rerun detection on the current screen. Refresh cached
  responses even for the same hash. Cancel obsolete pending timers when the
  prompt disappears or another path handles it. Preserve existing countdown
  behavior for continuously rendered dialogs with noisy body hashes.
- A confirm footer must include its cancellation binding before detection;
  accept-only footers abstain because they are indistinguishable from the first
  part of a normal fragmented footer. Cancel-only footers can use an explicit
  approval shortcut.
- Keep optional visible prompt-body evidence after approval. A transient
  footer miss must not release send suppression while that body remains visible.
  Refresh the evidence on matching redraws, and release it when the body is gone.

## Validation

Use small, attributed text fixtures extracted from the reference snapshots;
tests must run without the untracked Codex checkout. Cover valid approval kinds,
negative questions/prose/partial frames, selection changes, unsupported keymaps,
trailing stale content, wrapped lines and VT100 chunked UTF-8/ANSI input. Add
proxy regressions for selection changes during a countdown and transitions away
from an active prompt. Run focused tests, full Go tests, race checks and vet.

No release or remote publication is included. Snapshot replay validates local
rendered-input behavior, not all terminal configurations or future Codex builds.
Plain-text detection cannot distinguish a perfectly reproduced live-looking
menu from an identical transcript at the bottom of the screen. It also cannot
recover request IDs or hidden key bindings. An eventual Codex-specific
app-server integration could use `item/commandExecution/requestApproval`,
`item/fileChange/requestApproval` and `item/permissions/requestApproval`
separately from `item/tool/requestUserInput`; these are defined in
`codex-rs/app-server-protocol/src/protocol/common.rs` at the reference commit.

## Implementation sequence

1. Add source-derived fixtures and failing detector regressions; correct older
   synthetic tests that incorrectly treat missing options as a complete menu.
2. Implement structural Codex parsing and selection-aware response generation.
3. Add failing proxy lifecycle regressions, then refresh/revalidate responses.
4. Update supported-agent documentation and record validation results.

## Verification results (2026-09-10)

- Confirmed failing regressions before the relevant fixes: network detection,
  incomplete/unrelated menus, stale title reuse, incorrect selected-option
  responses, UTF-8 split at byte 13, and an obsolete timer sending twice.
- Final `go test -json -count=1 ./...`: all 11 packages passed; 383 test/subtest
  pass events. Three existing tmux tests initially skipped because macOS's
  default temporary path exceeded the Unix socket length limit. All four
  `TestTmux_ClaudeDetector_*` tests passed with `TMPDIR=/private/tmp`.
- The existing live tmux approval test passed outside the sandbox; its empty
  pane failure was reproduced twice inside the sandbox before that comparison.
- Race-enabled detector, screen, and affected proxy regression tests passed.
- Codex fuzzing completed 498,596 executions without a panic. This is a short
  robustness check, not a measurement of matching accuracy.
- `go vet ./...`, `go build`, and `git diff --check` passed. Build verification
  was repeated outside the sandbox to permit Go's module-version cache write.
- Seven source-derived approval fixtures passed at 60, 80 and 120 columns with
  one-byte input chunks. No live Codex model session or Linux/Windows runtime
  validation was performed.

Local validation logs: `/private/tmp/yoyo-validation-final.jsonl`,
`/private/tmp/yoyo-validation-tmux.jsonl`, and
`/private/tmp/yoyo-validation-race.log`. These are temporary local artifacts.

## Follow-up: controlled baseline comparison

The user requested execution of the before/after comparison and additional
tests. No original failure dump was found in `~/.yoyo/dumps` (absent), the current
workspace, or the earlier `~/Desktop/yoyo` checkout. The existing log has no dump
entries. Therefore no production incident recovery percentage is reported.

[`scripts/compare_matching.py`](../../../scripts/compare_matching.py) extracts
baseline `08c9acabc8cb2519cb18d8b9ed569d90a4ece9e4` into a temporary directory,
snapshots the current implementation separately, and runs the same current tests
and fixtures against both. Neither the working tree nor Git refs are reset.
Both result sets must contain identical leaf test cases. Parent summaries are
excluded; widths and chunk sizes remain separate samples.

```sh
python3 scripts/compare_matching.py --baseline 08c9aca --output /tmp/yoyo-comparison
```

The final curated corpus contains **93 cases**: baseline **51 pass / 42 fail**;
candidate **93 pass / 0 fail**. All 42 baseline failures recover, with zero
regressions within this corpus. These are sample counts, not 42 independent
root causes or a production accuracy claim.

| Origin | Cases | Recovered | Preserved passes |
|---|---:|---:|---:|
| Upstream snapshots, including width variants | 21 | 3 | 18 |
| Existing Claude historical regressions | 2 | 0 | 2 |
| Existing Cursor regressions | 8 | 0 | 8 |
| Constructed behavior and stream regressions | 62 | 39 | 23 |

Detailed evidence and the candidate content fingerprint are in the
[comparison table](../../testing/2026-09-10-matching-comparison.md) and
[machine-readable results](../../testing/2026-09-10-matching-comparison.json).

Additional tests found and reproduced Enter bindings overriding confirmation,
multiple sends during footer fragmentation, and stale visibility evidence after
selection/redraw. The fixes now retain suppression through footer changes but
allow a genuinely disappeared request to reappear. The command hash test was
also corrected to change only the marker, preserving surrounding spaces.

Exact visible-body retention is deliberately conservative. If an identical old
menu remains visible while an indistinguishable new request appears, text-only
matching may suppress the new request. Typed request IDs remain the route to
removing that ambiguity.

Final follow-up validation: `TMPDIR=/private/tmp go test -json -count=1 ./...`
passed all 11 packages with 417 test/subtest pass events, zero failures and zero
skips, including every tmux test. An earlier run hit the existing tmux shell
startup 2-second wait; the final run passed it without changing that test.
Race checks passed for the affected detector/screen/proxy paths, with the last
footer-redraw fix checked again under `-race`. `go vet ./...`, Python script
syntax validation, and `git diff --check` passed. The candidate fingerprint was
recomputed from the working tree and matched the comparison report.
Raw final full-suite log: `/private/tmp/yoyo-matching-full-complete.jsonl`.
