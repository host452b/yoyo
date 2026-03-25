/// Cursor Agent permission prompt detector.
///
/// Matches approval prompts surrounded by a box drawn with `┌─┐`, `│`, `└─┘`.
/// The box must contain `(y)` and `n)` options. Extracts the text inside the
/// box, stripping box-drawing characters (`│`) and the selection arrow (`→`).

use super::{Detector, PermissionRequest};

pub struct Cursor;

impl Detector for Cursor {
    fn detect(&self, contents: &str) -> Option<PermissionRequest> {
        let lines: Vec<&str> = contents.lines().collect();

        // Find the last bottom border └──┘
        let bottom_idx = lines.iter().rposition(|line| {
            let trimmed = line.trim();
            trimmed.starts_with('└') && trimmed.ends_with('┘') && trimmed.contains('─')
        })?;

        // Find the nearest top border ┌──┐ above it
        let top_idx = lines[..bottom_idx].iter().rposition(|line| {
            let trimmed = line.trim();
            trimmed.starts_with('┌') && trimmed.ends_with('┐') && trimmed.contains('─')
        })?;

        // Extract lines inside the box, stripping │ and →
        let body: String = lines[top_idx + 1..bottom_idx]
            .iter()
            .map(|line| {
                line.trim()
                    .trim_start_matches('│')
                    .trim_end_matches('│')
                    .replace('→', "")
                    .trim()
                    .to_string()
            })
            .filter(|line| !line.is_empty())
            .collect::<Vec<_>>()
            .join("\n");

        // Require (y) and n) to be present
        if !body.contains("(y)") || !body.contains("n)") {
            return None;
        }

        Some(PermissionRequest { body })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn boxed(lines: &[&str]) -> String {
        let width = 60;
        let mut s = String::new();
        s.push('┌');
        s.extend(std::iter::repeat('─').take(width));
        s.push_str("┐\n");
        for line in lines {
            s.push_str(&format!("│ {:<w$}│\n", line, w = width - 1));
        }
        s.push('└');
        s.extend(std::iter::repeat('─').take(width));
        s.push_str("┘\n");
        s
    }

    #[test]
    fn command_prompt() {
        let d = Cursor;
        let p = boxed(&[
            "Run this command?",
            "Not in allowlist: cargo test",
            " → Run (once) (y)",
            "   Skip (esc or n)",
        ]);
        let req = d.detect(&p).unwrap();
        assert!(req.body.contains("Run this command?"));
        assert!(req.body.contains("Run (once) (y)"));
        assert!(req.body.contains("Skip (esc or n)"));
        assert!(!req.body.contains('│'));
        assert!(!req.body.contains('→'));
    }

    #[test]
    fn no_match_without_options() {
        let d = Cursor;
        let p = boxed(&["Some text without options"]);
        assert!(d.detect(&p).is_none());
    }

    #[test]
    fn no_match_without_box() {
        let d = Cursor;
        assert!(d.detect("Run (once) (y)\nSkip (esc or n)").is_none());
    }

    #[test]
    fn ignores_input_box() {
        let d = Cursor;
        // Input box has no (y)/(n) options
        let p = boxed(&["→ Plan, search, build anything"]);
        assert!(d.detect(&p).is_none());
    }

    #[test]
    fn picks_last_box() {
        let d = Cursor;
        let mut s = boxed(&["→ Plan, search, build anything"]);
        s.push_str(&boxed(&[
            "Run this command?",
            " → Run (once) (y)",
            "   Skip (esc or n)",
        ]));
        let req = d.detect(&s).unwrap();
        assert!(req.body.contains("Run this command?"));
        assert!(!req.body.contains("Plan, search"));
    }

    #[test]
    fn stable_body() {
        let d = Cursor;
        let p = boxed(&[
            "Run this command?",
            " → Run (once) (y)",
            "   Skip (esc or n)",
        ]);
        assert_eq!(d.detect(&p), d.detect(&p));
    }

    #[test]
    fn incomplete_render_no_bottom() {
        let d = Cursor;
        // Top border but no bottom border
        let p = "┌────────────────────┐\n│ Run (y) / Skip n) │\n";
        assert!(d.detect(p).is_none());
    }
}
