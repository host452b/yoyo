use clap::Parser;

#[derive(Parser, Debug)]
#[command(name = "aaa", version, about = "PTY proxy that auto-approves Claude Code, Codex CLI, and Cursor Agent permission prompts")]
#[command(trailing_var_arg = true)]
pub struct Args {
    /// Write debug log to this file (raw bytes, stripped text, detection events)
    #[arg(long)]
    pub log: Option<String>,

    /// Seconds to wait before auto-approving (0 = immediate)
    #[arg(long, default_value = "3")]
    pub delay: f32,

    /// Command and arguments to execute
    #[arg(required = true, allow_hyphen_values = true)]
    pub command: Vec<String>,
}
