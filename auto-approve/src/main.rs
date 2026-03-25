mod agent;
mod cli;
mod detector;
mod proxy_state;
mod screen;
mod status_bar;

#[cfg(unix)]
mod proxy;
#[cfg(unix)]
mod pty;
#[cfg(unix)]
mod signal;
#[cfg(unix)]
mod terminal;
#[cfg(unix)]
mod unix_main;

#[cfg(windows)]
mod win_proxy;
#[cfg(windows)]
mod win_pty;
#[cfg(windows)]
mod win_terminal;
#[cfg(windows)]
mod win_main;

use std::process::ExitCode;

fn main() -> ExitCode {
    #[cfg(unix)]
    let result = unix_main::run();

    #[cfg(windows)]
    let result = win_main::run();

    match result {
        Ok(code) => ExitCode::from(code as u8),
        Err(e) => {
            eprintln!("aaa: {}", e);
            ExitCode::from(1)
        }
    }
}
