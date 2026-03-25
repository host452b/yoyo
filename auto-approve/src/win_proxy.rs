use std::io::{self, Read, Write};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::mpsc::{self, RecvTimeoutError};
use std::sync::Arc;
use std::time::{Duration, Instant};

use tracing::{debug, info};

use crate::agent::Agent;
use crate::proxy_state::{is_escape_sequence, ApprovalAction, ProxyState};
use crate::screen::Screen;
use crate::win_pty::PtyChild;
use crate::win_terminal;

/// How often to poll for console resize.
const RESIZE_POLL_INTERVAL: Duration = Duration::from_millis(250);

/// Raw byte for the prefix key (Ctrl+P = 0x10).
const PREFIX_KEY: u8 = 0x10;

/// Ctrl+P prefix key timeout.
const PREFIX_TIMEOUT: Duration = Duration::from_millis(1500);

enum ProxyEvent {
    StdinData(Vec<u8>),
    PtyData(Vec<u8>),
    PtyEof,
    Resize(u16, u16),
}

/// Compute the recv timeout as the minimum time until any active deadline.
/// Returns a large duration when no deadlines are active (block until event).
fn next_timeout(state: &ProxyState<'_>) -> Duration {
    let now = Instant::now();
    let deadlines = [
        state.pending.as_ref().map(|p| p.deadline),
        state.prefix_active.map(|t| t + PREFIX_TIMEOUT),
    ];
    deadlines
        .iter()
        .filter_map(|d| d.map(|t| t.saturating_duration_since(now)))
        .min()
        .unwrap_or(Duration::from_secs(86400))
}

/// Win32-input-mode key event flags.
const LEFT_CTRL_PRESSED: u32 = 0x0008;
const RIGHT_CTRL_PRESSED: u32 = 0x0004;
const CTRL_PRESSED: u32 = LEFT_CTRL_PRESSED | RIGHT_CTRL_PRESSED;

/// Virtual key code for `P` key.
const VK_P: u32 = 0x50;

/// Parsed win32-input-mode key event: `ESC [ Vk ; Sc ; Uc ; Kd ; Cs ; Rc _`
struct Win32KeyEvent {
    vk: u32,
    uc: u32,
    key_down: bool,
    ctrl_state: u32,
    /// Total byte length of the escape sequence in the input buffer.
    seq_len: usize,
}

/// Try to parse a win32-input-mode sequence at the start of `data`.
fn parse_win32_key(data: &[u8]) -> Option<Win32KeyEvent> {
    // Must start with ESC [ and end with _
    if data.len() < 6 || data[0] != 0x1b || data[1] != b'[' {
        return None;
    }
    let end = data.iter().position(|&b| b == b'_')?;
    let inner = std::str::from_utf8(&data[2..end]).ok()?;
    let parts: Vec<&str> = inner.split(';').collect();
    if parts.len() != 6 {
        return None;
    }
    Some(Win32KeyEvent {
        vk: parts[0].parse().ok()?,
        uc: parts[2].parse().ok()?,
        key_down: parts[3] == "1",
        ctrl_state: parts[4].parse().ok()?,
        seq_len: end + 1,
    })
}

// ── Event handlers ──────────────────────────────────────────────────────────

/// Execute a prefix command byte. Returns `true` if recognized.
fn exec_prefix_cmd(cmd: u8, state: &mut ProxyState<'_>) -> bool {
    match cmd {
        b'p' => {
            state.agent.toggle();
            if !state.agent.enabled {
                state.pending = None;
            }
        }
        b'0'..=b'9' => {
            if !state.agent.enabled {
                state.agent.toggle();
            }
            state.agent.set_delay((cmd - b'0') as u32);
            if state.agent.immediate() {
                state.pending = None;
            }
        }
        _ => return false,
    }
    // Immediate repaint with updated state.
    let paint = state.agent.status_bar.wrap_frame(&[]);
    if !paint.is_empty() {
        let mut stdout = io::stdout();
        let _ = stdout.write_all(&paint);
        let _ = stdout.flush();
    }
    true
}

/// Process the Ctrl+P prefix key state machine.
/// Handles both raw bytes (0x10) and win32-input-mode escape sequences.
/// Returns the data to forward to the child, or `None` if consumed by prefix logic.
fn handle_prefix(data: Vec<u8>, state: &mut ProxyState<'_>) -> Option<Vec<u8>> {
    // ── Win32-input-mode path ──────────────────────────────────────────
    if let Some(key) = parse_win32_key(&data) {
        // Only act on key-down events.
        if !key.key_down {
            return Some(data);
        }

        // Ctrl+P activates the prefix.
        if key.vk == VK_P && (key.ctrl_state & CTRL_PRESSED) != 0 {
            state.prefix_active = Some(Instant::now());
            // Drop the sequence (don't forward Ctrl+P to child).
            let rest = &data[key.seq_len..];
            return if rest.is_empty() { None } else { Some(rest.to_vec()) };
        }

        // If prefix is active, interpret this key-down as the command.
        if state.prefix_active.is_some() {
            state.prefix_active = None;
            if !exec_prefix_cmd(key.uc as u8, state) {
                return Some(data);
            }
            let rest = &data[key.seq_len..];
            return if rest.is_empty() { None } else { Some(rest.to_vec()) };
        }

        // Not a prefix key — forward as-is.
        return Some(data);
    }

    // ── Raw byte path (non-win32-input-mode) ───────────────────────────
    let (cmd, rest_from) = if state.prefix_active.is_some() {
        state.prefix_active = None;
        (data[0], 1)
    } else if data[0] == PREFIX_KEY {
        if data.len() == 1 {
            state.prefix_active = Some(Instant::now());
            return None;
        }
        (data[1], 2)
    } else {
        return Some(data);
    };

    if !exec_prefix_cmd(cmd, state) {
        let mut combined = vec![PREFIX_KEY];
        combined.extend_from_slice(&data[rest_from - 1..]);
        return Some(combined);
    }
    if rest_from < data.len() { Some(data[rest_from..].to_vec()) } else { None }
}

/// Handle stdin data — prefix keys, typing suppression, forward to PTY.
/// Returns true if the event was fully consumed (caller should `continue`).
fn handle_stdin(
    data: Vec<u8>,
    pty_input: &mut dyn Write,
    state: &mut ProxyState<'_>,
) -> bool {
    debug!("stdin bytes: {:02x?}", &data);
    let data = match handle_prefix(data, state) {
        Some(d) => d,
        None => return true,
    };
    // Don't cancel pending approval for escape sequences or win32-input-mode
    // key-up / modifier-only events.
    let is_passive = is_escape_sequence(&data)
        || parse_win32_key(&data).map_or(false, |k| !k.key_down || k.uc == 0);
    if !is_passive {
        if state.pending.is_some() {
            info!("user input during approval delay, aborting auto-approval");
            state.pending = None;
        }
    }
    let _ = pty_input.write_all(&data);
    let _ = pty_input.flush();
    false
}

/// Handle PTY output — feed screen, detect prompts, auto-approve, write to stdout.
fn handle_pty_output(
    frame: Vec<u8>,
    pty_input: &mut dyn Write,
    stdout: &mut dyn Write,
    state: &mut ProxyState<'_>,
) -> io::Result<()> {
    match state.process_frame(&frame) {
        ApprovalAction::SendCR => {
            let _ = pty_input.write_all(b"\r");
            let _ = pty_input.flush();
        }
        ApprovalAction::StartTimer(p) => state.pending = Some(p),
        ApprovalAction::None => {}
    }
    let wrapped = state.agent.status_bar.wrap_frame(&frame);
    stdout.write_all(&wrapped)?;
    stdout.flush()
}

fn handle_resize(
    cols: u16,
    rows: u16,
    pty_child: &mut PtyChild,
    state: &mut ProxyState<'_>,
) {
    pty_child.process.resize(cols as i16, rows as i16).ok();
    state.screen.resize(rows, cols);
    state.agent.status_bar.resize(rows, cols);
}

// ── Main loop ───────────────────────────────────────────────────────────────

pub fn run_proxy_loop(
    pty_child: &mut PtyChild,
    screen: &mut Screen,
    agent: &mut Agent,
) -> io::Result<i32> {
    let (tx, rx) = mpsc::channel::<ProxyEvent>();
    let shutdown = Arc::new(AtomicBool::new(false));

    // Thread 1: stdin reader
    let tx_stdin = tx.clone();
    let shutdown_stdin = Arc::clone(&shutdown);
    std::thread::spawn(move || {
        let mut buf = [0u8; 4096];
        let stdin = io::stdin();
        let mut handle = stdin.lock();
        while !shutdown_stdin.load(Ordering::Relaxed) {
            match handle.read(&mut buf) {
                Ok(0) => break,
                Ok(n) => {
                    let _ = tx_stdin.send(ProxyEvent::StdinData(buf[..n].to_vec()));
                }
                Err(_) => break,
            }
        }
    });

    // Thread 2: PTY output reader
    let tx_pty = tx.clone();
    let shutdown_pty = Arc::clone(&shutdown);
    let mut pty_output = pty_child.process.output().map_err(|e| {
        io::Error::new(io::ErrorKind::Other, e)
    })?;
    std::thread::spawn(move || {
        let mut buf = [0u8; 64 * 1024];
        loop {
            match pty_output.read(&mut buf) {
                Ok(0) => {
                    shutdown_pty.store(true, Ordering::Relaxed);
                    let _ = tx_pty.send(ProxyEvent::PtyEof);
                    break;
                }
                Ok(n) => {
                    let _ = tx_pty.send(ProxyEvent::PtyData(buf[..n].to_vec()));
                }
                Err(_) => {
                    shutdown_pty.store(true, Ordering::Relaxed);
                    let _ = tx_pty.send(ProxyEvent::PtyEof);
                    break;
                }
            }
        }
    });

    // Thread 3: resize poller
    let tx_resize = tx;
    let shutdown_resize = Arc::clone(&shutdown);
    std::thread::spawn(move || {
        let mut last_size = win_terminal::get_winsize().ok();
        while !shutdown_resize.load(Ordering::Relaxed) {
            std::thread::sleep(RESIZE_POLL_INTERVAL);
            if let Ok(current) = win_terminal::get_winsize() {
                let changed = match last_size {
                    Some(prev) => prev.cols != current.cols || prev.rows != current.rows,
                    None => true,
                };
                if changed {
                    last_size = Some(current);
                    let _ = tx_resize.send(ProxyEvent::Resize(current.cols, current.rows));
                }
            }
        }
    });

    let mut pty_input = pty_child.process.input().map_err(|e| {
        io::Error::new(io::ErrorKind::Other, e)
    })?;
    let mut stdout = io::stdout();

    let mut state = ProxyState::new(screen, agent);

    loop {
        let timeout = next_timeout(&state);

        match rx.recv_timeout(timeout) {
            Ok(event) => match event {
                ProxyEvent::StdinData(data) => {
                    if handle_stdin(data, &mut pty_input, &mut state) {
                        continue;
                    }
                }
                ProxyEvent::PtyData(data) => {
                    // Batch additional available frames
                    let mut frame = data;
                    while let Ok(ProxyEvent::PtyData(more)) = rx.try_recv() {
                        frame.extend(more);
                    }
                    handle_pty_output(frame, &mut pty_input, &mut stdout, &mut state)?;
                }
                ProxyEvent::Resize(cols, rows) => {
                    handle_resize(cols, rows, pty_child, &mut state);
                }
                ProxyEvent::PtyEof => break,
            },
            Err(RecvTimeoutError::Timeout) => {}
            Err(RecvTimeoutError::Disconnected) => break,
        }

        // Flush expired prefix key to child
        if let Some(t) = state.prefix_active {
            if t.elapsed() >= PREFIX_TIMEOUT {
                let _ = pty_input.write_all(&[PREFIX_KEY]);
                let _ = pty_input.flush();
                state.prefix_active = None;
            }
        }

        // Fire pending auto-approval
        if let Some(ref p) = state.pending {
            if Instant::now() >= p.deadline {
                info!("auto-approval delay expired, sending CR");
                let _ = pty_input.write_all(b"\r");
                let _ = pty_input.flush();
                state.pending = None;
            }
        }

        // Check if child process has exited (ConPTY may not send EOF promptly)
        if !pty_child.process.is_alive() {
            while let Ok(event) = rx.try_recv() {
                if let ProxyEvent::PtyData(data) = event {
                    let _ = stdout.write_all(&data);
                    let _ = stdout.flush();
                }
            }
            break;
        }
    }

    shutdown.store(true, Ordering::Relaxed);
    drop(pty_input);

    let exit_code = match pty_child.process.wait(Some(3000)) {
        Ok(code) => code as i32,
        Err(_) => {
            let _ = pty_child.process.exit(1);
            1
        }
    };

    Ok(exit_code)
}
