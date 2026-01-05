// Package handler provides command handlers for the v2 orchestration architecture.
// This file contains unified process handlers that work for both coordinator and worker
// roles, with role-specific branching only where necessary (spawn and replace operations).
package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// ===========================================================================
// Process Interface
// ===========================================================================

// Process represents a running AI process that can be controlled.
// This mirrors the essential methods from client.HeadlessProcess.
type Process interface {
	// SessionRef returns the session reference (session ID).
	SessionRef() string
	// IsRunning returns true if the process is actively running.
	IsRunning() bool
	// Cancel terminates the process.
	Cancel() error
	// Wait blocks until the process completes.
	Wait() error
}

// ===========================================================================
// ProcessSpawner Interface
// ===========================================================================

// ProcessSpawner is responsible for spawning AI processes.
// Implementations must be thread-safe.
type ProcessSpawner interface {
	// Spawn creates a new AI process for the given process ID.
	// Returns the process or an error if spawning fails.
	Spawn(ctx context.Context, processID string) (Process, error)
}

// ===========================================================================
// UnifiedProcessSpawner creates new AI processes for the unified architecture.
// Used by SpawnProcessHandler and ReplaceProcessHandler for role-aware spawning.
type UnifiedProcessSpawner interface {
	// SpawnProcess creates and starts a new AI process.
	// Returns the created process.Process instance.
	SpawnProcess(ctx context.Context, id string, role repository.ProcessRole) (*process.Process, error)
}

// ===========================================================================
// SendToProcessHandler
// ===========================================================================

// SendToProcessHandler handles CmdSendToProcess commands.
// It implements queue-or-deliver logic identically for both coordinator and workers:
// - If process is Working: queue message, emit ProcessQueueChanged
// - If process is Ready: queue message, return DeliverProcessQueuedCommand follow-up
type SendToProcessHandler struct {
	processRepo repository.ProcessRepository
	queueRepo   repository.QueueRepository
}

// NewSendToProcessHandler creates a new SendToProcessHandler.
func NewSendToProcessHandler(
	processRepo repository.ProcessRepository,
	queueRepo repository.QueueRepository,
) *SendToProcessHandler {
	return &SendToProcessHandler{
		processRepo: processRepo,
		queueRepo:   queueRepo,
	}
}

// Handle processes a SendToProcessCommand.
// Same queue-or-deliver logic for both coordinator and workers.
func (h *SendToProcessHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	sendCmd := cmd.(*command.SendToProcessCommand)

	// Get process from repository
	proc, err := h.processRepo.Get(sendCmd.ProcessID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get process: %w", err)
	}

	// Check if process is retired
	if proc.Status == repository.StatusRetired {
		return nil, ErrProcessRetired
	}

	// Determine sender based on command source
	sender := repository.SenderUser
	if sendCmd.Source() == command.SourceMCPTool {
		sender = repository.SenderCoordinator
	}

	// Always enqueue the message first - queue is the single path for all messages
	queue := h.queueRepo.GetOrCreate(sendCmd.ProcessID)
	if err := queue.Enqueue(sendCmd.Content, sender); err != nil {
		return nil, fmt.Errorf("failed to enqueue message: %w", err)
	}

	// Build result
	result := &SendToProcessResult{
		ProcessID: proc.ID,
		QueueSize: queue.Size(),
	}

	// If process is Working, just emit queue changed event - delivery happens when turn completes
	if proc.Status == repository.StatusWorking {
		result.Queued = true
		event := events.ProcessEvent{
			Type:       events.ProcessQueueChanged,
			ProcessID:  proc.ID,
			Role:       proc.Role,
			Status:     proc.Status,
			QueueCount: queue.Size(),
		}
		return SuccessWithEvents(result, event), nil
	}

	// Process is Ready - create follow-up to deliver from queue
	result.Queued = false // Will be delivered via follow-up
	deliverCmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, sendCmd.ProcessID)
	if sendCmd.TraceID() != "" {
		deliverCmd.SetTraceID(sendCmd.TraceID())
	}

	return SuccessWithFollowUp(result, deliverCmd), nil
}

// SendToProcessResult contains the result of sending a message to a process.
type SendToProcessResult struct {
	ProcessID string
	Queued    bool // True if message was queued, false if will be delivered via follow-up
	QueueSize int
}

// ===========================================================================
// DeliverProcessQueuedHandler
// ===========================================================================

// DeliverProcessQueuedHandler handles CmdDeliverProcessQueued commands.
// It dequeues and delivers pending messages to a process.
// Same logic for both coordinator and workers.
type DeliverProcessQueuedHandler struct {
	processRepo repository.ProcessRepository
	queueRepo   repository.QueueRepository
	registry    *process.ProcessRegistry
	deliverer   MessageDeliverer
	enforcer    TurnCompletionEnforcer
}

// DeliverProcessQueuedHandlerOption configures DeliverProcessQueuedHandler.
type DeliverProcessQueuedHandlerOption func(*DeliverProcessQueuedHandler)

// WithProcessDeliverer sets the message deliverer for the handler.
func WithProcessDeliverer(deliverer MessageDeliverer) DeliverProcessQueuedHandlerOption {
	return func(h *DeliverProcessQueuedHandler) {
		h.deliverer = deliverer
	}
}

// WithDeliverTurnEnforcer sets the turn completion enforcer for the handler.
// The enforcer is notified when a message is delivered to reset turn tracking state.
func WithDeliverTurnEnforcer(enforcer TurnCompletionEnforcer) DeliverProcessQueuedHandlerOption {
	return func(h *DeliverProcessQueuedHandler) {
		h.enforcer = enforcer
	}
}

// NewDeliverProcessQueuedHandler creates a new DeliverProcessQueuedHandler.
func NewDeliverProcessQueuedHandler(
	processRepo repository.ProcessRepository,
	queueRepo repository.QueueRepository,
	registry *process.ProcessRegistry,
	opts ...DeliverProcessQueuedHandlerOption,
) *DeliverProcessQueuedHandler {
	h := &DeliverProcessQueuedHandler{
		processRepo: processRepo,
		queueRepo:   queueRepo,
		registry:    registry,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle processes a DeliverProcessQueuedCommand.
// Dequeues the next message and delivers it to the process.
// Same logic for coordinator and workers.
func (h *DeliverProcessQueuedHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	deliverCmd := cmd.(*command.DeliverProcessQueuedCommand)

	// Get process from repository
	proc, err := h.processRepo.Get(deliverCmd.ProcessID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get process: %w", err)
	}

	// Check if process is retired
	if proc.Status == repository.StatusRetired {
		return nil, ErrProcessRetired
	}

	// Get process's queue
	queue := h.queueRepo.GetOrCreate(deliverCmd.ProcessID)

	// Dequeue next message
	entry, ok := queue.Dequeue()
	if !ok {
		// Queue is empty - return error
		return nil, ErrQueueEmpty
	}

	// Process must be Ready to receive delivery
	// If Working, re-enqueue and return (shouldn't happen in normal operation)
	if proc.Status == repository.StatusWorking {
		_ = queue.Enqueue(entry.Content, entry.Sender)
		result := &DeliverProcessQueuedResult{
			ProcessID:  proc.ID,
			Delivered:  false,
			QueueEmpty: false,
			QueueSize:  queue.Size(),
		}
		return SuccessResult(result), nil
	}

	// Update process status to Working
	proc.Status = repository.StatusWorking
	if err := h.processRepo.Save(proc); err != nil {
		// Re-enqueue on failure (preserve sender)
		_ = queue.Enqueue(entry.Content, entry.Sender)
		return nil, fmt.Errorf("failed to update process status: %w", err)
	}

	// Attempt actual delivery if deliverer is configured
	if h.deliverer != nil {
		if err := h.deliverer.Deliver(ctx, proc.ID, entry.Content); err != nil {
			// Revert process status on delivery failure (preserve sender)
			proc.Status = repository.StatusReady
			_ = h.processRepo.Save(proc)
			_ = queue.Enqueue(entry.Content, entry.Sender)
			return nil, fmt.Errorf("failed to deliver message: %w", err)
		}
	}

	// Reset turn tracking for the new turn, UNLESS this is an enforcement reminder.
	// Enforcement reminders (SenderSystem) continue the same turn, so we preserve
	// the retry count and other state.
	// For normal messages (SenderUser, SenderCoordinator), we start a fresh turn.
	if h.enforcer != nil && entry.Sender != repository.SenderSystem {
		h.enforcer.ResetTurn(proc.ID)
	}

	// Build events
	var resultEvents []any

	// Emit ProcessWorking event
	workingEvent := events.ProcessEvent{
		Type:      events.ProcessWorking,
		ProcessID: proc.ID,
		Role:      proc.Role,
		Status:    events.ProcessStatusWorking,
		TaskID:    proc.TaskID,
	}
	resultEvents = append(resultEvents, workingEvent)

	// Emit ProcessIncoming event with the message
	incomingEvent := events.ProcessEvent{
		Type:      events.ProcessIncoming,
		ProcessID: proc.ID,
		Role:      proc.Role,
		Message:   entry.Content,
		Sender:    string(entry.Sender),
		TaskID:    proc.TaskID,
	}
	resultEvents = append(resultEvents, incomingEvent)

	// Emit ProcessQueueChanged event so TUI updates the queue badge
	queueChangedEvent := events.ProcessEvent{
		Type:       events.ProcessQueueChanged,
		ProcessID:  proc.ID,
		Role:       proc.Role,
		Status:     proc.Status,
		QueueCount: queue.Size(),
	}
	resultEvents = append(resultEvents, queueChangedEvent)

	result := &DeliverProcessQueuedResult{
		ProcessID:  proc.ID,
		Delivered:  true,
		Content:    entry.Content,
		QueueEmpty: queue.IsEmpty(),
		QueueSize:  queue.Size(),
	}

	return SuccessWithEvents(result, resultEvents...), nil
}

// DeliverProcessQueuedResult contains the result of delivering queued messages.
type DeliverProcessQueuedResult struct {
	ProcessID  string
	Delivered  bool   // True if a message was delivered
	Content    string // The delivered message content (if Delivered is true)
	QueueEmpty bool   // True if queue is now empty
	QueueSize  int    // Remaining queue size after delivery
}

// ===========================================================================
// ProcessTurnCompleteHandler
// ===========================================================================

// ProcessTurnCompleteHandler handles CmdProcessTurnComplete commands.
// It processes turn completion, updates the repository, and triggers queue drain.
// Same logic for both coordinator and workers.
// For workers, it also enforces turn completion by checking if required tools were called.
type ProcessTurnCompleteHandler struct {
	processRepo repository.ProcessRepository
	queueRepo   repository.QueueRepository
	enforcer    TurnCompletionEnforcer
}

// ProcessTurnCompleteHandlerOption configures ProcessTurnCompleteHandler.
type ProcessTurnCompleteHandlerOption func(*ProcessTurnCompleteHandler)

// WithProcessTurnEnforcer sets the turn completion enforcer for the handler.
// The enforcer checks if workers called required tools during their turn
// and sends reminders if not.
func WithProcessTurnEnforcer(enforcer TurnCompletionEnforcer) ProcessTurnCompleteHandlerOption {
	return func(h *ProcessTurnCompleteHandler) {
		h.enforcer = enforcer
	}
}

// NewProcessTurnCompleteHandler creates a new ProcessTurnCompleteHandler.
func NewProcessTurnCompleteHandler(
	processRepo repository.ProcessRepository,
	queueRepo repository.QueueRepository,
	opts ...ProcessTurnCompleteHandlerOption,
) *ProcessTurnCompleteHandler {
	h := &ProcessTurnCompleteHandler{
		processRepo: processRepo,
		queueRepo:   queueRepo,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle processes a ProcessTurnCompleteCommand.
// Updates process status to Ready and triggers queue drain if needed.
// Same logic for coordinator and workers.
// For workers, enforces turn completion by checking if required tools were called.
func (h *ProcessTurnCompleteHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	turnCmd := cmd.(*command.ProcessTurnCompleteCommand)

	// Get process from repository
	proc, err := h.processRepo.Get(turnCmd.ProcessID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get process: %w", err)
	}

	// Handle idempotency: if process is already Retired, just return success
	if proc.Status == repository.StatusRetired {
		return SuccessResult(&ProcessTurnCompleteResult{
			ProcessID: proc.ID,
			NewStatus: repository.StatusRetired,
			WasNoOp:   true,
		}), nil
	}

	// ===========================================================================
	// Turn completion enforcement for workers
	// ===========================================================================
	// Check exemptions FIRST (workers only, coordinators are never enforced)
	if proc.Role == repository.RoleWorker && h.enforcer != nil {
		// Skip if turn failed (crash, error, context exceeded)
		if !turnCmd.Succeeded {
			// Skip enforcement - process had an error
		} else if h.enforcer.IsNewlySpawned(proc.ID) {
			// Skip enforcement - startup turn (workers call signal_ready on first turn)
		} else {
			// Check tool calls
			missingTools := h.enforcer.CheckTurnCompletion(proc.ID, proc.Role)
			if len(missingTools) > 0 {
				if h.enforcer.ShouldRetry(proc.ID) {
					// Increment retry counter
					h.enforcer.IncrementRetry(proc.ID)

					// Generate reminder message
					reminder := h.enforcer.GetReminderMessage(proc.ID, missingTools)

					// Enqueue the reminder directly to the queue
					queue := h.queueRepo.GetOrCreate(proc.ID)
					if err := queue.Enqueue(reminder, repository.SenderSystem); err != nil {
						return nil, fmt.Errorf("failed to enqueue enforcement reminder: %w", err)
					}

					// Transition to Ready so DeliverProcessQueuedCommand can deliver the reminder
					proc.Status = repository.StatusReady
					proc.LastActivityAt = time.Now()

					// Update metrics if provided
					if turnCmd.Metrics != nil {
						proc.Metrics = turnCmd.Metrics
					}

					if err := h.processRepo.Save(proc); err != nil {
						return nil, fmt.Errorf("failed to save process: %w", err)
					}

					// Build events - only emit token usage, NOT ProcessReady (to avoid UI confusion)
					var resultEvents []any
					if turnCmd.Metrics != nil {
						tokenEvent := events.ProcessEvent{
							Type:      events.ProcessTokenUsage,
							ProcessID: proc.ID,
							Role:      proc.Role,
							Metrics:   turnCmd.Metrics,
							TaskID:    proc.TaskID,
						}
						resultEvents = append(resultEvents, tokenEvent)
					}

					// Create DeliverProcessQueuedCommand to deliver the reminder
					deliverCmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, proc.ID)
					if turnCmd.TraceID() != "" {
						deliverCmd.SetTraceID(turnCmd.TraceID())
					}

					result := &ProcessTurnCompleteResult{
						ProcessID:            proc.ID,
						NewStatus:            repository.StatusReady, // Ready, pending reminder delivery
						QueuedDelivery:       true,
						WasNoOp:              false,
						EnforcementTriggered: true,
					}

					return SuccessWithEventsAndFollowUp(result, resultEvents, []command.Command{deliverCmd}), nil
				}
				// Max retries exceeded - log warning and allow turn to complete
				h.enforcer.OnMaxRetriesExceeded(proc.ID, missingTools)
			}
		}
	}
	// ===========================================================================
	// End of turn completion enforcement
	// ===========================================================================

	// ===========================================================================
	// Startup failure handling (coordinator and workers)
	// ===========================================================================
	// First turn failed before any success → terminal failure
	// This prevents continuing when a process never established a session.
	if !proc.HasCompletedTurn && !turnCmd.Succeeded {
		proc.Status = repository.StatusFailed
		proc.LastActivityAt = time.Now()
		// Keep HasCompletedTurn=false since we never succeeded

		if err := h.processRepo.Save(proc); err != nil {
			return nil, fmt.Errorf("failed to save process: %w", err)
		}

		// Build error event for consumers (Initializer, etc.)
		evErr := turnCmd.Error
		if evErr == nil {
			evErr = fmt.Errorf("process %s first turn failed; exited before establishing a session", proc.ID)
		}

		var resultEvents []any
		errorEvent := events.ProcessEvent{
			Type:      events.ProcessError,
			ProcessID: proc.ID,
			Role:      proc.Role,
			Status:    events.ProcessStatusFailed,
			Error:     evErr,
		}
		resultEvents = append(resultEvents, errorEvent)

		// Include token usage if available
		if turnCmd.Metrics != nil {
			tokenEvent := events.ProcessEvent{
				Type:      events.ProcessTokenUsage,
				ProcessID: proc.ID,
				Role:      proc.Role,
				Metrics:   turnCmd.Metrics,
			}
			resultEvents = append(resultEvents, tokenEvent)
		}

		result := &ProcessTurnCompleteResult{
			ProcessID: proc.ID,
			NewStatus: repository.StatusFailed,
			WasNoOp:   false,
		}

		return SuccessWithEvents(result, resultEvents...), nil
	}
	// ===========================================================================
	// End of startup failure handling
	// ===========================================================================

	// Mark successful turn completion
	if turnCmd.Succeeded && !proc.HasCompletedTurn {
		proc.HasCompletedTurn = true
	}

	// Update process state - same for coordinator and workers
	// Always transition to Ready (even if succeeded=false after first success, we don't auto-retire)
	proc.Status = repository.StatusReady
	proc.LastActivityAt = time.Now()

	// Update metrics if provided
	if turnCmd.Metrics != nil {
		proc.Metrics = turnCmd.Metrics
	}

	if err := h.processRepo.Save(proc); err != nil {
		return nil, fmt.Errorf("failed to save process: %w", err)
	}

	// Build events
	var resultEvents []any

	// Emit ProcessReady event
	readyEvent := events.ProcessEvent{
		Type:      events.ProcessReady,
		ProcessID: proc.ID,
		Role:      proc.Role,
		Status:    events.ProcessStatusReady,
		TaskID:    proc.TaskID,
	}
	resultEvents = append(resultEvents, readyEvent)

	// Emit ProcessTokenUsage if metrics provided
	if turnCmd.Metrics != nil {
		tokenEvent := events.ProcessEvent{
			Type:      events.ProcessTokenUsage,
			ProcessID: proc.ID,
			Role:      proc.Role,
			Metrics:   turnCmd.Metrics,
			TaskID:    proc.TaskID,
		}
		resultEvents = append(resultEvents, tokenEvent)
	}

	// Check for queued messages - same logic for both roles
	var followUps []command.Command
	queue := h.queueRepo.GetOrCreate(proc.ID)
	if !queue.IsEmpty() {
		deliverCmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, proc.ID)
		if turnCmd.TraceID() != "" {
			deliverCmd.SetTraceID(turnCmd.TraceID())
		}
		followUps = append(followUps, deliverCmd)
	}

	result := &ProcessTurnCompleteResult{
		ProcessID:      proc.ID,
		NewStatus:      repository.StatusReady,
		QueuedDelivery: len(followUps) > 0,
		WasNoOp:        false,
	}

	return SuccessWithEventsAndFollowUp(result, resultEvents, followUps), nil
}

// ProcessTurnCompleteResult contains the result of handling turn completion.
type ProcessTurnCompleteResult struct {
	ProcessID            string
	NewStatus            repository.ProcessStatus
	QueuedDelivery       bool // true if DeliverProcessQueuedCommand was added to follow-ups
	WasNoOp              bool // true if process was already Retired (idempotent)
	EnforcementTriggered bool // true if a turn completion enforcement reminder was sent
}

// ===========================================================================
// RetireProcessHandler
// ===========================================================================

// RetireProcessHandler handles CmdRetireProcess commands.
// It gracefully retires a process. Same logic for coordinator and workers.
type RetireProcessHandler struct {
	processRepo repository.ProcessRepository
	registry    *process.ProcessRegistry
	enforcer    TurnCompletionEnforcer
}

// RetireProcessHandlerOption configures RetireProcessHandler.
type RetireProcessHandlerOption func(*RetireProcessHandler)

// WithRetireTurnEnforcer sets the turn completion enforcer for the handler.
// The enforcer is notified when a process is retired to clean up tracking state.
func WithRetireTurnEnforcer(enforcer TurnCompletionEnforcer) RetireProcessHandlerOption {
	return func(h *RetireProcessHandler) {
		h.enforcer = enforcer
	}
}

// NewRetireProcessHandler creates a new RetireProcessHandler.
func NewRetireProcessHandler(
	processRepo repository.ProcessRepository,
	registry *process.ProcessRegistry,
	opts ...RetireProcessHandlerOption,
) *RetireProcessHandler {
	h := &RetireProcessHandler{
		processRepo: processRepo,
		registry:    registry,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle processes a RetireProcessCommand.
// Updates status to Retired and stops the process in registry.
// Same logic for coordinator and workers.
func (h *RetireProcessHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	retireCmd := cmd.(*command.RetireProcessCommand)

	// Get process from repository
	proc, err := h.processRepo.Get(retireCmd.ProcessID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get process: %w", err)
	}

	// Handle idempotency: if already retired, return success
	if proc.Status == repository.StatusRetired {
		return SuccessResult(&RetireProcessResult{
			ProcessID: proc.ID,
			WasNoOp:   true,
		}), nil
	}

	// Update process status
	proc.Status = repository.StatusRetired
	proc.RetiredAt = time.Now()

	if err := h.processRepo.Save(proc); err != nil {
		return nil, fmt.Errorf("failed to save process: %w", err)
	}

	// Stop the live process in registry
	if h.registry != nil {
		liveProcess := h.registry.Get(retireCmd.ProcessID)
		if liveProcess != nil {
			liveProcess.SetRetired(true)
			liveProcess.Stop()
		}
	}

	// Clean up turn completion tracking state to prevent memory leaks
	if h.enforcer != nil {
		h.enforcer.CleanupProcess(retireCmd.ProcessID)
	}

	// Emit ProcessStatusChange event
	event := events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		ProcessID: proc.ID,
		Role:      proc.Role,
		Status:    events.ProcessStatusRetired,
		TaskID:    proc.TaskID,
	}

	result := &RetireProcessResult{
		ProcessID: proc.ID,
		WasNoOp:   false,
	}

	return SuccessWithEvents(result, event), nil
}

// RetireProcessResult contains the result of retiring a process.
type RetireProcessResult struct {
	ProcessID string
	WasNoOp   bool // true if process was already retired
}

// ===========================================================================
// BroadcastHandler
// ===========================================================================

// BroadcastHandler handles CmdBroadcast commands.
// It sends a message to all active processes by creating SendToWorker follow-up commands.
type BroadcastHandler struct {
	processRepo repository.ProcessRepository
}

// NewBroadcastHandler creates a new BroadcastHandler.
func NewBroadcastHandler(processRepo repository.ProcessRepository) *BroadcastHandler {
	return &BroadcastHandler{
		processRepo: processRepo,
	}
}

// Handle processes a BroadcastCommand.
// Creates SendToProcessCommand follow-ups for each active process not in exclude list.
func (h *BroadcastHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	broadcastCmd := cmd.(*command.BroadcastCommand)

	// Build exclude set for O(1) lookup
	excludeSet := make(map[string]bool, len(broadcastCmd.ExcludeWorkers))
	for _, workerID := range broadcastCmd.ExcludeWorkers {
		excludeSet[workerID] = true
	}

	// Get all active processes
	activeProcesses := h.processRepo.ActiveWorkers()

	// Filter excluded processes and create follow-up commands
	var followUps []command.Command
	var targetWorkerIDs []string

	for _, proc := range activeProcesses {
		// Skip excluded processes
		if excludeSet[proc.ID] {
			continue
		}

		// Create SendToProcessCommand for each target process
		sendCmd := command.NewSendToProcessCommand(command.SourceInternal, proc.ID, broadcastCmd.Content)
		if broadcastCmd.TraceID() != "" {
			sendCmd.SetTraceID(broadcastCmd.TraceID())
		}

		followUps = append(followUps, sendCmd)
		targetWorkerIDs = append(targetWorkerIDs, proc.ID)
	}

	result := &BroadcastResult{
		TargetWorkers:   targetWorkerIDs,
		ExcludedWorkers: broadcastCmd.ExcludeWorkers,
		MessagesSent:    len(followUps),
	}

	return SuccessWithFollowUp(result, followUps...), nil
}

// BroadcastResult contains the result of broadcasting a message.
type BroadcastResult struct {
	TargetWorkers   []string
	ExcludedWorkers []string
	MessagesSent    int
}

// ===========================================================================
// SpawnProcessHandler
// ===========================================================================

// SpawnProcessHandler handles CmdSpawnProcess commands.
// This is one of the two handlers with role-specific branching:
// - Coordinator: uses CoordinatorID, coordinator system prompt/MCP config, enforces singleton
// - Worker: generates unique ID, worker system prompt/MCP config
type SpawnProcessHandler struct {
	processRepo repository.ProcessRepository
	registry    *process.ProcessRegistry
	spawner     UnifiedProcessSpawner
	enforcer    TurnCompletionEnforcer
}

// SpawnProcessHandlerOption configures SpawnProcessHandler.
type SpawnProcessHandlerOption func(*SpawnProcessHandler)

// WithUnifiedSpawner sets the process spawner for unified architecture.
func WithUnifiedSpawner(spawner UnifiedProcessSpawner) SpawnProcessHandlerOption {
	return func(h *SpawnProcessHandler) {
		h.spawner = spawner
	}
}

// WithTurnEnforcer sets the turn completion enforcer for spawn tracking.
// The enforcer is notified when a process is spawned so the first turn is exempt from enforcement.
func WithTurnEnforcer(enforcer TurnCompletionEnforcer) SpawnProcessHandlerOption {
	return func(h *SpawnProcessHandler) {
		h.enforcer = enforcer
	}
}

// NewSpawnProcessHandler creates a new SpawnProcessHandler.
func NewSpawnProcessHandler(
	processRepo repository.ProcessRepository,
	registry *process.ProcessRegistry,
	opts ...SpawnProcessHandlerOption,
) *SpawnProcessHandler {
	h := &SpawnProcessHandler{
		processRepo: processRepo,
		registry:    registry,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle processes a SpawnProcessCommand.
// Role-specific branching for ID generation, constraints, and configuration.
func (h *SpawnProcessHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	spawnCmd := cmd.(*command.SpawnProcessCommand)

	var processID string

	if spawnCmd.Role == repository.RoleCoordinator {
		// Coordinator-specific logic
		processID = repository.CoordinatorID

		// Enforce singleton constraint
		if _, err := h.processRepo.GetCoordinator(); err == nil {
			return nil, ErrCoordinatorExists
		}
	} else {
		// Worker-specific logic
		processID = h.generateWorkerID()
	}

	// Override with provided ProcessID if specified
	if spawnCmd.ProcessID != "" {
		processID = spawnCmd.ProcessID
	}

	// Create process entity
	proc := &repository.Process{
		ID:             processID,
		Role:           spawnCmd.Role,
		Status:         repository.StatusPending,
		CreatedAt:      time.Now(),
		LastActivityAt: time.Now(),
	}

	// Save to repository
	if err := h.processRepo.Save(proc); err != nil {
		return nil, fmt.Errorf("failed to save process: %w", err)
	}

	// Spawn live process if spawner is configured
	var liveProcess *process.Process
	if h.spawner != nil {
		var err error
		liveProcess, err = h.spawner.SpawnProcess(ctx, processID, spawnCmd.Role)
		if err != nil {
			// Rollback repository save - but for in-memory this isn't critical
			return nil, fmt.Errorf("failed to spawn process: %w", err)
		}

		// Register in registry
		if h.registry != nil {
			h.registry.Register(liveProcess)
		}
	}

	// Update status to Working if we spawned a live process (it's running its first turn).
	// Status transitions to Ready when the first turn completes via ProcessTurnCompleteHandler.
	// If no spawner (tests), set to Ready for backward compatibility.
	if liveProcess != nil {
		proc.Status = repository.StatusWorking
	} else {
		proc.Status = repository.StatusReady
	}
	_ = h.processRepo.Save(proc)

	// Mark process as newly spawned for turn completion enforcement exemption.
	// The first turn after spawn is exempt (workers call signal_ready).
	if h.enforcer != nil {
		h.enforcer.MarkAsNewlySpawned(processID)
	}

	// Emit ProcessSpawned event
	event := events.ProcessEvent{
		Type:      events.ProcessSpawned,
		ProcessID: processID,
		Role:      spawnCmd.Role,
		Status:    proc.Status,
	}

	result := &SpawnProcessResult{
		ProcessID: processID,
		Role:      spawnCmd.Role,
	}

	return SuccessWithEvents(result, event), nil
}

// generateWorkerID generates a unique worker ID.
func (h *SpawnProcessHandler) generateWorkerID() string {
	// Find the next available worker number
	workers := h.processRepo.Workers()
	maxNum := 0
	for _, w := range workers {
		var num int
		if _, err := fmt.Sscanf(w.ID, "worker-%d", &num); err == nil {
			if num > maxNum {
				maxNum = num
			}
		}
	}
	return fmt.Sprintf("worker-%d", maxNum+1)
}

// SpawnProcessResult contains the result of spawning a process.
type SpawnProcessResult struct {
	ProcessID string
	Role      repository.ProcessRole
}

// GetProcessID returns the process ID for interface compatibility.
func (r *SpawnProcessResult) GetProcessID() string {
	return r.ProcessID
}

// ===========================================================================
// ReplaceProcessHandler
// ===========================================================================

// ReplaceProcessHandler handles CmdReplaceProcess commands.
// This is one of the two handlers with role-specific branching:
// - Coordinator: context window refresh with handoff prompt
// - Worker: simple retire and spawn replacement
type ReplaceProcessHandler struct {
	processRepo repository.ProcessRepository
	registry    *process.ProcessRegistry
	spawner     UnifiedProcessSpawner
}

// ReplaceProcessHandlerOption configures ReplaceProcessHandler.
type ReplaceProcessHandlerOption func(*ReplaceProcessHandler)

// WithReplaceSpawner sets the process spawner for replacement.
func WithReplaceSpawner(spawner UnifiedProcessSpawner) ReplaceProcessHandlerOption {
	return func(h *ReplaceProcessHandler) {
		h.spawner = spawner
	}
}

// NewReplaceProcessHandler creates a new ReplaceProcessHandler.
func NewReplaceProcessHandler(
	processRepo repository.ProcessRepository,
	registry *process.ProcessRegistry,
	opts ...ReplaceProcessHandlerOption,
) *ReplaceProcessHandler {
	h := &ReplaceProcessHandler{
		processRepo: processRepo,
		registry:    registry,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle processes a ReplaceProcessCommand.
// Role-specific branching for coordinator handoff vs worker restart.
func (h *ReplaceProcessHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	replaceCmd := cmd.(*command.ReplaceProcessCommand)

	// Get process from repository
	proc, err := h.processRepo.Get(replaceCmd.ProcessID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get process: %w", err)
	}

	if proc.IsCoordinator() {
		return h.replaceCoordinator(ctx, proc, replaceCmd.Reason)
	}
	return h.replaceWorker(ctx, proc, replaceCmd.Reason)
}

// replaceCoordinator handles coordinator replacement with context handoff.
func (h *ReplaceProcessHandler) replaceCoordinator(ctx context.Context, proc *repository.Process, _ string) (*command.CommandResult, error) {
	// Stop the old coordinator
	if h.registry != nil {
		oldProcess := h.registry.Get(proc.ID)
		if oldProcess != nil {
			oldProcess.Stop()
		}
	}

	// Mark old as retired
	proc.Status = repository.StatusRetired
	proc.RetiredAt = time.Now()
	if err := h.processRepo.Save(proc); err != nil {
		return nil, fmt.Errorf("failed to retire old coordinator: %w", err)
	}

	// Create new coordinator entity (same ID)
	newProc := &repository.Process{
		ID:             repository.CoordinatorID,
		Role:           repository.RoleCoordinator,
		Status:         repository.StatusPending,
		CreatedAt:      time.Now(),
		LastActivityAt: time.Now(),
	}

	if err := h.processRepo.Save(newProc); err != nil {
		return nil, fmt.Errorf("failed to save new coordinator: %w", err)
	}

	// Spawn new coordinator process
	if h.spawner != nil {
		newLiveProcess, err := h.spawner.SpawnProcess(ctx, repository.CoordinatorID, repository.RoleCoordinator)
		if err != nil {
			return nil, fmt.Errorf("failed to spawn new coordinator: %w", err)
		}

		if h.registry != nil {
			h.registry.Unregister(proc.ID)
			h.registry.Register(newLiveProcess)
		}
	}

	// Update status to Ready (spawner success or no spawner = ready for tests)
	newProc.Status = repository.StatusReady
	_ = h.processRepo.Save(newProc)

	// Emit events
	var resultEvents []any

	// Old coordinator retired
	retiredEvent := events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		ProcessID: proc.ID,
		Role:      events.RoleCoordinator,
		Status:    events.ProcessStatusRetired,
	}
	resultEvents = append(resultEvents, retiredEvent)

	// New coordinator spawned
	spawnedEvent := events.ProcessEvent{
		Type:      events.ProcessSpawned,
		ProcessID: repository.CoordinatorID,
		Role:      events.RoleCoordinator,
		Status:    newProc.Status,
	}
	resultEvents = append(resultEvents, spawnedEvent)

	result := &ReplaceProcessResult{
		OldProcessID: proc.ID,
		NewProcessID: repository.CoordinatorID,
		Role:         repository.RoleCoordinator,
	}

	return SuccessWithEvents(result, resultEvents...), nil
}

// replaceWorker handles worker replacement with simple retire and spawn.
func (h *ReplaceProcessHandler) replaceWorker(ctx context.Context, proc *repository.Process, _ string) (*command.CommandResult, error) {
	// Generate new worker ID
	workers := h.processRepo.Workers()
	maxNum := 0
	for _, w := range workers {
		var num int
		if _, err := fmt.Sscanf(w.ID, "worker-%d", &num); err == nil {
			if num > maxNum {
				maxNum = num
			}
		}
	}
	newWorkerID := fmt.Sprintf("worker-%d", maxNum+1)

	// Stop the old worker
	if h.registry != nil {
		oldProcess := h.registry.Get(proc.ID)
		if oldProcess != nil {
			oldProcess.SetRetired(true)
			oldProcess.Stop()
		}
	}

	// Mark old as retired
	proc.Status = repository.StatusRetired
	proc.RetiredAt = time.Now()
	if err := h.processRepo.Save(proc); err != nil {
		return nil, fmt.Errorf("failed to retire old worker: %w", err)
	}

	// Create new worker entity
	newProc := &repository.Process{
		ID:             newWorkerID,
		Role:           repository.RoleWorker,
		Status:         repository.StatusPending,
		CreatedAt:      time.Now(),
		LastActivityAt: time.Now(),
	}

	if err := h.processRepo.Save(newProc); err != nil {
		return nil, fmt.Errorf("failed to save new worker: %w", err)
	}

	// Spawn new worker process
	if h.spawner != nil {
		newLiveProcess, err := h.spawner.SpawnProcess(ctx, newWorkerID, repository.RoleWorker)
		if err != nil {
			return nil, fmt.Errorf("failed to spawn new worker: %w", err)
		}

		if h.registry != nil {
			h.registry.Unregister(proc.ID)
			h.registry.Register(newLiveProcess)
		}
	}

	// Update status to Ready (spawner success or no spawner = ready for tests)
	newProc.Status = repository.StatusReady
	_ = h.processRepo.Save(newProc)

	// Emit events
	var resultEvents []any

	// Old worker retired
	retiredEvent := events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		ProcessID: proc.ID,
		Role:      events.RoleWorker,
		Status:    events.ProcessStatusRetired,
		TaskID:    proc.TaskID,
	}
	resultEvents = append(resultEvents, retiredEvent)

	// New worker spawned
	spawnedEvent := events.ProcessEvent{
		Type:      events.ProcessSpawned,
		ProcessID: newWorkerID,
		Role:      events.RoleWorker,
		Status:    newProc.Status,
	}
	resultEvents = append(resultEvents, spawnedEvent)

	result := &ReplaceProcessResult{
		OldProcessID: proc.ID,
		NewProcessID: newWorkerID,
		Role:         repository.RoleWorker,
	}

	return SuccessWithEvents(result, resultEvents...), nil
}

// ReplaceProcessResult contains the result of replacing a process.
type ReplaceProcessResult struct {
	OldProcessID string
	NewProcessID string
	Role         repository.ProcessRole
}

// ===========================================================================
// PauseProcessHandler
// ===========================================================================

// PauseProcessHandler handles CmdPauseProcess commands.
// It transitions a process from Ready/Working → Paused.
// Same logic for both coordinator and workers.
type PauseProcessHandler struct {
	processRepo repository.ProcessRepository
}

// NewPauseProcessHandler creates a new PauseProcessHandler.
func NewPauseProcessHandler(processRepo repository.ProcessRepository) *PauseProcessHandler {
	return &PauseProcessHandler{
		processRepo: processRepo,
	}
}

// Handle processes a PauseProcessCommand.
// Transitions Ready/Working → Paused with idempotent handling.
func (h *PauseProcessHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	pauseCmd := cmd.(*command.PauseProcessCommand)

	// Get process from repository
	proc, err := h.processRepo.Get(pauseCmd.ProcessID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get process: %w", err)
	}

	// Handle idempotency: if already paused, return success
	if proc.Status == repository.StatusPaused {
		return SuccessResult(&PauseProcessResult{
			ProcessID: proc.ID,
			WasNoOp:   true,
		}), nil
	}

	// Check if process is in a terminal state
	if proc.Status.IsTerminal() {
		return nil, ErrProcessRetired
	}

	// Only Ready or Working can transition to Paused
	if proc.Status != repository.StatusReady && proc.Status != repository.StatusWorking {
		return nil, fmt.Errorf("cannot pause process in status %s: only ready or working processes can be paused", proc.Status)
	}

	// Update process status
	proc.Status = repository.StatusPaused
	if err := h.processRepo.Save(proc); err != nil {
		return nil, fmt.Errorf("failed to save process: %w", err)
	}

	// Emit ProcessStatusChange event
	event := events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		ProcessID: proc.ID,
		Role:      proc.Role,
		Status:    events.ProcessStatusPaused,
		TaskID:    proc.TaskID,
	}

	result := &PauseProcessResult{
		ProcessID: proc.ID,
		WasNoOp:   false,
	}

	return SuccessWithEvents(result, event), nil
}

// PauseProcessResult contains the result of pausing a process.
type PauseProcessResult struct {
	ProcessID string
	WasNoOp   bool // true if process was already paused
}

// ===========================================================================
// ResumeProcessHandler
// ===========================================================================

// ResumeProcessHandler handles CmdResumeProcess commands.
// It transitions a process from Paused → Ready and triggers queue drain if needed.
// Same logic for both coordinator and workers.
type ResumeProcessHandler struct {
	processRepo repository.ProcessRepository
	queueRepo   repository.QueueRepository
}

// NewResumeProcessHandler creates a new ResumeProcessHandler.
func NewResumeProcessHandler(
	processRepo repository.ProcessRepository,
	queueRepo repository.QueueRepository,
) *ResumeProcessHandler {
	return &ResumeProcessHandler{
		processRepo: processRepo,
		queueRepo:   queueRepo,
	}
}

// Handle processes a ResumeProcessCommand.
// Transitions Paused → Ready with idempotent handling.
// Triggers queue drain if messages pending.
func (h *ResumeProcessHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	resumeCmd := cmd.(*command.ResumeProcessCommand)

	// Get process from repository
	proc, err := h.processRepo.Get(resumeCmd.ProcessID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get process: %w", err)
	}

	// Handle idempotency: if already Ready or Working, return success
	if proc.Status == repository.StatusReady || proc.Status == repository.StatusWorking {
		return SuccessResult(&ResumeProcessResult{
			ProcessID: proc.ID,
			WasNoOp:   true,
		}), nil
	}

	// Check if process is in a terminal state
	if proc.Status.IsTerminal() {
		return nil, ErrProcessRetired
	}

	// Only Paused or Stopped can be resumed
	if proc.Status != repository.StatusPaused && proc.Status != repository.StatusStopped {
		return nil, fmt.Errorf("cannot resume process in status %s: only paused or stopped processes can be resumed", proc.Status)
	}

	// Update process status to Ready
	proc.Status = repository.StatusReady
	proc.LastActivityAt = time.Now()
	if err := h.processRepo.Save(proc); err != nil {
		return nil, fmt.Errorf("failed to save process: %w", err)
	}

	// Emit ProcessStatusChange event
	event := events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		ProcessID: proc.ID,
		Role:      proc.Role,
		Status:    events.ProcessStatusReady,
		TaskID:    proc.TaskID,
	}

	// Check for queued messages - trigger drain if pending
	var followUps []command.Command
	queue := h.queueRepo.GetOrCreate(proc.ID)
	if !queue.IsEmpty() {
		deliverCmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, proc.ID)
		if resumeCmd.TraceID() != "" {
			deliverCmd.SetTraceID(resumeCmd.TraceID())
		}
		followUps = append(followUps, deliverCmd)
	}

	result := &ResumeProcessResult{
		ProcessID:      proc.ID,
		WasNoOp:        false,
		QueuedDelivery: len(followUps) > 0,
	}

	return SuccessWithEventsAndFollowUp(result, []any{event}, followUps), nil
}

// ResumeProcessResult contains the result of resuming a process.
type ResumeProcessResult struct {
	ProcessID      string
	WasNoOp        bool // true if process was already ready/working
	QueuedDelivery bool // true if DeliverProcessQueuedCommand was added to follow-ups
}

// ===========================================================================
// ReplaceCoordinatorHandler
// ===========================================================================

// MessagePoster is responsible for posting messages to the message log.
// Used for handoff protocol during coordinator replacement.
type MessagePoster interface {
	// PostHandoff posts a system-generated handoff message.
	PostHandoff(content string) error
}

// CoordinatorSpawnerWithPrompt creates a new coordinator AI process with a custom prompt.
// Used for coordinator replacement with handoff prompt.
type CoordinatorSpawnerWithPrompt interface {
	// SpawnCoordinatorWithPrompt creates and starts a coordinator with a custom prompt.
	// Returns the created process.Process instance.
	SpawnCoordinatorWithPrompt(ctx context.Context, prompt string) (*process.Process, error)
}

// ReplaceCoordinatorHandler handles CmdReplaceCoordinator commands.
// It implements the full handoff protocol from v1 coordinator Replace():
//  1. Post handoff message to message log (notify current coordinator)
//  2. Wait for current coordinator to complete current turn
//  3. Build replace prompt via buildReplacePrompt() for context transfer
//  4. Spawn new coordinator with the replace prompt
//  5. Retire old coordinator
type ReplaceCoordinatorHandler struct {
	processRepo   repository.ProcessRepository
	registry      *process.ProcessRegistry
	spawner       CoordinatorSpawnerWithPrompt
	messagePoster MessagePoster
}

// ReplaceCoordinatorHandlerOption configures ReplaceCoordinatorHandler.
type ReplaceCoordinatorHandlerOption func(*ReplaceCoordinatorHandler)

// WithCoordinatorSpawnerWithPrompt sets the coordinator spawner for replacement.
func WithCoordinatorSpawnerWithPrompt(spawner CoordinatorSpawnerWithPrompt) ReplaceCoordinatorHandlerOption {
	return func(h *ReplaceCoordinatorHandler) {
		h.spawner = spawner
	}
}

// WithMessagePoster sets the message poster for handoff messages.
func WithMessagePoster(poster MessagePoster) ReplaceCoordinatorHandlerOption {
	return func(h *ReplaceCoordinatorHandler) {
		h.messagePoster = poster
	}
}

// NewReplaceCoordinatorHandler creates a new ReplaceCoordinatorHandler.
func NewReplaceCoordinatorHandler(
	processRepo repository.ProcessRepository,
	registry *process.ProcessRegistry,
	opts ...ReplaceCoordinatorHandlerOption,
) *ReplaceCoordinatorHandler {
	h := &ReplaceCoordinatorHandler{
		processRepo: processRepo,
		registry:    registry,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle processes a ReplaceCoordinatorCommand.
// Implements the full handoff protocol for coordinator replacement.
func (h *ReplaceCoordinatorHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	replaceCmd := cmd.(*command.ReplaceCoordinatorCommand)

	// Get coordinator from repository
	proc, err := h.processRepo.GetCoordinator()
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get coordinator: %w", err)
	}

	// Check if coordinator is in a terminal state
	if proc.Status.IsTerminal() {
		return nil, ErrProcessRetired
	}

	// Step 1: Post handoff message to message log
	if h.messagePoster != nil {
		handoffContent := "[SYSTEM HANDOFF]\nCoordinator context refresh initiated. "
		if replaceCmd.Reason != "" {
			handoffContent += "Reason: " + replaceCmd.Reason + ". "
		}
		handoffContent += "New coordinator will read this message log to understand current state."
		_ = h.messagePoster.PostHandoff(handoffContent) // Non-fatal: ignore error and continue
	}

	// Step 2: Wait for current coordinator to complete current turn (if working)
	if h.registry != nil {
		liveProcess := h.registry.Get(proc.ID)
		if liveProcess != nil && liveProcess.IsRunning() {
			// Wait for current turn to complete with timeout
			waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			select {
			case <-waitCtx.Done():
				// Timeout - proceed anyway (coordinator might be stuck)
			default:
				// Try to wait for the process to finish its current operation
				_ = liveProcess.Wait() // Non-fatal: process might already be done
			}
		}
	}

	// Step 3: Stop the old coordinator
	if h.registry != nil {
		oldProcess := h.registry.Get(proc.ID)
		if oldProcess != nil {
			oldProcess.Stop()
		}
	}

	// Step 4: Mark old as retired
	proc.Status = repository.StatusRetired
	proc.RetiredAt = time.Now()
	if err := h.processRepo.Save(proc); err != nil {
		return nil, fmt.Errorf("failed to retire old coordinator: %w", err)
	}

	// Step 5: Build replace prompt (using the same logic as v1 buildReplacePrompt)
	replacePrompt := prompt.BuildReplacePrompt()

	// Step 6: Spawn new coordinator with replace prompt
	var newLiveProcess *process.Process
	if h.spawner != nil {
		var err error
		newLiveProcess, err = h.spawner.SpawnCoordinatorWithPrompt(ctx, replacePrompt)
		if err != nil {
			return nil, fmt.Errorf("failed to spawn new coordinator: %w", err)
		}

		// Update registry
		if h.registry != nil {
			h.registry.Unregister(proc.ID)
			h.registry.Register(newLiveProcess)
		}
	}

	// Step 7: Create new coordinator entity (reuse same ID)
	newProc := &repository.Process{
		ID:             repository.CoordinatorID,
		Role:           repository.RoleCoordinator,
		Status:         repository.StatusWorking, // New coordinator starts working (processing replace prompt)
		CreatedAt:      time.Now(),
		LastActivityAt: time.Now(),
	}

	if err := h.processRepo.Save(newProc); err != nil {
		return nil, fmt.Errorf("failed to save new coordinator: %w", err)
	}

	// Emit events
	var resultEvents []any

	// Old coordinator retired
	retiredEvent := events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		ProcessID: proc.ID,
		Role:      events.RoleCoordinator,
		Status:    events.ProcessStatusRetired,
	}
	resultEvents = append(resultEvents, retiredEvent)

	// Notify that coordinator was replaced
	chatEvent := events.ProcessEvent{
		Type:      events.ProcessOutput,
		ProcessID: repository.CoordinatorID,
		Role:      events.RoleCoordinator,
		Output:    "Coordinator replaced with fresh context window",
	}
	resultEvents = append(resultEvents, chatEvent)

	// New coordinator working (processing replace prompt)
	workingEvent := events.ProcessEvent{
		Type:      events.ProcessWorking,
		ProcessID: repository.CoordinatorID,
		Role:      events.RoleCoordinator,
		Status:    events.ProcessStatusWorking,
	}
	resultEvents = append(resultEvents, workingEvent)

	result := &ReplaceCoordinatorResult{
		OldProcessID:  proc.ID,
		NewProcessID:  repository.CoordinatorID,
		ReplacePrompt: replacePrompt,
	}

	return SuccessWithEvents(result, resultEvents...), nil
}

// ReplaceCoordinatorResult contains the result of replacing the coordinator.
type ReplaceCoordinatorResult struct {
	OldProcessID  string
	NewProcessID  string
	ReplacePrompt string // The handoff prompt sent to the new coordinator
}
