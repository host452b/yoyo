package agent_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/host452b/yoyo/internal/agent"
	"github.com/host452b/yoyo/internal/screen"
)

func TestAFKContinuation(t *testing.T) {
	for _, kind := range []agent.Kind{agent.KindCodex, agent.KindClaude} {
		t.Run(kind.String(), func(t *testing.T) {
			marker, bullet := "❯", "⏺"
			if kind == agent.KindCodex {
				marker, bullet = "›", "•"
			}
			base := bullet + " Should I continue?\n\n" + marker + " \n\n  ? for shortcuts"
			cases := []struct {
				name, text, draft string
				col, row          int
				hidden, want      bool
			}{
				{"empty composer", base, "", 2, 2, false, true},
				{"chinese", strings.Replace(base, "Should I continue?", "需要我继续吗？", 1), "", 2, 2, false, true},
				{"assistant context", strings.Replace(base, "Should I", "Step one finished.\nShould I", 1), "", 2, 3, false, true},
				{"draft", strings.Replace(base, marker+" ", marker+" my draft", 1), "", 10, 2, false, false},
				{"draft cursor at start", strings.Replace(base, marker+" ", marker+" my draft", 1), "", 2, 2, false, false},
				{"hidden cursor", base, "", 2, 2, true, false},
				{"wrong cursor row", base, "", 2, 0, false, false},
				{"unknown footer", strings.Replace(base, "? for shortcuts", "custom mode", 1), "", 2, 2, false, false},
				{"busy", base + "\nesc to interrupt", "", 2, 2, false, false},
				{"approval", base + "\nPress enter to confirm or esc to cancel", "", 2, 2, false, false},
				{"choice dialog", base + "\nenter to submit answer", "", 2, 2, false, false},
				{"completed", strings.Replace(base, "Should I continue?", "All tasks are complete.", 1), "", 2, 2, false, false},
				{"business question", strings.Replace(base, "Should I continue?", "Should I continue with production deployment?", 1), "", 2, 2, false, false},
				{"quoted question", strings.Replace(base, "Should I continue?", "> Should I continue?", 1), "", 2, 2, false, false},
				{"code example", strings.Replace(base, "Should I continue?", "```\nShould I continue?", 1), "", 2, 3, false, false},
				{"old question", strings.Replace(base, "Should I continue?", "Should I continue?\nWork is complete.", 1), "", 2, 3, false, false},
				{"no assistant marker", strings.Replace(base, bullet+" ", "", 1), "", 2, 2, false, false},
				{"echoed draft", strings.Replace(base, marker+" ", marker+"Continue", 1), "Continue", 9, 2, false, false},
				{"exact echoed draft", strings.Replace(base, marker+" ", marker+" Continue", 1), "Continue", 10, 2, false, true},
			}
			if kind == agent.KindCodex {
				cases = append(cases, struct {
					name, text, draft string
					col, row          int
					hidden, want      bool
				}{
					"upstream placeholder", strings.Replace(base, "› ", "› Ask Codex to do anything", 1), "", 2, 2, false, true})
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					s := screen.New(100, 24)
					s.Feed([]byte(strings.ReplaceAll(tc.text, "\n", "\r\n")))
					s.Feed([]byte(fmt.Sprintf("\x1b[%d;%dH", tc.row+1, tc.col+1)))
					if tc.hidden {
						s.Feed([]byte("\x1b[?25l"))
					}
					if got := kind.AFKContinuation(s.Snapshot(), tc.draft); (got != "") != tc.want {
						t.Fatalf("eligible = %v, want %v", got != "", tc.want)
					}
				})
			}
		})
	}
}

func TestAFKContinuationIdentitySurvivesContextWrap(t *testing.T) {
	var previous string
	for _, context := range []string{"Step one is finished.", "Step one is\nfinished."} {
		text := "• " + context + "\nShould I continue?\n\n› \n\n? for shortcuts"
		view := screen.Snapshot{Text: text, CursorX: 2, CursorY: 3 + strings.Count(context, "\n"), CursorVisible: true}
		key := agent.KindCodex.AFKContinuation(view, "")
		if key == "" {
			t.Fatal("expected eligible question")
		}
		if previous != "" && key != previous {
			t.Fatal("context wrapping changed question identity")
		}
		previous = key
	}
}
