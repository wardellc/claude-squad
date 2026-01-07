# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
go mod download              # Install dependencies
make build                   # Build binary to ./build/claude-squad
make install                 # Build and install to /opt/homebrew/bin/
make clean                   # Remove build directory
go test -v ./...             # Run all tests
gofmt -w .                   # Format code (required for CI)
./clean.sh                   # Cleanup: kills tmux server, removes ~/.claude-squad/
```

## Architecture

Claude Squad is a TUI application for managing multiple AI coding agents (Claude Code, Aider, Codex, Gemini) in isolated workspaces. It uses:
- **tmux** for isolated terminal sessions per agent
- **git worktrees** for isolated branches per session
- **Bubble Tea** (Charmbracelet) for the TUI framework

### Core Packages

| Package | Purpose |
|---------|---------|
| `app/` | Main TUI application. `home` struct implements `tea.Model` with states: Default, NewForm, Help, Confirm |
| `session/` | Instance lifecycle: `Instance` struct manages start/pause/resume, `Storage` handles persistence |
| `session/git/` | Git worktree creation, diff calculation, branch operations |
| `session/tmux/` | tmux session lifecycle, PTY handling, output capture |
| `ui/` | Components: `List` (instances), `TabbedWindow` (preview/diff), `Menu` (keybindings) |
| `ui/overlay/` | Modal dialogs: instance form, confirmation, help text |
| `config/` | App config and state persistence in `~/.claude-squad/` |
| `daemon/` | Background process for auto-accept mode (`--autoyes` flag) |

### Key Patterns

- **Instance States**: Running (tmux active, worktree exists) → Paused (worktree removed, branch preserved) → Resume (worktree recreated)
- **Multi-repo**: Instances grouped by repository, names can be reused across repos
- **Async Updates**: 500ms ticker for metadata (diff stats, status), 100ms for preview refresh
- **CLI**: Uses Cobra with subcommands: `reset`, `debug`, `version`

### Configuration

Config file: `~/.claude-squad/config.json`

| Field | Description |
|-------|-------------|
| `default_program` | Default program to run (e.g., "claude", "aider") |
| `repos_dir` | Default directory containing git repos (same as `--repos-dir` flag) |
| `auto_yes` | Auto-accept all prompts |
| `branch_prefix` | Prefix for git branches |
| `editor` | Command to open worktrees (e.g., "code", "cursor") |
