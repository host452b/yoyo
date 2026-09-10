# Codex approval fixtures

Visible rows extracted from local Codex commit `ddea03ad049142943bdbf13e937b1d67e8c1ba0c`.
Upstream: https://github.com/openai/codex (Apache-2.0). Only right padding and snapshot metadata were removed.
Tests do not require the Codex checkout.

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
