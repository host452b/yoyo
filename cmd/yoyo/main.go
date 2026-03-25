// cmd/yoyo/main.go
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	ptylib "github.com/aymanbagabas/go-pty"

	"yoyo/internal/agent"
	"yoyo/internal/config"
	"yoyo/internal/detector"
	"yoyo/internal/logger"
	"yoyo/internal/memory"
	"yoyo/internal/proxy"
	"yoyo/internal/screen"
	"yoyo/internal/statusbar"
	"yoyo/internal/term"
)

func main() {
	var (
		delay   = flag.Int("delay", -1, "approval delay in seconds (0=immediate, default from config)")
		logPath = flag.String("log", "", "log file path (default from config)")
		cfgPath = flag.String("config", config.DefaultPath(), "config file path")
	)
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: yoyo [flags] <command> [args...]")
		os.Exit(1)
	}

	// Load config (expand tilde in --config flag value)
	cfg, err := config.Load(config.ExpandTilde(*cfgPath))
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

	// Apply agent-specific delay override only when --delay was not explicitly provided
	if !delayFromFlag {
		if agentCfg, ok := cfg.Agents[kind.String()]; ok && agentCfg.Delay >= 0 {
			effectiveDelay = agentCfg.Delay
		}
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
	sb := statusbar.New(uint16(rows), uint16(cols), cfg.Defaults.Enabled, effectiveDelay)

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

	cmd := p.Command(args[0], args[1:]...)
	if err := cmd.Start(); err != nil {
		t.Restore()
		fmt.Fprintln(os.Stderr, "failed to start process:", err)
		os.Exit(1)
	}

	log.Infof("started %s (kind=%s, delay=%ds)", args[0], kind, effectiveDelay)

	// Set initial PTY size
	if err := p.Resize(cols, rows); err != nil {
		log.Errorf("failed to set initial PTY size: %v", err)
	}

	// Also hook resize to update PTY
	stopResize2 := t.WatchResize(func(c, r int) {
		_ = p.Resize(c, r)
	})
	defer stopResize2()

	// Run proxy
	pr := proxy.New(proxy.Config{
		PTY:       p,
		RuleChain: chain,
		Memory:    memory.New(),
		StatusBar: sb,
		Log:       log,
		Term:      t,
		Screen:    scr,
		AgentKind: kind,
		Delay:     effectiveDelay,
		Enabled:   cfg.Defaults.Enabled,
	})

	if err := pr.Run(); err != nil {
		log.Errorf("proxy error: %v", err)
	}
}
