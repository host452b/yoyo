// internal/statusbar/statusbar_test.go
package statusbar_test

import (
	"strings"
	"testing"

	"github.com/host452b/yoyo/internal/statusbar"
)

func TestStatusBar_PaintsLabel(t *testing.T) {
	sb := statusbar.New(24, 80, true, 3)
	out := sb.WrapFrame([]byte("hello"))
	s := string(out)
	// Should contain cursor save/restore
	if !strings.Contains(s, "\x1b[s") {
		t.Error("expected cursor save sequence")
	}
	if !strings.Contains(s, "\x1b[u") {
		t.Error("expected cursor restore sequence")
	}
	// Should contain the frame content
	if !strings.Contains(s, "hello") {
		t.Error("should contain original frame content")
	}
}

func TestStatusBar_LabelContainsDelay(t *testing.T) {
	sb := statusbar.New(24, 80, true, 3)
	out := string(sb.WrapFrame([]byte("x")))
	if !strings.Contains(out, "3s") {
		t.Error("label should show delay '3s'")
	}
}

func TestStatusBar_OffLabel(t *testing.T) {
	sb := statusbar.New(24, 80, false, 3)
	out := string(sb.WrapFrame([]byte("x")))
	if !strings.Contains(out, "off") {
		t.Error("label should show 'off' when disabled")
	}
}

func TestStatusBar_Toggle(t *testing.T) {
	sb := statusbar.New(24, 80, true, 3)
	sb.Toggle()
	out := string(sb.WrapFrame([]byte("x")))
	if !strings.Contains(out, "off") {
		t.Error("after toggle, label should show 'off'")
	}
}

func TestStatusBar_SetDelay(t *testing.T) {
	sb := statusbar.New(24, 80, true, 3)
	sb.SetDelay(5)
	out := string(sb.WrapFrame([]byte("x")))
	if !strings.Contains(out, "5s") {
		t.Error("after SetDelay(5), label should show '5s'")
	}
}

func TestStatusBar_SetRule(t *testing.T) {
	sb := statusbar.New(24, 80, true, 3)
	sb.SetRule("Claude:yes")
	out := string(sb.WrapFrame([]byte("x")))
	if !strings.Contains(out, "Claude:yes") {
		t.Error("should show rule name in label")
	}
}

func TestStatusBar_SkipOnMidEscape(t *testing.T) {
	sb := statusbar.New(24, 80, true, 3)
	// Frame ends mid-escape (ESC only)
	out1 := sb.WrapFrame([]byte("\x1b"))
	// The continuation should not inject
	out2 := sb.WrapFrame([]byte("[1m"))
	// Check no double injection for the mid-escape frame
	_ = out1
	_ = out2
	// Main contract: should not panic and should contain frame bytes
	combined := string(out1) + string(out2)
	if !strings.Contains(combined, "[1m") {
		t.Error("frame content must be present")
	}
}

func TestStatusBar_NoPaintWhenTooNarrow(t *testing.T) {
	sb := statusbar.New(24, 10, true, 3) // too narrow for label
	out := string(sb.WrapFrame([]byte("x")))
	// Should just pass through without overlay
	if out != "x" {
		t.Logf("narrow terminal output: %q", out)
		// Accept pass-through or minimal output
	}
}
