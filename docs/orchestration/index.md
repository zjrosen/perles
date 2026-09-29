# Orchestration

!!! warning "Cost Warning"
    Orchestration spawns multiple headless AI agents. If you care about cost, use it carefully. Every headless agent runs with **full permissions**.

Orchestration is a multi-agent control plane, run from the dashboard, that launches workflows where a single coordinator agent manages spawning, replacing, and retiring headless worker agents through built-in MCP tools. The coordinator delegates sub-tasks to multiple workers so you don't have to manually manage sessions.

- **Coordinator**: A single headless agent that receives workflow instructions and manages the session
- **Workers**: Multiple headless agents that execute specific sub-tasks (coding, testing, reviewing, documenting)

![Control Plane](../assets/control-plane.png)

---

## Getting Started

1. Open Perles in your project directory
2. Press `ctrl+o` in Kanban mode to open the orchestration dashboard (configurable via `ui.keybindings.dashboard`)
3. Press `n` to open the New Workflow dialog, select a template, and fill in any required fields

A typical coding workflow spans multiple workflow sessions:

1. **Research Proposal** -- Generate a proposal document
2. **Research to Tasks** -- Break down the proposal into beads epics and tasks
3. **Cook** -- Work through the entire epic's tasks with code review

---

## Configuration

The default settings use Claude Code. Customize them in your config file: `.perles/config.yaml` in the project directory takes precedence over `~/.config/perles/config.yaml` (see [Configuration](../configuration/index.md) for the full lookup order). If no config file exists anywhere, launching `perles` in a beads project writes a default `.perles/config.yaml`.

```yaml
orchestration:
  coordinator_client: "claude"  # Options: claude, amp, codex, gemini, opencode, cursor
  worker_client: "claude"       # Options: claude, amp, codex, gemini, opencode, cursor

  # Provider-specific settings
  claude:
    model: "claude-opus-5-5"    # Default; aliases opus, sonnet, haiku also work
  amp:
    model: "opus"               # Options: opus, sonnet
    mode: "smart"               # Options: free, rush, smart
  codex:
    model: "gpt-6-sol"          # Default; e.g. gpt-6-luna (any Codex model ID is passed through)
  gemini:
    model: "gemini-3.8-flash"   # Default; e.g. gemini-3.1-pro-preview
  opencode:
    model: "anthropic/claude-opus-5-5"  # Default
  cursor:
    model: ""                   # Empty = Cursor's default model
```

---

## Git Worktrees

Any workflow supports working inside a git worktree. In the New Workflow dialog, the **Git Worktree** field offers **No Worktree** (the default), **Existing Worktree** (pick a worktree you already created), or **New Worktree** (choose a base branch and an optional branch name, auto-generated if left empty). Worktrees are primarily useful when running multiple "Cook" workflows on different epics in parallel.

Research and planning workflows typically don't need worktrees since they don't change code.

![New Workflow](../assets/new-workflow.png)

---

## Layout

### Workflows Pane

Every launched workflow appears in a table showing status, name, epic ID, working directory, worker count, health (time since the last heartbeat), uptime, and start time. The epic ID, working directory, and start time columns are hidden when the table is narrow, for example while the coordinator panel is open. A 🔔 marks a workflow whose coordinator requested your attention (cleared when you press `Enter` on it or click it), and a 🔒 marks one owned by another Perles process.

- **Pause**: Press `x` to stop all running processes for the selected workflow
- **Resume**: Press `s` to resume a paused workflow (or start a pending one)
- **New**: Press `n` to create a new workflow; it starts as soon as it is created

### Coordinator Panel

Press `ctrl+w` (or `Enter` on a workflow) to open the coordinator panel for the selected workflow. Its tabs are `Coord` (the coordinator chat), `Obs` (the observer, when `orchestration.observer_enabled` is set), `Msgs` (the message log), `CmdLog` (debug mode only), and one tab per worker. Switch tabs with `ctrl+k` / `ctrl+j` or by clicking them.

The coordinator is the headless AI agent that plans and delegates work. Communicate via the chat input.

**What you see:**

- Status indicator on each agent tab
- Context usage for the active tab, such as `27k/1000k` (bottom-right of the pane)
- Queue count when messages are pending, such as `[2 queued]` (bottom-left of the pane)
- Full conversation history

**Status indicators:**

| Icon | Meaning |
|------|---------|
| `●` (blue) | Working -- actively processing |
| `○` (green) | Ready -- waiting for input |
| `○` (muted) | Pending / starting |
| `⏸` | Paused -- workflow paused |
| `⚠` (yellow) | Stopped -- needs attention |
| `✗` (red) | Retired or failed |

### Message Log (`Msgs` Tab)

Timeline of all inter-agent communication. Workers post to the message log when they finish their turns, which nudges the coordinator to read and act.

Workers are enforced to end their turn with an MCP tool call to post their message. If they don't, the system intercepts and reminds them.

### Worker Tabs

Spawned workers appear as tabs labeled `W1`, `W2`, … with a status indicator (same icons as the coordinator). Each tab shows the worker's output, with its context usage and queue count on the pane border.

**Worker phases:**

Workers also track a workflow phase. Phases are internal workflow state and are not shown in the TUI; the coordinator reads them with the `query_worker_state` MCP tool.

| Phase | Meaning |
|-------|---------|
| `idle` | Ready for assignment |
| `implementing` | Implementing a task |
| `awaiting_review` | Implementation done, waiting for a reviewer |
| `reviewing` | Reviewing another worker's code |
| `addressing_feedback` | Fixing issues from a review denial |
| `committing` | Committing changes |

### Chat Input Bar

Text input for messaging the coordinator or posting to a shared channel. The label in the bottom-right corner of the input shows the active target:

- **`DM: Coordinator`** sends directly to the coordinator
- **`#general`**, **`#tasks`**, **`#planning`** (and **`#observer`** when the observer is enabled) post to that channel

Press `Tab` to cycle targets. Switching to a channel also shows the `Msgs` tab, and switching back to `DM: Coordinator` shows the `Coord` tab. In a channel message, type `@` to mention `@coordinator`, `@worker-N`, `@observer` (when enabled), or `@here` (all participants); mentioned agents are notified.

Sending a message in a channel starts a thread and makes it the active thread, so your next messages in that channel are replies to it. Press `ctrl+t` to pick a different thread. The active thread is shown top-right as `↩ <id>`, and `esc` on an empty input leaves it so your next message starts a new thread.

When `ui.vim_mode` is enabled, the current vim mode is displayed (bottom-left).

### Epic Tree and Details

Every workflow is backed by a beads epic. View progress and task details in the tree pane. The "Cook" workflow uses an existing epic; other workflows create one automatically.

---

## Keybindings

### Dashboard Mode

Focus zones cycle in this order: workflow table, epic tree, epic details, then the coordinator panel (when open). With the workflow table focused:

| Key | Action |
|-----|--------|
| `j` / `k` | Move between workflows |
| `g` / `G` | Jump to first / last workflow |
| `Tab` / `ctrl+n` | Next focus zone |
| `Shift+Tab` / `ctrl+p` | Previous focus zone |
| `/` | Activate filter (`Enter` keeps the filter, `esc` clears it) |
| `esc` | Clear filter (with no filter active, return to Kanban) |
| `s` | Start / resume workflow |
| `x` | Pause workflow |
| `n` | New workflow |
| `r` | Rename workflow |
| `a` | Archive workflow (requires `flags.session-persistence`; not while it is running) |
| `o` | Open session in the [web session viewer](#web-session-viewer-api) |
| `Enter` | Focus coordinator panel (opens it if closed) |
| `ctrl+w` | Toggle coordinator panel |
| `ctrl+k` / `ctrl+j` | Previous / next coordinator panel tab |
| `?` | Help |
| `q` | Return to Kanban (workflows keep running) |

### Coordinator Panel (Chat Input)

With the coordinator panel focused:

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `alt+enter` | Insert newline |
| `Tab` | Cycle channel (`DM: Coordinator` → `#general` → `#tasks` → `#planning`, plus `#observer` when enabled) |
| `@` | Mention autocomplete (`@coordinator`, `@worker-N`, `@observer` when enabled, `@here`) |
| `ctrl+t` | Thread picker (channels only, not DM) |
| `esc` | Leave the active thread (empty input); with `vim_mode`, switch to normal mode |
| `ctrl+k` / `ctrl+j` | Previous / next tab |
| `ctrl+n` / `ctrl+p` / `Shift+Tab` | Move focus out of the panel |
| `ctrl+w` | Close panel (vim normal mode only; otherwise move focus out first) |
| `ctrl+c` | Return to Kanban |

### Epic Tree View

With the epic tree or details pane focused (`?` and `ctrl+w` also work here):

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate tree (scroll when the details pane is focused, where `g` / `G` jump to top / bottom) |
| `h` / `l` | Switch tree / details pane |
| `Enter` | Refocus tree on selected issue |
| `d` | Toggle direction (down: children and blocked issues; up: parent and blockers) |
| `m` | Toggle mode (deps / children) |
| `ctrl+e` | Edit issue |
| `c` | Add comment (details pane) |
| `y` | Copy ID (tree pane) / description (details pane) |
| `q` / `esc` | Return to Kanban |

---

## Slash Commands

Control workers directly from the chat input:

| Command | Action |
|---------|--------|
| `/spawn` | Spawn a new worker |
| `/stop <process-id> [--force]` | Gracefully stop a process (`--force` kills it immediately, even mid-commit) |
| `/retire <worker-id> [reason]` | Gracefully retire a worker (not allowed for the coordinator) |
| `/replace <process-id> [reason]` | Replace a worker, the coordinator (`/replace coordinator`), or the observer with a fresh process |

Unknown slash commands are sent to the coordinator as plain text.

---

## Session Storage

Session data is stored in `~/.perles/sessions/` by default (set `orchestration.session_storage.base_dir` to change it). Each project gets its own directory, named from `orchestration.session_storage.application_name` if set, otherwise the git `origin` repository name, otherwise the working directory name (`perles daemon` skips the git lookup). See [Configuration](../configuration/index.md).

```
~/.perles/sessions/
├── sessions.json                    # Global session index
└── {application-name}/
    ├── sessions.json                # Per-application index
    └── 2026-01-12/
        └── {session-uuid}/
            ├── metadata.json
            ├── coordinator/
            ├── workers/
            └── messages.jsonl
```

---

## Web Session Viewer & API

The first time you open the dashboard, Perles starts a local HTTP server on `localhost`. It uses the port from `perles -p/--port`, else `orchestration.api_port`; `0` (the default) picks a free port.

- The web session viewer is served at `/`. Press `o` on a workflow in the dashboard to open its session in your browser. If no browser can be opened, a toast shows the URL instead.
- The workflow REST API is served under `/api/v1`: `GET /templates`, `GET /workflows`, `POST /workflows`, `GET /workflows/{id}`, `POST /workflows/{id}/start` (also `/pause` and `/resume`), `GET /health`, and Server-Sent Events streams at `GET /events` and `GET /workflows/{id}/events`. See [HTTP API](../CONTROL_PLANE.md#http-api) for request and response details.
- `perles daemon [-p <port>]` runs the same API and viewer without the TUI.

The viewer reads sessions from `orchestration.session_storage.base_dir` (default `~/.perles/sessions`).
