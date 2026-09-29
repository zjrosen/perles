# Usage

Perles operates in multiple modes, each offering a different way to view and interact with your issues. Switch between them using keyboard shortcuts.

## Application Modes

| Mode | Access | Description |
|------|--------|-------------|
| **[Kanban](kanban.md)** | Default / `ctrl+space` | Organize issues in customizable board columns |
| **[Search](search.md)** | `ctrl+space` / `/` from kanban | Full-screen BQL search with live results |
| **[Dependency Explorer](dependency-explorer.md)** | `Enter` on an issue in kanban or search results | Visualize issue relationships as trees |
| **[Orchestration](../orchestration/index.md)** | `ctrl+o` from kanban | Multi-agent AI workflow management |

## Common Keybindings

These work in Kanban and Search modes. In Search, `ctrl+e`, `ctrl+g`, `?` and `q` only work once focus has left the search input (press `Enter`, `Tab` or `↓`).

| Key | Action |
|-----|--------|
| `ctrl+space` | Switch between Kanban and Search (configurable via `ui.keybindings.search`) |
| `ctrl+o` | Open the Dashboard / Orchestration mode from Kanban (configurable via `ui.keybindings.dashboard`) |
| `ctrl+e` | Edit the selected issue (also in the Dashboard epic tree) |
| `ctrl+w` | Toggle the [AI chat panel](#ai-chat-panel) |
| `ctrl+g` | Open the git diff viewer (press `?` inside it for its keys, `esc` or `q` to close) |
| `?` | Toggle help overlay |
| `q` | Quit immediately |
| `ctrl+c` | Quit with confirmation (press `ctrl+c` again to quit, `esc` to cancel) |

In the Dashboard, `q`, `ctrl+c` and `esc` return to Kanban instead of quitting.

## AI Chat Panel

Press `ctrl+w` in Kanban or Search to open an AI assistant chat panel beside the current view (the terminal must be at least 100 columns wide). It runs the coordinator agent configured by `orchestration.coordinator_client` (falling back to `orchestration.client`, then `claude`). While the panel is open, `tab` switches focus between the main view and the chat. With the chat focused:

| Key | Action |
|-----|--------|
| `ctrl+j` / `ctrl+k` | Next / previous tab (Chat, Sessions, Workflows) |
| `ctrl+t` | Jump to the Workflows tab; pick a workflow with `j`/`k` and press `enter` to run it |
| `ctrl+n` / `ctrl+p` | Next / previous chat session |

The panel closes when you open the Dashboard, where `ctrl+w` toggles the coordinator chat instead.

## Editing Issues

Press `ctrl+e` on any selected issue to open the issue editor. This works in kanban, search, dependency explorer, and the Dashboard epic tree. From the editor you can change the title, priority, parent (an epic or a task; epics have no parent field), status, labels, description, and notes. When you create an issue with `n` in kanban, you also choose its type. Use `ctrl+g` on text fields to open your `$VISUAL` or `$EDITOR` (falls back to `vi`) for longer edits.

Press `c` in an issue's details panel (search, dependency explorer, Dashboard epic tree) to add a comment.

![Issue Editor](../assets/edit-issue.png)
