/// Virtual terminal screen buffer.
///
/// Thin wrapper around `vt100::Parser` that converts raw PTY bytes into
/// visible screen text for the detectors to consume.

use tracing::debug;

pub struct Screen {
    parser: vt100::Parser,
    cols: u16,
}

impl Screen {
    pub fn new(rows: u16, cols: u16) -> Self {
        Self {
            parser: vt100::Parser::new(rows, cols, 0),
            cols,
        }
    }

    /// Feed raw PTY bytes into the virtual terminal.
    pub fn feed(&mut self, data: &[u8]) {
        let limit = data.len().min(200);
        let hex: String = data[..limit]
            .iter()
            .map(|b| format!("{:02x}", b))
            .collect::<Vec<_>>()
            .join(" ");
        debug!(
            len = data.len(),
            "raw: {}{}", hex, if data.len() > 200 { " ..." } else { "" }
        );
        self.parser.process(data);
    }

    /// Return the current visible screen text.
    ///
    /// Uses per-row extraction instead of `Screen::contents()` to avoid
    /// ConPTY wrapping artifacts.  ConPTY clears rows by writing long
    /// streams of spaces (instead of erase sequences), which sets the
    /// vt100 `wrapped` flag on those rows.  `contents()` then joins
    /// wrapped rows into a single line, merging unrelated visual rows.
    /// `rows()` ignores the wrapped flag and returns each row separately.
    pub fn contents(&self) -> String {
        let screen = self.parser.screen();
        let mut out = String::new();
        for row in screen.rows(0, self.cols) {
            let trimmed = row.trim_end();
            if !out.is_empty() || !trimmed.is_empty() {
                if !out.is_empty() {
                    out.push('\n');
                }
                out.push_str(trimmed);
            }
        }
        out
    }

    pub fn resize(&mut self, rows: u16, cols: u16) {
        self.cols = cols;
        self.parser.screen_mut().set_size(rows, cols);
    }
}
