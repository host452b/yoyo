use std::os::fd::{AsRawFd, OwnedFd};
use std::os::unix::process::CommandExt;
use std::process::Command;

use nix::libc;
use nix::pty;
use nix::sys::termios::Termios;

use crate::terminal::Winsize;

pub struct PtyChild {
    pub master: OwnedFd,
    pub child: std::process::Child,
}

pub fn spawn(
    command: &[String],
    termios: &Termios,
    winsize: &Winsize,
) -> Result<PtyChild, Box<dyn std::error::Error>> {
    let ws = libc::winsize {
        ws_row: winsize.rows,
        ws_col: winsize.cols,
        ws_xpixel: 0,
        ws_ypixel: 0,
    };

    let pty_result = pty::openpty(Some(&ws), Some(termios))?;
    let master = pty_result.master;
    let slave = pty_result.slave;

    let slave_raw = slave.as_raw_fd();

    let mut cmd = Command::new(&command[0]);
    if command.len() > 1 {
        cmd.args(&command[1..]);
    }

    // Set up the child to use the slave PTY
    unsafe {
        let slave_fd = slave_raw;
        cmd.pre_exec(move || {
            // Create a new session
            if libc::setsid() == -1 {
                return Err(std::io::Error::last_os_error());
            }
            // Set controlling terminal
            if libc::ioctl(slave_fd, libc::TIOCSCTTY.into(), 0) == -1 {
                return Err(std::io::Error::last_os_error());
            }
            // Dup slave to stdin/stdout/stderr
            if libc::dup2(slave_fd, 0) == -1 {
                return Err(std::io::Error::last_os_error());
            }
            if libc::dup2(slave_fd, 1) == -1 {
                return Err(std::io::Error::last_os_error());
            }
            if libc::dup2(slave_fd, 2) == -1 {
                return Err(std::io::Error::last_os_error());
            }
            // Close the original slave fd if it's not 0, 1, or 2
            if slave_fd > 2 {
                libc::close(slave_fd);
            }
            Ok(())
        });
    }

    // We need the slave fd to stay alive until spawn, but we don't want it to be
    // inherited. The pre_exec closure above uses the raw fd number directly.
    let child = cmd.spawn()?;

    // Close slave in parent — child has its own copies via dup2
    drop(slave);

    Ok(PtyChild { master, child })
}
