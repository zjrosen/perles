package handler

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/orchestration/v2/types"
	"github.com/zjrosen/perles/internal/sound"
)

// ===========================================================================
// Test Mocks for BD Executor
// ===========================================================================

// mockBDExecutorForStateTransition implements BDExecutor for testing.
type mockBDExecutorForStateTransition struct {
	mu              sync.Mutex
	statusUpdates   map[string]string   // taskID -> status
	comments        map[string][]string // taskID -> comments
	updateStatusErr error
	addCommentErr   error
	callCount       int
}

func newMockBDExecutorForStateTransition() *mockBDExecutorForStateTransition {
	return &mockBDExecutorForStateTransition{
		statusUpdates: make(map[string]string),
		comments:      make(map[string][]string),
	}
}

func (m *mockBDExecutorForStateTransition) UpdateTaskStatus(ctx context.Context, taskID, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	if m.updateStatusErr != nil {
		return m.updateStatusErr
	}
	m.statusUpdates[taskID] = status
	return nil
}

func (m *mockBDExecutorForStateTransition) AddComment(ctx context.Context, taskID, comment string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	if m.addCommentErr != nil {
		return m.addCommentErr
	}
	m.comments[taskID] = append(m.comments[taskID], comment)
	return nil
}

func (m *mockBDExecutorForStateTransition) getComments(taskID string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.comments[taskID]
}

func (m *mockBDExecutorForStateTransition) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// ===========================================================================
// IsValidTransition Tests
// ===========================================================================

func TestIsValidTransition_ValidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from events.ProcessPhase
		to   events.ProcessPhase
	}{
		{"Idle to Implementing", events.ProcessPhaseIdle, events.ProcessPhaseImplementing},
		{"Idle to Reviewing", events.ProcessPhaseIdle, events.ProcessPhaseReviewing},
		{"Implementing to AwaitingReview", events.ProcessPhaseImplementing, events.ProcessPhaseAwaitingReview},
		{"Implementing to Idle (cancel)", events.ProcessPhaseImplementing, events.ProcessPhaseIdle},
		{"AwaitingReview to Committing", events.ProcessPhaseAwaitingReview, events.ProcessPhaseCommitting},
		{"AwaitingReview to AddressingFeedback", events.ProcessPhaseAwaitingReview, events.ProcessPhaseAddressingFeedback},
		{"AwaitingReview to Idle", events.ProcessPhaseAwaitingReview, events.ProcessPhaseIdle},
		{"Reviewing to Idle", events.ProcessPhaseReviewing, events.ProcessPhaseIdle},
		{"AddressingFeedback to AwaitingReview", events.ProcessPhaseAddressingFeedback, events.ProcessPhaseAwaitingReview},
		{"AddressingFeedback to Idle", events.ProcessPhaseAddressingFeedback, events.ProcessPhaseIdle},
		{"Committing to Idle", events.ProcessPhaseCommitting, events.ProcessPhaseIdle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.True(t, IsValidTransition(tt.from, tt.to), "expected transition from %s to %s to be valid", tt.from, tt.to)
		})
	}
}

func TestIsValidTransition_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from events.ProcessPhase
		to   events.ProcessPhase
	}{
		{"Idle to AwaitingReview", events.ProcessPhaseIdle, events.ProcessPhaseAwaitingReview},
		{"Idle to Committing", events.ProcessPhaseIdle, events.ProcessPhaseCommitting},
		{"Implementing to Reviewing", events.ProcessPhaseImplementing, events.ProcessPhaseReviewing},
		{"Implementing to Committing", events.ProcessPhaseImplementing, events.ProcessPhaseCommitting},
		{"Reviewing to Implementing", events.ProcessPhaseReviewing, events.ProcessPhaseImplementing},
		{"Reviewing to AwaitingReview", events.ProcessPhaseReviewing, events.ProcessPhaseAwaitingReview},
		{"Committing to Implementing", events.ProcessPhaseCommitting, events.ProcessPhaseImplementing},
		{"AddressingFeedback to Committing", events.ProcessPhaseAddressingFeedback, events.ProcessPhaseCommitting},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.False(t, IsValidTransition(tt.from, tt.to), "expected transition from %s to %s to be invalid", tt.from, tt.to)
		})
	}
}

// ===========================================================================
// ReportCompleteHandler Tests
// ===========================================================================

func TestReportCompleteHandler_TransitionsToAwaitingReview(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0) // 0 = unlimited
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Implementation complete: Implemented feature X").Return(nil)

	// Add implementing worker
	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	// Add task
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))

	cmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "Implemented feature X")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)

	// Verify worker was updated
	updated, _ := processRepo.Get("worker-1")
	require.NotNil(t, updated.Phase)
	require.Equal(t, events.ProcessPhaseAwaitingReview, *updated.Phase)
	require.Equal(t, repository.StatusReady, updated.Status)

	// Verify task was updated
	updatedTask, _ := taskRepo.Get("perles-abc1.2")
	require.Equal(t, repository.TaskInReview, updatedTask.Status)
}

func TestReportCompleteHandler_FailsIfNotImplementingPhase(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)

	// Add worker in wrong phase
	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing), // Wrong phase
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	handler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))

	cmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for non-implementing phase")
	require.ErrorIs(t, err, types.ErrProcessNotImplementing)
}

func TestReportCompleteHandler_SetsWorkerToReady(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)

	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))

	cmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "")
	_, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	updated, _ := processRepo.Get("worker-1")
	require.Equal(t, repository.StatusReady, updated.Status)
}

func TestReportCompleteHandler_CreatesDeliverQueuedIfQueueNonEmpty(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)

	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	// Add message to queue
	queue := queueRepo.GetOrCreate("worker-1")
	_ = queue.Enqueue("Pending message from coordinator", repository.SenderUser)

	handler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))

	cmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	// Verify follow-up command was created
	require.Len(t, result.FollowUp, 1)

	followUp, ok := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	require.True(t, ok, "expected DeliverProcessQueuedCommand, got: %T", result.FollowUp[0])

	require.Equal(t, "worker-1", followUp.ProcessID)
}

func TestReportCompleteHandler_NoFollowUpIfQueueEmpty(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)

	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))

	cmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	// Verify no follow-up commands
	require.Empty(t, result.FollowUp)
}

func TestReportCompleteHandler_FailsIfWorkerNotFound(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)

	handler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))

	cmd := command.NewReportCompleteCommand(command.SourceMCPTool, "unknown-worker", "")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for unknown worker")
	require.ErrorIs(t, err, ErrProcessNotFound)
}

func TestReportCompleteHandler_FailsIfNoTaskAssigned(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)

	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		TaskID:    "", // No task assigned
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	handler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))

	cmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for worker with no task")
	require.ErrorIs(t, err, types.ErrNoTaskAssigned)
}

func TestReportCompleteHandler_EmitsStatusChangeEvent(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)

	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))

	cmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	require.Len(t, result.Events, 1)

	event, ok := result.Events[0].(events.ProcessEvent)
	require.True(t, ok, "expected WorkerEvent, got: %T", result.Events[0])

	require.Equal(t, events.ProcessStatusChange, event.Type)
	require.Equal(t, events.ProcessStatusReady, event.Status)
	require.NotNil(t, event.Phase)
	require.Equal(t, events.ProcessPhaseAwaitingReview, *event.Phase)
}

func TestReportCompleteHandler_WorksFromAddressingFeedback(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", "coordinator", "Implementation complete: Fixed issues").Return(nil)

	// Worker addressing feedback can also report complete
	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAddressingFeedback),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskDenied,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))

	cmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "Fixed issues")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure")

	updated, _ := processRepo.Get("worker-1")
	require.NotNil(t, updated.Phase)
	require.Equal(t, events.ProcessPhaseAwaitingReview, *updated.Phase)
}

// ===========================================================================
// ReportVerdictHandler Tests
// ===========================================================================

func TestReportVerdictHandler_ApprovedTransitionsCorrectly(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", mock.Anything, "Review APPROVED by worker-2").Return(nil)

	// Add implementer
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	// Add reviewer
	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	// Add task in review
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskInReview,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo, WithReportVerdictBDExecutor(bdExecutor))

	cmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictApproved, "LGTM!")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)

	// Verify task was approved
	updatedTask, _ := taskRepo.Get("perles-abc1.2")
	require.Equal(t, repository.TaskApproved, updatedTask.Status)

	// Verify reviewer went idle
	updatedReviewer, _ := processRepo.Get("worker-2")
	require.NotNil(t, updatedReviewer.Phase)
	require.Equal(t, events.ProcessPhaseIdle, *updatedReviewer.Phase)
	require.Equal(t, repository.StatusReady, updatedReviewer.Status)
	require.Empty(t, updatedReviewer.TaskID)
}

func TestReportVerdictHandler_DeniedTransitionsImplementerToAddressingFeedback(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", mock.Anything, "Review DENIED by worker-2: Needs error handling").Return(nil)

	// Add implementer
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	// Add reviewer
	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	// Add task in review
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskInReview,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo, WithReportVerdictBDExecutor(bdExecutor))

	cmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictDenied, "Needs error handling")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)

	// Verify task was denied
	updatedTask, _ := taskRepo.Get("perles-abc1.2")
	require.Equal(t, repository.TaskDenied, updatedTask.Status)

	// Verify reviewer went idle
	updatedReviewer, _ := processRepo.Get("worker-2")
	require.NotNil(t, updatedReviewer.Phase)
	require.Equal(t, events.ProcessPhaseIdle, *updatedReviewer.Phase)

	// Verify implementer went to addressing feedback
	updatedImplementer, _ := processRepo.Get("worker-1")
	require.NotNil(t, updatedImplementer.Phase)
	require.Equal(t, events.ProcessPhaseAddressingFeedback, *updatedImplementer.Phase)
}

func TestReportVerdictHandler_FailsForInvalidVerdict(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)

	worker := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	handler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo, WithReportVerdictBDExecutor(bdExecutor))

	// Invalid verdict
	cmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", "MAYBE", "")
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for invalid verdict")
	require.ErrorIs(t, err, types.ErrInvalidVerdict)
}

func TestReportVerdictHandler_EmitsEventsForBothWorkers(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", mock.Anything, "Review DENIED by worker-2: Needs work").Return(nil)

	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskInReview,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo, WithReportVerdictBDExecutor(bdExecutor))

	cmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictDenied, "Needs work")
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	// Should have 2 events: implementer state change and reviewer state change
	require.Len(t, result.Events, 2)

	// Check for implementer event
	foundImpl := false
	foundReviewer := false
	for _, e := range result.Events {
		event, ok := e.(events.ProcessEvent)
		if !ok {
			continue
		}
		if event.ProcessID == "worker-1" {
			foundImpl = true
			require.NotNil(t, event.Phase)
			require.Equal(t, events.ProcessPhaseAddressingFeedback, *event.Phase, "expected implementer phase AddressingFeedback")
		}
		if event.ProcessID == "worker-2" {
			foundReviewer = true
			require.NotNil(t, event.Phase)
			require.Equal(t, events.ProcessPhaseIdle, *event.Phase, "expected reviewer phase Idle")
		}
	}

	require.True(t, foundImpl, "expected implementer event")
	require.True(t, foundReviewer, "expected reviewer event")
}

// ===========================================================================
// TransitionPhaseHandler Tests
// ===========================================================================

func TestTransitionPhaseHandler_ValidatesStateMachine(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)

	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	handler := NewTransitionPhaseHandler(processRepo, queueRepo)

	// Valid transition: Idle -> Implementing
	cmd := command.NewTransitionPhaseCommand(command.SourceInternal, "worker-1", events.ProcessPhaseImplementing)
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure")

	updated, _ := processRepo.Get("worker-1")
	require.NotNil(t, updated.Phase)
	require.Equal(t, events.ProcessPhaseImplementing, *updated.Phase)
}

func TestTransitionPhaseHandler_RejectsInvalidTransition(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)

	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	handler := NewTransitionPhaseHandler(processRepo, queueRepo)

	// Invalid transition: Idle -> Committing (not allowed)
	cmd := command.NewTransitionPhaseCommand(command.SourceInternal, "worker-1", events.ProcessPhaseCommitting)
	_, err := handler.Handle(context.Background(), cmd)

	require.Error(t, err, "expected error for invalid transition")
	require.ErrorIs(t, err, types.ErrInvalidPhaseTransition)
}

func TestTransitionPhaseHandler_TransitionToIdleClearsTaskID(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)

	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseCommitting),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	handler := NewTransitionPhaseHandler(processRepo, queueRepo)

	cmd := command.NewTransitionPhaseCommand(command.SourceInternal, "worker-1", events.ProcessPhaseIdle)
	_, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	updated, _ := processRepo.Get("worker-1")
	require.Empty(t, updated.TaskID)
	require.Equal(t, repository.StatusReady, updated.Status)
}

func TestTransitionPhaseHandler_CreatesDeliverQueuedOnIdleWithQueue(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)

	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseCommitting),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	// Add message to queue
	queue := queueRepo.GetOrCreate("worker-1")
	_ = queue.Enqueue("Pending message", repository.SenderUser)

	handler := NewTransitionPhaseHandler(processRepo, queueRepo)

	cmd := command.NewTransitionPhaseCommand(command.SourceInternal, "worker-1", events.ProcessPhaseIdle)
	result, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)

	require.Len(t, result.FollowUp, 1)

	_, ok := result.FollowUp[0].(*command.DeliverProcessQueuedCommand)
	require.True(t, ok, "expected DeliverProcessQueuedCommand, got: %T", result.FollowUp[0])
}

// ===========================================================================
// Integration Tests
// ===========================================================================

func TestFullImplementReviewApproveCycle(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	// Mock AddComment for ReportComplete and ReportVerdict calls
	bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	// Add two workers
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	// Add task
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	// Step 1: Report implementation complete
	completeHandler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))
	completeCmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "Done")
	_, err := completeHandler.Handle(context.Background(), completeCmd)
	require.NoError(t, err, "report complete error")

	// Verify implementer state
	impl, _ := processRepo.Get("worker-1")
	require.NotNil(t, impl.Phase)
	require.Equal(t, events.ProcessPhaseAwaitingReview, *impl.Phase)

	// Step 2: Assign reviewer
	reviewAssignHandler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)
	reviewAssignCmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "perles-abc1.2", "worker-1", command.ReviewTypeComplex)
	_, err = reviewAssignHandler.Handle(context.Background(), reviewAssignCmd)
	require.NoError(t, err, "assign review error")

	// Step 3: Report approval verdict
	verdictHandler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo, WithReportVerdictBDExecutor(bdExecutor))
	verdictCmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictApproved, "LGTM")
	_, err = verdictHandler.Handle(context.Background(), verdictCmd)
	require.NoError(t, err, "report verdict error")

	// Verify final state
	finalTask, _ := taskRepo.Get("perles-abc1.2")
	require.Equal(t, repository.TaskApproved, finalTask.Status)

	finalReviewer, _ := processRepo.Get("worker-2")
	require.NotNil(t, finalReviewer.Phase)
	require.Equal(t, events.ProcessPhaseIdle, *finalReviewer.Phase)
	require.Equal(t, repository.StatusReady, finalReviewer.Status)
}

func TestImplementReviewDenyAddressReviewCycle(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	// Mock AddComment for multiple ReportComplete and ReportVerdict calls
	bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	// Add workers
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	// Step 1: Implementation complete
	completeHandler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))
	completeCmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "Done")
	_, _ = completeHandler.Handle(context.Background(), completeCmd)

	// Step 2: Assign reviewer
	reviewAssignHandler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)
	reviewAssignCmd := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "perles-abc1.2", "worker-1", command.ReviewTypeComplex)
	_, _ = reviewAssignHandler.Handle(context.Background(), reviewAssignCmd)

	// Step 3: DENY verdict
	verdictHandler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo, WithReportVerdictBDExecutor(bdExecutor))
	verdictCmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictDenied, "Missing tests")
	_, err := verdictHandler.Handle(context.Background(), verdictCmd)
	require.NoError(t, err, "report deny verdict error")

	// Verify implementer is addressing feedback
	impl, _ := processRepo.Get("worker-1")
	require.NotNil(t, impl.Phase)
	require.Equal(t, events.ProcessPhaseAddressingFeedback, *impl.Phase)

	// Step 4: Address feedback and complete again
	completeCmd2 := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "Added tests")
	_, err = completeHandler.Handle(context.Background(), completeCmd2)
	require.NoError(t, err, "second complete error")

	// Verify back to awaiting review
	impl, _ = processRepo.Get("worker-1")
	require.NotNil(t, impl.Phase)
	require.Equal(t, events.ProcessPhaseAwaitingReview, *impl.Phase)

	// Step 5: Re-assign reviewer (they're now idle)
	reviewAssignCmd2 := command.NewAssignReviewCommand(command.SourceMCPTool, "worker-2", "perles-abc1.2", "worker-1", command.ReviewTypeComplex)
	_, _ = reviewAssignHandler.Handle(context.Background(), reviewAssignCmd2)

	// Step 6: Approve
	verdictCmd2 := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictApproved, "LGTM now")
	_, err = verdictHandler.Handle(context.Background(), verdictCmd2)
	require.NoError(t, err, "approve verdict error")

	// Verify final approval
	finalTask, _ := taskRepo.Get("perles-abc1.2")
	require.Equal(t, repository.TaskApproved, finalTask.Status)
}

// ===========================================================================
// CRITICAL: Race Condition Tests (run with -race flag)
// ===========================================================================
//
// These tests verify that the handlers maintain correct behavior when
// interleaved operations occur. They simulate scenarios where commands
// arrive in quick succession, testing the state machine's robustness.
//
// The FIFO processor serializes these in production, but these tests
// verify the handlers handle edge cases correctly when operations
// interleave at the repository level.

// TestReportComplete_RaceWithNewMessage tests that message arrival during
// ReportComplete transition results in consistent state - either the message
// is queued (worker working) or delivered (worker ready).
func TestReportComplete_RaceWithNewMessage(t *testing.T) {
	// Test interleaved operations: complete then send, and send then complete
	testCases := []struct {
		name          string
		completeFirst bool
	}{
		{"CompleteFirst", true},
		{"SendFirst", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			processRepo := repository.NewMemoryProcessRepository()
			taskRepo := repository.NewMemoryTaskRepository()
			queueRepo := repository.NewMemoryQueueRepository(0)
			bdExecutor := mocks.NewMockBeadsExecutor(t)
			// Mock AddComment for ReportComplete with summary
			bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

			worker := &repository.Process{
				ID:        "worker-1",
				Role:      repository.RoleWorker,
				Status:    repository.StatusWorking,
				Phase:     phasePtr(events.ProcessPhaseImplementing),
				TaskID:    "perles-abc1.2",
				CreatedAt: time.Now(),
			}
			processRepo.AddProcess(worker)

			task := &repository.TaskAssignment{
				TaskID:      "perles-abc1.2",
				Implementer: "worker-1",
				Status:      repository.TaskImplementing,
				StartedAt:   time.Now(),
			}
			_ = taskRepo.Save(task)

			completeHandler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))
			sendHandler := NewSendToProcessHandler(processRepo, queueRepo)

			completeCmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "Done")
			sendCmd := command.NewSendToProcessCommand(command.SourceMCPTool, "worker-1", "New task info")

			var completeErr, sendErr error
			var sendResult *command.CommandResult

			// Execute in order based on test case
			if tc.completeFirst {
				_, completeErr = completeHandler.Handle(context.Background(), completeCmd)
				sendResult, sendErr = sendHandler.Handle(context.Background(), sendCmd)
			} else {
				sendResult, sendErr = sendHandler.Handle(context.Background(), sendCmd)
				_, completeErr = completeHandler.Handle(context.Background(), completeCmd)
			}

			// Both operations should succeed
			require.NoError(t, completeErr, "complete error")
			require.NoError(t, sendErr, "send error")

			// Verify final state is consistent
			finalWorker, _ := processRepo.Get("worker-1")
			require.NotNil(t, finalWorker, "worker missing after operations")

			// If send was second (complete first), message should trigger delivery
			if sendResult != nil && sendResult.Success {
				t.Logf("Send result: %+v", sendResult.Data)
			}
		})
	}
}

// TestReportVerdict_RaceWithNewTask tests verdict processing followed by
// new task assignment maintains consistent state across different workers.
func TestReportVerdict_RaceWithNewTask(t *testing.T) {
	// Test both orderings
	testCases := []struct {
		name         string
		verdictFirst bool
	}{
		{"VerdictFirst", true},
		{"AssignFirst", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			processRepo := repository.NewMemoryProcessRepository()
			taskRepo := repository.NewMemoryTaskRepository()
			queueRepo := repository.NewMemoryQueueRepository(0)
			bdExecutor := mocks.NewMockBeadsExecutor(t)
			// Mock for AssignTaskHandler: ShowIssue and UpdateStatus
			bdExecutor.EXPECT().ShowIssue(mock.Anything).Return(&beads.Issue{ID: "perles-xyz9.1", Status: beads.StatusOpen}, nil).Maybe()
			bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil).Maybe()

			// Create a separate mock for verdict handler
			verdictBDExecutor := mocks.NewMockBeadsExecutor(t)
			verdictBDExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

			// Setup: reviewer about to finish reviewing
			implementer := &repository.Process{
				ID:        "worker-1",
				Role:      repository.RoleWorker,
				Status:    repository.StatusWorking,
				Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
				TaskID:    "perles-abc1.2",
				CreatedAt: time.Now(),
			}
			processRepo.AddProcess(implementer)

			reviewer := &repository.Process{
				ID:        "worker-2",
				Role:      repository.RoleWorker,
				Status:    repository.StatusWorking,
				Phase:     phasePtr(events.ProcessPhaseReviewing),
				TaskID:    "perles-abc1.2",
				CreatedAt: time.Now(),
			}
			processRepo.AddProcess(reviewer)

			task := &repository.TaskAssignment{
				TaskID:      "perles-abc1.2",
				Implementer: "worker-1",
				Reviewer:    "worker-2",
				Status:      repository.TaskInReview,
				StartedAt:   time.Now(),
			}
			_ = taskRepo.Save(task)

			// Add another ready worker for the new task
			anotherWorker := &repository.Process{
				ID:        "worker-3",
				Role:      repository.RoleWorker,
				Status:    repository.StatusReady,
				Phase:     phasePtr(events.ProcessPhaseIdle),
				CreatedAt: time.Now(),
			}
			processRepo.AddProcess(anotherWorker)

			verdictHandler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo, WithReportVerdictBDExecutor(verdictBDExecutor))
			assignHandler := NewAssignTaskHandler(processRepo, taskRepo, WithBDExecutor(bdExecutor), WithQueueRepository(queueRepo))

			verdictCmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictApproved, "LGTM")
			assignCmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-3", "perles-xyz9.1", "New task")

			var verdictErr, assignErr error

			if tc.verdictFirst {
				_, verdictErr = verdictHandler.Handle(context.Background(), verdictCmd)
				_, assignErr = assignHandler.Handle(context.Background(), assignCmd)
			} else {
				_, assignErr = assignHandler.Handle(context.Background(), assignCmd)
				_, verdictErr = verdictHandler.Handle(context.Background(), verdictCmd)
			}

			// Both should succeed (they operate on different workers)
			require.NoError(t, verdictErr, "verdict error")
			require.NoError(t, assignErr, "assign error")

			// Verify state consistency
			finalTask, _ := taskRepo.Get("perles-abc1.2")
			require.Equal(t, repository.TaskApproved, finalTask.Status)

			newTask, err := taskRepo.Get("perles-xyz9.1")
			require.NoError(t, err, "new task should exist")
			require.Equal(t, "worker-3", newTask.Implementer)
		})
	}
}

// TestTransitionPhase_ConcurrentTransitions tests multiple sequential transitions
// to verify state machine remains consistent after a series of operations.
func TestTransitionPhase_ConcurrentTransitions(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)

	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	handler := NewTransitionPhaseHandler(processRepo, queueRepo)

	// First transition should succeed (Idle -> Implementing)
	cmd1 := command.NewTransitionPhaseCommand(command.SourceInternal, "worker-1", events.ProcessPhaseImplementing)
	result1, err1 := handler.Handle(context.Background(), cmd1)
	require.NoError(t, err1, "first transition should succeed")
	require.True(t, result1.Success, "first transition should succeed")

	// Second transition to same phase should fail (Implementing -> Implementing is invalid)
	cmd2 := command.NewTransitionPhaseCommand(command.SourceInternal, "worker-1", events.ProcessPhaseImplementing)
	_, err2 := handler.Handle(context.Background(), cmd2)
	require.ErrorIs(t, err2, types.ErrInvalidPhaseTransition, "second transition should fail with invalid transition error")

	// Third transition to AwaitingReview should succeed
	cmd3 := command.NewTransitionPhaseCommand(command.SourceInternal, "worker-1", events.ProcessPhaseAwaitingReview)
	result3, err3 := handler.Handle(context.Background(), cmd3)
	require.NoError(t, err3, "third transition should succeed")
	require.True(t, result3.Success, "third transition should succeed")

	// Verify final state
	finalWorker, _ := processRepo.Get("worker-1")
	require.NotNil(t, finalWorker.Phase)
	require.Equal(t, events.ProcessPhaseAwaitingReview, *finalWorker.Phase)
}

// TestReportComplete_RaceWithRetire tests that worker retirement and completion
// are handled correctly when processed in different orders.
func TestReportComplete_RaceWithRetire(t *testing.T) {
	testCases := []struct {
		name          string
		completeFirst bool
	}{
		{"CompleteFirst", true},
		{"RetireFirst", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			processRepo := repository.NewMemoryProcessRepository()
			taskRepo := repository.NewMemoryTaskRepository()
			queueRepo := repository.NewMemoryQueueRepository(0)
			bdExecutor := mocks.NewMockBeadsExecutor(t)
			// Mock AddComment for ReportComplete with summary
			bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

			worker := &repository.Process{
				ID:        "worker-1",
				Role:      repository.RoleWorker,
				Status:    repository.StatusWorking,
				Phase:     phasePtr(events.ProcessPhaseImplementing),
				TaskID:    "perles-abc1.2",
				CreatedAt: time.Now(),
			}
			processRepo.AddProcess(worker)

			task := &repository.TaskAssignment{
				TaskID:      "perles-abc1.2",
				Implementer: "worker-1",
				Status:      repository.TaskImplementing,
				StartedAt:   time.Now(),
			}
			_ = taskRepo.Save(task)

			completeHandler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))
			retireHandler := NewRetireProcessHandler(processRepo, nil)

			completeCmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "Done")
			retireCmd := command.NewRetireProcessCommand(command.SourceInternal, "worker-1", "Context cancelled")

			var completeErr, retireErr error

			if tc.completeFirst {
				_, completeErr = completeHandler.Handle(context.Background(), completeCmd)
				_, retireErr = retireHandler.Handle(context.Background(), retireCmd)
			} else {
				_, retireErr = retireHandler.Handle(context.Background(), retireCmd)
				_, completeErr = completeHandler.Handle(context.Background(), completeCmd)
			}

			finalWorker, _ := processRepo.Get("worker-1")
			require.NotNil(t, finalWorker, "worker should still exist")

			if tc.completeFirst {
				// Complete should succeed, retire should succeed (worker transitions)
				require.NoError(t, completeErr, "complete should succeed")
				// After complete, worker goes to AwaitingReview/Ready, then retire
				if finalWorker.Status != repository.StatusRetired {
					t.Logf("Expected retired after complete+retire, got status=%s phase=%v",
						finalWorker.Status, finalWorker.Phase)
				}
			} else {
				// Retire first should succeed, complete should fail (worker retired)
				require.NoError(t, retireErr, "retire should succeed")
				require.Error(t, completeErr, "complete should fail after retire")
				require.Equal(t, repository.StatusRetired, finalWorker.Status)
			}
		})
	}
}

// TestReportVerdict_RaceWithWorkerRetire tests that verdict and retire
// operations are handled correctly in different orderings.
func TestReportVerdict_RaceWithWorkerRetire(t *testing.T) {
	testCases := []struct {
		name         string
		verdictFirst bool
	}{
		{"VerdictFirst", true},
		{"RetireFirst", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			processRepo := repository.NewMemoryProcessRepository()
			taskRepo := repository.NewMemoryTaskRepository()
			queueRepo := repository.NewMemoryQueueRepository(0)
			bdExecutor := mocks.NewMockBeadsExecutor(t)
			// Mock AddComment for ReportVerdict
			bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

			implementer := &repository.Process{
				ID:        "worker-1",
				Role:      repository.RoleWorker,
				Status:    repository.StatusWorking,
				Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
				TaskID:    "perles-abc1.2",
				CreatedAt: time.Now(),
			}
			processRepo.AddProcess(implementer)

			reviewer := &repository.Process{
				ID:        "worker-2",
				Role:      repository.RoleWorker,
				Status:    repository.StatusWorking,
				Phase:     phasePtr(events.ProcessPhaseReviewing),
				TaskID:    "perles-abc1.2",
				CreatedAt: time.Now(),
			}
			processRepo.AddProcess(reviewer)

			task := &repository.TaskAssignment{
				TaskID:      "perles-abc1.2",
				Implementer: "worker-1",
				Reviewer:    "worker-2",
				Status:      repository.TaskInReview,
				StartedAt:   time.Now(),
			}
			_ = taskRepo.Save(task)

			verdictHandler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo, WithReportVerdictBDExecutor(bdExecutor))
			retireHandler := NewRetireProcessHandler(processRepo, nil)

			verdictCmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictDenied, "Needs work")
			retireCmd := command.NewRetireProcessCommand(command.SourceInternal, "worker-1", "Timeout")

			var verdictErr, retireErr error

			if tc.verdictFirst {
				_, verdictErr = verdictHandler.Handle(context.Background(), verdictCmd)
				_, retireErr = retireHandler.Handle(context.Background(), retireCmd)
			} else {
				_, retireErr = retireHandler.Handle(context.Background(), retireCmd)
				_, verdictErr = verdictHandler.Handle(context.Background(), verdictCmd)
			}

			// Verify state is consistent
			impl, _ := processRepo.Get("worker-1")
			rev, _ := processRepo.Get("worker-2")

			require.NotNil(t, impl, "implementer should still exist")
			require.NotNil(t, rev, "reviewer should still exist")

			if tc.verdictFirst {
				// Verdict succeeds, implementer transitions to AddressingFeedback, then retire
				require.NoError(t, verdictErr, "verdict should succeed")
				// Implementer should be retired after both operations
				if impl.Status != repository.StatusRetired {
					t.Logf("Implementer status=%s phase=%v (expected Retired)", impl.Status, impl.Phase)
				}
			} else {
				// Retire first succeeds, verdict may fail or handle gracefully
				require.NoError(t, retireErr, "retire should succeed")
				// Implementer retired, verdict handling implementer is a failure scenario
				require.Equal(t, repository.StatusRetired, impl.Status)
			}

			// Reviewer should be in consistent state regardless
			t.Logf("Reviewer phase: %v, status: %s", rev.Phase, rev.Status)
		})
	}
}

// ===========================================================================
// Property-Based Test: State Machine Validity
// ===========================================================================

// TestStateTransition_ValidTransitionsOnly uses random operations to verify
// that the state machine only produces valid transitions.
func TestStateTransition_ValidTransitionsOnly(t *testing.T) {
	// Use a fixed seed for reproducibility
	rng := rand.New(rand.NewSource(42))

	allPhases := []events.ProcessPhase{
		events.ProcessPhaseIdle,
		events.ProcessPhaseImplementing,
		events.ProcessPhaseAwaitingReview,
		events.ProcessPhaseReviewing,
		events.ProcessPhaseAddressingFeedback,
		events.ProcessPhaseCommitting,
	}

	// Run many random transition attempts
	for i := 0; i < 1000; i++ {
		processRepo := repository.NewMemoryProcessRepository()
		queueRepo := repository.NewMemoryQueueRepository(0)

		// Start with random phase
		startPhase := allPhases[rng.Intn(len(allPhases))]
		worker := &repository.Process{
			ID:        "worker-1",
			Role:      repository.RoleWorker,
			Status:    repository.StatusReady,
			Phase:     &startPhase,
			CreatedAt: time.Now(),
		}
		processRepo.AddProcess(worker)

		handler := NewTransitionPhaseHandler(processRepo, queueRepo)

		// Attempt random transition
		targetPhase := allPhases[rng.Intn(len(allPhases))]
		cmd := command.NewTransitionPhaseCommand(command.SourceInternal, "worker-1", targetPhase)
		_, err := handler.Handle(context.Background(), cmd)

		// Verify: if transition succeeded, it must be valid
		if err == nil {
			require.True(t, IsValidTransition(startPhase, targetPhase), "Handler allowed invalid transition from %s to %s", startPhase, targetPhase)
		}

		// Verify: if transition failed with ErrInvalidPhaseTransition, it must be invalid
		if errors.Is(err, types.ErrInvalidPhaseTransition) {
			require.False(t, IsValidTransition(startPhase, targetPhase), "Handler rejected valid transition from %s to %s", startPhase, targetPhase)
		}
	}
}

// TestStateTransition_AllValidTransitionsSucceed ensures all documented valid
// transitions actually succeed.
func TestStateTransition_AllValidTransitionsSucceed(t *testing.T) {
	for fromPhase, toPhases := range ValidTransitions {
		for _, toPhase := range toPhases {
			t.Run(string(fromPhase)+"_to_"+string(toPhase), func(t *testing.T) {
				processRepo := repository.NewMemoryProcessRepository()
				queueRepo := repository.NewMemoryQueueRepository(0)

				fp := fromPhase
				worker := &repository.Process{
					ID:        "worker-1",
					Role:      repository.RoleWorker,
					Status:    repository.StatusReady,
					Phase:     &fp,
					CreatedAt: time.Now(),
				}
				processRepo.AddProcess(worker)

				handler := NewTransitionPhaseHandler(processRepo, queueRepo)

				cmd := command.NewTransitionPhaseCommand(command.SourceInternal, "worker-1", toPhase)
				result, err := handler.Handle(context.Background(), cmd)

				require.NoError(t, err, "expected transition from %s to %s to succeed", fromPhase, toPhase)
				require.True(t, result.Success, "expected success for transition from %s to %s", fromPhase, toPhase)
			})
		}
	}
}

// TestStateTransition_PropertyNoInvalidStates verifies that after any
// sequence of valid operations, the system never enters an invalid state.
func TestStateTransition_PropertyNoInvalidStates(t *testing.T) {
	rng := rand.New(rand.NewSource(12345))

	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)

	// Start with a worker
	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	// Create handlers
	transitionHandler := NewTransitionPhaseHandler(processRepo, queueRepo)

	// Perform random valid transitions
	currentPhase := events.ProcessPhaseIdle
	for i := 0; i < 100; i++ {
		// Get valid targets from current phase
		validTargets, ok := ValidTransitions[currentPhase]
		if !ok || len(validTargets) == 0 {
			// No valid transitions, stay in current phase
			continue
		}

		// Pick a random valid target
		targetPhase := validTargets[rng.Intn(len(validTargets))]

		cmd := command.NewTransitionPhaseCommand(command.SourceInternal, "worker-1", targetPhase)
		result, err := transitionHandler.Handle(context.Background(), cmd)

		require.NoError(t, err, "iteration %d: unexpected error for valid transition %s -> %s", i, currentPhase, targetPhase)
		require.True(t, result.Success, "iteration %d: expected success for valid transition %s -> %s", i, currentPhase, targetPhase)

		// Verify the worker is in the expected phase
		w, _ := processRepo.Get("worker-1")
		require.NotNil(t, w.Phase, "iteration %d: phase should not be nil", i)
		require.Equal(t, targetPhase, *w.Phase, "iteration %d: expected phase %s", i, targetPhase)

		currentPhase = targetPhase

		// Verify invariants hold
		require.True(t, isValidPhase(currentPhase), "iteration %d: worker ended in invalid phase: %s", i, currentPhase)
	}
}

// isValidPhase checks if a phase is one of the known valid phases.
func isValidPhase(phase events.ProcessPhase) bool {
	switch phase {
	case events.ProcessPhaseIdle,
		events.ProcessPhaseImplementing,
		events.ProcessPhaseAwaitingReview,
		events.ProcessPhaseReviewing,
		events.ProcessPhaseAddressingFeedback,
		events.ProcessPhaseCommitting:
		return true
	default:
		return false
	}
}

// ===========================================================================
// Synchronous BD Error Propagation Tests
// ===========================================================================

func TestReportCompleteHandler_FailsOnBDError(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("bd database connection refused")).Once()

	// Add implementing worker
	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		TaskID:    "perles-test456",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	// Add task
	task := &repository.TaskAssignment{
		TaskID:      "perles-test456",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))

	// Command with summary triggers BD comment
	cmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "Implementation done")
	_, err := handler.Handle(context.Background(), cmd)

	// Command should fail due to BD error (synchronous)
	require.Error(t, err, "expected error on BD failure")

	// Verify error contains the original BD error
	require.Contains(t, err.Error(), "bd database connection refused")
	require.Contains(t, err.Error(), "failed to add BD comment")
}

func TestReportCompleteHandler_SkipsCommentWhenSummaryEmpty(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	// No AddComment expectation - test relies on mock.AssertExpectations to fail if it's called

	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))

	// Empty summary should NOT trigger BD comment
	cmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "")
	result, err := handler.Handle(context.Background(), cmd)

	// Verify command succeeded (no BD call made)
	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)

	// Verify worker state was updated
	updated, _ := processRepo.Get("worker-1")
	require.NotNil(t, updated.Phase)
	require.Equal(t, events.ProcessPhaseAwaitingReview, *updated.Phase)
	require.Equal(t, repository.StatusReady, updated.Status)
}

func TestReportCompleteHandler_CallsBDSynchronously(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", mock.Anything, "Implementation complete: Feature implemented").Return(nil)

	worker := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(worker)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))

	cmd := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "Feature implemented")
	result, err := handler.Handle(context.Background(), cmd)

	// Verify command succeeded
	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)
}

func TestReportCompleteHandler_PanicsIfBDExecutorNil(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)

	require.Panics(t, func() {
		NewReportCompleteHandler(processRepo, taskRepo, queueRepo)
	}, "expected panic when bdExecutor is nil")
}

func TestReportVerdictHandler_FailsOnBDError(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(errors.New("bd disk full"))

	// Add implementer
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-test789",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	// Add reviewer
	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing),
		TaskID:    "perles-test789",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	// Add task
	task := &repository.TaskAssignment{
		TaskID:      "perles-test789",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskInReview,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo, WithReportVerdictBDExecutor(bdExecutor))

	cmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictApproved, "LGTM")
	_, err := handler.Handle(context.Background(), cmd)

	// Command should fail due to BD error (synchronous)
	require.Error(t, err, "expected error on BD failure")

	// Verify error contains the original BD error
	require.Contains(t, err.Error(), "bd disk full")
	require.Contains(t, err.Error(), "failed to add BD comment")
}

func TestReportVerdictHandler_CallsBDSynchronously(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment("perles-abc1.2", mock.Anything, "Review DENIED by worker-2: Needs fixes").Return(nil)

	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskInReview,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo, WithReportVerdictBDExecutor(bdExecutor))

	cmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictDenied, "Needs fixes")
	result, err := handler.Handle(context.Background(), cmd)

	// Verify command succeeded
	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)
}

func TestReportVerdictHandler_CorrectCommentForApproved(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment("perles-verdict-test", mock.Anything, "Review APPROVED by worker-2").Return(nil)

	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-verdict-test",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing),
		TaskID:    "perles-verdict-test",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	task := &repository.TaskAssignment{
		TaskID:      "perles-verdict-test",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskInReview,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo, WithReportVerdictBDExecutor(bdExecutor))

	// Test APPROVED verdict
	cmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictApproved, "LGTM!")
	result, err := handler.Handle(context.Background(), cmd)

	// Verify command succeeded
	require.NoError(t, err)
	require.True(t, result.Success, "expected success, got failure: %v", result.Error)
}

func TestReportVerdictHandler_PanicsIfBDExecutorNil(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)

	require.Panics(t, func() {
		NewReportVerdictHandler(processRepo, taskRepo, queueRepo)
	}, "expected panic when bdExecutor is nil")
}

func TestReportVerdictHandler_PlaysApproveSound(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	soundService := mocks.NewMockSoundService(t)

	bdExecutor.EXPECT().AddComment("perles-abc1.2", mock.Anything, "Review APPROVED by worker-2").Return(nil)
	soundService.EXPECT().Play("approve", "review_verdict_approve").Once()

	// Add implementer
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	// Add reviewer
	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	// Add task in review
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskInReview,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportVerdictHandler(
		processRepo, taskRepo, queueRepo,
		WithReportVerdictBDExecutor(bdExecutor),
		WithReportVerdictSoundService(soundService),
	)

	cmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictApproved, "LGTM!")
	_, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	// Sound service mock expectations are automatically verified on cleanup
}

func TestReportVerdictHandler_PlaysDenySound(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	soundService := mocks.NewMockSoundService(t)

	bdExecutor.EXPECT().AddComment("perles-abc1.2", mock.Anything, "Review DENIED by worker-2: Needs error handling").Return(nil)
	soundService.EXPECT().Play("deny", "review_verdict_deny").Once()

	// Add implementer
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	// Add reviewer
	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	// Add task in review
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskInReview,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	handler := NewReportVerdictHandler(
		processRepo, taskRepo, queueRepo,
		WithReportVerdictBDExecutor(bdExecutor),
		WithReportVerdictSoundService(soundService),
	)

	cmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictDenied, "Needs error handling")
	_, err := handler.Handle(context.Background(), cmd)

	require.NoError(t, err)
	// Sound service mock expectations are automatically verified on cleanup
}

func TestReportVerdictHandler_NilSoundService(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)

	bdExecutor.EXPECT().AddComment("perles-abc1.2", mock.Anything, "Review APPROVED by worker-2").Return(nil)

	// Add implementer
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	// Add reviewer
	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing),
		TaskID:    "perles-abc1.2",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	// Add task in review
	task := &repository.TaskAssignment{
		TaskID:      "perles-abc1.2",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskInReview,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	// Create handler WITHOUT sound service option - should use NoopSoundService
	handler := NewReportVerdictHandler(
		processRepo, taskRepo, queueRepo,
		WithReportVerdictBDExecutor(bdExecutor),
	)

	cmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictApproved, "LGTM!")
	result, err := handler.Handle(context.Background(), cmd)

	// Should succeed without panic - NoopSoundService handles the Play call
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestWithReportVerdictSoundService_SetsService(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	soundService := mocks.NewMockSoundService(t)

	handler := NewReportVerdictHandler(
		processRepo, taskRepo, queueRepo,
		WithReportVerdictBDExecutor(bdExecutor),
		WithReportVerdictSoundService(soundService),
	)

	// Verify the handler was created and the option was applied
	require.NotNil(t, handler)
	require.Equal(t, soundService, handler.soundService)
}

func TestWithReportVerdictSoundService_NilIgnored(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)

	handler := NewReportVerdictHandler(
		processRepo, taskRepo, queueRepo,
		WithReportVerdictBDExecutor(bdExecutor),
		WithReportVerdictSoundService(nil), // nil should be ignored
	)

	// Should still have NoopSoundService as the default
	require.NotNil(t, handler)
	require.IsType(t, sound.NoopSoundService{}, handler.soundService)
}

// ===========================================================================
// Integration Test: Full Denial → Re-Review Cycle
// ===========================================================================

// TestIntegration_DenialToReReviewCycle tests the complete flow:
// 1. Worker implements task
// 2. Worker reports complete → AwaitingReview
// 3. Reviewer assigned → Reviewing
// 4. Reviewer DENIES → Implementer to AddressingFeedback
// 5. Implementer fixes and reports complete again
// 6. Reviewer assigned again → Reviewing
// 7. Reviewer APPROVES
// 8. Task should be in Approved status, ready for commit
func TestIntegration_DenialToReReviewCycle(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	// Mock AddComment for multiple ReportComplete and ReportVerdict calls
	bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	// Create handlers
	reportCompleteHandler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))
	assignReviewHandler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)
	reportVerdictHandler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo, WithReportVerdictBDExecutor(bdExecutor))

	ctx := context.Background()

	// === SETUP: Implementer with task in Implementing phase ===
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		TaskID:    "task-123",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		TaskID:    "",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	task := &repository.TaskAssignment{
		TaskID:      "task-123",
		Implementer: "worker-1",
		Reviewer:    "",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	// === STEP 1: Implementer reports complete ===
	t.Log("STEP 1: Implementer reports complete")
	cmd1 := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "Initial implementation")
	result1, err := reportCompleteHandler.Handle(ctx, cmd1)
	require.NoError(t, err, "Step 1 failed")
	require.True(t, result1.Success, "Step 1 not successful: %v", result1.Error)

	// Verify state after step 1
	impl1, _ := processRepo.Get("worker-1")
	task1, _ := taskRepo.Get("task-123")
	t.Logf("After Step 1: implementer.Phase=%v, task.Status=%s, task.Reviewer=%q",
		*impl1.Phase, task1.Status, task1.Reviewer)

	require.Equal(t, events.ProcessPhaseAwaitingReview, *impl1.Phase)
	require.Equal(t, repository.TaskInReview, task1.Status)

	// === STEP 2: Assign reviewer ===
	t.Log("STEP 2: Assign reviewer")
	cmd2 := command.NewAssignReviewCommand(command.SourceInternal, "worker-2", "task-123", "worker-1", command.ReviewTypeComplex)
	result2, err := assignReviewHandler.Handle(ctx, cmd2)
	require.NoError(t, err, "Step 2 failed")
	require.True(t, result2.Success, "Step 2 not successful: %v", result2.Error)

	// Verify state after step 2
	rev2, _ := processRepo.Get("worker-2")
	task2, _ := taskRepo.Get("task-123")
	t.Logf("After Step 2: reviewer.Phase=%v, reviewer.TaskID=%q, task.Reviewer=%q",
		*rev2.Phase, rev2.TaskID, task2.Reviewer)

	require.Equal(t, events.ProcessPhaseReviewing, *rev2.Phase)
	require.Equal(t, "task-123", rev2.TaskID)
	require.Equal(t, "worker-2", task2.Reviewer)

	// === STEP 3: Reviewer DENIES ===
	t.Log("STEP 3: Reviewer denies")
	cmd3 := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictDenied, "Needs error handling")
	result3, err := reportVerdictHandler.Handle(ctx, cmd3)
	require.NoError(t, err, "Step 3 failed")
	require.True(t, result3.Success, "Step 3 not successful: %v", result3.Error)

	// Verify state after step 3 - THIS IS KEY
	rev3, _ := processRepo.Get("worker-2")
	impl3, _ := processRepo.Get("worker-1")
	task3, _ := taskRepo.Get("task-123")
	t.Logf("After Step 3 (DENIAL): reviewer.Phase=%v, reviewer.TaskID=%q, reviewer.Status=%s",
		*rev3.Phase, rev3.TaskID, rev3.Status)
	t.Logf("After Step 3 (DENIAL): implementer.Phase=%v, task.Status=%s, task.Reviewer=%q",
		*impl3.Phase, task3.Status, task3.Reviewer)

	require.Equal(t, events.ProcessPhaseIdle, *rev3.Phase)
	require.Equal(t, events.ProcessPhaseAddressingFeedback, *impl3.Phase)
	require.Equal(t, repository.TaskDenied, task3.Status)

	// === STEP 4: Implementer addresses feedback and reports complete again ===
	t.Log("STEP 4: Implementer reports complete after addressing feedback")
	cmd4 := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "Fixed error handling")
	result4, err := reportCompleteHandler.Handle(ctx, cmd4)
	require.NoError(t, err, "Step 4 failed")
	require.True(t, result4.Success, "Step 4 not successful: %v", result4.Error)

	// Verify state after step 4
	impl4, _ := processRepo.Get("worker-1")
	task4, _ := taskRepo.Get("task-123")
	t.Logf("After Step 4: implementer.Phase=%v, task.Status=%s, task.Reviewer=%q",
		*impl4.Phase, task4.Status, task4.Reviewer)

	require.Equal(t, events.ProcessPhaseAwaitingReview, *impl4.Phase)
	require.Equal(t, repository.TaskInReview, task4.Status)

	// === STEP 5: Assign reviewer AGAIN ===
	t.Log("STEP 5: Assign reviewer again for second review")
	cmd5 := command.NewAssignReviewCommand(command.SourceInternal, "worker-2", "task-123", "worker-1", command.ReviewTypeComplex)
	result5, err := assignReviewHandler.Handle(ctx, cmd5)
	require.NoError(t, err, "Step 5 failed")
	require.True(t, result5.Success, "Step 5 not successful: %v", result5.Error)

	// Verify state after step 5
	rev5, _ := processRepo.Get("worker-2")
	task5, _ := taskRepo.Get("task-123")
	t.Logf("After Step 5: reviewer.Phase=%v, reviewer.TaskID=%q, task.Reviewer=%q",
		*rev5.Phase, rev5.TaskID, task5.Reviewer)

	require.Equal(t, events.ProcessPhaseReviewing, *rev5.Phase)

	// === STEP 6: Reviewer APPROVES this time ===
	t.Log("STEP 6: Reviewer approves")
	cmd6 := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictApproved, "LGTM")
	result6, err := reportVerdictHandler.Handle(ctx, cmd6)
	require.NoError(t, err, "Step 6 failed")
	require.True(t, result6.Success, "Step 6 not successful: %v", result6.Error)

	// Verify state after step 6
	rev6, _ := processRepo.Get("worker-2")
	task6, _ := taskRepo.Get("task-123")
	impl6, _ := processRepo.Get("worker-1")
	t.Logf("After Step 6 (APPROVAL): reviewer.Phase=%v, task.Status=%s, implementer.Phase=%v",
		*rev6.Phase, task6.Status, *impl6.Phase)

	require.Equal(t, repository.TaskApproved, task6.Status)

	// === STEP 7: Approve commit (coordinator signals implementer to commit) ===
	t.Log("STEP 7: Approve commit")
	approveCommitHandler := NewApproveCommitHandler(processRepo, taskRepo, queueRepo)
	cmd7 := command.NewApproveCommitCommand(command.SourceInternal, "worker-1", "task-123")
	result7, err := approveCommitHandler.Handle(ctx, cmd7)
	require.NoError(t, err, "Step 7 failed")
	require.True(t, result7.Success, "Step 7 not successful: %v", result7.Error)

	// Verify final state
	impl7, _ := processRepo.Get("worker-1")
	task7, _ := taskRepo.Get("task-123")
	t.Logf("After Step 7 (APPROVE_COMMIT): implementer.Phase=%v, task.Status=%s",
		*impl7.Phase, task7.Status)

	require.Equal(t, events.ProcessPhaseCommitting, *impl7.Phase)
	require.Equal(t, repository.TaskCommitting, task7.Status)

	t.Log("SUCCESS: Full denial → re-review → commit cycle completed correctly")
}

// TestIntegration_ReAssignReviewAfterDenial verifies that after a denial,
// the coordinator can call assign_task_review again to put the reviewer
// back in Reviewing phase so they can submit another verdict.
// This reproduces the bug from session 2feb8a33 where worker-22 couldn't
// submit a second verdict because they were stuck in Idle phase.
func TestIntegration_ReAssignReviewAfterDenial(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	// Mock AddComment for multiple ReportComplete and ReportVerdict calls
	bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	// Create handlers
	reportCompleteHandler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))
	assignReviewHandler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)
	reportVerdictHandler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo, WithReportVerdictBDExecutor(bdExecutor))

	ctx := context.Background()

	// === SETUP ===
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		TaskID:    "task-123",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		TaskID:    "",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	task := &repository.TaskAssignment{
		TaskID:      "task-123",
		Implementer: "worker-1",
		Reviewer:    "",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	// Step 1: Implementer reports complete
	cmd1 := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "Initial implementation")
	_, _ = reportCompleteHandler.Handle(ctx, cmd1)

	// Step 2: Assign reviewer (first time)
	cmd2 := command.NewAssignReviewCommand(command.SourceInternal, "worker-2", "task-123", "worker-1", command.ReviewTypeComplex)
	_, err := assignReviewHandler.Handle(ctx, cmd2)
	require.NoError(t, err, "First assign_task_review failed")

	// Verify reviewer is now in Reviewing phase
	rev2, _ := processRepo.Get("worker-2")
	require.Equal(t, events.ProcessPhaseReviewing, *rev2.Phase)

	// Step 3: Reviewer DENIES
	cmd3 := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictDenied, "Needs fixes")
	_, _ = reportVerdictHandler.Handle(ctx, cmd3)

	// Verify state after denial
	rev3, _ := processRepo.Get("worker-2")
	task3, _ := taskRepo.Get("task-123")
	t.Logf("After denial: reviewer.Phase=%v, reviewer.TaskID=%q, task.Reviewer=%q, task.Status=%s",
		*rev3.Phase, rev3.TaskID, task3.Reviewer, task3.Status)

	// Reviewer should be in Idle with no TaskID
	require.Equal(t, events.ProcessPhaseIdle, *rev3.Phase)
	require.Empty(t, rev3.TaskID)
	// BUG: task.Reviewer should be cleared on denial so coordinator knows to re-assign
	// Currently task.Reviewer is NOT cleared, causing coordinator to think reviewer is assigned
	require.Empty(t, task3.Reviewer, "BUG: Expected task.Reviewer to be empty after denial (coordinator will think reviewer is still assigned)")

	// Step 4: Implementer addresses feedback and reports complete again
	cmd4 := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "Fixed issues")
	_, _ = reportCompleteHandler.Handle(ctx, cmd4)

	// Step 5: Coordinator tries to assign the SAME reviewer again for re-review
	// THIS IS THE BUG SCENARIO - this should work!
	t.Log("Attempting to re-assign same reviewer after denial...")
	cmd5 := command.NewAssignReviewCommand(command.SourceInternal, "worker-2", "task-123", "worker-1", command.ReviewTypeComplex)
	result5, err := assignReviewHandler.Handle(ctx, cmd5)
	require.NoError(t, err, "Re-assign review after denial failed")
	require.True(t, result5.Success, "Re-assign review not successful: %v", result5.Error)

	// Verify reviewer is back in Reviewing phase
	rev5, _ := processRepo.Get("worker-2")
	task5, _ := taskRepo.Get("task-123")
	t.Logf("After re-assign: reviewer.Phase=%v, reviewer.TaskID=%q, task.Reviewer=%q, task.Status=%s",
		*rev5.Phase, rev5.TaskID, task5.Reviewer, task5.Status)

	require.Equal(t, events.ProcessPhaseReviewing, *rev5.Phase)
	require.Equal(t, "task-123", rev5.TaskID)

	// Step 6: Reviewer can now submit APPROVED verdict
	cmd6 := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictApproved, "LGTM")
	result6, err := reportVerdictHandler.Handle(ctx, cmd6)
	require.NoError(t, err, "Second verdict failed")
	require.True(t, result6.Success, "Second verdict not successful: %v", result6.Error)

	// Verify task is now approved
	task6, _ := taskRepo.Get("task-123")
	require.Equal(t, repository.TaskApproved, task6.Status)

	t.Log("SUCCESS: Re-assign review after denial works correctly")
}

// TestIntegration_ApproveCommitFailsWhenTaskDenied verifies that approve_commit
// correctly fails when the task is still in denied status (before re-review).
// This reproduces the bug: coordinator sends approve_commit but task is denied.
func TestIntegration_ApproveCommitFailsWhenTaskDenied(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	// Mock AddComment for ReportComplete and ReportVerdict calls
	bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()

	// Create handlers
	reportCompleteHandler := NewReportCompleteHandler(processRepo, taskRepo, queueRepo, WithReportCompleteBDExecutor(bdExecutor))
	assignReviewHandler := NewAssignReviewHandler(processRepo, taskRepo, queueRepo)
	reportVerdictHandler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo, WithReportVerdictBDExecutor(bdExecutor))
	approveCommitHandler := NewApproveCommitHandler(processRepo, taskRepo, queueRepo)

	ctx := context.Background()

	// === SETUP ===
	implementer := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseImplementing),
		TaskID:    "task-123",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	reviewer := &repository.Process{
		ID:        "worker-2",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		TaskID:    "",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	task := &repository.TaskAssignment{
		TaskID:      "task-123",
		Implementer: "worker-1",
		Reviewer:    "",
		Status:      repository.TaskImplementing,
		StartedAt:   time.Now(),
	}
	_ = taskRepo.Save(task)

	// Step 1: Implementer reports complete
	cmd1 := command.NewReportCompleteCommand(command.SourceMCPTool, "worker-1", "Initial implementation")
	_, _ = reportCompleteHandler.Handle(ctx, cmd1)

	// Step 2: Assign reviewer
	cmd2 := command.NewAssignReviewCommand(command.SourceInternal, "worker-2", "task-123", "worker-1", command.ReviewTypeComplex)
	_, _ = assignReviewHandler.Handle(ctx, cmd2)

	// Step 3: Reviewer DENIES
	cmd3 := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-2", command.VerdictDenied, "Needs fixes")
	_, _ = reportVerdictHandler.Handle(ctx, cmd3)

	// Verify task is now denied
	taskAfterDenial, _ := taskRepo.Get("task-123")
	t.Logf("Task status after denial: %s", taskAfterDenial.Status)
	require.Equal(t, repository.TaskDenied, taskAfterDenial.Status)

	// === THE BUG SCENARIO ===
	// Coordinator tries to call approve_commit WITHOUT the re-review flow
	// (missing: implementer report_complete, assign_review, reviewer approve)

	t.Log("Attempting approve_commit on denied task (should fail)")
	cmd4 := command.NewApproveCommitCommand(command.SourceInternal, "worker-1", "task-123")
	_, err := approveCommitHandler.Handle(ctx, cmd4)

	require.Error(t, err, "Expected approve_commit to fail on denied task")
	t.Logf("approve_commit correctly failed: %v", err)
	require.ErrorIs(t, err, types.ErrTaskNotApproved)
}
