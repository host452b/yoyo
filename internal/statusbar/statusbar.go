// internal/statusbar/statusbar.go
package statusbar

import (
	"fmt"
	"sync"
)

const (
	cursorSave    = "\x1b7"
	cursorRestore = "\x1b8"
	sgrReset      = "\x1b[0m"
	fgGreen       = "\x1b[32m"
	fgRed         = "\x1b[31m"
	fgYellow      = "\x1b[33m"
)

// labelWidth is the fixed width of the status label (widest possible label).
// " [yoyo: on Xs | RuleName]" — we reserve space for "on Xs" + rule.
// Keep it wide enough; truncate rule name if needed.
const minLabelWidth = 22 // " [yoyo: on Xs | ...]  " minimum

// StatusBar renders a bottom-right ANSI overlay around PTY output frames.
// All methods are goroutine-safe (SIGWINCH resize races with event loop writes).
type StatusBar struct {
	mu        sync.Mutex
	rows      uint16
	cols      uint16
	enabled   bool
	delaySecs int
	countdown int    // remaining seconds; -1 = no active countdown
	rule      string
	painted   bool
	midSeq    bool
	prefix    bool // true while waiting for Ctrl+Y command byte
	dryRun    bool
	buf       []byte // reusable output buffer
}

// New creates a StatusBar. enabled=true means auto-approve is active.
func New(rows, cols uint16, enabled bool, delaySecs int) *StatusBar {
	return &StatusBar{
		rows:      rows,
		cols:      cols,
		enabled:   enabled,
		delaySecs: delaySecs,
		countdown: -1,
	}
}

func (sb *StatusBar) Toggle() {
	sb.mu.Lock()
	sb.enabled = !sb.enabled
	sb.mu.Unlock()
}

func (sb *StatusBar) SetDelay(secs int) {
	sb.mu.Lock()
	sb.delaySecs = secs
	sb.mu.Unlock()
}

// SetRule sets the last-matched rule name shown in the label.
func (sb *StatusBar) SetRule(rule string) {
	sb.mu.Lock()
	sb.rule = rule
	sb.mu.Unlock()
}

// SetCountdown sets the remaining seconds shown during an active approval timer.
// Pass -1 to clear the countdown display.
func (sb *StatusBar) SetCountdown(secs int) {
	sb.mu.Lock()
	sb.countdown = secs
	sb.mu.Unlock()
}

// SetPrefix sets the Ctrl+Y prefix-waiting indicator.
func (sb *StatusBar) SetPrefix(active bool) {
	sb.mu.Lock()
	sb.prefix = active
	sb.mu.Unlock()
}

// SetDryRun enables dry-run mode display.
func (sb *StatusBar) SetDryRun(on bool) {
	sb.mu.Lock()
	sb.dryRun = on
	sb.mu.Unlock()
}

// Resize updates terminal dimensions.
func (sb *StatusBar) Resize(rows, cols uint16) {
	sb.mu.Lock()
	sb.rows = rows
	sb.cols = cols
	sb.mu.Unlock()
}

// WrapFrame injects clear-previous and paint-new ANSI sequences around frame.
// Returns a single buffer for one atomic write to stdout.
func (sb *StatusBar) WrapFrame(frame []byte) []byte {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	prevMid := sb.midSeq
	sb.midSeq = endsMidEscape(frame) || endsMidUTF8(frame)

	label := sb.labelText()
	lw := uint16(len(label))
	if sb.cols < lw+2 || sb.rows == 0 {
		return frame // terminal too narrow
	}

	col := sb.cols - lw

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
		color := sb.labelColor()
		paint = overlayAt(sb.rows, col, color, label)
	}

	// Reuse buffer to reduce allocations
	need := len(clear) + len(frame) + len(paint)
	if cap(sb.buf) < need {
		sb.buf = make([]byte, 0, need+256)
	}
	sb.buf = sb.buf[:0]
	sb.buf = append(sb.buf, clear...)
	sb.buf = append(sb.buf, frame...)
	sb.buf = append(sb.buf, paint...)
	return sb.buf
}

func (sb *StatusBar) labelColor() string {
	if sb.prefix {
		return fgYellow
	}
	if !sb.enabled {
		return fgRed
	}
	if sb.dryRun {
		return fgYellow
	}
	if sb.countdown >= 0 {
		return fgYellow // counting down — attention
	}
	return fgGreen
}

func (sb *StatusBar) labelText() string {
	if sb.prefix {
		return " [yoyo: ^Y …] "
	}
	if !sb.enabled {
		if sb.dryRun {
			return " [yoyo: dry off] "
		}
		return " [yoyo: off] "
	}
	mode := "on"
	if sb.dryRun {
		mode = "dry"
	}
	// Active countdown: show remaining seconds
	if sb.countdown >= 0 {
		if sb.rule == "" {
			return fmt.Sprintf(" [yoyo: %s %ds] ", mode, sb.countdown)
		}
		return fmt.Sprintf(" [yoyo: %s %ds | %s] ", mode, sb.countdown, sb.rule)
	}
	if sb.rule == "" {
		return fmt.Sprintf(" [yoyo: %s %ds] ", mode, sb.delaySecs)
	}
	return fmt.Sprintf(" [yoyo: %s %ds | %s] ", mode, sb.delaySecs, sb.rule)
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
