# BQL Reference

BQL (Beads Query Language) is the query language used throughout Perles to filter and organize issues. It powers column definitions, search mode, and dependency exploration.

## Basic Syntax

```
field operator value [and|or field operator value ...]
```

---

## Example Queries

Critical bugs:

```bql
type = bug and priority = P0
```

Ready work, excluding backlog:

```bql
status = open and ready = true and label not in (backlog)
```

Recently updated high-priority items:

```bql
priority <= P1 and updated >= -24h order by updated desc
```

Search by title:

```bql
title ~ authentication or title ~ login
```

Epic with all its children:

```bql
type = epic expand down depth *
```

---

## Fields

| Field | Description | Example Values |
|-------|-------------|----------------|
| `status` | Issue status | `open`, `in_progress`, `closed` |
| `type` | Issue type | `bug`, `feature`, `task`, `epic`, `chore`, `milestone`, `story`, `spike` |
| `priority` | Priority level | `P0`, `P1`, `P2`, `P3`, `P4` |
| `blocked` | Has blockers | `true`, `false` |
| `ready` | Open, unblocked, and not future-deferred | `true`, `false` |
| `pinned` | Is pinned | `true`, `false` |
| `is_template` | Is a template | `true`, `false` |
| `label` | Issue labels | any label string |
| `title` | Issue title | any text (use `~` for contains) |
| `description` | Issue description | any text (use `~` for contains) |
| `design` | Design notes | any text (use `~` for contains) |
| `notes` | Issue notes | any text (use `~` for contains) |
| `id` | Issue ID | e.g., `bd-123` |
| `assignee` | Assigned user | username |
| `sender` | Issue sender | username |
| `created_by` | Issue creator | username |
| `mol_type` | Molecule type | string |
| `created` | Creation date | `today`, `yesterday`, `-7d`, `-3m` |
| `updated` | Last update | `today`, `-24h` |
| `defer_until` | Deferred-until date | `now`, `today`, `-7d` |

### Metadata Fields

Issues can have custom metadata stored as JSON key-value pairs. Use the `metadata.<key>` syntax to filter by metadata:

| Field | Description | Example Values |
|-------|-------------|----------------|
| `metadata.<key>` | Custom metadata field | any string value |

#### Metadata Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `=` | Equals metadata value | `metadata.team = "backend"` |
| `!=` | Not equals metadata value | `metadata.team != "frontend"` |
| `~` | Contains pattern | `metadata.component ~ auth` |
| `!~` | Not contains pattern | `metadata.team !~ infra` |
| `= nil` | Key does not exist | `metadata.team = nil` |
| `!= nil` | Key exists | `metadata.team != nil` |

#### Metadata Examples

```bql
# Filter by metadata value
metadata.team = "backend"

# Combined with other filters
type = task and metadata.team = "backend"

# Find issues WITHOUT a specific key
metadata.sprint = nil

# Find issues WITH a specific key
metadata.component != nil

# Pattern matching in metadata values
metadata.component ~ auth

# Multiple conditions
type = bug and metadata.severity = "critical" or metadata.team = "security"
```

#### Nested Metadata

Metadata keys can contain dots for namespacing:

```bql
metadata.jira.sprint = "Q1-2026"
metadata.jira.priority = "high"
```

---

## Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `=` | Equals | `status = open` |
| `!=` | Not equals | `type != chore` |
| `<` | Less than | `priority < P2` |
| `>` | Greater than | `priority > P3` |
| `<=` | Less or equal | `priority <= P1` |
| `>=` | Greater or equal | `created >= -7d` |
| `~` | Contains | `title ~ auth` |
| `!~` | Not contains | `title !~ test` |
| `in` | In list | `status in (open, in_progress)` |
| `not in` | Not in list | `label not in (backlog)` |

---

## Boolean Logic

Combine conditions with `and`, `or`, `not`, and parentheses:

```bql
# AND - both conditions must match
status = open and priority = P0

# OR - either condition matches
type = bug or type = feature

# NOT - negate a condition
not blocked = true

# Parentheses for grouping
(type = bug or type = feature) and priority <= P1
```

---

## Date Filters

BQL supports relative dates and named dates:

```bql
# Relative dates
created >= -7d          # Last 7 days
updated >= -24h         # Last 24 hours
created >= -3m          # Last 3 months

# Named dates
defer_until > now
created >= today
created >= yesterday
```

---

## Sorting

Sort results with `order by`:

```bql
# Single field
status = open order by priority

# Multiple fields with direction
type = bug order by priority asc, created desc
```

---

## Expand

The `expand` keyword includes related issues in results, allowing you to see complete issue hierarchies and dependency chains.

```
<filter> expand <direction> [depth <n>]
```

### Directions

| Direction | Description |
|-----------|-------------|
| `up` | Issues you depend on (parents + blockers) |
| `down` | Issues that depend on you (children + blocked issues) |
| `all` | Both directions combined |

### Depth Control

| Depth | Description |
|-------|-------------|
| `depth 1` | Direct relationships only (default) |
| `depth 2-10` | Include relationships up to N levels deep |
| `depth *` | Unlimited depth (follows all relationships) |

### Expand Examples

```bql
# Get an epic and all its children
type = epic expand down

# Get an epic and all descendants (unlimited depth)
type = epic expand down depth *

# Get an issue and everything blocking it
id = bd-123 expand up

# Get an issue and all related issues (both directions)
id = bd-123 expand all depth *

# Get all epics with their full hierarchies
type = epic expand all depth *
```
