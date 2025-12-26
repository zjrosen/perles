---
name: "Quick Plan"
description: "Streamlined research and planning cycle with review gates - research, review, task breakdown, and approval"
category: "Planning"
---

# Quick Plan Workflow

## Overview

A lightweight 4-worker planning cycle that produces a researched proposal with actionable beads tasks. Unlike the full research_proposal workflow, this uses single workers per phase with review gates.

**Flow:**
```
Worker 1 (Researcher)     → Research & write proposal
Worker 2 (Proposal Reviewer) → Review proposal, request fixes until approved
Worker 3 (Planner)        → Break proposal into beads epics/tasks
Worker 4 (Task Reviewer)  → Review tasks, request fixes until approved
```

**Output:**
- `docs/proposals/YYYY-MM-DD-HHMM-{name}.md` - Research and implementation proposal
- Beads epic with tasks (tests included with implementation, not deferred)

---

## Roles

### Worker 1: Researcher
**Goal:** Deep research and write a complete proposal document.

**Responsibilities:**
- Understand the user's goal/problem
- Research existing codebase patterns, files, and constraints
- Write comprehensive proposal with implementation plan
- Save to `docs/proposals/`

**Output:** Complete proposal document with:
- Problem statement
- Research findings (file paths, patterns, constraints)
- Implementation approach
- Files to create/modify
- Testing strategy

---

### Worker 2: Proposal Reviewer
**Goal:** Ensure proposal is complete, accurate, and actionable.

**Responsibilities:**
- Read the proposal thoroughly
- Verify research citations are accurate (file paths exist, patterns are real)
- Check implementation plan is feasible
- Identify gaps, unclear areas, or missing considerations
- Provide structured feedback or approve

**Verdict:** `APPROVED` or `CHANGES NEEDED` with specific feedback

---

### Worker 3: Planner
**Goal:** Break the approved proposal into beads epics and tasks.

**Responsibilities:**
- Read the approved proposal
- Create epic for the body of work
- Break into granular, implementable tasks
- **Include tests WITH each task, not deferred to later**
- Set proper dependencies between tasks
- Reference proposal in epic description

**Output:** Beads epic with tasks, each task includes:
- Clear implementation instructions
- Corresponding test requirements
- Acceptance criteria

---

### Worker 4: Task Reviewer
**Goal:** Ensure tasks are well-structured and implementable.

**Responsibilities:**
- Review epic and all tasks
- Verify each task has clear scope and acceptance criteria
- Ensure tests are included (not deferred)
- Check dependencies make sense
- Verify tasks align with proposal
- Provide feedback or approve

**Verdict:** `APPROVED` or `CHANGES NEEDED` with specific feedback

---

## Workflow Phases

### Phase 1: Research & Proposal (Worker 1)

**Coordinator assigns Worker 1 with prompt:**
```
You are the **Researcher** for a quick planning workflow.

**User's Goal:**
[Paste user's request/problem here]

**Your Task:**
1. Research the codebase to understand:
   - Existing patterns and conventions
   - Files/components that will be affected
   - Technical constraints and dependencies
   - Similar implementations to learn from

2. Write a complete proposal to: `docs/proposals/YYYY-MM-DD-HHMM-{descriptive-name}.md`

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

Begin research now.
```

**Coordinator:** Wait for Worker 1 completion.

---

### Phase 2: Proposal Review (Worker 2)

**Coordinator assigns Worker 2 with prompt:**
```
You are the **Proposal Reviewer** for a quick planning workflow.

**Proposal to Review:** `docs/proposals/{filename}.md`

**Your Task:**
1. Read the proposal thoroughly
2. Verify research is accurate:
   - Do cited file paths exist?
   - Are pattern observations correct?
   - Is the approach feasible?
3. Check for gaps:
   - Missing considerations?
   - Unclear implementation steps?
   - Incomplete testing strategy?

**Provide your verdict:**

If proposal is solid:
```
## APPROVED

**Proposal:** {filename}

### Review Summary
- Research accuracy: Pass
- Implementation feasibility: Pass
- Testing coverage: Pass
- Gaps: None identified

**Ready for task breakdown.**
```

If changes needed:
```
## CHANGES NEEDED

**Proposal:** {filename}

### Issues Found

1. **{Category}** - {location}
   - Problem: {description}
   - Suggestion: {how to fix}

### Required Changes
1. {specific change}
2. {specific change}

**Address the issues and resubmit.**
```

Begin review now.
```

**If CHANGES NEEDED:**
1. Send feedback to Worker 1
2. Worker 1 fixes proposal
3. Worker 2 re-reviews
4. Repeat until APPROVED

**Coordinator:** Only proceed to Phase 3 after APPROVED.

---

### Phase 3: Task Breakdown (Worker 3)

**Coordinator assigns Worker 3 with prompt:**
```
You are the **Planner** for a quick planning workflow.

**Approved Proposal:** `docs/proposals/{filename}.md`

**Your Task:**
1. Read the approved proposal thoroughly
2. Create a beads epic for this work
3. Break into granular tasks

**Critical Rules:**
- Each task MUST include its tests (no "write tests later" tasks)
- Tasks should be small enough to complete in one session
- Reference the proposal in the epic description
- Set proper dependencies between tasks

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
# Task blocked by another task (task-2 depends on task-1 completing first)
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

**Note:** Using `--parent` creates the correct parent-child relationship where the epic shows tasks in its `dependents` array. Do NOT use `bd dep add {epic-id} {task-id}` as this creates the wrong "blocks" relationship.

**Verify dependencies:**
```bash
bd show {epic-id} --json  # Shows all linked tasks
bd ready --json           # Shows which tasks are unblocked
```

**Task Guidelines:**
- Include test requirements IN the task, not as separate tasks
- Use proposal's implementation steps as guide
- Make tasks independently verifiable
- **Always set dependencies** - epic blocked by tasks, tasks ordered by logical sequence
- Run `bd ready` to verify dependency chain is correct

Begin task breakdown now.
```

**Coordinator:** Wait for Worker 3 completion.

---

### Phase 4: Task Review (Worker 4)

**Coordinator assigns Worker 4 with prompt:**
```
You are the **Task Reviewer** for a quick planning workflow.

**Epic to Review:** {epic-id}
**Proposal Reference:** `docs/proposals/{filename}.md`

**Your Task:**
1. Read the proposal to understand the full scope
2. List all tasks: `bd show {epic-id} --json`
3. Review each task for:
   - Clear scope and instructions
   - Tests included (not deferred)
   - Proper acceptance criteria
   - Alignment with proposal
4. Check dependencies make sense

**Provide your verdict:**

If tasks are well-structured:
```
## APPROVED

**Epic:** {epic-id}
**Tasks:** {count} tasks

### Review Summary
- Task clarity: Pass
- Tests included: Pass
- Dependencies: Correct
- Proposal alignment: Pass

**Ready for implementation.**
```

If changes needed:
```
## CHANGES NEEDED

**Epic:** {epic-id}

### Issues Found

1. **Task {task-id}**
   - Problem: {description}
   - Suggestion: {how to fix}

### Required Changes
1. {specific change}
2. {specific change}

**Address the issues and resubmit.**
```

Begin review now.
```

**If CHANGES NEEDED:**
1. Send feedback to Worker 3
2. Worker 3 fixes tasks
3. Worker 4 re-reviews
4. Repeat until APPROVED

---

### Phase 5: Completion (Coordinator)

Once Worker 4 approves:

1. **Summarize to user:**
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

## Coordinator Instructions

### Setup
```
1. Spawn 4 workers (researcher, proposal-reviewer, planner, task-reviewer)
2. Get user's goal/problem statement
3. Track worker IDs for each role
```

### Execution
```
Phase 1: Assign Worker 1 → wait for completion
Phase 2: Assign Worker 2 → wait for verdict
         If CHANGES NEEDED: loop Worker 1 fixes → Worker 2 re-reviews
         Until APPROVED
Phase 3: Assign Worker 3 → wait for completion
Phase 4: Assign Worker 4 → wait for verdict
         If CHANGES NEEDED: loop Worker 3 fixes → Worker 4 re-reviews
         Until APPROVED
Phase 5: Report summary to user
```

### Review Loop Pattern
```
When reviewer returns CHANGES NEEDED:
1. Extract specific feedback from reviewer's response
2. Send to original worker with fix instructions
3. Original worker makes fixes
4. Same reviewer re-reviews
5. Repeat until APPROVED

Example:
Coordinator → Worker 3: "Task reviewer found issues: {feedback}. Please fix and confirm when ready."
Worker 3: Makes fixes, confirms ready
Coordinator → Worker 4: "Tasks have been updated. Please re-review."
```

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

## Common Pitfalls

1. **Don't skip proposal review** - Catches issues before task breakdown
2. **Don't defer tests** - Each task must include its tests
3. **Don't rush review loops** - Let workers properly fix issues
4. **Don't create vague tasks** - Each task should be independently executable
5. **Don't forget proposal reference** - Epic should link to proposal doc

---

## Example Session

```
[User Request]
User: "Add keyboard shortcut to copy issue ID to clipboard"

[Phase 1: Research]
Coordinator: Assigns Worker 1 with user goal
Worker 1: Researches keybinding patterns, clipboard handling
Worker 1: Creates docs/proposals/2025-01-15-1430-copy-issue-id.md

[Phase 2: Proposal Review]
Coordinator: Assigns Worker 2 to review proposal
Worker 2: Reviews → CHANGES NEEDED (missing edge case for empty selection)
Coordinator: Sends feedback to Worker 1
Worker 1: Updates proposal with edge case handling
Worker 2: Re-reviews → APPROVED

[Phase 3: Task Breakdown]
Coordinator: Assigns Worker 3 with approved proposal
Worker 3: Creates epic perles-abc, tasks perles-abc.1, perles-abc.2

[Phase 4: Task Review]
Coordinator: Assigns Worker 4 to review tasks
Worker 4: Reviews → APPROVED

[Phase 5: Completion]
Coordinator: "Quick Plan Complete. Epic perles-abc ready for implementation."
```

---

## Success Metrics

- ✅ Proposal has concrete file paths and patterns cited
- ✅ Review loops catch real issues (not rubber-stamped)
- ✅ Each task includes test requirements
- ✅ Tasks are small enough for single-session completion
- ✅ Dependencies between tasks are logical
- ✅ Epic links to proposal document
