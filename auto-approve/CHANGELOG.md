# Changelog

## 0.0.8

- Add `Ctrl+P` prefix key for runtime control: `p` toggles auto-approve, `0`–`9` sets delay
- Add bottom-right status bar overlay showing current auto-approve state and delay
- Remove `--no-auto-approve` CLI flag (replaced by runtime toggle)
- Extract `Agent`, `ProxyState`, and `StatusBar` into dedicated modules
- Add win32-input-mode key event parsing for `Ctrl+P` detection on Windows
- Remove typing suppression heuristic

## 0.0.7

- Fix Windows ConPTY screen parsing and simplify Claude detector
- Improve Windows detection robustness and fix re-approval timing bug

## 0.0.6

- Implement `Detector` trait directly on `AgentKind`, eliminating `Box<dyn Detector>` heap allocation

## 0.0.5

- Add fallback bottom boundary when "Esc to cancel" is missing from Claude prompts

## 0.0.4

- Handle Ctrl+Z (SIGTSTP) for proper suspend/resume
- Enable `--version` flag in CLI
- Simplify drain_to_stdout to use a local buffer

## 0.0.3

- Replace linear buffer with vt100 virtual terminal screen buffer
- Rewrite detectors with structured prompt parsing for Claude, Codex, and Cursor
- Add AgentKind enum for dedicated detector selection
- Extract Screen to dedicated file, split detector into trait + per-agent modules
- Fix duplicate detection and improve prompt robustness
- Fix yn prompt detection to match Cursor's (esc or n) decline marker
- Reduce match cooldown from 50ms to 20ms
- Update dependencies: vt100 0.15 to 0.16, windows-sys 0.59 to 0.61

## 0.0.2

- Add native Windows support via ConPTY
- Add Cursor Agent support with (y)/(n) prompt detection
- Rename binary `aa` to `aaa` and project to `auto-approve-agent`
- Add Windows install script, default delay to 3s
- Fix Windows exit hang and console corruption on Ctrl+C

## 0.0.1

- Initial release
- PTY proxy with auto-approve detection for Claude Code and Codex CLI
- Delayed auto-approval with user override (`--delay`)
- File logging via tracing
- CI pipeline and install script
