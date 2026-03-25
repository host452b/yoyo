use std::time::Instant;

use tracing::{debug, info};

use crate::agent::Agent;
use crate::detector::{AgentKind, PermissionRequest};
use crate::screen::Screen;

/// A pending auto-approval waiting for its delay to expire.
pub struct PendingApproval {
    pub deadline: Instant,
}

/// Shared mutable state for the proxy loop.
pub struct ProxyState<'a> {
    pub pending: Option<PendingApproval>,
    pub last_request: Option<PermissionRequest>,
    pub prefix_active: Option<Instant>,
    pub frames: u32,
    pub screen: &'a mut Screen,
    pub agent: &'a mut Agent,
}

/// What the caller should do after processing a frame.
pub enum ApprovalAction {
    /// No approval needed.
    None,
    /// Send `\r` immediately.
    SendCR,
    /// Start a pending approval timer.
    StartTimer(PendingApproval),
}

impl<'a> ProxyState<'a> {
    pub fn new(screen: &'a mut Screen, agent: &'a mut Agent) -> Self {
        Self {
            pending: None,
            last_request: None,
            prefix_active: None,
            frames: 0,
            screen,
            agent,
        }
    }

    /// Feed a frame into the screen, run detection, and return what the caller
    /// should do for auto-approval. The caller is responsible for writing
    /// the wrapped frame to stdout.
    pub fn process_frame(&mut self, frame: &[u8]) -> ApprovalAction {
        self.screen.feed(frame);
        let contents = self.screen.contents();
        debug!("screen:\n{}", contents);

        if matches!(self.agent.kind, AgentKind::Unknown) && self.frames < 10 {
            if let Some(k) = AgentKind::from_screen(&contents) {
                info!("detected agent from screen: {:?}", k);
                self.agent.kind = k;
            }
        }
        self.frames = self.frames.saturating_add(1);

        let result = self.agent.detect(&contents);
        let changed = match &result {
            Some(req) if self.last_request.as_ref() != Some(req) => {
                info!("detected approval request: {}", req.body);
                true
            }
            _ => false,
        };
        if result.is_none() && self.last_request.is_some() && self.pending.is_none() {
            debug!("prompt disappeared, clearing state");
            self.last_request = None;
        } else if result.is_some() {
            self.last_request = result;
        }

        if changed {
            if self.agent.immediate() {
                ApprovalAction::SendCR
            } else if self.pending.is_none() {
                info!(
                    "prompt detected, waiting {:.1}s before auto-approval",
                    self.agent.delay.as_secs_f32()
                );
                ApprovalAction::StartTimer(PendingApproval {
                    deadline: Instant::now() + self.agent.delay,
                })
            } else {
                ApprovalAction::None
            }
        } else {
            ApprovalAction::None
        }
    }
}

/// Returns true if `data` is a terminal-generated escape sequence (focus events,
/// cursor-position reports) rather than a user keypress.
pub fn is_escape_sequence(data: &[u8]) -> bool {
    if data.len() < 3 || data[0] != 0x1b || data[1] != b'[' {
        return false;
    }
    if data.len() == 3 && (data[2] == b'I' || data[2] == b'O') {
        return true;
    }
    if data.last() == Some(&b'R') {
        let inner = &data[2..data.len() - 1];
        return !inner.is_empty() && inner.iter().all(|&b| b.is_ascii_digit() || b == b';');
    }
    false
}
