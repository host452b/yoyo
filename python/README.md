# yoyo

Python distribution of [yoyo](https://github.com/host452b/yoyo) — a PTY proxy
that auto-approves AI agent permission prompts (Claude Code, Codex CLI, Cursor
Agent, …).

## Install

```bash
pip install yoyo
```

pip will pick the matching pre-built binary for your OS / architecture
(linux-amd64, linux-arm64, macos-amd64, macos-arm64). No Go toolchain needed.

After install:

```bash
yoyo -h       # usage
yoyo claude   # wrap Claude Code with 3s auto-approve delay
```

See the [main project README](https://github.com/host452b/yoyo#readme) for
flags, runtime controls, config file format, safety guard, AFK / fuzzy
fallback, and everything else.

## Why a binary wheel?

yoyo is written in Go. Packaging it as a Python wheel means you get the same
one-line install for Python-centric workflows (`pip install yoyo`) as you do
with `curl | sh` or `go install`. The wheel bundles only the Go binary — no
Python code, no runtime Python dependency.

## License

MIT
