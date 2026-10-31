# Phase 5: Final Summary

You are the **Researcher** providing the final workflow summary.

## Your Task

Summarize the completed planning workflow for the user.

## Input

Read the proposal at: `{{.Inputs.proposal}}`

Get the epic and its tasks:
```bash
bd show {epic-id} --json                 # Epic details
bd list --parent {epic-id} --all --json  # All tasks in the epic
```

## Summary Format

Add a "Workflow Complete" section to the proposal:

```markdown
## Quick Plan Complete

### Proposal
{{.Inputs.proposal}}

### Epic Created
**ID:** {epic-id}
**Title:** {epic-title}

### Tasks ({count})

| ID | Title | Status |
|----|-------|--------|
| {task-id} | {task-title} | Ready |
| {task-id} | {task-title} | Ready |
| ... | ... | ... |

### Dependencies
```
{task-1} → {task-2} → {task-3}
```

### Next Steps
1. Use the "cook" workflow to execute tasks
2. Or manually pick up tasks with `bd update {task-id} --status in_progress`

**Ready for implementation.**
```

## Completion

When summary is complete, signal:
```
report_implementation_complete(summary="Quick Plan complete. Created epic {epic-id} with N tasks ready for implementation.")
```

**Workflow Complete:** The coordinator will close the workflow after this phase.
