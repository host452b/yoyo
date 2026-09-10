package agent

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/host452b/yoyo/internal/detector"
	"github.com/host452b/yoyo/internal/screen"
)

var continuationQuestion = regexp.MustCompile(`(?i)^(?:(?:would you like me to|would you like to|do you want me to|should i|shall i) continue\?|continue\?|是否继续[?？]|(?:要|需要我|需要)继续吗[?？])$`)
var idleShortcuts = regexp.MustCompile(`^\? for shortcuts(?:\s+\d+% context left)?$`)

// AFKContinuation returns a stable question identity only for a recognized
// composer and an explicit continuation-only question from the assistant.
// draft is empty before paste, or the exact expected text before submission.
// Unknown layouts deliberately abstain; silence alone is never readiness.
func (k Kind) AFKContinuation(view screen.Snapshot, draft string) string {
	marker, bullet := "❯", "⏺"
	switch k {
	case KindCodex:
		marker, bullet = "›", "•"
	case KindClaude:
	default:
		return ""
	}
	lines := strings.Split(view.Text, "\n")
	row := view.CursorY
	if !view.CursorVisible || row < 1 || row >= len(lines) {
		return ""
	}
	line := strings.TrimLeft(lines[row], " ")
	indent := len(lines[row]) - len(line)
	want := marker + " " + draft
	if strings.TrimSpace(line) != strings.TrimSpace(want) &&
		!(k == KindCodex && draft == "" && strings.TrimSpace(line) == "› Ask Codex to do anything") {
		return ""
	}
	if view.CursorX != indent+2+utf8.RuneCountInString(draft) {
		return ""
	}
	// Only known footer furniture may follow the input row. In particular,
	// multiline drafts, status spinners and modal choices are not composers.
	footer := false
	for _, l := range lines[row+1:] {
		l = strings.TrimSpace(l)
		if afkSeparator(l) {
			continue
		}
		if !idleShortcuts.MatchString(l) {
			return ""
		}
		footer = true
	}
	if !footer {
		return ""
	}
	end := row
	for end > 0 && afkSeparator(strings.TrimSpace(lines[end-1])) {
		end--
	}
	if end == 0 {
		return ""
	}
	start := end - 1
	for start >= 0 && !strings.HasPrefix(strings.TrimSpace(lines[start]), bullet+" ") {
		start--
	}
	if start < 0 {
		return ""
	}
	message := append([]string(nil), lines[start:end]...)
	message[0] = strings.TrimPrefix(strings.TrimSpace(message[0]), bullet+" ")
	for i := range message {
		message[i] = strings.TrimSpace(message[i])
	}
	if !continuationQuestion.MatchString(message[len(message)-1]) {
		return ""
	}
	body := strings.Join(message, "\n")
	lower := strings.ToLower(body)
	for _, blocked := range []string{"```", "~~~", "esc to", "ctrl+c", "ctrl-c", "enter to", "permission", "approval", "批准", "授权"} {
		if strings.Contains(lower, blocked) {
			return ""
		}
	}
	return detector.HashBody(k.String() + "\n" + strings.Join(strings.Fields(body), " "))
}

func afkSeparator(line string) bool {
	return strings.Trim(line, " ─━") == ""
}
