// internal/proxy/proxy_kitty_e2e_test.go
//
// End-to-end scenario tests for the Kitty keyboard protocol path. Kitty-capable
// terminals (Ghostty, kitty, WezTerm, foot, …) encode Ctrl+Y as "\x1b[121;5u"
// instead of the legacy 0x19 byte once the child enables progressive
// enhancement. These tests drive the full Run() loop with Kitty-encoded input
// and assert the prefix state machine behaves identically to the legacy path.
//
// Reuses the fakePTY / fakeStdin / makeProxy* harness from proxy_e2e_test.go.
package proxy_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/host452b/yoyo/internal/agent"
)

// Kitty-encoded Ctrl+Y, with the command byte appended raw (Kitty sends
// unmodified printable keys as literal text, so the digit/letter that follows
// the prefix arrives unchanged).
const (
	kittyCtrlY = "\x1b[121;5u" // Ctrl+Y press
)

// K1. Kitty Ctrl+Y 0 toggles auto-approve off, then a second one re-enables it.
// Mirrors the legacy TestProxy_E2E_CtrlY_Toggle.
func TestProxy_E2E_Kitty_CtrlY_Toggle(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 0, true, nil)
	defer pty.close()

	done := runProxy(pr)

	// Disable via Kitty Ctrl+Y 0
	stdin.send(kittyCtrlY + "0")
	time.Sleep(50 * time.Millisecond)

	pty.send(claudePrompt)
	ensureNotWritten(t, pty, "\r", 300*time.Millisecond)

	// Re-enable via Kitty Ctrl+Y 0
	stdin.send(kittyCtrlY + "0")
	time.Sleep(50 * time.Millisecond)

	pty.send(claudePrompt)
	waitWritten(t, pty, "\r", 2*time.Second)

	stdin.close()
	<-done
}

// K2. Kitty Ctrl+Y 1 sets the delay to 1 second (and re-enables if disabled).
// Mirrors the legacy TestProxy_E2E_CtrlY_SetDelay.
func TestProxy_E2E_Kitty_CtrlY_SetDelay(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 0, false, nil)
	defer stdin.close()

	done := runProxy(pr)

	stdin.send(kittyCtrlY + "1")
	time.Sleep(50 * time.Millisecond)

	pty.send(claudePrompt)

	// delay is now 1s, so no immediate approval...
	ensureNotWritten(t, pty, "\r", 200*time.Millisecond)
	// ...but it fires within the window.
	waitWritten(t, pty, "\r", 2*time.Second)

	pty.close()
	<-done
}

// K3. The Kitty prefix sequence and its command byte arriving in SEPARATE
// stdin chunks must still be recognized. The normalizer converts the lone
// prefix chunk to 0x19, arming prefixActive; the following raw digit is then
// handled by the prefix state machine.
func TestProxy_E2E_Kitty_PrefixSplitAcrossChunks(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 0, false, nil)
	defer stdin.close()

	done := runProxy(pr)

	// Prefix in one chunk, the '1' command in the next.
	stdin.send(kittyCtrlY)
	time.Sleep(30 * time.Millisecond)
	stdin.send("1")
	time.Sleep(50 * time.Millisecond)

	pty.send(claudePrompt)

	ensureNotWritten(t, pty, "\r", 200*time.Millisecond)
	waitWritten(t, pty, "\r", 2*time.Second)

	pty.close()
	<-done
}

// K4. Kitty Ctrl+Y q triggers the force-kill callback. Confirms the inline
// a/f/q/d prefix path works under Kitty too (only the bare 3×Ctrl-C path,
// which keys off the raw 0x03 byte, remains legacy-only — see CHANGELOG).
func TestProxy_E2E_Kitty_CtrlYQ_ForceKill(t *testing.T) {
	var kills int64
	pr, pty, stdin := makeProxyWithKill(t, &kills)
	defer stdin.close()
	done := runProxy(pr)

	stdin.send(kittyCtrlY + "q")

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&kills) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt64(&kills); got != 1 {
		t.Errorf("expected 1 kill, got %d", got)
	}

	pty.close()
	<-done
}

// K5. A Kitty-encoded key that is NOT the prefix (here Ctrl+P, "\x1b[112;5u")
// must pass through untouched to the child PTY — the normalizer must not eat
// or rewrite unrelated keys.
func TestProxy_E2E_Kitty_NonPrefixKeyForwarded(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 0, true, nil)
	defer stdin.close()
	defer pty.close()

	done := runProxy(pr)

	const ctrlP = "\x1b[112;5u"
	stdin.send(ctrlP)
	waitWritten(t, pty, ctrlP, 1*time.Second)

	select {
	case err := <-done:
		t.Errorf("proxy exited unexpectedly: %v", err)
	default:
		// still running — pass
	}
}

// K6. A Kitty Ctrl+Y key-release event ("\x1b[121;5:3u") must be dropped, not
// treated as a prefix press. Sending only a release must not arm prefix mode;
// a subsequent press+command still works.
func TestProxy_E2E_Kitty_ReleaseEventIgnored(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 0, true, nil)
	defer pty.close()

	done := runProxy(pr)

	// Release event alone: must NOT arm the prefix (so the following '0'
	// is just a literal keystroke, not a toggle-off command).
	stdin.send("\x1b[121;5:3u")
	time.Sleep(50 * time.Millisecond)
	stdin.send("0")
	time.Sleep(50 * time.Millisecond)

	// Auto-approve must still be ON (never toggled off) → prompt approved.
	pty.send(claudePrompt)
	waitWritten(t, pty, "\r", 2*time.Second)

	stdin.close()
	<-done
}
