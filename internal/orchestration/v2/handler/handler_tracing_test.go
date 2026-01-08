package handler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/tracing"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// ===========================================================================
// Test Helpers
// ===========================================================================

// setupTestTracer creates a test tracer with an in-memory exporter.
func setupTestTracer(t *testing.T) (trace.Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	tracer := provider.Tracer("test-tracer")
	return tracer, exporter
}

// getSpanByName finds a span by name from the exporter.
func getSpanByName(exporter *tracetest.InMemoryExporter, name string) (tracetest.SpanStub, bool) {
	for _, span := range exporter.GetSpans() {
		if span.Name == name {
			return span, true
		}
	}
	return tracetest.SpanStub{}, false
}

// getAttributeValue extracts an attribute value from a span.
func getAttributeValue(span tracetest.SpanStub, key string) (attribute.Value, bool) {
	for _, attr := range span.Attributes {
		if string(attr.Key) == key {
			return attr.Value, true
		}
	}
	return attribute.Value{}, false
}

// hasEvent checks if a span has an event with the given name.
func hasEvent(span tracetest.SpanStub, eventName string) bool {
	for _, event := range span.Events {
		if event.Name == eventName {
			return true
		}
	}
	return false
}

// getEventAttributeValue extracts an attribute value from a specific event in a span.
func getEventAttributeValue(span tracetest.SpanStub, eventName, attrKey string) (attribute.Value, bool) {
	for _, event := range span.Events {
		if event.Name == eventName {
			for _, attr := range event.Attributes {
				if string(attr.Key) == attrKey {
					return attr.Value, true
				}
			}
		}
	}
	return attribute.Value{}, false
}

// ===========================================================================
// AssignTaskHandler Tracing Tests
// ===========================================================================

func TestAssignTaskHandler_Tracing_CreatesSpan(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().ShowIssue(mock.Anything).Return(&beads.Issue{ID: "task-123", Status: beads.StatusOpen}, nil)
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil)

	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo,
		WithBDExecutor(bdExecutor),
		WithQueueRepository(queueRepo),
		WithAssignTaskTracer(tracer),
	)

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "task-123", "Test task")
	_, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Verify span was created with correct name
	span, found := getSpanByName(exporter, tracing.SpanPrefixHandler+"assign_task")
	require.True(t, found, "expected span 'handler.assign_task' to be created")

	// Verify span attributes
	taskID, found := getAttributeValue(span, tracing.AttrTaskID)
	require.True(t, found, "expected task.id attribute")
	require.Equal(t, "task-123", taskID.AsString())

	workerID, found := getAttributeValue(span, tracing.AttrWorkerID)
	require.True(t, found, "expected worker.id attribute")
	require.Equal(t, "worker-1", workerID.AsString())

	// Verify span status is OK
	require.Equal(t, codes.Ok, span.Status.Code)
}

func TestAssignTaskHandler_Tracing_RecordsEvents(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().ShowIssue(mock.Anything).Return(&beads.Issue{ID: "task-123", Status: beads.StatusOpen}, nil)
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil)

	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo,
		WithBDExecutor(bdExecutor),
		WithQueueRepository(queueRepo),
		WithAssignTaskTracer(tracer),
	)

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "task-123", "Test task")
	_, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)

	span, found := getSpanByName(exporter, tracing.SpanPrefixHandler+"assign_task")
	require.True(t, found)

	// Verify span events
	require.True(t, hasEvent(span, tracing.EventWorkerLookup), "expected worker.lookup event")
	require.True(t, hasEvent(span, tracing.EventTaskValidated), "expected task.validated event")
	require.True(t, hasEvent(span, tracing.EventTaskAssigned), "expected task.assigned event")
}

func TestAssignTaskHandler_Tracing_RecordsErrorOnFailure(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockBeadsExecutor(t)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo,
		WithBDExecutor(bdExecutor),
		WithQueueRepository(queueRepo),
		WithAssignTaskTracer(tracer),
	)

	// Try to assign to non-existent worker
	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "unknown-worker", "task-123", "Test task")
	_, err := handler.Handle(context.Background(), cmd)
	require.Error(t, err)

	span, found := getSpanByName(exporter, tracing.SpanPrefixHandler+"assign_task")
	require.True(t, found)

	// Verify span status is Error
	require.Equal(t, codes.Error, span.Status.Code)
}

func TestAssignTaskHandler_Tracing_WorksWithNilTracer(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().ShowIssue(mock.Anything).Return(&beads.Issue{ID: "task-123", Status: beads.StatusOpen}, nil)
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil)

	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	queueRepo := repository.NewMemoryQueueRepository(0)
	// No tracer configured
	handler := NewAssignTaskHandler(processRepo, taskRepo,
		WithBDExecutor(bdExecutor),
		WithQueueRepository(queueRepo),
	)

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "task-123", "Test task")
	result, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, result.Success)
}

// ===========================================================================
// SpawnProcessHandler Tracing Tests
// ===========================================================================

func TestSpawnProcessHandler_Tracing_CreatesSpan(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	processRepo := repository.NewMemoryProcessRepository()
	var registry *process.ProcessRegistry // No registry needed for basic test

	handler := NewSpawnProcessHandler(processRepo, registry,
		WithSpawnProcessTracer(tracer),
	)

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleWorker)
	cmd.ProcessID = "worker-test"
	_, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Verify span was created with correct name
	span, found := getSpanByName(exporter, tracing.SpanPrefixHandler+"spawn_process")
	require.True(t, found, "expected span 'handler.spawn_process' to be created")

	// Verify span attributes
	processRole, found := getAttributeValue(span, tracing.AttrProcessRole)
	require.True(t, found, "expected process.role attribute")
	require.Equal(t, string(repository.RoleWorker), processRole.AsString())

	processID, found := getAttributeValue(span, tracing.AttrProcessID)
	require.True(t, found, "expected process.id attribute")
	require.Equal(t, "worker-test", processID.AsString())

	// Verify span status is OK
	require.Equal(t, codes.Ok, span.Status.Code)
}

func TestSpawnProcessHandler_Tracing_RecordsErrorOnFailure(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	processRepo := repository.NewMemoryProcessRepository()
	var registry *process.ProcessRegistry

	// Add existing coordinator to trigger error
	coord := &repository.Process{
		ID:        repository.CoordinatorID,
		Role:      repository.RoleCoordinator,
		Status:    repository.StatusReady,
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(coord)

	handler := NewSpawnProcessHandler(processRepo, registry,
		WithSpawnProcessTracer(tracer),
	)

	// Try to spawn another coordinator (should fail)
	cmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleCoordinator)
	_, err := handler.Handle(context.Background(), cmd)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrCoordinatorExists)

	span, found := getSpanByName(exporter, tracing.SpanPrefixHandler+"spawn_process")
	require.True(t, found)

	// Verify span status is Error
	require.Equal(t, codes.Error, span.Status.Code)
}

func TestSpawnProcessHandler_Tracing_WorksWithNilTracer(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	var registry *process.ProcessRegistry

	// No tracer configured
	handler := NewSpawnProcessHandler(processRepo, registry)

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleWorker)
	cmd.ProcessID = "worker-test"
	result, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, result.Success)
}

// ===========================================================================
// ReportVerdictHandler Tracing Tests
// ===========================================================================

func TestReportVerdictHandler_Tracing_CreatesSpan(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(nil)

	// Setup reviewer in reviewing phase
	reviewer := &repository.Process{
		ID:        "worker-reviewer",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing),
		TaskID:    "task-123",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	// Setup implementer
	implementer := &repository.Process{
		ID:        "worker-impl",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "task-123",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	// Setup task
	task := &repository.TaskAssignment{
		TaskID:      "task-123",
		Implementer: "worker-impl",
		Reviewer:    "worker-reviewer",
		Status:      repository.TaskInReview,
		StartedAt:   time.Now(),
	}
	taskRepo.Save(task)

	handler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo,
		WithReportVerdictBDExecutor(bdExecutor),
		WithReportVerdictTracer(tracer),
	)

	cmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-reviewer", command.VerdictApproved, "Looks good!")
	_, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Verify span was created with correct name
	span, found := getSpanByName(exporter, tracing.SpanPrefixHandler+"report_verdict")
	require.True(t, found, "expected span 'handler.report_verdict' to be created")

	// Verify span attributes
	verdict, found := getAttributeValue(span, tracing.AttrVerdict)
	require.True(t, found, "expected verdict attribute")
	require.Equal(t, string(command.VerdictApproved), verdict.AsString())

	reviewerID, found := getAttributeValue(span, tracing.AttrReviewerID)
	require.True(t, found, "expected reviewer.id attribute")
	require.Equal(t, "worker-reviewer", reviewerID.AsString())

	taskID, found := getAttributeValue(span, tracing.AttrTaskID)
	require.True(t, found, "expected task.id attribute")
	require.Equal(t, "task-123", taskID.AsString())

	implementerID, found := getAttributeValue(span, tracing.AttrImplementerID)
	require.True(t, found, "expected implementer.id attribute")
	require.Equal(t, "worker-impl", implementerID.AsString())

	// Verify span status is OK
	require.Equal(t, codes.Ok, span.Status.Code)
}

func TestReportVerdictHandler_Tracing_RecordsErrorOnFailure(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)

	handler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo,
		WithReportVerdictBDExecutor(bdExecutor),
		WithReportVerdictTracer(tracer),
	)

	// Try to report verdict for non-existent reviewer
	cmd := command.NewReportVerdictCommand(command.SourceMCPTool, "unknown-worker", command.VerdictApproved, "")
	_, err := handler.Handle(context.Background(), cmd)
	require.Error(t, err)

	span, found := getSpanByName(exporter, tracing.SpanPrefixHandler+"report_verdict")
	require.True(t, found)

	// Verify span status is Error
	require.Equal(t, codes.Error, span.Status.Code)
}

func TestReportVerdictHandler_Tracing_WorksWithNilTracer(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(nil)

	// Setup reviewer in reviewing phase
	reviewer := &repository.Process{
		ID:        "worker-reviewer",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing),
		TaskID:    "task-123",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	// Setup implementer
	implementer := &repository.Process{
		ID:        "worker-impl",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "task-123",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	// Setup task
	task := &repository.TaskAssignment{
		TaskID:      "task-123",
		Implementer: "worker-impl",
		Reviewer:    "worker-reviewer",
		Status:      repository.TaskInReview,
		StartedAt:   time.Now(),
	}
	taskRepo.Save(task)

	// No tracer configured
	handler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo,
		WithReportVerdictBDExecutor(bdExecutor),
	)

	cmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-reviewer", command.VerdictApproved, "")
	result, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, result.Success)
}

// ===========================================================================
// SendToProcessHandler Tracing Tests
// ===========================================================================

func TestSendToProcessHandler_Tracing_CreatesSpan(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)

	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	handler := NewSendToProcessHandler(processRepo, queueRepo,
		WithSendToProcessTracer(tracer),
	)

	cmd := command.NewSendToProcessCommand(command.SourceMCPTool, "worker-1", "Hello worker!")
	_, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)

	// Verify span was created with correct name
	span, found := getSpanByName(exporter, tracing.SpanPrefixHandler+"send_to_process")
	require.True(t, found, "expected span 'handler.send_to_process' to be created")

	// Verify span attributes
	processID, found := getAttributeValue(span, tracing.AttrProcessID)
	require.True(t, found, "expected process.id attribute")
	require.Equal(t, "worker-1", processID.AsString())

	// Verify span status is OK
	require.Equal(t, codes.Ok, span.Status.Code)
}

func TestSendToProcessHandler_Tracing_RecordsMessageQueuedEvent(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)

	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	handler := NewSendToProcessHandler(processRepo, queueRepo,
		WithSendToProcessTracer(tracer),
	)

	cmd := command.NewSendToProcessCommand(command.SourceMCPTool, "worker-1", "Hello worker!")
	_, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)

	span, found := getSpanByName(exporter, tracing.SpanPrefixHandler+"send_to_process")
	require.True(t, found)

	// Verify message.queued event was recorded
	require.True(t, hasEvent(span, tracing.EventMessageQueued), "expected message.queued event")

	// Verify event has queue.size attribute
	queueSize, found := getEventAttributeValue(span, tracing.EventMessageQueued, "queue.size")
	require.True(t, found, "expected queue.size attribute in message.queued event")
	require.Equal(t, int64(1), queueSize.AsInt64())
}

func TestSendToProcessHandler_Tracing_RecordsErrorOnFailure(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)

	handler := NewSendToProcessHandler(processRepo, queueRepo,
		WithSendToProcessTracer(tracer),
	)

	// Try to send to non-existent process
	cmd := command.NewSendToProcessCommand(command.SourceMCPTool, "unknown-process", "Hello!")
	_, err := handler.Handle(context.Background(), cmd)
	require.Error(t, err)

	span, found := getSpanByName(exporter, tracing.SpanPrefixHandler+"send_to_process")
	require.True(t, found)

	// Verify span status is Error
	require.Equal(t, codes.Error, span.Status.Code)
}

func TestSendToProcessHandler_Tracing_WorksWithNilTracer(t *testing.T) {
	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)

	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	// No tracer configured
	handler := NewSendToProcessHandler(processRepo, queueRepo)

	cmd := command.NewSendToProcessCommand(command.SourceMCPTool, "worker-1", "Hello worker!")
	result, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)
	require.True(t, result.Success)
}

// ===========================================================================
// Span End Verification Tests
// ===========================================================================

func TestAssignTaskHandler_Tracing_SpanEndsOnSuccess(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().ShowIssue(mock.Anything).Return(&beads.Issue{ID: "task-123", Status: beads.StatusOpen}, nil)
	bdExecutor.EXPECT().UpdateStatus(mock.Anything, mock.Anything).Return(nil)

	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     phasePtr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	queueRepo := repository.NewMemoryQueueRepository(0)
	handler := NewAssignTaskHandler(processRepo, taskRepo,
		WithBDExecutor(bdExecutor),
		WithQueueRepository(queueRepo),
		WithAssignTaskTracer(tracer),
	)

	cmd := command.NewAssignTaskCommand(command.SourceMCPTool, "worker-1", "task-123", "Test task")
	_, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)

	span, found := getSpanByName(exporter, tracing.SpanPrefixHandler+"assign_task")
	require.True(t, found)

	// Verify span has ended (EndTime is set)
	require.False(t, span.EndTime.IsZero(), "expected span to have ended")
}

func TestSendToProcessHandler_Tracing_SpanEndsOnSuccess(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)

	proc := &repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(proc)

	handler := NewSendToProcessHandler(processRepo, queueRepo,
		WithSendToProcessTracer(tracer),
	)

	cmd := command.NewSendToProcessCommand(command.SourceMCPTool, "worker-1", "Hello!")
	_, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)

	span, found := getSpanByName(exporter, tracing.SpanPrefixHandler+"send_to_process")
	require.True(t, found)

	// Verify span has ended (EndTime is set)
	require.False(t, span.EndTime.IsZero(), "expected span to have ended")
}

func TestSpawnProcessHandler_Tracing_SpanEndsOnSuccess(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	processRepo := repository.NewMemoryProcessRepository()
	var registry *process.ProcessRegistry

	handler := NewSpawnProcessHandler(processRepo, registry,
		WithSpawnProcessTracer(tracer),
	)

	cmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleWorker)
	cmd.ProcessID = "worker-test"
	_, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)

	span, found := getSpanByName(exporter, tracing.SpanPrefixHandler+"spawn_process")
	require.True(t, found)

	// Verify span has ended (EndTime is set)
	require.False(t, span.EndTime.IsZero(), "expected span to have ended")
}

func TestReportVerdictHandler_Tracing_SpanEndsOnSuccess(t *testing.T) {
	tracer, exporter := setupTestTracer(t)
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(0)
	bdExecutor := mocks.NewMockBeadsExecutor(t)
	bdExecutor.EXPECT().AddComment(mock.Anything, mock.Anything, mock.Anything).Return(nil)

	reviewer := &repository.Process{
		ID:        "worker-reviewer",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseReviewing),
		TaskID:    "task-123",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(reviewer)

	implementer := &repository.Process{
		ID:        "worker-impl",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     phasePtr(events.ProcessPhaseAwaitingReview),
		TaskID:    "task-123",
		CreatedAt: time.Now(),
	}
	processRepo.AddProcess(implementer)

	task := &repository.TaskAssignment{
		TaskID:      "task-123",
		Implementer: "worker-impl",
		Reviewer:    "worker-reviewer",
		Status:      repository.TaskInReview,
		StartedAt:   time.Now(),
	}
	taskRepo.Save(task)

	handler := NewReportVerdictHandler(processRepo, taskRepo, queueRepo,
		WithReportVerdictBDExecutor(bdExecutor),
		WithReportVerdictTracer(tracer),
	)

	cmd := command.NewReportVerdictCommand(command.SourceMCPTool, "worker-reviewer", command.VerdictApproved, "")
	_, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)

	span, found := getSpanByName(exporter, tracing.SpanPrefixHandler+"report_verdict")
	require.True(t, found)

	// Verify span has ended (EndTime is set)
	require.False(t, span.EndTime.IsZero(), "expected span to have ended")
}
