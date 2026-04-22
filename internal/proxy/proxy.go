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
	PTY       io.ReadWriter  // child PTY (go-pty Pty)
	Stdin     io.Reader      // defaults to os.Stdin if nil
	Stdout    io.Writer      // defaults to os.Stdout if nil
	RuleChain detector.RuleChain
	Memory    *memory.Memory
	StatusBar *statusbar.StatusBar
	Log       *logger.Logger
	Term      *term.Term
	Screen    *screen.Screen
	AgentKind agent.Kind
	Delay     int  // seconds
	Enabled   bool
	DryRun    bool

	AfkEnabled bool
	AfkIdle    time.Duration
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
	var approvedHash string    // suppress re-approvals while prompt is still on screen
	var approvalDeadline time.Time // for countdown display

	var prefixTimer *time.Timer
	var prefixTimerCh <-chan time.Time
	prefixActive := false

	afkEnabled := cfg.AfkEnabled
	var afkIdleTimer *time.Timer
	var afkIdleTimerCh <-chan time.Time
	var afkNudgedUntil time.Time // status-bar flash window; used by later tasks

	armAfk := func() {
		if !afkEnabled || cfg.AfkIdle <= 0 {
			return
		}
		if afkIdleTimer != nil {
			afkIdleTimer.Stop()
		}
		afkIdleTimer = time.NewTimer(cfg.AfkIdle)
		afkIdleTimerCh = afkIdleTimer.C
	}
	stopAfk := func() {
		if afkIdleTimer != nil {
			afkIdleTimer.Stop()
		}
		afkIdleTimer = nil
		afkIdleTimerCh = nil
	}
	armAfk()

	// Rebuild rule chain when agent kind is resolved
	chain := cfg.RuleChain

	// Helper to send approval (respects dry-run mode)
	sendApproval := func(result *detector.MatchResult, label string) {
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
				closeDone()
				return nil
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
				closeDone()
				return nil
			}

			cfg.Screen.Feed(data)
			text := cfg.Screen.Text()

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
					} else if lastResult == nil || lastResult.Hash != result.Hash {
						// New or changed prompt: (re)start timer
						if approvalTimer != nil {
							approvalTimer.Stop()
						}
						lastResult = result
						approvalTimer = time.NewTimer(time.Duration(delaySecs) * time.Second)
						timerCh = approvalTimer.C
						approvalDeadline = time.Now().Add(time.Duration(delaySecs) * time.Second)
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
			if cfg.DryRun {
				if cfg.Log != nil {
					cfg.Log.Infof("afk: would send y + continue")
				}
			} else {
				if _, err := cfg.PTY.Write([]byte("y\r")); err != nil && cfg.Log != nil {
					cfg.Log.Errorf("afk: failed to send y: %v", err)
				}
				time.Sleep(200 * time.Millisecond)
				if _, err := cfg.PTY.Write([]byte("continue, Choose based on your project understanding.\r")); err != nil && cfg.Log != nil {
					cfg.Log.Errorf("afk: failed to send continue: %v", err)
				}
			}
			afkNudgedUntil = time.Now().Add(2 * time.Second)
			_ = afkNudgedUntil // consumed by Task 11 (status-bar flash)
			armAfk()
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
