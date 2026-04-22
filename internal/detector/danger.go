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

import "regexp"

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

// ContainsDangerousCommand reports whether text visibly contains a
// deletion-class command that must never be auto-approved. Returns
// (true, matchedSnippet) on hit; (false, "") otherwise. The snippet is
// the exact substring that triggered the match — useful for logging
// and for showing the user why yoyo stepped back.
func ContainsDangerousCommand(text string) (bool, string) {
	for _, re := range dangerousPatterns {
		if loc := re.FindStringIndex(text); loc != nil {
			return true, text[loc[0]:loc[1]]
		}
	}
	return false, ""
}
