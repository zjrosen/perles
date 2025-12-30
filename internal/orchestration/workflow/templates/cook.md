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
8. **Structured tools for state changes** - Use MCP tools (`assign_task`, `assign_task_review`, etc.) for workflow state transitions
9. **Query before assign** - Always check worker state before making assignments to prevent duplicates

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

**Goal** Assign a task to a fresh worker using structured tools for deterministic state tracking.

**Process:**

1. **Query worker state** to find an available worker and verify task isn't already assigned:
   ```
   query_worker_state(task_id="<task-id>")
   ```
   - Check `ready_workers` list for available workers
   - Verify task not in `task_assignments` (prevents duplicate assignment)

2. **Assign task** using the structured tool:
   ```
   assign_task(worker_id="<worker-id>", task_id="<task-id>", summary="<optional task instructions>")
   ```
   - This automatically:
     - Marks task as `in_progress` in BD
     - Sets worker phase to `Implementing`
     - Tracks the assignment for state queries
     - Sends task details to worker

3. **Optionally send supplementary context** via `send_to_worker` if needed:
   ```
   send_to_worker(worker_id, "Additional context: <details>")
   ```

**Note:** The worker will receive instructions to call `report_implementation_complete` when done, which transitions them to `AwaitingReview` phase.

#### Step 2: Wait for Implementation Completion

Monitor message log for worker completion signal. The worker will call `report_implementation_complete(summary)` when done, which:
- Transitions worker phase from `Implementing` to `AwaitingReview`
- Posts a message to coordinator with implementation summary
- Updates the internal state tracking

When you receive this notification via message log, proceed to assign a reviewer.

#### Step 3: Assign Review Worker

**Goal** Assign a reviewer using the structured `assign_task_review` tool for deterministic state tracking.

**Process:**

1. **Query worker state** to verify implementer is awaiting review and find available reviewer:
   ```
   query_worker_state(worker_id="<implementer-id>")
   ```
   - Verify implementer phase is `awaiting_review`
   - Check `ready_workers` for available reviewer (must be different from implementer)

2. **Assign review** using the structured tool:
   ```
   assign_task_review(
       reviewer_id="<reviewer-id>",
       task_id="<task-id>",
       implementer_id="<implementer-id>",
       summary="<brief summary of what was implemented>"
   )
   ```
   - This automatically:
     - Validates reviewer ≠ implementer
     - Sets reviewer phase to `Reviewing`
     - Sends review instructions to reviewer
     - Tracks the review assignment
     - Adds BD comment: "Review assigned to worker-X"

**Note:** The reviewer will call `report_review_verdict(verdict, comments)` when done, which handles the state transition and notifies the coordinator.

**What the reviewer receives:**

The `assign_task_review` tool sends a comprehensive review prompt to the reviewer including:
- Task details and acceptance criteria (fetched from BD)
- Summary of what was implemented
- Instructions to review all dimensions (correctness, tests, dead code, acceptance)
- Instructions to use `report_review_verdict("APPROVED", comments)` or `report_review_verdict("DENIED", comments)`

#### Step 4: Handle Review Results

The reviewer will call `report_review_verdict(verdict, comments)` which:
- Updates task status to `approved` or `denied`
- Transitions reviewer phase back to `Idle` (ready for next assignment)
- Adds BD comment with the verdict
- Posts message to coordinator

**If APPROVED:**

1. **Approve commit** using the structured tool:
   ```
   approve_commit(
       implementer_id="<implementer-id>",
       task_id="<task-id>",
       commit_message="<suggested commit message>"  // optional
   )
   ```
   - This automatically:
     - Validates task is in `approved` status
     - Sets implementer phase to `Committing`
     - Sends commit instructions to implementer
     - Adds BD comment: "Commit approved"

2. **Wait for commit confirmation** from implementer

3. **Mark task complete**: `mark_task_complete(task_id="<task-id>")`

4. **Cycle out workers** (see Step 5)

5. **Move to next task**

**If DENIED:**

1. **Send review feedback** using the structured tool:
   ```
   assign_review_feedback(
       implementer_id="<implementer-id>",
       task_id="<task-id>",
       feedback="<specific feedback about required changes>"
   )
   ```
   - This automatically:
     - Validates task is in `denied` status
     - Sets implementer phase to `AddressingFeedback`
     - Sends feedback message to implementer
     - Adds BD comment with feedback

2. **Wait for implementer to fix** - they will call `report_implementation_complete` again when ready

3. **Assign another review** (return to Step 3) - use a fresh reviewer

4. **Repeat until approved**

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

**Handling replacement worker "ready" messages:**

After calling `replace_worker`, new workers will spawn and send "ready" messages. The human will nudge you with "[worker-X sent a message]".

When this happens:
1. Call `read_message_log()` to confirm the new workers are ready
2. Note their readiness: "worker-9 and worker-10 are now ready"
3. **Do NOT assign them work yet** - they're backups for future tasks
4. Continue with the next task using workers that haven't been used yet
5. Only assign work to completely fresh workers (never reuse a worker that already worked on a previous task in this epic)

**Worker selection priority:**
- Use workers that have NEVER been assigned work in this epic
- Skip workers that completed previous tasks (even if they've been replaced)
- Replacement workers are available for future tasks if you run out of fresh workers

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

### Coordinator → Worker: Structured Tools (State Transitions)

Use structured MCP tools for all state-changing operations:

| Operation | Tool | Purpose |
|-----------|------|---------|
| Assign implementation | `assign_task(worker_id, task_id)` | Start task with deterministic tracking |
| Assign review | `assign_task_review(reviewer_id, task_id, implementer_id, summary)` | Assign reviewer with validation |
| Send denial feedback | `assign_review_feedback(implementer_id, task_id, feedback)` | Send fixes needed after denial |
| Approve commit | `approve_commit(implementer_id, task_id, commit_message)` | Authorize implementer to commit |

### Coordinator → Worker: Free-Form Messages (Supplementary)

Use `send_to_worker(worker_id, message)` **only** for:
- Clarifications or additional context after task assignment
- Nudges or reminders
- Custom instructions not covered by structured tools
- **NOT** for state-changing operations (use structured tools instead)

### Worker → Coordinator: Structured Tools (State Transitions)

Workers use these MCP tools to signal workflow state changes:

| Operation | Tool | Purpose |
|-----------|------|---------|
| Implementation done | `report_implementation_complete(summary)` | Transition to `AwaitingReview` phase |
| Review verdict | `report_review_verdict(verdict, comments)` | Report APPROVED/DENIED, transition to `Idle` |

### Coordinator → User Messages

Provide status updates:
- Task completion confirmations
- Review results
- Progress through epic
- Worker cycling notifications

### Message Log Monitoring

**CRITICAL: Coordinator must deduplicate worker messages to avoid duplicate actions.**

Workers may send duplicate messages due to system behavior. The coordinator MUST track which state transitions have been processed and ignore duplicates.

#### Deduplication Strategy

**Track worker state in your working memory:**
```
worker-1: assigned → implementing
worker-2: assigned → reviewing
worker-1: implementing → completed (PROCESSED)
worker-2: reviewing → approved (PROCESSED)
```

**When human nudges you that a worker sent a message:**

1. Call `read_message_log()` to see new messages
2. Identify the worker and the state transition (e.g., "worker-3 completed implementation")
3. **Check if you've already processed this exact state transition:**
   - If YES: Respond "Already processed: worker-3 completion. No action needed."
   - If NO: Process the message and take action (assign reviewer, tell worker to commit, etc.)
4. Update your mental state tracker after taking action

#### Examples of Duplicate Messages

**Scenario 1: Worker sends 3 identical "implementation complete" messages**
- First message at 14:08:42: Process it → Assign reviewer
- Second message at 14:08:44: "Already processed: worker-3 completion. No action needed."
- Third message at 14:08:48: "Already processed: worker-3 completion. No action needed."

**Scenario 2: Reviewer sends 3 identical "APPROVED" messages**
- First message at 14:09:49: Process it → Tell writer to commit
- Second message at 14:10:14: "Already processed: worker-4 approval. No action needed."
- Third message at 14:10:15: "Already processed: worker-4 approval. No action needed."

**Scenario 3: New worker sends "ready" message after replacement**
- This is a NEW state transition (worker spawned → ready)
- Process it once: "worker-5 ready"
- Don't confuse with task completion messages

#### What NOT to do

❌ Don't just say "No response requested" without explanation
❌ Don't re-assign work that's already been assigned
❌ Don't tell a worker to commit multiple times
❌ Don't assign multiple reviewers for the same task

#### State Transition Examples (Worker Phases)

Valid phase transitions tracked by structured tools:

| From Phase | To Phase | Trigger | Tool |
|------------|----------|---------|------|
| `Idle` | `Implementing` | Task assigned | `assign_task` |
| `Implementing` | `AwaitingReview` | Worker signals done | `report_implementation_complete` |
| `AwaitingReview` | `Reviewing` | Reviewer assigned | `assign_task_review` |
| `Reviewing` | `Idle` | Verdict delivered | `report_review_verdict` |
| `AwaitingReview` | `Committing` | Commit approved | `approve_commit` |
| `AwaitingReview` | `AddressingFeedback` | Denial feedback sent | `assign_review_feedback` |
| `AddressingFeedback` | `AwaitingReview` | Worker signals fixes done | `report_implementation_complete` |

**Note:** The structured tools automatically track these transitions. Use `query_worker_state()` to check current state before making assignments.

Each transition should only be processed ONCE.

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
# Query state before assignment
Coordinator: query_worker_state(task_id="perles-abc1.1")
# Response: task not assigned, worker-1 in ready_workers

# Assign implementation
Coordinator: assign_task(worker_id="worker-1", task_id="perles-abc1.1")
# worker-1 phase transitions: Idle → Implementing

# Wait for implementation completion
Worker-1: report_implementation_complete(summary="Added validation logic for user input")
# worker-1 phase transitions: Implementing → AwaitingReview
# Coordinator receives message via message log

# Query state before review assignment
Coordinator: query_worker_state(worker_id="worker-1")
# Response: worker-1 phase is "awaiting_review", worker-2 in ready_workers

# Assign review
Coordinator: assign_task_review(
    reviewer_id="worker-2",
    task_id="perles-abc1.1",
    implementer_id="worker-1",
    summary="Added validation logic for user input"
)
# worker-2 phase transitions: Idle → Reviewing

# Wait for review verdict
Worker-2: report_review_verdict(verdict="APPROVED", comments="All tests pass, code is correct")
# worker-2 phase transitions: Reviewing → Idle
# Task status: approved

# Approve commit
Coordinator: approve_commit(
    implementer_id="worker-1",
    task_id="perles-abc1.1",
    commit_message="feat(validation): add user input validation"
)
# worker-1 phase transitions: AwaitingReview → Committing

# Wait for commit confirmation, then mark task complete
Coordinator: mark_task_complete(task_id="perles-abc1.1")

# Cycle workers
Coordinator: replace_worker(worker-1, "Completed perles-abc1.1")
Coordinator: replace_worker(worker-2, "Reviewed perles-abc1.1")

[Task 2: perles-abc1.2]
# Same pattern with worker-3 (implement) and worker-4 (review)
Coordinator: query_worker_state(task_id="perles-abc1.2")
Coordinator: assign_task(worker_id="worker-3", task_id="perles-abc1.2")
Worker-3: report_implementation_complete(summary="...")
Coordinator: assign_task_review(reviewer_id="worker-4", ...)
Worker-4: report_review_verdict(verdict="APPROVED", comments="...")
Coordinator: approve_commit(implementer_id="worker-3", ...)
Coordinator: mark_task_complete(task_id="perles-abc1.2"), replace workers

[Continue pattern for remaining tasks...]

[Epic Completion]
Coordinator: Verifies all 7 tasks are closed via bd show perles-abc1 --json
Coordinator: bd close perles-abc1 --reason "All tasks completed"
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
- ✅ **Structured tools used for all state transitions** (assign_task, assign_task_review, etc.)
- ✅ **Query-before-assign pattern followed** (query_worker_state before assignments)
- ✅ **Worker tools used for completion signals** (report_implementation_complete, report_review_verdict)

## Anti-Patterns to Avoid

❌ **Parallel implementation** - Causes merge conflicts and inconsistency
❌ **Skipping proposal review** - Workers miss critical context
❌ **No code review** - Quality issues slip through
❌ **Reusing workers across tasks** - Context pollution grows
❌ **Committing before approval** - Can't easily undo if problems found
❌ **Batching multiple tasks** - Harder to track and review
❌ **Skipping tests** - Bugs ship to next task
❌ **Ignoring duplicate messages** - Process each state transition only once
❌ **Re-assigning already assigned work** - Check if you've already assigned a reviewer/committer
❌ **Assigning work to replacement workers immediately** - Let them sit idle as backups
❌ **Saying "No response requested" without explanation** - Always explain why you're not taking action
❌ **Using send_to_worker for state changes** - Use structured tools (assign_task, assign_task_review, etc.) instead
❌ **Skipping query_worker_state** - Always check state before making assignments

## Adaptation for Different Scenarios

### Complex Tasks (multi-file refactoring)
- Give worker extra time
- Consider more detailed review checklist
- May need multiple review passes

### Simple Tasks (documentation, small fixes)
- Can streamline but maintain review
- Still cycle workers to keep pool fresh

## Tools Reference

### Coordinator MCP Tools

#### State-Changing Tools (Use for Workflow Transitions)

| Tool | Parameters | Purpose |
|------|------------|---------|
| `assign_task` | `worker_id`, `task_id`, `summary` (optional) | Assign implementation task to worker |
| `assign_task_review` | `reviewer_id`, `task_id`, `implementer_id`, `summary` | Assign reviewer (validates ≠ implementer) |
| `assign_review_feedback` | `implementer_id`, `task_id`, `feedback` | Send denial feedback to implementer |
| `approve_commit` | `implementer_id`, `task_id`, `commit_message` (optional) | Authorize worker to commit |

#### Query and Management Tools

| Tool | Parameters | Purpose |
|------|------------|---------|
| `query_worker_state` | `worker_id` (optional), `task_id` (optional) | Get worker phases, roles, and ready workers |
| `list_workers` | none | List all workers with status and phase info |
| `replace_worker` | `worker_id`, `reason` | Cycle out worker and spawn replacement |
| `read_message_log` | `limit` (optional) | Monitor worker messages |

#### Supplementary Communication

| Tool | Parameters | Purpose |
|------|------------|---------|
| `send_to_worker` | `worker_id`, `message` | Send clarifications or additional context (NOT for state changes) |

### Worker MCP Tools

| Tool | Parameters | Purpose |
|------|------------|---------|
| `report_implementation_complete` | `summary` | Signal implementation done, transition to `AwaitingReview` |
| `report_review_verdict` | `verdict` (APPROVED/DENIED), `comments` | Report review result, transition to `Idle` |
| `signal_ready` | none | Signal ready for task assignment (on startup) |
| `post_message` | `to`, `content` | Send message to coordinator or workers |
| `check_messages` | none | Check for new messages |

### BD Commands

- `bd show <task-id> --json` - Get task details
- `bd ready --json` - Find unblocked tasks
- `bd update <task-id> --status in_progress` - Mark task as in progress (usually done automatically by `assign_task`)
- `bd close <epic-id>` - Close epic (use `mark_task_complete` MCP tool for tasks)

### Git Commands

- Commit with proper format (see above)
- Always include task ID in commit message

---

## Query-Before-Assign Pattern

**CRITICAL:** Always query worker state before making assignments to prevent duplicates and ensure valid state.

### Why This Matters

Without state checks:
- Same task might be assigned to multiple workers
- Reviewer might be assigned when implementer isn't ready
- Self-review might accidentally occur (reviewer == implementer)
- Task might be assigned to worker already working on something else

### Pattern

```
# Before any assignment, query current state
query_worker_state(task_id="<task-id>")

# Check the response:
# - ready_workers: list of workers available for assignment
# - task_assignments: map showing who's working on what
# - workers: list with phase/role for each worker

# Only proceed if:
# 1. Task is not already assigned (not in task_assignments)
# 2. Target worker is in ready_workers (or appropriate phase)
# 3. For review: implementer is in AwaitingReview phase
```

### Examples

**Before assigning implementation:**
```
query_worker_state(task_id="perles-abc.1")
# Response shows task not in task_assignments
# worker-3 in ready_workers
assign_task(worker_id="worker-3", task_id="perles-abc.1")
```

**Before assigning review:**
```
query_worker_state(worker_id="worker-3")
# Response shows worker-3 phase is "awaiting_review"
# worker-4 in ready_workers (and worker-4 ≠ worker-3)
assign_task_review(reviewer_id="worker-4", task_id="perles-abc.1", implementer_id="worker-3", summary="...")
```

---

## Hybrid Approach: Structured Tools + send_to_worker

This workflow uses a **hybrid approach** for communication:

### Use Structured Tools For:

✅ **All state-changing operations**
- Task assignment → `assign_task`
- Review assignment → `assign_task_review`
- Denial feedback → `assign_review_feedback`
- Commit approval → `approve_commit`

✅ **State queries**
- Check availability → `query_worker_state`

### Use send_to_worker For:

✅ **Supplementary communication only**
- Additional context after task assignment
- Clarifications about requirements
- Nudges or reminders
- Custom instructions not in structured tool prompts

### Rule: "Structured tools for state changes, send_to_worker for details"

**Good:**
```
assign_task(worker_id="worker-1", task_id="perles-abc.1")
send_to_worker(worker_id="worker-1", "Note: Pay special attention to error handling in the validation logic")
```

**Bad:**
```
# Don't use send_to_worker for task assignment!
send_to_worker(worker_id="worker-1", "You are being assigned task perles-abc.1...")  # ❌ NO STATE TRACKING
```

---

**This workflow has proven effective for coordinating multi-worker task execution with quality gates, deterministic state tracking, and fresh context per task.**
