# Matching replay comparison

Historical matching snapshot captured before the subsequent AFK changes.
The candidate fingerprint below is not the final v2.6.0 source fingerprint;
see [AFK verification](2026-09-10-afk-verification.md) for the later test scope.

Baseline: `08c9acabc8cb2519cb18d8b9ed569d90a4ece9e4`. Candidate: current working-tree snapshot.
Candidate content SHA-256 (internal sources/data and go.mod/go.sum): `e89462ed5030a54155b3d68bd259331c0d3f3eff51375db74135c38eff85962f`.

Both versions ran the same current tests. Counts are leaf cases, including width/parameter variants.
This corpus is not a production incident sample. Its pass rate is not a production accuracy estimate.

| Sample origin | Cases | Fixed | Preserved passes | Regressed | Still failing | Unevaluated |
|---|---:|---:|---:|---:|---:|---:|
| constructed_regression | 62 | 39 | 23 | 0 | 0 | 0 |
| existing_regression | 8 | 0 | 8 | 0 | 0 | 0 |
| historical_regression | 2 | 0 | 2 | 0 | 0 | 0 |
| upstream_snapshot | 21 | 3 | 18 | 0 | 0 | 0 |

| Case | Baseline | Candidate | Outcome |
|---|---|---|---|
| `TestClaude_3OptionPrompt_Bash` | pass | pass | unchanged_pass |
| `TestClaude_3OptionPrompt_PreselectedDontAskAgain` | pass | pass | unchanged_pass |
| `TestCodex_CommandPrompt` | pass | pass | unchanged_pass |
| `TestCodex_DefaultResponse` | pass | pass | unchanged_pass |
| `TestCodex_EditPrompt` | pass | pass | unchanged_pass |
| `TestCodex_HashPreservesCommandAndIgnoresSelection/command_marker_preserved` | fail | pass | fixed |
| `TestCodex_HashPreservesCommandAndIgnoresSelection/quoted_spaces_preserved` | pass | pass | unchanged_pass |
| `TestCodex_HashPreservesCommandAndIgnoresSelection/selection_ignored` | pass | pass | unchanged_pass |
| `TestCodex_McpPrompt` | pass | pass | unchanged_pass |
| `TestCodex_NoFooter` | pass | pass | unchanged_pass |
| `TestCodex_NoMatch` | pass | pass | unchanged_pass |
| `TestCodex_StripsSelectionMarker` | pass | pass | unchanged_pass |
| `TestCodex_StructuralMatching/approval_key_opens_thread` | pass | pass | unchanged_pass |
| `TestCodex_StructuralMatching/complete` | pass | pass | unchanged_pass |
| `TestCodex_StructuralMatching/conflicting_key` | pass | pass | unchanged_pass |
| `TestCodex_StructuralMatching/custom_approval` | fail | pass | fixed |
| `TestCodex_StructuralMatching/custom_confirmation` | fail | pass | fixed |
| `TestCodex_StructuralMatching/enter_assigned_to_cancel` | fail | pass | fixed |
| `TestCodex_StructuralMatching/enter_assigned_to_rejection` | fail | pass | fixed |
| `TestCodex_StructuralMatching/footer_ends_before_cancel_binding` | pass | pass | unchanged_pass |
| `TestCodex_StructuralMatching/general_question` | fail | pass | fixed |
| `TestCodex_StructuralMatching/header_and_footer_only` | fail | pass | fixed |
| `TestCodex_StructuralMatching/missing_reject` | fail | pass | fixed |
| `TestCodex_StructuralMatching/missing_selection` | fail | pass | fixed |
| `TestCodex_StructuralMatching/multiple_selections` | fail | pass | fixed |
| `TestCodex_StructuralMatching/new_incomplete_menu` | fail | pass | fixed |
| `TestCodex_StructuralMatching/number_gap` | fail | pass | fixed |
| `TestCodex_StructuralMatching/off-target_with_no_shortcut` | fail | pass | fixed |
| `TestCodex_StructuralMatching/old_title_with_unrelated_new_menu` | fail | pass | fixed |
| `TestCodex_StructuralMatching/persistent_grant_only` | fail | pass | fixed |
| `TestCodex_StructuralMatching/quoted_header` | fail | pass | fixed |
| `TestCodex_StructuralMatching/quoted_menu` | fail | pass | fixed |
| `TestCodex_StructuralMatching/rejection_selected` | fail | pass | fixed |
| `TestCodex_StructuralMatching/session_grant_selected` | fail | pass | fixed |
| `TestCodex_StructuralMatching/stale_menu` | fail | pass | fixed |
| `TestCodex_StructuralMatching/unknown_yes` | fail | pass | fixed |
| `TestCodex_StructuralMatching/unsupported_keys` | pass | pass | unchanged_pass |
| `TestCodex_StructuralMatching/wrapped_footer` | fail | pass | fixed |
| `TestCodex_StructuralMatching/wrapped_label` | pass | pass | unchanged_pass |
| `TestCodex_StructuralMatching/wrapped_title` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/additional-permissions.txt/120` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/additional-permissions.txt/60` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/additional-permissions.txt/80` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/cross-thread.txt/120` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/cross-thread.txt/60` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/cross-thread.txt/80` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/exec.txt/120` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/exec.txt/60` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/exec.txt/80` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/network.txt/120` | fail | pass | fixed |
| `TestCodex_UpstreamSnapshots/network.txt/60` | fail | pass | fixed |
| `TestCodex_UpstreamSnapshots/network.txt/80` | fail | pass | fixed |
| `TestCodex_UpstreamSnapshots/patch.txt/120` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/patch.txt/60` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/patch.txt/80` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/permissions.txt/120` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/permissions.txt/60` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/permissions.txt/80` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/stdin.txt/120` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/stdin.txt/60` | pass | pass | unchanged_pass |
| `TestCodex_UpstreamSnapshots/stdin.txt/80` | pass | pass | unchanged_pass |
| `TestCursor_CommandBoxWithoutPromptIsIgnored` | pass | pass | unchanged_pass |
| `TestCursor_CommandPrompt` | pass | pass | unchanged_pass |
| `TestCursor_DefaultResponse` | pass | pass | unchanged_pass |
| `TestCursor_IgnoresInputBox` | pass | pass | unchanged_pass |
| `TestCursor_NoMatchWithoutBox` | pass | pass | unchanged_pass |
| `TestCursor_NoMatchWithoutOptions` | pass | pass | unchanged_pass |
| `TestCursor_PicksLastBox` | pass | pass | unchanged_pass |
| `TestCursor_PromptBelowCommandBox` | pass | pass | unchanged_pass |
| `TestProxy_E2E_CodexFooterRedraw` | fail | pass | fixed |
| `TestProxy_E2E_CodexReappearance` | pass | pass | unchanged_pass |
| `TestProxy_E2E_FragmentedCodexApproval/Press_enter_to_confirm_or_esc_to_cancel` | pass | pass | unchanged_pass |
| `TestProxy_E2E_FragmentedCodexApproval/Press_enter_to_confirm_or_esc_to_cancel_or_o_to_open_thread` | pass | pass | unchanged_pass |
| `TestProxy_E2E_ResizeInvalidatesDelayedMatch` | fail | pass | fixed |
| `TestProxy_E2E_SeenPromptCancelsPreviousDelay` | fail | pass | fixed |
| `TestProxy_E2E_SelectionChangesDuringDelay/claude` | fail | pass | fixed |
| `TestProxy_E2E_SelectionChangesDuringDelay/codex` | fail | pass | fixed |
| `TestScreen_SplitUTF8AndANSI` | fail | pass | fixed |
| `TestScreen_StreamEquivalence/claude/chunk_1` | fail | pass | fixed |
| `TestScreen_StreamEquivalence/claude/chunk_2` | fail | pass | fixed |
| `TestScreen_StreamEquivalence/claude/chunk_3` | fail | pass | fixed |
| `TestScreen_StreamEquivalence/claude/chunk_4096` | pass | pass | unchanged_pass |
| `TestScreen_StreamEquivalence/claude/chunk_7` | fail | pass | fixed |
| `TestScreen_StreamEquivalence/codex/chunk_1` | fail | pass | fixed |
| `TestScreen_StreamEquivalence/codex/chunk_2` | fail | pass | fixed |
| `TestScreen_StreamEquivalence/codex/chunk_3` | fail | pass | fixed |
| `TestScreen_StreamEquivalence/codex/chunk_4096` | pass | pass | unchanged_pass |
| `TestScreen_StreamEquivalence/codex/chunk_7` | pass | pass | unchanged_pass |
| `TestScreen_StreamEquivalence/cursor/chunk_1` | fail | pass | fixed |
| `TestScreen_StreamEquivalence/cursor/chunk_2` | fail | pass | fixed |
| `TestScreen_StreamEquivalence/cursor/chunk_3` | fail | pass | fixed |
| `TestScreen_StreamEquivalence/cursor/chunk_4096` | pass | pass | unchanged_pass |
| `TestScreen_StreamEquivalence/cursor/chunk_7` | fail | pass | fixed |
