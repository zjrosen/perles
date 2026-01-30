package prompt

import "fmt"

// WorkerMCPInstructions generates the MCP server instructions for a worker agent.
// This is a brief description of available tools sent during MCP initialization.
func WorkerMCPInstructions(workerID string) string {
	return fmt.Sprintf(`MCP server for %s - a worker agent in the Perles orchestration system.

Available tools:
- signal_ready: Signal readiness for task assignment (call once on startup)
- fabric_inbox: Check for unread messages addressed to you
- fabric_send: Start NEW conversation in a channel (#general, #planning, #tasks, #system)
- fabric_reply: Reply to an EXISTING message thread (use the message_id from the message you're responding to)
- report_implementation_complete: Report bd task completion with summary
- report_review_verdict: Report code review verdict (APPROVED/DENIED)
- post_accountability_summary: Save accountability summary for session tracking

**IMPORTANT: fabric_send vs fabric_reply:**
- When someone @mentions you in a message: use fabric_reply with that message's ID to continue the thread
- When reporting task completion: use fabric_reply to the task assignment thread
- When starting a genuinely new topic: use fabric_send to create a new message
- Thread replies keep conversations organized and notify all thread participants

Workers receive tasks via messages and must report completion:
- For bd tasks: use report_implementation_complete (falls back to fabric_reply if tool errors)
- For task completions: use fabric_reply to the task assignment thread
- For new topics or asking for help: use fabric_send`, workerID)
}

// TaskAssignmentPrompt generates the prompt sent to a worker when assigning a task.
// The summary parameter is optional and provides additional instructions/context from the coordinator.
// The threadID parameter is the Fabric thread ID for task updates - workers should use fabric_reply to this thread.
func TaskAssignmentPrompt(taskID, title, summary, threadID string) string {
	prompt := fmt.Sprintf(`[TASK ASSIGNMENT]

**Task ID:** %s
**Title:** %s
**Fabric Thread ID:** %s

## Implementation Workflow

Follow these phases in order. Use sub-agents for parallel exploration and verification.

---

### Phase 1: Understand

**Goal:** Fully understand what you're building before writing code.

1. **Read the task**: `+"`bd show %s`"+`
   - Extract ALL acceptance criteria (checkboxes)
   - Note any verification commands specified
   - Identify dependencies or blockers

2. **Read the proposal** (if epic references one):
   - Understand the "why" behind this task
   - Note architectural decisions that affect implementation
   - Find any patterns or examples to follow

3. **Explore the codebase** (spawn sub-agent if complex):
   - Find existing patterns to follow
   - Identify files that will be modified
   - Understand interfaces and dependencies

**Sub-Agent Option - Code Explorer:**
`+"```"+`
Spawn a sub-agent to explore: "Find all usages of [interface/function] and understand
the existing patterns. Return: entry points, call chains, and patterns to follow."
`+"```"+`

---

### Phase 2: Plan

**Goal:** Have a clear implementation plan before coding.

1. **List the changes needed:**
   - Which files to create/modify
   - Which interfaces to update
   - Which tests to add

2. **Identify risks:**
   - What could break?
   - What edge cases matter?
   - Are there integration points to test?

3. **Order of operations:**
   - What must be done first?
   - What can be parallelized?

---

### Phase 3: Implement

**Goal:** Write clean, correct code following project conventions.

1. **Follow existing patterns** - match the codebase style
2. **Handle edge cases** - nil checks, empty inputs, boundaries
3. **Handle errors properly** - no swallowed errors, wrap with context
4. **Write tests as you go** - don't leave testing for the end

**CRITICAL - Avoid These Anti-Patterns:**

❌ **Test-only helpers** (BLOCKER - always wrong):
- Do NOT add methods like `+"`Focused()`"+`, `+"`GetState()`"+`, `+"`Values()`"+` just to make testing easier
- If a method is only called from test files, it's dead code
- Tests should verify behavior through the public interface

❌ **Dead code:**
- Every function you write MUST be called from production code
- Verify with: `+"`grep -rn 'FunctionName' --include='*.go' | grep -v '_test.go'`"+`
- If only the definition appears, delete it

❌ **Swallowed errors:**
- Never use `+"`_, _ = someFunc()`"+` to ignore errors
- Always check and propagate errors

---

### Phase 4: Test

**Goal:** Verify your implementation is correct and complete.

1. **Run the tests:**
   `+"```bash"+`
   go test ./path/to/package -v
   `+"```"+`

2. **Verify ALL tests pass** - not just the ones you wrote

3. **Check test quality:**
   - Are edge cases covered?
   - Are error paths tested?
   - Do assertions verify actual behavior (not just "doesn't crash")?

**Sub-Agent Option - Test Verification:**
`+"```"+`
Spawn a sub-agent: "Run all tests in [package]. Verify they pass. Check for
any flaky tests or missing coverage. Return: test results and coverage gaps."
`+"```"+`

---

### Phase 5: Verify Acceptance Criteria

**Goal:** Gather evidence that EVERY acceptance criterion is met.

For EACH checkbox in the task description:

1. **Run verification commands** exactly as specified
2. **Document the evidence** (command output, file:line references)
3. **Mark criterion as verified** only with proof

**Evidence Format:**
`+"```"+`
Criterion: "Interface signature updated in repo.go"
Evidence: repo.go:45 now has `+"`func FindByID(ctx context.Context, id string)`"+`
Verified: ✅

Criterion: "NO mock.Anything for ID parameter"
Command: grep -rn "mock.Anything" --include="*_test.go" | grep -i "id"
Output: (zero results)
Verified: ✅
`+"```"+`

**Sub-Agent Option - Acceptance Verification:**
`+"```"+`
Spawn a sub-agent: "Verify each acceptance criterion from the task. Run all
verification commands. Return: JSON with each criterion, status, and evidence."
`+"```"+`

---

### Phase 6: Report Completion

**Goal:** Signal completion with a summary of what was done.

**Before reporting, verify:**
- [ ] All tests pass
- [ ] No dead code added (verified with grep)
- [ ] No test-only helpers added
- [ ] All acceptance criteria met with evidence
- [ ] Code follows project conventions

**Report using EXACTLY ONE tool call:**
`+"```"+`
report_implementation_complete(
    summary="[What you implemented]. Tests: [X passing]. Acceptance: [Y/Y criteria met]. Files changed: [list key files]."
)
`+"```"+`

⚠️ This is your ONLY completion action. Do NOT also call fabric_send - the tool already notifies the coordinator.

**Example:**
`+"```"+`
report_implementation_complete(
    summary="Added ShardKey parameter to Transaction interface and updated 15 call sites. Tests: 47 passing. Acceptance: 6/6 criteria met. Files: repo/interface.go, repo/impl.go, 13 test files updated."
)
`+"```"+`

**Fallback if tool errors:**
If report_implementation_complete fails (e.g., "process is not in implementing phase"), use fabric_reply to your task thread instead:
`+"```"+`
fabric_reply(message_id="%s", content="Implementation complete: [summary]")
`+"```"+`
Never silently fail - always report completion somehow.`, taskID, title, threadID, taskID, threadID)

	if summary != "" {
		prompt += fmt.Sprintf(`

---

## Coordinator Instructions

%s`, summary)
	}

	return prompt
}

// ReviewAssignmentPrompt generates the prompt sent to a reviewer when assigning a code review.
func ReviewAssignmentPrompt(taskID, implementerID string) string {
	return fmt.Sprintf(`[REVIEW ASSIGNMENT]

You are being assigned to **review** the work completed by %s on task **%s**.

## Your Review Process

### Step 1: Gather Context
- Read the task description: bd show %s
- Identify acceptance criteria that must be verified
- Get the git diff to understand what changed

### Step 2: Spawn Parallel Review Sub-Agents

Spawn 4 sub-agents in parallel, each focused on one review dimension. Each sub-agent MUST return findings in the standardized JSON format below.

---

**SUB-AGENT 1: Correctness & Logic Review**

Focus exclusively on:
- Logic errors: Incorrect conditionals, off-by-one errors, wrong operators
- Edge cases: Nil/null handling, empty collections, boundary conditions
- Error handling: Missing error checks, swallowed errors, incorrect propagation
- Control flow: Unreachable code, infinite loops, missing returns

Return findings in this format:
`+"```json"+`
{
  "reviewer": "correctness",
  "confidence": 0.85,
  "verdict": "pass|fail|warning",
  "findings": [
    {
      "severity": "blocker|major|minor|info",
      "category": "logic|edge-case|error-handling|control-flow",
      "location": "file.go:42",
      "problem": "Description of the issue",
      "evidence": "How you verified this is an issue",
      "suggested_fix": "How to fix it"
    }
  ],
  "summary": "One-line summary"
}
`+"```"+`

---

**SUB-AGENT 2: Test Coverage Review**

Focus exclusively on:
- Test execution: Actually run the tests - do they pass?
- Coverage: Are the changes adequately tested?
- Mock correctness: Are mocks set up properly? Correct expectations?
- Assertion quality: Do tests actually verify behavior?

CRITICAL: You MUST run the tests, not just read them.

Return findings in this format:
`+"```json"+`
{
  "reviewer": "tests",
  "confidence": 0.90,
  "verdict": "pass|fail|warning",
  "findings": [
    {
      "severity": "blocker|major|minor|info",
      "category": "execution|coverage|mocks|assertions",
      "location": "file_test.go:42",
      "problem": "Test fails with nil pointer panic",
      "evidence": "go test output: panic: runtime error: invalid memory address",
      "suggested_fix": "Initialize the struct before using"
    }
  ],
  "summary": "One-line summary"
}
`+"```"+`

---

**SUB-AGENT 3: Dead Code Review**

Focus exclusively on:
- Dead code: Functions/methods never called from production code
- Test-only helpers: Methods that only exist to support tests (BLOCKER - always wrong)
- Unused exports: Public functions/types that nothing uses
- Orphaned code: Code left behind from refactoring

Use grep to verify usage claims - don't guess.

Return findings in this format:
`+"```json"+`
{
  "reviewer": "dead-code",
  "confidence": 0.80,
  "verdict": "pass|fail|warning",
  "findings": [
    {
      "severity": "blocker|major|minor|info",
      "category": "test-only-helper|unused-function|orphaned|complexity",
      "location": "file.go:42",
      "problem": "Method Focused() is only called from test files",
      "evidence": "grep -rn 'Focused()' --include='*.go' | grep -v '_test.go' shows only the definition",
      "suggested_fix": "Remove the method. Tests should verify behavior through public interface."
    }
  ],
  "summary": "One-line summary"
}
`+"```"+`

---

**SUB-AGENT 4: Acceptance Criteria Verification**

Focus exclusively on:
- Each checkbox item in the task description
- Running any verification commands exactly as specified
- Documenting evidence for each criterion

Return findings in this format:
`+"```json"+`
{
  "reviewer": "acceptance",
  "confidence": 0.95,
  "verdict": "pass|fail|warning",
  "findings": [
    {
      "severity": "blocker|major|minor|info",
      "category": "implementation|testing|verification|documentation",
      "location": "criterion text or file:line",
      "problem": "Criterion not met: Interface signature not updated",
      "evidence": "interface.go still has old signature",
      "suggested_fix": "Update interface signature to include context.Context"
    }
  ],
  "criteria_summary": {
    "total": 6,
    "met": 5,
    "not_met": 1,
    "details": [
      {"criterion": "Description", "status": "met|not_met", "evidence": "..."}
    ]
  },
  "summary": "5/6 acceptance criteria met"
}
`+"```"+`

---

### Step 3: Synthesize Findings

After all sub-agents return, aggregate their findings:

1. **Parse all findings** - Handle missing or failed reviewers
2. **Categorize by severity** - Blockers > Majors > Minors > Info
3. **Deduplicate** - Same issue caught by multiple reviewers
4. **Resolve conflicts** - Tests failing always wins (DENY)

**DENY if ANY of:**
- Any blocker severity finding
- Tests reviewer verdict is "fail"
- Acceptance criteria not met
- Multiple major findings

**APPROVE if:**
- No blockers
- Tests pass
- Acceptance criteria met
- No more than minor issues

### Step 4: Report Your Verdict 

**Goal** report your verdict as the **LAST ACTION** you take when ending your turn.

This is the final step of your assignment you call this tool when you are done with your work not while doing your work.
You only call this once during your turn unless the coordinator assigns a re-review then you call it once at the end of that turn.
After you call this tool your turn ends and your in an idle state until the coordinator messages you.

Use report_review_verdict with structured comments:

report_review_verdict(
    verdict="APPROVED|DENIED",
    comments="## Summary\n[1-2 sentence overview]\n\n## Sub-Reviewer Results\n| Reviewer | Verdict | Confidence | Summary |\n|----------|---------|------------|----------|\n| Correctness | PASS | 0.85 | ... |\n| Tests | PASS | 0.90 | ... |\n| Dead Code | PASS | 0.80 | ... |\n| Acceptance | PASS | 0.95 | 6/6 met |\n\n## Aggregate Findings\nBlockers: 0 | Majors: 0 | Minors: 2 | Info: 3\n\n## Issues (if any)\n[List issues by severity with location and fix]\n\n## Required Changes (if DENIED)\n1. [specific actionable feedback]\n2. [specific actionable feedback]"
)`, implementerID, taskID, taskID)
}

// ReviewAssignmentPromptSimple generates a streamlined review prompt for simple changes.
// Unlike ReviewAssignmentPrompt, this does NOT instruct the reviewer to spawn sub-agents.
// The reviewer performs all quality checks directly in a single pass.
func ReviewAssignmentPromptSimple(taskID, implementerID string) string {
	return fmt.Sprintf(`[REVIEW ASSIGNMENT]

You are being assigned to review the work completed by %s on task **%s**.

---

## Step 1: Gather Context

- Read the task description: `+"`bd show %s`"+`
- Get the git diff to see what changed
- Note the acceptance criteria (checkboxes in the task)

---

## Step 2: Quick Review Checklist

Work through each dimension directly:

### ✓ Correctness & Logic
- Any obvious logic errors or typos?
- Edge cases handled (nil checks, empty inputs)?
- Errors propagated correctly (no swallowed errors)?

### ✓ Tests
**CRITICAL: Run the tests - do not just read them. This is mandatory, not optional.**
`+"```bash"+`
go test ./path/to/package -v
`+"```"+`
- Do all tests pass?
- Is the change adequately tested?

### ✓ Dead Code
- Any new functions that aren't called from production code?
- Verify with grep: `+"`grep -rn 'FunctionName' --include='*.go' | grep -v '_test.go'`"+`
- Test-only helpers are ALWAYS wrong - reject if found

### ✓ Acceptance Criteria
- Does the change meet EACH criterion in the task?
- Run any verification commands specified in the task

---

## Step 3: Report Your Verdict

**DENY if ANY of:**
- Tests fail
- Obvious bugs or logic errors
- Acceptance criteria not met
- Dead code or test-only helpers found

**APPROVE if:**
- All tests pass
- Code is correct
- All acceptance criteria met

### Step 4: Report Your Verdict 

**Goal** report your verdict as the **LAST ACTION** you take when ending your turn.

This is the final step of your assignment you call this tool when you are done with your work not while doing your work.
You only call this once during your turn unless the coordinator assigns a re-review then you call it once at the end of that turn.
After you call this tool your turn ends and your in an idle state until the coordinator messages you.

`+"```"+`
report_review_verdict(
    verdict="APPROVED|DENIED",
    comments="Quick review: [1-2 sentence summary]. Tests: PASS/FAIL. Acceptance: X/X met. [If DENIED: specific issues to fix]"
)
`+"```"+``, implementerID, taskID, taskID)
}

// ReviewFeedbackPrompt generates the prompt sent to an implementer when their code was denied.
func ReviewFeedbackPrompt(taskID, feedback string) string {
	return fmt.Sprintf(`[REVIEW FEEDBACK]

Your implementation of task **%s** was **DENIED** during code review.

## Required Changes:
%s

Please address the feedback above and make the necessary changes.

When you have addressed all feedback, report via fabric_reply(content="Ready for re-review on task %s").`, taskID, feedback, taskID)
}

// CommitApprovalPrompt generates the prompt sent to an implementer when their code is approved.
func CommitApprovalPrompt(taskID, commitMessage string) string {
	prompt := fmt.Sprintf(`[COMMIT APPROVED]

Your implementation of task **%s** has been **APPROVED** by the reviewer.

Please create a git commit for your changes.`, taskID)

	if commitMessage != "" {
		prompt += fmt.Sprintf(`

Suggested commit message:
%s`, commitMessage)
	}

	prompt += fmt.Sprintf(`

## After Committing

Please document your work using post_accountability_summary. This helps capture valuable learnings and provides accountability for future sessions.

**What to include:**
- **task_id**: The task you completed (required)
- **summary**: What you actually implemented (required)
- **commits**: List of commit hashes you made
- **issues_closed**: Any bd issue IDs you closed
- **issues_discovered**: Any bugs or blockers you found (bd IDs)
- **verification_points**: How you verified acceptance criteria
- **retro**: Structured feedback (went_well, friction, patterns, takeaways)
- **next_steps**: Recommendations for follow-up work

Example:
post_accountability_summary(
    task_id="%s",
    summary="Added validation layer with regex patterns for user input sanitization",
    commits=["abc123"],
    verification_points=["All tests pass", "Manual testing confirms validation works"],
    retro={
        went_well="Using table-driven tests made edge case coverage much easier",
        friction="Initially forgot to handle empty string case, caught by reviewer",
        patterns="This codebase prefers returning errors over panicking for invalid input"
    }
)

Then report via fabric_reply(content="Committed: [hash]").`, taskID)

	return prompt
}

// AggregationWorkerPrompt generates the prompt for a worker assigned to aggregate
// accountability summaries from all workers into a unified session summary.
func AggregationWorkerPrompt(sessionDir string) string {
	return fmt.Sprintf(`[AGGREGATION TASK]

You are assigned to aggregate accountability summaries from all workers into a unified session summary.

## Your Task

1. Read all worker accountability summaries from: %s/workers/*/accountability_summary.md
2. Generate an aggregated summary with all sections A-H
3. Write the result to: %s/accountability_summary.md

## Aggregated Summary Format (Sections A-H)

### A. Session Metrics
- Total Commits Made: [aggregate count with descriptions from all workers]
- Issues Closed: [combined count with verification]
- Issues Discovered: [combined count with bd IDs]

### B. Needs Your Attention
- Any decisions required or human verification items from workers

### C. Progress
- Visual progress tree showing epic/task completion status

### D. What Was Accomplished This Session
- Narrative combining all worker accomplishments, framed by epic progress
- Attribute contributions to specific workers where relevant

### E. Issues Discovered
- Combined list of bugs/blockers found by workers during execution

### F. Issues Closed (Verification)
- Key verification points for each closed issue from all workers

### G. Retro Feedback
- **What Went Well**: Combined insights from all workers
- **Friction**: Problems encountered during execution
- **Patterns Noticed**: Recurring themes or approaches
- **Takeaways**: Key learnings for future sessions

### H. Next Steps
- Combined recommendations from workers for follow-up work

## Instructions

1. Read each worker's accountability_summary.md file
2. Merge metrics (deduplicate commits/issues lists)
3. Combine narratives with worker attribution
4. Preserve all verification points
5. Write the aggregated summary in markdown format

When complete, report via fabric_reply(content="Aggregation complete").`, sessionDir, sessionDir)
}
