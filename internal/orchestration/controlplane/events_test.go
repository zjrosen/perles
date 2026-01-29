package controlplane

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/fabric"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
)

func TestEventTypeConstants(t *testing.T) {
	// Test that all EventType constants are defined
	tests := []struct {
		name     string
		event    EventType
		expected string
	}{
		// Workflow lifecycle
		{"WorkflowCreated", EventWorkflowCreated, "workflow.created"},
		{"WorkflowStarted", EventWorkflowStarted, "workflow.started"},
		{"WorkflowPaused", EventWorkflowPaused, "workflow.paused"},
		{"WorkflowResumed", EventWorkflowResumed, "workflow.resumed"},
		{"WorkflowCompleted", EventWorkflowCompleted, "workflow.completed"},
		{"WorkflowFailed", EventWorkflowFailed, "workflow.failed"},
		// Coordinator events
		{"CoordinatorSpawned", EventCoordinatorSpawned, "coordinator.spawned"},
		{"CoordinatorReplaced", EventCoordinatorReplaced, "coordinator.replaced"},
		{"CoordinatorOutput", EventCoordinatorOutput, "coordinator.output"},
		// Worker events
		{"WorkerSpawned", EventWorkerSpawned, "worker.spawned"},
		{"WorkerRetired", EventWorkerRetired, "worker.retired"},
		{"WorkerOutput", EventWorkerOutput, "worker.output"},
		// Task events
		{"TaskAssigned", EventTaskAssigned, "task.assigned"},
		{"TaskCompleted", EventTaskCompleted, "task.completed"},
		{"TaskFailed", EventTaskFailed, "task.failed"},
		// Health events
		{"HealthUnhealthy", EventHealthUnhealthy, "health.unhealthy"},
		{"HealthStuck", EventHealthStuck, "health.stuck"},
		{"HealthRecovering", EventHealthRecovering, "health.recovering"},
		{"HealthRecovered", EventHealthRecovered, "health.recovered"},
		// Command log events
		{"CommandLog", EventCommandLog, "command.log"},
		// Fabric events
		{"FabricPosted", EventFabricPosted, "fabric.posted"},
		// Unknown
		{"Unknown", EventUnknown, "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, string(tc.event))
			require.Equal(t, tc.expected, tc.event.String())
		})
	}
}

func TestClassifyEvent_ProcessSpawned(t *testing.T) {
	tests := []struct {
		name     string
		event    events.ProcessEvent
		expected EventType
	}{
		{
			name: "CoordinatorSpawned",
			event: events.ProcessEvent{
				Type:      events.ProcessSpawned,
				Role:      events.RoleCoordinator,
				ProcessID: "coord-1",
			},
			expected: EventCoordinatorSpawned,
		},
		{
			name: "WorkerSpawned",
			event: events.ProcessEvent{
				Type:      events.ProcessSpawned,
				Role:      events.RoleWorker,
				ProcessID: "worker-1",
			},
			expected: EventWorkerSpawned,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyEvent(tc.event)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestClassifyEvent_ProcessOutput(t *testing.T) {
	tests := []struct {
		name     string
		event    events.ProcessEvent
		expected EventType
	}{
		{
			name: "CoordinatorOutput",
			event: events.ProcessEvent{
				Type:      events.ProcessOutput,
				Role:      events.RoleCoordinator,
				ProcessID: "coord-1",
				Output:    "thinking...",
			},
			expected: EventCoordinatorOutput,
		},
		{
			name: "WorkerOutput",
			event: events.ProcessEvent{
				Type:      events.ProcessOutput,
				Role:      events.RoleWorker,
				ProcessID: "worker-1",
				Output:    "implementing...",
			},
			expected: EventWorkerOutput,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyEvent(tc.event)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestClassifyEvent_TaskEvents(t *testing.T) {
	// Worker error maps to task failed
	event := events.ProcessEvent{
		Type:      events.ProcessError,
		Role:      events.RoleWorker,
		ProcessID: "worker-1",
	}
	result := ClassifyEvent(event)
	require.Equal(t, EventTaskFailed, result)
}

func TestClassifyEvent_CoordinatorError(t *testing.T) {
	// Coordinator errors should map to coordinator output so TUI displays them
	event := events.ProcessEvent{
		Type:      events.ProcessError,
		Role:      events.RoleCoordinator,
		ProcessID: "coordinator",
	}
	result := ClassifyEvent(event)
	require.Equal(t, EventCoordinatorOutput, result)
}

func TestClassifyEvent_UnknownEvents(t *testing.T) {
	tests := []struct {
		name  string
		event any
	}{
		{
			name:  "NonProcessEvent",
			event: "not a process event",
		},
		{
			name:  "NilEvent",
			event: nil,
		},
		{
			name: "UnknownEventType",
			event: events.ProcessEvent{
				Type: events.ProcessEventType("unknown_type"), // Truly unknown event type
				Role: events.RoleWorker,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyEvent(tc.event)
			require.Equal(t, EventUnknown, result)
		})
	}
}

func TestClassifyEvent_StatusChange(t *testing.T) {
	tests := []struct {
		name     string
		event    events.ProcessEvent
		expected EventType
	}{
		{
			name: "CoordinatorRetired",
			event: events.ProcessEvent{
				Type:   events.ProcessStatusChange,
				Role:   events.RoleCoordinator,
				Status: events.ProcessStatusRetired,
			},
			expected: EventCoordinatorReplaced,
		},
		{
			name: "WorkerRetired",
			event: events.ProcessEvent{
				Type:   events.ProcessStatusChange,
				Role:   events.RoleWorker,
				Status: events.ProcessStatusRetired,
			},
			expected: EventWorkerRetired,
		},
		{
			name: "CoordinatorWorkingStatus",
			event: events.ProcessEvent{
				Type:   events.ProcessStatusChange,
				Role:   events.RoleCoordinator,
				Status: events.ProcessStatusWorking,
			},
			expected: EventCoordinatorOutput,
		},
		{
			name: "WorkerWorkingStatus",
			event: events.ProcessEvent{
				Type:   events.ProcessStatusChange,
				Role:   events.RoleWorker,
				Status: events.ProcessStatusWorking,
			},
			expected: EventWorkerOutput,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyEvent(tc.event)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestClassifyEvent_WorkflowComplete(t *testing.T) {
	event := events.ProcessEvent{
		Type: events.ProcessWorkflowComplete,
		Role: events.RoleCoordinator,
	}
	result := ClassifyEvent(event)
	require.Equal(t, EventWorkflowCompleted, result)
}

func TestClassifyEvent_CommandLogEvent(t *testing.T) {
	event := processor.CommandLogEvent{
		CommandID:   "cmd-123",
		CommandType: "spawn_worker",
		Success:     true,
		Duration:    time.Millisecond * 100,
		Timestamp:   time.Now(),
	}
	result := ClassifyEvent(event)
	require.Equal(t, EventCommandLog, result)
}

func TestClassifyEvent_ReadyWorkingEvents(t *testing.T) {
	tests := []struct {
		name     string
		event    events.ProcessEvent
		expected EventType
	}{
		{
			name: "CoordinatorReady",
			event: events.ProcessEvent{
				Type:   events.ProcessReady,
				Role:   events.RoleCoordinator,
				Status: events.ProcessStatusReady,
			},
			expected: EventCoordinatorOutput,
		},
		{
			name: "CoordinatorWorking",
			event: events.ProcessEvent{
				Type:   events.ProcessWorking,
				Role:   events.RoleCoordinator,
				Status: events.ProcessStatusWorking,
			},
			expected: EventCoordinatorOutput,
		},
		{
			name: "WorkerReady",
			event: events.ProcessEvent{
				Type:   events.ProcessReady,
				Role:   events.RoleWorker,
				Status: events.ProcessStatusReady,
			},
			expected: EventWorkerOutput,
		},
		{
			name: "WorkerWorking",
			event: events.ProcessEvent{
				Type:   events.ProcessWorking,
				Role:   events.RoleWorker,
				Status: events.ProcessStatusWorking,
			},
			expected: EventWorkerOutput,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyEvent(tc.event)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestClassifyEvent_ProcessTokenUsage(t *testing.T) {
	tests := []struct {
		name     string
		event    events.ProcessEvent
		expected EventType
	}{
		{
			name: "CoordinatorTokenUsage",
			event: events.ProcessEvent{
				Type: events.ProcessTokenUsage,
				Role: events.RoleCoordinator,
			},
			expected: EventCoordinatorOutput,
		},
		{
			name: "WorkerTokenUsage",
			event: events.ProcessEvent{
				Type: events.ProcessTokenUsage,
				Role: events.RoleWorker,
			},
			expected: EventWorkerOutput,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyEvent(tc.event)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestClassifyEvent_ProcessQueueChanged(t *testing.T) {
	tests := []struct {
		name     string
		event    events.ProcessEvent
		expected EventType
	}{
		{
			name: "CoordinatorQueueChanged",
			event: events.ProcessEvent{
				Type:       events.ProcessQueueChanged,
				Role:       events.RoleCoordinator,
				QueueCount: 2,
			},
			expected: EventCoordinatorOutput,
		},
		{
			name: "WorkerQueueChanged",
			event: events.ProcessEvent{
				Type:       events.ProcessQueueChanged,
				Role:       events.RoleWorker,
				QueueCount: 3,
			},
			expected: EventWorkerOutput,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyEvent(tc.event)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestClassifyEvent_ProcessIncoming(t *testing.T) {
	tests := []struct {
		name     string
		event    events.ProcessEvent
		expected EventType
	}{
		{
			name: "CoordinatorIncoming",
			event: events.ProcessEvent{
				Type:    events.ProcessIncoming,
				Role:    events.RoleCoordinator,
				Message: "User message",
				Sender:  "user",
			},
			expected: EventCoordinatorIncoming,
		},
		{
			name: "WorkerIncoming",
			event: events.ProcessEvent{
				Type:    events.ProcessIncoming,
				Role:    events.RoleWorker,
				Message: "Task assignment",
				Sender:  "coordinator",
			},
			expected: EventWorkerIncoming,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyEvent(tc.event)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestIsLifecycleEvent(t *testing.T) {
	lifecycleEvents := []EventType{
		EventWorkflowCreated,
		EventWorkflowStarted,
		EventWorkflowPaused,
		EventWorkflowResumed,
		EventWorkflowCompleted,
		EventWorkflowFailed,
	}

	for _, e := range lifecycleEvents {
		t.Run(e.String(), func(t *testing.T) {
			require.True(t, e.IsLifecycleEvent())
		})
	}

	// Non-lifecycle events
	nonLifecycleEvents := []EventType{
		EventCoordinatorSpawned,
		EventWorkerSpawned,
		EventTaskAssigned,
		EventHealthUnhealthy,
		EventUnknown,
	}

	for _, e := range nonLifecycleEvents {
		t.Run(e.String()+"_not_lifecycle", func(t *testing.T) {
			require.False(t, e.IsLifecycleEvent())
		})
	}
}

func TestIsCoordinatorEvent(t *testing.T) {
	coordinatorEvents := []EventType{
		EventCoordinatorSpawned,
		EventCoordinatorReplaced,
		EventCoordinatorOutput,
	}

	for _, e := range coordinatorEvents {
		t.Run(e.String(), func(t *testing.T) {
			require.True(t, e.IsCoordinatorEvent())
		})
	}

	// Non-coordinator events
	nonCoordinatorEvents := []EventType{
		EventWorkerSpawned,
		EventTaskAssigned,
		EventWorkflowCreated,
	}

	for _, e := range nonCoordinatorEvents {
		t.Run(e.String()+"_not_coordinator", func(t *testing.T) {
			require.False(t, e.IsCoordinatorEvent())
		})
	}
}

func TestIsWorkerEvent(t *testing.T) {
	workerEvents := []EventType{
		EventWorkerSpawned,
		EventWorkerRetired,
		EventWorkerOutput,
	}

	for _, e := range workerEvents {
		t.Run(e.String(), func(t *testing.T) {
			require.True(t, e.IsWorkerEvent())
		})
	}

	// Non-worker events
	nonWorkerEvents := []EventType{
		EventCoordinatorSpawned,
		EventTaskAssigned,
		EventWorkflowCreated,
	}

	for _, e := range nonWorkerEvents {
		t.Run(e.String()+"_not_worker", func(t *testing.T) {
			require.False(t, e.IsWorkerEvent())
		})
	}
}

func TestIsTaskEvent(t *testing.T) {
	taskEvents := []EventType{
		EventTaskAssigned,
		EventTaskCompleted,
		EventTaskFailed,
	}

	for _, e := range taskEvents {
		t.Run(e.String(), func(t *testing.T) {
			require.True(t, e.IsTaskEvent())
		})
	}

	// Non-task events
	nonTaskEvents := []EventType{
		EventWorkerSpawned,
		EventCoordinatorSpawned,
	}

	for _, e := range nonTaskEvents {
		t.Run(e.String()+"_not_task", func(t *testing.T) {
			require.False(t, e.IsTaskEvent())
		})
	}
}

func TestIsHealthEvent(t *testing.T) {
	healthEvents := []EventType{
		EventHealthUnhealthy,
		EventHealthStuck,
		EventHealthRecovering,
		EventHealthRecovered,
	}

	for _, e := range healthEvents {
		t.Run(e.String(), func(t *testing.T) {
			require.True(t, e.IsHealthEvent())
		})
	}

	// Non-health events
	nonHealthEvents := []EventType{
		EventWorkerSpawned,
		EventTaskAssigned,
	}

	for _, e := range nonHealthEvents {
		t.Run(e.String()+"_not_health", func(t *testing.T) {
			require.False(t, e.IsHealthEvent())
		})
	}
}

func TestControlPlaneEvent_Serialization(t *testing.T) {
	event := ControlPlaneEvent{
		Type:         EventWorkflowStarted,
		Timestamp:    time.Date(2026, 1, 18, 12, 0, 0, 0, time.UTC),
		WorkflowID:   WorkflowID("test-workflow-123"),
		TemplateID:   "cook.md",
		WorkflowName: "Feature: Auth System",
		State:        WorkflowRunning,
		ProcessID:    "coord-1",
		TaskID:       "",
		Payload:      "started successfully",
	}

	// Test JSON serialization
	data, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded ControlPlaneEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Equal(t, event.Type, decoded.Type)
	require.Equal(t, event.WorkflowID, decoded.WorkflowID)
	require.Equal(t, event.TemplateID, decoded.TemplateID)
	require.Equal(t, event.WorkflowName, decoded.WorkflowName)
	require.Equal(t, event.State, decoded.State)
	require.Equal(t, event.ProcessID, decoded.ProcessID)
	require.Equal(t, event.TaskID, decoded.TaskID)
}

func TestControlPlaneEvent_WithWorkflow(t *testing.T) {
	inst := &WorkflowInstance{
		ID:         WorkflowID("wf-123"),
		TemplateID: "research.md",
		Name:       "Research: AI Patterns",
		State:      WorkflowRunning,
	}

	event := NewControlPlaneEvent(EventWorkflowStarted, nil).WithWorkflow(inst)

	require.Equal(t, inst.ID, event.WorkflowID)
	require.Equal(t, inst.TemplateID, event.TemplateID)
	require.Equal(t, inst.Name, event.WorkflowName)
	require.Equal(t, inst.State, event.State)
}

func TestControlPlaneEvent_WithProcess(t *testing.T) {
	event := NewControlPlaneEvent(EventCoordinatorOutput, nil).WithProcess("coord-1")
	require.Equal(t, "coord-1", event.ProcessID)
}

func TestControlPlaneEvent_WithTask(t *testing.T) {
	event := NewControlPlaneEvent(EventTaskAssigned, nil).WithTask("task-42")
	require.Equal(t, "task-42", event.TaskID)
}

func TestControlPlaneEvent_Chaining(t *testing.T) {
	inst := &WorkflowInstance{
		ID:         WorkflowID("wf-456"),
		TemplateID: "cook.md",
		Name:       "Bug Fix",
		State:      WorkflowRunning,
	}

	event := NewControlPlaneEvent(EventTaskAssigned, map[string]string{"task": "fix-auth"}).
		WithWorkflow(inst).
		WithProcess("worker-2").
		WithTask("task-99")

	require.Equal(t, EventTaskAssigned, event.Type)
	require.Equal(t, inst.ID, event.WorkflowID)
	require.Equal(t, inst.TemplateID, event.TemplateID)
	require.Equal(t, "worker-2", event.ProcessID)
	require.Equal(t, "task-99", event.TaskID)
	require.NotNil(t, event.Payload)
}

// === EventFilter Tests ===

func TestEventFilter_EmptyFilter_MatchesAllEvents(t *testing.T) {
	filter := EventFilter{}

	events := []ControlPlaneEvent{
		{Type: EventWorkflowStarted, WorkflowID: "wf-1"},
		{Type: EventWorkerSpawned, WorkflowID: "wf-2"},
		{Type: EventCoordinatorOutput, WorkflowID: "wf-3"},
	}

	for _, e := range events {
		require.True(t, filter.Matches(e), "empty filter should match event: %+v", e)
	}
}

func TestEventFilter_TypesFilter(t *testing.T) {
	filter := EventFilter{
		Types: []EventType{EventWorkerSpawned, EventWorkerRetired},
	}

	tests := []struct {
		name     string
		event    ControlPlaneEvent
		expected bool
	}{
		{
			name:     "MatchesWorkerSpawned",
			event:    ControlPlaneEvent{Type: EventWorkerSpawned},
			expected: true,
		},
		{
			name:     "MatchesWorkerRetired",
			event:    ControlPlaneEvent{Type: EventWorkerRetired},
			expected: true,
		},
		{
			name:     "DoesNotMatchCoordinatorOutput",
			event:    ControlPlaneEvent{Type: EventCoordinatorOutput},
			expected: false,
		},
		{
			name:     "DoesNotMatchTaskAssigned",
			event:    ControlPlaneEvent{Type: EventTaskAssigned},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, filter.Matches(tc.event))
		})
	}
}

func TestEventFilter_WorkflowIDsFilter(t *testing.T) {
	filter := EventFilter{
		WorkflowIDs: []WorkflowID{"wf-1", "wf-2"},
	}

	tests := []struct {
		name     string
		event    ControlPlaneEvent
		expected bool
	}{
		{
			name:     "MatchesWorkflow1",
			event:    ControlPlaneEvent{Type: EventWorkerSpawned, WorkflowID: "wf-1"},
			expected: true,
		},
		{
			name:     "MatchesWorkflow2",
			event:    ControlPlaneEvent{Type: EventWorkerSpawned, WorkflowID: "wf-2"},
			expected: true,
		},
		{
			name:     "DoesNotMatchWorkflow3",
			event:    ControlPlaneEvent{Type: EventWorkerSpawned, WorkflowID: "wf-3"},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, filter.Matches(tc.event))
		})
	}
}

func TestEventFilter_ExcludeTypesFilter(t *testing.T) {
	filter := EventFilter{
		ExcludeTypes: []EventType{EventCoordinatorOutput, EventWorkerOutput},
	}

	tests := []struct {
		name     string
		event    ControlPlaneEvent
		expected bool
	}{
		{
			name:     "PassesWorkerSpawned",
			event:    ControlPlaneEvent{Type: EventWorkerSpawned},
			expected: true,
		},
		{
			name:     "PassesTaskCompleted",
			event:    ControlPlaneEvent{Type: EventTaskCompleted},
			expected: true,
		},
		{
			name:     "ExcludesCoordinatorOutput",
			event:    ControlPlaneEvent{Type: EventCoordinatorOutput},
			expected: false,
		},
		{
			name:     "ExcludesWorkerOutput",
			event:    ControlPlaneEvent{Type: EventWorkerOutput},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, filter.Matches(tc.event))
		})
	}
}

func TestEventFilter_CombinedFilters(t *testing.T) {
	// Filter: Include only worker events from wf-1, but exclude output
	filter := EventFilter{
		Types:        []EventType{EventWorkerSpawned, EventWorkerRetired, EventWorkerOutput},
		WorkflowIDs:  []WorkflowID{"wf-1"},
		ExcludeTypes: []EventType{EventWorkerOutput},
	}

	tests := []struct {
		name     string
		event    ControlPlaneEvent
		expected bool
	}{
		{
			name:     "PassesWorkerSpawnedFromWf1",
			event:    ControlPlaneEvent{Type: EventWorkerSpawned, WorkflowID: "wf-1"},
			expected: true,
		},
		{
			name:     "PassesWorkerRetiredFromWf1",
			event:    ControlPlaneEvent{Type: EventWorkerRetired, WorkflowID: "wf-1"},
			expected: true,
		},
		{
			name:     "FailsWorkerSpawnedFromWf2_WrongWorkflow",
			event:    ControlPlaneEvent{Type: EventWorkerSpawned, WorkflowID: "wf-2"},
			expected: false,
		},
		{
			name:     "FailsCoordinatorOutputFromWf1_WrongType",
			event:    ControlPlaneEvent{Type: EventCoordinatorOutput, WorkflowID: "wf-1"},
			expected: false,
		},
		{
			name:     "FailsWorkerOutputFromWf1_Excluded",
			event:    ControlPlaneEvent{Type: EventWorkerOutput, WorkflowID: "wf-1"},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, filter.Matches(tc.event))
		})
	}
}

func TestEventFilter_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		filter   EventFilter
		expected bool
	}{
		{
			name:     "EmptyFilter",
			filter:   EventFilter{},
			expected: true,
		},
		{
			name:     "WithTypes",
			filter:   EventFilter{Types: []EventType{EventWorkerSpawned}},
			expected: false,
		},
		{
			name:     "WithWorkflowIDs",
			filter:   EventFilter{WorkflowIDs: []WorkflowID{"wf-1"}},
			expected: false,
		},
		{
			name:     "WithExcludeTypes",
			filter:   EventFilter{ExcludeTypes: []EventType{EventWorkerOutput}},
			expected: false,
		},
		{
			name:     "WithAllEmpty",
			filter:   EventFilter{Types: []EventType{}, WorkflowIDs: []WorkflowID{}, ExcludeTypes: []EventType{}},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.filter.IsEmpty())
		})
	}
}

func TestEventFilter_MultipleWorkflowIDs(t *testing.T) {
	filter := EventFilter{
		WorkflowIDs: []WorkflowID{"wf-a", "wf-b", "wf-c"},
	}

	// Should match any of the specified workflows
	require.True(t, filter.Matches(ControlPlaneEvent{WorkflowID: "wf-a"}))
	require.True(t, filter.Matches(ControlPlaneEvent{WorkflowID: "wf-b"}))
	require.True(t, filter.Matches(ControlPlaneEvent{WorkflowID: "wf-c"}))
	require.False(t, filter.Matches(ControlPlaneEvent{WorkflowID: "wf-d"}))
}

func TestEventFilter_MultipleExcludeTypes(t *testing.T) {
	filter := EventFilter{
		ExcludeTypes: []EventType{
			EventCoordinatorOutput,
			EventWorkerOutput,
			EventHealthUnhealthy,
		},
	}

	// Should exclude any of the specified types
	require.False(t, filter.Matches(ControlPlaneEvent{Type: EventCoordinatorOutput}))
	require.False(t, filter.Matches(ControlPlaneEvent{Type: EventWorkerOutput}))
	require.False(t, filter.Matches(ControlPlaneEvent{Type: EventHealthUnhealthy}))
	require.True(t, filter.Matches(ControlPlaneEvent{Type: EventWorkerSpawned}))
}

// === Fabric Event Tests ===

func TestEventType_EventFabricPosted(t *testing.T) {
	// Verify constant exists with correct value
	require.Equal(t, "fabric.posted", string(EventFabricPosted))
	require.Equal(t, "fabric.posted", EventFabricPosted.String())
}

func TestIsFabricEvent_True(t *testing.T) {
	// Verify IsFabricEvent() returns true for EventFabricPosted
	require.True(t, EventFabricPosted.IsFabricEvent())
}

func TestIsFabricEvent_False(t *testing.T) {
	// Verify IsFabricEvent() returns false for other event types
	nonFabricEvents := []EventType{
		EventWorkflowCreated,
		EventWorkflowStarted,
		EventCoordinatorSpawned,
		EventCoordinatorOutput,
		EventWorkerSpawned,
		EventWorkerOutput,
		EventTaskAssigned,
		EventHealthUnhealthy,
		EventCommandLog,
		EventUnknown,
	}

	for _, e := range nonFabricEvents {
		t.Run(e.String()+"_not_fabric", func(t *testing.T) {
			require.False(t, e.IsFabricEvent())
		})
	}
}

func TestClassifyEvent_FabricEvent(t *testing.T) {
	// Verify fabric.Event is classified as EventFabricPosted
	fabricEvent := fabric.Event{
		Type:        fabric.EventMessagePosted,
		ChannelID:   "channel-123",
		ChannelSlug: "tasks",
		AgentID:     "worker-1",
	}

	result := ClassifyEvent(fabricEvent)
	require.Equal(t, EventFabricPosted, result)
}

func TestClassifyEvent_OtherTypes_Unchanged(t *testing.T) {
	// Verify existing classifications still work after adding fabric.Event support
	tests := []struct {
		name     string
		event    any
		expected EventType
	}{
		{
			name: "ProcessEventStillWorks",
			event: events.ProcessEvent{
				Type:      events.ProcessSpawned,
				Role:      events.RoleCoordinator,
				ProcessID: "coord-1",
			},
			expected: EventCoordinatorSpawned,
		},
		{
			name: "CommandLogEventStillWorks",
			event: processor.CommandLogEvent{
				CommandID:   "cmd-123",
				CommandType: "spawn_worker",
				Success:     true,
				Duration:    time.Millisecond * 100,
				Timestamp:   time.Now(),
			},
			expected: EventCommandLog,
		},
		{
			name:     "UnknownEventStillWorks",
			event:    "not a recognized event",
			expected: EventUnknown,
		},
		{
			name: "WorkerEventStillWorks",
			event: events.ProcessEvent{
				Type:      events.ProcessOutput,
				Role:      events.RoleWorker,
				ProcessID: "worker-1",
				Output:    "implementing...",
			},
			expected: EventWorkerOutput,
		},
		{
			name: "WorkflowCompleteStillWorks",
			event: events.ProcessEvent{
				Type: events.ProcessWorkflowComplete,
				Role: events.RoleCoordinator,
			},
			expected: EventWorkflowCompleted,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ClassifyEvent(tc.event)
			require.Equal(t, tc.expected, result)
		})
	}
}
