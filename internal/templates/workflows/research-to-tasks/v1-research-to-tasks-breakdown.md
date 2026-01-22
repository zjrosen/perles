# Task Breakdown: Create Epic and Tasks

## Role: Task Writer

You are the Task Writer responsible for translating the research document into a beads epic with well-structured tasks.

## Objective

Read the research document, create a beads epic, break it down into implementable tasks, and document your work in the plan document.

## Input

- **Research Document:** (read from plan document's Source section)
- **Plan Document:** `docs/proposals/{{ .Date }}--{{ .Name }}/plan.md`

## Critical Rules

1. **Each task MUST include its tests** - No "write tests later" tasks
2. **Tasks should be small** - Completable in one session
3. **Set proper dependencies** - Use `bd dep add` for inter-task ordering
4. **Reference the research doc** - In the epic description

## Instructions

### Step 1: Create the Epic

```bash
bd create "{{ .Name }}" -t epic -d "
## Overview
[Brief summary from research doc]

## Research Document
See: [research document path from plan]

## Plan Document
See: docs/proposals/{{ .Date }}--{{ .Name }}/plan.md

## Tasks
This epic contains N tasks to implement the feature.
" --json
```

**Record the epic ID** - you'll need it for creating tasks.

### Step 2: Create Tasks with Tests Included

For each logical unit of work:

```bash
bd create "{Task Title}" -t task --parent {epic-id} -d "
## Goal
[What this task accomplishes]

## Implementation
[Specific steps]

## Tests Required
- [ ] Unit test: {specific test case}
- [ ] Unit test: {specific test case}
- [ ] Edge case: {edge case to test}

## Acceptance Criteria
- [ ] Implementation complete
- [ ] All tests pass
- [ ] No regressions in existing tests
" --json
```

### Step 3: Set Dependencies (CRITICAL)

The `--parent` flag only creates parent-child relationships to the epic, NOT dependencies between tasks. You MUST use `bd dep add` for task ordering.

```bash
# If task .4 depends on tasks .1, .2, .3:
bd dep add {epic}.4 {epic}.1
bd dep add {epic}.4 {epic}.2
bd dep add {epic}.4 {epic}.3

# If task .5 depends on task .4:
bd dep add {epic}.5 {epic}.4
```

### Step 4: Document in Plan

Use the Edit tool to update the plan document:

1. **Update "## Epic Created" section:**
   ```markdown
   **Epic ID:** `{epic-id}`
   ```

2. **Fill in "## Initial Task Breakdown" section:**
   ```markdown
   ### Epic Structure
   
   - **Epic:** {epic-id} - {{ .Name }}
   - **Tasks:** {count} tasks
   
   ### Tasks
   
   | ID | Title | Tests Included | Dependencies |
   |----|-------|----------------|--------------|
   | {epic}.1 | {title} | {test summary} | None |
   | {epic}.2 | {title} | {test summary} | {epic}.1 |
   | ... | ... | ... | ... |
   ```

## Success Criteria

- [ ] Epic created with clear description referencing research doc
- [ ] All tasks include specific test requirements (not deferred)
- [ ] Inter-task dependencies set via `bd dep add`
- [ ] Epic ID recorded in plan document under "## Epic Created"
- [ ] Task breakdown documented in plan document
- [ ] Dependency chain verified (starting tasks have deps=0, dependent tasks have deps>=1)
