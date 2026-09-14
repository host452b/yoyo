# Codex approval fixtures

Visible rows extracted from local Codex commit `ddea03ad049142943bdbf13e937b1d67e8c1ba0c`.
Upstream: https://github.com/openai/codex (Apache-2.0). Only right padding and snapshot metadata were removed.
Tests do not require the Codex checkout.

Retry dialogs under `retry/` cover the two user-reported templates (`prompt.txt`
and `confirmation.txt`, with GPT-5.6-Luna) and the upstream narrow confirmation
snapshot (`confirmation-narrow.txt`). The latter comes from
`chatwidget/snapshots/codex_tui__chatwidget__tests__safety_buffering_retry_confirmation_narrow.snap`
at the same commit. Rendering and numeric shortcut behavior were checked in
`chatwidget/safety_buffering.rs` and `bottom_pane/list_selection_view.rs`.
Both dialogs select option 1: "Retry with a faster model" followed by "Keep waiting".

The upstream Apache-2.0 license is included in [LICENSE](LICENSE).

| Fixture | Upstream source under `codex-rs/tui/src/` |
|---|---|
| `network.txt` | `bottom_pane/snapshots/codex_tui__bottom_pane__approval_overlay__tests__network_exec_prompt.snap` |
| `permissions.txt` | `bottom_pane/snapshots/codex_tui__bottom_pane__approval_overlay__tests__approval_overlay_permissions_prompt.snap` |
| `cross-thread.txt` | `bottom_pane/snapshots/codex_tui__bottom_pane__approval_overlay__tests__approval_overlay_cross_thread_prompt.snap` |
| `additional-permissions.txt` | `bottom_pane/snapshots/codex_tui__bottom_pane__approval_overlay__tests__approval_overlay_additional_permissions_prompt.snap` |
| `patch.txt` | `bottom_pane/snapshots/codex_tui__bottom_pane__approval_overlay__tests__approval_overlay_patch_destination_unavailable.snap` |
| `exec.txt` | `chatwidget/tests/snapshots/codex_tui__chatwidget__tests__approval_requests__exec_approval_modal_exec.snap` |
| `stdin.txt` | `chatwidget/tests/snapshots/codex_tui__chatwidget__tests__approval_requests__write_stdin_approval_modal.snap` |
