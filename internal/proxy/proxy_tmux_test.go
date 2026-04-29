//go:build !windows

// proxy_tmux_test.go — tmux integration and exploratory tests.
//
// These tests spin up isolated headless tmux sessions (private socket) to
// verify that yoyo's detector and proxy work correctly in a real tmux
// environment. All tests skip in -short mode and when tmux is not found.
//
// Run with:
//
//	go test -v -run TestTmux ./internal/proxy/
package proxy_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/host452b/yoyo/internal/detector"
)

// ── tmux session helper ───────────────────────────────────────────────────────

// tmuxH manages a headless tmux session via a private socket so tests never
// interfere with the user's running tmux server.
type tmuxH struct {
	t       *testing.T
	socket  string
	session string
}

func newTmuxH(t *testing.T, cols, rows int) *tmuxH {
	t.Helper()
	if testing.Short() {
		t.Skip("tmux: skipped in -short mode")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found in PATH:", err)
	}
	h := &tmuxH{
		t:       t,
		socket:  filepath.Join(t.TempDir(), "tmux.sock"),
		session: "yoyo",
	}
	out, err := exec.Command("tmux", "-S", h.socket,
		"new-session", "-d", "-s", h.session,
		"-x", fmt.Sprint(cols), "-y", fmt.Sprint(rows)).CombinedOutput()
	if err != nil {
		t.Skipf("tmux new-session: %v — %s", err, out)
	}
	t.Cleanup(func() { exec.Command("tmux", "-S", h.socket, "kill-server").Run() })
	return h
}

// run types cmd in the pane and presses Enter (like a user would).
func (h *tmuxH) run(cmd string) {
	exec.Command("tmux", "-S", h.socket, "send-keys", "-t", h.session, cmd, "Enter").Run()
}

// capture returns the full visible text of the pane (trailing blank rows stripped).
func (h *tmuxH) capture() string {
	out, _ := exec.Command("tmux", "-S", h.socket,
		"capture-pane", "-t", h.session, "-p").Output()
	return strings.TrimRight(string(out), "\n")
}

// paneSize returns the (cols, rows) tmux reports for the pane.
func (h *tmuxH) paneSize() (cols, rows int) {
	out, _ := exec.Command("tmux", "-S", h.socket,
		"display-message", "-t", h.session, "-p", "#{pane_width} #{pane_height}").Output()
	fmt.Sscan(strings.TrimSpace(string(out)), &cols, &rows)
	return
}

// waitFor polls capture() until it contains want or timeout expires.
func (h *tmuxH) waitFor(want string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(h.capture(), want) {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// catFile writes content to a temp file and then runs `cat file` in the pane,
// avoiding all shell-quoting issues with Unicode or special characters.
func (h *tmuxH) catFile(content string) {
	f := filepath.Join(h.t.TempDir(), "tmux_content.txt")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		h.t.Fatal("catFile WriteFile:", err)
	}
	h.run("cat " + f)
}

// ── dialog fixtures ───────────────────────────────────────────────────────────

const (
	// Standard 2-option dialog.
	tmuxDlg2Opt = "─────────────────────────────────────────────\n" +
		" Read /etc/hosts\n\n" +
		"   1. Yes\n" +
		"   2. No\n\n" +
		" Esc to cancel · Tab to amend\n"

	// 3-option dialog with "don't ask again" — detector should pick option 2.
	tmuxDlg3Opt = "─────────────────────────────────────────────\n" +
		" Bash command\n\n" +
		"   pip3 show python-dotenv 2>/dev/null\n\n" +
		" Do you want to proceed?\n" +
		" ❯ 1. Yes\n" +
		"   2. Yes, and don't ask again for: pip3 show *\n" +
		"   3. No\n\n" +
		" Esc to cancel · Tab to amend\n"

	// Compact dialog — shorter separator matches a narrower pane.
	tmuxDlgNarrow = "─────────────────────────────────────\n" +
		" Write /tmp/out.txt\n\n" +
		"   1. Yes\n" +
		"   2. No\n\n" +
		" Esc to cancel\n"
)

// ── exploratory tests ─────────────────────────────────────────────────────────

// TestTmux_Probe_TermEnvironment is an exploratory test that reports what
// the terminal environment looks like inside a tmux pane. Run with -v.
func TestTmux_Probe_TermEnvironment(t *testing.T) {
	h := newTmuxH(t, 120, 40)

	h.run("echo TERM=$TERM")
	h.run("echo COLORTERM=$COLORTERM")
	h.run("stty size")
	time.Sleep(400 * time.Millisecond)

	pane := h.capture()
	cols, rows := h.paneSize()
	t.Logf("tmux pane dimensions: %d cols × %d rows", cols, rows)

	for _, line := range strings.Split(pane, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "TERM=") || strings.HasPrefix(l, "COLORTERM=") {
			t.Logf("  %s", l)
		}
		// stty size prints "rows cols"
		if l != "" {
			var a, b int
			if n, _ := fmt.Sscan(l, &a, &b); n == 2 && b > 0 {
				t.Logf("  stty size: rows=%d cols=%d", a, b)
			}
		}
	}
}

// TestTmux_PaneSize_HonorsRequest verifies that a detached tmux session
// respects the -x/-y dimensions passed at creation time.
func TestTmux_PaneSize_HonorsRequest(t *testing.T) {
	const wantCols, wantRows = 80, 24
	h := newTmuxH(t, wantCols, wantRows)

	cols, rows := h.paneSize()
	t.Logf("requested %dx%d → got %dx%d from tmux", wantCols, wantRows, cols, rows)
	if cols != wantCols || rows != wantRows {
		t.Errorf("pane size = %dx%d, want %dx%d", cols, rows, wantCols, wantRows)
	}

	// Double-check via stty inside the pane (stty reports "rows cols").
	h.run("stty size")
	want := fmt.Sprintf("%d %d", wantRows, wantCols)
	if !h.waitFor(want, 2*time.Second) {
		t.Errorf("stty did not report %q within 2s; pane:\n%s", want, h.capture())
	}
}

// ── detector-on-pane-capture tests ───────────────────────────────────────────

// TestTmux_ClaudeDetector_2Option injects a 2-option dialog into a tmux pane,
// captures the pane text, and verifies our detector matches. This confirms that
// tmux's line-padding and TERM rendering do not break detection.
func TestTmux_ClaudeDetector_2Option(t *testing.T) {
	h := newTmuxH(t, 120, 40)
	h.catFile(tmuxDlg2Opt)
	time.Sleep(300 * time.Millisecond)

	pane := h.capture()
	t.Logf("pane capture:\n%s", pane)

	r := detector.Claude{}.Detect(pane)
	if r == nil {
		t.Fatal("Claude detector did not match 2-option dialog in tmux pane capture")
	}
	t.Logf("matched: RuleName=%q Response=%q Hash=%s", r.RuleName, r.Response, r.Hash)
	if r.Response != "\r" {
		t.Errorf("Response = %q, want \"\\r\"", r.Response)
	}
}

// TestTmux_ClaudeDetector_3Option injects a 3-option dialog ("don't ask again")
// into a tmux pane and verifies the detector selects option 2 (↓+Enter).
func TestTmux_ClaudeDetector_3Option(t *testing.T) {
	h := newTmuxH(t, 120, 40)
	h.catFile(tmuxDlg3Opt)
	time.Sleep(300 * time.Millisecond)

	pane := h.capture()
	t.Logf("pane capture:\n%s", pane)

	r := detector.Claude{}.Detect(pane)
	if r == nil {
		t.Fatal("Claude detector did not match 3-option dialog in tmux pane capture")
	}
	t.Logf("matched: RuleName=%q Response=%q Hash=%s", r.RuleName, r.Response, r.Hash)
	if r.Response != "\x1b[B\r" {
		t.Errorf("Response = %q, want \"\\x1b[B\\r\" (↓+Enter for don't-ask-again)", r.Response)
	}
}

// TestTmux_ClaudeDetector_NarrowPane tests detection in a 60-column pane.
// Claude Code may render shorter separators in narrow panes; the detector
// must still match.
func TestTmux_ClaudeDetector_NarrowPane(t *testing.T) {
	h := newTmuxH(t, 60, 24)
	h.catFile(tmuxDlgNarrow)
	time.Sleep(300 * time.Millisecond)

	pane := h.capture()
	cols, rows := h.paneSize()
	t.Logf("narrow pane: %dx%d", cols, rows)
	t.Logf("pane capture:\n%s", pane)

	r := detector.Claude{}.Detect(pane)
	if r == nil {
		t.Fatal("Claude detector did not match dialog in narrow (60-col) tmux pane")
	}
	t.Logf("matched: Response=%q Hash=%s", r.Response, r.Hash)
}

// TestTmux_ClaudeDetector_MultipleDialogs verifies the detector picks the LAST
// dialog when multiple appear on screen (e.g. tmux scroll buffer showing history).
func TestTmux_ClaudeDetector_MultipleDialogs(t *testing.T) {
	h := newTmuxH(t, 120, 50) // tall pane to fit both dialogs

	// First: a Read dialog.
	h.catFile(tmuxDlg2Opt)
	time.Sleep(150 * time.Millisecond)
	// Second (newer): a Bash dialog that has "don't ask again".
	h.catFile(tmuxDlg3Opt)
	time.Sleep(300 * time.Millisecond)

	pane := h.capture()
	t.Logf("pane with two dialogs:\n%s", pane)

	r := detector.Claude{}.Detect(pane)
	if r == nil {
		t.Fatal("detector found no dialog in pane with two dialogs")
	}
	t.Logf("matched: Response=%q Hash=%s", r.Response, r.Hash)
	// The LAST dialog has "don't ask again" → expect ↓+Enter.
	if r.Response != "\x1b[B\r" {
		t.Errorf("Response = %q; want \"\\x1b[B\\r\" (should match the last/newest dialog)", r.Response)
	}
}

// ── live approval test ────────────────────────────────────────────────────────

// TestTmux_LiveApproval builds the yoyo binary, runs it inside a real tmux pane
// wrapping a bash dialog-simulator script, and verifies yoyo auto-approves.
// This is the most integrated test: real PTY → real yoyo process → real tmux.
func TestTmux_LiveApproval(t *testing.T) {
	if testing.Short() {
		t.Skip("tmux live approval: skipped in -short mode")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not in PATH")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not in PATH (needed to build yoyo binary)")
	}

	// Build yoyo binary into a temp dir.
	// The working directory for this test is internal/proxy/, so ../../cmd/yoyo/
	// points to the module root's cmd/yoyo.
	bin := filepath.Join(t.TempDir(), "yoyo-test")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/yoyo/")
	if out, err := build.CombinedOutput(); err != nil {
		t.Skipf("go build failed: %v\n%s", err, out)
	}

	// Result file: the dialog script creates it when it gets the approval Enter.
	resultFile := filepath.Join(t.TempDir(), "result.txt")

	// Dialog simulator: prints a Claude dialog, waits for Enter (yoyo's approval),
	// then writes a marker file to signal success.
	scriptFile := filepath.Join(t.TempDir(), "dialog.sh")
	script := fmt.Sprintf(`#!/bin/bash
set -e
printf '─────────────────────────────────────────────\n Read /tmp/hosts-test\n\n   1. Yes\n   2. No\n\n Esc to cancel\n'
read -r _ans
echo "APPROVED" > %s
`, resultFile)
	if err := os.WriteFile(scriptFile, []byte(script), 0755); err != nil {
		t.Fatal("WriteFile dialog.sh:", err)
	}

	// Dedicated tmux socket + session.
	sock := filepath.Join(t.TempDir(), "tmux-live.sock")
	sess := "live"
	if out, err := exec.Command("tmux", "-S", sock,
		"new-session", "-d", "-s", sess, "-x", "120", "-y", "40").CombinedOutput(); err != nil {
		t.Skipf("tmux new-session: %v — %s", err, out)
	}
	t.Cleanup(func() { exec.Command("tmux", "-S", sock, "kill-server").Run() })

	logFile := filepath.Join(t.TempDir(), "yoyo.log")

	// Launch: yoyo -delay 0 -log <log> -- bash dialog.sh
	yoyoCmd := fmt.Sprintf("%s -delay 0 -log %s -- bash %s", bin, logFile, scriptFile)
	exec.Command("tmux", "-S", sock, "send-keys", "-t", sess, yoyoCmd, "Enter").Run()

	// Poll for the result file (up to 8 seconds).
	deadline := time.Now().Add(8 * time.Second)
	approved := false
	for time.Now().Before(deadline) {
		if _, err := os.Stat(resultFile); err == nil {
			approved = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Capture diagnostics regardless of outcome.
	pane, _ := exec.Command("tmux", "-S", sock, "capture-pane", "-t", sess, "-p").Output()
	t.Logf("pane at end:\n%s", strings.TrimRight(string(pane), "\n"))
	if logData, err := os.ReadFile(logFile); err == nil && len(logData) > 0 {
		t.Logf("yoyo log (last 20 lines):\n%s", lastNLines(string(logData), 20))
	}

	if !approved {
		t.Error("yoyo did not auto-approve the dialog within 8s (result file not created)")
	} else {
		t.Log("yoyo auto-approved the dialog in a live tmux pane")
	}
}

// lastNLines returns up to n trailing lines of s.
func lastNLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
