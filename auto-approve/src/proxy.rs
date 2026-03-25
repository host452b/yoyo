use std::io;
use std::os::fd::{AsRawFd, BorrowedFd};
use std::time::{Duration, Instant};

use nix::libc;
use nix::poll::{self, PollFd, PollFlags, PollTimeout};
use nix::sys::signal::{self, SigHandler, Signal};
use nix::sys::termios::Termios;
use nix::unistd::{self, Pid};
use tracing::info;

use crate::agent::Agent;
use crate::proxy_state::{is_escape_sequence, ApprovalAction, ProxyState};
use crate::screen::Screen;
use crate::terminal;

/// Raw byte for the prefix key (Ctrl+P = 0x10).
const PREFIX_KEY: u8 = 0x10;

/// Ctrl+P prefix key timeout.
const PREFIX_TIMEOUT: Duration = Duration::from_millis(1500);

/// Maximum time to drain PTY output after forwarding `0x1a` to the child.
const SUSPEND_TIMEOUT: Duration = Duration::from_millis(500);

/// Duration of PTY silence that indicates the child has stopped writing.
const SILENCE_TIMEOUT: Duration = Duration::from_millis(100);

/// Compute the poll timeout as the minimum time until any active deadline.
/// Returns `NONE` (block forever) when no deadlines are active.
fn next_timeout(state: &ProxyState<'_>) -> PollTimeout {
    let now = Instant::now();
    let deadlines = [
        state.pending.as_ref().map(|p| p.deadline),
        state.prefix_active.map(|t| t + PREFIX_TIMEOUT),
    ];
    match deadlines
        .iter()
        .filter_map(|d| d.map(|t| t.saturating_duration_since(now)))
        .min()
    {
        None => PollTimeout::NONE,
        Some(d) if d.is_zero() => PollTimeout::from(0u16),
        Some(d) => {
            let ms = (d.as_millis() as u16).max(1);
            PollTimeout::from(ms)
        }
    }
}

// ── Event handlers ──────────────────────────────────────────────────────────

/// Handle SIGWINCH — forward window size change to child PTY, screen, status bar.
fn handle_sigwinch(
    stdin_fd: BorrowedFd<'_>,
    master_fd: BorrowedFd<'_>,
    sigwinch_fd: BorrowedFd<'_>,
    state: &mut ProxyState<'_>,
) {
    // Drain the signal pipe
    let mut drain = [0u8; 64];
    while let Ok(n) = unistd::read(sigwinch_fd, &mut drain) {
        if n < drain.len() {
            break;
        }
    }
    if let Ok(ws) = terminal::get_winsize(stdin_fd) {
        let _ = terminal::set_winsize(master_fd, &ws);
        state.screen.resize(ws.rows, ws.cols);
        state.agent.status_bar.resize(ws.rows, ws.cols);
    }
}

/// Process the Ctrl+P prefix key state machine.
/// Returns the remaining data to forward to the child (may be empty → caller should `continue`).
/// Returns `None` if the entire input was consumed by the prefix logic.
fn handle_prefix<'a>(
    data: &'a [u8],
    master_fd: BorrowedFd<'_>,
    state: &mut ProxyState<'_>,
) -> io::Result<Option<&'a [u8]>> {
    // Extract (command_byte, rest_including_cmd, rest_after_cmd) from the prefix state machine.
    let (cmd, with_cmd, rest) = if state.prefix_active.is_some() {
        state.prefix_active = None;
        (data[0], data, &data[1..])
    } else if data[0] == PREFIX_KEY {
        if data.len() == 1 {
            state.prefix_active = Some(Instant::now());
            return Ok(None);
        }
        (data[1], &data[1..], &data[2..])
    } else {
        return Ok(Some(data));
    };

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
        _ => {
            write_all_raw(master_fd, &[PREFIX_KEY])?;
            return Ok(Some(with_cmd));
        }
    }
    // Immediate repaint with updated state.
    let paint = state.agent.status_bar.wrap_frame(&[]);
    if !paint.is_empty() {
        write_all_stdout(&paint)?;
    }
    Ok(if rest.is_empty() { None } else { Some(rest) })
}

/// Handle stdin data — prefix keys, Ctrl+Z suspend, typing suppression, forward to PTY.
fn handle_stdin(
    stdin_buf: &[u8],
    stdin_fd: BorrowedFd<'_>,
    master_fd: BorrowedFd<'_>,
    child_pid: u32,
    original_termios: &Termios,
    state: &mut ProxyState<'_>,
) -> io::Result<bool> {
    let data = match handle_prefix(stdin_buf, master_fd, state)? {
        Some(d) => d,
        None => return Ok(true), // consumed by prefix logic
    };

    // Intercept Ctrl+Z (0x1a) — perform suspend/resume cycle
    if data.contains(&0x1a) {
        suspend_and_resume(stdin_fd, master_fd, child_pid, original_termios, state.screen);
        return Ok(true);
    }

    if !is_escape_sequence(data) {
        if state.pending.is_some() {
            info!("user input during approval delay, aborting auto-approval");
            state.pending = None;
        }
    }
    write_all_raw(master_fd, data)?;
    Ok(false)
}

/// Handle PTY output — feed screen, detect prompts, auto-approve, write to stdout.
fn handle_pty_output(
    master_fd: BorrowedFd<'_>,
    buf: &mut [u8; 4096],
    state: &mut ProxyState<'_>,
) -> io::Result<bool> {
    // Drain all available data (non-blocking) to get complete frames
    let mut frame = Vec::new();
    let mut eof = false;
    loop {
        match unistd::read(master_fd, &mut buf[..]) {
            Ok(0) => { eof = true; break; }
            Ok(n) => frame.extend_from_slice(&buf[..n]),
            Err(nix::errno::Errno::EAGAIN) => break,
            Err(nix::errno::Errno::EIO) => { eof = true; break; }
            Err(nix::errno::Errno::EINTR) => continue,
            Err(e) => return Err(io::Error::from(e)),
        }
    }
    if frame.is_empty() {
        return Ok(eof);
    }

    match state.process_frame(&frame) {
        ApprovalAction::SendCR => write_all_raw(master_fd, b"\r")?,
        ApprovalAction::StartTimer(p) => state.pending = Some(p),
        ApprovalAction::None => {}
    }
    let wrapped = state.agent.status_bar.wrap_frame(&frame);
    write_all_stdout(&wrapped)?;
    Ok(eof)
}

/// Handle POLLHUP — drain remaining PTY output.
fn handle_pty_hup(master_fd: BorrowedFd<'_>, screen: &mut Screen) {
    let mut buf = [0u8; 4096];
    loop {
        match unistd::read(master_fd, &mut buf) {
            Ok(0) | Err(_) => break,
            Ok(n) => {
                let _ = write_all_stdout(&buf[..n]);
                screen.feed(&buf[..n]);
            }
        }
    }
}

// ── Main loop ───────────────────────────────────────────────────────────────

pub fn run_proxy_loop(
    stdin_fd: BorrowedFd<'_>,
    master_fd: BorrowedFd<'_>,
    sigwinch_fd: BorrowedFd<'_>,
    screen: &mut Screen,
    agent: &mut Agent,
    child_pid: u32,
    original_termios: &Termios,
) -> io::Result<()> {
    let mut buf = [0u8; 4096];
    let mut stdin_buf = [0u8; 128 * 1024];

    // Set master_fd to non-blocking so we can drain complete output frames
    unsafe {
        let flags = libc::fcntl(master_fd.as_raw_fd(), libc::F_GETFL);
        libc::fcntl(master_fd.as_raw_fd(), libc::F_SETFL, flags | libc::O_NONBLOCK);
    }

    let mut state = ProxyState::new(screen, agent);

    loop {
        let mut fds = [
            PollFd::new(stdin_fd, PollFlags::POLLIN),
            PollFd::new(master_fd, PollFlags::POLLIN),
            PollFd::new(sigwinch_fd, PollFlags::POLLIN),
        ];

        let timeout = next_timeout(&state);

        match poll::poll(&mut fds, timeout) {
            Ok(_) => {}
            Err(nix::errno::Errno::EINTR) => continue,
            Err(e) => return Err(io::Error::from(e)),
        }

        // Flush expired prefix key to child
        if let Some(t) = state.prefix_active {
            if t.elapsed() >= PREFIX_TIMEOUT {
                let _ = write_all_raw(master_fd, &[PREFIX_KEY]);
                state.prefix_active = None;
            }
        }

        // Fire pending auto-approval
        if let Some(ref p) = state.pending {
            if Instant::now() >= p.deadline {
                info!("auto-approval delay expired, sending CR");
                write_all_raw(master_fd, b"\r")?;
                state.pending = None;
            }
        }

        // SIGWINCH
        if let Some(revents) = fds[2].revents() {
            if revents.contains(PollFlags::POLLIN) {
                handle_sigwinch(stdin_fd, master_fd, sigwinch_fd, &mut state);
            }
        }

        // stdin → master
        if let Some(revents) = fds[0].revents() {
            if revents.contains(PollFlags::POLLIN) {
                match unistd::read(stdin_fd, &mut stdin_buf) {
                    Ok(0) => {}
                    Ok(n) => {
                        if handle_stdin(
                            &stdin_buf[..n], stdin_fd, master_fd,
                            child_pid, original_termios, &mut state,
                        )? {
                            continue;
                        }
                    }
                    Err(nix::errno::Errno::EIO | nix::errno::Errno::EINTR) => {}
                    Err(e) => return Err(io::Error::from(e)),
                }
            }
        }

        // master → stdout
        if let Some(revents) = fds[1].revents() {
            if revents.contains(PollFlags::POLLIN) {
                if handle_pty_output(master_fd, &mut buf, &mut state)? {
                    break;
                }
            }
            if revents.contains(PollFlags::POLLHUP) {
                handle_pty_hup(master_fd, state.screen);
                break;
            }
        }
    }

    Ok(())
}

// ── Suspend / resume ────────────────────────────────────────────────────────

/// Drain PTY master output to stdout until it goes silent for
/// [`SILENCE_TIMEOUT`] or [`deadline`] is reached.
fn drain_until_quiet(master_fd: BorrowedFd<'_>, deadline: Instant) {
    loop {
        let remaining = deadline.saturating_duration_since(Instant::now());
        if remaining.is_zero() {
            return;
        }
        let poll_ms = remaining.as_millis().min(SILENCE_TIMEOUT.as_millis()) as u16;
        let mut pfd = [PollFd::new(master_fd, PollFlags::POLLIN)];
        match poll::poll(&mut pfd, PollTimeout::from(poll_ms)) {
            Ok(0) => return,
            Ok(_) => drain_to_stdout(master_fd),
            Err(nix::errno::Errno::EINTR) => continue,
            Err(_) => return,
        }
    }
}

fn suspend_and_resume(
    stdin_fd: BorrowedFd<'_>,
    master_fd: BorrowedFd<'_>,
    child_pid: u32,
    original_termios: &Termios,
    screen: &mut Screen,
) {
    info!("Ctrl+Z detected, suspending");

    let child_pgid_neg = Pid::from_raw(-(child_pid as i32));

    let _ = write_all_raw(master_fd, &[0x1a]);

    let deadline = Instant::now() + SUSPEND_TIMEOUT;
    drain_until_quiet(master_fd, deadline);

    terminal::restore_terminal(stdin_fd, original_termios);

    unsafe { signal::signal(Signal::SIGTSTP, SigHandler::SigDfl) }.ok();
    let _ = signal::raise(Signal::SIGTSTP);

    // -- resumed by SIGCONT (user typed `fg`) --
    info!("resumed from suspend");

    unsafe { signal::signal(Signal::SIGTSTP, SigHandler::SigIgn) }.ok();
    let _ = terminal::enable_raw_mode(stdin_fd);
    let _ = signal::kill(child_pgid_neg, Signal::SIGCONT);

    if let Ok(ws) = terminal::get_winsize(stdin_fd) {
        let _ = terminal::set_winsize(master_fd, &ws);
        screen.resize(ws.rows, ws.cols);
    }
}

// ── I/O helpers ─────────────────────────────────────────────────────────────

fn drain_to_stdout(fd: BorrowedFd<'_>) {
    let mut buf = [0u8; 4096];
    loop {
        match unistd::read(fd, &mut buf) {
            Ok(n) if n > 0 => {
                let _ = write_all_stdout(&buf[..n]);
            }
            _ => break,
        }
    }
}

fn write_all_raw(fd: BorrowedFd<'_>, mut data: &[u8]) -> io::Result<()> {
    while !data.is_empty() {
        match unistd::write(fd, data) {
            Ok(n) => data = &data[n..],
            Err(nix::errno::Errno::EINTR) => continue,
            Err(nix::errno::Errno::EAGAIN) => {
                drain_to_stdout(fd);
                let mut pfd = [PollFd::new(fd, PollFlags::POLLOUT | PollFlags::POLLIN)];
                let _ = poll::poll(&mut pfd, PollTimeout::from(10u8));
                if let Some(revents) = pfd[0].revents() {
                    if revents.contains(PollFlags::POLLIN) {
                        drain_to_stdout(fd);
                    }
                }
            }
            Err(e) => return Err(io::Error::from(e)),
        }
    }
    Ok(())
}

fn write_all_stdout(data: &[u8]) -> io::Result<()> {
    use io::Write;
    let mut stdout = io::stdout();
    stdout.write_all(data)?;
    stdout.flush()
}
