# Claude Code Session Log Taxonomy

This document defines a concrete taxonomy for Claude Code `.jsonl` session logs, based on the record shapes observed in local session history under `~/.claude/projects/**/*.jsonl`.

The goal is to support a semantically rich session viewer. The key design constraint is that raw `role` alone is not enough. For example, tool results are commonly stored in top-level `type:"user"` records even though they do not represent a human prompt.

## Model Layers

There are three distinct layers in the raw logs:

1. Top-level record type
2. Message content block type
3. Progress subtype

The viewer should preserve all three.

## Top-Level Record Types

### `user`

Represents either:

- an actual human prompt
- a tool result payload returned back into the conversation

Common structure:

```json
{
  "type": "user",
  "timestamp": "2026-03-19T16:38:42.855Z",
  "message": {
    "role": "user",
    "content": "init"
  }
}
```

Or:

```json
{
  "type": "user",
  "timestamp": "2026-03-19T16:38:47.781Z",
  "message": {
    "role": "user",
    "content": [
      {
        "type": "tool_result",
        "tool_use_id": "toolu_...",
        "content": "total 128\n...",
        "is_error": false
      }
    ]
  }
}
```

Viewer class:

- `human_prompt` when the content is plain text or `text` blocks authored by the user
- `tool_result` when the content contains `tool_result` blocks

### `assistant`

Represents Claude output. This may be:

- natural-language text
- hidden thinking markers
- tool invocations
- server-side tool invocations

Common structure:

```json
{
  "type": "assistant",
  "timestamp": "2026-03-19T16:38:47.734Z",
  "message": {
    "role": "assistant",
    "model": "claude-opus-4-6",
    "content": [
      {
        "type": "tool_use",
        "id": "toolu_...",
        "name": "Bash",
        "input": {
          "command": "ls -la /path",
          "description": "List files"
        }
      }
    ],
    "stop_reason": "tool_use"
  }
}
```

Viewer class:

- `assistant_text`
- `assistant_thinking`
- `tool_call`
- `server_tool_call`

depending on the nested block types.

### `progress`

Represents status and execution telemetry. This is not conversational content.

Common structure:

```json
{
  "type": "progress",
  "timestamp": "2026-03-19T16:38:49.065Z",
  "data": {
    "type": "hook_progress",
    "hookEvent": "PostToolUse",
    "hookName": "PostToolUse:Glob",
    "command": "callback"
  }
}
```

Viewer class:

- `progress_status`

with more specific presentation driven by `data.type`.

### `system`

Represents session metadata emitted during execution.

Observed example:

```json
{
  "type": "system",
  "subtype": "turn_duration",
  "durationMs": 40237,
  "timestamp": "2026-03-19T16:51:50.751Z"
}
```

Viewer class:

- `meta_system`

This is useful for diagnostics but should usually be visually subdued.

### `file-history-snapshot`

Represents file backup/snapshot bookkeeping, not conversation.

Observed example:

```json
{
  "type": "file-history-snapshot",
  "messageId": "84d25d64-...",
  "snapshot": {
    "messageId": "84d25d64-...",
    "trackedFileBackups": {},
    "timestamp": "2026-03-19T16:38:42.844Z"
  },
  "isSnapshotUpdate": false
}
```

Viewer class:

- `meta_filesystem`

These should likely be collapsed or hidden by default.

### `queue-operation`

Represents queued follow-up work outside the main conversational turn.

Observed example:

```json
{
  "type": "queue-operation",
  "operation": "enqueue",
  "timestamp": "2026-02-26T19:46:07.216Z",
  "content": "You can also read the logs from a recent run with `gh run view --job 65040129646`"
}
```

Viewer class:

- `meta_queue`

### `last-prompt`

Stores the last prompt string for the session.

Observed example:

```json
{
  "type": "last-prompt",
  "lastPrompt": "save this to a markdown notes file in this directory"
}
```

Viewer class:

- `meta_summary`

This is session metadata, not a transcript line.

### `summary`

Stores a generated or captured session summary.

Observed example:

```json
{
  "type": "summary",
  "summary": "API Error: 401 ... Please run /login",
  "leafUuid": "520afc0a-..."
}
```

Viewer class:

- `meta_summary`

### `pr-link`

Stores a PR reference associated with the session.

Observed example:

```json
{
  "type": "pr-link",
  "prNumber": 101,
  "prUrl": "https://github.com/augmented-org/prototype-platform/pull/101",
  "prRepository": "augmented-org/prototype-platform",
  "timestamp": "2026-02-26T20:28:31.333Z"
}
```

Viewer class:

- `meta_link`

### `custom-title`

Stores a user- or system-defined title for the session.

Viewer class:

- `meta_summary`

### `agent-name`

Stores the name of an agent or sub-agent session.

Viewer class:

- `meta_agent`

## Message Content Block Types

These occur inside `message.content` for `user` and `assistant` records.

### `text`

Common structure:

```json
{
  "type": "text",
  "text": "Let me check what's in this directory and any existing memory."
}
```

Represents:

- human prompt text
- assistant prose

Display:

- full prose block

### `thinking`

Common structure:

```json
{
  "type": "thinking",
  "thinking": "",
  "signature": "EtoCCkYI..."
}
```

Represents:

- hidden reasoning or reasoning marker metadata

Display:

- hidden by default, or a muted one-line placeholder such as `Thinking`

### `tool_use`

Common structure:

```json
{
  "type": "tool_use",
  "id": "toolu_...",
  "name": "Bash",
  "input": {
    "command": "ls -la /path",
    "description": "List files"
  },
  "caller": {
    "type": "direct"
  }
}
```

Represents:

- Claude deciding to invoke a tool

Display:

- compact labeled action block
- tool name should be prominent
- input summary should be visible

### `tool_result`

Common structure:

```json
{
  "type": "tool_result",
  "tool_use_id": "toolu_...",
  "content": "total 128\n...",
  "is_error": false
}
```

Represents:

- output or error returned from a tool invocation

Display:

- visually paired with the matching `tool_use`
- monospace is appropriate for command output
- error state should be visually distinct

### `server_tool_use`

Common structure:

```json
{
  "type": "server_tool_use",
  "id": "call_...",
  "name": "webReader",
  "input": "{\"url\":\"https://...\",\"return_format\":\"markdown\"}"
}
```

Represents:

- server-side tool execution initiated by Claude

Display:

- same semantic class as `tool_use`
- may warrant a distinct label such as `Server Tool`

## Progress Subtypes

These occur as `type:"progress"` with a nested `data.type`.

### `hook_progress`

Common structure:

```json
{
  "type": "hook_progress",
  "hookEvent": "SessionStart",
  "hookName": "SessionStart:startup",
  "command": "node \"$HOME/.claude/hooks/gsd-check-update.js\""
}
```

Represents:

- session lifecycle hooks
- post-tool hooks

Display:

- muted status line

### `bash_progress`

Common structure:

```json
{
  "type": "bash_progress",
  "output": "WARN using --force ...",
  "fullOutput": "WARN using --force ...",
  "elapsedTimeSeconds": 2,
  "totalLines": 3
}
```

Represents:

- incremental shell progress for long-running commands

Display:

- subdued streaming output block
- likely collapsible when verbose

### `agent_progress`

Common structure:

```json
{
  "type": "agent_progress",
  "agentId": "aefc4f54ffee782ff",
  "prompt": "I need to understand the full CI pipeline ...",
  "message": {
    "type": "user",
    "message": {
      "role": "user",
      "content": [
        {
          "type": "text",
          "text": "I need to understand the full CI pipeline ..."
        }
      ]
    }
  },
  "normalizedMessages": []
}
```

Represents:

- sub-agent start or sub-agent task state

Display:

- distinct delegated-work status block

### `query_update`

Common structure:

```json
{
  "type": "query_update",
  "query": "claude code programmatic invocation system prompt"
}
```

Represents:

- an evolving search query

Display:

- compact research-status line

### `search_results_received`

Common structure:

```json
{
  "type": "search_results_received",
  "query": "claude code --print flag CLI",
  "resultCount": 10
}
```

Represents:

- result arrival for a search step

Display:

- compact research-status line

### `waiting_for_task`

Common structure:

```json
{
  "type": "waiting_for_task",
  "taskDescription": "Re-check for failed GitHub Actions checks",
  "taskType": "local_bash"
}
```

Represents:

- the session entering a wait state for deferred work

Display:

- muted waiting-state line

### `mcp_progress`

Common structure:

```json
{
  "type": "mcp_progress",
  "status": "started",
  "serverName": "claude.ai Linear",
  "toolName": "get_issue"
}
```

Represents:

- MCP or remote tool progress

Display:

- compact remote-tool status line

## Recommended Normalized Viewer Taxonomy

The raw records should be normalized into semantic display items.

### `human_prompt`

Source:

- top-level `user` with plain string content
- top-level `user` with `text` content blocks not associated with tool results

Meaning:

- direct user intent or instruction

Display:

- indented block
- strong font weight
- higher contrast
- more vertical spacing than status rows

### `assistant_text`

Source:

- top-level `assistant` with `text` blocks

Meaning:

- Claude explanation, reasoning summary, or answer prose

Display:

- standard body block
- high readability

### `tool_call`

Source:

- `assistant` with `tool_use`
- `assistant` with `server_tool_use`

Meaning:

- agent action selection

Display:

- compact labeled card or row
- distinct accent color by tool family if useful

### `tool_result`

Source:

- `user` with `tool_result`

Meaning:

- output returned from a previously issued tool call

Display:

- nested under or near the matching `tool_call`
- preserve error state

### `thinking_marker`

Source:

- `assistant` with `thinking`

Meaning:

- reasoning metadata, not user-facing prose

Display:

- hidden by default or collapsed

### `progress_status`

Source:

- top-level `progress`

Meaning:

- streaming operational status

Display:

- one-line muted row
- smaller spacing than prose blocks

### `meta_event`

Source:

- `system`
- `file-history-snapshot`
- `queue-operation`
- `last-prompt`
- `summary`
- `pr-link`
- `custom-title`
- `agent-name`

Meaning:

- bookkeeping or session metadata

Display:

- collapsed by default, or rendered in a metadata section above the transcript

## Ordering And Pairing Rules

The viewer should preserve file order.

Additional pairing rules:

- pair `tool_result.tool_use_id` to the corresponding `tool_use.id`
- nest or visually attach `bash_progress` rows to the currently active tool call when possible
- render `agent_progress` as delegated-work rows, distinct from the main assistant prose

## Current Parser Gaps In This Repo

The current parser in `internal/claude/sessions.go` only preserves:

- `record.type`
- `message.role`
- flattened `text`

That means it currently loses:

- `tool_use`
- `tool_result`
- `thinking`
- `server_tool_use`
- all `progress.data` details
- most metadata-only records with no plain text payload

This is why the next implementation step should be a richer parser that preserves top-level record type, nested content blocks, and progress subtype separately.
