---
name: "Cook"
description: "Sequential task execution with code review and worker cycling"
category: "Work"
---

# Multi-Worker Orchestration Workflow

## Overview

This workflow coordinates multiple AI workers to complete all tasks from a **single epic** with mandatory code review, using the perles orchestration mode.

**Important:** This workflow is scoped to exactly one epic. When all tasks in the epic are complete, the epic itself must be closed.

## Getting Started

**First, ask the user which epic to work on:**

```
Which epic would you like me to work on? Please provide the epic ID (e.g., perles-abc1).
```

Once the user provides the epic ID, validate it exists and show the tasks:
```bash
bd show <epic-id> --json
```

## Key Principles

1. **Single epic scope** - Work only on tasks belonging to the specified epic
2. **One task at a time** - Only one worker works on implementation at any moment
3. **Fresh context per task** - Cycle out workers after each task to avoid context pollution
4. **Mandatory code review** - Every implementation gets reviewed by a different worker
5. **Proposal-first** - Workers must review the proposal before starting implementation
6. **No work without approval** - Writer must wait for reviewer approval before committing
7. **Close epic when done** - Mark the epic as closed after all tasks complete

## Workflow Pattern

### Phase 1: Setup

1. **Get epic from user**: Ask for the epic ID if not already provided

2. **Identify tasks**: Get all subtasks for the epic
   ```bash
   bd show <epic-id> --json
   bd ready --json
   ```

3. **Present execution plan**: Show user the task sequence and get approval

### Phase 2: Task Execution Loop

For each task in sequence (from first to last) NEVER parallelize only work on a single task at a time:

#### Step 1: Assign Implementation Worker

**Goal** Mark a task as in progress and assign to a fresh worker to work on the task coordinate the work and review.

**Critical** Mark the task as in progress before assigning: `bd update <task-id> --status in_progress`

Send task to a fresh worker with the follow exact prompt instructions:

**Prompt**
```
**Goal** Complete the task assigned to you with the highest possible quality effort. 

You are being assigned task **<task-id>**: <task-title>

**IMPORTANT: Before starting, if your tasks parent epic references a proposal document, you must read the full context of the proposal to understand the work.**
Your instructions are in the task description which you can read using the bd tool with `bd show <task-id>` read the full description this is your work!.

**CRITICAL*: Before committing your changes ask the coorindator to review your changes. The cooridnator will review and then
let you know if changes are needed or if you can proceed to commit. If changes are needed, you must make the changes and then 
ask for another review.
```

#### Step 2: Wait for Completion

Monitor message log for worker completion message.

#### Step 3: Assign Review Worker

Send to a **different** worker:
- Instructions to review the proposal first
- What was implemented
- Specific acceptance criteria to verify
- Code quality checks
- Request for explicit verdict: "APPROVED" or "CHANGES NEEDED"

**Example review prompt:**
```
You are being assigned to **review** the work completed by worker-X on task **<task-id>**.

## What worker-X did:
[Summary of changes]

## Your Review Process

### Step 1: Gather Context

```bash
# Get task details and acceptance criteria
bd show <task-id> --json

# Get changed files
git diff HEAD --name-only

# Get diff
git diff HEAD
```

### Step 2: Review All Dimensions

Perform a focused review covering:

#### A. Correctness
- Logic errors and edge cases
- Error handling
- Nil checks and control flow

#### B. Tests
- Run tests: `go test ./...` (or relevant package)
- Verify ALL pass
- Check test coverage for new code

#### C. Dead Code
- Any unused functions/variables?
- Test-only helpers that shouldn't exist?
- Unnecessary complexity?

#### D. Acceptance Criteria
- Each criterion from task description
- Verify with evidence from code

### Step 4: Issue Verdict

**After review, you MUST:**
1. Add comment to task with verdict
2. Update assignee back to coding-agent

## Response Formats

### APPROVAL

```bash
# Step 1: Add approval comment
bd comment <task-id> --author code-reviewer "APPROVED: <summary>. All tests pass. Code is correct and meets acceptance criteria."

# Step 2: Update assignee
bd update <task-id> --assignee coding-agent
```

Then respond:

```
## APPROVED

**Task:** <task-id>
**Files:** <count> files, <lines> lines changed

### Review Summary
- **Correctness:** Pass - <brief note>
- **Tests:** Pass - All tests pass
- **Dead Code:** Pass - No issues
- **Acceptance:** <X>/<X> criteria met

### Verification
- Tests run: `<command>`
- Result: ALL PASSING

### Actions Taken
- Comment added to task
- Assignee updated to coding-agent

**The coding-agent may now commit this code.**
```

### DENIED

```bash
# Step 1: Add denial comment
bd comment <task-id> --author code-reviewer "DENIED: <issues found>. Required: <fixes needed>."

# Step 2: Update assignee
bd update <task-id> --assignee coding-agent
```

Then respond:

```
## DENIED

**Task:** <task-id>
**Files:** <count> files, <lines> lines changed

### Issues Found

1. **<Category>** - <file:line>
   - Problem: <description>
   - Fix: <suggestion>

### Verification
- Tests run: `<command>`
- Result: <PASS/FAIL details>

### Required Changes
1. <specific change>
2. <specific change>

### Actions Taken
- Comment added to task
- Assignee updated to coding-agent

**Address the issues and resubmit for review.**
```

## Critical Rules

1. **Always verify assignee first** - Don't review if not `code-reviewer`
2. **Always run tests** - Never trust claims without verification
3. **Always add comment + update assignee** - Both actions are mandatory
4. **Be concise** - This is a simple review, not a dissertation
5. **Focus on what matters** - Correctness and tests are primary; style is secondary

## Quality Checklist

Before APPROVAL:
- [ ] All acceptance criteria met
- [ ] Code compiles
- [ ] All tests pass (verified)
- [ ] No dead code
- [ ] No obvious errors
- [ ] Changes are focused and minimal
```

#### Step 4: Handle Review Results

**If APPROVED:**
1. Tell writer worker to commit changes with specific commit message
2. Wait for commit confirmation
3. Mark task complete in bd: `bd close <task-id>`
4. Update todo list
5. Cycle out both workers (writer and reviewer)
6. Move to next task

**If CHANGES NEEDED:**
1. Tell writer worker what to fix
2. Writer fixes issues
3. Get another review from a different worker
4. Repeat until approved

#### Step 5: Worker Cycling

**Goal** Replace ONLY the workers that just finished working on the last task to avoid context pollution using your mcp tools.

Example: worker-1 wrote, worker-2 reviewed → replace worker-1 and worker-2 only.

```
replace_worker(writer-id, "Completed <task-id>, cycling for fresh context")
replace_worker(reviewer-id, "Reviewed <task-id>, cycling for fresh context")
```

This ensures:
- Fresh workers for next task (no context pollution)
- Retired workers get replaced automatically

### Phase 3: Epic Completion

When all tasks in the epic are complete:

1. **Verify all tasks are closed**: Check that every subtask has been completed
   ```bash
   bd show <epic-id> --json
   ```

2. **Close the epic**: Mark the epic itself as complete
   ```bash
   bd close <epic-id> --reason "All tasks completed successfully"
   ```

3. **Report completion**: Inform the user that the epic is finished
   ```
   Epic <epic-id> is now complete. All X tasks have been implemented and reviewed.
   ```

**Important:** Do not close the epic until ALL subtasks are verified complete. The epic closure signifies that the entire body of work is done.

## Task Assignment Strategy

### Rotation Pattern

```
Task 1: worker-1 implements → worker-2 reviews → both retire
Task 2: worker-3 implements → worker-4 reviews → both retire
Task 3: worker-5 implements → worker-6 reviews → both retire
Task 4: worker-7 implements → worker-8 reviews → both retire
Task 5: worker-9 implements → worker-10 reviews → both retire
...
```

## Communication Patterns

### Coordinator → Worker Messages

Use `send_to_worker(worker_id, message)` for:
- Task assignment with full context
- Review assignment
- Fix requests
- Commit instructions

### Coordinator → User Messages

Provide status updates:
- Task completion confirmations
- Review results
- Progress through epic
- Worker cycling notifications

### Message Log Monitoring

Check `read_message_log()` when workers send nudges:
- Track last processed timestamp to identify new messages
- Deduplicate completion notifications
- Only report meaningful state changes

## Commit Message Format

Every commit should include:
```
<type>(<scope>): <subject>

<body with details>
- Bullet points of changes

Part of <task-id>

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
```

## Todo List Management

Track epic progress:
```javascript
[
  {"content": "Complete task-1", "activeForm": "...", "status": "completed"},
  {"content": "Complete task-2", "activeForm": "...", "status": "in_progress"},
  {"content": "Complete task-3", "activeForm": "...", "status": "pending"},
  ...
]
```

Update after each task completion to show progress.

## Error Handling

### Worker Replacement Failures

If `replace_worker` fails with "max workers limit reached":
- New workers are still spawned automatically
- Check message log for new worker ready messages
- Continue with available workers

### Review Failures

If review finds critical issues:
1. Don't mark task complete
2. Send specific fix instructions to writer
3. Get re-review from fresh worker
4. Only commit after approval

### Test Failures

If tests fail during implementation:
- Keep task as in_progress
- Worker must fix until all tests pass
- Reviewer verifies test passage
- No commits until all green

## Best Practices

### 1. Always Review Proposal First

Every worker (implementation and review) must read the proposal to understand:
- Overall system design
- Why specific approaches were chosen
- How pieces fit together
- Potential pitfalls

### 2. Explicit Communication

- Clear task assignments with all necessary context
- Explicit approval/rejection from reviewers
- Confirmation of commits before marking complete

### 3. Systematic Progress

- One task fully complete before starting next
- No parallel task execution (prevents conflicts)
- Clean worker cycling between tasks

### 4. Quality Gates

- All acceptance criteria must be met
- All tests must pass (including race detection)
- Code must compile
- Reviewer must explicitly approve

### 5. Context Management

- Retire the workers that just worked on the last task (prevents context buildup)
- Fresh workers get clean slate
- Proposal review ensures consistency across workers

## Example Session Flow

```
[Getting Started]
Coordinator: "Which epic would you like me to work on?"
User: "perles-abc1"
Coordinator: Validates epic exists, shows 7 tasks
Coordinator: Presents execution plan → User approves

[Task 1: perles-abc1.1]
Coordinator: Assigns to worker-1 (with proposal review)
Worker-1: Completes implementation
Coordinator: Assigns review to worker-2
Worker-2: Reviews → APPROVED
Coordinator: Tells worker-1 to commit
Worker-1: Commits
Coordinator: Marks task complete (bd close perles-abc1.1)
Coordinator: Replaces worker-1 and worker-2

[Task 2: perles-abc1.2]
Coordinator: Assigns to worker-3 (with proposal review)
Worker-3: Completes implementation
Coordinator: Assigns review to worker-4
Worker-4: Reviews → APPROVED
Coordinator: Tells worker-3 to commit
Worker-3: Commits
Coordinator: Marks task complete
Coordinator: Replaces worker-3 and worker-4

[Continue pattern for remaining tasks...]

[Epic Completion]
Coordinator: Verifies all 7 tasks are closed
Coordinator: Closes epic (bd close perles-abc1 --reason "All tasks completed")
Coordinator: "Epic perles-abc1 is now complete. All 7 tasks implemented and reviewed."
```

## Key Success Metrics

- ✅ Epic ID obtained from user at start
- ✅ Every task reviewed before commit
- ✅ Every worker reads proposal before implementation
- ✅ All tests pass before task completion
- ✅ Workers cycled after each task
- ✅ Clean git history with atomic commits
- ✅ No context pollution across tasks
- ✅ Epic closed after all tasks complete

## Anti-Patterns to Avoid

❌ **Parallel implementation** - Causes merge conflicts and inconsistency
❌ **Skipping proposal review** - Workers miss critical context
❌ **No code review** - Quality issues slip through
❌ **Reusing workers across tasks** - Context pollution grows
❌ **Committing before approval** - Can't easily undo if problems found
❌ **Batching multiple tasks** - Harder to track and review
❌ **Skipping tests** - Bugs ship to next task

## Adaptation for Different Scenarios

### Complex Tasks (multi-file refactoring)
- Give worker extra time
- Consider more detailed review checklist
- May need multiple review passes

### Simple Tasks (documentation, small fixes)
- Can streamline but maintain review
- Still cycle workers to keep pool fresh

## Tools Reference

### MCP Tools Used

- `assign_task(worker_id, task_id)` - For bd-tracked tasks
- `send_to_worker(worker_id, message)` - For custom instructions
- `replace_worker(worker_id, reason)` - Cycle out used workers
- `list_workers()` - Check pool state
- `read_message_log(limit)` - Monitor worker messages
- `mark_task_complete(task_id)` - Close tasks in bd

### BD Commands

- `bd show <task-id> --json` - Get task details
- `bd ready --json` - Find unblocked tasks
- `bd update <task-id> --status in_progress` - Mark as task as in progress
- `bd close <task-id>` - Mark complete

### Git Commands

- Commit with proper format (see above)
- Always include task ID in commit message

---

**This workflow has proven effective for coordinating multi-worker task execution with quality gates and fresh context per task.**
