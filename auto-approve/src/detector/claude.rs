/// Claude Code permission prompt detector.
///
/// Matches the structured block delimited by a line of `─` characters (top)
/// and a line containing `Esc to cancel` (bottom).  When the prompt body is
/// too long for the terminal, "Esc to cancel" may be absent; in that case the
/// last numbered "No" option line is used as an inclusive bottom boundary.
/// Similarly, when the terminal is small the separator may scroll off screen;
/// in that case "Do you want to" is used as a fallback top boundary.

use super::{Detector, PermissionRequest};

pub struct Claude;

impl Detector for Claude {
    fn detect(&self, contents: &str) -> Option<PermissionRequest> {
        let lines: Vec<&str> = contents.lines().collect();

        // Find bottom boundary: prefer "Esc to cancel", fall back to last "N. No" line
        let (bottom_idx, bottom_inclusive) =
            if let Some(idx) = lines.iter().rposition(|line| line.contains("Esc to cancel")) {
                (idx, false) // exclusive — don't include the Esc line
            } else {
                // Find last line matching "N. No" pattern (e.g. "3. No" or "❯ 3. No")
                let idx = lines.iter().rposition(|line| {
                    let t = line.trim().replace('❯', "");
                    let t = t.trim();
                    t.len() >= 4
                        && t.as_bytes()[0].is_ascii_digit()
                        && t[1..].trim_start().starts_with(". No")
                })?;
                (idx, true) // inclusive — the No line is part of the body
            };

        // Find the nearest separator line above the bottom boundary.
        // Fallback: separator scrolled off screen — use "Do you want to"
        let (top_idx, top_inclusive) = if let Some(idx) = lines[..bottom_idx]
            .iter()
            .rposition(|line| {
                let trimmed = line.trim();
                !trimmed.is_empty() && trimmed.chars().all(|c| c == '─')
            })
        {
            (idx, false) // separator is not part of the body
        } else {
            let idx = lines[..bottom_idx]
                .iter()
                .rposition(|line| line.contains("Do you want to"))?;
            (idx, true) // the "Do you want to" line IS part of the body
        };

        let body_start = if top_inclusive { top_idx } else { top_idx + 1 };
        let end = if bottom_inclusive { bottom_idx + 1 } else { bottom_idx };
        let body: String = lines[body_start..end]
            .iter()
            .map(|line| line.trim().replace('❯', "").trim().to_string())
            .filter(|line| !line.is_empty())
            .collect::<Vec<_>>()
            .join("\n");

        // Require Yes/No options to be present — guards against incomplete
        // renders where the separator and footer are visible but options
        // haven't been drawn yet.
        if !body.contains("Yes") || !body.contains("No") {
            return None;
        }

        Some(PermissionRequest { body })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn prompt(action: &str, options: &[&str]) -> String {
        let mut s = String::new();
        s.push_str("─────────────────────────────────────────────\n");
        s.push_str(&format!(" {}\n\n", action));
        for (i, opt) in options.iter().enumerate() {
            s.push_str(&format!("   {}. {}\n", i + 1, opt));
        }
        s.push_str("\n Esc to cancel · Tab to amend\n");
        s
    }

    #[test]
    fn detects_prompt() {
        let d = Claude;
        let p = prompt("Read file", &["Yes", "No"]);
        assert!(d.detect(&p).is_some());
    }

    #[test]
    fn strips_whitespace_and_special_chars() {
        let d = Claude;
        let p = "─────────────\n  ❯ Read file  \n\n   1. Yes\n   2. No\n\n Esc to cancel\n";
        let req = d.detect(p).unwrap();
        assert_eq!(req.body, "Read file\n1. Yes\n2. No");
    }

    #[test]
    fn full_example() {
        let d = Claude;
        let p = concat!(
            "─────────────────────────────────────────────────────────────────────\n",
            " Read file\n",
            "\n",
            "  Read(/Users/aljin/.cargo/registry/src/vt100-0.16.2/src/screen.rs)\n",
            "\n",
            " Do you want to proceed?\n",
            "   1. Yes\n",
            "   2. Yes, allow reading from src/ during this session\n",
            "   3. No\n",
            "\n",
            " Esc to cancel · Tab to amend\n",
        );
        let req = d.detect(p).unwrap();
        assert!(req.body.contains("Read file"));
        assert!(req.body.contains("Do you want to proceed?"));
        assert!(req.body.contains("1. Yes"));
        assert!(req.body.contains("3. No"));
    }

    #[test]
    fn no_separator() {
        let d = Claude;
        assert!(d.detect(" Read file\n\n Esc to cancel\n").is_none());
    }

    #[test]
    fn no_esc_line_no_no_option() {
        let d = Claude;
        // No "Esc to cancel" and no "N. No" line → no detection
        assert!(d.detect("─────────────\n Read file\n 1. Yes\n").is_none());
    }

    #[test]
    fn fallback_no_esc_line_with_no_option() {
        let d = Claude;
        let p = concat!(
            "─────────────────────────────────────────────\n",
            " Read file\n",
            "\n",
            " 1. Yes\n",
            " 2. No\n",
        );
        let req = d.detect(p).unwrap();
        assert!(req.body.contains("Read file"));
        assert!(req.body.contains("1. Yes"));
        assert!(req.body.contains("2. No"));
    }

    #[test]
    fn fallback_long_body_no_esc() {
        let d = Claude;
        let p = concat!(
            "─────────────────────────────────────────────────────────────────────\n",
            " Read file\n",
            "\n",
            "  Read(/Users/aljin/.cargo/registry/src/vt100-0.16.2/src/screen.rs)\n",
            "\n",
            " Do you want to proceed?\n",
            "   1. Yes\n",
            "   2. Yes, allow reading from src/ during this session\n",
            "   3. No\n",
        );
        let req = d.detect(p).unwrap();
        assert!(req.body.contains("Read file"));
        assert!(req.body.contains("Do you want to proceed?"));
        assert!(req.body.contains("1. Yes"));
        assert!(req.body.contains("3. No"));
    }

    #[test]
    fn incomplete_render_no_options() {
        let d = Claude;
        // Separator and footer present but options not yet rendered
        let p = "─────────────\n Read file\n\n Esc to cancel\n";
        assert!(d.detect(p).is_none());
    }

    #[test]
    fn incomplete_render_partial_options() {
        let d = Claude;
        // Only Yes rendered, No not yet
        let p = "─────────────\n Read file\n 1. Yes\n Esc to cancel\n";
        assert!(d.detect(p).is_none());
    }

    #[test]
    fn picks_last_separator_when_multiple() {
        let d = Claude;
        let p = concat!(
            "─────────────\n",
            " old output with separator\n",
            "─────────────\n",
            " Write file\n",
            " 1. Yes\n",
            " 2. No\n",
            " Esc to cancel\n",
        );
        let req = d.detect(p).unwrap();
        assert!(req.body.contains("Write file"));
        assert!(!req.body.contains("old output"));
    }

    #[test]
    fn stable_across_redraws() {
        let d = Claude;
        let p = prompt("Read file", &["Yes", "No"]);
        let r1 = d.detect(&p);
        let r2 = d.detect(&p);
        assert_eq!(r1, r2);
    }

    #[test]
    fn different_prompts_differ() {
        let d = Claude;
        let p1 = prompt("Read file", &["Yes", "No"]);
        let p2 = prompt("Write file", &["Yes", "No"]);
        assert_ne!(d.detect(&p1), d.detect(&p2));
    }

    #[test]
    fn separator_scrolled_off() {
        let d = Claude;
        // Separator not visible — "Do you want to proceed?" is the fallback top boundary
        let p = concat!(
            "  Read(/some/very/long/path/to/file.rs)\n",
            "\n",
            " Do you want to proceed?\n",
            "   1. Yes\n",
            "   2. Yes, allow reading from src/ during this session\n",
            "   3. No\n",
            "\n",
            " Esc to cancel · Tab to amend\n",
        );
        let req = d.detect(p).unwrap();
        assert!(req.body.contains("Do you want to proceed?"));
        assert!(req.body.contains("1. Yes"));
        assert!(req.body.contains("3. No"));
    }

    #[test]
    fn separator_scrolled_off_no_esc() {
        let d = Claude;
        // Neither separator nor "Esc to cancel" — fallback top + fallback bottom
        let p = concat!(
            "  Read(/some/very/long/path/to/file.rs)\n",
            "\n",
            " Do you want to proceed?\n",
            "   1. Yes\n",
            "   2. No\n",
        );
        let req = d.detect(p).unwrap();
        assert!(req.body.contains("Do you want to proceed?"));
        assert!(req.body.contains("1. Yes"));
        assert!(req.body.contains("2. No"));
    }

    #[test]
    fn separator_scrolled_off_edit_variant() {
        let d = Claude;
        // "Do you want to edit" instead of "Do you want to proceed"
        let p = concat!(
            "  some long content\n",
            "\n",
            " Do you want to edit the file?\n",
            "   1. Yes\n",
            "   2. No\n",
            "\n",
            " Esc to cancel · Tab to amend\n",
        );
        let req = d.detect(p).unwrap();
        assert!(req.body.contains("Do you want to edit"));
        assert!(req.body.contains("1. Yes"));
        assert!(req.body.contains("2. No"));
    }

    #[test]
    fn separator_preferred_over_fallback() {
        let d = Claude;
        // When both separator and "Do you want to proceed?" are present,
        // separator should be used (includes more context in body)
        let p = concat!(
            "─────────────────────────────────────────────\n",
            " Read file\n",
            "\n",
            " Do you want to proceed?\n",
            "   1. Yes\n",
            "   2. No\n",
            "\n",
            " Esc to cancel\n",
        );
        let req = d.detect(p).unwrap();
        assert!(req.body.contains("Read file"));
        assert!(req.body.contains("Do you want to proceed?"));
    }
}
