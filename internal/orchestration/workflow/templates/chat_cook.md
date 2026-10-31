---
name: "Cook"
description: "Sequential task execution with self-review gates - implement, review, commit, repeat"
category: "Work"
target_mode: "chat"
---

# Cook Workflow (Chat)

## Overview

A single-agent implementation workflow that executes all tasks from an epic with mandatory self-review gates. This chat version combines implementation and review into a disciplined solo flow.

**Flow:**
```
Phase 1: Setup - Get epic, review proposal, plan execution
Phase 2: Task Loop (for each task):
  Step A: Implement task
  Step B: Self-review implementation
  Step C: Fix any issues found
  Step D: Commit and close task
Phase 3: Close epic and report summary
```

**Key Principles:**
1. **Single epic scope** - Work only on tasks belonging to the specified epic
2. **One task at a time** - Complete each task fully before moving to the next
3. **Mandatory self-review** - Review your own implementation before committing
4. **Proposal-first** - Always review the proposal before starting implementation
5. **No shortcuts** - Self-review catches real issues, not rubber-stamped

---

## Getting Started

**First, ask the user which epic to work on:**

```
Which epic would you like me to work on? Please provide the epic ID (e.g., perles-abc).
```

Once the user provides the epic ID, validate and show the tasks:
```bash
bd show <epic-id> --json                 # Epic details
bd list --parent <epic-id> --all --json  # All tasks in the epic (including closed)
bd ready --parent <epic-id> --json       # Unblocked tasks in the epic
```

---

## Workflow Phases

### Phase 1: Setup

**Goal:** Understand the work, review context, plan execution order.

**Steps:**

1. **Validate epic exists:**
   ```bash
   bd show <epic-id> --json
   ```

2. **Read the proposal** (linked in epic description):
   - Understand the big picture
   - Note implementation patterns and constraints
   - Review testing strategy
   - Understand acceptance criteria

3. **Identify task order:**
   ```bash
   bd ready --parent <epic-id> --json  # Shows unblocked tasks in the epic
   ```

4. **Present execution plan to user:**
   ```
   ## Execution Plan
   
   **Epic:** <epic-id> - <title>
   **Proposal:** <proposal-path>
   
   **Task Order:**
   1. <task-id>: <title> (ready)
   2. <task-id>: <title> (blocked by #1)
   3. <task-id>: <title> (blocked by #2)
   ...
   
   **Approach:** I'll implement each task sequentially, self-review before committing.
   
   Ready to begin?
   ```

5. **Get user approval** before starting implementation.

---

### Phase 2: Task Execution Loop

For each task in sequence:

#### Step A: Implement Task

1. **Read task details:**
   ```bash
   bd show <task-id> --json
   ```

2. **Mark in progress:**
   ```bash
   bd update <task-id> --status in_progress --json
   ```

3. **Implement the task:**
   - Follow the implementation steps in the task description
   - Write tests as specified in "Tests Required"
   - Follow patterns from the proposal

4. **Run tests:**
   ```bash
   go test ./path/to/package -v
   # Or appropriate test command for the codebase
   ```

5. **Verify all tests pass before proceeding.**

---

#### Step B: Self-Review Implementation

**Goal:** Catch issues before committing. Be honest - this is your quality gate.

**Review Checklist:**

**Correctness:**
- [ ] Does the implementation match the task requirements?
- [ ] Are all acceptance criteria from the task met?
- [ ] Does it follow patterns established in the proposal?
- [ ] Are edge cases handled?

**Tests:**
- [ ] Are all "Tests Required" from the task implemented?
- [ ] Do tests actually verify the functionality (not just pass)?
- [ ] Are error cases tested?
- [ ] Are there any missing test scenarios?

**Code Quality:**
- [ ] Is the code readable and maintainable?
- [ ] Are there any dead code or debug statements left?
- [ ] Does it follow codebase conventions?
- [ ] Is error handling appropriate?

**Integration:**
- [ ] Does it work with existing code?
- [ ] Are there any unintended side effects?
- [ ] Do all existing tests still pass?

**Red Flags to Fix:**
- Hardcoded values that should be configurable
- Missing error handling
- Tests that don't actually assert behavior
- Copy-pasted code that should be refactored
- TODOs or FIXMEs left in code
- Acceptance criteria not fully met

---

#### Step C: Fix Issues Found

If self-review identified issues:

1. **Document what you found:**
   ```
   ## Self-Review Findings
   
   **Issues Found:**
   1. Missing error handling in <function>
   2. Test doesn't verify <edge case>
   
   **Fixing now...**
   ```

2. **Make the fixes**

3. **Re-run tests**

4. **Re-review the fixes** (don't skip!)

5. **Repeat until all issues resolved**

---

#### Step D: Commit and Close Task

1. **Stage and commit with proper format:**
   ```bash
   git add <files>
   git commit -m "<type>(<scope>): <description> (<task-id>)

   <detailed description if needed>

   - <bullet point of key change>
   - <bullet point of key change>

   🤖 Generated with Amp
   Co-Authored-By: Amp <noreply@sourcegraph.com>"
   ```

2. **Mark task complete:**
   ```bash
   bd close <task-id> --reason "Implemented and tested"
   ```

3. **Confirm completion:**
   ```
   ✅ Task <task-id> complete: <title>
   - Implementation: Done
   - Tests: All passing
   - Self-review: Approved
   - Committed: <commit-hash>
   ```

4. **Check for next task:**
   ```bash
   bd ready --parent <epic-id> --json
   ```

5. **Proceed to next task or Phase 3 if epic complete.**

---

### Phase 3: Epic Completion

When all tasks are done:

1. **Verify all tasks closed:**
   ```bash
   bd list --parent <epic-id> --all --json
   ```
   - All subtasks should show `status: closed`

2. **Run full test suite:**
   ```bash
   go test ./... -v
   # Or appropriate test command
   ```

3. **Close the epic:**
   ```bash
   bd close <epic-id> --reason "All tasks completed"
   ```

4. **Report summary to user:**
   ```
   ## Epic Complete
   
   **Epic:** <epic-id> - <title>
   **Proposal:** <proposal-path>
   
   **Tasks Completed:**
   1. ✅ <task-id>: <title> - <commit-hash>
   2. ✅ <task-id>: <title> - <commit-hash>
   ...
   
   **Test Health:** All tests passing
   
   **Commits:** <count> commits
   
   **Epic is now closed.**
   ```

---

## Self-Review Guidelines

### What Makes a Good Self-Review

**DO:**
- Actually check each item, don't just mark them done
- Re-read your code as if seeing it for the first time
- Run the tests and verify they test what they claim
- Check the acceptance criteria from the task explicitly
- Find at least one thing to improve (if you find nothing, look harder)

**DON'T:**
- Rubber-stamp your own work
- Skip items because "it's obviously fine"
- Rush through after implementing
- Ignore the gut feeling that something isn't right
- Commit code you wouldn't want to review later

### Self-Review Intensity by Change Type

| Change Type | Review Focus |
|-------------|--------------|
| **Small fix** (typo, config) | Quick sanity check, ensure no regressions |
| **New feature** | Full checklist, acceptance criteria verification |
| **Refactoring** | Behavior preservation, test coverage, edge cases |
| **Bug fix** | Root cause addressed, regression test added |
| **API change** | Backwards compatibility, documentation, error cases |

---

## Common Pitfalls

### Process Pitfalls
1. **Skipping self-review** - "It's just a small change" → bugs ship
2. **Rubber-stamp reviews** - Checking boxes without thinking → quality degrades
3. **Committing before tests pass** - Creates broken state → wastes time
4. **Skipping proposal review** - Miss important context → wrong implementation
5. **Batching multiple tasks** - Harder to review and debug → confusion

### Implementation Pitfalls
1. **Ignoring task acceptance criteria** - Implement what's asked
2. **Skipping specified tests** - Tests Required = tests required
3. **Leaving TODOs** - Complete the work fully
4. **Not running full test suite** - Regressions slip through

---

## Example Session

```
[Phase 1: Setup]
User: "Work on epic perles-abc"
AI: Reads epic, finds proposal at docs/proposals/2025-01-15-clipboard.md
AI: Reads proposal thoroughly
AI: Lists tasks: perles-abc.1, perles-abc.2, perles-abc.3
AI: "Ready to implement 3 tasks sequentially. Begin?"
User: "Yes"

[Phase 2: Task 1]
AI: Reads perles-abc.1, marks in_progress
AI: Implements clipboard package with unit tests
AI: Runs tests - all pass

[Self-Review Task 1]
AI: Reviews against checklist
AI: Finds: "Error case for failed clipboard access not tested"
AI: Adds error handling test
AI: Re-runs tests - all pass
AI: "Self-review complete, no remaining issues"

AI: Commits with message "feat(clipboard): add package (perles-abc.1)"
AI: Closes task perles-abc.1
AI: "✅ Task perles-abc.1 complete"

[Phase 2: Task 2]
AI: Reads perles-abc.2, marks in_progress
AI: Implements copy keybinding
AI: Runs tests - all pass

[Self-Review Task 2]
AI: Reviews against checklist
AI: All items pass
AI: Commits with message "feat(keys): add copy keybinding (perles-abc.2)"
AI: Closes task perles-abc.2

[Phase 2: Task 3]
... same pattern ...

[Phase 3: Completion]
AI: Verifies all 3 tasks closed
AI: Runs full test suite - all pass
AI: Closes epic perles-abc
AI: "Epic perles-abc complete. 3 tasks implemented and reviewed."
```

---

## Success Metrics

- ✅ Proposal reviewed before starting
- ✅ Each task self-reviewed before commit
- ✅ All tests pass before each commit
- ✅ No acceptance criteria skipped
- ✅ Clean git history with atomic commits
- ✅ Epic closed after all tasks complete

---

## When to Use This Workflow

**Good for:**
- Executing well-planned epics
- Sequential task dependencies
- When you want thorough single-agent execution
- Medium-complexity work (3-10 tasks)

**Use orchestration cook.md instead for:**
- Large epics benefiting from parallel workers
- When external code review is required
- Complex work needing fresh context per task
- Team settings with multiple reviewers