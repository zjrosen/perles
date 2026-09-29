# Dependency Explorer

Visualize and navigate issue relationships -- blockers, dependencies, and parent/child hierarchies as interactive trees.

## Dependency Chain

![Dependency Chain](../assets/issues-dependencies.png)

## Parent/Child Hierarchy

![Parent Child Hierarchy](../assets/issues-children.png)

---

## Accessing the Explorer

The dependency explorer is a sub-mode of search. Press `Enter` on an issue in the search results, or on a card in kanban mode (clicking a card also works), to open the explorer with that issue as the root. A details panel for the selected node sits beside the tree. Press `/` to go back to the search list.

---

## Tree Modes

| Mode | Description |
|------|-------------|
| **Dependencies** (`deps`, default) | All relationships: parent/child, blocks/blocked-by, and discovered-from. The direction picks the side (down: children, blocked and discovered issues; up: parent, blockers, and discovered-from origin) |
| **Children** (`children`) | Parent/child hierarchy only, with no blocking or discovered-from links (down: children; up: parent chain) |

Toggle between modes with `m` while in the tree view. The tree pane's title shows the current direction and mode, for example `Tree (↓ down) (deps)`.

---

## Tree Direction

| Direction | Description |
|-----------|-------------|
| **Down** (default) | Navigate downward through children, blocked issues, and issues discovered from this one |
| **Up** | Navigate upward through parents, blockers, and discovered-from origins |

Toggle direction with `d` while in the tree view. The explorer always loads the full tree (`depth *`) in the current direction.

---

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `k` | Move cursor up/down in tree |
| `l` / `Tab` | Focus details panel |
| `h` | Focus tree panel |
| `Enter` | Refocus tree on selected node |
| `u` | Go back to previous root |
| `U` | Go to original root |
| `d` | Toggle direction (up/down) |
| `m` | Toggle mode (deps/children) |
| `y` | Copy issue ID |
| `ctrl+s` | Save the tree as a kanban tree column (new or existing view) |
| `ctrl+e` | Edit selected issue |
| `ctrl+d` | Delete selected issue |
| `c` | Add comment (details panel) |
| `0`-`9` | Run a [user-defined action](../configuration/index.md#user-defined-actions) on the selected issue (tree panel) |
| `/` | Switch to list mode |
| `ctrl+space` | Switch to kanban mode |
| `Esc` | Exit to kanban mode |
| `?` | Toggle help |
| `q` | Quit |

When you save a tree with `ctrl+s`, the current root becomes the column's `issue_id`, and the form's Tree Mode toggle (preset to the explorer's current mode) picks the column's mode: Parent-Child writes `tree_mode: child`, while Dependencies is the default and writes nothing. The direction isn't saved, because kanban tree columns always show the down direction; see [Tree Columns](kanban.md#tree-columns).

---

## Using with BQL Expand

The dependency explorer works alongside BQL's `expand` keyword. You can use expand queries to pull in related issues:

```bql
# Get an epic and its direct children (plus issues it directly blocks)
type = epic expand down

# Get an epic and its full hierarchy
type = epic expand down depth *

# Get an issue, its parent, and its direct blockers
id = bd-123 expand up

# Get an issue and its full upstream chain
id = bd-123 expand up depth *

# Full relationship graph
id = bd-123 expand all depth *
```

Without `depth`, `expand` goes one level deep.

See the [BQL Reference](../bql/index.md#expand) for full expand syntax.
