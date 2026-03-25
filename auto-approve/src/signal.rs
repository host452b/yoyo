use std::os::fd::OwnedFd;

use nix::unistd;

/// Sets up a SIGWINCH handler that writes to a pipe.
/// Returns the read end of the pipe for inclusion in poll().
pub fn setup_sigwinch_pipe() -> Result<OwnedFd, Box<dyn std::error::Error>> {
    let (read_fd, write_fd) = unistd::pipe()?;

    // Register the signal handler to write to the pipe
    // SAFETY: pipe write is async-signal-safe
    signal_hook::low_level::pipe::register(nix::libc::SIGWINCH, write_fd)?;

    Ok(read_fd)
}
