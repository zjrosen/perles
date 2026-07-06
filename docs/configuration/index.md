# Configuration

Perles looks for configuration in these locations (in order of precedence):

1. `--config` flag
2. `.perles/config.yaml` (current directory)
3. `~/.config/perles/config.yaml`

---

## Configuration Options

### General

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `backend` | string | `"beads"` | Backend data source type (`beads`, `beads_rust`) |
| `beads_dir` | string | `""` | Path to beads database directory (default: current directory) |

### UI

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `ui.show_counts` | bool | `true` | Show issue counts in column headers |
| `ui.show_status_bar` | bool | `true` | Show status bar at bottom |
| `ui.vim_mode` | bool | `false` | Vim support for all textarea inputs |
| `ui.markdown_style` | string | `"dark"` | Markdown rendering style (`dark`, `light`) |
| `ui.keybindings.search` | string | `"ctrl+space"` | Keybinding to toggle search mode |
| `ui.keybindings.dashboard` | string | `"ctrl+o"` | Keybinding to toggle dashboard mode |

### Theme

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `theme.preset` | string | `""` | Theme preset name (see [Theming](theming.md)) |
| `theme.mode` | string | `""` | Force `light` or `dark` mode (empty = auto-detect) |
| `theme.colors.*` | hex | varies | Individual color token overrides |

### Orchestration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `orchestration.coordinator_client` | string | `"claude"` | AI client for coordinator: `claude`, `amp`, `codex`, `gemini`, `opencode`, `cursor` |
| `orchestration.worker_client` | string | `"claude"` | AI client for workers: `claude`, `amp`, `codex`, `gemini`, `opencode`, `cursor` |
| `orchestration.observer_client` | string | `"claude"` | AI client for the observer agent |
| `orchestration.observer_enabled` | bool | `false` | Enable the observer agent |
| `orchestration.api_port` | int | `0` | HTTP API server port (`0` = auto-assign) |
| `orchestration.community_workflows` | list | `[]` | Community workflow IDs to enable |
| `orchestration.session_storage.base_dir` | string | `"~/.perles/sessions"` | Root directory for session storage |
| `orchestration.session_storage.application_name` | string | auto | Override application name (default: derived from git remote) |
| `orchestration.templates.document_path` | string | `"docs/proposals"` | Base path for generated workflow documents |
| `orchestration.timeouts.worktree_creation` | duration | `"30s"` | Timeout for git worktree creation |

### AI Provider Settings

Configure model and environment for each AI provider.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `orchestration.claude.model` | string | `"claude-opus-4-8"` | Claude model to use |
| `orchestration.claude.env` | map | `{}` | Environment variables passed to Claude Code CLI |
| `orchestration.claude_worker.model` | string | inherits `claude.model` | Worker-specific Claude model override |
| `orchestration.claude_worker.env` | map | `{}` | Worker-specific environment variables |
| `orchestration.claude_observer.model` | string | inherits `claude.model` | Observer-specific Claude model override |
| `orchestration.claude_observer.env` | map | `{}` | Observer-specific environment variables |
| `orchestration.amp.model` | string | `"opus"` | Amp model (`opus`, `sonnet`) |
| `orchestration.amp.mode` | string | `"smart"` | Amp execution mode (`free`, `rush`, `smart`) |
| `orchestration.codex.model` | string | `"gpt-5.5"` | OpenAI Codex model |
| `orchestration.gemini.model` | string | `"gemini-3.1-pro-preview"` | Gemini model |
| `orchestration.opencode.model` | string | `"anthropic/claude-opus-4-8"` | OpenCode model |
| `orchestration.cursor.model` | string | `""` | Cursor model (empty = Cursor default) |

### Tracing

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `orchestration.tracing.enabled` | bool | `false` | Enable distributed tracing |
| `orchestration.tracing.exporter` | string | `"file"` | Trace exporter (`none`, `file`, `stdout`, `otlp`) |
| `orchestration.tracing.file_path` | string | `"~/.config/perles/traces/traces.jsonl"` | Output file for the `file` exporter |
| `orchestration.tracing.otlp_endpoint` | string | `"localhost:4317"` | Collector endpoint for the `otlp` exporter |
| `orchestration.tracing.sample_rate` | float | `1.0` | Sampling rate (`0.0` to `1.0`) |

### Feature Flags

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `flags.session-resume` | bool | `false` | Enable session resumption (`ctrl+r`) in orchestration mode |
| `flags.session-persistence` | bool | `false` | Persist workflow sessions to SQLite |

---

## Example Configuration

```yaml
# Backend data source
# backend: beads

# Path to beads database directory (default: current directory)
# beads_dir: /path/to/project

# UI settings
ui:
  show_counts: true
  show_status_bar: true
  vim_mode: false
  # keybindings:
  #   search: ctrl+space
  #   dashboard: ctrl+o

# Theme (use a preset or customize colors)
theme:
  # preset: catppuccin-mocha
  # mode: dark
  # colors:
  #   text.primary: "#FFFFFF"
  #   status.error: "#FF0000"

# Board views
views:
  - name: Default
    columns:
      - name: Blocked
        type: bql
        query: "status = open and blocked = true"
        color: "#FF8787"
      - name: Deferred
        type: bql
        query: "status = deferred or status = open and defer_until > now"
        color: "#808000"
      - name: Ready
        query: "status = open and ready = true"
        color: "#73F59F"
      - name: In Progress
        type: bql
        query: "status = in_progress"
        color: "#54A0FF"
      - name: Closed
        type: bql
        query: "status = closed"
        color: "#BBBBBB"

  - name: Bugs Only
    columns:
      - name: Open Bugs
        type: bql
        query: "type = bug and status = open"
        color: "#EF4444"
      - name: In Progress
        type: bql
        query: "type = bug and status = in_progress"
        color: "#F59E0B"
      - name: Fixed
        type: bql
        query: "type = bug and status = closed"
        color: "#10B981"

  - name: By Team
    columns:
      - name: Backend Team
        type: bql
        query: 'status = open and metadata.team = "backend"'
        color: "#73F59F"
      - name: Frontend Team
        type: bql
        query: 'status = open and metadata.team = "frontend"'
        color: "#54A0FF"
      - name: Unassigned
        type: bql
        query: 'status = open and metadata.team = nil'
        color: "#BBBBBB"

  - name: Work
    columns:
      - name: Current
        type: tree
        issue_id: bd-123
        tree_mode: child
        color: "#EF4444"

# AI Orchestration settings
orchestration:
  coordinator_client: claude
  worker_client: claude
  # observer_client: claude
  # observer_enabled: false
  # api_port: 0
  claude:
    model: claude-opus-4-8
    # env:
    #   CUSTOM_VAR: value
  # claude_worker:
  #   model: sonnet
  # amp:
  #   model: opus
  #   mode: smart
  # codex:
  #   model: gpt-5.5
  # gemini:
  #   model: gemini-3.1-pro-preview
  session_storage:
    # application_name: my-project
  templates:
    document_path: docs/proposals

# Feature flags
# flags:
#   session-resume: false
#   session-persistence: false
```

---

## User-Defined Actions

User actions allow custom keybindings that execute shell commands with issue context. Actions work in any mode where an issue is selected (kanban, search, search tree sub-mode).

!!! note
    Config changes require restarting perles to take effect.

### Configuration

```yaml
ui:
  actions:
    issue_action:
      open-claude:
        key: "1"
        command: "tmux split-window -h \"claude 'Work on {{.ID}}: {{.TitleText}}'\""
        description: "Open Claude"
```

### Template Variables

| Variable | Description | Escaped |
|----------|-------------|---------|
| `{{.ID}}` | Issue ID (e.g., "PROJ-123") | No |
| `{{.TitleText}}` | Issue title | Yes (shell-escaped) |

### Allowed Keys

User actions are restricted to numeric keys only: `0` through `9`. This prevents conflicts with built-in keybindings.

---

## Sound Configuration

Perles supports audio feedback for orchestration events. Sounds are enabled by default.

### Available Events

| Event | Description |
|-------|-------------|
| `review_verdict_approve` | Review approved in a cook workflow |
| `review_verdict_deny` | Review denied in a cook workflow |
| `user_notification` | User attention needed |
| `worker_out_of_context` | Worker ran out of context |
| `coordinator_out_of_context` | Coordinator ran out of context |
| `workflow_complete` | Workflow completed |

### Example

```yaml
sound:
  events:
    review_verdict_approve:
      enabled: true
      override_sounds:
        - "~/.perles/sounds/checkpoint.wav"
        - "~/.perles/sounds/proceed.wav"  # Multiple = random selection
    workflow_complete:
      enabled: true
      override_sounds:
        - "~/.perles/sounds/complete.wav"
```

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Whether to play sounds for this event |
| `override_sounds` | list | Custom sound file paths (WAV format). Multiple = random selection |
