# Workflow Templates

Workflow templates are pre-defined "recipes" for common orchestration patterns. They are DAGs configured via YAML files which are converted into beads epics and tasks.

---

## Built-in Templates

| Template | Description |
|----------|-------------|
| **Cook** | Sequential task execution with code review |
| **Research to Tasks** | Convert an existing research or proposal document into a beads epic and tasks, with multi-perspective review |
| **Technical Debate** | Structured multi-perspective debate (moderator, affirmative, negative, neutral analyst) |
| **Mediated Investigation** | Structured investigation with mediator |
| **Research Proposal** | Collaborative proposal development |
| **Quick Plan** | Rapid planning and task breakdown |

Start a workflow from the dashboard's New Workflow dialog: press `ctrl+o` on the kanban board to open the dashboard, then `n`. The dialog lists the built-in templates, any [community workflows](#community-workflows) you have enabled, and your own [custom workflows](custom-workflows.md) from `~/.perles/workflows/<name>/template.yaml`.

List the same templates from the command line with:

```bash
perles workflows
```

The first section, **Dashboard Workflows**, shows each template's key and name with a `[built-in]`, `[community]`, or `[user]` label. The second section, **Chat Panel Workflows**, lists the separate markdown workflows offered by the chat panel's Workflows tab. For JSON output, use `perles registry:list -n workflow`.

---

## Community Workflows

Community workflows are contributed templates that ship with the perles binary but stay hidden until you opt in. Currently available:

| ID | Description |
|----|-------------|
| `joke-contest` | Two workers write jokes in parallel, a human reviews them, then a third worker judges and picks a winner |

Enable them in your config and restart perles:

```yaml
orchestration:
  community_workflows:
    - "joke-contest"          # or the fully-qualified "workflow/joke-contest"
```

Enabled community workflows appear in the New Workflow dialog and in `perles workflows` with a `[community]` label.

---

## Typical Workflow

A common development cycle uses multiple workflows in sequence:

### 1. Research Proposal

Start by generating a research document or proposal:

- Launch the **Research Proposal** workflow
- Provide a topic or problem description
- The coordinator assigns workers to research and draft a proposal
- Output: a proposal document in your configured `document_path`

### 2. Research to Tasks

Break down the proposal into actionable work:

- Launch **Research to Tasks** with the proposal document
- Workers analyze the proposal and create beads epics and tasks
- Output: a structured epic with prioritized tasks

### 3. Cook

Execute the tasks with code review:

- Launch **Cook** with the epic from step 2
- The coordinator assigns tasks to workers sequentially
- Each task goes through implementation, review, and commit phases
- Workers automatically cycle through phases with built-in code review

---

## Cook Workflow Details

Cook is the primary implementation workflow. It processes an existing epic's tasks in order:

1. **Task Assignment**: Coordinator assigns the next task to an available worker
2. **Implementation**: Worker implements the task (`implementing` phase)
3. **Review**: A different worker reviews the implementation (`reviewing` phase) while the implementer waits (`awaiting_review` phase); a worker can never review its own task
4. **Feedback**: If the review is denied, the implementer addresses the feedback (`addressing_feedback` phase) and the task is reviewed again
5. **Commit**: After approval, the coordinator approves the commit and the implementer commits the changes (`committing` phase)
6. **Worker Cycling**: The coordinator replaces both the implementer and the reviewer with fresh workers
7. **Next Task**: The coordinator moves to the next task; when all tasks are done it closes the epic and signals completion

If a worker runs out of context mid-task, it is marked failed and the coordinator is prompted to replace it and reassign the task. The coordinator itself is replaced automatically when it runs out of context.
