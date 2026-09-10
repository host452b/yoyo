# AFK continuation guard

Scope: replace the blind AFK `y` and follow-up sequence. Fuzzy and approval
selection remain separate. AFK stays opt-in with the existing idle threshold.

After no input/output for the idle interval, require a supported Codex/Claude
composer, a visible cursor at the empty input position, a recognized shortcuts
footer, and a last assistant message ending in an explicit continuation-only
question. Unknown layouts, drafts, busy states, approvals, choice dialogs,
completed answers and quoted examples do not qualify. This is conservative
terminal evidence, not knowledge of the agent's internal execution state.

Paste one continuation message using bracketed paste. A separate event-loop
timer waits for the exact draft to be visible in the same composer before
pressing Enter. User input, a changed question, approval/busy state, write
failure, or lack of echo cancels submission. Never sleep inside the event loop.
Keep a bounded two-second submission window with at least 300 ms of output
silence before revalidation. Record attempted question hashes per session so
redraws and unchanged unanswered screens cannot cause an infinite nudge loop.
Do not erase text on cancellation; the user retains control of any visible draft.

Preserve dry-run and deletion-command checks at both paste and submit. Runtime
AFK-off cancels pending submission. AFK remains independent of auto-approve,
but never handles a screen currently matched by the approval rule chain.

Implementation: atomic text/cursor snapshots in screen, a conservative readiness
predicate in agent, and pending paste/submit state in proxy. Extend existing
proxy tests and add focused readiness cases covering both agent layouts, drafts,
busy/unknown states, questions, redraws, cancellation, delayed echo and failures.
Run affected tests, full Go tests, race checks on affected packages, and vet.

Evidence boundary: Codex layout/submit behavior is grounded in the local Codex
source; Claude idle layouts are constructed regression inputs. Real installed
CLI canaries remain necessary before claiming version-wide compatibility.
