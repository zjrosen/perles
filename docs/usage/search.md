# Search Mode

Full-screen BQL-powered search interface with live results and issue details.

![Search Mode](../assets/search.png)

## Features

- Full-screen BQL-powered search interface
- Live results with detail panel
- Save searches as kanban columns
- Create new views from search results
- Sub-mode for viewing issue dependencies and hierarchies

---

## Videos

### BQL Search

Use `ctrl+space` to switch modes between Kanban and Search or while on a column use `/` to be dropped into search mode with the current column's BQL query.

<video src="./../assets/search.mp4" controls width="100%"></video>

### Creating a View from Search Results

Use `ctrl+s` from search mode to save the BQL query to a new or existing view.

<video src="./../assets/save-to-new-view.mp4" controls width="100%"></video>

---

## Using Search

1. Press `ctrl+space` from kanban mode, or `/` from a column to pre-fill its BQL query
2. Type a [BQL query](../bql/index.md) in the search input
3. Focus starts in the search input, where `j`/`k`/`h`/`l` are typed as text. Press `Enter` (or `Tab`/`↓`) to move to the results, then navigate with `j`/`k` and switch between the list and details with `h`/`l`
4. Press `Enter` on a result to open it in the [Dependency Explorer](dependency-explorer.md)

### Saving Searches

Press `ctrl+s` to save the current search as a column in a new or existing view. This lets you build kanban boards from your most-used queries.

---

## Keybindings

| Key | Action |
|-----|--------|
| `/` | Focus search input |
| `Enter` | Execute query (search input) / Open the [Dependency Explorer](dependency-explorer.md) for the selected result / Jump to the selected dependency (details) |
| `Tab` / `ctrl+n` / `ctrl+p` | Cycle focus between search input, results, and details |
| `h` | Move to results list |
| `l` | Move to details panel |
| `j` / `k` | Navigate results |
| `y` | Copy issue ID |
| `ctrl+s` | Save search as column |
| `ctrl+e` | Edit issue (title, priority, parent, status, labels, description, notes) |
| `ctrl+d` | Delete issue |
| `c` | Add comment (details panel) |
| `ctrl+g` | Open git diff viewer |
| `0`-`9` | Run a [user-defined action](../configuration/index.md#user-defined-actions) (results list) |
| `?` | Toggle help (includes a BQL reference) |
| `Esc` | Exit to kanban mode |
| `q` | Quit |
| `ctrl+c` | Quit with confirmation |

While the search input is focused, other keys are typed into the query. Only `Enter`, `Tab`/`ctrl+n`/`ctrl+p`, `↓`, `ctrl+s`, `Esc`, `ctrl+space`, `ctrl+w` and `ctrl+c` act as shortcuts there (with vim mode on, `Esc` first leaves insert mode).

---

## Search Examples

```bql
# Find critical bugs
type = bug and priority = P0

# Ready work excluding backlog
status = open and ready = true and label not in (backlog)

# Recently updated high-priority items
priority <= P1 and updated >= -24h order by updated desc

# Search by title
title ~ authentication or title ~ login

# Epic with its full hierarchy
type = epic expand down depth *
```

See the [BQL Reference](../bql/index.md) for the complete query language documentation.
