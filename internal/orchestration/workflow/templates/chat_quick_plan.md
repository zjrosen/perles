---
name: "Quick Plan"
description: "Streamlined research and planning cycle with self-review gates - research, review, task breakdown, and approval"
category: "Planning"
target_mode: "chat"
---

# Quick Plan Workflow (Chat)

## Overview

A lightweight planning workflow that produces a researched proposal with actionable beads tasks. This chat version combines research, review, planning, and approval into a single-agent flow with self-review gates.

**Flow:**
```
Phase 1: Research & write proposal
Phase 2: Self-review proposal, fix issues
Phase 3: Break proposal into beads epics/tasks
Phase 4: Self-review tasks, fix issues
Phase 5: Report summary
```

**Output:**
- `docs/proposals/YYYY-MM-DD-HHMM-{name}.md` - Research and implementation proposal
- Beads epic with tasks (tests included with implementation, not deferred)

---

## Workflow Phases

### Phase 1: Research & Proposal

**Goal:** Deep research and write a complete proposal document.

**Steps:**
1. Understand the user's goal/problem
2. Research the codebase to understand:
   - Existing patterns and conventions
   - Files/components that will be affected
   - Technical constraints and dependencies
   - Similar implementations to learn from
3. Write comprehensive proposal with implementation plan
4. Save to `docs/proposals/YYYY-MM-DD-HHMM-{descriptive-name}.md`

**Proposal Structure:**
```markdown
# Proposal: {Feature/Change Name}

## Problem Statement
[What needs to be built and why - 2-3 paragraphs]

## Research Findings

### Existing Patterns
[What patterns exist in the codebase that apply here]
[Include specific file paths and line numbers]

### Files to Modify/Create
- `path/to/file.go` - [what changes]
- `path/to/new_file.go` - [what it does]

### Technical Constraints
[Dependencies, limitations, considerations]

## Implementation Plan

### Approach
[2-3 paragraphs explaining the implementation strategy]

### Steps
1. [Step with rationale]
2. [Step with rationale]
3. [Step with rationale]

### Testing Strategy
[How this will be tested - unit tests, integration tests, etc.]
[Which test files need creation/modification]

## Risks and Mitigations
- **Risk:** [identified risk]
  - **Mitigation:** [how to address it]

## Acceptance Criteria
- [ ] [Testable criterion]
- [ ] [Testable criterion]
```

**Requirements:**
- Use Grep/Glob/Read to explore codebase thoroughly
- Cite specific file paths and line numbers
- Make the proposal actionable (ready for task breakdown)

---

### Phase 2: Proposal Self-Review

Before proceeding to task breakdown, review your proposal against these checklists:

**Research Accuracy:**
- [ ] Do all cited file paths exist?
- [ ] Are pattern observations accurate?
- [ ] Are code examples verified against actual code?
- [ ] Are version/dependency assumptions correct?

**Implementation Feasibility:**
- [ ] Is the approach technically sound?
- [ ] Are implementation steps achievable?
- [ ] Are dependencies correctly identified?
- [ ] Are edge cases considered?

**Testing Coverage:**
- [ ] Is the testing strategy complete?
- [ ] Are test file locations identified?
- [ ] Are integration points covered?
- [ ] Are error cases addressed?

**Completeness:**
- [ ] Problem statement is clear?
- [ ] All affected files listed?
- [ ] Risks and mitigations identified?
- [ ] Acceptance criteria are testable?

**Red Flags to Fix:**
- File paths that don't exist
- Patterns described but not verified
- Vague implementation steps
- Missing testing strategy
- Unaddressed edge cases

**If issues found:** Fix them in the proposal before proceeding.

---

### Phase 3: Task Breakdown

**Goal:** Break the proposal into beads epic and tasks.

**Create Epic:**
```bash
bd create "{Epic Title}" -t epic -d "
## Overview
[Brief summary]

## Proposal
See: docs/proposals/{filename}.md

## Tasks
This epic contains N tasks to implement {feature}.
" --json
```

**Create Tasks:**
For each task, use the `--parent` flag to create proper parent-child relationship:
```bash
bd create "{Task Title}" -t task --parent {epic-id} -d "
## Goal
[What this task accomplishes]

## Implementation
[Specific steps from the proposal]

## Tests Required
- [ ] {Test case 1}
- [ ] {Test case 2}

## Acceptance Criteria
- [ ] {Criterion}
- [ ] All tests pass
" --json
```

**Setting Task Dependencies:**
If tasks have dependencies on each other (task-2 depends on task-1), use `bd dep add`:
```bash
bd dep add {task-2-id} {task-1-id}
```

**Complete Example:**
```bash
# Create epic
bd create "Add clipboard support" -t epic --json
# Returns: perles-abc

# Create tasks with --parent flag (creates parent-child relationship)
bd create "Add clipboard package" -t task --parent perles-abc --json      # Returns: perles-abc.1
bd create "Add copy keybinding" -t task --parent perles-abc --json        # Returns: perles-abc.2
bd create "Add visual feedback" -t task --parent perles-abc --json        # Returns: perles-abc.3

# Set task order dependencies (if task-2 depends on task-1)
bd dep add perles-abc.2 perles-abc.1  # Copy keybinding depends on clipboard package
bd dep add perles-abc.3 perles-abc.2  # Visual feedback depends on copy keybinding
```

**Verify dependencies:**
```bash
bd show {epic-id} --json  # Shows all linked tasks
bd ready --json           # Shows which tasks are unblocked
```

**Critical Rules:**
- Each task MUST include its tests (no "write tests later" tasks)
- Tasks should be small enough to complete in one session
- Reference the proposal in the epic description
- Set proper dependencies between tasks

---

### Phase 4: Task Self-Review

Before completing, review your tasks against these checklists:

**Task Clarity:**
- [ ] Each task has clear, specific scope?
- [ ] Implementation steps are actionable?
- [ ] No vague or ambiguous instructions?
- [ ] Tasks achievable in single session?

**Test Integration:**
- [ ] Every task includes specific test requirements?
- [ ] No "write tests later" patterns?
- [ ] Test cases are specific (not "add unit tests")?
- [ ] Edge cases and error cases covered?

**Dependencies:**
- [ ] Dependencies between tasks are logical?
- [ ] No circular dependencies?
- [ ] `bd ready` shows correct starting tasks?
- [ ] Dependency chain makes sense?

**Proposal Alignment:**
- [ ] Tasks cover all proposal implementation steps?
- [ ] Task scope matches proposal complexity?
- [ ] No missing functionality?
- [ ] Acceptance criteria traceable to proposal?

**Red Flags to Fix:**
- Tasks with implementation but no test requirements
- Vague test requirements like "add tests"
- Tests deferred to separate tasks
- Missing error case testing
- Oversized tasks that can't complete in one session
- Tasks that don't align with proposal

**If issues found:** Fix them before proceeding.

---

### Phase 5: Completion

**Report summary to user:**
```
## Quick Plan Complete

**Proposal:** docs/proposals/{filename}.md

**Epic Created:** {epic-id} - {epic-title}

**Tasks ({count}):**
1. {task-id}: {task-title}
2. {task-id}: {task-title}
...

**Ready for implementation.** Use `/cook` or `/pickup` to start executing tasks.
```

---

## Common Pitfalls

1. **Skipping research** - Don't write proposals without exploring the codebase
2. **Skipping self-review** - Always verify your work before proceeding
3. **Deferred tests** - Each task must include its tests
4. **Vague tasks** - Each task should be independently executable
5. **Missing proposal reference** - Epic should link to proposal doc
6. **Flat task structure** - Use `bd dep add` for inter-task dependencies, not just `--parent`

---

## When to Use This Workflow

**Good for:**
- Features with clear scope
- When you want tasks ready to execute immediately
- Medium-complexity work (1-5 tasks)
- When parallel research isn't needed

**Use research_proposal.md instead for:**
- Complex architectural decisions
- When you need multiple perspectives
- Large features requiring extensive research
- When trade-off analysis is important

---

## Example Session

```
[Phase 1: Research]
User: "Add keyboard shortcut to copy issue ID to clipboard"
AI: Researches keybinding patterns, clipboard handling
AI: Creates docs/proposals/2025-01-15-1430-copy-issue-id.md

[Phase 2: Proposal Self-Review]
AI: Reviews against checklist
AI: Finds missing edge case for empty selection
AI: Updates proposal with edge case handling

[Phase 3: Task Breakdown]
AI: Creates epic perles-abc
AI: Creates tasks perles-abc.1, perles-abc.2

[Phase 4: Task Self-Review]
AI: Reviews against checklist
AI: Finds task 2 missing error handling tests
AI: Updates task with error case test requirements

[Phase 5: Completion]
AI: "Quick Plan Complete. Epic perles-abc ready for implementation."
```

---

## Success Metrics

- Proposal has concrete file paths and patterns cited
- Self-reviews catch real issues (not rubber-stamped)
- Each task includes test requirements
- Tasks are small enough for single-session completion
- Dependencies between tasks are logical
- Epic links to proposal document

---

## Related Workflows

- **research_to_tasks.md** - When you already have a research document
- **research_proposal.md** - When you need extensive multi-perspective research
- **cook.md** - For executing the tasks once planned
