---
name: "Create Workflow Template"
description: "Create a new workflow template with YAML registry, epic template, instructions, and node templates"
category: "Meta"
target_mode: "chat"
---

# Create Workflow Template (Chat)

## Overview

A single-agent workflow for creating new workflow templates. You will gather requirements from the user, then create a complete workflow template package with all necessary files.

> **CRITICAL DISTINCTION: Coordinator vs Workers**
>
> The **Coordinator** is the orchestrating agent - it is NOT a worker. The coordinator:
> - Receives pre-filled context via **arguments** 
> - Does NOT get assigned tasks in `template.yaml`
> - Assigns tasks to workers and monitors their progress
>
> **Workers** (worker-1, worker-2, etc.) are the agents that execute tasks defined in `template.yaml`.

**Output Directory:** `~/.perles/workflows/{workflow-name}/`

**Files Created:**
```
~/.perles/workflows/{workflow-name}/
├── template.yaml                    # Registry definition
├── v1-{name}-epic.md                # Epic template (coordinator instructions)
├── v1-epic-instructions.md          # Coordinator reference instructions
└── v1-{name}-{node-key}.md          # Node templates (one per task)
```

---

## Template Variables Reference

All templates (epic and node) are rendered with Go template syntax. The following variables are available:

### Core Variables

| Variable | Description | Example Value |
|----------|-------------|---------------|
| `{{.Name}}` | Feature/slug name | `my-feature` |
| `{{.Slug}}` | Same as Name | `my-feature` |
| `{{.Date}}` | Current date (YYYY-MM-DD) | `2026-01-23` |

### Arguments (`{{.Args.key}}`)

User-provided values from the workflow arguments form. Access via `{{.Args.key}}` where `key` matches the argument's `key` field in template.yaml.

```markdown
# Report for {{.Args.app_name}}
Environment: {{.Args.env}}
```

### Inputs (`{{.Inputs.key}}`)

Full paths to input artifacts. The `key` matches the artifact's `key` field in template.yaml. Resolves to the complete path including `path` prefix.

```markdown
## Inputs
- Read findings from: `{{.Inputs.findings}}`
- Read recon from: `{{.Inputs.recon}}`
```

### Outputs (`{{.Outputs.key}}`)

Full paths to output artifacts. Same pattern as inputs.

```markdown
## Output
Create the report at `{{.Outputs.report}}`
```

### Iterating Over Artifacts

Use Go template range to iterate:

```markdown
## Inputs
{{- if .Inputs}}
{{- range $key, $path := .Inputs}}
- `{{$path}}`
{{- end}}
{{- end}}
```

---

## Getting Started

**First, ask the user about their workflow:**

```
I'll help you create a new workflow template. Please tell me:

1. **Workflow name** - Short identifier (e.g., "code-review", "bug-fix", "feature-planning")
2. **Description** - What does this workflow accomplish?
3. **Worker count** - How many parallel workers (1-4 recommended)?
4. **Phases** - What are the high-level phases? (e.g., "research → implement → review")
5. **Key tasks** - What specific tasks should workers perform?
6. **Arguments** - What user inputs are needed? (e.g., app name, environment, credentials)

Example response:
> Name: security-assessment
> Description: Comprehensive web application penetration test
> Workers: 4 (recon, vuln-tester, validator, reporter)
> Phases: recon → vuln-test → validate → report
> Arguments: app_name (required), base_urls, credentials, environment (select)
```

---

## Workflow Phases

### Phase 1: Requirements Gathering

**Goal:** Understand the workflow structure completely before creating files.

**Gather:**

1. **Workflow identity:**
   - `key`: lowercase-hyphenated identifier (e.g., `security-assessment`)
   - `name`: Human-readable name (e.g., "Web Application Security Assessment")
   - `description`: Clear description for AI agents

2. **Coordinator vs Workers:**

   **IMPORTANT:** The **Coordinator** is NOT a worker. The coordinator is the orchestrating agent that:
   - Receives pre-filled context via arguments (no interactive setup needed)
   - Assigns tasks to workers
   - Monitors progress and handles transitions between phases
   - Does NOT get assigned tasks in template.yaml

   **Workers** are the agents that execute tasks:
   | Worker | Role Name | Responsibilities |
   |--------|-----------|------------------|
   | worker-1 | Role 1 | First specialist role |
   | worker-2 | Role 2 | Second specialist role |
   | worker-3 | Role 3 | Third specialist role |
   | worker-4 | Role 4 | Fourth specialist role |

3. **Arguments (User Inputs):**
   - What information does the user need to provide?
   - What type of input? (text, textarea, number, select, multi-select)
   - Which are required vs optional?
   - What are the options for select/multi-select?

4. **Task DAG:**
   - Which tasks can run in parallel?
   - Which tasks have dependencies (`after`)?
   - Which tasks write to shared files (must be sequential)?
   - Are there human checkpoints needed?

5. **Artifacts:**
   - What files are created/modified?
   - What are the inputs/outputs for each task?
   - Do filenames need to be dynamic? (use `{{.Date}}`, `{{.Args.key}}`)

**Present plan to user for approval before proceeding.**

---

### Phase 2: Create Workflow Directory

```bash
mkdir -p ~/.perles/workflows/{workflow-name}
```

---

### Phase 3: Create template.yaml

Create the registry definition file with the workflow structure.

**File:** `~/.perles/workflows/{workflow-name}/template.yaml`

**Template Structure:**

```yaml
registry:
  - namespace: "workflow"
    key: "{workflow-key}"
    version: "v1"
    name: "{Workflow Name}"
    description: "{Description for AI agents}"
    template: "v1-{key}-epic.md"
    instructions: "v1-epic-instructions.md"
    path: "{artifact-directory}"        # e.g., "reports/security" or ".spec"
    labels:
      - "category:{category}"           # planning, work, review, meta, security

    # ═══════════════════════════════════════════════════════════════════════
    # ARGUMENTS: User-configurable parameters
    # ═══════════════════════════════════════════════════════════════════════
    # Arguments are collected from the user before workflow execution.
    # Access in templates via {{.Args.key}}
    #
    # Types:
    #   - text:         Single-line text input
    #   - textarea:     Multi-line text input
    #   - number:       Numeric input
    #   - select:       Single-select dropdown (requires options)
    #   - multi-select: Multi-select list (requires options)
    # ═══════════════════════════════════════════════════════════════════════
    arguments:
      # Required text input
      - key: "app_name"
        label: "Application Name"
        type: "text"
        required: true

      # Optional text input with description
      - key: "base_urls"
        label: "Base URLs"
        description: "Comma-separated list of URLs to test"
        type: "text"
        required: false

      # Textarea for longer input
      - key: "context"
        label: "Additional Context"
        description: "Provide any background information"
        type: "textarea"
        required: false

      # Select dropdown (single choice)
      - key: "env"
        label: "Environment"
        type: "select"
        options:
          - "production"
          - "staging"
          - "development"
        required: true

      # Multi-select (multiple choices)
      - key: "browsers"
        label: "Browsers to Test"
        type: "multi-select"
        options:
          - "Chrome"
          - "Firefox"
          - "Safari"
        required: true

    # ═══════════════════════════════════════════════════════════════════════
    # NODES: Workflow tasks (DAG structure)
    # ═══════════════════════════════════════════════════════════════════════
    # Context is collected upfront via arguments - no Phase 0 needed.
    # The coordinator starts executing Phase 1 immediately.
    # Only create nodes for work that workers will execute.
    # ═══════════════════════════════════════════════════════════════════════
    nodes:
      # Phase 1: First worker task with dynamic output filename
      - key: "recon"
        name: "Reconnaissance"
        template: "v1-{key}-recon.md"
        assignee: "worker-1"
        outputs:
          # Artifact key + file pattern
          # Key is used in templates: {{.Outputs.recon}}
          # File can use template variables: {{.Date}}, {{.Args.app_name}}
          - key: "recon"
            file: "{{.Date}}-{{.Args.app_name}}-recon.md"

      # Phase 2: Task with input dependency
      - key: "vuln-test"
        name: "Vulnerability Testing"
        template: "v1-{key}-vuln-test.md"
        assignee: "worker-2"
        inputs:
          # Reference artifact from previous task by key
          - key: "recon"
            file: "{{.Date}}-{{.Args.app_name}}-recon.md"
        outputs:
          - key: "findings"
            file: "{{.Date}}-{{.Args.app_name}}-findings.md"
        after:
          - "recon"

      # Phase 3: Task that updates an existing file (no new outputs)
      - key: "validate"
        name: "Validation"
        template: "v1-{key}-validate.md"
        assignee: "worker-3"
        inputs:
          - key: "findings"
            file: "{{.Date}}-{{.Args.app_name}}-findings.md"
        after:
          - "vuln-test"

      # Phase 4: Task with multiple inputs
      - key: "report"
        name: "Final Report"
        template: "v1-{key}-report.md"
        assignee: "worker-4"
        inputs:
          - key: "recon"
            file: "{{.Date}}-{{.Args.app_name}}-recon.md"
          - key: "findings"
            file: "{{.Date}}-{{.Args.app_name}}-findings.md"
        outputs:
          - key: "report"
            file: "{{.Date}}-{{.Args.app_name}}-report.md"
        after:
          - "validate"

      # Human checkpoint (optional)
      - key: "human-review"
        name: "Human Review"
        template: "v1-human-review.md"
        assignee: "human"
        inputs:
          - key: "report"
            file: "{{.Date}}-{{.Args.app_name}}-report.md"
        after:
          - "report"
```

**Key Rules:**

1. **Assignees:** `worker-1` through `worker-99` or `human`
2. **Dependencies:** Use `after` to define DAG edges
3. **Parallel tasks:** Same `after` dependency = can run in parallel
4. **Sequential writes:** Chain `after` when multiple workers write to same file
5. **Human checkpoints:** Use `assignee: "human"` for pause points
6. **Artifact keys:** Must be unique; used as `{{.Inputs.key}}` / `{{.Outputs.key}}`
7. **Dynamic filenames:** Use `{{.Date}}`, `{{.Args.key}}` in artifact `file` field

---

### Phase 4: Create Epic Template

The epic template is the coordinator's main instruction file, rendered with Go template variables.

**File:** `~/.perles/workflows/{workflow-name}/v1-{key}-epic.md`

**Template Structure:**

```markdown
# {Workflow Name}: {{.Name}}

You are the **Coordinator** for a multi-agent {workflow purpose} workflow. Your job is to orchestrate {N} workers through a structured process.

## Context (from arguments)

The user has provided the following information:

- **Goal:** {{.Args.goal}}
- **{Other arg}:** {{.Args.other_arg}}

## Output Files

- `{path}/{{.Date}}-{{.Args.feature}}-{artifact1}.md`
- `{path}/{{.Date}}-{{.Args.feature}}-{artifact2}.md`

## Your Workers

| Worker | Role | Responsibilities | Phase |
|--------|------|------------------|-------|
| worker-1 | {Role 1} | {Responsibilities} | 1 |
| worker-2 | {Role 2} | {Responsibilities} | 2 |
| worker-3 | {Role 3} | {Responsibilities} | 3 |
| worker-4 | {Role 4} | {Responsibilities} | 4 |

**NOTE:** You (the Coordinator) are NOT a worker. Context is pre-filled via arguments - start executing immediately.

---

## Workflow Execution

Work through the tasks in this epic sequentially. Each task has an **assignee** field indicating which worker should execute it.

### Parallel Execution

When multiple tasks have the same dependencies, assign them **in parallel**:
- {List parallel phase tasks}
- Use `assign_task` for each, then monitor all

### Sequential Execution

When tasks have file write conflicts or chain dependencies, execute **one at a time**:
- {List sequential phase tasks}

**CRITICAL**: Never assign multiple workers to write to the same file simultaneously.

## MCP Tool Usage

### Assigning Work
```
assign_task(worker_id="worker-2", task_id="<task-id>", instructions="<from task description>")
```

### Checking Status
```
get_task_status(task_id="<task-id>")
```

### Completing Tasks
```
mark_task_complete(task_id="<task-id>", summary="<what was accomplished>")
```

## Phase-by-Phase Guide

{Describe each phase with:}
- What happens
- Which workers are involved
- Expected outputs
- When to proceed to next phase

## Quality Standards

{Define quality expectations for the workflow outputs}

## Common Pitfalls

1. {Pitfall 1 and how to avoid}
2. {Pitfall 2 and how to avoid}

## Success Criteria

A successful {output} should:
- {Criterion 1}
- {Criterion 2}
- {Criterion 3}
```

**Available Template Variables:**
- `{{.Name}}` - Human-readable feature name
- `{{.Slug}}` - URL-safe name slug
- `{{.Date}}` - Current date (YYYY-MM-DD)
- `{{.Inputs.artifact_key}}` - The key lookup to the input artifact
- `{{.Outputs.artifact_key}}` - The key lookup to the output artifact
- `{{.Args.key}}` - User-provided argument value (where `key` matches an argument's `key` field)

---

### Phase 5: Create Coordinator Instructions

Shared instructions template referenced by all workflow epics.

**File:** `~/.perles/workflows/{workflow-name}/v1-epic-instructions.md`

**Can copy from built-in:** Check if `~/.perles/workflows/v1-epic-instructions.md` exists, otherwise create:

```markdown
# Epic-Driven Workflow

You are the **Coordinator** for a multi-agent workflow. Your instructions are embedded in the **epic** that was created for this workflow.

## How This Works

1. **Read the Epic** - The epic contains your complete instructions, worker assignments, phases, and quality standards
2. **Follow the Phases** - Execute the workflow as described in the epic
3. **Use MCP Tools** - Coordinate workers using the standard orchestration tools

The epic will define specific roles and responsibilities for each worker.

## MCP Tools Available

### Task Management

| Tool | Purpose | Key Behavior |
|------|---------|--------------|
| `assign_task(worker_id, task_id, summary)` | Assign a bd task to a worker | Automatically marks task as `in_progress` in BD |
| `get_task_status(task_id)` | Check task progress | Returns current status and assignee |
| `mark_task_complete(task_id)` | Mark task done | **You must call this** after worker confirms completion |
| `mark_task_failed(task_id, reason)` | Mark task failed | Use when task cannot be completed |

**Important**: `assign_task` only works for bd tasks. For non-bd work, use `send_to_worker` instead.

### Worker Communication

| Tool | Purpose | When to Use |
|------|---------|-------------|
| `spawn_worker(role, instructions)` | Create a new worker | When you need additional workers beyond initial pool |
| `send_to_worker(worker_id, message)` | Send message to worker | For non-bd work, clarifications, or additional context |
| `retire_worker(worker_id, reason)` | Retire a worker | When worker is no longer needed or context is stale |
| `query_worker_state(worker_id, task_id)` | Check worker/task state | Before assignments to verify availability |

**Important**: Always check `query_worker_state()` before assigning tasks to ensure the worker is ready.

### Human Communication

| Tool | Purpose |
|------|---------|
| `notify_user(message)` | Get user's attention for human-assigned tasks |

### Example: Task Completion Flow

```
# 1. Assign task to worker (automatically marks as in_progress)
assign_task(worker_id="worker-1", task_id="proj-abc.1", summary="Implement feature X")

# 2. Worker completes work and signals done (you'll see this in message log)
# Worker calls: report_implementation_complete(summary="Added feature X with tests")

# 3. YOU must mark the task complete - this doesn't happen automatically
mark_task_complete(task_id="proj-abc.1")
```

## Getting Started

**IMPORTANT**: All context is pre-filled via arguments. Start executing immediately.

1. **Read the epic description** - Contains workflow instructions with `{{.Args.key}}` values already filled in
2. **Identify the phases** - Understand what needs to happen and in what order
3. **Note worker assignments** - Each task specifies which worker should execute it
4. **Begin execution immediately** - Context is ready, start Phase 1

## Key Principles

- **Start immediately** - Arguments provide all context; no interactive setup needed
- **Follow epic instructions** - The epic is your source of truth (with arguments rendered)
- **Sequential file writes** - Never assign multiple workers to write the same file simultaneously
- **Wait for completion** - Don't proceed to next phase until current phase completes
- **Use read before write** - Workers must read files before editing them
- **Track progress** - Use task status tools to monitor workflow state

## Human-Assigned Tasks

When a task has `assignee: human` or is assigned to the human role:

1. **Read the task instructions carefully** - The task description contains specific instructions for how to notify and interact with the human
2. **Use `notify_user`** - Follow the notification instructions in the task to alert the user
3. **Wait for response** - Pause workflow execution until the human responds
4. **Do not proceed without human input** - Human tasks are explicit checkpoints requiring user action

## If the Epic is Missing Instructions

If the epic doesn't provide clear instructions for a phase or task:

1. **Ask the user** for clarification before proceeding
2. **Don't assume** - Better to pause and confirm than execute incorrectly
3. **Document gaps** - Note any ambiguities for future workflow improvements

## Completing the Workflow

**CRITICAL**: When all phases are complete, you MUST:

1. **Close all remaining open tasks** in the epic (including any that were skipped):
   ```
mark_task_complete(task_id="epic-id.N")
   ```

2. **Close the epic itself**:
   ```
mark_task_complete(task_id="epic-id")
   ```

3. **Signal workflow completion**:
   ```
signal_workflow_complete(
status="success",
summary="Completed [workflow name]. [Brief description of what was accomplished and key outputs]."
)
   ```

If the workflow fails or cannot continue:

```
signal_workflow_complete(
status="failed",
summary="Failed [workflow name]. Reason: [why it failed and what was attempted]."
)
```

**Do not end the workflow without closing the epic and calling `signal_workflow_complete`** - this is how the system knows the workflow has finished and keeps the tracker clean.

## Success Criteria

A successful workflow completes all phases defined in the epic with:
- All tasks marked complete
- All workers' contributions integrated
- Quality standards from the epic met
- User confirmation of completion (if required)
- `signal_workflow_complete` called with status and summary
```

---

### Phase 6: Create Node Templates

Create a template for each node defined in template.yaml.

**File pattern:** `~/.perles/workflows/{workflow-name}/v1-{key}-{node-key}.md`

**Node Template Structure:**

```markdown
# {Node Name}

## Role: {Worker Role}

You are the {Role} responsible for {responsibility}.

## Objective

{Clear statement of what this task accomplishes}

## Inputs

{{- if .Inputs}}
{{- range $key, $path := .Inputs}}
- `{{$path}}`
{{- end}}
{{- end}}

Read the above file(s) to understand the current state before proceeding.

## Instructions

1. **Step 1** - {Specific action}
   - {Details}
   - {Details}

2. **Step 2** - {Specific action}
   - {Details}

3. **Step 3** - {Specific action}

## Output

Create `{{.Outputs.{key}}}` with the following structure:

```markdown
# {Expected document title}

## Section 1
[Description of what goes here]

## Section 2
[Description of what goes here]
```

## Success Criteria

- [ ] {Criterion 1}
- [ ] {Criterion 2}
- [ ] {Criterion 3}
- [ ] Output file created at specified path
```

**Key Patterns for Node Templates:**

1. **Reference specific artifact by key** (preferred for single artifacts):
   ```markdown
   Read the reconnaissance findings from `{{.Inputs.recon}}`
   Create the final report at `{{.Outputs.report}}`
   ```

2. **Iterate over all inputs** (when multiple inputs):
   ```markdown
   ## Inputs
   {{- if .Inputs}}
   {{- range $key, $path := .Inputs}}
   - `{{$path}}`
   {{- end}}
   {{- end}}
   ```

3. **Access user arguments**:
   ```markdown
   # Security Assessment: {{.Args.app_name}}
   Environment: {{.Args.env}}
   Target URLs: {{.Args.base_urls}}
   ```

4. **Dynamic paths in prose** (for epic templates):
   ```markdown
   Output files will be created at:
   - `reports/appsec/{{.Date}}-{{.Args.app_name}}-recon.md`
   - `reports/appsec/{{.Date}}-{{.Args.app_name}}-findings.md`
   ```

---

### Phase 7: Validation

**Verify the workflow is valid:**

1. **Check file structure:**
   ```bash
   ls -la ~/.perles/workflows/{workflow-name}/
   ```

2. **Validate YAML syntax:**
   ```bash
   cat ~/.perles/workflows/{workflow-name}/template.yaml
   ```

3. **Verify template references:**
   - All `template` fields in YAML reference existing .md files
   - All node keys are unique
   - All `after` references point to valid node keys
   - Assignees are valid (`worker-N` or `human`)

4. **Test loading:**
   ```bash
   perles workflows
   ```
   - New workflow should appear in list with "(user)" suffix

---

## Common Patterns

### Parallel Research Pattern

```yaml
# Multiple workers research simultaneously (no writes)
# Note: These have no "after" dependency - they start immediately
# Context is pre-filled via arguments, so coordinator assigns these right away
- key: "research-a"
  assignee: "worker-1"
  # No "after" - first tasks in the DAG

- key: "research-b"
  assignee: "worker-2"
  # No "after" - runs in parallel with research-a

- key: "research-c"
  assignee: "worker-3"
  # No "after" - runs in parallel with research-a and research-b
```

### Sequential Documentation Pattern

```yaml
# Workers write to same file one at a time
- key: "document-a"
  assignee: "worker-2"
  after: ["research-a"]

- key: "document-b"
  assignee: "worker-3"
  after: ["research-b", "document-a"]  # Wait for previous write

- key: "document-c"
  assignee: "worker-4"
  after: ["research-c", "document-b"]  # Wait for previous write
```

### Human Checkpoint Pattern

```yaml
- key: "human-review"
  name: "Human Review"
  template: "v1-human-review.md"
  assignee: "human"
  after: ["previous-task"]
```

### Cross-Review Pattern

```yaml
# Each worker reviews others' work
- key: "cross-review-a"
  assignee: "worker-3"
  after: ["document-a", "document-b", "document-c"]

- key: "cross-review-b"
  assignee: "worker-4"
  after: ["cross-review-a"]

- key: "cross-review-c"
  assignee: "worker-2"
  after: ["cross-review-b"]
```

---

## Example: Bug Investigation Workflow

**User request:**
> Create a workflow for investigating bugs with parallel hypothesis testing

**Worker roles:**
- Coordinator: Collects bug details from user (Phase 0 - handled in epic template)
- worker-1: Reproducer - attempts to reproduce the bug
- worker-2: Hypothesizer - generates hypotheses about root cause
- worker-3: Fix Proposer - proposes fixes based on findings

**Files created:**

`~/.perles/workflows/bug-investigation/template.yaml`:
```yaml
registry:
  - namespace: "workflow"
    key: "bug-investigation"
    version: "v1"
    name: "Bug Investigation"
    description: "Investigate bugs with parallel hypothesis testing and structured diagnosis"
    template: "v1-bug-investigation-epic.md"
    instructions: "v1-epic-instructions.md"
    labels:
      - "category:work"
      - "workers:3"
    nodes:
      # Phase 0 (setup) is handled by coordinator in epic template
      # Phase 1: Parallel investigation
      - key: "reproduce"
        name: "Reproduce Bug"
        assignee: "worker-1"
        template: "v1-bug-investigation-reproduce.md"
        outputs:
          - "investigation.md"

      - key: "hypothesize"
        name: "Generate Hypotheses"
        assignee: "worker-2"
        template: "v1-bug-investigation-hypothesize.md"
        # Runs in parallel with reproduce (no after dependency)

      # Phase 2: Documentation (sequential to avoid file conflicts)
      - key: "document-reproduction"
        name: "Document Reproduction Steps"
        assignee: "worker-1"
        template: "v1-bug-investigation-document-reproduction.md"
        inputs:
          - "investigation.md"
        after:
          - "reproduce"

      - key: "document-hypotheses"
        name: "Document Hypotheses"
        assignee: "worker-2"
        template: "v1-bug-investigation-document-hypotheses.md"
        inputs:
          - "investigation.md"
        after:
          - "hypothesize"
          - "document-reproduction"  # Sequential write to same file

      # Phase 3: Propose fix
      - key: "propose-fix"
        name: "Propose Fix"
        assignee: "worker-3"
        template: "v1-bug-investigation-propose-fix.md"
        inputs:
          - "investigation.md"
        after:
          - "document-hypotheses"
```

---

## Success Criteria

- [ ] Directory created at `~/.perles/workflows/{name}/`
- [ ] `template.yaml` is valid YAML with correct structure
- [ ] Epic template uses Go template syntax correctly
- [ ] All referenced template files exist
- [ ] Node DAG is valid (no cycles, all `after` refs valid)
- [ ] Labels include `category:` tags

---

## When to Use This Workflow

**Good for:**
- Creating repeatable multi-agent workflows
- Standardizing team processes
- Experimenting with orchestration patterns
