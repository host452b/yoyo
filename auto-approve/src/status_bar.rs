use std::fmt::Write;

const LABEL_OFF: &str = " [auto-approve: off] ";

const CURSOR_SAVE: &str = "\x1b[s";
const CURSOR_RESTORE: &str = "\x1b[u";
const SGR_RESET: &str = "\x1b[0m";
const FG_RED: &str = "\x1b[31m";
const FG_GREEN: &str = "\x1b[32m";

pub struct StatusBar {
    rows: u16,
    cols: u16,
    enabled: bool,
    delay_secs: u32,
    label_width: u16,
    /// Whether the label is currently drawn on screen.
    painted: bool,
    /// Whether the previous frame left the terminal mid-escape or mid-UTF-8.
    mid_sequence: bool,
}

impl StatusBar {
    pub fn new(rows: u16, cols: u16, enabled: bool, delay_secs: u32) -> Self {
        let mut bar = StatusBar {
            rows,
            cols,
            enabled,
            delay_secs,
            label_width: 0,
            painted: false,
            mid_sequence: false,
        };
        bar.recompute_label_width();
        bar
    }

    pub fn toggle(&mut self) {
        self.enabled = !self.enabled;
    }

    pub fn set_delay(&mut self, secs: u32) {
        self.delay_secs = secs;
    }

    pub fn resize(&mut self, rows: u16, cols: u16) {
        self.rows = rows;
        if self.cols != cols {
            self.cols = cols;
            self.recompute_label_width();
        }
    }

    /// Wrap a PTY frame with clear (prefix) and paint (suffix) sequences,
    /// returned as a single buffer for one atomic `write_all` call.
    ///
    /// Skips injection when the output stream is mid-escape or mid-UTF-8 to
    /// avoid corrupting multi-byte characters or escape sequences that span
    /// frame boundaries.
    pub fn wrap_frame(&mut self, frame: &[u8]) -> Vec<u8> {
        let prev_mid = self.mid_sequence;
        self.mid_sequence = ends_mid_escape(frame) || ends_mid_utf8(frame);

        if self.label_width == 0 || self.rows == 0 {
            return frame.to_vec();
        }

        let label_col = self.cols - self.label_width + 1;

        // Phase 1: clear previous label.
        // Skip if previous frame left us mid-sequence (current frame starts
        // with continuation bytes — injecting would corrupt them).
        let clear = if self.painted && !prev_mid {
            self.painted = false;
            let blank: String = std::iter::repeat(' ').take(self.label_width as usize).collect();
            overlay_at(self.rows, label_col, "", &blank)
        } else {
            if prev_mid {
                self.painted = false;
            }
            Vec::new()
        };

        // Phase 2: paint new label after the frame.
        // Skip if this frame ends mid-sequence.
        let paint = if self.mid_sequence {
            Vec::new()
        } else {
            self.painted = true;
            let fg_color = if self.enabled { FG_RED } else { FG_GREEN };
            overlay_at(self.rows, label_col, fg_color, &self.label_text())
        };

        let mut buf = Vec::with_capacity(clear.len() + frame.len() + paint.len());
        buf.extend_from_slice(&clear);
        buf.extend_from_slice(frame);
        buf.extend_from_slice(&paint);
        buf
    }

    fn label_text(&self) -> String {
        let label = if self.enabled {
            format!(" [auto-approve: on {}s] ", self.delay_secs)
        } else {
            LABEL_OFF.to_string()
        };
        let on_width = " [auto-approve: on 0s] ".len();
        format!("{:>width$}", label, width = on_width)
    }

    fn recompute_label_width(&mut self) {
        let on_width = " [auto-approve: on 0s] ".len() as u16;
        if self.cols < on_width {
            self.label_width = 0;
        } else {
            self.label_width = on_width;
        }
    }
}

/// Write `text` at the given `row` and `col` with optional SGR color, wrapped
/// in cursor save/restore so the cursor position is preserved.
fn overlay_at(row: u16, col: u16, color: &str, text: &str) -> Vec<u8> {
    let mut s = String::with_capacity(48 + text.len());
    s.push_str(CURSOR_SAVE);
    let _ = write!(s, "\x1b[{};{}H", row, col);
    s.push_str(SGR_RESET);
    s.push_str(color);
    s.push_str(text);
    s.push_str(SGR_RESET);
    s.push_str(CURSOR_RESTORE);
    s.into_bytes()
}

/// Check if `data` ends with an incomplete ANSI escape sequence.
fn ends_mid_escape(data: &[u8]) -> bool {
    let Some(esc_pos) = data.iter().rposition(|&b| b == 0x1b) else {
        return false;
    };
    let tail = &data[esc_pos..];

    if tail.len() == 1 {
        return true;
    }

    match tail[1] {
        // CSI: ESC [ <params> <terminator 0x40-0x7E>
        b'[' => !tail[2..].iter().any(|&b| (0x40..=0x7E).contains(&b)),
        // OSC: ESC ] ... terminated by BEL (0x07) or ST (ESC \)
        b']' => !tail[2..].iter().any(|&b| b == 0x07)
            && !data[esc_pos + 1..].windows(2).any(|w| w == b"\x1b\\"),
        _ => false,
    }
}

/// Check if `data` ends with an incomplete multi-byte UTF-8 character.
fn ends_mid_utf8(data: &[u8]) -> bool {
    if data.is_empty() {
        return false;
    }
    // Walk backwards from the end to find the start byte of the last character.
    let mut i = data.len() - 1;
    // Skip continuation bytes (10xxxxxx).
    while i > 0 && (data[i] & 0xC0) == 0x80 {
        i -= 1;
    }
    let start = data[i];
    let expected_len = if start < 0x80 {
        1
    } else if start < 0xE0 {
        2
    } else if start < 0xF0 {
        3
    } else {
        4
    };
    (data.len() - i) < expected_len
}
