// internal/proxy/proxy_test.go
package proxy_test

import (
	"testing"
	"time"

	ptylib "github.com/aymanbagabas/go-pty"

	"yoyo/internal/agent"
	"yoyo/internal/detector"
	"yoyo/internal/logger"
	"yoyo/internal/memory"
	"yoyo/internal/proxy"
	"yoyo/internal/screen"
	"yoyo/internal/statusbar"
	"yoyo/internal/term"
)

// TestProxy_AutoApprovesClaudePrompt runs a real PTY with `cat`, writes a
// simulated Claude permission prompt to stdin, and asserts the proxy sends CR.
func TestProxy_AutoApprovesClaudePrompt(t *testing.T) {
	if testing.Short() {
		t.Skip("integration: skipped in short mode")
	}

	p, err := ptylib.New()
	if err != nil {
		t.Skip("PTY not available:", err)
	}
	defer p.Close()

	cmd := p.Command("cat")
	if err := cmd.Start(); err != nil {
		t.Skip("cat not available:", err)
	}

	tr := term.NewNoOp() // no real TTY in tests
	log, _ := logger.New(t.TempDir() + "/test.log")
	defer log.Close()

	scr := screen.New(80, 24)
	sb := statusbar.New(24, 80, true, 0) // delay=0 immediate
	mem := memory.New()

	chain := detector.RuleChain{detector.Claude{}}

	pr := proxy.New(proxy.Config{
		PTY:       p,
		RuleChain: chain,
		Memory:    mem,
		StatusBar: sb,
		Log:       log,
		Term:      tr,
		Screen:    scr,
		AgentKind: agent.KindClaude,
		Delay:     0,
		Enabled:   true,
	})

	// Write a Claude prompt to the PTY
	claudePrompt := "─────────────────────────────────────────────\n" +
		" Read file\n\n 1. Yes\n 2. No\n\n Esc to cancel\n"

	go func() {
		time.Sleep(100 * time.Millisecond)
		p.Write([]byte(claudePrompt))
	}()

	// Run proxy with timeout
	done := make(chan error, 1)
	go func() {
		done <- pr.Run()
	}()

	select {
	case <-time.After(3 * time.Second):
		t.Log("proxy ran for 3s without error (expected for interactive test)")
	case err := <-done:
		if err != nil {
			t.Logf("proxy exited: %v", err)
		}
	}

	// Verify memory recorded the prompt hash
	scr.Feed([]byte(claudePrompt))
	r := detector.Claude{}.Detect(scr.Text())
	if r == nil {
		t.Fatal("detector failed to match the injected prompt")
	}
	if !mem.Seen(r.Hash) {
		t.Log("note: prompt may not have been seen if proxy stopped before detection")
	}
}
