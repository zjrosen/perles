# Custom Workflows

Create your own workflow templates to define custom orchestration patterns. Templates in `~/.perles/workflows/` are loaded at startup and appear in the dashboard's New Workflow dialog (press `ctrl+o` on the kanban board to open the dashboard, then `n`). `perles workflows` also lists them with a `[user]` label. Restart perles after adding or changing a template.

## Template Location

Place custom templates in:

```
~/.perles/workflows/{your_workflow_name}/
```

Each workflow directory contains a `template.yaml` file and markdown templates for each task.

---

## Template Structure

A workflow template consists of:

```
~/.perles/workflows/joke-contest/
├── template.yaml                    # DAG definition and metadata
├── v1-joke-contest-epic.md          # Epic instructions (coordinator)
├── v1-joke-contest-joke-1.md        # Task 1 template
├── v1-joke-contest-joke-2.md        # Task 2 template
└── v1-joke-contest-judge.md         # Task 3 template
```

The human review node's `v1-human-review.md` and the default coordinator system prompt `v1-epic-instructions.md` come from perles' built-in shared templates, so you don't need to copy them (see [Shared Templates](#shared-templates)).

---

## Template YAML Reference

### Top-Level Registration Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespace` | string | Yes | Always `"workflow"` |
| `key` | string | Yes | Unique identifier (e.g., `"joke-contest"`) |
| `version` | string | Yes | Version identifier (e.g., `"v1"`) |
| `name` | string | Yes | Human-readable name for the workflow picker |
| `description` | string | Yes | Description of what the workflow does |
| `epic_template` | string | No (recommended) | Filename of the markdown template rendered as the epic description. If omitted, the epic description is just `Workflow: <key>` and `Feature: <name>` |
| `system_prompt` | string | No | Filename of the coordinator system prompt template. Orchestration workflows (any node has an `assignee`, or the workflow is [epic-driven](#epic-driven-workflows)) default to `v1-epic-instructions.md`, which resolves to perles' built-in copy unless you provide your own |
| `path` | string | No | Path prefix for artifact inputs/outputs (e.g., `".spec"`) |
| `labels` | list | No | Tags for filtering (e.g., `["category:meta", "lang:go"]`) |
| `arguments` | list | No | User-configurable parameters |
| `nodes` | list | Conditional | DAG of workflow tasks. Required unless the workflow is [epic-driven](#epic-driven-workflows) or sets `system_prompt` (node-less orchestration: an epic is created but no tasks) |

### Argument Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string | Yes | Unique identifier, accessed as `{{.Args.key}}` |
| `label` | string | Yes | Human-readable label for form field |
| `description` | string | No | Help text / placeholder |
| `type` | string | Yes | Input type: `text`, `number`, `textarea`, `select`, `multi-select`, `epic-search` (searchable epic picker) |
| `required` | bool | No | Whether the field must be filled (default: `false`) |
| `default` | string | No | Default value |
| `options` | list | Conditional | Required for `select` and `multi-select` types |

### Node Fields (DAG Tasks)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string | Yes | Unique identifier within the workflow |
| `name` | string | Yes | Display name for the task |
| `template` | string | Yes | Filename of the markdown template |
| `assignee` | string | No | Worker role: `"worker-1"` through `"worker-99"`, or `"human"` for review gates |
| `after` | list | No | Node keys this node depends on (runs after these complete) |
| `inputs` | list | No | Artifacts consumed by this node. Each `file` must match another node's output `file`, and that node becomes an implicit dependency |
| `outputs` | list | No | Artifacts produced by this node |

### Artifact Fields (Inputs/Outputs)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string | Yes | Stable identifier for template access (e.g., `{{.Inputs.plan}}`) |
| `file` | string | Yes | Filename, may contain template syntax |

---

## Complete Example

### template.yaml

```yaml
registry:
  - namespace: "workflow"
    key: "joke-contest"
    version: "v1"
    name: "Joke Contest"
    description: "Two workers write jokes in parallel, then a third judges"
    epic_template: "v1-joke-contest-epic.md"
    labels:
      - "category:meta"

    arguments:
      - key: "theme"
        label: "Joke Theme"
        description: "Optional theme or topic for the jokes"
        type: "text"
        required: false

    nodes:
      # Phase 1: Parallel joke writing
      - key: "joke-1"
        name: "Joker 1 - Write Joke"
        template: "v1-joke-contest-joke-1.md"
        assignee: "worker-1"

      - key: "joke-2"
        name: "Joker 2 - Write Joke"
        template: "v1-joke-contest-joke-2.md"
        assignee: "worker-2"

      # Phase 2: Human review gate
      - key: "review"
        name: "Human Review"
        template: "v1-human-review.md"
        assignee: "human"
        after:
          - "joke-1"
          - "joke-2"

      # Phase 3: Judge picks winner
      - key: "judge"
        name: "Judge - Pick Winner"
        template: "v1-joke-contest-judge.md"
        assignee: "worker-3"
        after:
          - "review"
```

### Epic Template (v1-joke-contest-epic.md)

Epic templates use Go template syntax and have access to workflow arguments:

```markdown
# Joke Contest: {{.Name}}

You are the **Coordinator** for a joke contest workflow.

## Context

{{- if .Args.theme}}
- **Theme:** {{.Args.theme}}
{{- else}}
- **Theme:** Any topic (no theme specified)
{{- end}}

## Your Workers

| Worker | Role | Phase |
|--------|------|-------|
| worker-1 | Joker 1 | 1 |
| worker-2 | Joker 2 | 1 |
| human | Reviewer | 2 |
| worker-3 | Judge | 3 |

**NOTE:** You (the Coordinator) are NOT a worker. Start immediately.
```

---

## Template Variables

Epic and node templates have access to these variables:

| Variable | Description |
|----------|-------------|
| `{{.Name}}` | Name entered in the New Workflow dialog (defaults to the template `key` when left blank) |
| `{{.Slug}}` | Same value as `{{.Name}}` (not slugified) |
| `{{.Args.<key>}}` | User-provided argument values |
| `{{.Date}}` | Current date (`YYYY-MM-DD`) |
| `{{.Inputs.<key>}}` | Input artifact path (`path` prefix, if set, + rendered `file`); node templates only |
| `{{.Outputs.<key>}}` | Output artifact path (`path` prefix, if set, + rendered `file`); node templates only |
| `{{.Config.document_path}}` | Value of `orchestration.templates.document_path` (default `docs/proposals`) |

Artifact `file` values can use the same variables except `Inputs` and `Outputs`. The `system_prompt` file is sent to the coordinator as-is, without template rendering.

---

## DAG Execution

Nodes define a directed acyclic graph (DAG):

- Nodes with no `after` and no `inputs` start immediately (can run in parallel)
- A node waits for every node listed in `after` and for every node that produces one of its `inputs`
- Every `inputs[].file` must exactly match an `outputs[].file` of another node (compared before template rendering), and no two nodes may output the same `file`
- `after` may only reference existing node keys, and the graph must not contain cycles
- `assignee: "human"` creates a human review gate that pauses for manual approval

A workflow that breaks any of these rules fails to load (see [Troubleshooting](#troubleshooting)).

```
joke-1 ──┐
          ├──> review (human) ──> judge
joke-2 ──┘
```

---

## Shared Templates

User workflows can reference perles' built-in shared templates by filename without copying them:

| Template | Purpose |
|----------|---------|
| `v1-epic-instructions.md` | Default coordinator system prompt (used when `system_prompt` is omitted) |
| `v1-human-review.md` | Human review checkpoint for `assignee: "human"` nodes |

A bare filename (no `/`) in `epic_template`, `system_prompt`, or a node `template` is looked up in this order:

1. `~/.perles/workflows/{your_workflow_name}/<file>`
2. `~/.perles/workflows/<file>`
3. perles' built-in shared templates

Your files always take precedence. To customize a shared template for one workflow, put a file with the same name in that workflow's directory. To customize it for all your workflows, put it directly in `~/.perles/workflows/`. (Markdown files placed there are also scanned as chat panel workflows; files without workflow frontmatter are skipped with an error in `debug.log`.)

Paths that contain `/` are relative to `~/.perles/` (e.g., `workflows/v1-human-review.md` means `~/.perles/workflows/v1-human-review.md`). If the file doesn't exist there, perles uses the file at the same path in its built-in templates. Absolute paths and paths containing `..` are rejected.

---

## Epic-Driven Workflows

An epic-driven workflow runs against an existing epic instead of creating a new one. A workflow is epic-driven when it has exactly one argument, with key `epic_id`, and no `nodes`. The built-in **Cook** workflow works this way:

```yaml
registry:
  - namespace: "workflow"
    key: "cook"
    version: "v1"
    name: "Cook"
    description: "Sequential task execution with code review and worker cycling for an existing epic"
    system_prompt: "v1-cook-instructions.md"
    labels:
      - "category:work"
    arguments:
      - key: "epic_id"
        label: "Epic ID"
        description: "The epic ID to work on (e.g., perles-abc1)"
        type: "epic-search"
        required: true
```

When you start an epic-driven workflow, perles creates no epic or tasks. The coordinator receives the `system_prompt` content plus the selected epic ID, so put the coordinator's instructions in `system_prompt`. `epic_template` is not used.

---

## Troubleshooting

If your workflow doesn't appear in the New Workflow dialog or in `perles workflows`:

- **Restart perles.** Templates are only read at startup.
- **Check the debug log.** Run `perles -d` and search `debug.log` for `skipping invalid workflow`. Each entry includes the `template.yaml` path and the error, for example a missing template file, an invalid assignee, an unknown `after` key, or an input with no matching output.
- **Invalid workflows are skipped individually.** A YAML parse error (or an unreadable file) skips the whole `template.yaml`. An error in one registration skips only that registration, so other workflows (and other registrations in the same file) still load. If no user workflow is valid, the log shows `loading user registrations`.
- **Keys must be unique.** A user workflow with the same `namespace` and `key` as a built-in or community workflow replaces it. For example, the `joke-contest` key above is also used by the community Joke Contest workflow.
- **Size limit.** A `template.yaml` larger than 1MB is skipped (`skipping oversized template.yaml`).
