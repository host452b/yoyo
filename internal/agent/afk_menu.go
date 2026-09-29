package agent

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/host452b/yoyo/internal/detector"
	"github.com/host452b/yoyo/internal/screen"
)

var afkMenuOption = regexp.MustCompile(`^([›❯]\s*)?([1-9][0-9]*)\.\s+(.+)$`)

// AFKDefaultEnter recognizes menus whose Enter key accepts the highlighted
// choice. It does not select a different item or submit a chat draft.
func (k Kind) AFKDefaultEnter(view screen.Snapshot) *detector.MatchResult {
	if k != KindCodex && k != KindClaude {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(view.Text), "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	compact := func(rows []string) string {
		return strings.Join(strings.Fields(strings.Join(rows, "")), "")
	}
	if match := k.Detect(view.Text); match != nil {
		selected := 0
		for _, line := range lines {
			if m := afkMenuOption.FindStringSubmatch(line); m != nil && m[1] != "" {
				selected++
			}
		}
		if selected != 1 {
			return nil
		}
		if k == KindClaude {
			if lines[len(lines)-1] != "Esc to cancel" {
				return nil
			}
		} else if match.Response != "1" { // Known Codex retry menus accept Enter too.
			footer := ""
			for i := len(lines) - 1; i >= 0; i-- {
				if strings.HasPrefix(lines[i], "Press ") {
					footer = compact(lines[i:])
					break
				}
			}
			if (footer != "Pressentertoconfirmoresctocancel" && footer != "Pressentertoconfirmoresctocancelorotoopenthread") ||
				strings.Contains(compact(lines), "(enter)") {
				return nil
			}
		}
		match.Response = "\r"
		return match
	}
	if k != KindCodex {
		return nil
	}

	// Codex single-field select forms (including MCP tool approval) display
	// a numbered list and this footer. Multiple fields and free-text inputs
	// need further user decisions, so they do not qualify for default Enter.
	footer, field, menu := -1, -1, -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "enter ") {
			if compact(lines[i:]) != "entertosubmit|esctocancel" {
				return nil
			}
			footer = i
			break
		}
	}
	if footer < 0 {
		return nil
	}
	for i := footer - 1; i >= 0; i-- {
		if menu < 0 {
			if m := afkMenuOption.FindStringSubmatch(lines[i]); m != nil && m[2] == "1" {
				menu = i
			}
		}
		if strings.HasPrefix(lines[i], "Field ") {
			field = i
			break
		}
	}
	if field < 0 || menu <= field+1 || (lines[field] != "Field 1/1" && lines[field] != "Field 1/1 (1 required unanswered)") {
		return nil
	}
	if field > 0 && lines[field-1] != "" {
		return nil
	}
	body := append([]string(nil), lines[field+1:menu]...)
	count, selected := 0, 0
	for _, line := range lines[menu:footer] {
		if m := afkMenuOption.FindStringSubmatch(line); m != nil {
			n, err := strconv.Atoi(m[2])
			if err != nil || n != count+1 {
				return nil
			}
			count++
			if m[1] != "" {
				selected++
			}
			line = m[2] + ". " + m[3]
		}
		body = append(body, line)
	}
	if count < 2 || selected != 1 {
		return nil
	}
	return &detector.MatchResult{
		RuleName: "Codex", Response: "\r", Hash: detector.HashBody("afk-default\n" + strings.Join(body, "\n")),
		PromptText: strings.Join(lines[field+1:menu], "\n"),
	}
}
