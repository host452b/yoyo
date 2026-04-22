// internal/detector/danger.go
//
// Safety guard: reject auto-approval when the visible screen text looks
// like it is asking the user to confirm a **deletion or cleanup**
// operation. Scope is deliberately narrow — this is for containerised
// dev workflows where mkfs / dd / chmod / curl|sh are normal; the only
// class yoyo refuses to auto-approve is the one that removes data or
// resources you can't trivially recreate.
//
// The contract:
//
//   - patterns err on the side of false-positives (fall back to manual)
//   - the user can still approve by pressing y themselves; yoyo just
//     refuses to do it automatically
//   - pass -no-safety to disable this entirely
//
// Scope is hard-coded (not user-extensible) so operators of shared
// environments can rely on a predictable floor.
package detector

import (
	"regexp"
	"strings"
)

// safetyScanTail is how many trailing lines of screen text the safety guard
// examines. Keeping this small ensures the guard reacts to the CURRENT
// prompt / active command area, not to stale scrollback from previously
// approved commands (e.g. an approved `kubectl delete` scrolling slowly
// off-screen would otherwise block every subsequent approval until it
// scrolled out of the buffer entirely).
const safetyScanTail = 20

var dangerousPatterns = []*regexp.Regexp{
	// rm -r / rm -rf targeting top-level or glob paths.
	regexp.MustCompile(`(?i)\brm\s+-[a-zA-Z]*r[a-zA-Z]*\s+(-[a-zA-Z]+\s+)*(\*|~|/|/\*|/\s|\$HOME)`),
	// git rm with recursive flag.
	regexp.MustCompile(`(?i)\bgit\s+rm\s+-r`),
	// git clean -fdx and similar force-cleans.
	regexp.MustCompile(`(?i)\bgit\s+clean\s+-[a-zA-Z]*f`),
	// find ... -delete or find ... -exec rm
	regexp.MustCompile(`(?i)\bfind\b[^\n]*(-delete\b|-exec\s+rm\b)`),
	// Database destruction.
	regexp.MustCompile(`(?i)\bDROP\s+(DATABASE|TABLE|SCHEMA|USER)\b`),
	regexp.MustCompile(`(?i)\bTRUNCATE\s+TABLE\b`),
	regexp.MustCompile(`(?i)\bDELETE\s+FROM\b[^\n]*(?:;|$)`), // DELETE FROM without WHERE (best-effort)
	// kubectl delete against any resource.
	regexp.MustCompile(`(?i)\bkubectl\s+delete\b`),
	// Terraform destroy.
	regexp.MustCompile(`(?i)\bterraform\s+(destroy|apply\s+-destroy)\b`),
	// Docker / podman volume or system prune.
	regexp.MustCompile(`(?i)\b(docker|podman)\s+(volume\s+rm|system\s+prune|image\s+prune\s+-a)`),
}

// ContainsDangerousCommand reports whether the trailing ~20 lines of text
// visibly contain a deletion-class command that must never be
// auto-approved. Returns (true, matchedSnippet) on hit; (false, "")
// otherwise. The snippet is the exact substring that triggered the match
// — useful for logging and for showing the user why yoyo stepped back.
//
// The last-N-lines scope exists so an approved destructive command
// lingering in the scrollback (e.g. the user manually approved
// `kubectl delete ns foo` earlier and yoyo is now handling a totally
// unrelated prompt on the bottom of the screen) doesn't block every
// subsequent auto-approval until it scrolls off.
func ContainsDangerousCommand(text string) (bool, string) {
	return containsDangerousIn(tailLines(text, safetyScanTail))
}

// ContainsDangerousCommandFull is an escape hatch for tests / callers
// that explicitly want to scan the entire input without the trailing
// window clamp. Production code (proxy.sendApproval, AFK fire case)
// must use ContainsDangerousCommand.
func ContainsDangerousCommandFull(text string) (bool, string) {
	return containsDangerousIn(text)
}

func containsDangerousIn(text string) (bool, string) {
	for _, re := range dangerousPatterns {
		if loc := re.FindStringIndex(text); loc != nil {
			return true, text[loc[0]:loc[1]]
		}
	}
	return false, ""
}

// tailLines returns the last n non-empty-suffix lines of s, joined by \n.
// Trailing blank padding (vt10x renders a full-height screen) is trimmed
// first so that the window covers real content, not blank rows.
func tailLines(s string, n int) string {
	// Right-strip trailing blank / whitespace-only lines.
	trimmed := strings.TrimRight(s, "\n \t\r")
	lines := strings.Split(trimmed, "\n")
	start := 0
	if len(lines) > n {
		start = len(lines) - n
	}
	return strings.Join(lines[start:], "\n")
}
