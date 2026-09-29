# Community Workflows

Community-contributed workflow templates for the Perles orchestration dashboard.
Workflows here are embedded at compile time and distributed with the binary,
but users must explicitly opt in via config to use them.

## Directory Structure

```
workflows/
├── README.md                          # This file
├── v1-epic-instructions.md            # Shared: default coordinator system prompt
├── v1-human-review.md                 # Shared: human review checkpoint template
└── joke-contest/                      # One directory per workflow
    ├── template.yaml                  # Workflow definition (required)
    ├── v1-joke-contest-epic.md        # Epic template
    ├── v1-joke-contest-joke-1.md      # Node template: joker 1
    ├── v1-joke-contest-joke-2.md      # Node template: joker 2
    └── v1-joke-contest-judge.md       # Node template: judge
```

Each workflow lives in its own subdirectory with a `template.yaml` that defines
the workflow metadata, arguments, and node DAG. Markdown files provide the
prompt templates for each node.

## Enabling a Community Workflow

Users opt in by adding the workflow key to their config:

```yaml
# ~/.config/perles/config.yaml (or .perles/config.yaml)
orchestration:
  community_workflows:
    - "joke-contest"                   # bare key works
    - "workflow/joke-contest"          # fully-qualified also works
```

Restart perles after changing config. The workflow will appear in the
dashboard's new workflow modal (press `n`) and in `perles workflows` with a
`[community]` label.

## Creating a New Community Workflow

### 1. Create a directory

```
workflows/my-workflow/
```

### 2. Add a `template.yaml`

```yaml
registry:
  - namespace: "workflow"
    key: "my-workflow"
    version: "v1"
    name: "My Workflow"
    description: "Brief description of what this workflow does"
    epic_template: "v1-my-workflow-epic.md"
    labels:
      - "category:work"            # or category:meta, category:research, etc.

    arguments:                     # optional user-configurable parameters
      - key: "topic"
        label: "Topic"
        description: "What should the workflow focus on?"
        type: "text"
        required: true

    nodes:
      - key: "research"
        name: "Research Phase"
        template: "v1-my-workflow-research.md"
        assignee: "worker-1"

      - key: "review"
        name: "Human Review"
        template: "v1-human-review.md"   # shared template (auto-resolved)
        assignee: "human"
        after:
          - "research"

      - key: "implement"
        name: "Implementation"
        template: "v1-my-workflow-implement.md"
        assignee: "worker-2"
        after:
          - "review"
```

### 3. Add prompt templates

Each node references a markdown template file. Templates in the workflow
directory are resolved first, then shared templates in `workflows/`, then
perles' built-in templates as a final fallback.

Template files support Go template syntax with these variables:

| Variable | Description |
|----------|-------------|
| `{{.Slug}}` | Workflow name as entered in the New Workflow form (defaults to the template key; not slugified) |
| `{{.Name}}` | Same value as `{{.Slug}}` |
| `{{.Date}}` | Current date (`YYYY-MM-DD`) |
| `{{.Args.key}}` | User-provided argument values |
| `{{.Inputs.key}}` | Input artifact paths (node templates only) |
| `{{.Outputs.key}}` | Output artifact paths (node templates only) |
| `{{.Config.document_path}}` | Base docs path from `orchestration.templates.document_path` (default `docs/proposals`) |

The `system_prompt` template is passed to the coordinator as-is and is not rendered.

### 4. Key conventions

- **Namespace** must be `"workflow"`
- **Key** should be lowercase with hyphens (e.g., `"code-review"`)
- **Version** should be `"v1"` (bump when making breaking changes)
- **Assignees**: `worker-1` through `worker-99` for AI agents, `human` for review checkpoints
- **Node ordering**: dependencies come from `after` and from `inputs` (a node waits for whichever node outputs the same `file`). Nodes with neither run in parallel. Every input `file` must exactly match another node's output `file`
- **File naming**: prefix template files with the version (e.g., `v1-my-workflow-research.md`)

Community workflows load as a set: if any community `template.yaml` is invalid
(bad YAML, missing template, dangling input, unknown `after` key, cycle, invalid
assignee), no community workflows load. The warning
(`loading community registrations`) is only visible in `debug.log` when perles
runs with `-d`.

### Shared Templates

Templates at the top level of `workflows/` are shared across all community
workflows. Currently available:

| Template | Purpose |
|----------|---------|
| `v1-epic-instructions.md` | Default coordinator system prompt (auto-applied if `system_prompt` is omitted) |
| `v1-human-review.md` | Human review checkpoint that pauses for user input |

Reference shared templates by filename in your node definitions. The loader
resolves them automatically -- it checks the workflow directory first, then
the shared `workflows/` directory, then perles' built-in templates
(`internal/templates/workflows/`).

These two files are copies of perles' built-in shared templates and must stay
identical to them; `TestSharedTemplateSyncCheck` fails if they diverge.