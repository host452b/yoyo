// internal/proxy/proxy.go
package proxy

import (
	"io"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/host452b/yoyo/internal/agent"
	"github.com/host452b/yoyo/internal/detector"
	"github.com/host452b/yoyo/internal/logger"
	"github.com/host452b/yoyo/internal/memory"
	"github.com/host452b/yoyo/internal/screen"
	"github.com/host452b/yoyo/internal/statusbar"
	"github.com/host452b/yoyo/internal/term"
)

const (
	prefixByte    = byte(0x19) // Ctrl+Y
	prefixTimeout = 1500 * time.Millisecond
)

// Config holds all dependencies for the Proxy.
type Config struct {
	PTY       io.ReadWriter // child PTY (go-pty Pty)
	Stdin     io.Reader     // defaults to os.Stdin if nil
	Stdout    io.Writer     // defaults to os.Stdout if nil
	RuleChain detector.RuleChain
	Memory    *memory.Memory
	StatusBar *statusbar.StatusBar
	Log       *logger.Logger
	Term      *term.Term
	Screen    *screen.Screen
	AgentKind agent.Kind
	Delay     int // seconds
	Enabled   bool
	DryRun    bool

	AfkEnabled bool
	AfkIdle    time.Duration

	FuzzyEnabled bool
	FuzzyStable  time.Duration

	// SafetyEnabled makes sendApproval and the AFK fire case refuse to
	// auto-approve when the visible screen contains a deletion-class
	// command (rm -rf, git clean, kubectl delete, DROP TABLE, terraform
	// destroy, …). The user can still approve manually by pressing the
	// appropriate key; yoyo just won't pull the trigger for them. See
	// internal/detector/danger.go for the full pattern list.
	SafetyEnabled bool

	// Kill is the escape-hatch callback for killing the child process
	// when the agent's TUI has wedged its input handling (e.g. Claude
	// Code's "Press Ctrl-C again to exit" stuck state). Invoked by two
	// paths: the Ctrl+Y q prefix command, and 3× Ctrl-C within 500ms.
	// Typically set to func() { cmd.Process.Kill() }.
	Kill func()

	// Dump is the Ctrl+Y d callback. It receives the proxy's current
	// runtime state, collects a diagnostic snapshot, and writes it to a
	// timestamped file under ~/.yoyo/dumps/. Returns the absolute path
	// of the written file, or an error. The status bar briefly shows
	// "dumped: <path>" so the user knows where to find it.
	Dump func(RuntimeState) (path string, err error)
}

// RuntimeState is the mutable proxy state captured at the exact moment a
// diagnostic dump is requested.
type RuntimeState struct {
	AgentKind     agent.Kind
	Enabled       bool
	Delay         int
	DryRun        bool
	AfkEnabled    bool
	AfkIdle       time.Duration
	FuzzyEnabled  bool
	FuzzyStable   time.Duration
	SafetyEnabled bool
	ApprovalCount int64
}

// Proxy is the coordinator that routes bytes between stdin, child PTY, and stdout.
type Proxy struct {
	cfg           Config
	approvalCount int64 // atomic; total prompts approved
}

func New(cfg Config) *Proxy {
	return &Proxy{cfg: cfg}
}

// ApprovalCount returns the number of prompts approved during this session.
func (p *Proxy) ApprovalCount() int64 {
	return atomic.LoadInt64(&p.approvalCount)
}

// safeGo launches fn in a goroutine with panic recovery.
// On panic, restores terminal and exits.
func safeGo(t *term.Term, log *logger.Logger, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.Restore()
				if log != nil {
					log.Errorf("goroutine panic: %v\n%s", r, debug.Stack())
					log.Close()
				}
				os.Exit(1)
			}
		}()
		fn()
	}()
}

// Run starts the proxy event loop. Blocks until the child exits.
func (p *Proxy) Run() error {
	cfg := p.cfg
	stdin := cfg.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	stdout := cfg.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}

	inputCh := make(chan []byte, 32)
	outputCh := make(chan []byte, 32)
	done := make(chan struct{})
	var closeOnce sync.Once
	closeDone := func() { closeOnce.Do(func() { close(done) }) }

	// stdin → inputCh
	safeGo(cfg.Term, cfg.Log, func() {
		defer close(inputCh)
		buf := make([]byte, 128*1024)
		for {
			n, err := stdin.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case inputCh <- data:
				case <-done:
					return
				}
			}
			if err != nil {
				return
			}
		}
	})

	// PTY → outputCh
	safeGo(cfg.Term, cfg.Log, func() {
		defer close(outputCh)
		buf := make([]byte, 4096)
		for {
			n, err := cfg.PTY.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])
				select {
				case outputCh <- data:
				case <-done:
					return
				}
			}
			if err != nil {
				return
			}
		}
	})

	enabled := cfg.Enabled
	delaySecs := cfg.Delay
	dryRun := cfg.DryRun
	agentKind := cfg.AgentKind
	frames := 0

	var approvalTimer *time.Timer
	var timerCh <-chan time.Time
	var lastResult *detector.MatchResult
	var approvedHash string        // suppress re-approvals while prompt is still on screen
	var approvalDeadline time.Time // for countdown display

	var prefixTimer *time.Timer
	var prefixTimerCh <-chan time.Time
	prefixActive := false

	// Sliding-window Ctrl-C detector: 3 hits within 500 ms → force-kill.
	// Holds at most 3 timestamps; trimmed on each new 0x03 byte.
	ctrlCHits := make([]time.Time, 0, 3)

	afkEnabled := cfg.AfkEnabled
	var afkIdleTimer *time.Timer
	var afkIdleTimerCh <-chan time.Time
	var afkDeadline time.Time    // zero = idle timer inactive
	var afkNudgedUntil time.Time // status-bar flash window after a fire

	armAfk := func() {
		if !afkEnabled || cfg.AfkIdle <= 0 {
			afkDeadline = time.Time{}
			return
		}
		if afkIdleTimer != nil {
			afkIdleTimer.Stop()
		}
		afkIdleTimer = time.NewTimer(cfg.AfkIdle)
		afkIdleTimerCh = afkIdleTimer.C
		afkDeadline = time.Now().Add(cfg.AfkIdle)
	}
	stopAfk := func() {
		if afkIdleTimer != nil {
			afkIdleTimer.Stop()
		}
		afkIdleTimer = nil
		afkIdleTimerCh = nil
		afkDeadline = time.Time{}
	}
	armAfk()

	fuzzyEnabled := cfg.FuzzyEnabled
	var fuzzyStableTimer *time.Timer
	var fuzzyStableTimerCh <-chan time.Time
	var fuzzyLastHash string

	armFuzzyStable := func() {
		if !fuzzyEnabled || cfg.FuzzyStable <= 0 {
			return
		}
		if fuzzyStableTimer != nil {
			fuzzyStableTimer.Stop()
		}
		fuzzyStableTimer = time.NewTimer(cfg.FuzzyStable)
		fuzzyStableTimerCh = fuzzyStableTimer.C
	}
	stopFuzzy := func() {
		if fuzzyStableTimer != nil {
			fuzzyStableTimer.Stop()
		}
		fuzzyStableTimer = nil
		fuzzyStableTimerCh = nil
		fuzzyLastHash = ""
	}

	// Rebuild rule chain when agent kind is resolved
	chain := cfg.RuleChain

	currentState := func() RuntimeState {
		return RuntimeState{
			AgentKind:     agentKind,
			Enabled:       enabled,
			Delay:         delaySecs,
			DryRun:        dryRun,
			AfkEnabled:    afkEnabled,
			AfkIdle:       cfg.AfkIdle,
			FuzzyEnabled:  fuzzyEnabled,
			FuzzyStable:   cfg.FuzzyStable,
			SafetyEnabled: cfg.SafetyEnabled,
			ApprovalCount: p.ApprovalCount(),
		}
	}

	// Helper to send approval (respects dry-run mode).
	//
	// Safety guard: if SafetyEnabled and the current screen contains a
	// deletion-class command (rm -rf, git clean, kubectl delete, DROP
	// TABLE, terraform destroy, etc.), auto-approval is refused. The
	// user can still press y manually; yoyo just won't pull the trigger
	// for them. Status bar flips to show "danger: <snippet>" so the user
	// knows why approval was skipped.
	sendApproval := func(result *detector.MatchResult, label string) {
		if cfg.SafetyEnabled {
			if hit, snippet := detector.ContainsDangerousCommand(cfg.Screen.Text()); hit {
				cfg.StatusBar.SetRule("danger: " + snippet)
				if cfg.Log != nil {
					cfg.Log.Errorf("safety: blocked %s — dangerous command on screen: %q", label, snippet)
				}
				return
			}
		}
		if dryRun {
			if cfg.Log != nil {
				cfg.Log.Infof("dry-run: would send %q for %s", result.Response, label)
			}
		} else {
			if _, err := cfg.PTY.Write([]byte(result.Response)); err != nil && cfg.Log != nil {
				cfg.Log.Errorf("failed to send %s response: %v", label, err)
			}
		}
		atomic.AddInt64(&p.approvalCount, 1)
	}

	for {
		select {
		case data, ok := <-inputCh:
			if !ok {
				if approvalTimer != nil {
					approvalTimer.Stop()
				}
				if prefixTimer != nil {
					prefixTimer.Stop()
				}
				stopAfk()
				stopFuzzy()
				closeDone()
				return nil
			}
			// Force-kill escape hatch: three Ctrl-C (0x03) presses within
			// 1 s trigger cfg.Kill(). Covers the case where the agent's
			// TUI has wedged its input handling and normal Ctrl-C isn't
			// taking effect anymore. The 1-second window is deliberately
			// generous — 500ms proved too tight in practice for users
			// whose frustrated-Ctrl-C cadence is ~600ms. Also honoured:
			// Ctrl+Y q (below).
			for _, b := range data {
				if b != 0x03 {
					ctrlCHits = ctrlCHits[:0]
					continue
				}
				now := time.Now()
				cut := now.Add(-1 * time.Second)
				trimmed := ctrlCHits[:0]
				for _, t := range ctrlCHits {
					if t.After(cut) {
						trimmed = append(trimmed, t)
					}
				}
				ctrlCHits = append(trimmed, now)
				if len(ctrlCHits) >= 3 && cfg.Kill != nil {
					if cfg.Log != nil {
						cfg.Log.Errorf("force-kill: 3x Ctrl-C within 1s — killing child")
					}
					cfg.Kill()
					ctrlCHits = ctrlCHits[:0]
				}
			}

			// Handle Ctrl+Y a / f / q / d inline (rather than inside handlePrefix)
			// because their state is local to Run. Covers two scenarios per letter:
			//   (A) "\x19<letter>" arrives as one chunk from stdin
			//   (B) "\x19" then "<letter>" arrive as separate chunks (prefixActive=true)
			var toggleCmd byte
			switch {
			case len(data) >= 2 && data[0] == prefixByte && (data[1] == 'a' || data[1] == 'f' || data[1] == 'q' || data[1] == 'd'):
				toggleCmd = data[1]
				data = data[2:]
			case prefixActive && len(data) > 0 && (data[0] == 'a' || data[0] == 'f' || data[0] == 'q' || data[0] == 'd'):
				toggleCmd = data[0]
				data = data[1:]
			}
			if toggleCmd != 0 {
				prefixActive = false
				cfg.StatusBar.SetPrefix(false)
				if prefixTimer != nil {
					prefixTimer.Stop()
					prefixTimer = nil
					prefixTimerCh = nil
				}
				switch toggleCmd {
				case 'a':
					afkEnabled = !afkEnabled
					if afkEnabled {
						armAfk()
					} else {
						stopAfk()
					}
				case 'f':
					fuzzyEnabled = !fuzzyEnabled
					if !fuzzyEnabled {
						stopFuzzy()
					} else {
						// Re-enable: prime hash from current screen and arm the
						// stability timer immediately, so a screen that was
						// already stable at toggle time still gets evaluated.
						fuzzyLastHash = detector.HashBody(cfg.Screen.Text())
						armFuzzyStable()
					}
				case 'q':
					// Force-kill the child. Deliberate escape hatch for wedged
					// agents (Claude Code's "Press Ctrl-C again to exit" stuck
					// state, etc.). Same action as 3× Ctrl-C within 500ms.
					if cfg.Log != nil {
						cfg.Log.Errorf("force-kill: Ctrl+Y q pressed — killing child")
					}
					if cfg.Kill != nil {
						cfg.Kill()
					}
				case 'd':
					// Write a diagnostic dump to ~/.yoyo/dumps/ and show the
					// path in the status bar. Useful for capturing "why did
					// yoyo do/not do X here?" situations for follow-up.
					if cfg.Dump != nil {
						if path, err := cfg.Dump(currentState()); err != nil {
							cfg.StatusBar.SetRule("dump failed: " + err.Error())
							if cfg.Log != nil {
								cfg.Log.Errorf("dump: write failed: %v", err)
							}
						} else {
							cfg.StatusBar.SetRule("dumped: " + path)
							if cfg.Log != nil {
								cfg.Log.Infof("dump: wrote %s", path)
							}
						}
					} else if cfg.Log != nil {
						cfg.Log.Errorf("dump: no callback configured")
					}
				}
				stdout.Write(cfg.StatusBar.WrapFrame([]byte{}))
				if len(data) == 0 {
					continue
				}
			}

			data = p.handlePrefix(data, &prefixActive, &prefixTimer, &prefixTimerCh,
				&enabled, &delaySecs, &approvalTimer, &timerCh, cfg, stdout)

			if len(data) == 0 {
				continue
			}

			// Any non-escape user keypress cancels pending approval
			if !isEscapeSequence(data) && approvalTimer != nil {
				if cfg.Log != nil {
					cfg.Log.Infof("user input during delay, cancelling approval")
				}
				approvalTimer.Stop()
				approvalTimer = nil
				timerCh = nil
				cfg.StatusBar.SetCountdown(-1)
			}

			if afkIdleTimer != nil {
				afkIdleTimer.Reset(cfg.AfkIdle)
				afkDeadline = time.Now().Add(cfg.AfkIdle)
			}

			cfg.PTY.Write(data)

		case data, ok := <-outputCh:
			if !ok {
				if approvalTimer != nil {
					approvalTimer.Stop()
				}
				if prefixTimer != nil {
					prefixTimer.Stop()
				}
				stopAfk()
				stopFuzzy()
				closeDone()
				return nil
			}

			cfg.Screen.Feed(data)
			if afkIdleTimer != nil {
				afkIdleTimer.Reset(cfg.AfkIdle)
				afkDeadline = time.Now().Add(cfg.AfkIdle)
			}
			text := cfg.Screen.Text()

			if fuzzyEnabled {
				h := detector.HashBody(text)
				if h != fuzzyLastHash {
					fuzzyLastHash = h
					armFuzzyStable()
				}
			}

			// Try to resolve unknown agent from screen during first 10 frames
			if agentKind == agent.KindUnknown && frames < 10 {
				if k := agent.KindFromScreen(text); k != agent.KindUnknown {
					agentKind = k
					if cfg.Log != nil {
						cfg.Log.Infof("detected agent from screen: %s", k)
					}
					// Rebuild chain with resolved kind
					chain = append(cfg.RuleChain[:len(cfg.RuleChain):len(cfg.RuleChain)],
						agentKind.Detector())
				}
			}
			frames++

			if enabled {
				result := chain.Detect(text)
				if result == nil {
					approvedHash = "" // prompt gone, allow future re-approval
					lastResult = nil  // reset so the next fresh appearance starts a new countdown
				} else if result.Hash == approvedHash {
					// Already approved this prompt instance; skip until it disappears
				} else if cfg.Memory.Seen(result.Hash) {
					cfg.StatusBar.SetRule("seen: " + result.RuleName)
					approvedHash = result.Hash
					sendApproval(result, "seen-approval")
				} else {
					if cfg.Log != nil {
						cfg.Log.Infof("prompt detected: %s", result.RuleName)
					}
					cfg.StatusBar.SetRule(result.RuleName)
					if delaySecs == 0 {
						cfg.Memory.Record(result.Hash)
						approvedHash = result.Hash
						sendApproval(result, "immediate-approval")
					} else if lastResult == nil {
						// Fresh prompt appearance: start the approval countdown.
						lastResult = result
						approvalTimer = time.NewTimer(time.Duration(delaySecs) * time.Second)
						timerCh = approvalTimer.C
						approvalDeadline = time.Now().Add(time.Duration(delaySecs) * time.Second)
					} else if lastResult.Hash != result.Hash {
						// Hash drifted (rendering noise from background output or
						// overlapping dialogs) — update the stored result so the
						// correct response fires, but do NOT restart the timer.
						// Restarting on every hash change caused the countdown to
						// reset indefinitely when a progress table bled into the
						// dialog body on each screen refresh.
						lastResult = result
					}
				}
			}

			// Update countdown display
			if approvalTimer != nil {
				remaining := int(time.Until(approvalDeadline).Seconds() + 0.5)
				if remaining < 0 {
					remaining = 0
				}
				cfg.StatusBar.SetCountdown(remaining)
			}

			if afkEnabled && !afkDeadline.IsZero() {
				remaining := int(time.Until(afkDeadline).Seconds() + 0.5)
				if remaining < 0 {
					remaining = 0
				}
				nudged := time.Now().Before(afkNudgedUntil)
				cfg.StatusBar.SetAfk(true, remaining, nudged)
			} else {
				cfg.StatusBar.SetAfk(false, 0, false)
			}

			out := cfg.StatusBar.WrapFrame(data)
			stdout.Write(out)

		case <-timerCh:
			timerCh = nil
			approvalTimer = nil
			cfg.StatusBar.SetCountdown(-1)
			if lastResult != nil {
				if cfg.Log != nil {
					cfg.Log.Infof("approval timer fired, sending response for: %s", lastResult.RuleName)
				}
				cfg.Memory.Record(lastResult.Hash)
				approvedHash = lastResult.Hash
				sendApproval(lastResult, "delayed-approval")
				lastResult = nil
			}

		case <-prefixTimerCh:
			// Prefix key timed out without command byte — forward 0x19 to child
			prefixTimerCh = nil
			prefixTimer = nil
			prefixActive = false
			cfg.StatusBar.SetPrefix(false)
			cfg.PTY.Write([]byte{prefixByte})

		case <-afkIdleTimerCh:
			afkIdleTimerCh = nil
			afkIdleTimer = nil
			// Safety guard: AFK is the most dangerous approval path (blind
			// nudge with no pattern match), so we refuse to fire when the
			// screen shows a deletion-class command.
			if cfg.SafetyEnabled {
				if hit, snippet := detector.ContainsDangerousCommand(cfg.Screen.Text()); hit {
					cfg.StatusBar.SetRule("danger: " + snippet)
					if cfg.Log != nil {
						cfg.Log.Errorf("safety: blocked afk nudge — dangerous command on screen: %q", snippet)
					}
					armAfk() // rearm — user might manually handle, then afk resumes
					break
				}
			}
			if cfg.DryRun {
				if cfg.Log != nil {
					cfg.Log.Infof("afk: would send y + continue")
				}
			} else {
				if _, err := cfg.PTY.Write([]byte("y\r")); err != nil && cfg.Log != nil {
					cfg.Log.Errorf("afk: failed to send y: %v", err)
				}
				// Blocks the select for 200ms once per AFK fire (~1 time per AfkIdle
				// window). Accepted over a second timer: by construction the loop was
				// idle when we got here, and buffered channels absorb arrivals.
				time.Sleep(200 * time.Millisecond)
				if _, err := cfg.PTY.Write([]byte("continue, Choose based on your project understanding.\r")); err != nil && cfg.Log != nil {
					cfg.Log.Errorf("afk: failed to send continue: %v", err)
				}
			}
			afkNudgedUntil = time.Now().Add(2 * time.Second)
			armAfk()

		case <-fuzzyStableTimerCh:
			fuzzyStableTimerCh = nil
			fuzzyStableTimer = nil
			// Re-read the current text and run vocab match.
			currentText := cfg.Screen.Text()
			// Fuzzy is a fallback: if a specific detector currently matches
			// this screen, let the outputCh approval path own it. Prevents a
			// double-fire when a specific prompt also contains a y/n marker.
			if r := chain.Detect(currentText); r != nil {
				break
			}
			if !detector.FuzzyMatch(currentText) {
				break
			}
			synth := &detector.MatchResult{
				RuleName: "fuzzy",
				Response: "\r",
				Hash:     detector.HashBody(currentText),
			}
			if synth.Hash == approvedHash {
				break // already handled
			}
			if cfg.Log != nil {
				cfg.Log.Infof("fuzzy: match at stable-window expiry")
			}
			cfg.StatusBar.SetRule(synth.RuleName)
			if cfg.Memory.Seen(synth.Hash) {
				approvedHash = synth.Hash
				sendApproval(synth, "fuzzy-seen-approval")
			} else if delaySecs == 0 {
				cfg.Memory.Record(synth.Hash)
				approvedHash = synth.Hash
				sendApproval(synth, "fuzzy-immediate-approval")
			} else if lastResult == nil || lastResult.Hash != synth.Hash {
				if approvalTimer != nil {
					approvalTimer.Stop()
				}
				lastResult = synth
				approvalTimer = time.NewTimer(time.Duration(delaySecs) * time.Second)
				timerCh = approvalTimer.C
				approvalDeadline = time.Now().Add(time.Duration(delaySecs) * time.Second)
			}
		}
	}
}

// handlePrefix processes the Ctrl+Y prefix key state machine.
// Returns the remaining data to forward to the child (may be empty).
func (p *Proxy) handlePrefix(
	data []byte,
	prefixActive *bool,
	prefixTimer **time.Timer,
	prefixTimerCh *<-chan time.Time,
	enabled *bool,
	delaySecs *int,
	approvalTimer **time.Timer,
	timerCh *<-chan time.Time,
	cfg Config,
	stdout io.Writer,
) []byte {
	sb := cfg.StatusBar

	if *prefixActive {
		*prefixActive = false
		sb.SetPrefix(false)
		if *prefixTimer != nil {
			(*prefixTimer).Stop()
			*prefixTimer = nil
			*prefixTimerCh = nil
		}
		cmd := data[0]
		rest := data[1:]
		switch cmd {
		case '0':
			*enabled = !*enabled
			if !*enabled && *approvalTimer != nil {
				(*approvalTimer).Stop()
				*approvalTimer = nil
				*timerCh = nil
				sb.SetCountdown(-1)
			}
			sb.Toggle()
		case '1', '2', '3', '4', '5':
			secs := int(cmd - '0')
			*delaySecs = secs
			if !*enabled {
				*enabled = true
				sb.Toggle() // re-enable
			}
			sb.SetDelay(secs)
		default:
			// Unrecognized: forward prefix + cmd byte
			cfg.PTY.Write([]byte{prefixByte})
			return append([]byte{cmd}, rest...)
		}
		// Repaint status bar immediately
		out := sb.WrapFrame([]byte{})
		stdout.Write(out)
		return rest
	}

	if len(data) > 0 && data[0] == prefixByte {
		if len(data) == 1 {
			// Saw Ctrl+Y alone — enter prefix mode with timeout
			*prefixActive = true
			sb.SetPrefix(true)
			t := time.NewTimer(prefixTimeout)
			*prefixTimer = t
			*prefixTimerCh = t.C
			// Repaint to show prefix indicator
			out := sb.WrapFrame([]byte{})
			stdout.Write(out)
			return nil
		}
		// Ctrl+Y followed immediately by another byte
		*prefixActive = true
		return p.handlePrefix(data[1:], prefixActive, prefixTimer, prefixTimerCh,
			enabled, delaySecs, approvalTimer, timerCh, cfg, stdout)
	}

	return data
}

// isEscapeSequence returns true if data is a complete CSI escape sequence
// (terminal-generated: arrow keys, function keys, focus events, cursor reports,
// mouse events, bracketed paste markers, etc.) rather than intentional user input.
// Any complete CSI sequence starts with ESC [ and ends with a byte in 0x40–0x7E.
func isEscapeSequence(data []byte) bool {
	if len(data) < 3 || data[0] != 0x1b || data[1] != '[' {
		return false
	}
	last := data[len(data)-1]
	return last >= 0x40 && last <= 0x7E
}
