// proxy_sim_test.go — 10-action sequential simulation.
//
// Drives 10 distinct Claude Code permission dialogs through the proxy:
// 9 are auto-approved after the 1-second countdown; 1 is manually
// cancelled mid-countdown by a user keypress (tests the interrupt path).
//
// Run with:
//
//	go test -v -run TestProxy_E2E_10Actions_Simulation ./internal/proxy/
package proxy_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/host452b/yoyo/internal/agent"
)

// actionPrompt builds a minimal Claude Code dialog for the given command.
func actionPrompt(cmd string) string {
	return "─────────────────────────────────────────────\r\n" +
		" " + cmd + "\r\n\r\n 1. Yes\r\n 2. No\r\n\r\n Esc to cancel\r\n"
}

// approvalCount returns the number of auto-approval responses the proxy has
// written to the PTY so far. Each approval is "\r" (or "\x1b[B\r" for the
// 3-option variant, which also contains exactly one "\r"). The forwarded
// "q" keypress used for cancellation does not contain "\r".
func approvalCount(pty *fakePTY) int {
	return strings.Count(pty.written(), "\r")
}

func TestProxy_E2E_10Actions_Simulation(t *testing.T) {
	type step struct {
		cmd       string
		interrupt bool // true: user presses a key to cancel the countdown
	}
	steps := []step{
		{"Read /etc/hosts", false},
		{"Bash: pip install numpy", false},
		{"Write /tmp/result.json", false},
		{"Read ~/.ssh/config", false},
		{"Bash: git clone https://github.com/example/repo", false},
		{"Bash: rm -rf /tmp/build_cache  ← user cancels this one", true},
		{"Read /var/log/app.log", false},
		{"Bash: curl https://api.example.com/data", false},
		{"Write /etc/crontab", false},
		{"Bash: docker build -t myapp:latest .", false},
	}

	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 1, true, nil)
	defer stdin.close()
	done := runProxy(pr)

	// noDialog clears the screen (VT100 erase-display + cursor-home), forcing
	// result==nil which resets approvedHash and lastResult so the next prompt
	// starts a fresh countdown.
	const noDialog = "\x1b[2J\x1b[H"

	approved, interrupted := 0, 0

	for i, s := range steps {
		prev := approvalCount(pty)
		pty.send(actionPrompt(s.cmd))

		if s.interrupt {
			time.Sleep(200 * time.Millisecond) // let the timer register
			stdin.send("q")                    // any non-Ctrl+Y key cancels the countdown
			time.Sleep(100 * time.Millisecond)
			pty.send(noDialog)
			time.Sleep(100 * time.Millisecond)
			interrupted++
			t.Logf("action %2d/10  %-54s  INTERRUPTED", i+1,
				fmt.Sprintf("%q", s.cmd))
		} else {
			deadline := time.Now().Add(3 * time.Second)
			ok := false
			for time.Now().Before(deadline) {
				if approvalCount(pty) > prev {
					ok = true
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			pty.send(noDialog)
			time.Sleep(100 * time.Millisecond)
			if ok {
				approved++
				t.Logf("action %2d/10  %-54s  APPROVED", i+1,
					fmt.Sprintf("%q", s.cmd))
			} else {
				t.Errorf("action %2d/10  %-54s  TIMEOUT", i+1,
					fmt.Sprintf("%q", s.cmd))
			}
		}
	}

	t.Logf("──────────────────────────────────────────────────────────────────")
	t.Logf("result: %d/10 auto-approved, %d/10 interrupted by user", approved, interrupted)
	if approved != 9 || interrupted != 1 {
		t.Errorf("expected 9 approved + 1 interrupted, got %d + %d", approved, interrupted)
	}

	pty.close()
	<-done
}
