/// Permission prompt detectors.
///
/// Each detector is a pure function that inspects screen text and returns
/// `Some(PermissionRequest)` when a prompt is found. The proxy iterates
/// detectors in priority order and compares the result with the last
/// seen request to decide whether to fire an approval.

mod claude;
mod codex;
mod cursor;

pub use claude::Claude;
pub use codex::Codex;
pub use cursor::Cursor;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PermissionRequest {
    pub body: String,
}

pub trait Detector {
    fn detect(&self, contents: &str) -> Option<PermissionRequest>;
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum AgentKind {
    Claude,
    Codex,
    Cursor,
    Unknown,
}

impl AgentKind {
    /// Determine the agent kind from a command name (first element of the command).
    pub fn from_command(command: &str) -> AgentKind {
        let basename = command.rsplit('/').next().unwrap_or(command);
        let basename = basename.rsplit('\\').next().unwrap_or(basename);
        // Strip common Windows executable extensions (.exe, .cmd, .bat)
        let stem = basename
            .strip_suffix(".exe")
            .or_else(|| basename.strip_suffix(".cmd"))
            .or_else(|| basename.strip_suffix(".bat"))
            .unwrap_or(basename);
        match stem {
            "claude" => AgentKind::Claude,
            "codex" => AgentKind::Codex,
            "cursor" => AgentKind::Cursor,
            _ => AgentKind::Unknown,
        }
    }

    /// Try to identify the agent from screen contents.
    pub fn from_screen(contents: &str) -> Option<AgentKind> {
        if contents.contains("Claude Code") {
            Some(AgentKind::Claude)
        } else if contents.contains("codex") || contents.contains("Codex") {
            Some(AgentKind::Codex)
        } else if contents.contains("cursor") || contents.contains("Cursor") {
            Some(AgentKind::Cursor)
        } else {
            None
        }
    }

}

impl Detector for AgentKind {
    fn detect(&self, contents: &str) -> Option<PermissionRequest> {
        match self {
            AgentKind::Claude => Claude.detect(contents),
            AgentKind::Codex => Codex.detect(contents),
            AgentKind::Cursor => Cursor.detect(contents),
            AgentKind::Unknown => [&Claude as &dyn Detector, &Codex, &Cursor]
                .iter()
                .find_map(|d| d.detect(contents)),
        }
    }
}
