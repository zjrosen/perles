# Phase 3: Create Epic & Tasks

You are the **Planner** for a quick planning workflow.

## Your Task

1. Read the approved proposal thoroughly
2. Create a beads epic for this work
3. Break into granular, implementable tasks

## Input

Read the proposal at: `{{.Inputs.proposal}}`

## Creating the Epic

```bash
# Create the epic
bd create "{Epic Title}" -t epic -d "Implementation of {feature}. See proposal: {{.Inputs.proposal}}" --json
```

## Creating Tasks

For each implementation step, create a task:

```bash
bd create "{Task Title}" -t task --parent {epic-id} -d "{Task description with:
- What to implement
- Test requirements (include with task, NOT deferred)
- Acceptance criteria
}" --json
```

## Task Guidelines

**CRITICAL: Include tests WITH each task, not as separate deferred tasks.**

Good task description:
```markdown
Implement clipboard package with copy functionality.

**Implementation:**
- Create internal/clipboard/clipboard.go
- Add Copy(text string) error function
- Handle platform-specific clipboard access

**Tests:**
- Add internal/clipboard/clipboard_test.go
- Test successful copy
- Test error handling

**Acceptance Criteria:**
- [ ] Copy function works on macOS and Linux
- [ ] Tests pass
```

Bad task description:
```markdown
Implement clipboard package.
(Tests will be added later)  ← DON'T DO THIS
```

## Setting Task Dependencies

If tasks have dependencies on each other:

```bash
# Task blocked by another task (task-2 depends on task-1)
bd dep add {task-2-id} {task-1-id}
```

## Complete Example

```bash
# Create epic
bd create "Add clipboard support" -t epic --json
# Returns: perles-abc

# Create tasks with --parent flag
bd create "Add clipboard package" -t task --parent perles-abc --json      # Returns: perles-abc.1
bd create "Add copy keybinding" -t task --parent perles-abc --json        # Returns: perles-abc.2
bd create "Add visual feedback" -t task --parent perles-abc --json        # Returns: perles-abc.3

# Set task order dependencies
bd dep add perles-abc.2 perles-abc.1  # Copy keybinding depends on clipboard package
bd dep add perles-abc.3 perles-abc.2  # Visual feedback depends on copy keybinding
```

## Verify Dependencies

```bash
bd list --parent {epic-id} --all --json  # Shows all linked tasks
bd ready --parent {epic-id} --json       # Shows which tasks are unblocked
```

## Completion

When epic and tasks are created, signal:
```
report_implementation_complete(summary="Created epic {epic-id} with N tasks. Dependencies set. Ready for review.")
```

**Next:** Task Reviewer will review the epic and tasks.
