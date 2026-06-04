package main

import "strings"

// buildChildEnv returns a sanitised copy of parent suitable for a child process
// running inside yoyo's own PTY.
//
// tmux's environ_for_session() injects a cluster of terminal-identity vars into
// every pane child: TERM, TMUX, TMUX_PANE, TERM_PROGRAM=tmux, and
// TERM_PROGRAM_VERSION. Since the child actually runs in yoyo's own
// xterm-compatible PTY, we strip them all so the child sees a clean xterm.
// Overriding TERM alone is not enough — a TUI that branches on TERM_PROGRAM
// (e.g. for tmux passthrough) would still emit sequences the vt10x screen
// emulator can't parse, breaking prompt detection.
func buildChildEnv(parent []string) []string {
	out := make([]string, 0, len(parent)+1)
	hasTERM := false
	for _, e := range parent {
		switch {
		case strings.HasPrefix(e, "TMUX="),
			strings.HasPrefix(e, "TMUX_PANE="),
			strings.HasPrefix(e, "TERM_PROGRAM="),
			strings.HasPrefix(e, "TERM_PROGRAM_VERSION="):
			continue
		case strings.HasPrefix(e, "TERM="):
			out = append(out, "TERM=xterm-256color")
			hasTERM = true
		default:
			out = append(out, e)
		}
	}
	if !hasTERM {
		out = append(out, "TERM=xterm-256color")
	}
	return out
}
