use std::os::fd::{AsFd, BorrowedFd};

use nix::libc;
use nix::sys::termios::{self, Termios};

#[derive(Debug, Clone, Copy)]
pub struct Winsize {
    pub rows: u16,
    pub cols: u16,
}

pub fn get_termios(fd: BorrowedFd<'_>) -> nix::Result<Termios> {
    termios::tcgetattr(fd)
}

pub fn enable_raw_mode(fd: BorrowedFd<'_>) -> nix::Result<Termios> {
    let original = termios::tcgetattr(fd)?;
    let mut raw = original.clone();
    termios::cfmakeraw(&mut raw);
    termios::tcsetattr(fd, termios::SetArg::TCSAFLUSH, &raw)?;
    Ok(original)
}

pub fn restore_terminal(fd: BorrowedFd<'_>, termios: &Termios) {
    let _ = termios::tcsetattr(fd, termios::SetArg::TCSAFLUSH, termios);
}

pub fn get_winsize(fd: BorrowedFd<'_>) -> nix::Result<Winsize> {
    // SAFETY: ioctl TIOCGWINSZ reads into a winsize struct
    let mut ws: libc::winsize = unsafe { std::mem::zeroed() };
    let res = unsafe { libc::ioctl(fd.as_fd().as_raw_fd(), libc::TIOCGWINSZ, &mut ws) };
    if res == -1 {
        return Err(nix::errno::Errno::last());
    }
    Ok(Winsize {
        rows: ws.ws_row,
        cols: ws.ws_col,
    })
}

pub fn set_winsize(fd: BorrowedFd<'_>, size: &Winsize) -> nix::Result<()> {
    let ws = libc::winsize {
        ws_row: size.rows,
        ws_col: size.cols,
        ws_xpixel: 0,
        ws_ypixel: 0,
    };
    // SAFETY: ioctl TIOCSWINSZ writes from a winsize struct
    let res = unsafe { libc::ioctl(fd.as_fd().as_raw_fd(), libc::TIOCSWINSZ, &ws) };
    if res == -1 {
        return Err(nix::errno::Errno::last());
    }
    Ok(())
}

use std::os::fd::AsRawFd;
