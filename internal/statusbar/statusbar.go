// internal/statusbar/statusbar.go
package statusbar

import "fmt"

const (
	cursorSave    = "\x1b[s"
	cursorRestore = "\x1b[u"
	sgrReset      = "\x1b[0m"
	fgGreen       = "\x1b[32m"
	fgRed         = "\x1b[31m"
)

// labelWidth is the fixed width of the status label (widest possible label).
// " [yoyo: on Xs | RuleName]" — we reserve space for "on Xs" + rule.
// Keep it wide enough; truncate rule name if needed.
const minLabelWidth = 22 // " [yoyo: on Xs | ...]  " minimum

// StatusBar renders a bottom-right ANSI overlay around PTY output frames.
type StatusBar struct {
	rows      uint16
	cols      uint16
	enabled   bool
	delaySecs int
	rule      string
	painted   bool
	midSeq    bool
}

// New creates a StatusBar. enabled=true means auto-approve is active.
func New(rows, cols uint16, enabled bool, delaySecs int) *StatusBar {
	return &StatusBar{
		rows:      rows,
		cols:      cols,
		enabled:   enabled,
		delaySecs: delaySecs,
	}
}

func (sb *StatusBar) Toggle() { sb.enabled = !sb.enabled }

func (sb *StatusBar) SetDelay(secs int) { sb.delaySecs = secs }

// SetRule sets the last-matched rule name shown in the label.
func (sb *StatusBar) SetRule(rule string) { sb.rule = rule }

// Resize updates terminal dimensions.
func (sb *StatusBar) Resize(rows, cols uint16) {
	sb.rows = rows
	sb.cols = cols
}

// WrapFrame injects clear-previous and paint-new ANSI sequences around frame.
// Returns a single buffer for one atomic write to stdout.
func (sb *StatusBar) WrapFrame(frame []byte) []byte {
	prevMid := sb.midSeq
	sb.midSeq = endsMidEscape(frame) || endsMidUTF8(frame)

	label := sb.labelText()
	lw := uint16(len(label))
	if sb.cols < lw+2 || sb.rows == 0 {
		return frame // terminal too narrow
	}

	col := sb.cols - lw + 1

	var clear []byte
	if sb.painted && !prevMid {
		sb.painted = false
		blank := make([]byte, lw)
		for i := range blank {
			blank[i] = ' '
		}
		clear = overlayAt(sb.rows, col, "", string(blank))
	} else if prevMid {
		sb.painted = false
	}

	var paint []byte
	if !sb.midSeq {
		sb.painted = true
		color := fgRed // off = red (warning)
		if sb.enabled {
			color = fgGreen // on = green (active/good)
		}
		paint = overlayAt(sb.rows, col, color, label)
	}

	out := make([]byte, 0, len(clear)+len(frame)+len(paint))
	out = append(out, clear...)
	out = append(out, frame...)
	out = append(out, paint...)
	return out
}

func (sb *StatusBar) labelText() string {
	if !sb.enabled {
		return " [yoyo: off] "
	}
	rule := sb.rule
	if rule == "" {
		return fmt.Sprintf(" [yoyo: on %ds] ", sb.delaySecs)
	}
	return fmt.Sprintf(" [yoyo: on %ds | %s] ", sb.delaySecs, rule)
}

func overlayAt(row, col uint16, color, text string) []byte {
	s := fmt.Sprintf("%s\x1b[%d;%dH%s%s%s%s",
		cursorSave, row, col, sgrReset, color, text, cursorRestore)
	return []byte(s)
}

// endsMidEscape reports whether data ends with an incomplete ANSI escape.
func endsMidEscape(data []byte) bool {
	pos := -1
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] == 0x1b {
			pos = i
			break
		}
	}
	if pos < 0 {
		return false
	}
	tail := data[pos:]
	if len(tail) == 1 {
		return true
	}
	switch tail[1] {
	case '[':
		for _, b := range tail[2:] {
			if b >= 0x40 && b <= 0x7E {
				return false
			}
		}
		return true
	case ']':
		for i := 0; i < len(tail)-1; i++ {
			if tail[i] == 0x07 {
				return false
			}
			if tail[i] == 0x1b && tail[i+1] == '\\' {
				return false
			}
		}
		return true
	}
	return false
}

// endsMidUTF8 reports whether data ends with an incomplete UTF-8 character.
func endsMidUTF8(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	i := len(data) - 1
	for i > 0 && (data[i]&0xC0) == 0x80 {
		i--
	}
	b := data[i]
	var expected int
	switch {
	case b < 0x80:
		expected = 1
	case b < 0xE0:
		expected = 2
	case b < 0xF0:
		expected = 3
	default:
		expected = 4
	}
	return (len(data) - i) < expected
}
