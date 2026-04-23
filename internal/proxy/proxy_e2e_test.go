// internal/proxy/proxy_e2e_test.go
//
// End-to-end scenario tests for the proxy event loop.
// Uses fake PTY and stdin so no real TTY is required.
package proxy_test

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/host452b/yoyo/internal/agent"
	"github.com/host452b/yoyo/internal/detector"
	"github.com/host452b/yoyo/internal/logger"
	"github.com/host452b/yoyo/internal/memory"
	"github.com/host452b/yoyo/internal/proxy"
	"github.com/host452b/yoyo/internal/screen"
	"github.com/host452b/yoyo/internal/statusbar"
	"github.com/host452b/yoyo/internal/term"
)

// ── test doubles ────────────────────────────────────────────────────────────

// fakePTY simulates the child process's PTY.
// Tests inject agent output via send(); proxy writes (approvals) are captured.
type fakePTY struct {
	out chan []byte
	mu  sync.Mutex
	buf []byte
}

func newFakePTY() *fakePTY { return &fakePTY{out: make(chan []byte, 64)} }

func (f *fakePTY) Read(b []byte) (int, error) {
	data, ok := <-f.out
	if !ok {
		return 0, io.EOF
	}
	n := copy(b, data)
	return n, nil
}

func (f *fakePTY) Write(b []byte) (int, error) {
	f.mu.Lock()
	f.buf = append(f.buf, b...)
	f.mu.Unlock()
	return len(b), nil
}

func (f *fakePTY) send(data string) { f.out <- []byte(data) }
func (f *fakePTY) close()           { close(f.out) }
func (f *fakePTY) written() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return string(f.buf)
}

// fakeStdin injects keystrokes into the proxy's stdin path.
type fakeStdin struct{ ch chan []byte }

func newFakeStdin() *fakeStdin          { return &fakeStdin{ch: make(chan []byte, 64)} }
func (f *fakeStdin) Read(b []byte) (int, error) {
	data, ok := <-f.ch
	if !ok {
		return 0, io.EOF
	}
	n := copy(b, data)
	return n, nil
}
func (f *fakeStdin) send(data string) { f.ch <- []byte(data) }
func (f *fakeStdin) close()           { close(f.ch) }

// ── helpers ──────────────────────────────────────────────────────────────────

// makeProxy wires up a proxy using fake dependencies.
// Returns (proxy, memory, fakePTY, fakeStdin).
func makeProxy(t *testing.T, kind agent.Kind, delay int, enabled bool, chain detector.RuleChain) (*proxy.Proxy, *memory.Memory, *fakePTY, *fakeStdin) {
	t.Helper()
	log, err := logger.New(t.TempDir() + "/test.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })

	pty := newFakePTY()
	stdin := newFakeStdin()
	mem := memory.New()
	scr := screen.New(80, 24)
	sb := statusbar.New(24, 80, enabled, delay)

	if chain == nil {
		chain = detector.RuleChain{kind.Detector()}
	}

	pr := proxy.New(proxy.Config{
		PTY:       pty,
		Stdin:     stdin,
		Stdout:    io.Discard,
		RuleChain: chain,
		Memory:    mem,
		StatusBar: sb,
		Log:       log,
		Term:      term.NewNoOp(),
		Screen:    scr,
		AgentKind: kind,
		Delay:     delay,
		Enabled:   enabled,
	})
	return pr, mem, pty, stdin
}

// runProxy starts proxy.Run in a goroutine and returns a done channel.
func runProxy(pr *proxy.Proxy) <-chan error {
	ch := make(chan error, 1)
	go func() { ch <- pr.Run() }()
	return ch
}

// waitWritten polls pty.written() until it contains want or timeout expires.
func waitWritten(t *testing.T, pty *fakePTY, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(pty.written(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("timeout waiting for PTY write %q; got %q", want, pty.written())
}

// ensureNotWritten waits for a short period and asserts want is NOT in written.
func ensureNotWritten(t *testing.T, pty *fakePTY, want string, wait time.Duration) {
	t.Helper()
	time.Sleep(wait)
	if strings.Contains(pty.written(), want) {
		t.Errorf("expected PTY write %q to be absent; got %q", want, pty.written())
	}
}

// Claude, Codex, Cursor sample prompts ──────────────────────────────────────

// Prompts use \r\n (CRLF) to mirror real PTY output, where the terminal
// driver's ONLCR setting converts \n → \r\n before bytes reach the screen buffer.
const claudePrompt = "─────────────────────────────────────────────\r\n" +
	" Read /etc/hosts\r\n\r\n 1. Yes\r\n 2. No\r\n\r\n Esc to cancel\r\n"

const codexPrompt = "This command needs your approval\r\n" +
	"  rm -rf /tmp/test\r\n" +
	"Press enter to confirm or esc to cancel\r\n"

const cursorPrompt = "┌──────────────────────────────────────┐\r\n" +
	"│ Run shell command: ls -la            │\r\n" +
	"│ (y) yes   n) no                      │\r\n" +
	"└──────────────────────────────────────┘\r\n"

// ── scenario tests ───────────────────────────────────────────────────────────

// 1. Immediate approval — Claude
func TestProxy_E2E_ImmediateApproval_Claude(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 0, true, nil)
	defer stdin.close()

	done := runProxy(pr)
	pty.send(claudePrompt)
	waitWritten(t, pty, "\r", 2*time.Second)

	pty.close()
	<-done
}

// 2. Immediate approval — Codex
func TestProxy_E2E_ImmediateApproval_Codex(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindCodex, 0, true, nil)
	defer stdin.close()

	done := runProxy(pr)
	pty.send(codexPrompt)
	waitWritten(t, pty, "\r", 2*time.Second)

	pty.close()
	<-done
}

// 3. Immediate approval — Cursor
func TestProxy_E2E_ImmediateApproval_Cursor(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindCursor, 0, true, nil)
	defer stdin.close()

	done := runProxy(pr)
	pty.send(cursorPrompt)
	waitWritten(t, pty, "\r", 2*time.Second)

	pty.close()
	<-done
}

// 4. Delayed approval fires after the configured delay
func TestProxy_E2E_DelayedApproval(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 1, true, nil)
	defer stdin.close()

	done := runProxy(pr)
	pty.send(claudePrompt)

	// Should NOT be approved immediately
	ensureNotWritten(t, pty, "\r", 200*time.Millisecond)

	// Should be approved within delay + buffer
	waitWritten(t, pty, "\r", 2*time.Second)

	pty.close()
	<-done
}

// 5. User keypress during countdown cancels pending approval
func TestProxy_E2E_CancelOnKeypress(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 2, true, nil)
	defer pty.close()

	done := runProxy(pr)
	pty.send(claudePrompt)
	time.Sleep(100 * time.Millisecond) // let timer start

	// Send a real key (not an escape sequence) to cancel
	stdin.send("x")

	// After 2.5s the timer would have fired if not cancelled
	ensureNotWritten(t, pty, "\r", 2500*time.Millisecond)

	stdin.close()
	<-done
}

// 6. Session memory: same prompt approved immediately on second occurrence
func TestProxy_E2E_SessionMemory(t *testing.T) {
	pr, mem, pty, stdin := makeProxy(t, agent.KindClaude, 0, true, nil)
	defer stdin.close()

	done := runProxy(pr)

	// First occurrence: approved immediately, hash recorded
	pty.send(claudePrompt)
	waitWritten(t, pty, "\r", 2*time.Second)

	// Verify hash is in memory
	scr := screen.New(80, 24)
	scr.Feed([]byte(claudePrompt))
	r := detector.Claude{}.Detect(scr.Text())
	if r == nil {
		t.Fatal("detector failed to match claudePrompt")
	}
	if !mem.Seen(r.Hash) {
		t.Error("hash not recorded in memory after first approval")
	}

	// Simulate child redraw: clear screen so the prompt disappears (resets debounce)
	pty.send("\x1b[2J\x1b[H")
	time.Sleep(50 * time.Millisecond)

	// Second occurrence of the same prompt: should be approved immediately via seen path
	pty.send(claudePrompt)
	waitWritten(t, pty, "\r\r", 2*time.Second) // two CRs total

	pty.close()
	<-done
}

// 7. Disabled proxy does not approve prompts
func TestProxy_E2E_Disabled_NoApproval(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 0, false, nil)
	defer pty.close()

	done := runProxy(pr)
	pty.send(claudePrompt)

	ensureNotWritten(t, pty, "\r", 500*time.Millisecond)

	stdin.close()
	<-done
}

// 8. Ctrl+Y 0 toggles auto-approve off; second Ctrl+Y 0 re-enables it
func TestProxy_E2E_CtrlY_Toggle(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 0, true, nil)
	defer pty.close()

	done := runProxy(pr)

	// Disable via Ctrl+Y 0
	stdin.send("\x190")
	time.Sleep(50 * time.Millisecond)

	// Prompt should NOT be approved while disabled
	pty.send(claudePrompt)
	ensureNotWritten(t, pty, "\r", 300*time.Millisecond)

	// Re-enable via Ctrl+Y 0
	stdin.send("\x190")
	time.Sleep(50 * time.Millisecond)

	// Prompt should now be approved
	pty.send(claudePrompt)
	waitWritten(t, pty, "\r", 2*time.Second)

	stdin.close()
	<-done
}

// 9. Ctrl+Y 1 sets delay to 1 second (and re-enables if disabled)
func TestProxy_E2E_CtrlY_SetDelay(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 0, false, nil)
	defer stdin.close()

	done := runProxy(pr)

	// Set delay=1 and re-enable via Ctrl+Y 1
	stdin.send("\x191")
	time.Sleep(50 * time.Millisecond)

	pty.send(claudePrompt)

	// Should NOT approve immediately (delay=1s now)
	ensureNotWritten(t, pty, "\r", 200*time.Millisecond)

	// Should approve after ~1s
	waitWritten(t, pty, "\r", 2*time.Second)

	pty.close()
	<-done
}

// 10. PTY exits while approval timer is pending — no crash, clean exit
func TestProxy_E2E_TimerStoppedOnPTYExit(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 10, true, nil)
	defer stdin.close()

	done := runProxy(pr)

	// Trigger the 10-second timer
	pty.send(claudePrompt)
	time.Sleep(100 * time.Millisecond) // let timer arm

	// PTY exits before timer fires
	pty.close()

	select {
	case <-done:
		// clean exit — pass
	case <-time.After(2 * time.Second):
		t.Error("proxy did not exit after PTY closed")
	}
}

// 11. Custom regexp rule fires and sends configured response
func TestProxy_E2E_CustomRule(t *testing.T) {
	d, err := detector.NewRegexpDetector("deploy-confirm", `Continue\? \[y/N\]`, "y\r")
	if err != nil {
		t.Fatal(err)
	}
	chain := detector.RuleChain{d}

	pr, _, pty, stdin := makeProxy(t, agent.KindUnknown, 0, true, chain)
	defer stdin.close()

	done := runProxy(pr)
	pty.send("Deploying to production...\nContinue? [y/N] ")
	waitWritten(t, pty, "y\r", 2*time.Second)

	pty.close()
	<-done
}

// 12. Unknown agent: auto-detected from screen content (Claude Code banner)
func TestProxy_E2E_UnknownAgent_AutoDetect(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindUnknown, 0, true, nil)
	defer stdin.close()

	done := runProxy(pr)

	// Frame 1: banner identifies agent
	pty.send("Claude Code v1.2.3 - AI assistant\n")
	time.Sleep(50 * time.Millisecond)

	// Frame 2: permission prompt
	pty.send(claudePrompt)
	waitWritten(t, pty, "\r", 2*time.Second)

	pty.close()
	<-done
}

// 13. Prefix key timeout: Ctrl+Y not followed by command → forwarded to PTY
func TestProxy_E2E_PrefixKeyTimeout(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 0, true, nil)
	defer stdin.close()
	defer pty.close()

	done := runProxy(pr)

	// Send Ctrl+Y alone and wait for prefix timeout (1500ms)
	stdin.send("\x19")
	waitWritten(t, pty, "\x19", 2500*time.Millisecond) // 0x19 forwarded after timeout

	// Proxy should still be running
	select {
	case err := <-done:
		t.Errorf("proxy exited unexpectedly: %v", err)
	default:
		// still running — pass
	}
}

// ── tmux-related tests ───────────────────────────────────────────────────────

// 15. Prompt detection at non-80×24 screen sizes (tmux panes vary)
func TestProxy_E2E_NonStandardScreenSizes(t *testing.T) {
	sizes := []struct {
		cols, rows int
	}{
		{120, 40},  // wide tmux pane
		{200, 50},  // ultra-wide monitor
		{60, 20},   // narrow tmux split
		{132, 43},  // classic VT132
		{40, 15},   // very narrow pane
	}
	for _, sz := range sizes {
		name := fmt.Sprintf("%dx%d", sz.cols, sz.rows)
		t.Run(name, func(t *testing.T) {
			log, err := logger.New(t.TempDir() + "/test.log")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { log.Close() })

			pty := newFakePTY()
			stdin := newFakeStdin()
			scr := screen.New(sz.cols, sz.rows)
			sb := statusbar.New(uint16(sz.rows), uint16(sz.cols), true, 0)
			chain := detector.RuleChain{agent.KindClaude.Detector()}

			pr := proxy.New(proxy.Config{
				PTY:       pty,
				Stdin:     stdin,
				Stdout:    io.Discard,
				RuleChain: chain,
				Memory:    memory.New(),
				StatusBar: sb,
				Log:       log,
				Term:      term.NewNoOp(),
				Screen:    scr,
				AgentKind: agent.KindClaude,
				Delay:     0,
				Enabled:   true,
			})

			done := runProxy(pr)
			pty.send(claudePrompt)
			waitWritten(t, pty, "\r", 2*time.Second)

			stdin.close()
			pty.close()
			<-done
		})
	}
}

// 16. tmux focus events (ESC [ I / ESC [ O) must NOT cancel pending approval
func TestProxy_E2E_FocusEventsPreserveApproval(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 2, true, nil)
	defer pty.close()

	done := runProxy(pr)
	pty.send(claudePrompt)
	time.Sleep(100 * time.Millisecond) // let timer start

	// Send tmux focus-out and focus-in events
	stdin.send("\x1b[O") // focus out
	time.Sleep(50 * time.Millisecond)
	stdin.send("\x1b[I") // focus in
	time.Sleep(50 * time.Millisecond)

	// Timer should NOT have been cancelled — approval fires after delay
	waitWritten(t, pty, "\r", 3*time.Second)

	stdin.close()
	<-done
}

// 14. Unrecognised Ctrl+Y command: prefix + unknown byte forwarded to PTY
func TestProxy_E2E_PrefixKeyUnknown(t *testing.T) {
	pr, _, pty, stdin := makeProxy(t, agent.KindClaude, 0, true, nil)
	defer stdin.close()
	defer pty.close()

	done := runProxy(pr)

	// Ctrl+Y followed by '9' (not a valid command)
	stdin.send("\x199")
	waitWritten(t, pty, "9", 500*time.Millisecond)

	select {
	case err := <-done:
		t.Errorf("proxy exited unexpectedly: %v", err)
	default:
		// still running — pass
	}
}

// makeProxyWithAfk wires up a proxy with AFK enabled and a short idle.
func makeProxyWithAfk(t *testing.T, idle time.Duration, dryRun bool) (*proxy.Proxy, *fakePTY, *fakeStdin) {
	t.Helper()
	log, err := logger.New(t.TempDir() + "/test.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	pty := newFakePTY()
	stdin := newFakeStdin()
	scr := screen.New(80, 24)
	sb := statusbar.New(24, 80, true, 0)
	chain := detector.RuleChain{agent.KindClaude.Detector()}
	pr := proxy.New(proxy.Config{
		PTY:        pty,
		Stdin:      stdin,
		Stdout:     io.Discard,
		RuleChain:  chain,
		Memory:     memory.New(),
		StatusBar:  sb,
		Log:        log,
		Term:       term.NewNoOp(),
		Screen:     scr,
		AgentKind:  agent.KindClaude,
		Delay:      0,
		Enabled:    true,
		DryRun:     dryRun,
		AfkEnabled: true,
		AfkIdle:    idle,
	})
	return pr, pty, stdin
}

// 17. AFK fires after idle with y\r + continue message
func TestProxy_E2E_AfkFires(t *testing.T) {
	pr, pty, stdin := makeProxyWithAfk(t, 300*time.Millisecond, false)
	defer stdin.close()
	done := runProxy(pr)

	waitWritten(t, pty, "y\r", 1*time.Second)
	waitWritten(t, pty, "continue, Choose based on your project understanding.\r", 1*time.Second)

	pty.close()
	<-done
}

// 18. AFK rearms and fires a second time while still idle
func TestProxy_E2E_AfkRearmsAndFiresTwice(t *testing.T) {
	pr, pty, stdin := makeProxyWithAfk(t, 300*time.Millisecond, false)
	defer stdin.close()
	done := runProxy(pr)

	waitWritten(t, pty, "continue, Choose based on your project understanding.\r", 1*time.Second)

	// Count how many "continue, ..." strings appear after the second idle window
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Count(pty.written(), "continue, Choose based on your project understanding.\r") >= 2 {
			pty.close()
			<-done
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("expected 2 AFK fires; got written=%q", pty.written())
	pty.close()
	<-done
}

// 19. Output from child during idle window keeps resetting the AFK timer
func TestProxy_E2E_AfkResetOnOutput(t *testing.T) {
	pr, pty, stdin := makeProxyWithAfk(t, 300*time.Millisecond, false)
	defer stdin.close()
	done := runProxy(pr)

	// Keep pumping output every 100 ms for ~600 ms (>2× the idle window).
	// A WaitGroup ensures the sender goroutine has fully exited before we
	// close the channel it was sending on — otherwise close races with send.
	stop := make(chan struct{})
	var senderWg sync.WaitGroup
	senderWg.Add(1)
	go func() {
		defer senderWg.Done()
		tk := time.NewTicker(100 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
				pty.send(".")
			}
		}
	}()

	ensureNotWritten(t, pty, "y\r", 600*time.Millisecond)
	close(stop)
	senderWg.Wait()
	pty.close()
	<-done
}

// 20. User stdin activity during idle window keeps resetting the AFK timer
func TestProxy_E2E_AfkResetOnUserInput(t *testing.T) {
	pr, pty, stdin := makeProxyWithAfk(t, 300*time.Millisecond, false)
	done := runProxy(pr)

	stop := make(chan struct{})
	var senderWg sync.WaitGroup
	senderWg.Add(1)
	go func() {
		defer senderWg.Done()
		tk := time.NewTicker(100 * time.Millisecond)
		defer tk.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tk.C:
				stdin.send("x")
			}
		}
	}()

	ensureNotWritten(t, pty, "y\r", 600*time.Millisecond)
	close(stop)
	senderWg.Wait()
	stdin.close()
	pty.close()
	<-done
}

// 21. AFK in dry-run mode must log the intent but NOT write to the PTY
func TestProxy_E2E_AfkDryRun(t *testing.T) {
	pr, pty, stdin := makeProxyWithAfk(t, 200*time.Millisecond, true /*dryRun*/)
	defer stdin.close()
	done := runProxy(pr)

	// Wait long enough for a real fire to have happened
	time.Sleep(500 * time.Millisecond)

	if strings.Contains(pty.written(), "y\r") ||
		strings.Contains(pty.written(), "continue, Choose") {
		t.Errorf("dry-run wrote to PTY: %q", pty.written())
	}

	pty.close()
	<-done
}

// 22. Ctrl+Y a disables AFK at runtime; second Ctrl+Y a re-enables it
func TestProxy_E2E_AfkToggleViaPrefix(t *testing.T) {
	pr, pty, stdin := makeProxyWithAfk(t, 250*time.Millisecond, false)
	defer stdin.close()
	done := runProxy(pr)

	// Toggle OFF before any fire
	stdin.send("\x19a")
	time.Sleep(50 * time.Millisecond)

	// No fire expected for >2× idle duration
	ensureNotWritten(t, pty, "y\r", 700*time.Millisecond)

	// Toggle ON — fire should occur within one idle window
	stdin.send("\x19a")
	waitWritten(t, pty, "y\r", 1*time.Second)

	pty.close()
	<-done
}

// makeProxyWithFuzzy wires up a proxy with fuzzy enabled and a short stable window.
func makeProxyWithFuzzy(t *testing.T, stable time.Duration, dryRun bool) (*proxy.Proxy, *fakePTY, *fakeStdin) {
	t.Helper()
	log, err := logger.New(t.TempDir() + "/test.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	pty := newFakePTY()
	stdin := newFakeStdin()
	scr := screen.New(80, 24)
	sb := statusbar.New(24, 80, true, 0)
	chain := detector.RuleChain{agent.KindClaude.Detector()} // claude detector won't match our test prompts
	pr := proxy.New(proxy.Config{
		PTY:          pty,
		Stdin:        stdin,
		Stdout:       io.Discard,
		RuleChain:    chain,
		Memory:       memory.New(),
		StatusBar:    sb,
		Log:          log,
		Term:         term.NewNoOp(),
		Screen:       scr,
		AgentKind:    agent.KindClaude,
		Delay:        0,
		Enabled:      true,
		DryRun:       dryRun,
		FuzzyEnabled: true,
		FuzzyStable:  stable,
	})
	return pr, pty, stdin
}

// 23. Fuzzy fires after stable window when last 15 lines match the vocab
func TestProxy_E2E_FuzzyFires(t *testing.T) {
	pr, pty, stdin := makeProxyWithFuzzy(t, 200*time.Millisecond, false)
	defer stdin.close()
	done := runProxy(pr)

	// Send a prompt that the built-in detectors will NOT match but fuzzy will.
	pty.send("deploying to prod\r\ncontinue? (y/n) ")

	// Fuzzy stable window is 200 ms; approval delay is 0; expect \r within 1 s.
	waitWritten(t, pty, "\r", 1*time.Second)

	pty.close()
	<-done
}

// 24. Fuzzy does NOT fire when no vocab marker is present
func TestProxy_E2E_FuzzyNoMatch(t *testing.T) {
	pr, pty, stdin := makeProxyWithFuzzy(t, 150*time.Millisecond, false)
	defer stdin.close()
	done := runProxy(pr)

	pty.send("building... done\r\nwaiting... ")
	ensureNotWritten(t, pty, "\r", 400*time.Millisecond)

	pty.close()
	<-done
}

// 25. Fuzzy in dry-run does not write to PTY
func TestProxy_E2E_FuzzyDryRun(t *testing.T) {
	pr, pty, stdin := makeProxyWithFuzzy(t, 150*time.Millisecond, true)
	defer stdin.close()
	done := runProxy(pr)

	pty.send("continue (y/n) ")
	time.Sleep(400 * time.Millisecond)

	if len(pty.written()) != 0 {
		t.Errorf("dry-run wrote to PTY: %q", pty.written())
	}

	pty.close()
	<-done
}

// 26. Ctrl+Y f disables fuzzy at runtime; second Ctrl+Y f re-enables it
func TestProxy_E2E_FuzzyToggleViaPrefix(t *testing.T) {
	pr, pty, stdin := makeProxyWithFuzzy(t, 150*time.Millisecond, false)
	defer stdin.close()
	done := runProxy(pr)

	// Toggle fuzzy OFF
	stdin.send("\x19f")
	time.Sleep(50 * time.Millisecond)

	// Send a y/n prompt — fuzzy would normally fire, but toggled off → no \r
	pty.send("continue (y/n) ")
	ensureNotWritten(t, pty, "\r", 400*time.Millisecond)

	// Toggle fuzzy back ON — should now fire within a stable window
	stdin.send("\x19f")
	waitWritten(t, pty, "\r", 1*time.Second)

	pty.close()
	<-done
}

// ── Rigorous edge-case tests (v2.1.0 hardening) ─────────────────────────────

// 27. Toggling AFK OFF mid-countdown cancels the pending fire, not just
//     future ones. Confirms stopAfk() actually stops the armed timer.
func TestProxy_E2E_AfkToggleOffCancelsPendingFire(t *testing.T) {
	pr, pty, stdin := makeProxyWithAfk(t, 250*time.Millisecond, false)
	defer stdin.close()
	done := runProxy(pr)

	// Let the idle timer arm and start counting down.
	time.Sleep(100 * time.Millisecond)

	// Disable AFK before the 250ms window elapses.
	stdin.send("\x19a")

	// Wait longer than the original idle window would have needed; no fire.
	ensureNotWritten(t, pty, "y\r", 600*time.Millisecond)

	pty.close()
	<-done
}

// 28. Fuzzy stability timer must reset whenever the screen hash changes
//     within the stable window. A flicker of new output right before the
//     window expires should push the match to the next stable window.
func TestProxy_E2E_FuzzyStabilityResetsOnChange(t *testing.T) {
	pr, pty, stdin := makeProxyWithFuzzy(t, 250*time.Millisecond, false)
	defer stdin.close()
	done := runProxy(pr)

	// Prime the screen with a y/n prompt.
	pty.send("continue (y/n) ")

	// After 150ms — still inside the stable window — flicker new content.
	time.Sleep(150 * time.Millisecond)
	pty.send(".")

	// The flicker pushes the stable window by another 250ms.
	// At t=150+100=250ms total, no match should have fired yet.
	ensureNotWritten(t, pty, "\r", 100*time.Millisecond)

	// Once the new stable window elapses (250ms after the flicker),
	// fuzzy matches and sends \r.
	waitWritten(t, pty, "\r", 400*time.Millisecond)

	pty.close()
	<-done
}

// 29. Fuzzy matches must respect -delay like any other detector. A match
//     with Delay=1 must NOT fire immediately; it must wait ~1 second.
func TestProxy_E2E_FuzzyRespectsDelay(t *testing.T) {
	log, err := logger.New(t.TempDir() + "/test.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	pty := newFakePTY()
	stdin := newFakeStdin()
	scr := screen.New(80, 24)
	sb := statusbar.New(24, 80, true, 1)
	chain := detector.RuleChain{agent.KindClaude.Detector()}
	pr := proxy.New(proxy.Config{
		PTY:          pty,
		Stdin:        stdin,
		Stdout:       io.Discard,
		RuleChain:    chain,
		Memory:       memory.New(),
		StatusBar:    sb,
		Log:          log,
		Term:         term.NewNoOp(),
		Screen:       scr,
		AgentKind:    agent.KindClaude,
		Delay:        1, // 1-second approval delay
		Enabled:      true,
		FuzzyEnabled: true,
		FuzzyStable:  150 * time.Millisecond,
	})
	defer stdin.close()
	done := runProxy(pr)

	pty.send("continue (y/n) ")

	// Stability window (150ms) + delay (1s) ≈ 1.15s before fire.
	// 500ms after the send, nothing should be written yet.
	ensureNotWritten(t, pty, "\r", 500*time.Millisecond)

	// By ~1.5s the delayed approval should have fired.
	waitWritten(t, pty, "\r", 1500*time.Millisecond)

	pty.close()
	<-done
}

// 30. When a specific detector matches, fuzzy MUST NOT also fire. The
//     specific detector owns the approval; fuzzy's stability timer either
//     never arms (screen kept changing) or arms but finds the hash already
//     approved via approvedHash dedup.
func TestProxy_E2E_SpecificDetectorWinsOverFuzzy(t *testing.T) {
	// Enable both fuzzy and the Claude detector. Send a Claude prompt that
	// ALSO contains a fuzzy-compatible "(y/n)" snippet — we want exactly
	// one \r, not two.
	log, err := logger.New(t.TempDir() + "/test.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	pty := newFakePTY()
	stdin := newFakeStdin()
	scr := screen.New(80, 24)
	sb := statusbar.New(24, 80, true, 0)
	chain := detector.RuleChain{agent.KindClaude.Detector()}
	pr := proxy.New(proxy.Config{
		PTY:          pty,
		Stdin:        stdin,
		Stdout:       io.Discard,
		RuleChain:    chain,
		Memory:       memory.New(),
		StatusBar:    sb,
		Log:          log,
		Term:         term.NewNoOp(),
		Screen:       scr,
		AgentKind:    agent.KindClaude,
		Delay:        0,
		Enabled:      true,
		FuzzyEnabled: true,
		FuzzyStable:  100 * time.Millisecond,
	})
	defer stdin.close()
	done := runProxy(pr)

	// A prompt that BOTH the Claude detector and fuzzy would match.
	pty.send(claudePrompt + "\r\ncontinue (y/n) ")

	// Claude approves immediately. Give fuzzy ample time to (wrongly) also fire.
	time.Sleep(500 * time.Millisecond)

	if got := strings.Count(pty.written(), "\r"); got != 1 {
		t.Errorf("expected exactly 1 approval write; got %d (%q)", got, pty.written())
	}

	pty.close()
	<-done
}

// ── Safety guard tests (v2.2.0 hardening) ───────────────────────────────────

// makeProxyWithSafety wires up a proxy with Safety enabled and either
// auto-approve or fuzzy/afk as specified. The caller also controls
// whether -no-safety was passed (safetyEnabled flag).
func makeProxyWithSafety(t *testing.T, safetyEnabled bool) (*proxy.Proxy, *fakePTY, *fakeStdin) {
	t.Helper()
	log, err := logger.New(t.TempDir() + "/test.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	pty := newFakePTY()
	stdin := newFakeStdin()
	scr := screen.New(80, 24)
	sb := statusbar.New(24, 80, true, 0)
	chain := detector.RuleChain{agent.KindClaude.Detector()}
	pr := proxy.New(proxy.Config{
		PTY:           pty,
		Stdin:         stdin,
		Stdout:        io.Discard,
		RuleChain:     chain,
		Memory:        memory.New(),
		StatusBar:     sb,
		Log:           log,
		Term:          term.NewNoOp(),
		Screen:        scr,
		AgentKind:     agent.KindClaude,
		Delay:         0,
		Enabled:       true,
		SafetyEnabled: safetyEnabled,
	})
	return pr, pty, stdin
}

// A Claude-style prompt whose body contains rm -rf /.
const claudeDeletePrompt = "─────────────────────────────────────────────\r\n" +
	" Run: rm -rf /tmp/work-area\r\n" +
	" Then run: rm -rf /\r\n\r\n 1. Yes\r\n 2. No\r\n\r\n Esc to cancel\r\n"

// 31. Safety ON: specific detector refuses to auto-approve when the
//     prompt contains a deletion-class command.
func TestProxy_E2E_SafetyBlocksClaudeApprovalWithDanger(t *testing.T) {
	pr, pty, stdin := makeProxyWithSafety(t, true)
	defer stdin.close()
	done := runProxy(pr)

	pty.send(claudeDeletePrompt)

	// Claude detector would normally approve immediately. With safety on,
	// no \r should land on the PTY.
	ensureNotWritten(t, pty, "\r", 500*time.Millisecond)

	pty.close()
	<-done
}

// 32. Safety OFF (-no-safety): same prompt approves as usual. Proves the
//     opt-out actually opts out, and that the danger content alone doesn't
//     block approval without the flag.
func TestProxy_E2E_NoSafetyFlagPermitsApproval(t *testing.T) {
	pr, pty, stdin := makeProxyWithSafety(t, false)
	defer stdin.close()
	done := runProxy(pr)

	pty.send(claudeDeletePrompt)
	waitWritten(t, pty, "\r", 1*time.Second)

	pty.close()
	<-done
}

// ── Force-kill escape hatch tests (v2.2.1) ──────────────────────────────────

// makeProxyWithKill sets up a proxy whose Kill callback records how
// many times it was invoked (via an atomic counter).
func makeProxyWithKill(t *testing.T, counter *int64) (*proxy.Proxy, *fakePTY, *fakeStdin) {
	t.Helper()
	log, err := logger.New(t.TempDir() + "/test.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	pty := newFakePTY()
	stdin := newFakeStdin()
	scr := screen.New(80, 24)
	sb := statusbar.New(24, 80, true, 0)
	chain := detector.RuleChain{agent.KindClaude.Detector()}
	pr := proxy.New(proxy.Config{
		PTY:       pty,
		Stdin:     stdin,
		Stdout:    io.Discard,
		RuleChain: chain,
		Memory:    memory.New(),
		StatusBar: sb,
		Log:       log,
		Term:      term.NewNoOp(),
		Screen:    scr,
		AgentKind: agent.KindClaude,
		Delay:     0,
		Enabled:   true,
		Kill:      func() { atomic.AddInt64(counter, 1) },
	})
	return pr, pty, stdin
}

// 34a. Ctrl+Y d triggers the diagnostic-dump callback and surfaces the
//      returned path on the status bar.
func TestProxy_E2E_CtrlYD_DumpCallback(t *testing.T) {
	log, err := logger.New(t.TempDir() + "/test.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	pty := newFakePTY()
	stdin := newFakeStdin()
	sb := statusbar.New(24, 80, true, 0)

	var dumpCalls int64
	pr := proxy.New(proxy.Config{
		PTY:       pty,
		Stdin:     stdin,
		Stdout:    io.Discard,
		RuleChain: detector.RuleChain{agent.KindClaude.Detector()},
		Memory:    memory.New(),
		StatusBar: sb,
		Log:       log,
		Term:      term.NewNoOp(),
		Screen:    screen.New(80, 24),
		AgentKind: agent.KindClaude,
		Enabled:   true,
		Dump: func() (string, error) {
			atomic.AddInt64(&dumpCalls, 1)
			return "/tmp/yoyo-dump-test.md", nil
		},
	})
	defer stdin.close()
	done := runProxy(pr)

	stdin.send("\x19d")

	// Poll for the dump to land.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&dumpCalls) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt64(&dumpCalls); got != 1 {
		t.Errorf("expected 1 dump call, got %d", got)
	}

	pty.close()
	<-done
}

// 34b. If Dump returns an error, the proxy keeps running and surfaces
//      the failure on the status bar rather than crashing.
func TestProxy_E2E_CtrlYD_DumpErrorIsNonFatal(t *testing.T) {
	log, err := logger.New(t.TempDir() + "/test.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	pty := newFakePTY()
	stdin := newFakeStdin()
	pr := proxy.New(proxy.Config{
		PTY:       pty,
		Stdin:     stdin,
		Stdout:    io.Discard,
		RuleChain: detector.RuleChain{agent.KindClaude.Detector()},
		Memory:    memory.New(),
		StatusBar: statusbar.New(24, 80, true, 0),
		Log:       log,
		Term:      term.NewNoOp(),
		Screen:    screen.New(80, 24),
		AgentKind: agent.KindClaude,
		Enabled:   true,
		Dump: func() (string, error) {
			return "", fmt.Errorf("simulated disk full")
		},
	})
	defer stdin.close()
	done := runProxy(pr)

	stdin.send("\x19d")
	time.Sleep(100 * time.Millisecond)

	// Proxy must still be running.
	select {
	case err := <-done:
		t.Fatalf("proxy exited unexpectedly after failing dump: %v", err)
	default:
	}

	pty.close()
	<-done
}

// 34. Ctrl+Y q triggers the force-kill callback.
func TestProxy_E2E_CtrlYQ_ForceKill(t *testing.T) {
	var kills int64
	pr, pty, stdin := makeProxyWithKill(t, &kills)
	defer stdin.close()
	done := runProxy(pr)

	stdin.send("\x19q")

	// Poll briefly for the kill to land.
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

// 35. Three Ctrl-C within 500 ms triggers force-kill (muscle-memory path).
func TestProxy_E2E_TripleCtrlC_ForceKill(t *testing.T) {
	var kills int64
	pr, pty, stdin := makeProxyWithKill(t, &kills)
	defer stdin.close()
	done := runProxy(pr)

	// 3 Ctrl-C bytes back-to-back.
	stdin.send("\x03\x03\x03")

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&kills) >= 1 {
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

// 36. Ctrl-C hits spaced > 500 ms apart must NOT trigger force-kill. This
//     locks in the sliding-window semantics: a user who absent-mindedly
//     taps Ctrl-C occasionally shouldn't accidentally kill the agent.
func TestProxy_E2E_SpacedCtrlC_DoesNotKill(t *testing.T) {
	var kills int64
	pr, pty, stdin := makeProxyWithKill(t, &kills)
	defer stdin.close()
	done := runProxy(pr)

	stdin.send("\x03")
	time.Sleep(600 * time.Millisecond)
	stdin.send("\x03")
	time.Sleep(600 * time.Millisecond)
	stdin.send("\x03")
	time.Sleep(100 * time.Millisecond)

	if got := atomic.LoadInt64(&kills); got != 0 {
		t.Errorf("expected 0 kills (hits spaced >500ms), got %d", got)
	}

	pty.close()
	<-done
}

// 33. Safety ON: AFK refuses to blind-nudge when the screen shows a
//     deletion-class command.
func TestProxy_E2E_SafetyBlocksAfkNudgeWithDanger(t *testing.T) {
	log, err := logger.New(t.TempDir() + "/test.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	pty := newFakePTY()
	stdin := newFakeStdin()
	scr := screen.New(80, 24)
	sb := statusbar.New(24, 80, true, 0)
	chain := detector.RuleChain{agent.KindClaude.Detector()}
	pr := proxy.New(proxy.Config{
		PTY:           pty,
		Stdin:         stdin,
		Stdout:        io.Discard,
		RuleChain:     chain,
		Memory:        memory.New(),
		StatusBar:     sb,
		Log:           log,
		Term:          term.NewNoOp(),
		Screen:        scr,
		AgentKind:     agent.KindClaude,
		Delay:         0,
		Enabled:       true,
		AfkEnabled:    true,
		AfkIdle:       200 * time.Millisecond,
		SafetyEnabled: true,
	})
	defer stdin.close()
	done := runProxy(pr)

	// Plant a dangerous-looking command on the screen.
	pty.send("about to run: rm -rf /\r\n")

	// AFK would fire at ~200ms idle. With safety on, the y+continue must
	// not land on the PTY.
	time.Sleep(600 * time.Millisecond)
	if got := pty.written(); len(got) != 0 {
		t.Errorf("expected no afk writes under safety; got %q", got)
	}

	pty.close()
	<-done
}

// ── Additional hardening tests ──────────────────────────────────────────────

// 37. cfg.Kill == nil must not crash: users of the proxy package can
//     legitimately omit Kill (e.g. in tests that don't need the force-kill
//     path), and hitting Ctrl+Y q or 3x Ctrl-C should degrade to a no-op.
func TestProxy_E2E_NilKillCallback_NoPanic(t *testing.T) {
	log, err := logger.New(t.TempDir() + "/test.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	pty := newFakePTY()
	stdin := newFakeStdin()
	pr := proxy.New(proxy.Config{
		PTY:       pty,
		Stdin:     stdin,
		Stdout:    io.Discard,
		RuleChain: detector.RuleChain{agent.KindClaude.Detector()},
		Memory:    memory.New(),
		StatusBar: statusbar.New(24, 80, true, 0),
		Log:       log,
		Term:      term.NewNoOp(),
		Screen:    screen.New(80, 24),
		AgentKind: agent.KindClaude,
		Enabled:   true,
		// Kill: intentionally left nil.
	})
	defer stdin.close()
	done := runProxy(pr)

	// Ctrl+Y q with nil Kill — must not crash.
	stdin.send("\x19q")
	time.Sleep(100 * time.Millisecond)

	// 3 Ctrl-C with nil Kill — must not crash.
	stdin.send("\x03\x03\x03")
	time.Sleep(100 * time.Millisecond)

	// Proxy still running?
	select {
	case err := <-done:
		t.Fatalf("proxy exited unexpectedly: %v", err)
	default:
		// still running — pass
	}

	pty.close()
	<-done
}

// 38. Safety must block even the "memory-seen" fast path. A prompt hash
//     that's already in memory (would normally approve immediately) must
//     still be blocked if the current screen carries a dangerous command.
//     Prevents a replay attack: attacker plants an innocuous prompt once,
//     then later swaps in a destructive command with the same memory hash.
func TestProxy_E2E_SafetyBlocksMemorySeenWithDanger(t *testing.T) {
	log, err := logger.New(t.TempDir() + "/test.log")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	pty := newFakePTY()
	stdin := newFakeStdin()
	scr := screen.New(80, 24)
	sb := statusbar.New(24, 80, true, 0)
	chain := detector.RuleChain{agent.KindClaude.Detector()}
	mem := memory.New()
	// Pre-seed memory with the hash of what claudeDeletePrompt produces
	// through the Claude detector.
	seedScr := screen.New(80, 24)
	seedScr.Feed([]byte(claudeDeletePrompt))
	result := detector.Claude{}.Detect(seedScr.Text())
	if result == nil {
		t.Fatal("setup: Claude detector didn't match seed prompt")
	}
	mem.Record(result.Hash)

	pr := proxy.New(proxy.Config{
		PTY:           pty,
		Stdin:         stdin,
		Stdout:        io.Discard,
		RuleChain:     chain,
		Memory:        mem,
		StatusBar:     sb,
		Log:           log,
		Term:          term.NewNoOp(),
		Screen:        scr,
		AgentKind:     agent.KindClaude,
		Delay:         0,
		Enabled:       true,
		SafetyEnabled: true,
	})
	defer stdin.close()
	done := runProxy(pr)

	pty.send(claudeDeletePrompt)
	// Without safety + memory seen, this would approve instantly (0s
	// delay). With safety on, the dangerous content blocks the write.
	ensureNotWritten(t, pty, "\r", 400*time.Millisecond)

	pty.close()
	<-done
}
