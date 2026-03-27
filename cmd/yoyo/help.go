package main

import "strings"

// helpText returns the usage text, optionally with ANSI color codes.
// Pass color=true only when stderr is a terminal.
func helpText(color bool) string {
	if !color {
		return usageText
	}

	const (
		bold   = "\x1b[1m"
		cyan   = "\x1b[1;36m"
		yellow = "\x1b[1;33m"
		reset  = "\x1b[0m"
	)

	lines := strings.Split(usageText, "\n")
	out := make([]string, len(lines))

	for i, line := range lines {
		switch {
		// Title line (first non-empty): bold
		case i == 0:
			out[i] = bold + line + reset

		// Section headers: start at column 0 with an uppercase letter
		case len(line) > 0 && line[0] >= 'A' && line[0] <= 'Z':
			out[i] = cyan + line + reset

		// Flag entries: "  -flagname type"
		case strings.HasPrefix(line, "  -"):
			out[i] = "  " + yellow + line[2:] + reset

		default:
			out[i] = line
		}
	}

	result := strings.Join(out, "\n")
	// Highlight Ctrl+Y wherever it appears (including inside already-colored lines)
	result = strings.ReplaceAll(result, "Ctrl+Y", bold+"Ctrl+Y"+reset)
	return result
}
