// Package handler provides command handlers for the v2 orchestration architecture.
// This file contains handlers for task assignment commands: AssignTask, AssignReview, and ApproveCommit.
// These handlers use the unified ProcessRepository for process state management.
package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/mcp"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/orchestration/v2/types"
)

// ===========================================================================
// AssignTaskHandler
// ===========================================================================

// AssignTaskHandler handles CmdAssignTask commands.
// It assigns a bd task to an idle process, updating both process and task state.
// After updating state, it queues a TaskAssignmentPrompt message to the worker.
type AssignTaskHandler struct {
	processRepo repository.ProcessRepository
	taskRepo    repository.TaskRepository
	queueRepo   repository.QueueRepository
	bdExecutor  beads.BeadsExecutor
}

// AssignTaskHandlerOption configures AssignTaskHandler.
type AssignTaskHandlerOption func(*AssignTaskHandler)

// WithBDExecutor sets the BD executor for task status updates.
// Note: bdExecutor is required and must not be nil.
func WithBDExecutor(executor beads.BeadsExecutor) AssignTaskHandlerOption {
	return func(h *AssignTaskHandler) {
		h.bdExecutor = executor
	}
}

// WithQueueRepository sets the queue repository for message delivery.
func WithQueueRepository(queueRepo repository.QueueRepository) AssignTaskHandlerOption {
	return func(h *AssignTaskHandler) {
		h.queueRepo = queueRepo
	}
}

// NewAssignTaskHandler creates a new AssignTaskHandler.
// Panics if bdExecutor or queueRepo is not provided.
func NewAssignTaskHandler(
	processRepo repository.ProcessRepository,
	taskRepo repository.TaskRepository,
	opts ...AssignTaskHandlerOption,
) *AssignTaskHandler {
	h := &AssignTaskHandler{
		processRepo: processRepo,
		taskRepo:    taskRepo,
	}
	for _, opt := range opts {
		opt(h)
	}
	if h.bdExecutor == nil {
		panic("bdExecutor is required for AssignTaskHandler")
	}
	if h.queueRepo == nil {
		panic("queueRepo is required for AssignTaskHandler")
	}
	return h
}

// Handle processes an AssignTaskCommand.
// It validates the process state, creates a task assignment, and updates both repositories.
// Phase transition: Idle -> Implementing
// Status transition: Ready -> Working
func (h *AssignTaskHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	assignCmd := cmd.(*command.AssignTaskCommand)

	// 1. Get process from repository
	proc, err := h.processRepo.Get(assignCmd.WorkerID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get process: %w", err)
	}

	// 2. Validate process.Status == StatusReady
	if proc.Status != repository.StatusReady {
		return nil, types.ErrProcessNotReady
	}

	// 3. Validate process.Phase == PhaseIdle (nil or Idle)
	if proc.Phase != nil && *proc.Phase != events.ProcessPhaseIdle {
		return nil, types.ErrProcessNotIdle
	}

	// 4. Validate no existing task assigned to process
	if proc.TaskID != "" {
		return nil, types.ErrProcessAlreadyAssigned
	}

	issue, err := h.bdExecutor.ShowIssue(assignCmd.TaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bd issue: %w. did you mean to use send_to_worker", err)
	}
	if issue == nil {
		return nil, fmt.Errorf("bd issue not found: %s. did you mean to use send_to_worker", proc.TaskID)
	}

	// Also check task repo for any task where this process is implementer
	existingTasks, err := h.taskRepo.GetByImplementer(assignCmd.WorkerID)
	if err != nil && !errors.Is(err, repository.ErrTaskNotFound) {
		return nil, fmt.Errorf("failed to check existing tasks: %w", err)
	}
	if len(existingTasks) > 0 {
		return nil, types.ErrProcessAlreadyAssigned
	}

	// 5. Create TaskAssignment with Implementer = workerID
	task := &repository.TaskAssignment{
		TaskID:      assignCmd.TaskID,
		Implementer: assignCmd.WorkerID,
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}

	// 6. Update process: Phase = PhaseImplementing, TaskID = taskID
	// NOTE: We do NOT set StatusWorking here - that happens in DeliverProcessQueuedHandler
	// when the message is actually delivered to the worker
	implementing := events.ProcessPhaseImplementing
	proc.Phase = &implementing
	proc.TaskID = assignCmd.TaskID

	// 7. Save both to repositories
	if err := h.taskRepo.Save(task); err != nil {
		return nil, fmt.Errorf("failed to save task assignment: %w", err)
	}

	if err := h.processRepo.Save(proc); err != nil {
		// Attempt to clean up task on process save failure
		_ = h.taskRepo.Delete(assignCmd.TaskID)
		return nil, fmt.Errorf("failed to save process: %w", err)
	}

	// 8. Update bd task status to in_progress synchronously
	if err := h.bdExecutor.UpdateStatus(assignCmd.TaskID, beads.StatusInProgress); err != nil {
		return nil, fmt.Errorf("failed to update BD task status: %w", err)
	}

	// 9. Queue TaskAssignmentPrompt to the worker
	// The worker will receive instructions to work on the task (from coordinator)
	prompt := mcp.TaskAssignmentPrompt(assignCmd.TaskID, assignCmd.TaskID, assignCmd.Summary)
	queue := h.queueRepo.GetOrCreate(assignCmd.WorkerID)
	if err := queue.Enqueue(prompt, repository.SenderCoordinator); err != nil {
		return nil, fmt.Errorf("failed to queue task prompt: %w", err)
	}

	// 10. Create follow-up command to deliver the queued message
	// DeliverProcessQueuedHandler will set StatusWorking and actually deliver
	deliverCmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, assignCmd.WorkerID)

	// 11. Return with ProcessEvent and follow-up
	// Note: We emit the phase change event, but status change happens in delivery
	event := events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		ProcessID: proc.ID,
		Role:      proc.Role,
		TaskID:    assignCmd.TaskID,
		Status:    proc.Status, // Still Ready at this point
		Phase:     proc.Phase,
	}

	result := &AssignTaskResult{
		WorkerID: proc.ID,
		TaskID:   assignCmd.TaskID,
		Summary:  assignCmd.Summary,
	}

	return SuccessWithEventsAndFollowUp(result, []any{event}, []command.Command{deliverCmd}), nil
}

// AssignTaskResult contains the result of assigning a task to a worker.
type AssignTaskResult struct {
	WorkerID string
	TaskID   string
	Summary  string
}

// ===========================================================================
// AssignReviewHandler
// ===========================================================================

// AssignReviewHandler handles CmdAssignReview commands.
// It assigns a reviewer to an implemented task.
// After updating state, it queues a ReviewAssignmentPrompt message to the reviewer.
type AssignReviewHandler struct {
	processRepo repository.ProcessRepository
	taskRepo    repository.TaskRepository
	queueRepo   repository.QueueRepository
}

// NewAssignReviewHandler creates a new AssignReviewHandler.
// Panics if queueRepo is nil.
func NewAssignReviewHandler(
	processRepo repository.ProcessRepository,
	taskRepo repository.TaskRepository,
	queueRepo repository.QueueRepository,
) *AssignReviewHandler {
	if queueRepo == nil {
		panic("queueRepo is required for AssignReviewHandler")
	}
	return &AssignReviewHandler{
		processRepo: processRepo,
		taskRepo:    taskRepo,
		queueRepo:   queueRepo,
	}
}

// Handle processes an AssignReviewCommand.
// It validates the reviewer state and updates the task with the reviewer assignment.
// Phase transition for reviewer: Idle -> Reviewing
// Phase transition for implementer: Implementing -> AwaitingReview (already happened)
func (h *AssignReviewHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	reviewCmd := cmd.(*command.AssignReviewCommand)

	// 1. Validate reviewer != implementer
	if reviewCmd.ReviewerID == reviewCmd.ImplementerID {
		return nil, types.ErrReviewerIsImplementer
	}

	// 2. Validate reviewer is Ready/Idle
	reviewer, err := h.processRepo.Get(reviewCmd.ReviewerID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get reviewer: %w", err)
	}

	if reviewer.Status != repository.StatusReady {
		return nil, types.ErrProcessNotReady
	}

	if reviewer.Phase != nil && *reviewer.Phase != events.ProcessPhaseIdle {
		return nil, types.ErrProcessNotIdle
	}

	// 3. Get existing TaskAssignment
	task, err := h.taskRepo.Get(reviewCmd.TaskID)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			return nil, fmt.Errorf("task not found: %s", reviewCmd.TaskID)
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	// Validate task's implementer matches
	if task.Implementer != reviewCmd.ImplementerID {
		return nil, types.ErrProcessNotImplementer
	}

	// 4. Update task with Reviewer = reviewerID
	task.Reviewer = reviewCmd.ReviewerID
	task.Status = repository.TaskInReview
	task.ReviewStartedAt = time.Now()

	// 5. Update reviewer: Phase = PhaseReviewing, TaskID
	// NOTE: We do NOT set StatusWorking here - that happens in DeliverProcessQueuedHandler
	reviewing := events.ProcessPhaseReviewing
	reviewer.Phase = &reviewing
	reviewer.TaskID = reviewCmd.TaskID

	// 6. Save to repositories
	if err := h.taskRepo.Save(task); err != nil {
		return nil, fmt.Errorf("failed to save task: %w", err)
	}

	if err := h.processRepo.Save(reviewer); err != nil {
		// Revert task changes on failure
		task.Reviewer = ""
		task.Status = repository.TaskImplementing
		task.ReviewStartedAt = time.Time{}
		_ = h.taskRepo.Save(task)
		return nil, fmt.Errorf("failed to save reviewer: %w", err)
	}

	// 7. Queue ReviewAssignmentPrompt to the reviewer (from coordinator)
	// Note: Summary is not stored in TaskAssignment yet, so we use a placeholder
	prompt := mcp.ReviewAssignmentPrompt(reviewCmd.TaskID, reviewCmd.ImplementerID)
	queue := h.queueRepo.GetOrCreate(reviewCmd.ReviewerID)
	if err := queue.Enqueue(prompt, repository.SenderCoordinator); err != nil {
		return nil, fmt.Errorf("failed to queue review prompt: %w", err)
	}

	// 8. Create follow-up command to deliver the queued message
	// DeliverProcessQueuedHandler will set StatusWorking and actually deliver
	deliverCmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, reviewCmd.ReviewerID)

	// 9. Return with ProcessEvent and follow-up
	event := events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		ProcessID: reviewer.ID,
		Role:      reviewer.Role,
		TaskID:    reviewCmd.TaskID,
		Status:    reviewer.Status, // Still Ready at this point
		Phase:     reviewer.Phase,
	}

	result := &AssignReviewResult{
		ReviewerID:    reviewer.ID,
		TaskID:        reviewCmd.TaskID,
		ImplementerID: reviewCmd.ImplementerID,
	}

	return SuccessWithEventsAndFollowUp(result, []any{event}, []command.Command{deliverCmd}), nil
}

// AssignReviewResult contains the result of assigning a reviewer to a task.
type AssignReviewResult struct {
	ReviewerID    string
	TaskID        string
	ImplementerID string
}

// ===========================================================================
// ApproveCommitHandler
// ===========================================================================

// ApproveCommitHandler handles CmdApproveCommit commands.
// It transitions the implementer to the committing phase after approval.
// After updating state, it queues a CommitApprovalPrompt message to the implementer.
type ApproveCommitHandler struct {
	processRepo repository.ProcessRepository
	taskRepo    repository.TaskRepository
	queueRepo   repository.QueueRepository
}

// NewApproveCommitHandler creates a new ApproveCommitHandler.
// Panics if queueRepo is nil.
func NewApproveCommitHandler(
	processRepo repository.ProcessRepository,
	taskRepo repository.TaskRepository,
	queueRepo repository.QueueRepository,
) *ApproveCommitHandler {
	if queueRepo == nil {
		panic("queueRepo is required for ApproveCommitHandler")
	}
	return &ApproveCommitHandler{
		processRepo: processRepo,
		taskRepo:    taskRepo,
		queueRepo:   queueRepo,
	}
}

// Handle processes an ApproveCommitCommand.
// It validates the task was approved and transitions the implementer to committing.
// Phase transition for implementer: AwaitingReview -> Committing
func (h *ApproveCommitHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	approveCmd := cmd.(*command.ApproveCommitCommand)

	// 1. Get task and validate it was approved
	task, err := h.taskRepo.Get(approveCmd.TaskID)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			return nil, fmt.Errorf("task not found: %s", approveCmd.TaskID)
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	if task.Status != repository.TaskApproved {
		return nil, types.ErrTaskNotApproved
	}

	// Validate task's implementer matches
	if task.Implementer != approveCmd.ImplementerID {
		return nil, types.ErrProcessNotImplementer
	}

	// 2. Get implementer and validate in AwaitingReview phase
	implementer, err := h.processRepo.Get(approveCmd.ImplementerID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get implementer: %w", err)
	}

	if implementer.Phase == nil || *implementer.Phase != events.ProcessPhaseAwaitingReview {
		return nil, types.ErrProcessNotAwaitingReview
	}

	// 3. Update implementer: Phase = PhaseCommitting
	committing := events.ProcessPhaseCommitting
	implementer.Phase = &committing

	// 4. Update task: Status = TaskCommitting
	task.Status = repository.TaskCommitting

	// 5. Save to repositories
	if err := h.taskRepo.Save(task); err != nil {
		return nil, fmt.Errorf("failed to save task: %w", err)
	}

	if err := h.processRepo.Save(implementer); err != nil {
		// Revert task changes on failure
		task.Status = repository.TaskApproved
		_ = h.taskRepo.Save(task)
		return nil, fmt.Errorf("failed to save implementer: %w", err)
	}

	// 6. Queue CommitApprovalPrompt to the implementer (from coordinator)
	prompt := mcp.CommitApprovalPrompt(approveCmd.TaskID, "")
	queue := h.queueRepo.GetOrCreate(approveCmd.ImplementerID)
	if err := queue.Enqueue(prompt, repository.SenderCoordinator); err != nil {
		return nil, fmt.Errorf("failed to queue commit prompt: %w", err)
	}

	// 7. Create follow-up command to deliver the queued message
	deliverCmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, approveCmd.ImplementerID)

	// 8. Return with ProcessEvent and follow-up
	event := events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		ProcessID: implementer.ID,
		Role:      implementer.Role,
		TaskID:    approveCmd.TaskID,
		Status:    implementer.Status,
		Phase:     implementer.Phase,
	}

	result := &ApproveCommitResult{
		ImplementerID: implementer.ID,
		TaskID:        approveCmd.TaskID,
	}

	return SuccessWithEventsAndFollowUp(result, []any{event}, []command.Command{deliverCmd}), nil
}

// ApproveCommitResult contains the result of approving a commit.
type ApproveCommitResult struct {
	ImplementerID string
	TaskID        string
}

// ===========================================================================
// AssignReviewFeedbackHandler
// ===========================================================================

// AssignReviewFeedbackHandler handles CmdAssignReviewFeedback commands.
// It transitions the implementer to the AddressingFeedback phase after a denial.
// After updating state, it queues a ReviewFeedbackPrompt message to the implementer.
type AssignReviewFeedbackHandler struct {
	processRepo repository.ProcessRepository
	taskRepo    repository.TaskRepository
	queueRepo   repository.QueueRepository
}

// NewAssignReviewFeedbackHandler creates a new AssignReviewFeedbackHandler.
// Panics if queueRepo is nil.
func NewAssignReviewFeedbackHandler(
	processRepo repository.ProcessRepository,
	taskRepo repository.TaskRepository,
	queueRepo repository.QueueRepository,
) *AssignReviewFeedbackHandler {
	if queueRepo == nil {
		panic("queueRepo is required for AssignReviewFeedbackHandler")
	}
	return &AssignReviewFeedbackHandler{
		processRepo: processRepo,
		taskRepo:    taskRepo,
		queueRepo:   queueRepo,
	}
}

// Handle processes an AssignReviewFeedbackCommand.
// It validates the task was denied and transitions the implementer to addressing feedback.
// Phase transition for implementer: AwaitingReview -> AddressingFeedback
func (h *AssignReviewFeedbackHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	feedbackCmd := cmd.(*command.AssignReviewFeedbackCommand)

	// 1. Get task and validate it was denied
	task, err := h.taskRepo.Get(feedbackCmd.TaskID)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			return nil, fmt.Errorf("task not found: %s", feedbackCmd.TaskID)
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	if task.Status != repository.TaskDenied {
		return nil, fmt.Errorf("task must be in denied status to send feedback")
	}

	// Validate task's implementer matches
	if task.Implementer != feedbackCmd.ImplementerID {
		return nil, types.ErrProcessNotImplementer
	}

	// 2. Get implementer and validate in AwaitingReview phase
	implementer, err := h.processRepo.Get(feedbackCmd.ImplementerID)
	if err != nil {
		if errors.Is(err, repository.ErrProcessNotFound) {
			return nil, ErrProcessNotFound
		}
		return nil, fmt.Errorf("failed to get implementer: %w", err)
	}

	if implementer.Phase == nil || *implementer.Phase != events.ProcessPhaseAwaitingReview {
		return nil, types.ErrProcessNotAwaitingReview
	}

	// 3. Update implementer: Phase = PhaseAddressingFeedback
	addressing := events.ProcessPhaseAddressingFeedback
	implementer.Phase = &addressing

	// 4. Update task: Status = TaskImplementing (back to implementing to address feedback)
	task.Status = repository.TaskImplementing

	// 5. Save to repositories
	if err := h.taskRepo.Save(task); err != nil {
		return nil, fmt.Errorf("failed to save task: %w", err)
	}

	if err := h.processRepo.Save(implementer); err != nil {
		// Revert task changes on failure
		task.Status = repository.TaskDenied
		_ = h.taskRepo.Save(task)
		return nil, fmt.Errorf("failed to save implementer: %w", err)
	}

	// 6. Queue ReviewFeedbackPrompt to the implementer (from coordinator)
	prompt := mcp.ReviewFeedbackPrompt(feedbackCmd.TaskID, feedbackCmd.Feedback)
	queue := h.queueRepo.GetOrCreate(feedbackCmd.ImplementerID)
	if err := queue.Enqueue(prompt, repository.SenderCoordinator); err != nil {
		return nil, fmt.Errorf("failed to queue feedback prompt: %w", err)
	}

	// 7. Create follow-up command to deliver the queued message
	deliverCmd := command.NewDeliverProcessQueuedCommand(command.SourceInternal, feedbackCmd.ImplementerID)

	// 8. Return with ProcessEvent and follow-up
	event := events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		ProcessID: implementer.ID,
		Role:      implementer.Role,
		TaskID:    feedbackCmd.TaskID,
		Status:    implementer.Status,
		Phase:     implementer.Phase,
	}

	result := &AssignReviewFeedbackResult{
		ImplementerID: implementer.ID,
		TaskID:        feedbackCmd.TaskID,
	}

	return SuccessWithEventsAndFollowUp(result, []any{event}, []command.Command{deliverCmd}), nil
}

// AssignReviewFeedbackResult contains the result of assigning review feedback.
type AssignReviewFeedbackResult struct {
	ImplementerID string
	TaskID        string
}
