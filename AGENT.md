# AGENT.md

## Purpose

`ccsessions` is a Go terminal UI for browsing Claude Code session history for the current working directory. The app maps the current repo path to Claude's history under `~/.claude/projects`, parses session `.jsonl` files, and renders a searchable list plus a full transcript view.

## Primary entry points

- `cmd/ccsessions/main.go`: process entry point; creates the UI model and runs the Bubble Tea program.
- `internal/claude/sessions.go`: session discovery, JSONL parsing, normalization into `Session` and `Entry` values.
- `internal/ui/model.go`: TUI state, filtering, focus management, pane layout, and transcript rendering.

## How the app works

1. Resolve the current working directory into the matching Claude history folder under `~/.claude/projects`.
2. Load matching `.jsonl` files and parse them into normalized session/transcript data.
3. Classify raw records into display-oriented entry kinds such as prompts, assistant text, tool calls, tool results, progress, and metadata.
4. Build Bubble Tea view state for search, session list, and session detail panes.
5. Render the interface and handle keyboard navigation, filtering, and scrolling.

## Developer workflow

- Run locally: `go run ./cmd/ccsessions`
- Build: `make build`
- Test: `make test`
- Lint: `make lint`

The `Makefile` sets `GOCACHE` and `GOMODCACHE` into the workspace-local `.cache/` directory. Prefer the `Makefile` targets when validating changes.

## Repo-specific guidance

- Read `README.md` first for the user-facing behavior and control scheme.
- Use `docs/claude-session-log-taxonomy.md` when changing parsing or transcript presentation. The viewer intentionally preserves non-chat events from Claude logs.
- `projectHistoryDir` in `internal/claude/sessions.go` sanitizes paths by replacing `/` with `-`; if session discovery appears broken, verify the expected Claude history folder name first.
- The repo currently has no `*_test.go` files even though CI runs `go test ./...`. Be careful with parser and UI changes because automated coverage is minimal.
- Keep changes small and consistent with the existing structure: data ingestion in `internal/claude`, presentation/state in `internal/ui`, startup in `cmd/ccsessions`.

## Dependencies and UI stack

- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/bubbles`
- `github.com/charmbracelet/lipgloss`

## Useful files

- `README.md`
- `Makefile`
- `go.mod`
- `docs/claude-session-log-taxonomy.md`
- `.github/workflows/build.yml`
- `.github/workflows/lint.yml`
