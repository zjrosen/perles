# BQL Reference

BQL (Beads Query Language) is the query language used throughout Perles to filter and organize issues. It powers column definitions, search mode, and dependency exploration.

## Basic Syntax

```
field operator value [and|or field operator value ...] [expand ...] [order by ...]
```

The `expand` and `order by` clauses are optional. When both are used, `expand` must come first. BQL has no comment syntax.

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

Epics with all their descendants:

```bql
type = epic expand down depth *
```

---

## Fields

| Field | Description | Example Values |
|-------|-------------|----------------|
| `status` | Issue status | `open`, `in_progress`, `blocked`, `deferred`, `closed` |
| `type` | Issue type | `bug`, `feature`, `task`, `epic`, `chore`, `milestone`, `story`, `spike` |
| `priority` | Priority level | `P0`, `P1`, `P2`, `P3`, `P4` (or `0`-`4`) |
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
| `mol_type` | Molecule type (not available with `backend: beads_rust`) | string |
| `owner` | Issue owner (`backend: beads_rust` only) | username |
| `created` | Creation date | `today`, `yesterday`, `-7d`, `-3m` |
| `updated` | Last update | `today`, `-24h` |
| `defer_until` | Deferred-until date | `now`, `today`, `-7d` |

The `backend` option is described in [Configuration](../configuration/index.md#general).

### Metadata Fields

Issues can have custom metadata stored as JSON key-value pairs. Use the `metadata.<key>` syntax to filter by metadata:

| Field | Description | Example Values |
|-------|-------------|----------------|
| `metadata.<key>` | Custom metadata field | any string value, or `nil` |

- **Values must be strings or `nil`.** Quote values that would otherwise parse as a priority, number, boolean, or date, e.g. `metadata.level = "P1"`, `metadata.points = "5"`, `metadata.flaky = "true"`. Unquoted words such as `metadata.team = backend` are strings.
- **Values compare as text.** Strings compare without their JSON quotes. Numbers compare as their JSON text, so `{"points": 5}` and `{"points": "5"}` both match `metadata.points = "5"`. Booleans compare as `true` or `false` (not `1`), and a JSON `null` compares as `null`. Dolt-backed databases normalize numbers first (`1.0` compares as `1`, `1e3` as `1000`), while SQLite compares them exactly as stored.
- `=` and `!=` are case-sensitive. `~` and `!~` are case-insensitive substring matches.
- `!=` and `!~` only match issues that have the key. Issues without the key are left out.
- `in` / `not in` are not supported on metadata fields. Combine conditions with `or` instead.
- Metadata fields are not available with `backend: beads_rust`. Any `metadata.*` query fails there.

#### Metadata Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `=` | Equals metadata value | `metadata.team = "backend"` |
| `!=` | Not equals metadata value (key must exist) | `metadata.team != "frontend"` |
| `~` | Contains pattern (case-insensitive) | `metadata.component ~ auth` |
| `!~` | Not contains pattern (key must exist) | `metadata.team !~ infra` |
| `= nil` | Key does not exist | `metadata.team = nil` |
| `!= nil` | Key exists (including a key set to JSON `null`) | `metadata.team != nil` |

#### Metadata Examples

Filter by metadata value:

```bql
metadata.team = "backend"
```

Combined with other filters:

```bql
type = task and metadata.team = "backend"
```

Find issues without a specific key:

```bql
metadata.sprint = nil
```

Find issues with a specific key:

```bql
metadata.component != nil
```

Pattern matching in metadata values:

```bql
metadata.component ~ auth
```

Critical bugs, plus any issue owned by the security team (`and` binds tighter than `or`):

```bql
type = bug and metadata.severity = "critical" or metadata.team = "security"
```

#### Nested Metadata

Use dots to reach into nested JSON objects. `metadata.jira.sprint` reads the `sprint` field inside a `jira` object, as in `{"jira": {"sprint": "Q1-2026", "priority": "high"}}`:

```bql
metadata.jira.sprint = "Q1-2026" and metadata.jira.priority = "high"
```

A top-level key that literally contains a dot (such as `{"jira.sprint": "Q1-2026"}`) cannot be matched. Keys must start with a letter or `_` and may contain only letters, digits, `_`, and `.`, so a key like `story-points` cannot be queried.

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
| `~` | Contains (case-insensitive) | `title ~ auth` |
| `!~` | Not contains (case-insensitive) | `title !~ test` |
| `in` | In list | `status in (open, in_progress)` |
| `not in` | Not in list | `label not in (backlog)` |

`~` and `!~` match a substring anywhere in the value. `%` and `_` in the search text act as SQL `LIKE` wildcards (any run of characters, and any single character). On SQLite databases, `~` and `!~` ignore case only for ASCII letters.

Operators are checked against the field type, so a query such as `status ~ open` fails validation. The supported combinations are:

- `<`, `>`, `<=`, `>=`: `priority` and date fields (`created`, `updated`, `defer_until`) only.
- `~`, `!~`: text fields (`title`, `description`, `design`, `notes`, `label`, `id`, `assignee`, `sender`, `created_by`, `mol_type`, `owner`) and metadata fields.
- `status`, `type`: `=`, `!=`, `in`, `not in`.
- Boolean fields (`blocked`, `ready`, `pinned`, `is_template`): `=` and `!=` only. `!=` currently behaves like `=` on these fields (`blocked != true` returns blocked issues), so write `blocked = false` or `not blocked = true` instead.
- `in` / `not in` are not valid on boolean or date fields.

---

## Boolean Logic

Combine conditions with `and`, `or`, `not`, and parentheses.

Precedence is `not`, then `and`, then `or`. `not` applies only to the single condition (or parenthesized group) that follows it, and `a and b or c` is evaluated as `(a and b) or c`. Use parentheses to override.

Both conditions must match (`and`):

```bql
status = open and priority = P0
```

Either condition matches (`or`):

```bql
type = bug or type = feature
```

Negate a condition (`not`):

```bql
not blocked = true
```

Group with parentheses:

```bql
(type = bug or type = feature) and priority <= P1
```

---

## Date Filters

BQL supports relative dates and named dates:

| Value | Meaning | Example |
|-------|---------|---------|
| `-Nd` | N days ago, from the start of that day | `created >= -7d` |
| `-Nh` | N hours ago | `updated >= -24h` |
| `-Nm` | N months ago, from the start of that day (`m` means months, not minutes) | `created >= -3m` |
| `now` | The current date and time | `defer_until > now` |
| `today` | The start of today | `created >= today` |
| `yesterday` | The start of yesterday | `created >= yesterday` |

Relative offsets must be negative. Future offsets and absolute dates are not supported: `+7d`, `2026-01-01` and `"2026-01-01"` are rejected, and a bare `7d` is compared as the literal text `7d`, which silently matches the wrong issues. To compare against the present, use `now` or `today`, e.g. `defer_until > now`.

Each value is a single point in time, so compare with `>=`, `>`, `<`, or `<=`. `created = today` does not mean "created today"; use `created >= today`.

---

## Sorting

Sort results with `order by`.

Open issues, highest priority (P0) first:

```bql
status = open order by priority
```

Multiple fields with direction:

```bql
type = bug order by priority asc, created desc
```

- Direction defaults to `asc`.
- Without `order by`, results are sorted by `updated desc` (most recently updated first).
- `label`, `blocked`, and `ready` cannot be used in `order by`. The query fails at run time.
- With `expand`, `order by` sorts only the issues matched by the filter. Expanded issues follow them, most recently updated first.

---

## Expand

The `expand` keyword includes related issues in results, allowing you to see complete issue hierarchies and dependency chains. Expanded issues are added whatever the filter says, so `type = epic and status = open expand down` can include closed children.

```
<filter> expand <direction> [depth <n>]
```

### Directions

Expansion follows every dependency, whatever its type.

| Direction | Description |
|-----------|-------------|
| `up` | Issues this issue depends on: its parent, blockers, discovered-from origins, and any other dependency type (e.g. `related`) |
| `down` | Issues that depend on this issue: its children, blocked issues, discovered issues, and any other dependency type |
| `all` | Both directions combined |

### Depth Control

| Depth | Description |
|-------|-------------|
| `depth 1` | Direct relationships only (default) |
| `depth 2-10` | Include relationships up to N levels deep |
| `depth *` | Unlimited depth (follows all relationships) |

### Expand Examples

Each epic and the issues that directly depend on it (its children, issues it blocks, etc.):

```bql
type = epic expand down
```

Each epic and all its descendants (unlimited depth):

```bql
type = epic expand down depth *
```

An issue and its direct dependencies (parent, blockers, etc.). Use `depth *` for the full chain:

```bql
id = bd-123 expand up
```

An issue and every issue connected to it, in both directions:

```bql
id = bd-123 expand all depth *
```

All epics and every issue connected to them:

```bql
type = epic expand all depth *
```
