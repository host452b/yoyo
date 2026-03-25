/// Codex CLI permission prompt detector.
///
/// Matches prompts that start with "Would you like to" or "needs your approval"
/// and end with "Press enter to confirm or esc to cancel".

use super::{Detector, PermissionRequest};

const START_PATTERNS: &[&str] = &[
    "Would you like to",
    "needs your approval",
];

const FOOTER: &str = "Press enter to confirm or esc to cancel";

pub struct Codex;

impl Detector for Codex {
    fn detect(&self, contents: &str) -> Option<PermissionRequest> {
        let lines: Vec<&str> = contents.lines().collect();

        // Find the last footer line
        let footer_idx = lines.iter().rposition(|line| line.contains(FOOTER))?;

        // Find the nearest start pattern above the footer
        let start_idx = lines[..footer_idx].iter().rposition(|line| {
            START_PATTERNS.iter().any(|pat| line.contains(pat))
        })?;

        let body: String = lines[start_idx..footer_idx]
            .iter()
            .map(|line| line.trim().replace('›', "").trim().to_string())
            .filter(|line| !line.is_empty())
            .collect::<Vec<_>>()
            .join("\n");

        if body.is_empty() {
            return None;
        }

        Some(PermissionRequest { body })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn prompt(action: &str, command: &str, options: &[&str]) -> String {
        let mut s = String::new();
        s.push_str(&format!("  {}\n\n", action));
        s.push_str(&format!("  $ {}\n\n", command));
        for (i, opt) in options.iter().enumerate() {
            if i == 0 {
                s.push_str(&format!("› {}. {}\n", i + 1, opt));
            } else {
                s.push_str(&format!("  {}. {}\n", i + 1, opt));
            }
        }
        s.push_str(&format!("\n  {}\n", FOOTER));
        s
    }

    #[test]
    fn command_prompt() {
        let d = Codex;
        let p = prompt(
            "Would you like to run the following command?",
            "cargo test",
            &["Yes, proceed (y)", "Yes, and don't ask again (p)", "No (esc)"],
        );
        let req = d.detect(&p).unwrap();
        assert!(req.body.contains("Would you like to run"));
        assert!(req.body.contains("cargo test"));
    }

    #[test]
    fn edit_prompt() {
        let d = Codex;
        let p = "  Would you like to make the following edits?\n\n  file.rs\n\n  Press enter to confirm or esc to cancel\n";
        assert!(d.detect(p).is_some());
    }

    #[test]
    fn mcp_prompt() {
        let d = Codex;
        let p = "  MyServer needs your approval.\n\n  Press enter to confirm or esc to cancel\n";
        assert!(d.detect(p).is_some());
    }

    #[test]
    fn no_match() {
        let d = Codex;
        assert!(d.detect("Hello world").is_none());
    }

    #[test]
    fn no_footer() {
        let d = Codex;
        assert!(d.detect("Would you like to run the following command?").is_none());
    }

    #[test]
    fn no_start_pattern() {
        let d = Codex;
        assert!(d.detect("some text\nPress enter to confirm or esc to cancel\n").is_none());
    }

    #[test]
    fn stable_body() {
        let d = Codex;
        let p = prompt(
            "Would you like to run the following command?",
            "npm install",
            &["Yes, proceed (y)", "No (esc)"],
        );
        assert_eq!(d.detect(&p), d.detect(&p));
    }

    #[test]
    fn strips_selection_marker() {
        let d = Codex;
        let p = "  Would you like to run this?\n› 1. Yes\n  2. No\n  Press enter to confirm or esc to cancel\n";
        let req = d.detect(p).unwrap();
        assert!(!req.body.contains('›'));
        assert!(req.body.contains("1. Yes"));
    }
}
