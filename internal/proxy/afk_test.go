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
			"approval":          codexPrompt,
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
