// internal/agent/agent.go
package agent

import (
	"path/filepath"
	"strings"

	"yoyo/internal/detector"
)

// Kind identifies the type of AI agent CLI being proxied.
type Kind int

const (
	KindUnknown Kind = iota
	KindClaude
	KindCodex
	KindCursor
)

func (k Kind) String() string {
	switch k {
	case KindClaude:
		return "claude"
	case KindCodex:
		return "codex"
	case KindCursor:
		return "cursor"
	default:
		return "unknown"
	}
}

// KindFromCommand identifies the agent from the command name.
// Strips path components and Windows extensions (.exe, .cmd, .bat).
func KindFromCommand(command string) Kind {
	if command == "" {
		return KindUnknown
	}
	base := filepath.Base(command)
	// Strip Windows extensions
	for _, ext := range []string{".exe", ".cmd", ".bat"} {
		base = strings.TrimSuffix(base, ext)
	}
	switch strings.ToLower(base) {
	case "claude":
		return KindClaude
	case "codex":
		return KindCodex
	case "cursor", "cursor-agent":
		return KindCursor
	default:
		return KindUnknown
	}
}

// KindFromScreen identifies the agent from visible terminal content.
// Used as fallback when the command name is not recognized.
func KindFromScreen(screenText string) Kind {
	switch {
	case strings.Contains(screenText, "Claude Code"):
		return KindClaude
	case strings.Contains(screenText, "codex") || strings.Contains(screenText, "Codex"):
		return KindCodex
	case strings.Contains(screenText, "cursor") || strings.Contains(screenText, "Cursor"):
		return KindCursor
	default:
		return KindUnknown
	}
}

// Detector returns the appropriate built-in Detector for this agent kind.
// For KindUnknown, returns a multi-detector that tries all built-ins in order.
func (k Kind) Detector() detector.Detector {
	switch k {
	case KindClaude:
		return detector.Claude{}
	case KindCodex:
		return detector.Codex{}
	case KindCursor:
		return detector.Cursor{}
	default:
		return multiDetector{detector.Claude{}, detector.Codex{}, detector.Cursor{}}
	}
}

// Detect implements the detector.Detector interface.
// For a specific Kind, uses its specialized detector.
// For KindUnknown, tries all detectors in order.
func (k Kind) Detect(screenText string) *detector.MatchResult {
	return k.Detector().Detect(screenText)
}

// multiDetector tries each detector in order, returning the first match.
type multiDetector []detector.Detector

func (m multiDetector) Detect(screenText string) *detector.MatchResult {
	for _, d := range m {
		if r := d.Detect(screenText); r != nil {
			return r
		}
	}
	return nil
}
