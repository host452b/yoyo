# aaa (auto-approve-agent)

PTY proxy that auto-approves permission prompts for Claude Code, Codex CLI, and Cursor Agent.

## Supported CLIs

- [Claude Code](https://docs.anthropic.com/en/docs/claude-code)
- [Codex CLI](https://github.com/openai/codex)
- [Cursor Agent](https://docs.cursor.com/agent)

## Installation

### Quick Install (Linux/macOS)

```bash
curl -fsSL https://gitlab-master.nvidia.com/aljin/auto-approve/-/raw/main/scripts/install.sh | bash
```

Set a specific version:

```bash
curl -fsSL https://gitlab-master.nvidia.com/aljin/auto-approve/-/raw/main/scripts/install.sh | AAA_VERSION=0.0.8 bash
```

### Quick Install (Windows)

```powershell
iwr -useb https://gitlab-master.nvidia.com/aljin/auto-approve/-/raw/main/scripts/install.ps1 | iex
```

### Build from Source

```bash
git clone https://gitlab-master.nvidia.com/aljin/auto-approve.git
cd auto-approve
cargo build --release
cp target/release/aaa ~/.local/bin/   # Linux/macOS
# Windows: copy target\release\aaa.exe %USERPROFILE%\.local\bin\
```

## Usage

```bash
aaa claude            # Launch Claude Code with auto-approval
aaa codex             # Launch Codex CLI with auto-approval
aaa cursor-agent      # Launch Cursor Agent with auto-approval
aaa claude --model sonnet   # Pass extra args to the CLI
```

On Windows (PowerShell):

```powershell
aaa claude
aaa --delay 5 claude
```

## CLI Flags

| Flag | Description |
|------|-------------|
| `--delay <seconds>` | Seconds to wait before auto-approving (default: 3) |
| `--log <file>` | Write debug log to a file |

## Examples

```bash
# Auto-approve with 5-second delay
aaa --delay 5 claude

# Debug logging
aaa --log debug.log claude

# Immediate approval (no delay)
aaa --delay 0 claude
```

## Keyboard Shortcuts

Press `Ctrl+P` followed by a command key:

| Key | Action |
|-----|--------|
| `p` | Toggle auto-approve on/off |
| `0`–`9` | Set approval delay to N seconds (`0` = immediate) |

## How It Works

`aaa` spawns the target CLI inside a PTY (pseudo-terminal) and sits between your terminal and the child process. It:

1. Forwards all input/output transparently between your terminal and the child process
2. Strips ANSI escape sequences from the output stream and scans for permission prompt patterns
3. When a prompt is detected, waits for the configured delay period (default: 3 seconds)
4. If you don't press a key during the delay, automatically sends Enter to approve
5. If you press any key during the delay, cancels the auto-approval so you can respond manually

## Supported Platforms

| Platform | Architecture | Binary | Notes |
|----------|-------------|--------|-------|
| Linux | x86_64 | `aaa-linux-x86_64` | Statically linked (musl) |
| Linux | aarch64 | `aaa-linux-aarch64` | Statically linked (musl) |
| macOS | x86_64 | `aaa-macos-x86_64` | Dynamically links libSystem.dylib |
| macOS | aarch64 (Apple Silicon) | `aaa-macos-aarch64` | Dynamically links libSystem.dylib |
| Windows | x86_64 | `aaa-windows-x86_64.exe` | Uses ConPTY |
