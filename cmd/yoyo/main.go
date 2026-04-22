// cmd/yoyo/main.go
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	ptylib "github.com/aymanbagabas/go-pty"
	xterm "golang.org/x/term"

	"github.com/host452b/yoyo/internal/agent"
	"github.com/host452b/yoyo/internal/config"
	"github.com/host452b/yoyo/internal/detector"
	"github.com/host452b/yoyo/internal/logger"
	"github.com/host452b/yoyo/internal/memory"
	"github.com/host452b/yoyo/internal/proxy"
	"github.com/host452b/yoyo/internal/screen"
	"github.com/host452b/yoyo/internal/statusbar"
	"github.com/host452b/yoyo/internal/term"
)

const usageText = `yoyo — you only yes once. A PTY proxy that auto-approves AI agent permission prompts.

USAGE
  yoyo [flags] <command> [args...]

DESCRIPTION
  yoyo wraps any AI agent CLI (claude, codex, cursor, …) in a PTY proxy.
  It watches the agent's output, detects permission/approval prompts, and
  automatically sends the confirmation keystroke after a configurable delay.

  A status bar is rendered in the bottom-right corner of the terminal:
    [yoyo: on 3s]           — enabled, 3-second delay before auto-approve
    [yoyo: on 3s | Claude]  — a prompt was detected (rule name shown)
    [yoyo: on 0s | seen: X] — prompt already approved this session, sent immediately
    [yoyo: off]             — auto-approve disabled (manual mode)

SUPPORTED AGENTS
  claude        Claude Code CLI  (detects ─── bordered permission prompts)
  codex         OpenAI Codex CLI (detects "Would you like to" / "needs your approval")
  cursor        Cursor agent     (detects box-drawn ┌─┐ prompts with y/n options)
  <any command> Unknown agents are auto-detected from screen content within the
                first 10 output frames; all built-in detectors run in parallel.

FLAGS
  -delay int
        Seconds to wait before auto-approving a detected prompt.
        0 = approve immediately (no countdown).
        -1 = use value from config file (default: 3).
        Explicit -delay always takes priority over per-agent config.

  -config string
        Path to TOML config file. Supports ~/. (default: ~/.config/yoyo/config.toml)

  -log string
        Path to log file. Supports ~/. (default: ~/.yoyo/yoyo.log)

  -dry-run
        Detect prompts but do not send approval keystrokes.
        The status bar shows "dry" instead of "on". Useful for testing rules.

  -afk
        Enable AFK mode: after afk-idle without any output or input, yoyo
        injects 'y' + Enter, then 'continue, Choose based on your project
        understanding.' + Enter, and rearms. Loops until Ctrl+Y a is pressed.

  -afk-idle duration
        Idle threshold before AFK fires (default 10m). Accepts Go duration
        strings like "30m", "1h", "90s".

  -v    Print version and exit.

RUNTIME CONTROLS  (Ctrl+Y is the prefix key)
  Ctrl+Y  0     Toggle auto-approve on/off
  Ctrl+Y  1     Set delay to 1 second  (enables if currently off)
  Ctrl+Y  2     Set delay to 2 seconds (enables if currently off)
  Ctrl+Y  3     Set delay to 3 seconds (enables if currently off)
  Ctrl+Y  4     Set delay to 4 seconds (enables if currently off)
  Ctrl+Y  5     Set delay to 5 seconds (enables if currently off)
  Ctrl+Y  a     Toggle AFK mode on/off (independent of auto-approve)

  Pressing any non-escape key while the countdown is running cancels
  the pending approval, letting you inspect or respond manually.

CONFIG FILE  (~/.config/yoyo/config.toml)
  [defaults]
  delay    = 3       # default approval delay in seconds
  enabled  = true    # start with auto-approve on
  afk      = false   # enable AFK idle-nudge mode
  afk_idle = "10m"   # idle threshold before nudging
  log_file = "~/.yoyo/yoyo.log"

  # Per-agent overrides (keys: "claude", "codex", "cursor")
  [agents.claude]
  delay = 0          # approve Claude prompts immediately

  # Global custom rules (checked before built-in detectors)
  [[rules]]
  name     = "my-tool"
  pattern  = "Continue\\? \\[y/N\\]"   # Go regexp matched against screen text
  response = "y\r"                      # keystrokes to send on match

  # Agent-specific custom rules (checked first, before global rules)
  [[agents.claude.rules]]
  name     = "custom-confirm"
  pattern  = "Are you sure"
  response = "y\r"

EXAMPLES
  # Wrap claude with default settings (3-second delay)
  yoyo claude

  # Wrap claude, approve immediately
  yoyo -delay 0 claude

  # Wrap codex with a 5-second review window
  yoyo -delay 5 codex

  # Wrap an unknown tool (auto-detected from screen)
  yoyo -delay 2 my-ai-agent --some-flag

  # Use a custom config file
  yoyo -config ~/work/yoyo.toml claude

EXIT BEHAVIOR
  yoyo exits when the child process exits.
  Signals (SIGINT, SIGTERM, SIGHUP, SIGQUIT) restore the terminal and exit cleanly.
  The terminal is always restored even if yoyo crashes internally.
`

// version is set at build time via -ldflags "-X main.version=..."
var version = "dev"

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, helpText(xterm.IsTerminal(int(os.Stderr.Fd()))))
	}

	var (
		delay      = flag.Int("delay", -1, "approval delay in seconds (0=immediate, -1=from config)")
		logPath    = flag.String("log", "", "log file path (default from config)")
		cfgPath    = flag.String("config", config.DefaultPath(), "config file path")
		showVer    = flag.Bool("v", false, "print version and exit")
		dryRun     = flag.Bool("dry-run", false, "detect prompts but do not send approvals")
		afk        = flag.Bool("afk", false, "enable AFK mode (idle-timer nudges)")
		afkIdle    = flag.Duration("afk-idle", 10*time.Minute, "AFK idle threshold")
	)
	flag.Parse()

	if *showVer {
		fmt.Fprintf(os.Stderr, "yoyo %s\n", version)
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(1)
	}

	// Validate -delay: only -1 (use config) or >= 0 are valid
	if *delay < -1 {
		fmt.Fprintf(os.Stderr, "invalid -delay value %d: must be 0 or greater (use -1 to read from config)\n", *delay)
		os.Exit(2)
	}

	// Load config — error if the user explicitly provided a path that doesn't exist
	var cfgExplicit bool
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			cfgExplicit = true
		}
	})
	var (
		cfg *config.Config
		err error
	)
	if cfgExplicit {
		cfg, err = config.LoadRequired(config.ExpandTilde(*cfgPath))
	} else {
		cfg, err = config.Load(config.ExpandTilde(*cfgPath))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}

	// Resolve effective settings
	delayFromFlag := *delay >= 0
	effectiveDelay := cfg.Defaults.Delay
	if delayFromFlag {
		effectiveDelay = *delay
	}
	effectiveLog := cfg.Defaults.LogFile
	if *logPath != "" {
		effectiveLog = config.ExpandTilde(*logPath)
	}

	// Identify agent kind
	kind := agent.KindFromCommand(args[0])

	// Apply agent-specific delay override only when --delay was not explicitly provided.
	// Delay is a pointer: nil means "inherit from defaults", non-nil means explicitly set.
	if !delayFromFlag {
		if agentCfg, ok := cfg.Agents[kind.String()]; ok && agentCfg.Delay != nil {
			effectiveDelay = *agentCfg.Delay
		}
	}

	// Resolve effective AFK.
	var afkFromFlag, afkIdleFromFlag bool
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "afk":
			afkFromFlag = true
		case "afk-idle":
			afkIdleFromFlag = true
		}
	})

	effectiveAfk := cfg.Defaults.Afk
	effectiveAfkIdle := cfg.Defaults.AfkIdle
	if agentCfg, ok := cfg.Agents[kind.String()]; ok {
		if !afkFromFlag && agentCfg.Afk != nil {
			effectiveAfk = *agentCfg.Afk
		}
		if !afkIdleFromFlag && agentCfg.AfkIdle != nil {
			effectiveAfkIdle = *agentCfg.AfkIdle
		}
	}
	if afkFromFlag {
		effectiveAfk = *afk
	}
	if afkIdleFromFlag {
		effectiveAfkIdle = *afkIdle
	}
	if effectiveAfkIdle <= 0 {
		effectiveAfkIdle = 10 * time.Minute
	}

	// Start logger
	if err := os.MkdirAll(filepath.Dir(effectiveLog), 0o755); err == nil {
		// ignore mkdir error — logger.New will fail with a clear message
	}
	log, err := logger.New(effectiveLog)
	if err != nil {
		fmt.Fprintln(os.Stderr, "logger error:", err)
		os.Exit(1)
	}
	defer log.Close()

	// Build rule chain: agent rules → global rules → built-in detector
	var chain detector.RuleChain
	if agentCfg, ok := cfg.Agents[kind.String()]; ok {
		for _, r := range agentCfg.Rules {
			name := r.Name
			if name == "" {
				name = "custom"
			}
			d, err := detector.NewRegexpDetector(name, r.Pattern, r.Response)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid rule %q: %v\n", name, err)
				os.Exit(1)
			}
			chain = append(chain, d)
		}
	}
	for _, r := range cfg.Rules {
		name := r.Name
		if name == "" {
			name = "custom"
		}
		d, err := detector.NewRegexpDetector(name, r.Pattern, r.Response)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid rule %q: %v\n", name, err)
			os.Exit(1)
		}
		chain = append(chain, d)
	}
	chain = append(chain, kind.Detector())

	// Setup terminal raw mode
	t := term.New(os.Stdin)
	if err := t.EnableRaw(); err != nil {
		fmt.Fprintln(os.Stderr, "failed to enable raw mode:", err)
		os.Exit(1)
	}
	defer t.Restore()

	// Signal handler for clean exit
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	go func() {
		<-sigs
		t.Restore()
		log.Close()
		os.Exit(0)
	}()

	// Get initial terminal size
	cols, rows := t.Size()

	// Start SIGWINCH watcher (Unix only; no-op on Windows)
	scr := screen.New(cols, rows)
	scr.SetLogger(log)
	sb := statusbar.New(uint16(rows), uint16(cols), cfg.Defaults.Enabled, effectiveDelay)
	if *dryRun {
		sb.SetDryRun(true)
	}

	stopResize := t.WatchResize(func(c, r int) {
		scr.Resize(c, r)
		sb.Resize(uint16(r), uint16(c))
	})
	defer stopResize()

	// Create PTY and start child process
	p, err := ptylib.New()
	if err != nil {
		t.Restore()
		fmt.Fprintln(os.Stderr, "failed to create PTY:", err)
		os.Exit(1)
	}
	defer p.Close()

	// Set PTY size BEFORE starting the child so the child never sees 0×0.
	if err := p.Resize(cols, rows); err != nil {
		log.Errorf("failed to set initial PTY size: %v", err)
	}

	cmd := p.Command(args[0], args[1:]...)
	// Give the child a clean environment: correct TERM for the PTY,
	// and strip tmux variables so the child doesn't think it's in tmux.
	cmd.Env = buildChildEnv(os.Environ())
	if err := cmd.Start(); err != nil {
		t.Restore()
		fmt.Fprintln(os.Stderr, "failed to start process:", err)
		os.Exit(1)
	}

	log.Infof("started %s (kind=%s, delay=%ds)", args[0], kind, effectiveDelay)

	// Also hook resize to update PTY
	stopResize2 := t.WatchResize(func(c, r int) {
		_ = p.Resize(c, r)
	})
	defer stopResize2()

	// Run proxy
	pr := proxy.New(proxy.Config{
		PTY:        p,
		RuleChain:  chain,
		Memory:     memory.New(),
		StatusBar:  sb,
		Log:        log,
		Term:       t,
		Screen:     scr,
		AgentKind:  kind,
		Delay:      effectiveDelay,
		Enabled:    cfg.Defaults.Enabled,
		DryRun:     *dryRun,
		AfkEnabled: effectiveAfk,
		AfkIdle:    effectiveAfkIdle,
	})

	if err := pr.Run(); err != nil {
		log.Errorf("proxy error: %v", err)
	}

	// Exit summary
	if n := pr.ApprovalCount(); n > 0 {
		fmt.Fprintf(os.Stderr, "\nyoyo: %d prompt(s) auto-approved\n", n)
	}
}
