package detector

import (
	"regexp"
	"strconv"
	"strings"
)

var codexMCPToolTitle = regexp.MustCompile(`^Allowthe.+MCPservertoruntool"[^"]+"\?$`)

// MCP tool approvals use a single-field elicitation form, with different titles,
// options and key handling from the ordinary approval overlay.
func detectCodexMCPToolApproval(rawLines, lines []string) *MatchResult {
	compact := func(rows []string) string {
		return strings.Join(strings.Fields(strings.Join(rows, "")), "")
	}
	footerIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "enter ") {
			if compact(lines[i:]) != "entertosubmit|esctocancel" {
				return nil
			}
			footerIdx = i
			break
		}
	}
	if footerIdx < 0 {
		return nil
	}
	fieldIdx, menuIdx := -1, -1
	for i := footerIdx - 1; i >= 0; i-- {
		if menuIdx < 0 {
			if m := codexOptionLine.FindStringSubmatch(lines[i]); m != nil && m[2] == "1" {
				menuIdx = i
			}
		}
		if strings.HasPrefix(lines[i], "Field ") {
			fieldIdx = i
			break
		}
	}
	if fieldIdx < 0 || menuIdx <= fieldIdx+1 ||
		(lines[fieldIdx] != "Field 1/1" && lines[fieldIdx] != "Field 1/1 (1 required unanswered)") {
		return nil
	}
	titleEnd := fieldIdx + 1
	for titleEnd < menuIdx && lines[titleEnd] != "" {
		titleEnd++
	}
	if !codexMCPToolTitle.MatchString(compact(lines[fieldIdx+1 : titleEnd])) {
		return nil
	}

	var labels []string
	selections := 0
	body := append([]string(nil), lines[fieldIdx+1:menuIdx]...)
	for _, line := range lines[menuIdx:footerIdx] {
		if line == "" {
			continue
		}
		if m := codexOptionLine.FindStringSubmatch(line); m != nil {
			n, err := strconv.Atoi(m[2])
			if err != nil || n != len(labels)+1 {
				return nil
			}
			if m[1] != "" {
				selections++
			}
			labels = append(labels, compact([]string{m[3]}))
			body = append(body, m[2]+". "+m[3])
		} else {
			labels[len(labels)-1] += compact([]string{line})
			body = append(body, line)
		}
	}
	if selections != 1 || len(labels) < 2 || len(labels) > 4 ||
		labels[0] != "AllowRunthetoolandcontinue." || labels[len(labels)-1] != "CancelCancelthistoolcall" {
		return nil
	}
	// Session and persistent grants are optional, in that order. Require the
	// full known choices so a truncated or unrelated form cannot be submitted.
	pos := 1
	if labels[pos] == "AllowforthissessionRunthetoolandrememberthischoiceforthissession." {
		pos++
	}
	if labels[pos] == "AlwaysallowRunthetoolandrememberthischoiceforfuturetoolcalls." {
		pos++
	}
	if pos != len(labels)-1 {
		return nil
	}
	// McpServerElicitationOverlay's digit handler selects AND submits. Always
	// choose the single-call grant, regardless of the highlighted option.
	return &MatchResult{
		RuleName: "Codex", Response: "1", Hash: hashBody(strings.Join(body, "\n")),
		// Keep the stable request title and arguments as visibility evidence:
		// progress, selection and footer can redraw together after submission.
		PromptText: strings.TrimSpace(strings.Join(rawLines[fieldIdx+1:menuIdx], "\n")),
	}
}
