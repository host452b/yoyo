package proxy_test

import (
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/host452b/yoyo/internal/agent"
	"github.com/host452b/yoyo/internal/proxy"
)

func TestProxy_AfkSkipsNonContinuationStates(t *testing.T) {
	for _, kind := range []agent.Kind{agent.KindCodex, agent.KindClaude, agent.KindUnknown} {
		frames := map[string]string{
			"no output":         "",
			"completed":         afkTestScreen(kind, "All work is complete.", ""),
			"draft":             afkTestScreen(kind, "Should I continue?", "my draft"),
			"busy":              afkTestScreen(kind, "Should I continue?\r\nesc to interrupt", ""),
			"business question": afkTestScreen(kind, "Which database should I use?", ""),
		}
		if kind == agent.KindUnknown {
			frames["unidentified composer"] = afkTestScreen(agent.KindClaude, "Should I continue?", "")
		}
		for name, frame := range frames {
			t.Run(kind.String()+"/"+name, func(t *testing.T) {
				pr, pty, stdin := makeProxyWithAfkConfig(t, 70*time.Millisecond, false, kind, func(cfg *proxy.Config) {
					cfg.Enabled = false // approvals must stay manual
				})
				defer stdin.close()
				done := runProxy(pr)
				defer func() { pty.close(); <-done }()
				if frame != "" {
					pty.send(frame)
				}
				time.Sleep(250 * time.Millisecond)
				if got := pty.written(); got != "" {
					t.Fatalf("unexpected input: %q", got)
				}
			})
		}
	}
}

const afkDefaultMenu = "Field 1/1 (1 required unanswered)\r\n" +
	"Allow the maas-jira MCP server to run tool \"jira_get_issue\"?\r\n\r\n" +
	"  1. Allow                   Run the tool and continue.\r\n" +
	"  2. Allow for this session  Run the tool and remember this choice for this session.\r\n" +
	"› 3. Always allow            Run the tool and remember this choice for future tool calls.\r\n" +
	"  4. Cancel                  Cancel this tool call\r\n" +
	"enter to submit | esc to cancel\r\n"

func TestProxy_AfkDefaultEnter(t *testing.T) {
	for _, tc := range []struct {
		name, prompt string
		kind         agent.Kind
	}{
		{"codex approval", strings.ReplaceAll(strings.ReplaceAll(codexPrompt, "› 1.", "  1."), "  2.", "› 2."), agent.KindCodex},
		{"codex form", afkDefaultMenu, agent.KindCodex},
		{"claude approval", "──────────────\r\nRead /etc/hosts\r\n  1. Yes\r\n❯ 2. No\r\nEsc to cancel\r\n", agent.KindClaude},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pr, pty, stdin := makeProxyWithAfkConfig(t, 150*time.Millisecond, false, tc.kind, func(cfg *proxy.Config) {
				cfg.Enabled = false // AFK has its own opt-in switch.
			})
			done := runProxy(pr)
			defer func() { pty.close(); stdin.close(); <-done }()
			pty.send(tc.prompt)
			ensureNotWritten(t, pty, "\r", 70*time.Millisecond)
			waitWritten(t, pty, "\r", time.Second)
			pty.send("\x1b[2J\x1b[H" + tc.prompt)
			time.Sleep(350 * time.Millisecond)
			if got := pty.written(); got != "\r" {
				t.Fatalf("expected exactly one Enter for the highlighted choice, got %q", got)
			}
		})
	}
}

func TestProxy_AfkDefaultEnterGuards(t *testing.T) {
	for _, tc := range []struct {
		name, prompt     string
		dryRun, disabled bool
	}{
		{"dry run", afkDefaultMenu, true, false},
		{"AFK off", afkDefaultMenu, false, true},
		{"no selection", strings.ReplaceAll(afkDefaultMenu, "›", " "), false, false},
		{"incomplete footer", strings.ReplaceAll(afkDefaultMenu, "esc to cancel", "esc to can"), false, false},
		{"multiple fields", strings.ReplaceAll(afkDefaultMenu, "Field 1/1", "Field 1/2"), false, false},
		{"danger", strings.ReplaceAll(afkDefaultMenu, "jira_get_issue", "rm -rf /"), false, false},
		{"stale menu", afkDefaultMenu + "Working...\r\n", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pr, pty, stdin := makeProxyWithAfkConfig(t, 70*time.Millisecond, tc.dryRun, agent.KindCodex, func(cfg *proxy.Config) {
				cfg.Enabled = false
				cfg.AfkEnabled = !tc.disabled
				cfg.SafetyEnabled = true
			})
			done := runProxy(pr)
			defer func() { pty.close(); stdin.close(); <-done }()
			pty.send(tc.prompt)
			time.Sleep(200 * time.Millisecond)
			if got := pty.written(); got != "" {
				t.Fatalf("unexpected AFK input: %q", got)
			}
		})
	}
}

func TestProxy_AfkDoesNotRepeatAutomaticApproval(t *testing.T) {
	pr, pty, stdin := makeProxyWithAfk(t, 70*time.Millisecond, false, agent.KindCodex)
	done := runProxy(pr)
	defer func() { pty.close(); stdin.close(); <-done }()
	prompt := strings.ReplaceAll(strings.ReplaceAll(codexPrompt, "› 1.", "  1."), "  2.", "› 2.")
	pty.send(prompt)
	waitWritten(t, pty, "y", time.Second)
	time.Sleep(250 * time.Millisecond)
	if got := pty.written(); got != "y" {
		t.Fatalf("AFK confirmed an already handled approval: %q", got)
	}
}

func TestProxy_AfkDefaultEnterAdvancesToNextMenu(t *testing.T) {
	pr, pty, stdin := makeProxyWithAfkConfig(t, 70*time.Millisecond, false, agent.KindCodex, func(cfg *proxy.Config) {
		cfg.Enabled = false
	})
	done := runProxy(pr)
	defer func() { pty.close(); stdin.close(); <-done }()
	for i, tool := range []string{"jira_get_issue", "jira_get_comments"} {
		pty.send("\x1b[2J\x1b[H" + strings.ReplaceAll(afkDefaultMenu, "jira_get_issue", tool))
		waitWritten(t, pty, strings.Repeat("\r", i+1), time.Second)
	}
	if got := pty.written(); got != "\r\r" {
		t.Fatalf("wrong input across two menus: %q", got)
	}
}

func TestProxy_AfkCancelsPendingSubmission(t *testing.T) {
	for _, kind := range []agent.Kind{agent.KindCodex, agent.KindClaude} {
		for _, cause := range []string{"user text", "navigation", "afk off", "busy", "different question", "edited draft", "danger"} {
			t.Run(kind.String()+"/"+cause, func(t *testing.T) {
				pr, pty, stdin := makeProxyWithAfkConfig(t, 80*time.Millisecond, false, kind, func(cfg *proxy.Config) {
					cfg.SafetyEnabled = true
				})
				defer stdin.close()
				done := runProxy(pr)
				defer func() { pty.close(); <-done }()
				pty.send(afkTestScreen(kind, "Should I continue?", ""))
				waitWritten(t, pty, afkTestPaste, time.Second)
				frame := afkTestScreen(kind, "Should I continue?", afkTestMessage)
				switch cause {
				case "user text":
					stdin.send("x")
				case "navigation":
					stdin.send("\x1b[D")
				case "afk off":
					stdin.send("\x19a")
				case "busy":
					frame = afkTestScreen(kind, "Should I continue?\r\nesc to interrupt", afkTestMessage)
				case "different question":
					frame = afkTestScreen(kind, "Would you like me to continue?", afkTestMessage)
				case "edited draft":
					frame = afkTestScreen(kind, "Should I continue?", afkTestMessage+"x")
				case "danger":
					frame = "\x1b[2J\x1b[Hrm -rf /\r\n" + strings.Replace(afkTestScreen(kind, "Should I continue?", afkTestMessage), "\x1b[2J\x1b[H", "\x1b[2;1H", 1)
					frame = strings.ReplaceAll(frame, "\x1b[3;", "\x1b[4;")
				}
				pty.send(frame)
				ensureNotWritten(t, pty, "\r", 650*time.Millisecond)
			})
		}
	}
}

func TestProxy_AfkAcceptsNewQuestionAfterResponse(t *testing.T) {
	pr, pty, stdin := makeProxyWithAfk(t, 80*time.Millisecond, false, agent.KindCodex)
	defer stdin.close()
	done := runProxy(pr)
	defer func() { pty.close(); <-done }()
	for i, question := range []string{"Step one finished.\r\nShould I continue?", "Step two finished.\r\nShould I continue?"} {
		pty.send(afkTestScreen(agent.KindCodex, question, ""))
		waitWritten(t, pty, strings.Repeat(afkTestPaste+"\r", i)+afkTestPaste, time.Second)
		pty.send(afkTestScreen(agent.KindCodex, question, afkTestMessage))
		waitWritten(t, pty, strings.Repeat(afkTestPaste+"\r", i+1), time.Second)
	}
}

type afkBrokenWriter struct {
	io.ReadWriter
	short bool
	calls *atomic.Int64
}

func (w afkBrokenWriter) Write(p []byte) (int, error) {
	w.calls.Add(1)
	if w.short {
		return w.ReadWriter.Write(p[:len(p)/2])
	}
	return 0, fmt.Errorf("simulated PTY write failure")
}

func TestProxy_AfkPasteFailureDoesNotSubmit(t *testing.T) {
	for _, short := range []bool{false, true} {
		t.Run(fmt.Sprint("short=", short), func(t *testing.T) {
			var calls atomic.Int64
			pr, pty, stdin := makeProxyWithAfkConfig(t, 70*time.Millisecond, false, agent.KindClaude, func(cfg *proxy.Config) {
				cfg.PTY = afkBrokenWriter{cfg.PTY, short, &calls}
			})
			defer stdin.close()
			done := runProxy(pr)
			defer func() { pty.close(); <-done }()
			pty.send(afkTestScreen(agent.KindClaude, "Should I continue?", ""))
			time.Sleep(250 * time.Millisecond)
			pty.send(afkTestScreen(agent.KindClaude, "Should I continue?", afkTestMessage))
			time.Sleep(450 * time.Millisecond)
			want := ""
			if short {
				want = afkTestPaste[:len(afkTestPaste)/2]
			}
			if got := pty.written(); got != want {
				t.Fatalf("retried failed paste or submitted it: %q", got)
			}
			if got := calls.Load(); got != 1 {
				t.Fatalf("write calls = %d, want one attempt", got)
			}
		})
	}
}

func TestProxy_AfkPendingPasteCannotBeSubmittedByFuzzy(t *testing.T) {
	pr, pty, stdin := makeProxyWithAfkConfig(t, 70*time.Millisecond, false, agent.KindCodex, func(cfg *proxy.Config) {
		cfg.FuzzyEnabled = true
		cfg.FuzzyStable = 150 * time.Millisecond
	})
	defer stdin.close()
	done := runProxy(pr)
	defer func() { pty.close(); <-done }()
	question := "Earlier output mentioned yes/no.\r\nShould I continue?"
	pty.send(afkTestScreen(agent.KindCodex, question, ""))
	waitWritten(t, pty, afkTestPaste, time.Second)
	pty.send(afkTestScreen(agent.KindCodex, question, afkTestMessage))
	ensureNotWritten(t, pty, "\r", 250*time.Millisecond)
	waitWritten(t, pty, afkTestPaste+"\r", time.Second)
	if got := pty.written(); got != afkTestPaste+"\r" {
		t.Fatalf("duplicate submission: %q", got)
	}
}

func TestProxy_AfkCountdownRefreshesWithoutOutput(t *testing.T) {
	output := newFakePTY() // Synchronized writer also captures status-bar output.
	pr, pty, stdin := makeProxyWithAfkConfig(t, 3*time.Second, false, agent.KindCodex, func(cfg *proxy.Config) {
		cfg.Stdout = output
	})
	done := runProxy(pr)
	defer func() { pty.close(); stdin.close(); <-done }()
	pty.send(afkTestScreen(agent.KindCodex, "Should I continue?", ""))
	waitWritten(t, output, "afk 3s", 500*time.Millisecond)
	waitWritten(t, output, "afk 2s", 1500*time.Millisecond)
	waitWritten(t, output, "afk 1s", 1500*time.Millisecond)
	// Status-only repaints must not reset the actual inactivity deadline.
	waitWritten(t, pty, afkTestPaste, 1500*time.Millisecond)
	pty.send(afkTestScreen(agent.KindCodex, "Should I continue?", afkTestMessage))
	waitWritten(t, pty, afkTestPaste+"\r", time.Second)
	if got := pty.written(); got != afkTestPaste+"\r" {
		t.Fatalf("countdown did not submit exactly once: %q", got)
	}
}

func TestProxy_AfkPrefixCommandResetsIdle(t *testing.T) {
	pr, pty, stdin := makeProxyWithAfk(t, 600*time.Millisecond, false, agent.KindCodex)
	done := runProxy(pr)
	defer func() { pty.close(); stdin.close(); <-done }()
	pty.send(afkTestScreen(agent.KindCodex, "Should I continue?", ""))
	time.Sleep(350 * time.Millisecond)
	stdin.send("\x19f")
	ensureNotWritten(t, pty, afkTestPaste, 400*time.Millisecond)
	waitWritten(t, pty, afkTestPaste, time.Second)
}
