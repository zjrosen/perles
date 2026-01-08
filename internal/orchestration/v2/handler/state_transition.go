// Package handler provides command handlers for the v2 orchestration architecture.
// This file contains handlers for state transition commands: ReportComplete, ReportVerdict, TransitionPhase.
// These are high-risk handlers that manage critical state machine transitions.
// These handlers use the unified ProcessRepository for process state management.
package handler

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/tracing"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/orchestration/v2/types"
)

// ===========================================================================
// Valid Phase Transitions (State Machine)
// ===========================================================================

// ValidTransitions defines the allowed state machine transitions.
// Map key is the "from" phase, value is a slice of valid "to" phases.
var ValidTransitions = map[events.ProcessPhase][]events.ProcessPhase{
	events.ProcessPhaseIdle:               {events.ProcessPhaseImplementing, events.ProcessPhaseReviewing},
	events.ProcessPhaseImplementing:       {events.ProcessPhaseAwaitingReview, events.ProcessPhaseIdle}, // idle on cancel/error
	events.ProcessPhaseAwaitingReview:     {events.ProcessPhaseCommitting, events.ProcessPhaseAddressingFeedback, events.ProcessPhaseIdle},
	events.ProcessPhaseReviewing:          {events.ProcessPhaseIdle},
	events.ProcessPhaseAddressingFeedback: {events.ProcessPhaseAwaitingReview, events.ProcessPhaseIdle},
	events.ProcessPhaseCommitting:         {events.ProcessPhaseIdle},
}

// IsValidTransition checks if transitioning from one phase to another is valid.
func IsValidTransition(from, to events.ProcessPhase) bool {
	validTos, ok := ValidTransitions[from]
	if !ok {
		return false
	}
	return slices.Contains(validTos, to)
}

// ===========================================================================
// ReportCompleteHandler
// ===========================================================================

// ReportCompleteHandler handles CmdReportComplete commands.
// It transitions a process from Implementing to AwaitingReview phase when they report
// their implementation is complete. This handler maintains the callback-before-event
// invariant by checking if the queue has pending messages and creating a DeliverQueued
// follow-up command before the Ready event is emitted.
type ReportCompleteHandler struct {
	processRepo repository.ProcessRepository
	taskRepo    repository.TaskRepository
	queueRepo   repository.QueueRepository
	bdExecutor  beads.BeadsExecutor
}

// ReportCompleteHandlerOption configures ReportCompleteHandler.
type ReportCompleteHandlerOption func(*ReportCompleteHandler)

// WithReportCompleteBDExecutor sets the BD executor for task comments.
// Note: bdExecutor is required and must not be nil.
func WithReportCompleteBDExecutor(executor beads.BeadsExecutor) ReportCompleteHandlerOption {
	return func(h *ReportCompleteHandler) {
		h.bdExecutor = executor
	}
}

// NewReportCompleteHandler creates a new ReportCompleteHandler.
// Panics if bdExecutor is not provided via WithReportCompleteBDExecutor option.
func NewReportCompleteHandler(
	processRepo repository.ProcessRepository,
	taskRepo repository.TaskRepository,
	queueRepo repository.QueueRepository,
	opts ...ReportCompleteHandlerOption,
) *ReportCompleteHandler {
	h := &ReportCompleteHandler{
		processRepo: processRepo,
		taskRepo:    taskRepo,
		queueRepo:   queueRepo,
	}
	for _, opt := range opts {
		opt(h)
	}
	if h.bdExecutor == nil {
		panic("bdExecutor is required for ReportCompleteHandler")
	}
	return h
}

// Handle processes a ReportCompleteCommand.
// Phase transition: Implementing -> AwaitingReview
// Status transition: Working -> Ready (available for messaging)
// CRITICAL: Preserves callback-before-event invariant by checking queue before Ready event.
func (h *ReportCompleteHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	reportCmd := cmd.(*command.ReportCompleteCommand)

	// 1. Get process and validate phase == PhaseImplementing
	proc, err := h.processRepo.Get(reportCmd.WorkerID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get process: %w", err)
	}

	// Check if process is retired
	if proc.Status == repository.StatusRetired {
		return nil, types.ErrProcessRetired
	}

	if proc.Phase == nil || (*proc.Phase != events.ProcessPhaseImplementing && *proc.Phase != events.ProcessPhaseAddressingFeedback) {
		return nil, types.ErrProcessNotImplementing
	}

	// 2. Get task assigned to process
	if proc.TaskID == "" {
		return nil, types.ErrNoTaskAssigned
	}

	task, err := h.taskRepo.Get(proc.TaskID)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			return nil, fmt.Errorf("task not found: %s", proc.TaskID)
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	// Validate process is the implementer
	if task.Implementer != reportCmd.WorkerID {
		return nil, types.ErrProcessNotImplementer
	}

	// 3. Update process: Phase = PhaseAwaitingReview, Status = StatusReady
	awaitingReview := events.ProcessPhaseAwaitingReview
	proc.Phase = &awaitingReview
	proc.Status = repository.StatusReady

	// 4. Update task: Status = TaskInReview
	task.Status = repository.TaskInReview
	task.ReviewStartedAt = time.Now()

	// 5. Save to repositories
	if err := h.taskRepo.Save(task); err != nil {
		return nil, fmt.Errorf("failed to save task: %w", err)
	}

	if err := h.processRepo.Save(proc); err != nil {
		// Revert task changes on failure
		task.Status = repository.TaskImplementing
		task.ReviewStartedAt = time.Time{}
		_ = h.taskRepo.Save(task)
		return nil, fmt.Errorf("failed to save process: %w", err)
	}

	// 6. Check if queue has pending messages -> CmdDeliverProcessQueued follow-up
	// CRITICAL: This preserves the callback-before-event invariant
	var followUps []command.Command
	queue := h.queueRepo.GetOrCreate(reportCmd.WorkerID)
	if !queue.IsEmpty() {
		deliverCmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, reportCmd.WorkerID)
		if reportCmd.TraceID() != "" {
			deliverCmd.SetTraceID(reportCmd.TraceID())
		}
		followUps = append(followUps, deliverCmd)
	}

	// 7. Add comment to bd task synchronously (only if summary provided)
	if reportCmd.Summary != "" {
		comment := fmt.Sprintf("Implementation complete: %s", reportCmd.Summary)
		if err := h.bdExecutor.AddComment(task.TaskID, "coordinator", comment); err != nil {
			return nil, fmt.Errorf("failed to add BD comment: %w", err)
		}
	}

	// 8. Return with ProcessEvent
	event := events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		ProcessID: proc.ID,
		Role:      proc.Role,
		TaskID:    task.TaskID,
		Status:    events.ProcessStatusReady,
		Phase:     proc.Phase,
	}

	result := &ReportCompleteResult{
		WorkerID: proc.ID,
		TaskID:   task.TaskID,
		Summary:  reportCmd.Summary,
	}

	return SuccessWithEventsAndFollowUp(result, []any{event}, followUps), nil
}

// ReportCompleteResult contains the result of reporting implementation complete.
type ReportCompleteResult struct {
	WorkerID string
	TaskID   string
	Summary  string
}

// ===========================================================================
// ReportVerdictHandler
// ===========================================================================

// ReportVerdictHandler handles CmdReportVerdict commands.
// It processes a reviewer's approval or denial verdict and updates both
// reviewer and implementer states accordingly.
type ReportVerdictHandler struct {
	processRepo repository.ProcessRepository
	taskRepo    repository.TaskRepository
	queueRepo   repository.QueueRepository
	bdExecutor  beads.BeadsExecutor
	tracer      trace.Tracer
}

// ReportVerdictHandlerOption configures ReportVerdictHandler.
type ReportVerdictHandlerOption func(*ReportVerdictHandler)

// WithReportVerdictBDExecutor sets the BD executor for task comments.
// Note: bdExecutor is required and must not be nil.
func WithReportVerdictBDExecutor(executor beads.BeadsExecutor) ReportVerdictHandlerOption {
	return func(h *ReportVerdictHandler) {
		h.bdExecutor = executor
	}
}

// WithReportVerdictTracer sets the tracer for span instrumentation.
// If tracer is nil, the handler keeps its default noop tracer.
func WithReportVerdictTracer(tracer trace.Tracer) ReportVerdictHandlerOption {
	return func(h *ReportVerdictHandler) {
		if tracer != nil {
			h.tracer = tracer
		}
	}
}

// NewReportVerdictHandler creates a new ReportVerdictHandler.
// Panics if bdExecutor is not provided via WithReportVerdictBDExecutor option.
func NewReportVerdictHandler(
	processRepo repository.ProcessRepository,
	taskRepo repository.TaskRepository,
	queueRepo repository.QueueRepository,
	opts ...ReportVerdictHandlerOption,
) *ReportVerdictHandler {
	h := &ReportVerdictHandler{
		processRepo: processRepo,
		taskRepo:    taskRepo,
		queueRepo:   queueRepo,
		tracer:      noop.NewTracerProvider().Tracer("noop"),
	}
	for _, opt := range opts {
		opt(h)
	}
	if h.bdExecutor == nil {
		panic("bdExecutor is required for ReportVerdictHandler")
	}
	return h
}

// Handle processes a ReportVerdictCommand.
// If APPROVED: task -> Approved, reviewer -> Idle/Ready
// If DENIED: task -> Denied, reviewer -> Idle/Ready, implementer -> AddressingFeedback
// CRITICAL: Preserves callback-before-event invariant for both reviewer and implementer.
func (h *ReportVerdictHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	verdictCmd := cmd.(*command.ReportVerdictCommand)

	// Create child span for handler-specific tracing
	var span trace.Span
	ctx, span = h.tracer.Start(ctx, tracing.SpanPrefixHandler+"report_verdict",
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	defer span.End()

	// Set handler-specific attributes
	span.SetAttributes(
		attribute.String(tracing.AttrVerdict, string(verdictCmd.Verdict)),
		attribute.String(tracing.AttrReviewerID, verdictCmd.WorkerID),
	)

	// Execute with span error recording
	result, err := h.handleVerdict(ctx, verdictCmd, span)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return result, err
}

// handleVerdict executes the report verdict logic with optional span event recording.
func (h *ReportVerdictHandler) handleVerdict(_ context.Context, verdictCmd *command.ReportVerdictCommand, span trace.Span) (*command.CommandResult, error) {
	// 1. Validate verdict value
	if !verdictCmd.Verdict.IsValid() {
		return nil, types.ErrInvalidVerdict
	}

	// 2. Validate reviewer phase == PhaseReviewing
	reviewer, err := h.processRepo.Get(verdictCmd.WorkerID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get reviewer: %w", err)
	}

	if reviewer.Phase == nil || *reviewer.Phase != events.ProcessPhaseReviewing {
		return nil, types.ErrProcessNotReviewing
	}

	// 3. Get task being reviewed
	if reviewer.TaskID == "" {
		return nil, types.ErrNoTaskAssigned
	}

	task, err := h.taskRepo.Get(reviewer.TaskID)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			return nil, fmt.Errorf("task not found: %s", reviewer.TaskID)
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	// Validate reviewer is the task's reviewer
	if task.Reviewer != verdictCmd.WorkerID {
		return nil, fmt.Errorf("worker is not the reviewer for this task")
	}

	// Get implementer for potential state update
	implementer, err := h.processRepo.Get(task.Implementer)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, fmt.Errorf("implementer not found: %s", task.Implementer)
		}
		return nil, fmt.Errorf("failed to get implementer: %w", err)
	}

	// Update span with additional attributes now that we have the task info
	if span != nil {
		span.SetAttributes(
			attribute.String(tracing.AttrTaskID, task.TaskID),
			attribute.String(tracing.AttrImplementerID, task.Implementer),
		)
	}

	// Build events and follow-ups
	var resultEvents []any
	var followUps []command.Command

	// 4. Handle verdict
	idle := events.ProcessPhaseIdle
	if verdictCmd.Verdict == command.VerdictApproved {
		// APPROVED: task -> Approved, reviewer -> Idle/Ready
		task.Status = repository.TaskApproved
		reviewer.Phase = &idle
		reviewer.Status = repository.StatusReady
		reviewer.TaskID = ""
	} else {
		// DENIED: task -> Denied, reviewer -> Idle/Ready, implementer -> AddressingFeedback
		task.Status = repository.TaskDenied
		task.Reviewer = "" // Clear reviewer so a new one can be assigned for re-review
		reviewer.Phase = &idle
		reviewer.Status = repository.StatusReady
		reviewer.TaskID = ""

		// Update implementer to AddressingFeedback
		addressingFeedback := events.ProcessPhaseAddressingFeedback
		implementer.Phase = &addressingFeedback
		// Implementer stays Working status since they need to address feedback

		// Save implementer
		if err := h.processRepo.Save(implementer); err != nil {
			return nil, fmt.Errorf("failed to save implementer: %w", err)
		}

		// Emit implementer status change event
		implEvent := events.ProcessEvent{
			Type:      events.ProcessStatusChange,
			ProcessID: implementer.ID,
			Role:      implementer.Role,
			TaskID:    task.TaskID,
			Status:    implementer.Status,
			Phase:     implementer.Phase,
		}
		resultEvents = append(resultEvents, implEvent)
	}

	// 5. Save task and reviewer
	if err := h.taskRepo.Save(task); err != nil {
		return nil, fmt.Errorf("failed to save task: %w", err)
	}

	if err := h.processRepo.Save(reviewer); err != nil {
		// Revert task changes on failure
		if verdictCmd.Verdict == command.VerdictApproved {
			task.Status = repository.TaskInReview
		} else {
			task.Status = repository.TaskInReview
		}
		_ = h.taskRepo.Save(task)
		return nil, fmt.Errorf("failed to save reviewer: %w", err)
	}

	// 6. Check queues for processes -> CmdDeliverProcessQueued follow-ups
	// CRITICAL: Preserves callback-before-event invariant
	reviewerQueue := h.queueRepo.GetOrCreate(reviewer.ID)
	if !reviewerQueue.IsEmpty() {
		deliverCmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, reviewer.ID)
		if verdictCmd.TraceID() != "" {
			deliverCmd.SetTraceID(verdictCmd.TraceID())
		}
		followUps = append(followUps, deliverCmd)
	}

	// Emit reviewer status change event
	reviewerEvent := events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		ProcessID: reviewer.ID,
		Role:      reviewer.Role,
		TaskID:    task.TaskID,
		Status:    events.ProcessStatusReady,
		Phase:     reviewer.Phase,
	}
	resultEvents = append(resultEvents, reviewerEvent)

	// 7. Add comment to bd task synchronously
	var comment string
	if verdictCmd.Verdict == command.VerdictApproved {
		comment = fmt.Sprintf("Review APPROVED by %s", verdictCmd.WorkerID)
	} else {
		comment = fmt.Sprintf("Review DENIED by %s: %s", verdictCmd.WorkerID, verdictCmd.Comments)
	}
	if err := h.bdExecutor.AddComment(task.TaskID, "coordinator", comment); err != nil {
		return nil, fmt.Errorf("failed to add BD comment: %w", err)
	}

	result := &ReportVerdictResult{
		ReviewerID:    reviewer.ID,
		TaskID:        task.TaskID,
		Verdict:       verdictCmd.Verdict,
		ImplementerID: task.Implementer,
	}

	return SuccessWithEventsAndFollowUp(result, resultEvents, followUps), nil
}

// ReportVerdictResult contains the result of reporting a review verdict.
type ReportVerdictResult struct {
	ReviewerID    string
	TaskID        string
	Verdict       command.Verdict
	ImplementerID string
}

// ===========================================================================
// TransitionPhaseHandler
// ===========================================================================

// TransitionPhaseHandler handles CmdTransitionPhase commands.
// It provides a generic phase transition mechanism that validates state machine rules.
// This handler is primarily for internal use by other handlers or for direct phase
// manipulation by the coordinator.
type TransitionPhaseHandler struct {
	processRepo repository.ProcessRepository
	queueRepo   repository.QueueRepository
}

// NewTransitionPhaseHandler creates a new TransitionPhaseHandler.
func NewTransitionPhaseHandler(
	processRepo repository.ProcessRepository,
	queueRepo repository.QueueRepository,
) *TransitionPhaseHandler {
	return &TransitionPhaseHandler{
		processRepo: processRepo,
		queueRepo:   queueRepo,
	}
}

// Handle processes a TransitionPhaseCommand.
// Validates the state machine transition and updates the process's phase.
func (h *TransitionPhaseHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	transitionCmd := cmd.(*command.TransitionPhaseCommand)

	// 1. Get process
	proc, err := h.processRepo.Get(transitionCmd.WorkerID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get process: %w", err)
	}

	// 2. Validate transition is allowed
	var oldPhase events.ProcessPhase
	if proc.Phase != nil {
		oldPhase = *proc.Phase
	}
	if !IsValidTransition(oldPhase, transitionCmd.NewPhase) {
		return nil, fmt.Errorf("%w: cannot transition from %s to %s",
			types.ErrInvalidPhaseTransition, oldPhase, transitionCmd.NewPhase)
	}

	// 3. Update process phase
	newPhase := transitionCmd.NewPhase
	proc.Phase = &newPhase

	// Determine new status based on phase
	if transitionCmd.NewPhase == events.ProcessPhaseIdle {
		proc.Status = repository.StatusReady
		proc.TaskID = ""
	}

	// 4. Save process
	if err := h.processRepo.Save(proc); err != nil {
		return nil, fmt.Errorf("failed to save process: %w", err)
	}

	// 5. Build follow-ups for queue drain if transitioning to idle/ready
	var followUps []command.Command
	if proc.Status == repository.StatusReady {
		queue := h.queueRepo.GetOrCreate(proc.ID)
		if !queue.IsEmpty() {
			deliverCmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, proc.ID)
			if transitionCmd.TraceID() != "" {
				deliverCmd.SetTraceID(transitionCmd.TraceID())
			}
			followUps = append(followUps, deliverCmd)
		}
	}

	// 6. Emit status change event
	event := events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		ProcessID: proc.ID,
		Role:      proc.Role,
		Status:    proc.Status,
		Phase:     proc.Phase,
	}

	result := &TransitionPhaseResult{
		WorkerID: proc.ID,
		OldPhase: oldPhase,
		NewPhase: newPhase,
	}

	return SuccessWithEventsAndFollowUp(result, []any{event}, followUps), nil
}

// TransitionPhaseResult contains the result of a phase transition.
type TransitionPhaseResult struct {
	WorkerID string
	OldPhase events.ProcessPhase
	NewPhase events.ProcessPhase
}
