use std::fs::File;
use std::io;
use std::os::fd::AsFd;
use std::time::Duration;

use clap::Parser;
use nix::sys::termios::Termios;
use tracing_subscriber::fmt;
use tracing_subscriber::prelude::*;

use crate::cli::Args;
use crate::detector;
use crate::screen::Screen;
use crate::{proxy, pty, signal, terminal};

struct TerminalGuard {
    original: Termios,
}

impl Drop for TerminalGuard {
    fn drop(&mut self) {
        terminal::restore_terminal(io::stdin().as_fd(), &self.original);
    }
}

pub fn run() -> Result<i32, Box<dyn std::error::Error>> {
    let args = Args::parse();

    // Set up file-based tracing if --log is provided
    if let Some(ref log_path) = args.log {
        let file = File::create(log_path)?;
        let file_layer = fmt::layer()
            .with_writer(file)
            .with_ansi(false)
            .with_target(false);
        tracing_subscriber::registry().with(file_layer).init();
    }

    if args.command.is_empty() {
        return Err("no command specified".into());
    }

    let stdin_handle = io::stdin();
    let stdin_fd = stdin_handle.as_fd();

    // Get current terminal state before we modify anything
    let original_termios = terminal::get_termios(stdin_fd)?;
    let winsize = terminal::get_winsize(stdin_fd)?;

    // Determine agent kind and create screen buffer
    let approve_delay = Duration::from_secs_f32(args.delay);
    let mut agent = crate::agent::Agent::new(
        detector::AgentKind::from_command(&args.command[0]),
        winsize.rows,
        winsize.cols,
        approve_delay,
    );
    let mut screen = Screen::new(winsize.rows, winsize.cols);

    // Spawn child in PTY
    let pty_child = pty::spawn(&args.command, &original_termios, &winsize)?;
    let master_fd = pty_child.master;
    let mut child = pty_child.child;

    // Set up SIGWINCH pipe (before raw mode)
    let sigwinch_fd = signal::setup_sigwinch_pipe()?;

    // Enable raw mode — this is the point of no return.
    let saved = terminal::enable_raw_mode(stdin_fd)?;
    let _guard = TerminalGuard { original: saved };

    // Run the proxy loop
    let child_pid = child.id();
    let _ = proxy::run_proxy_loop(
        stdin_fd,
        master_fd.as_fd(),
        sigwinch_fd.as_fd(),
        &mut screen,
        &mut agent,
        child_pid,
        &original_termios,
    );

    // Drop guard restores terminal before we wait
    drop(_guard);

    // Wait for child and get exit code
    let status = child.wait()?;
    Ok(status.code().unwrap_or(1))
}
