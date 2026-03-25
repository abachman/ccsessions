# ccsessions

`ccsessions` (from **C**laude **C**ode **Sessions**) is a terminal UI for browsing Claude Code session history for the current working directory.

It maps the current directory to Claude's project history folder under `~/.claude/projects` by default, or a custom Claude directory passed with `--claude-dir`, loads each `.jsonl` session file, and shows:

- a searchable session list
- session metadata such as timestamps and branch
- the full session log for the selected session

## Run

```bash
go run ./cmd/ccsessions
```

To read sessions from a different Claude config directory:

```bash
go run ./cmd/ccsessions --claude-dir ~/.claude-personal
```

To show discovery diagnostics in the UI header:

```bash
go run ./cmd/ccsessions --claude-dir ~/.claude-personal --debug
```

## Build

Build the binary into `./bin/ccsessions`:

```bash
make build
```

Or build it directly with Go:

```bash
go build -o bin/ccsessions ./cmd/ccsessions
```

## Install

Install `ccsessions` into your Go binary directory:

```bash
go install ./cmd/ccsessions
```

After that, run it with:

```bash
ccsessions
```

Or:

```bash
ccsessions --claude-dir ~/.claude-personal
```

You can combine it with:

```bash
ccsessions --debug
```

## Controls

- Type to filter sessions
- `Tab` to cycle focus between search, session list, and session log
- `j` / `k` or arrow keys to move in the session list or scroll the session log when that pane is focused
- `q` to quit

## Notes

This repo depends on:

- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/bubbles`
- `github.com/charmbracelet/lipgloss`
