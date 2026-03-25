use std::time::Duration;

use tracing::info;

use crate::detector::{AgentKind, Detector, PermissionRequest};
use crate::status_bar::StatusBar;

/// Wraps an `AgentKind` with a toggle, approval delay, and status bar.
pub struct Agent {
    pub kind: AgentKind,
    pub enabled: bool,
    pub status_bar: StatusBar,
    pub delay: Duration,
}

impl Agent {
    pub fn new(kind: AgentKind, rows: u16, cols: u16, delay: Duration) -> Self {
        Agent {
            kind,
            enabled: true,
            status_bar: StatusBar::new(rows, cols, true, delay.as_secs() as u32),
            delay,
        }
    }

    pub fn immediate(&self) -> bool {
        self.delay.is_zero()
    }

    pub fn set_delay(&mut self, secs: u32) {
        self.delay = Duration::from_secs(secs as u64);
        self.status_bar.set_delay(secs);
        info!("approval delay set to {}s", secs);
    }

    pub fn toggle(&mut self) {
        self.enabled = !self.enabled;
        self.status_bar.toggle();
        info!("auto-approve toggled: {}", self.enabled);
    }

    pub fn detect(&self, contents: &str) -> Option<PermissionRequest> {
        if !self.enabled {
            return None;
        }
        self.kind.detect(contents)
    }
}
