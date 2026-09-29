# Configuration

Perles looks for configuration in these locations (in order of precedence):

1. `--config` / `-c` flag
2. `.perles/config.yaml` (current directory)
3. `~/.config/perles/config.yaml`

Only the first file found is loaded; files are not merged. Options that file leaves out use the defaults listed below.

`perles init` writes a default `.perles/config.yaml` in the current directory. It fails with `config file already exists: .perles/config.yaml` if that file is already there. Because `.perles/config.yaml` is checked before `~/.config/perles/config.yaml`, a project file created this way takes precedence over your user config in that directory.

Running `perles` with no config file in any of these locations writes the same default `.perles/config.yaml`, but only once the board can start (the beads database opened and passed the version check). Apart from `perles init`, subcommands never create a config file. If `--config` points at a file that doesn't exist, perles starts on the built-in defaults without creating it (saving a view or column change later writes that file).

View and column changes made in the TUI are saved back to the loaded config file.

Some flags override config options: `-b/--beads-dir` overrides `beads_dir`, `-p/--port` (on `perles` and `perles daemon`) overrides `orchestration.api_port`, and `--markdown-style` overrides `ui.markdown_style`.

---

## Configuration Options

### General

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `backend` | string | `"beads"` | Backend data source type (`beads`, `beads_rust`) |
| `beads_dir` | string | `""` | Path to beads database directory. A leading `~` is expanded. Precedence: `-b/--beads-dir` > `BEADS_DIR` env var > `beads_dir` > current directory (`perles daemon` has no `-b` flag) |

### UI

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `ui.show_counts` | bool | `true` | Show issue counts in column headers |
| `ui.show_status_bar` | bool | `true` | Show status bar at bottom |
| `ui.vim_mode` | bool | `false` | Vim support for all textarea inputs |
| `ui.markdown_style` | string | `"dark"` | Markdown rendering style (`dark`, `light`) |
| `ui.keybindings.search` | string | `"ctrl+space"` | Keybinding to switch between kanban and search mode |
| `ui.keybindings.dashboard` | string | `"ctrl+o"` | Keybinding to open the dashboard from kanban mode (leave it with `q` or `esc`) |

The keybinding options can't use the reserved keys `q`, `ctrl+c`, `esc`, `?`, or `enter`, and the two can't share a key.

### Theme

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `theme.preset` | string | `""` | Theme preset name (see [Theming](theming.md)) |
| `theme.mode` | string | `""` | Currently ignored (kept for backward compatibility). Use a light preset such as `catppuccin-latte` for light terminals |
| `theme.colors.*` | hex | varies | Individual color token overrides |

An unknown preset, unknown color token, or invalid hex value makes perles ignore the whole `theme` section and use the default theme. Startup continues and a `Theme config ignored: ...` warning toast is shown.

### Orchestration

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `orchestration.coordinator_client` | string | `client`, else `"claude"` | AI client for coordinator: `claude`, `amp`, `codex`, `gemini`, `opencode`, `cursor` |
| `orchestration.worker_client` | string | `client`, else `"claude"` | AI client for workers: `claude`, `amp`, `codex`, `gemini`, `opencode`, `cursor` |
| `orchestration.client` | string | `""` | Legacy fallback client for both roles, used only when `coordinator_client` / `worker_client` are unset. The generated config sets both explicitly, so remove those lines for `client` to take effect |
| `orchestration.observer_client` | string | `"claude"` | AI client for the observer agent |
| `orchestration.observer_enabled` | bool | `false` | Enable the observer agent |
| `orchestration.api_port` | int | `0` | HTTP API server port (`0` = auto-assign) |
| `orchestration.community_workflows` | list | `[]` | Community workflow IDs to enable |
| `orchestration.workflows` | list | `[]` | Chat panel workflow overrides, each matched by `name` (case-insensitive): `enabled: false` hides the workflow, `description` replaces its description. Applies to the chat panel Workflows tab and the chat section of `perles workflows`, not to dashboard templates |
| `orchestration.session_storage.base_dir` | string | `$HOME/.perles/sessions` | Root directory for session storage. A leading `~` is expanded; the result must be an absolute path, or perles refuses to start. The browser session viewer (`o` in the dashboard) reads sessions from this directory too |
| `orchestration.session_storage.application_name` | string | derived | Application directory name for sessions: `{base_dir}/{application_name}/{YYYY-MM-DD}/{session-id}/`. Default: the git `origin` repo name, else the workflow's working directory name (`perles daemon` always uses the directory name). With `flags.session-persistence`, it is also the project key for persisted dashboard workflows, so changing it hides workflows saved under the old name |
| `orchestration.templates.document_path` | string | `"docs/proposals"` | Base path for generated workflow documents |
| `orchestration.timeouts.worktree_creation` | duration | `"30s"` | Timeout for git worktree creation |

### AI Provider Settings

Configure model and environment for each AI provider.

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `orchestration.claude.model` | string | `"claude-opus-5-5"` | Claude model to use |
| `orchestration.claude.env` | map | `{}` | Environment variables passed to Claude Code CLI |
| `orchestration.claude_worker.model` | string | inherits `claude.model` | Worker-specific Claude model. Only applied when `claude_worker.env` has at least one entry; otherwise the whole `claude_worker` block is ignored |
| `orchestration.claude_worker.env` | map | `{}` | Worker environment variables. When non-empty, workers use the `claude_worker` block instead of `claude`, so this replaces `claude.env` for workers (the maps are not merged) |
| `orchestration.claude_observer.model` | string | inherits `claude.model` | Observer-specific Claude model override |
| `orchestration.claude_observer.env` | map | `{}` | Currently ignored: the observer receives no custom environment variables (neither this nor `claude.env`) |
| `orchestration.amp.model` | string | `"opus"` | Amp model (`opus`, `sonnet`) |
| `orchestration.amp.mode` | string | `"smart"` | Amp execution mode (`free`, `rush`, `smart`) |
| `orchestration.codex.model` | string | `"gpt-6-sol"` | OpenAI Codex model |
| `orchestration.gemini.model` | string | `"gemini-3.8-flash"` | Gemini model |
| `orchestration.opencode.model` | string | `"anthropic/claude-opus-5-5"` | OpenCode model |
| `orchestration.cursor.model` | string | `""` | Cursor model (empty = Cursor default) |

### Tracing

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `orchestration.tracing.enabled` | bool | `false` | Enable OpenTelemetry tracing |
| `orchestration.tracing.exporter` | string | `"file"` | Trace exporter (`none`, `file`, `stdout`, `otlp`) |
| `orchestration.tracing.file_path` | string | `$HOME/.config/perles/traces/traces.jsonl` | Output file for the `file` exporter. A leading `~` is expanded |
| `orchestration.tracing.otlp_endpoint` | string | `"localhost:4317"` | Collector endpoint for the `otlp` exporter |
| `orchestration.tracing.sample_rate` | float | `1.0` | Parent-based sampling ratio (`0.0` to `1.0`; `0` is treated as `1.0`) |

Setting only `enabled: true` is enough: spans go to the default `traces.jsonl` file. When enabled, `perles` and `perles daemon` each create one tracer (service name `perles-orchestrator`) at startup and use it for every dashboard or daemon workflow. That covers a span per orchestration command (`command.process.<command_type>`), handler spans (`handler.*`), and coordinator MCP tool calls (`mcp.tool.<tool_name>`). Worker MCP tool calls and the kanban/search chat panel are not traced. Batched spans are flushed on exit.

Exporters:

- `file` appends JSONL spans to `file_path`, creating parent directories as needed.
- `stdout` pretty-prints spans to standard output. In the TUI this garbles the display, so use it only with `perles daemon`.
- `otlp` exports over insecure gRPC to `otlp_endpoint`.
- `none` creates spans but exports nothing.

If tracing is enabled but the tracer can't be created (for example, the trace file can't be opened), startup fails with `creating tracing provider: ...`. When tracing is disabled, nothing is created or exported.

### Feature Flags

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `flags.session-persistence` | bool | `false` | Persist dashboard workflows to SQLite (`~/.perles/perles.db`) across restarts; also enables archiving workflows with `a` in the dashboard |

### Environment Variables

| Variable | Description |
|----------|-------------|
| `BEADS_DIR` | Beads database directory. Overrides `beads_dir`; `-b/--beads-dir` still wins |
| `BEADS_DOLT_SERVER_MODE` | `1` forces Dolt server mode, whatever `dolt_mode` says in `.beads/metadata.json` |
| `BEADS_DOLT_SHARED_SERVER` | `1` or `true` also forces Dolt server mode |
| `BEADS_DOLT_SERVER_HOST` | Dolt server host (overrides `.beads/metadata.json`; default `127.0.0.1`) |
| `BEADS_DOLT_SERVER_PORT` | Dolt server port (overrides `.beads/metadata.json` and `.beads/dolt-server.port`; default `3307`) |
| `BEADS_DOLT_SERVER_USER` | Dolt server user (overrides `.beads/metadata.json`; default `root`) |
| `BEADS_DOLT_PASSWORD` | Dolt server password |

The `BEADS_DOLT_*` variables only matter when `.beads/metadata.json` selects the Dolt backend. See [Debug Mode](../getting-started.md#debug-mode) for `PERLES_DEBUG` and `PERLES_LOG`.

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
    model: claude-opus-5-5
    # env:
    #   CUSTOM_VAR: value
  # claude_worker:          # only used when env has at least one entry
  #   model: sonnet
  #   env:
  #     CUSTOM_VAR: value     # replaces claude.env for workers
  # amp:
  #   model: opus
  #   mode: smart
  # codex:
  #   model: gpt-6-sol
  # gemini:
  #   model: gemini-3.8-flash
  session_storage:
    # base_dir: ~/.perles/sessions
    # application_name: my-project
  templates:
    document_path: docs/proposals

# Feature flags
# flags:
#   session-persistence: false
```

---

## User-Defined Actions

User actions allow custom keybindings that execute shell commands with issue context. Actions work in any mode where an issue is selected (kanban, search, search tree sub-mode). They are not available in the dashboard.

Commands are rendered as Go templates and run with `sh -c` in the perles working directory. They are fire-and-forget: perles doesn't wait for them, and their stdout/stderr are discarded.

!!! note
    Config changes require restarting perles to take effect.

### Configuration

```yaml
ui:
  actions:
    issue_action:
      open-claude:
        key: "1"
        command: 'tmux split-window -h claude "Work on {{.ID}}: "{{.TitleText}}'
        description: "Open Claude"
```

`key` and `command` are required; `description` is shown in the help overlay (`?`). See `examples/user-actions.yaml` in the repository for more examples.

### Template Variables

| Variable | Description | Escaped |
|----------|-------------|---------|
| `{{.ID}}` | Issue ID (e.g., "PROJ-123") | No |
| `{{.TitleText}}` | Issue title as a single shell word | Yes (POSIX single-quoted) |
| `{{.Title}}` | Issue title, inserted raw | No |

Any other field makes the action fail with `template rendering failed`, and the command doesn't run.

`{{.TitleText}}` is already wrapped in single quotes (an embedded `'` becomes `'\''`), so use it bare. Don't put it inside `'...'` or `"..."`: that undoes the escaping and lets the title run as shell code. To add fixed text, quote the text separately and place it right next to the variable with no space, as in `"Work on {{.ID}}: "{{.TitleText}}`. `{{.Title}}` is unescaped, so quotes, `$(...)`, backticks, `;`, or newlines in a title can break or change the command.

!!! warning
    `tmux split-window` and `new-window` run a pane command given as one quoted string through `sh -c` again, and `{{.TitleText}}` is only escaped for one shell. Pass the pane command as separate arguments, as in the example above, so tmux runs it directly.

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
| `user_notification` | User attention needed (no built-in sound; plays only if `override_sounds` is set) |
| `worker_out_of_context` | Worker ran out of context |
| `coordinator_out_of_context` | Coordinator ran out of context |
| `observer_out_of_context` | Observer ran out of context |
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

`sound` is a top-level key. Config files generated by older versions nested it under `orchestration:`, where it is ignored; move it to the top level for its settings to apply.

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Whether to play sounds for this event |
| `override_sounds` | list | Custom sound file paths. Multiple = random selection |

Events left out of `sound.events` still play their built-in sound. An event you do list plays only if it sets `enabled: true`: listing it with just `override_sounds` (or with `enabled: false`) silences it.

Each `override_sounds` path may start with `~`, which is expanded to your home directory (`$HOME` and other environment variables are not). Every file must end in `.wav` (any case), exist, resolve (after following symlinks) inside `~/.perles/sounds/`, and be 1MB or smaller. Otherwise `perles` refuses to start with `invalid sound configuration: ...`.
