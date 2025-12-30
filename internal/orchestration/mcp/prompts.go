package mcp

import "fmt"

// WorkerIdlePrompt generates the initial prompt for an idle worker.
// This is sent when spawning a worker that has no task yet.
func WorkerIdlePrompt(workerID string) string {
	return fmt.Sprintf(`You are %s. You are now in IDLE state waiting for task assignment.

Use signal_ready to tell the coordinator you are ready, then STOP.
Do NOT run any other tools. Do NOT check for tasks. Do NOT start any work.

You will receive task assignments from the coordinator in a follow-up message.`, workerID)
}

// WorkerSystemPrompt generates the system prompt for a worker agent.
// This is used as AppendSystemPrompt in Claude config.
func WorkerSystemPrompt(workerID string) string {
	return fmt.Sprintf(`You are %s in orchestration mode.

**WORK CYCLE:**
1. Wait for task assignment from coordinator
2. When assigned a task, work on it thoroughly to completion
3. **MANDATORY**: Use post_message to notify coordinator when done
4. Return to ready state for next task

**Available MCP Tools (use MCPSearch to load them):**
- mcp__perles-worker__check_messages: Check for new messages addressed to you
- mcp__perles-worker__post_message: Send a message to the coordinator (REQUIRED when task complete)
- mcp__perles-worker__signal_ready: Signal that you are ready for task assignment (call on startup)
- mcp__perles-worker__report_implementation_complete: Signal implementation is done (for state-driven workflow)
- mcp__perles-worker__report_review_verdict: Report code review verdict: APPROVED or DENIED (for reviewers)

**HOW TO REPORT COMPLETION:**
1. Call: mcp__perles-worker__post_message(to="COORDINATOR", content="Task completed! [brief summary]")

**CRITICAL RULES:**
- NEVER update the bd task status yourself; coordinator handles that
- NEVER use bd to update tasks
- ALWAYS call post_message when task is complete
- If stuck, use post_message to ask coordinator for help`, workerID)
}

// TaskAssignmentPrompt generates the prompt sent to a worker when assigning a task.
// The summary parameter is optional and provides additional instructions/context from the coordinator.
func TaskAssignmentPrompt(taskID, title, summary string) string {
	prompt := fmt.Sprintf(`[TASK ASSIGNMENT]

**Goal** Complete the task assigned to you with the highest possible quality effort.

Task ID: %s
Title: %s

**IMPORTANT: Before starting, if your tasks parent epic references a proposal document, you must read the full context of the proposal to understand the work.**
Your instructions are in the task description which you can read using the bd tool with "bd show <task-id>" read the full description this is your work and contains
import acceptance criteria to adhere to.`, taskID, title)

	if summary != "" {
		prompt += fmt.Sprintf(`

Coordinator Instructions:
%s`, summary)
	}

	prompt += `

**CRITICAL*: Work on this task thoroughly. When complete, Before committing your changes, use report_implementation_complete to signal you're done.
Example: report_implementation_complete(summary="Implemented feature X with tests")`

	return prompt
}

// ReviewAssignmentPrompt generates the prompt sent to a reviewer when assigning a code review.
func ReviewAssignmentPrompt(taskID, implementerID, summary string) string {
	return fmt.Sprintf(`[REVIEW ASSIGNMENT]

You are being assigned to **review** the work completed by %s on task **%s**.

## What was implemented:
%s

## Your Review Process

### Step 1: Gather Context
- Read the task description using: bd show %s
- Examine the changes made by the implementer
- Check that acceptance criteria are met

### Step 2: Verify the Implementation
- Check for correctness and completeness
- Look for edge cases and error handling
- Verify tests exist and pass (if applicable)
- Check code style and conventions

### Step 3: Report Your Verdict
Use the report_review_verdict tool to submit your verdict:
- **APPROVED**: The implementation is complete and correct
- **DENIED**: Changes are required (include specific feedback in comments)

Example: report_review_verdict(verdict="APPROVED", comments="Code looks good, tests pass")`, implementerID, taskID, summary, taskID)
}

// ReviewFeedbackPrompt generates the prompt sent to an implementer when their code was denied.
func ReviewFeedbackPrompt(taskID, feedback string) string {
	return fmt.Sprintf(`[REVIEW FEEDBACK]

Your implementation of task **%s** was **DENIED** during code review.

## Required Changes:
%s

Please address the feedback above and make the necessary changes.

When you have addressed all feedback, report via post_message to COORDINATOR that you are ready for re-review.`, taskID, feedback)
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

	prompt += `

After committing, report via post_message to COORDINATOR with the commit hash.`

	return prompt
}
