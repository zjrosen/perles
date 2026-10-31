---
name: "Research to Tasks"
description: "Translate research documents into well-planned beads epics and tasks with multi-perspective review"
category: "Planning"
target_mode: "orchestration"
---

# Research Document to Beads Tasks Workflow

## Overview

This workflow translates a research document into a well-structured beads epic with tasks, ensuring proper planning through collaborative review. The key focus is ensuring tasks are correctly scoped with tests included (not deferred).

**Critical Philosophy:** Tests are not a separate phase. Every implementation task includes its corresponding tests. "Implement X" means "Implement and test X."

## Worker Roles

### Worker 1: Task Writer
**Specialization:** Epic and task creation

**Responsibilities:**
- Read and understand the research document
- Create beads epic with clear description
- Break down into granular, implementable tasks
- Include test requirements IN each task (not deferred)
- Set proper task dependencies
- Incorporate reviewer feedback

---

### Worker 2: Implementation Reviewer
**Specialization:** Technical correctness and implementation feasibility

**Responsibilities:**
- Verify task scope is achievable in a single session
- Check implementation steps are clear and actionable
- Identify missing dependencies or prerequisites
- Ensure tasks follow codebase patterns
- Flag overly complex or under-specified tasks

---

### Worker 3: Test Reviewer
**Specialization:** Test coverage and quality assurance

**Responsibilities:**
- Verify every task includes appropriate test requirements
- Check test coverage matches implementation scope
- Identify missing edge cases or test scenarios
- Ensure test dependencies are properly ordered
- Flag any "implement now, test later" patterns

---

### Worker 4: Mediator
**Specialization:** Synthesis and plan document management

**Responsibilities:**
- Create and maintain the shared plan document
- Frame the initial structure based on research doc
- Orchestrate sequential contributions from all workers
- Synthesize feedback into actionable revisions
- Write final summary and approval status
- Ensure all file operations are sequential

---

## Output Files

**Plan Document:** `docs/plans/YYYY-MM-DD-HHMM-{research-doc-name}-plan.md`

This document captures the full planning process:
- Research summary
- Initial task breakdown
- Reviewer feedback
- Revisions made
- Final approval status

**Beads Artifacts:**
- Epic created via `bd create`
- Tasks created with `--parent` flag
- Dependencies set via `bd dep add`

---

## Workflow Phases

### Phase 1: Setup (Mediator)

**Action:** Create plan document with structure from research doc

**Coordinator assigns Worker 4 (Mediator) with prompt:**
```
You are the **Mediator** for a research-to-tasks planning workflow.

**Research Document:** {path/to/research-doc.md}

**Your Task:**
1. Read the research document thoroughly
2. Create plan document at: `docs/plans/YYYY-MM-DD-HHMM-{name}-plan.md`

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

## Initial Task Breakdown

### Epic Structure
[To be filled by Task Writer]

### Tasks
[To be filled by Task Writer]

---

## Implementation Review (Worker 2)
[To be filled by Implementation Reviewer]

---

## Test Review (Worker 3)
[To be filled by Test Reviewer]

---

## Revisions
[To be filled by Task Writer after reviews]

---

## Final Approval

### Implementation Reviewer
**Status:** Pending
**Comments:**

### Test Reviewer
**Status:** Pending
**Comments:**

---

## Summary
[To be filled by Mediator after all approvals]
```

Create this document now. The Task Writer will fill in the task breakdown next.
```

**Coordinator:** Wait for completion, then proceed to Phase 2.

---

### Phase 2: Initial Task Breakdown (Task Writer)

**Action:** Writer reads research doc and creates beads epic/tasks

**Coordinator assigns Worker 1 (Task Writer) with prompt:**
```
You are the **Task Writer** for a research-to-tasks planning workflow.

**Research Document:** {path/to/research-doc.md}
**Plan Document:** {path/to/plan-doc.md}

**Your Task:**
1. Read the research document thoroughly
2. Read the plan document to understand the structure
3. Create beads epic and tasks
4. Document your work in the plan document

**Critical Rules:**
- Each task MUST include its tests (no "write tests later" tasks)
- Tasks should be small enough to complete in one session
- Set proper dependencies between tasks
- Reference the research doc in the epic description

**Step 1: Create Epic**
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

**Step 2: Create Tasks with Tests Included**
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

**Step 3: Set Dependencies (CRITICAL - Do Not Skip)**

Inter-task dependencies MUST be created using `bd dep add`. The `--parent` flag only creates parent-child relationships to the epic, NOT dependencies between tasks.

For each task that depends on another task:
```bash
bd dep add {dependent-task-id} {prerequisite-task-id}
```

Example for a 4-task milestone chain:
```bash
# If .4 depends on .1, .2, .3 completing first:
bd dep add {epic}.4 {epic}.1
bd dep add {epic}.4 {epic}.2
bd dep add {epic}.4 {epic}.3

# If .5 depends on .4:
bd dep add {epic}.5 {epic}.4
```

**After adding all dependencies, verify with:**
```bash
bd list --parent {epic-id} --all --json | jq -r '.[] | "\(.id): \(.status), deps: \([.dependencies[]? | select(.type == "blocks")] | length)"'
```
This lists all tasks under the epic with their blocking dependency count. Starting tasks should have 0 blocking deps, dependent tasks should have 1+. If ALL tasks show 0 blocking deps, inter-task dependencies were not set correctly.

**Step 4: Document in Plan**
After creating epic and tasks, use Edit tool to update the plan document under "## Initial Task Breakdown":

```markdown
### Epic Structure

**Epic ID:** {epic-id}
**Title:** {epic-title}

### Tasks

| ID | Title | Dependencies | Test Coverage |
|----|-------|--------------|---------------|
| {id} | {title} | None | Unit: X, Integration: Y |
| {id} | {title} | {dep-id} | Unit: X, Edge: Y |

### Task Details

#### {task-id}: {task-title}
**Goal:** [summary]
**Implementation:** [brief steps]
**Tests:** [test coverage summary]
**Dependencies:** [list or None]

[Repeat for each task]
```

Complete your work and document it in the plan. The reviewers will examine your breakdown next.
```

**Coordinator:** Wait for completion, then proceed to Phase 3.

---

### Phase 3: Implementation Review (Implementation Reviewer)

**Action:** Review tasks for technical correctness and feasibility

**Coordinator assigns Worker 2 (Implementation Reviewer) with prompt:**
```
You are the **Implementation Reviewer** for a research-to-tasks planning workflow.

**Research Document:** {path/to/research-doc.md}
**Plan Document:** {path/to/plan-doc.md}
**Epic ID:** {epic-id}

**Your Task:**
1. Read the research document for context
2. Read the plan document to see the task breakdown
3. Review each task using `bd show {task-id} --json`
4. Document your review in the plan document

**Review Checklist:**
- [ ] Task scope achievable in single session?
- [ ] Implementation steps clear and actionable?
- [ ] Dependencies properly identified?
- [ ] Follows existing codebase patterns?
- [ ] No overly complex or under-specified tasks?
- [ ] Prerequisites and constraints addressed?

**CRITICAL: Verify Dependencies Actually Exist in bd**

Do NOT just read the plan document - verify dependencies were created in bd:

1. List tasks with dependency counts:
   ```bash
   bd list --parent {epic-id} --all --json | jq -r '.[] | "\(.id): deps=\([.dependencies[]? | select(.type == "blocks")] | length)"'
   ```
2. Starting tasks should show `deps=0`, dependent tasks should show `deps=1` or more
3. If ALL tasks show `deps=0`, inter-task dependencies are MISSING - this is a blocker
4. For detailed check: `bd show {task-id} --json | jq '.[] | .dependencies[] | select(.dependency_type == "blocks")'`

If dependencies are missing, your verdict MUST be **CHANGES NEEDED** with specific instruction to run `bd dep add` commands.

**Use Edit tool to update plan under "## Implementation Review (Worker 2)":**

```markdown
## Implementation Review (Worker 2)

### Review Date
{YYYY-MM-DD}

### Overall Assessment
[PASS / CONCERNS]

### Task-by-Task Review

#### {task-id}: {task-title}
**Scope:** [Appropriate / Too Large / Too Small]
**Clarity:** [Clear / Needs Work]
**Issues:** [List any issues or "None"]
**Suggestions:** [Improvements or "None"]

[Repeat for each task]

### Dependency Verification
**Command run:** `bd list --parent {epic-id} --all --json | jq ...`
**Tasks with deps=0 (starting tasks):** [List task IDs]
**Tasks with deps>=1 (dependent tasks):** [List task IDs]
**Dependency check:** [PASS - mix of 0 and 1+ deps / FAIL - all tasks show deps=0]

### Critical Issues
[List blocking issues that MUST be addressed]

### Recommendations
[List non-blocking suggestions for improvement]

### Verdict
**Status:** [APPROVED / CHANGES NEEDED]

If CHANGES NEEDED:
1. {Specific required change}
2. {Specific required change}
```

Be thorough but constructive. Your goal is to ensure tasks are implementable.

**IMPORTANT:** If all tasks under the epic show `deps=0`, inter-task dependencies are missing. This MUST be flagged as CHANGES NEEDED.
```

**Coordinator:** Wait for completion, then proceed to Phase 4.

---

### Phase 4: Test Review (Test Reviewer)

**Action:** Review tasks for test coverage and quality

**Coordinator assigns Worker 3 (Test Reviewer) with prompt:**
```
You are the **Test Reviewer** for a research-to-tasks planning workflow.

**Research Document:** {path/to/research-doc.md}
**Plan Document:** {path/to/plan-doc.md}
**Epic ID:** {epic-id}

**Your Task:**
1. Read the research document for context
2. Read the plan document to see the task breakdown and implementation review
3. Review each task's test requirements using `bd show {task-id} --json`
4. Document your review in the plan document

**Review Checklist:**
- [ ] Every task includes specific test requirements?
- [ ] Test coverage matches implementation scope?
- [ ] Edge cases identified for complex logic?
- [ ] Integration points have test plans?
- [ ] No "implement now, test later" patterns?
- [ ] Test dependencies properly ordered?

**Red Flags to Watch For:**
- Tasks with implementation steps but no test steps
- Vague test requirements like "add tests"
- Tests deferred to later tasks
- Missing error case testing
- No negative test cases

**Use Edit tool to update plan under "## Test Review (Worker 3)":**

```markdown
## Test Review (Worker 3)

### Review Date
{YYYY-MM-DD}

### Overall Assessment
[PASS / CONCERNS]

### Test Coverage Analysis

#### {task-id}: {task-title}
**Unit Tests:** [Sufficient / Missing / Vague]
**Edge Cases:** [Covered / Missing: {list}]
**Integration:** [Covered / Not Applicable / Missing]
**Issues:** [List any issues or "None"]
**Suggestions:** [Specific test cases to add]

[Repeat for each task]

### Missing Test Coverage
[List specific areas lacking test coverage]

### Deferred Testing Found
[List any tasks that defer tests - this is a CRITICAL issue]

### Recommendations
[Specific improvements for test planning]

### Verdict
**Status:** [APPROVED / CHANGES NEEDED]

If CHANGES NEEDED:
1. {Specific required change}
2. {Specific required change}
```

Be strict about test coverage. Every task must have clear, specific test requirements.
```

**Coordinator:** Wait for completion, then proceed to Phase 5.

---

### Phase 5: Incorporate Feedback (Task Writer)

**Action:** Writer addresses reviewer feedback

**Only execute if either reviewer returned CHANGES NEEDED**

**Coordinator assigns Worker 1 (Task Writer) with prompt:**
```
You are the **Task Writer** returning to incorporate reviewer feedback.

**Plan Document:** {path/to/plan-doc.md}
**Epic ID:** {epic-id}

**Reviewer Feedback Summary:**
[Paste key issues from both reviews]

**Your Task:**
1. Read both reviews in the plan document
2. Address each issue raised:
   - Update task descriptions in bd
   - Add missing test requirements
   - Adjust scope or dependencies
   - Split or merge tasks as needed
3. Document your revisions in the plan document

**For each bd update:**
```bash
bd update {task-id} -d "
## Goal
[Updated goal]

## Implementation
[Updated steps]

## Tests Required
[Updated test requirements]

## Acceptance Criteria
[Updated criteria]
" --json
```

**Use Edit tool to update plan under "## Revisions":**

```markdown
## Revisions

### Revision Date
{YYYY-MM-DD}

### Changes Made

#### Issue: {issue from review}
**Action:** {what you did}
**Task(s) Affected:** {task-id(s)}

#### Issue: {issue from review}
**Action:** {what you did}
**Task(s) Affected:** {task-id(s)}

### Updated Task Summary

| ID | Title | Changes Made |
|----|-------|--------------|
| {id} | {title} | {brief change description} |

### Unaddressed Items
[List any feedback not incorporated and why, or "None"]
```

Address all critical issues. The reviewers will verify your changes.
```

**Coordinator:** Wait for completion, then proceed to Phase 6.

---

### Phase 6: Final Approval (Both Reviewers)

**Action:** Both reviewers verify changes and give final approval

**CRITICAL: Must be sequential to avoid file conflicts**

**Step 6a: Implementation Reviewer Final Check**

**Coordinator assigns Worker 2 with prompt:**
```
You are the **Implementation Reviewer** doing final verification.

**Plan Document:** {path/to/plan-doc.md}
**Epic ID:** {epic-id}

**Your Task:**
1. Read the "## Revisions" section
2. Verify your issues were addressed
3. Quick re-check of updated tasks via `bd show {task-id} --json`
4. Provide final verdict

**Use Edit tool to update "## Final Approval" section:**

```markdown
### Implementation Reviewer
**Status:** [APPROVED / NOT APPROVED]
**Comments:**
[Brief summary of verification - what was fixed, any remaining concerns]
```

If NOT APPROVED, be specific about what still needs work.
```

**Coordinator:** Wait for completion.

**Step 6b: Test Reviewer Final Check**

**Coordinator assigns Worker 3 with prompt:**
```
You are the **Test Reviewer** doing final verification.

**Plan Document:** {path/to/plan-doc.md}
**Epic ID:** {epic-id}

**Your Task:**
1. Read the "## Revisions" section
2. Verify test coverage issues were addressed
3. Quick re-check of updated tasks via `bd show {task-id} --json`
4. Provide final verdict

**Use Edit tool to update "## Final Approval" section:**

```markdown
### Test Reviewer
**Status:** [APPROVED / NOT APPROVED]
**Comments:**
[Brief summary of verification - what was fixed, any remaining concerns]
```

If NOT APPROVED, be specific about what still needs work.
```

**Coordinator:** Wait for completion, then proceed to Phase 7.

---

### Phase 7: Final Summary (Mediator)

**Action:** Mediator writes synthesis and closing summary

**Coordinator assigns Worker 4 (Mediator) with prompt:**
```
You are the **Mediator** completing the planning workflow.

**Plan Document:** {path/to/plan-doc.md}
**Epic ID:** {epic-id}

**Your Task:**
1. Read the entire plan document
2. Verify both reviewers approved (if not, flag for coordinator)
3. Write final summary synthesizing the planning process

**Use Edit tool to update "## Summary" section:**

```markdown
## Summary

### Planning Outcome
**Status:** [APPROVED AND READY / NEEDS ADDITIONAL WORK]

### Epic Created
- **ID:** {epic-id}
- **Title:** {epic-title}
- **Tasks:** {count} tasks

### Key Decisions Made
- [Decision 1 from planning process]
- [Decision 2 from planning process]

### Reviewer Consensus
- **Implementation:** {status} - {brief summary}
- **Test Coverage:** {status} - {brief summary}

### Test Integration Summary
[Confirm that all tasks include tests, no deferred testing]

### Ready for Execution
The epic and tasks are ready for implementation. Each task includes:
- Clear implementation steps
- Specific test requirements
- Proper dependencies

### Next Steps
1. Use `/cook` or `/pickup` to begin implementation
2. Each task should be completed with its tests before moving on
3. See research doc for detailed context: {path/to/research-doc.md}
```

Ensure the summary accurately reflects the planning process and outcome.
```

**Coordinator:** Wait for completion, then signal workflow complete:
```
signal_workflow_complete(
    status="success",
    summary="Translated research document into epic {epic-id} with {count} tasks. Both reviewers approved. Plan document at docs/plans/{filename}.md"
)
```

---

## Coordinator Instructions

### Setup
```
1. Get research document path from user
2. Spawn 4 workers (writer, impl-reviewer, test-reviewer, mediator)
3. Track worker IDs for each role
4. Generate plan document filename: docs/plans/YYYY-MM-DD-HHMM-{name}-plan.md
```

### Execution Flow
```
Phase 1: Assign Mediator → wait for plan doc creation
Phase 2: Assign Writer → wait for epic/task creation
Phase 3: Assign Implementation Reviewer → wait for review
Phase 4: Assign Test Reviewer → wait for review

If BOTH reviewers approved:
  → Skip Phase 5, proceed to Phase 6

If ANY reviewer returned CHANGES NEEDED:
  Phase 5: Assign Writer → wait for revisions
  Phase 6a: Assign Implementation Reviewer → wait for final approval
  Phase 6b: Assign Test Reviewer → wait for final approval

  If still NOT APPROVED:
    → Loop back to Phase 5 until approved

Phase 7: Assign Mediator → wait for summary
```

### Iteration Pattern
```
If reviewers still have concerns after revisions:
1. Send specific feedback to Writer
2. Writer makes additional revisions
3. Reviewers re-verify
4. Repeat until both approve

Max iterations: 3 (if still not approved, escalate to user)
```

---

## File Operation Rules

**CRITICAL: All file operations must be sequential**

The plan document is shared across all workers. To prevent conflicts:

1. Only ONE worker writes to the plan doc at a time
2. Coordinator waits for each phase to complete before assigning next
3. Workers must use Read tool before Edit tool
4. Each worker writes only to their designated section

**Violation = corrupted plan document**

---

## Common Pitfalls

### Task Planning Pitfalls
1. **Deferred testing** - "Add tests" as a separate task = REJECT
2. **Vague tests** - "Write unit tests" without specifics = REJECT
3. **Oversized tasks** - If it can't be done in one session, split it
4. **Missing dependencies** - Tasks that need prior work but don't declare it
5. **Orphan tests** - Tests without corresponding implementation
6. **Flat task structure** - Using only `--parent` creates parent-child links, NOT inter-task dependencies. You MUST use `bd dep add` for task ordering. Verify with the `bd list --parent {epic-id} --all --json | jq ...` dependency check - if all tasks show deps=0, inter-task dependencies are missing.

### Process Pitfalls
1. **Skipping reviews** - Both reviewers must approve
2. **Parallel file writes** - Always sequential
3. **Ignoring feedback** - Writer must address all critical issues
4. **Rushing approvals** - Reviewers must actually verify changes

---

## Success Criteria

A successful planning session produces:

- [ ] Plan document with full audit trail
- [ ] Beads epic with clear description
- [ ] Tasks with specific test requirements (not deferred)
- [ ] Proper inter-task dependencies via `bd dep add` (not just parent-child)
- [ ] Dependency verification passed (starting tasks have deps=0, dependent tasks have deps>=1)
- [ ] Both reviewers approved
- [ ] Mediator summary confirms readiness

### Quality Checks

**Every task should answer:**
1. What exactly needs to be implemented?
2. What specific tests validate the implementation?
3. What are the acceptance criteria?
4. What depends on this task? What does this task depend on?

**The plan document should show:**
1. Research context was understood
2. Reviewer concerns were addressed
3. Test coverage is comprehensive
4. No deferred testing

---

## Example Session

```
[Setup]
User: "Translate docs/research/clipboard-support.md into tasks"
Coordinator: Spawns 4 workers, generates plan filename

[Phase 1: Setup]
Mediator: Creates docs/plans/2025-01-15-1430-clipboard-support-plan.md

[Phase 2: Task Breakdown]
Writer: Creates epic perles-xyz, 4 tasks
- perles-xyz.1: Add clipboard package + unit tests
- perles-xyz.2: Add copy keybinding + keyboard tests
- perles-xyz.3: Add paste support + integration tests
- perles-xyz.4: Add visual feedback + UI tests

[Phase 3: Implementation Review]
Impl Reviewer: CHANGES NEEDED
- perles-xyz.2: Scope too large, split keybinding from handler
- perles-xyz.4: Missing dependency on xyz.2

[Phase 4: Test Review]
Test Reviewer: CHANGES NEEDED
- perles-xyz.1: Missing error case tests
- perles-xyz.3: Edge case for empty clipboard not covered

[Phase 5: Revisions]
Writer:
- Splits xyz.2 into xyz.2a (keybinding) and xyz.2b (handler)
- Adds dependency xyz.4 → xyz.2b
- Updates xyz.1 with error case tests
- Updates xyz.3 with empty clipboard test

[Phase 6a: Implementation Final]
Impl Reviewer: APPROVED - all concerns addressed

[Phase 6b: Test Final]
Test Reviewer: APPROVED - test coverage now comprehensive

[Phase 7: Summary]
Mediator: Writes final summary
- Epic perles-xyz ready with 5 tasks
- All tasks include specific tests
- Proper dependency chain established
- Ready for /cook execution

[Complete]
Coordinator: "Epic perles-xyz planned and ready. See docs/plans/2025-01-15-1430-clipboard-support-plan.md"
```

---

## When to Use This Workflow

**Good for:**
- Translating research/design docs into executable work
- Features requiring careful test planning
- Work that benefits from multiple review perspectives
- Creating audit trail of planning decisions

**Not necessary for:**
- Simple, well-understood changes
- Urgent hotfixes
- Single-task work items
- Already-planned epics

---

## Related Workflows

- **quick_plan.md** - Lighter weight, single reviewer
- **research_proposal.md** - When you need to DO research first
- **cook.md** - For executing the tasks once planned
