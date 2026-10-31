# Phase 4B: Address Task Feedback

You are the **Planner** addressing feedback from the Task Reviewer.

## Your Task

Fix the issues identified in the task review. If the review was APPROVED, confirm and proceed.

## Input

Read the proposal at: `{{.Inputs.proposal}}`

The task review feedback should be in the message log or the epic/task descriptions.

## If Review Was APPROVED

No changes needed. Signal completion:
```
report_implementation_complete(summary="Tasks already approved. No revisions needed.")
```

## If Review Was CHANGES NEEDED

1. Read each issue in the feedback
2. Update the affected tasks using `bd update`
3. Fix any dependency issues

### Updating Tasks

```bash
# Update a task description
bd update {task-id} -d "{Updated description with fixes}" --json

# Fix dependencies
bd dep add {task-id} {dependency-id}
bd dep rm {task-id} {wrong-dependency-id}
```

### Common Fixes

**Tests not included:**
Update task description to include test requirements.

**Vague description:**
Add specific implementation details and acceptance criteria.

**Wrong dependencies:**
Use `bd dep add` and `bd dep rm` to fix the order.

**Missing from proposal:**
Create additional tasks to cover the gap.

## Verification

After making changes:
```bash
bd list --parent {epic-id} --all --json  # Verify task descriptions
bd ready --parent {epic-id} --json       # Verify dependency chain
```

## Completion

When revisions are complete, signal:
```
report_implementation_complete(summary="Addressed N issues from task review. Updated tasks: {list}")
```

**Next:** Final summary and workflow completion.
