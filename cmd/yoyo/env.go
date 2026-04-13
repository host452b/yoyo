package main

import "strings"

// buildChildEnv returns a sanitised copy of parent suitable for a child process
// running inside yoyo's own PTY.
func buildChildEnv(parent []string) []string {
	out := make([]string, 0, len(parent)+1)
	hasTERM := false
	for _, e := range parent {
		switch {
		case strings.HasPrefix(e, "TMUX="),
			strings.HasPrefix(e, "TMUX_PANE="):
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
