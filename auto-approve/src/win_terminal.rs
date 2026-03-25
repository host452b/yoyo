use std::io;
use std::sync::atomic::{AtomicU32, Ordering};

use windows_sys::Win32::System::Console::{
    GetConsoleMode, GetConsoleScreenBufferInfo, GetStdHandle, SetConsoleCP, SetConsoleCtrlHandler,
    SetConsoleMode, SetConsoleOutputCP, CONSOLE_SCREEN_BUFFER_INFO, ENABLE_ECHO_INPUT,
    ENABLE_LINE_INPUT, ENABLE_PROCESSED_INPUT, ENABLE_VIRTUAL_TERMINAL_INPUT,
    ENABLE_VIRTUAL_TERMINAL_PROCESSING, STD_INPUT_HANDLE, STD_OUTPUT_HANDLE,
};

/// Stores the original stdin console mode for restoration on Ctrl+C / close.
/// 0 means not yet saved.
static ORIGINAL_CONSOLE_MODE: AtomicU32 = AtomicU32::new(0);

unsafe extern "system" fn ctrl_handler(_ctrl_type: u32) -> i32 {
    let mode = ORIGINAL_CONSOLE_MODE.load(Ordering::Relaxed);
    if mode != 0 {
        let handle = GetStdHandle(STD_INPUT_HANDLE);
        SetConsoleMode(handle, mode);
    }
    // Return 0 (FALSE) so the default handler (process exit) still runs
    0
}

#[derive(Debug, Clone, Copy)]
pub struct Winsize {
    pub rows: u16,
    pub cols: u16,
}

pub fn get_console_mode() -> io::Result<u32> {
    unsafe {
        let handle = GetStdHandle(STD_INPUT_HANDLE);
        let mut mode: u32 = 0;
        if GetConsoleMode(handle, &mut mode) == 0 {
            return Err(io::Error::last_os_error());
        }
        Ok(mode)
    }
}

pub fn enable_raw_mode(original_mode: u32) -> io::Result<()> {
    // Save original mode and install ctrl handler for cleanup on Ctrl+C / close
    ORIGINAL_CONSOLE_MODE.store(original_mode, Ordering::Relaxed);
    unsafe {
        SetConsoleCtrlHandler(Some(ctrl_handler), 1); // 1 = TRUE (add handler)
    }

    unsafe {
        let stdin_handle = GetStdHandle(STD_INPUT_HANDLE);
        let mut mode: u32 = 0;
        if GetConsoleMode(stdin_handle, &mut mode) == 0 {
            return Err(io::Error::last_os_error());
        }
        // Disable line editing and echo, enable VT input
        mode &= !(ENABLE_LINE_INPUT | ENABLE_ECHO_INPUT | ENABLE_PROCESSED_INPUT);
        mode |= ENABLE_VIRTUAL_TERMINAL_INPUT;
        if SetConsoleMode(stdin_handle, mode) == 0 {
            return Err(io::Error::last_os_error());
        }

        // Enable VT processing on stdout for ANSI escape rendering
        let stdout_handle = GetStdHandle(STD_OUTPUT_HANDLE);
        let mut out_mode: u32 = 0;
        if GetConsoleMode(stdout_handle, &mut out_mode) == 0 {
            return Err(io::Error::last_os_error());
        }
        out_mode |= ENABLE_VIRTUAL_TERMINAL_PROCESSING;
        if SetConsoleMode(stdout_handle, out_mode) == 0 {
            return Err(io::Error::last_os_error());
        }

        Ok(())
    }
}

pub fn restore_console_mode(original: u32) -> io::Result<()> {
    unsafe {
        let handle = GetStdHandle(STD_INPUT_HANDLE);
        if SetConsoleMode(handle, original) == 0 {
            return Err(io::Error::last_os_error());
        }
        Ok(())
    }
}

/// Set console input and output code pages to UTF-8 (65001).
///
/// ConPTY inherits the parent console's code page.  If the code page is not
/// UTF-8, box-drawing characters like `─` (U+2500) will be encoded in the
/// OEM code page (e.g. 0xC4 in CP437) and the vt100 parser will mis-decode
/// them, breaking prompt detection.  Call this before spawning the ConPTY.
pub fn set_utf8_codepage() {
    unsafe {
        SetConsoleOutputCP(65001);
        SetConsoleCP(65001);
    }
}

pub fn get_winsize() -> io::Result<Winsize> {
    unsafe {
        let handle = GetStdHandle(STD_OUTPUT_HANDLE);
        let mut info: CONSOLE_SCREEN_BUFFER_INFO = std::mem::zeroed();
        if GetConsoleScreenBufferInfo(handle, &mut info) == 0 {
            return Err(io::Error::last_os_error());
        }
        let cols = (info.srWindow.Right - info.srWindow.Left + 1) as u16;
        let rows = (info.srWindow.Bottom - info.srWindow.Top + 1) as u16;
        Ok(Winsize { rows, cols })
    }
}
