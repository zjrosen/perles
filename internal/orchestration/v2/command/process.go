// Package command provides unified process commands for the v2 orchestration architecture.
// These commands work for both coordinator and worker processes, replacing role-specific commands.
package command

import (
	"fmt"

	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt/roles"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// ===========================================================================
// Unified Process Lifecycle Commands
// ===========================================================================

// SpawnProcessCommand creates a new process (worker or coordinator).
// The ProcessID is auto-generated for workers if not specified.
// For coordinator, the ProcessID is always "coordinator".
type SpawnProcessCommand struct {
	*BaseCommand
	Role           repository.ProcessRole // Required: coordinator or worker
	ProcessID      string                 // Optional: specific ID (auto-generated for workers if empty)
	AgentType      roles.AgentType        // Optional: agent specialization (default: generic)
	WorkflowConfig *roles.WorkflowConfig  // Optional: workflow-specific prompt customizations
}

// SpawnProcessOption configures a SpawnProcessCommand.
type SpawnProcessOption func(*SpawnProcessCommand)

// WithAgentType sets the agent type for the spawn command.
// This determines which specialized prompts are used for the worker.
func WithAgentType(agentType roles.AgentType) SpawnProcessOption {
	return func(cmd *SpawnProcessCommand) {
		cmd.AgentType = agentType
	}
}

// WithWorkflowConfig sets the workflow-specific prompt customizations.
// This enables workflow templates to override or append to default system prompts.
func WithWorkflowConfig(config *roles.WorkflowConfig) SpawnProcessOption {
	return func(cmd *SpawnProcessCommand) {
		cmd.WorkflowConfig = config
	}
}

// NewSpawnProcessCommand creates a new SpawnProcessCommand.
// Options can be provided to configure optional fields like AgentType.
func NewSpawnProcessCommand(source CommandSource, role repository.ProcessRole, opts ...SpawnProcessOption) *SpawnProcessCommand {
	base := NewBaseCommand(CmdSpawnProcess, source)
	cmd := &SpawnProcessCommand{
		BaseCommand: &base,
		Role:        role,
		AgentType:   roles.AgentTypeGeneric, // Default to generic
	}
	for _, opt := range opts {
		opt(cmd)
	}
	return cmd
}

// Validate checks that Role is coordinator, worker, or observer.
func (c *SpawnProcessCommand) Validate() error {
	if c.Role != repository.RoleCoordinator && c.Role != repository.RoleWorker && c.Role != repository.RoleObserver {
		return fmt.Errorf("role must be coordinator, worker, or observer, got: %s", c.Role)
	}
	return nil
}

// RetireProcessCommand terminates a process gracefully.
type RetireProcessCommand struct {
	*BaseCommand
	ProcessID string // Required: ID of the process to retire
	Reason    string // Optional: reason for retirement
}

// NewRetireProcessCommand creates a new RetireProcessCommand.
func NewRetireProcessCommand(source CommandSource, processID, reason string) *RetireProcessCommand {
	base := NewBaseCommand(CmdRetireProcess, source)
	return &RetireProcessCommand{
		BaseCommand: &base,
		ProcessID:   processID,
		Reason:      reason,
	}
}

// Validate checks that ProcessID is provided.
func (c *RetireProcessCommand) Validate() error {
	if c.ProcessID == "" {
		return fmt.Errorf("process_id is required")
	}
	return nil
}

// ReplaceProcessCommand retires a process and spawns a replacement with fresh context.
type ReplaceProcessCommand struct {
	*BaseCommand
	ProcessID string // Required: ID of the process to replace
	Reason    string // Optional: reason for replacement
}

// NewReplaceProcessCommand creates a new ReplaceProcessCommand.
func NewReplaceProcessCommand(source CommandSource, processID, reason string) *ReplaceProcessCommand {
	base := NewBaseCommand(CmdReplaceProcess, source)
	return &ReplaceProcessCommand{
		BaseCommand: &base,
		ProcessID:   processID,
		Reason:      reason,
	}
}

// Validate checks that ProcessID is provided.
func (c *ReplaceProcessCommand) Validate() error {
	if c.ProcessID == "" {
		return fmt.Errorf("process_id is required")
	}
	return nil
}

// ===========================================================================
// Unified Process Message Commands
// ===========================================================================

// SendToProcessCommand sends a message to a specific process.
// Works for both coordinator ("coordinator") and workers ("worker-1", etc.).
type SendToProcessCommand struct {
	*BaseCommand
	ProcessID string // Required: ID of the process (e.g., "coordinator", "worker-1")
	Content   string // Required: message content
}

// NewSendToProcessCommand creates a new SendToProcessCommand.
func NewSendToProcessCommand(source CommandSource, processID, content string) *SendToProcessCommand {
	base := NewBaseCommand(CmdSendToProcess, source)
	return &SendToProcessCommand{
		BaseCommand: &base,
		ProcessID:   processID,
		Content:     content,
	}
}

// Validate checks that ProcessID and Content are provided.
func (c *SendToProcessCommand) Validate() error {
	if c.ProcessID == "" {
		return fmt.Errorf("process_id is required")
	}
	if c.Content == "" {
		return fmt.Errorf("content is required")
	}
	return nil
}

// DeliverProcessQueuedCommand delivers queued messages to a process.
// This is an internal command triggered after queue drain decisions.
type DeliverProcessQueuedCommand struct {
	*BaseCommand
	ProcessID string // Required: ID of the process to deliver messages to
}

// NewDeliverProcessQueuedCommand creates a new DeliverProcessQueuedCommand.
func NewDeliverProcessQueuedCommand(source CommandSource, processID string) *DeliverProcessQueuedCommand {
	base := NewBaseCommand(CmdDeliverProcessQueued, source)
	return &DeliverProcessQueuedCommand{
		BaseCommand: &base,
		ProcessID:   processID,
	}
}

// Validate checks that ProcessID is provided.
func (c *DeliverProcessQueuedCommand) Validate() error {
	if c.ProcessID == "" {
		return fmt.Errorf("process_id is required")
	}
	return nil
}

// ===========================================================================
// Unified Process State Commands
// ===========================================================================

// ProcessTurnCompleteCommand signals that a process's AI turn has finished.
// Submitted by Process.handleProcessComplete() when the AI process completes.
// The handler updates the repository (single source of truth) and triggers queue drain.
// Source is always SourceCallback.
type ProcessTurnCompleteCommand struct {
	*BaseCommand
	ProcessID string                // Required: ID of the process whose turn completed
	Succeeded bool                  // true if turn completed normally, false if error/cancelled
	Metrics   *metrics.TokenMetrics // Optional: token usage from this turn
	Error     error                 // Optional: error if turn failed
}

// NewProcessTurnCompleteCommand creates a new ProcessTurnCompleteCommand.
// Source is always SourceCallback since this comes from process event loops.
func NewProcessTurnCompleteCommand(processID string, succeeded bool, m *metrics.TokenMetrics, err error) *ProcessTurnCompleteCommand {
	base := NewBaseCommand(CmdProcessTurnComplete, SourceCallback)
	return &ProcessTurnCompleteCommand{
		BaseCommand: &base,
		ProcessID:   processID,
		Succeeded:   succeeded,
		Metrics:     m,
		Error:       err,
	}
}

// Validate checks that ProcessID is provided.
func (c *ProcessTurnCompleteCommand) Validate() error {
	if c.ProcessID == "" {
		return fmt.Errorf("process_id is required")
	}
	return nil
}
