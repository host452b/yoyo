// internal/proxy/proxy_test.go
package proxy_test

import (
	"io"
	"strings"
	"testing"
	"time"

	ptylib "github.com/aymanbagabas/go-pty"

	"github.com/host452b/yoyo/internal/agent"
	"github.com/host452b/yoyo/internal/detector"
	"github.com/host452b/yoyo/internal/logger"
	"github.com/host452b/yoyo/internal/memory"
	"github.com/host452b/yoyo/internal/proxy"
	"github.com/host452b/yoyo/internal/screen"
	"github.com/host452b/yoyo/internal/statusbar"
	"github.com/host452b/yoyo/internal/term"
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

	// Use a pipe for stdin so the proxy doesn't exit when os.Stdin reaches EOF.
	stdinR, stdinW := io.Pipe()
	defer stdinW.Close()

	scr := screen.New(80, 24)
	sb := statusbar.New(24, 80, true, 0) // delay=0 immediate
	mem := memory.New()

	chain := detector.RuleChain{detector.Claude{}}

	pr := proxy.New(proxy.Config{
		PTY:       p,
		Stdin:     stdinR,
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

	// Verify memory recorded the prompt hash.
	// Use a fresh screen fed with the CRLF version (PTY ONLCR converts \n→\r\n)
	// so the hash matches what the proxy computed from actual PTY output.
	freshScr := screen.New(80, 24)
	freshScr.Feed([]byte(strings.ReplaceAll(claudePrompt, "\n", "\r\n")))
	r := detector.Claude{}.Detect(freshScr.Text())
	if r == nil {
		t.Fatal("detector failed to match the injected prompt")
	}
	if !mem.Seen(r.Hash) {
		t.Error("proxy did not record the prompt hash — approval response was not sent")
	}
}
