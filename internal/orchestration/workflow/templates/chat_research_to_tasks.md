---
name: "Research to Tasks"
description: "Translate research documents into well-planned beads epics and tasks"
category: "Planning"
target_mode: "chat"
---

# Research Document to Beads Tasks Workflow

## Overview

This workflow translates a research document into a well-structured beads epic with tasks. The key focus is ensuring tasks are correctly scoped with tests included (not deferred).

**Critical Philosophy:** Tests are not a separate phase. Every implementation task includes its corresponding tests. "Implement X" means "Implement and test X."

---

## Workflow Phases

### Phase 1: Understand the Research

**Read the research document thoroughly:**
1. Identify the core feature or change being proposed
2. Note key implementation points and dependencies
3. Understand the acceptance criteria
4. Identify potential risks or unknowns

**Summarize your understanding:**
- What is being built?
- What are the key technical decisions?
- What existing patterns should be followed?

---

### Phase 2: Plan the Epic Structure

**Create the plan document:**

Create a plan document at: `docs/plans/YYYY-MM-DD-HHMM-{name}-plan.md`

**Plan Document Structure:**
```markdown
# Task Plan: {Feature Name}

## Source
Research document: {path/to/research-doc.md}

## Research Summary
[2-3 paragraphs summarizing what needs to be built]

## Key Implementation Points
- [Key point from research]
- [Key point from research]

## Test Integration Philosophy
Every task in this plan includes its tests. We do NOT defer testing.
- Implementation + unit tests = one task
- Integration points get tested when implemented
- No separate "write tests" phase at the end

---

## Task Breakdown

### Epic Structure
**Epic ID:** [to be filled after creation]
**Title:** {epic-title}

### Tasks
| ID | Title | Dependencies | Test Coverage |
|----|-------|--------------|---------------|
| [id] | [title] | [deps] | [test summary] |

### Task Details
[Detailed breakdown of each task]

---

## Risk Assessment
- [Potential risks and mitigations]

## Success Criteria
- [ ] All tasks have specific test requirements
- [ ] Dependencies are properly ordered
- [ ] No deferred testing patterns
```

---

### Phase 3: Create the Epic

**Create the beads epic:**
```bash
bd create "{Epic Title}" -t epic -d "
## Overview
[Brief summary from research doc]

## Research Document
See: {path/to/research-doc.md}

## Plan Document
See: {path/to/plan-doc.md}

## Tasks
This epic contains N tasks to implement {feature}.
" --json
```

---

### Phase 4: Create Tasks with Tests

For each logical unit of work, create a task with embedded test requirements:

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

**Critical Rules:**
- Each task MUST include its tests (no "write tests later" tasks)
- Tasks should be small enough to complete in one session
- Set proper dependencies between tasks
- Reference the research doc in the epic description

---

### Phase 5: Set Dependencies

**CRITICAL: Use `bd dep add` for inter-task dependencies**

The `--parent` flag only creates parent-child relationships to the epic, NOT dependencies between tasks.

For each task that depends on another task:
```bash
bd dep add {dependent-task-id} {prerequisite-task-id}
```

**Example for a 4-task chain:**
```bash
# If .4 depends on .1, .2, .3 completing first:
bd dep add {epic}.4 {epic}.1
bd dep add {epic}.4 {epic}.2
bd dep add {epic}.4 {epic}.3

# If .5 depends on .4:
bd dep add {epic}.5 {epic}.4
```

**Verify dependencies were created:**
```bash
bd list --parent {epic-id} --all --json | jq -r '.[] | "\(.id): deps=\([.dependencies[]? | select(.type == "blocks")] | length)"'
```

Starting tasks should show `deps=0`, dependent tasks should show `deps>=1`.

---

### Phase 6: Self-Review

Before completing, review your work against these checklists:

**Implementation Review:**
- [ ] Task scope achievable in single session?
- [ ] Implementation steps clear and actionable?
- [ ] Dependencies properly identified?
- [ ] Follows existing codebase patterns?
- [ ] No overly complex or under-specified tasks?
- [ ] Prerequisites and constraints addressed?

**Test Coverage Review:**
- [ ] Every task includes specific test requirements?
- [ ] Test coverage matches implementation scope?
- [ ] Edge cases identified for complex logic?
- [ ] Integration points have test plans?
- [ ] No "implement now, test later" patterns?
- [ ] Test dependencies properly ordered?

**Red Flags to Fix:**
- Tasks with implementation steps but no test steps
- Vague test requirements like "add tests"
- Tests deferred to later tasks
- Missing error case testing
- No negative test cases

---

### Phase 7: Update Plan Document

After creating all tasks, update the plan document with:

1. **Epic ID** and all task IDs
2. **Dependency verification results** (output from the `bd list --parent {epic-id} --all --json | jq ...` check)
3. **Final task summary table**
4. **Any decisions made during planning**

---

## Common Pitfalls

### Task Planning Pitfalls
1. **Deferred testing** - "Add tests" as a separate task = REJECT
2. **Vague tests** - "Write unit tests" without specifics = REJECT
3. **Oversized tasks** - If it can't be done in one session, split it
4. **Missing dependencies** - Tasks that need prior work but don't declare it
5. **Orphan tests** - Tests without corresponding implementation
6. **Flat task structure** - Using only `--parent` creates parent-child links, NOT inter-task dependencies. You MUST use `bd dep add` for task ordering.

### Process Pitfalls
1. **Skipping self-review** - Always verify your work
2. **Rushing** - Take time to think through dependencies
3. **Ignoring patterns** - Follow existing codebase conventions

---

## Success Criteria

A successful planning session produces:

- [ ] Plan document with research summary
- [ ] Beads epic with clear description
- [ ] Tasks with specific test requirements (not deferred)
- [ ] Proper inter-task dependencies via `bd dep add`
- [ ] Dependency verification passed (starting tasks have deps=0, dependent tasks have deps>=1)

### Quality Checks

**Every task should answer:**
1. What exactly needs to be implemented?
2. What specific tests validate the implementation?
3. What are the acceptance criteria?
4. What depends on this task? What does this task depend on?

---

## Example Session

```
[Phase 1: Understand]
User: "Translate docs/research/clipboard-support.md into tasks"
AI: Reads research doc, summarizes key points

[Phase 2: Plan]
AI: Creates docs/plans/2025-01-15-1430-clipboard-support-plan.md

[Phase 3: Create Epic]
AI: Creates epic perles-xyz

[Phase 4: Create Tasks]
AI: Creates 4 tasks with test requirements:
- perles-xyz.1: Add clipboard package + unit tests
- perles-xyz.2: Add copy keybinding + keyboard tests
- perles-xyz.3: Add paste support + integration tests
- perles-xyz.4: Add visual feedback + UI tests

[Phase 5: Set Dependencies]
AI: Sets dependencies between tasks
- perles-xyz.4 depends on perles-xyz.2
AI: Verifies with the bd list --parent --json | jq dependency check

[Phase 6: Self-Review]
AI: Reviews against checklists, fixes issues:
- Splits xyz.2 into xyz.2a (keybinding) and xyz.2b (handler) - was too large
- Adds error case tests to xyz.1
- Adds empty clipboard edge case to xyz.3

[Phase 7: Update Plan]
AI: Updates plan document with final structure

[Complete]
AI: "Epic perles-xyz created with 5 tasks. See docs/plans/2025-01-15-1430-clipboard-support-plan.md"
```

---

## When to Use This Workflow

**Good for:**
- Translating research/design docs into executable work
- Features requiring careful test planning
- Creating audit trail of planning decisions

**Not necessary for:**
- Simple, well-understood changes
- Urgent hotfixes
- Single-task work items
- Already-planned epics

---

## Related Workflows

- **quick_plan.md** - Lighter weight planning
- **research_proposal.md** - When you need to DO research first
- **cook.md** - For executing the tasks once planned
