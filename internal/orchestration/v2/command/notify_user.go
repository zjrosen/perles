// Package command provides concrete command types for the v2 orchestration architecture.
package command

import "fmt"

// ===========================================================================
// User Interaction Commands
// ===========================================================================

// NotifyUserCommand requests user attention for a human checkpoint.
// This is used in DAG workflows when a phase requires human review or input.
type NotifyUserCommand struct {
	*BaseCommand
	Message string // Required: message to display to the user
	Phase   string // Optional: phase name (e.g., "clarification-review")
	TaskID  string // Optional: task ID associated with this notification
}

// NewNotifyUserCommand creates a new NotifyUserCommand.
func NewNotifyUserCommand(source CommandSource, message, phase, taskID string) *NotifyUserCommand {
	base := NewBaseCommand(CmdNotifyUser, source)
	return &NotifyUserCommand{
		BaseCommand: &base,
		Message:     message,
		Phase:       phase,
		TaskID:      taskID,
	}
}

// Validate checks that Message is provided.
func (c *NotifyUserCommand) Validate() error {
	if c.Message == "" {
		return fmt.Errorf("message is required")
	}
	return nil
}

// String returns a readable representation of the command.
func (c *NotifyUserCommand) String() string {
	if c.Phase != "" {
		return fmt.Sprintf("NotifyUser{phase=%s, message=%q}", c.Phase, truncate(c.Message, 50))
	}
	return fmt.Sprintf("NotifyUser{message=%q}", truncate(c.Message, 50))
}

// truncate shortens a string to maxLen characters, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
