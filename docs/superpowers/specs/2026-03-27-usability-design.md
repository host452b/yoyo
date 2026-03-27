# yoyo — Usability Improvements Design Spec

**Date:** 2026-03-27
**Status:** Approved
**Summary:** Two usability improvements — live countdown in the status bar so users can see remaining approval time, and an exit summary so users know how many prompts were auto-approved during the session. Lightweight observability via README log viewing documentation.

---

## 1. Problem Statement

### 1.1 Countdown invisibility

When a permission prompt is detected, the status bar shows:

```
[yoyo: on 3s | Claude]
```

`3s` is the **configured delay**, not the remaining time. It never changes during the countdown. Users cannot tell:
- Whether a countdown is currently running
- How many seconds remain before auto-approval fires
- When to intervene by pressing a key

### 1.2 No exit summary

After a session ends, users have no confirmation that yoyo did anything. There is no record of how many prompts were auto-approved without consulting the raw log file.

### 1.3 Log file not documented

The log file exists at `~/.yoyo/yoyo.log` but the README does not explain it exists, its format, or how to watch it live.

---

## 2. Design

### 2.1 Countdown Visibility

#### Display states

| State | Format | Example |
|-------|--------|---------|
| Idle, auto-approve on | `[yoyo: on Xs]` | `[yoyo: on 3s]` |
| Idle, auto-approve on, rule shown | `[yoyo: on Xs \| Rule]` | `[yoyo: on 3s \| Claude]` |
| **Countdown active** | `[yoyo: Ns \| Rule]` | `[yoyo: 2s \| Claude]` |
| Seen (immediate approval) | `[yoyo: on 0s \| seen: Rule]` | unchanged |
| Auto-approve off | `[yoyo: off]` | unchanged |

During countdown, `"on "` is removed and the displayed number ticks down each second. The visual change is deliberate: users learn to read `"on Xs"` as idle and `"Xs | Rule"` as active countdown.

#### StatusBar changes (`internal/statusbar/statusbar.go`)

New fields:
```go
countdown   int  // remaining seconds during active countdown
inCountdown bool // true while a countdown is running
```

New methods:
```go
// SetCountdown sets remaining seconds and enters countdown display mode.
func (sb *StatusBar) SetCountdown(remaining int)

// ClearCountdown exits countdown display mode and restores normal display.
func (sb *StatusBar) ClearCountdown()
```

`labelText()` new branch (highest priority, checked before rule/idle):
```go
if sb.inCountdown {
    return fmt.Sprintf(" [yoyo: %ds | %s] ", sb.countdown, sb.rule)
}
```

All methods are mutex-protected (existing pattern).

#### Proxy changes (`internal/proxy/proxy.go`)

New variables in `Run()`:
```go
var countdownTicker    *time.Ticker
var tickerCh           <-chan time.Time
var countdownRemaining int
```

**On prompt detection (new timer start):**
```go
// Stop any existing ticker
if countdownTicker != nil {
    countdownTicker.Stop()
    cfg.StatusBar.ClearCountdown()
}
// Start approval timer (existing)
approvalTimer = time.NewTimer(time.Duration(delaySecs) * time.Second)
timerCh = approvalTimer.C
// Start countdown ticker
countdownRemaining = delaySecs
cfg.StatusBar.SetCountdown(countdownRemaining)
countdownTicker = time.NewTicker(time.Second)
tickerCh = countdownTicker.C
```

Only started when `delaySecs > 0`. Immediate approval (`delaySecs == 0`) skips the ticker.

**New select case:**
```go
case <-tickerCh:
    countdownRemaining--
    if countdownRemaining > 0 {
        cfg.StatusBar.SetCountdown(countdownRemaining)
        stdout.Write(cfg.StatusBar.WrapFrame([]byte{}))
    }
    // When countdownRemaining == 0, approvalTimer fires imminently — let it clear
```

**On approval timer fire** (existing `case <-timerCh`): stop ticker, call `ClearCountdown()`.

**On user keypress cancel** (existing cancel block): stop ticker, call `ClearCountdown()`.

**On inputCh/outputCh close** (existing two `closeDone()` paths): stop ticker, call `ClearCountdown()`.

#### Helper: stopTicker

To avoid repetition across four cleanup paths, a private helper:
```go
func stopTicker(ticker **time.Ticker, ch *<-chan time.Time) {
    if *ticker != nil {
        (*ticker).Stop()
        *ticker = nil
        *ch = nil
    }
}
```

---

### 2.2 Exit Summary

#### Memory changes (`internal/memory/memory.go`)

New field: `approvedCount int`

`Record(hash)` updated:
```go
func (m *Memory) Record(hash string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    if _, exists := m.seen[hash]; !exists {
        m.seen[hash] = struct{}{}
        m.approvedCount++
    }
}
```

New method:
```go
func (m *Memory) ApprovedCount() int {
    m.mu.Lock()
    defer m.mu.Unlock()
    return m.approvedCount
}
```

#### Main changes (`cmd/yoyo/main.go`)

`memory.New()` assigned to a named variable `mem`:
```go
mem := memory.New()
pr := proxy.New(proxy.Config{
    ...
    Memory: mem,
    ...
})
if err := pr.Run(); err != nil {
    log.Errorf("proxy error: %v", err)
}
if count := mem.ApprovedCount(); count > 0 {
    fmt.Fprintf(os.Stderr, "[yoyo] session ended — auto-approved %d prompt(s)\n", count)
}
```

Printed to stderr only when `count > 0`. Silent exit if nothing was approved (does not pollute script/CI output).

---

### 2.3 README: Viewing Logs

New section added after **Config File**, before **Session Memory**:

````markdown
## Viewing Logs

yoyo logs every auto-approval event to `~/.yoyo/yoyo.log` (overridable with `-log`).

**Watch live during a session:**
```bash
tail -f ~/.yoyo/yoyo.log
```

**Example log output:**
```
[INFO] 2026-03-27 14:32:10.123 started claude (kind=claude, delay=3s)
[INFO] 2026-03-27 14:32:15.456 prompt detected: Claude
[INFO] 2026-03-27 14:32:18.456 approval timer fired, sending response for: Claude
[INFO] 2026-03-27 14:32:45.456 prompt detected: Claude
[INFO] 2026-03-27 14:32:45.457 user input during delay, cancelling approval
```

The log is appended across sessions. To start fresh:
```bash
> ~/.yoyo/yoyo.log
```
````

---

## 3. Files Changed

| File | Change |
|------|--------|
| `internal/statusbar/statusbar.go` | +2 fields, +2 methods (`SetCountdown`, `ClearCountdown`), `labelText` countdown branch |
| `internal/statusbar/statusbar_test.go` | +countdown rendering tests |
| `internal/proxy/proxy.go` | +ticker variables, +`tickerCh` select case, +`stopTicker` helper, 4 cleanup paths updated |
| `internal/proxy/proxy_e2e_test.go` | +countdown tick E2E scenario |
| `internal/memory/memory.go` | +`approvedCount` field, `Record` de-dup check, +`ApprovedCount()` |
| `internal/memory/memory_test.go` | +counter tests |
| `cmd/yoyo/main.go` | `mem` named variable, exit summary print |
| `README.md` | +Viewing Logs section |

---

## 4. Testing

### StatusBar
- `TestStatusBar_CountdownDisplay`: verify `labelText()` shows remaining seconds without "on" when `inCountdown=true`
- `TestStatusBar_ClearCountdown`: verify reverts to normal display after `ClearCountdown()`
- `TestStatusBar_CountdownZeroRemainingDoesNotShowOn`: edge case: `remaining=0` should not render as idle

### Memory
- `TestMemory_ApprovedCount_IncrementOnNewRecord`: each new hash increments count
- `TestMemory_ApprovedCount_NoDuplicateCount`: recording same hash twice counts once
- `TestMemory_ApprovedCount_ZeroInitial`: starts at 0

### Proxy E2E
- `TestProxy_CountdownTicksVisibleInStatusBar`: prompt detected with delay=2, verify status bar updated at t=1s with `1s` remaining before approval fires

---

## 5. Out of Scope

- Countdown for `delaySecs == 0` (immediate): not applicable
- Persistent approval log across sessions: not in scope (session memory resets by design)
- `yoyo log` subcommand: not in scope (user chose lightweight option A)
- `yoyo rules --test`: not in scope
