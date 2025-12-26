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

**HOW TO REPORT COMPLETION:**
1. Call: mcp__perles-worker__post_message(to="COORDINATOR", content="Task completed! [brief summary]")

**CRITICAL RULES:**
- NEVER update the bd task status yourself; coordinator handles that
- NEVER use bd to update tasks
- ALWAYS call post_message when task is complete
- If stuck, use post_message to ask coordinator for help`, workerID)
}

// TaskAssignmentPrompt generates the prompt sent to a worker when assigning a task.
func TaskAssignmentPrompt(taskID, title, description, acceptance string) string {
	prompt := fmt.Sprintf(`[TASK ASSIGNMENT]

Task ID: %s
Title: %s

Description:
%s`, taskID, title, description)

	if acceptance != "" {
		prompt += fmt.Sprintf(`

Acceptance Criteria:
%s`, acceptance)
	}

	prompt += `

Work on this task thoroughly. When complete, report via post_message to COORDINATOR.`

	return prompt
}
