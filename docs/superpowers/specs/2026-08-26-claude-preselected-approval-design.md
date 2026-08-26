# Claude Preselected Approval — Design

Status: approved
Date: 2026-08-26

## Purpose

Claude Code can render a three-option Bash approval dialog with option 2
already selected:

```text
  1. Yes
❯ 2. Yes, and don't ask again for: <pattern>
  3. No
```

yoyo currently assumes option 1 is selected whenever the dialog contains
`ask again for`, so it sends Down followed by Enter. With the newer initial
selection this moves the cursor to option 3 and rejects the command.

A diagnostic dump confirmed that the Claude detector matched, safety did not
block the command, and the approval timer sent its response. This is therefore
a response-selection bug rather than a prompt-detection bug.

## Behaviour

Keep the existing preference for Claude's `don't ask again` option while
accounting for the new initial selection:

- If the newest Claude dialog contains `ask again for` and its option 2 line
  is marked with `❯`, send Enter only.
- Otherwise, preserve the current `ask again for` response of Down followed by
  Enter. This keeps compatibility with dialogs that initially select option 1
  or whose selection marker is unavailable.
- Preserve Enter-only responses for ordinary two-option Claude dialogs.

The selection check is limited to the already-isolated newest dialog. Older
stacked dialogs must not influence the response.

## Code Scope

- `internal/detector/claude.go`: inspect the isolated dialog lines for a
  selected option 2 before removing the `❯` marker, then choose the response.
- `internal/detector/claude_3opt_test.go`: add a regression test based on the
  diagnostic screen shape, including VT100 screen rendering.
- Keep the existing option-1 and two-option tests unchanged as compatibility
  coverage.

No proxy, safety, memory, configuration, or non-Claude detector behaviour
changes.

## Test Strategy

Follow a red-green cycle:

1. Add a regression test where option 2 is preselected and assert an Enter-only
   response. Confirm it fails because the detector returns Down plus Enter.
2. Add the minimal selection-aware response logic.
3. Confirm the focused Claude detector tests pass.
4. Run the complete Go test suite and inspect the final diff.

## Non-goals

- General cursor navigation between arbitrary option numbers.
- Changing which Claude option yoyo prefers.
- Altering approval timing, deduplication, or dangerous-command handling.
- Releasing or installing a new yoyo binary.
