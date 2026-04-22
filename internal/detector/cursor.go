// internal/detector/cursor.go
package detector

import "strings"

// Cursor detects Cursor Agent permission prompts (box-drawn UI).
type Cursor struct{}

func (c Cursor) Detect(screenText string) *MatchResult {
	lines := strings.Split(screenText, "\n")

	// Find last bottom border └──┘
	bottomIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "└") && strings.HasSuffix(t, "┘") && strings.Contains(t, "─") {
			bottomIdx = i
			break
		}
	}
	if bottomIdx < 0 {
		return nil
	}

	// Find nearest top border ┌──┐ above bottom
	topIdx := -1
	for i := bottomIdx - 1; i >= 0; i-- {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "┌") && strings.HasSuffix(t, "┐") && strings.Contains(t, "─") {
			topIdx = i
			break
		}
	}
	if topIdx < 0 {
		return nil
	}

	var cleaned []string
	for _, line := range lines[topIdx+1 : bottomIdx] {
		l := strings.TrimSpace(line)
		l = strings.TrimPrefix(l, "│")
		l = strings.TrimSuffix(l, "│")
		l = strings.ReplaceAll(l, "→", "")
		l = strings.TrimSpace(l)
		if l != "" {
			cleaned = append(cleaned, l)
		}
	}
	// Newer Cursor Agent layout renders the command inside the box and the
	// approval question/options below it. Include post-box lines so the (y)/n)
	// markers are visible to the body check.
	for _, line := range lines[bottomIdx+1:] {
		l := strings.TrimSpace(line)
		l = strings.ReplaceAll(l, "→", "")
		l = strings.TrimSpace(l)
		if l != "" {
			cleaned = append(cleaned, l)
		}
	}
	body := strings.Join(cleaned, "\n")

	if !strings.Contains(body, "(y)") || !strings.Contains(body, "n)") {
		return nil
	}

	return &MatchResult{
		RuleName: "Cursor",
		Response: "\r",
		Hash:     hashBody(body),
	}
}
