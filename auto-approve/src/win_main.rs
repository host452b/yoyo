use std::fs::File;
use std::time::Duration;

use clap::Parser;
use tracing_subscriber::fmt;
use tracing_subscriber::prelude::*;

use crate::cli::Args;
use crate::detector;
use crate::screen::Screen;
use crate::{win_proxy, win_pty, win_terminal};

struct ConsoleGuard {
    original: u32,
}

impl Drop for ConsoleGuard {
    fn drop(&mut self) {
        let _ = win_terminal::restore_console_mode(self.original);
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

    // Ensure ConPTY will use UTF-8 encoding so the vt100 parser can
    // correctly decode box-drawing characters used in permission prompts.
    win_terminal::set_utf8_codepage();

    // Get current console mode and window size
    let original_mode = win_terminal::get_console_mode()?;
    let winsize = win_terminal::get_winsize()?;

    // Determine agent kind and create screen buffer
    let approve_delay = Duration::from_secs_f32(args.delay);
    let mut agent = crate::agent::Agent::new(
        detector::AgentKind::from_command(&args.command[0]),
        winsize.rows,
        winsize.cols,
        approve_delay,
    );
    let mut screen = Screen::new(winsize.rows, winsize.cols);

    // Spawn child in ConPTY
    let mut pty_child = win_pty::spawn(&args.command, winsize.cols, winsize.rows)?;

    // Enable raw mode
    win_terminal::enable_raw_mode(original_mode)?;
    let _guard = ConsoleGuard {
        original: original_mode,
    };

    // Run the proxy loop
    let exit_code = win_proxy::run_proxy_loop(
        &mut pty_child,
        &mut screen,
        &mut agent,
    )?;

    drop(_guard);

    Ok(exit_code)
}
