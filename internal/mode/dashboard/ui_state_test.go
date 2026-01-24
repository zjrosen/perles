package dashboard

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	beads "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/mode"
	"github.com/zjrosen/perles/internal/orchestration/controlplane"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
	"github.com/zjrosen/perles/internal/ui/tree"
)

func TestNewWorkflowUIState_CreatesEmptyState(t *testing.T) {
	state := NewWorkflowUIState()

	require.NotNil(t, state)
	require.Empty(t, state.CoordinatorMessages)
	require.Empty(t, state.MessageEntries)
	require.Empty(t, state.WorkerIDs)
	require.Nil(t, state.CoordinatorMetrics)
	require.Equal(t, events.ProcessStatus(""), state.CoordinatorStatus)
	require.Equal(t, 0, state.CoordinatorQueueCount)
	require.Equal(t, 0.0, state.CoordinatorScrollPercent)
	require.Equal(t, 0.0, state.MessageScrollPercent)
	require.True(t, state.LastUpdated.IsZero())
}

func TestNewWorkflowUIState_AllMapsInitialized(t *testing.T) {
	state := NewWorkflowUIState()

	// All maps should be initialized (not nil) to prevent nil panics
	require.NotNil(t, state.WorkerStatus, "WorkerStatus map should be initialized")
	require.NotNil(t, state.WorkerPhases, "WorkerPhases map should be initialized")
	require.NotNil(t, state.WorkerMessages, "WorkerMessages map should be initialized")
	require.NotNil(t, state.WorkerMetrics, "WorkerMetrics map should be initialized")
	require.NotNil(t, state.WorkerQueueCounts, "WorkerQueueCounts map should be initialized")
	require.NotNil(t, state.WorkerScrollPercents, "WorkerScrollPercents map should be initialized")

	// Verify maps are usable without panic
	state.WorkerStatus["worker-1"] = events.ProcessStatusReady
	state.WorkerPhases["worker-1"] = events.ProcessPhaseIdle
	state.WorkerMessages["worker-1"] = []chatrender.Message{{Role: "assistant", Content: "test"}}
	state.WorkerMetrics["worker-1"] = &metrics.TokenMetrics{TokensUsed: 100}
	state.WorkerQueueCounts["worker-1"] = 5
	state.WorkerScrollPercents["worker-1"] = 0.5

	require.Equal(t, events.ProcessStatusReady, state.WorkerStatus["worker-1"])
	require.Equal(t, events.ProcessPhaseIdle, state.WorkerPhases["worker-1"])
	require.Len(t, state.WorkerMessages["worker-1"], 1)
	require.Equal(t, 100, state.WorkerMetrics["worker-1"].TokensUsed)
	require.Equal(t, 5, state.WorkerQueueCounts["worker-1"])
	require.Equal(t, 0.5, state.WorkerScrollPercents["worker-1"])
}

func TestWorkflowUIState_IsEmpty_ReturnsTrueForNewState(t *testing.T) {
	state := NewWorkflowUIState()

	require.True(t, state.IsEmpty())
}

func TestWorkflowUIState_IsEmpty_ReturnsFalseAfterAddingCoordinatorMessage(t *testing.T) {
	state := NewWorkflowUIState()
	state.CoordinatorMessages = append(state.CoordinatorMessages, chatrender.Message{
		Role:    "assistant",
		Content: "Hello from coordinator",
	})

	require.False(t, state.IsEmpty())
}

func TestWorkflowUIState_IsEmpty_ReturnsFalseAfterAddingMessageEntry(t *testing.T) {
	state := NewWorkflowUIState()
	state.MessageEntries = append(state.MessageEntries, message.Entry{
		ID:      "msg-1",
		From:    "COORDINATOR",
		To:      "WORKER.1",
		Content: "Task assignment",
	})

	require.False(t, state.IsEmpty())
}

func TestWorkflowUIState_IsEmpty_ReturnsFalseAfterAddingWorker(t *testing.T) {
	state := NewWorkflowUIState()
	state.WorkerIDs = append(state.WorkerIDs, "worker-1")

	require.False(t, state.IsEmpty())
}

func TestWorkflowUIState_IsEmpty_WorkerMapsDoNotAffectEmptyCheck(t *testing.T) {
	// IsEmpty should only consider content (messages, entries, worker IDs)
	// not metadata like status, metrics, or scroll positions
	state := NewWorkflowUIState()

	// Add metadata but no actual content
	state.CoordinatorStatus = events.ProcessStatusReady
	state.CoordinatorMetrics = &metrics.TokenMetrics{TokensUsed: 1000}
	state.CoordinatorQueueCount = 3
	state.CoordinatorScrollPercent = 0.75
	state.MessageScrollPercent = 0.5
	state.LastUpdated = time.Now()

	// State should still be empty since there's no content
	require.True(t, state.IsEmpty())
}

func TestWorkflowUIState_AllSlicesInitialized(t *testing.T) {
	state := NewWorkflowUIState()

	// All slices should be initialized (not nil) for safe appending
	require.NotNil(t, state.CoordinatorMessages, "CoordinatorMessages slice should be initialized")
	require.NotNil(t, state.MessageEntries, "MessageEntries slice should be initialized")
	require.NotNil(t, state.WorkerIDs, "WorkerIDs slice should be initialized")

	// Verify slices are usable without panic
	state.CoordinatorMessages = append(state.CoordinatorMessages, chatrender.Message{Role: "test"})
	state.MessageEntries = append(state.MessageEntries, message.Entry{ID: "test"})
	state.WorkerIDs = append(state.WorkerIDs, "worker-1")

	require.Len(t, state.CoordinatorMessages, 1)
	require.Len(t, state.MessageEntries, 1)
	require.Len(t, state.WorkerIDs, 1)
}

// === Unit Tests: State Isolation ===

func TestModel_StateIsolation_WorkflowAStateDoesNotLeakToWorkflowB(t *testing.T) {
	// Test that workflow A's UI state is isolated from workflow B
	stateA := NewWorkflowUIState()
	stateB := NewWorkflowUIState()

	// Populate state A with data
	stateA.CoordinatorMessages = []chatrender.Message{
		{Role: "assistant", Content: "Message for workflow A"},
	}
	stateA.MessageEntries = []message.Entry{
		{ID: "msg-a-1", From: "COORDINATOR", To: "WORKER.1", Content: "Task A"},
	}
	stateA.WorkerIDs = []string{"worker-a-1", "worker-a-2"}
	stateA.CoordinatorScrollPercent = 0.75
	stateA.MessageScrollPercent = 0.50

	// Populate state B with different data
	stateB.CoordinatorMessages = []chatrender.Message{
		{Role: "user", Content: "Message for workflow B"},
	}
	stateB.MessageEntries = []message.Entry{
		{ID: "msg-b-1", From: "COORDINATOR", To: "WORKER.2", Content: "Task B"},
	}
	stateB.WorkerIDs = []string{"worker-b-1"}
	stateB.CoordinatorScrollPercent = 0.25
	stateB.MessageScrollPercent = 0.10

	// Verify states are completely independent
	require.NotEqual(t, stateA.CoordinatorMessages[0].Content, stateB.CoordinatorMessages[0].Content)
	require.NotEqual(t, stateA.MessageEntries[0].ID, stateB.MessageEntries[0].ID)
	require.NotEqual(t, len(stateA.WorkerIDs), len(stateB.WorkerIDs))
	require.NotEqual(t, stateA.CoordinatorScrollPercent, stateB.CoordinatorScrollPercent)
	require.NotEqual(t, stateA.MessageScrollPercent, stateB.MessageScrollPercent)

	// Modify state A and verify state B is unaffected
	stateA.CoordinatorMessages = append(stateA.CoordinatorMessages, chatrender.Message{
		Role:    "assistant",
		Content: "Additional message for A",
	})
	stateA.CoordinatorScrollPercent = 0.99

	require.Len(t, stateA.CoordinatorMessages, 2)
	require.Len(t, stateB.CoordinatorMessages, 1)           // B unchanged
	require.Equal(t, 0.25, stateB.CoordinatorScrollPercent) // B unchanged
}

func TestModel_StateIsolation_MapIndependence(t *testing.T) {
	// Test that map data in one state doesn't affect another
	stateA := NewWorkflowUIState()
	stateB := NewWorkflowUIState()

	// Populate state A worker maps
	stateA.WorkerStatus["worker-1"] = events.ProcessStatusWorking
	stateA.WorkerPhases["worker-1"] = events.ProcessPhaseImplementing
	stateA.WorkerMessages["worker-1"] = []chatrender.Message{{Role: "assistant", Content: "A's worker output"}}
	stateA.WorkerQueueCounts["worker-1"] = 5
	stateA.WorkerScrollPercents["worker-1"] = 0.8

	// Populate state B worker maps with different data
	stateB.WorkerStatus["worker-2"] = events.ProcessStatusReady
	stateB.WorkerPhases["worker-2"] = events.ProcessPhaseIdle
	stateB.WorkerMessages["worker-2"] = []chatrender.Message{{Role: "assistant", Content: "B's worker output"}}
	stateB.WorkerQueueCounts["worker-2"] = 0
	stateB.WorkerScrollPercents["worker-2"] = 0.0

	// Verify states don't share worker data
	require.Contains(t, stateA.WorkerStatus, "worker-1")
	require.NotContains(t, stateA.WorkerStatus, "worker-2")
	require.Contains(t, stateB.WorkerStatus, "worker-2")
	require.NotContains(t, stateB.WorkerStatus, "worker-1")

	// Modify A's worker data
	stateA.WorkerStatus["worker-1"] = events.ProcessStatusStopped
	stateA.WorkerQueueCounts["worker-1"] = 10

	// Verify B is unaffected
	require.Equal(t, events.ProcessStatusReady, stateB.WorkerStatus["worker-2"])
	require.Equal(t, 0, stateB.WorkerQueueCounts["worker-2"])
}

// === Unit Tests: Round-Trip Persistence ===

func TestModel_RoundTrip_StatePreservedAfterSaveAndLoad(t *testing.T) {
	// Create a state with all fields populated
	originalState := NewWorkflowUIState()

	// Populate coordinator data
	originalState.CoordinatorMessages = []chatrender.Message{
		{Role: "user", Content: "Hello coordinator"},
		{Role: "assistant", Content: "Hello user!"},
	}
	originalState.CoordinatorStatus = events.ProcessStatusWorking
	originalState.CoordinatorMetrics = &metrics.TokenMetrics{TokensUsed: 5000}
	originalState.CoordinatorQueueCount = 3
	originalState.CoordinatorScrollPercent = 0.75

	// Populate message data
	originalState.MessageEntries = []message.Entry{
		{ID: "msg-1", From: "COORDINATOR", To: "WORKER.1", Content: "Task 1"},
		{ID: "msg-2", From: "WORKER.1", To: "COORDINATOR", Content: "Done"},
	}
	originalState.MessageScrollPercent = 0.50

	// Populate worker data
	originalState.WorkerIDs = []string{"worker-1", "worker-2"}
	originalState.WorkerStatus["worker-1"] = events.ProcessStatusWorking
	originalState.WorkerStatus["worker-2"] = events.ProcessStatusReady
	originalState.WorkerPhases["worker-1"] = events.ProcessPhaseImplementing
	originalState.WorkerPhases["worker-2"] = events.ProcessPhaseIdle
	originalState.WorkerMessages["worker-1"] = []chatrender.Message{
		{Role: "assistant", Content: "Working on task 1"},
	}
	originalState.WorkerMessages["worker-2"] = []chatrender.Message{}
	originalState.WorkerMetrics["worker-1"] = &metrics.TokenMetrics{TokensUsed: 1000}
	originalState.WorkerQueueCounts["worker-1"] = 2
	originalState.WorkerScrollPercents["worker-1"] = 0.33
	originalState.WorkerScrollPercents["worker-2"] = 0.0

	// Simulate "round-trip" by creating a new state and copying data
	// This mimics what loadUIState does
	restoredState := NewWorkflowUIState()

	// Copy coordinator data
	restoredState.CoordinatorMessages = make([]chatrender.Message, len(originalState.CoordinatorMessages))
	copy(restoredState.CoordinatorMessages, originalState.CoordinatorMessages)
	restoredState.CoordinatorStatus = originalState.CoordinatorStatus
	restoredState.CoordinatorMetrics = originalState.CoordinatorMetrics
	restoredState.CoordinatorQueueCount = originalState.CoordinatorQueueCount
	restoredState.CoordinatorScrollPercent = originalState.CoordinatorScrollPercent

	// Copy message data
	restoredState.MessageEntries = make([]message.Entry, len(originalState.MessageEntries))
	copy(restoredState.MessageEntries, originalState.MessageEntries)
	restoredState.MessageScrollPercent = originalState.MessageScrollPercent

	// Copy worker data
	restoredState.WorkerIDs = make([]string, len(originalState.WorkerIDs))
	copy(restoredState.WorkerIDs, originalState.WorkerIDs)
	for k, v := range originalState.WorkerStatus {
		restoredState.WorkerStatus[k] = v
	}
	for k, v := range originalState.WorkerPhases {
		restoredState.WorkerPhases[k] = v
	}
	for k, v := range originalState.WorkerMessages {
		msgCopy := make([]chatrender.Message, len(v))
		copy(msgCopy, v)
		restoredState.WorkerMessages[k] = msgCopy
	}
	for k, v := range originalState.WorkerMetrics {
		restoredState.WorkerMetrics[k] = v
	}
	for k, v := range originalState.WorkerQueueCounts {
		restoredState.WorkerQueueCounts[k] = v
	}
	for k, v := range originalState.WorkerScrollPercents {
		restoredState.WorkerScrollPercents[k] = v
	}

	// Verify all data was preserved
	require.Equal(t, len(originalState.CoordinatorMessages), len(restoredState.CoordinatorMessages))
	require.Equal(t, originalState.CoordinatorMessages[0].Content, restoredState.CoordinatorMessages[0].Content)
	require.Equal(t, originalState.CoordinatorStatus, restoredState.CoordinatorStatus)
	require.Equal(t, originalState.CoordinatorQueueCount, restoredState.CoordinatorQueueCount)
	require.Equal(t, originalState.CoordinatorScrollPercent, restoredState.CoordinatorScrollPercent)

	require.Equal(t, len(originalState.MessageEntries), len(restoredState.MessageEntries))
	require.Equal(t, originalState.MessageEntries[0].ID, restoredState.MessageEntries[0].ID)
	require.Equal(t, originalState.MessageScrollPercent, restoredState.MessageScrollPercent)

	require.Equal(t, originalState.WorkerIDs, restoredState.WorkerIDs)
	require.Equal(t, originalState.WorkerStatus["worker-1"], restoredState.WorkerStatus["worker-1"])
	require.Equal(t, originalState.WorkerPhases["worker-1"], restoredState.WorkerPhases["worker-1"])
	require.Equal(t, originalState.WorkerScrollPercents["worker-1"], restoredState.WorkerScrollPercents["worker-1"])
}

// === Unit Tests: New Workflow Initialization ===

func TestModel_NewWorkflowState_InitializesEmpty(t *testing.T) {
	state := NewWorkflowUIState()

	// New state should be empty
	require.True(t, state.IsEmpty())
	require.Empty(t, state.CoordinatorMessages)
	require.Empty(t, state.MessageEntries)
	require.Empty(t, state.WorkerIDs)
	require.Equal(t, 0.0, state.CoordinatorScrollPercent)
	require.Equal(t, 0.0, state.MessageScrollPercent)
	require.Empty(t, state.WorkerScrollPercents)
}

func TestModel_NewWorkflowState_AllMapsUsableWithoutInitialization(t *testing.T) {
	state := NewWorkflowUIState()

	// Should be able to write to all maps immediately without nil panic
	require.NotPanics(t, func() {
		state.WorkerStatus["new-worker"] = events.ProcessStatusReady
		state.WorkerPhases["new-worker"] = events.ProcessPhaseIdle
		state.WorkerMessages["new-worker"] = []chatrender.Message{{Role: "test"}}
		state.WorkerMetrics["new-worker"] = &metrics.TokenMetrics{TokensUsed: 0}
		state.WorkerQueueCounts["new-worker"] = 0
		state.WorkerScrollPercents["new-worker"] = 0.0
	})
}

func TestModel_NewWorkflowState_ReadFromEmptyMapsReturnsZeroValues(t *testing.T) {
	state := NewWorkflowUIState()

	// Reading from empty maps should return zero values, not panic
	require.Equal(t, events.ProcessStatus(""), state.WorkerStatus["nonexistent"])
	require.Equal(t, events.ProcessPhase(""), state.WorkerPhases["nonexistent"])
	require.Nil(t, state.WorkerMessages["nonexistent"])
	require.Nil(t, state.WorkerMetrics["nonexistent"])
	require.Equal(t, 0, state.WorkerQueueCounts["nonexistent"])
	require.Equal(t, 0.0, state.WorkerScrollPercents["nonexistent"])
}

// === Unit Tests: Scroll Position Capture ===

func TestWorkflowUIState_ScrollPositions_InitiallyZero(t *testing.T) {
	state := NewWorkflowUIState()

	require.Equal(t, 0.0, state.CoordinatorScrollPercent)
	require.Equal(t, 0.0, state.MessageScrollPercent)
	require.Empty(t, state.WorkerScrollPercents)
}

func TestWorkflowUIState_ScrollPositions_CanBeSetAndRetrieved(t *testing.T) {
	state := NewWorkflowUIState()

	// Set scroll positions
	state.CoordinatorScrollPercent = 0.75
	state.MessageScrollPercent = 0.50
	state.WorkerScrollPercents["worker-1"] = 0.33
	state.WorkerScrollPercents["worker-2"] = 0.99

	// Verify retrieval
	require.Equal(t, 0.75, state.CoordinatorScrollPercent)
	require.Equal(t, 0.50, state.MessageScrollPercent)
	require.Equal(t, 0.33, state.WorkerScrollPercents["worker-1"])
	require.Equal(t, 0.99, state.WorkerScrollPercents["worker-2"])
}

func TestWorkflowUIState_ScrollPositions_BoundaryValues(t *testing.T) {
	state := NewWorkflowUIState()

	// Test boundary values: 0.0 (top) and 1.0 (bottom)
	state.CoordinatorScrollPercent = 0.0
	require.Equal(t, 0.0, state.CoordinatorScrollPercent)

	state.CoordinatorScrollPercent = 1.0
	require.Equal(t, 1.0, state.CoordinatorScrollPercent)

	// Test values just inside boundaries
	state.MessageScrollPercent = 0.001
	require.InDelta(t, 0.001, state.MessageScrollPercent, 0.0001)

	state.MessageScrollPercent = 0.999
	require.InDelta(t, 0.999, state.MessageScrollPercent, 0.0001)
}

func TestWorkflowUIState_ScrollPositions_MultipleWorkersIndependent(t *testing.T) {
	state := NewWorkflowUIState()

	// Set different scroll positions for multiple workers
	state.WorkerScrollPercents["worker-1"] = 0.1
	state.WorkerScrollPercents["worker-2"] = 0.5
	state.WorkerScrollPercents["worker-3"] = 0.9

	// Modify one worker's position
	state.WorkerScrollPercents["worker-2"] = 0.75

	// Verify other workers are unaffected
	require.Equal(t, 0.1, state.WorkerScrollPercents["worker-1"])
	require.Equal(t, 0.75, state.WorkerScrollPercents["worker-2"])
	require.Equal(t, 0.9, state.WorkerScrollPercents["worker-3"])
}

func TestWorkflowUIState_ScrollPositions_PreservedOnCopy(t *testing.T) {
	// Create original state with scroll positions
	original := NewWorkflowUIState()
	original.CoordinatorScrollPercent = 0.42
	original.MessageScrollPercent = 0.73
	original.WorkerScrollPercents["worker-1"] = 0.88

	// Create copy
	copy := NewWorkflowUIState()
	copy.CoordinatorScrollPercent = original.CoordinatorScrollPercent
	copy.MessageScrollPercent = original.MessageScrollPercent
	for k, v := range original.WorkerScrollPercents {
		copy.WorkerScrollPercents[k] = v
	}

	// Verify copy has same values
	require.Equal(t, original.CoordinatorScrollPercent, copy.CoordinatorScrollPercent)
	require.Equal(t, original.MessageScrollPercent, copy.MessageScrollPercent)
	require.Equal(t, original.WorkerScrollPercents["worker-1"], copy.WorkerScrollPercents["worker-1"])

	// Modify original
	original.CoordinatorScrollPercent = 0.99
	original.WorkerScrollPercents["worker-1"] = 0.11

	// Verify copy is unaffected
	require.Equal(t, 0.42, copy.CoordinatorScrollPercent)
	require.Equal(t, 0.88, copy.WorkerScrollPercents["worker-1"])
}

// === Unit Tests: maxCachedWorkflows Constant ===

func TestMaxCachedWorkflows_HasExpectedValue(t *testing.T) {
	require.Equal(t, 10, maxCachedWorkflows)
}

// === Unit Tests: Message History Loading from MessageRepo ===

// mockMessageRepository is a test mock for repository.MessageRepository.
type mockMessageRepository struct {
	entries []message.Entry
}

func (m *mockMessageRepository) Append(from, to, content string, msgType message.MessageType) (*message.Entry, error) {
	return nil, nil
}

func (m *mockMessageRepository) Entries() []message.Entry {
	return m.entries
}

func (m *mockMessageRepository) ReadAndMark(agentID string) []message.Entry {
	return nil
}

func (m *mockMessageRepository) Count() int {
	return len(m.entries)
}

func (m *mockMessageRepository) Broker() *pubsub.Broker[message.Event] {
	return nil
}

func (m *mockMessageRepository) AppendRestored(entry message.Entry) (*message.Entry, error) {
	return nil, nil
}

// === Unit Tests: Global Event Caching ===

func TestModel_GlobalEvent_CachesForAllWorkflows(t *testing.T) {
	// Verify that events received via global subscription are cached per-workflow
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowRunning),
	}

	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()

	globalEventCh := make(chan controlplane.ControlPlaneEvent)
	close(globalEventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(globalEventCh), func() {}).Maybe()

	cfg := Config{
		ControlPlane: mockCP,
		Services:     mode.Services{},
	}

	m := New(cfg)
	m.workflows = workflows
	m.selectedIndex = 0
	m = m.SetSize(100, 40).(Model)

	// Simulate global event for wf-1
	event1 := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			Output: "Hello from wf-1",
		},
	}
	result, _ := m.Update(event1)
	m = result.(Model)

	// Simulate global event for wf-2
	event2 := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-2",
		Payload: events.ProcessEvent{
			Output: "Hello from wf-2",
		},
	}
	result, _ = m.Update(event2)
	m = result.(Model)

	// Verify both workflows have cached state
	state1 := m.getOrCreateUIState("wf-1")
	require.Len(t, state1.CoordinatorMessages, 1)
	require.Equal(t, "Hello from wf-1", state1.CoordinatorMessages[0].Content)

	state2 := m.getOrCreateUIState("wf-2")
	require.Len(t, state2.CoordinatorMessages, 1)
	require.Equal(t, "Hello from wf-2", state2.CoordinatorMessages[0].Content)
}

func TestModel_WorkflowSelectionChange_ShowsCachedState(t *testing.T) {
	// Verify that switching selection shows cached state from global events
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowRunning),
	}

	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()

	globalEventCh := make(chan controlplane.ControlPlaneEvent)
	close(globalEventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(globalEventCh), func() {}).Maybe()

	cfg := Config{
		ControlPlane: mockCP,
		Services:     mode.Services{},
	}

	m := New(cfg)
	m.workflows = workflows
	m.selectedIndex = 0 // Start at wf-1
	m = m.SetSize(100, 40).(Model)

	// Simulate events for wf-2 while viewing wf-1
	event := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-2",
		Payload: events.ProcessEvent{
			Output: "Message while not selected",
		},
	}
	result, _ := m.Update(event)
	m = result.(Model)

	// Switch to wf-2
	m.handleWorkflowSelectionChange(1)

	// Verify wf-2's cached state is available
	require.Equal(t, 1, m.selectedIndex)
	state := m.getOrCreateUIState("wf-2")
	require.Len(t, state.CoordinatorMessages, 1)
	require.Equal(t, "Message while not selected", state.CoordinatorMessages[0].Content)
}

func TestModel_Cleanup_UnsubscribesFromGlobal(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	globalUnsubscribeCalled := false

	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()

	globalEventCh := make(chan controlplane.ControlPlaneEvent)
	close(globalEventCh)
	mockCP.On("Subscribe", mock.Anything).Return(
		(<-chan controlplane.ControlPlaneEvent)(globalEventCh),
		func() { globalUnsubscribeCalled = true },
	).Maybe()

	cfg := Config{
		ControlPlane: mockCP,
		Services:     mode.Services{},
	}

	m := New(cfg)
	m.workflows = workflows
	m.unsubscribe = func() { globalUnsubscribeCalled = true }

	// Cleanup
	m.Cleanup()

	// Verify global subscription was cleaned up
	require.True(t, globalUnsubscribeCalled, "global unsubscribe should be called on cleanup")
}

func TestModel_GlobalEvent_CoordinatorStatusFromProcessReadyWorking(t *testing.T) {
	// Verify that ProcessReady and ProcessWorking events update CoordinatorStatus
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
	}

	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()

	globalEventCh := make(chan controlplane.ControlPlaneEvent)
	close(globalEventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(globalEventCh), func() {}).Maybe()

	cfg := Config{
		ControlPlane: mockCP,
		Services:     mode.Services{},
	}

	m := New(cfg)
	m.workflows = workflows
	m.selectedIndex = 0
	m = m.SetSize(100, 40).(Model)

	// Initial state should have empty status
	state := m.getOrCreateUIState("wf-1")
	require.Equal(t, events.ProcessStatus(""), state.CoordinatorStatus, "initial status should be empty")

	// Simulate ProcessWorking event (classified as EventCoordinatorOutput)
	eventWorking := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			Type:   events.ProcessWorking,
			Role:   events.RoleCoordinator,
			Status: events.ProcessStatusWorking,
		},
	}
	result, _ := m.Update(eventWorking)
	m = result.(Model)

	// Verify status updated to Working
	state = m.getOrCreateUIState("wf-1")
	require.Equal(t, events.ProcessStatusWorking, state.CoordinatorStatus, "status should be Working after ProcessWorking event")

	// Simulate ProcessReady event (classified as EventCoordinatorOutput)
	eventReady := controlplane.ControlPlaneEvent{
		Type:       controlplane.EventCoordinatorOutput,
		WorkflowID: "wf-1",
		Payload: events.ProcessEvent{
			Type:   events.ProcessReady,
			Role:   events.RoleCoordinator,
			Status: events.ProcessStatusReady,
		},
	}
	result, _ = m.Update(eventReady)
	m = result.(Model)

	// Verify status updated to Ready
	state = m.getOrCreateUIState("wf-1")
	require.Equal(t, events.ProcessStatusReady, state.CoordinatorStatus, "status should be Ready after ProcessReady event")
}

func TestModel_WorkflowSelectionChange_SameIndexNoOp(t *testing.T) {
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowRunning),
	}

	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return(workflows, nil).Maybe()

	globalEventCh := make(chan controlplane.ControlPlaneEvent)
	close(globalEventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(globalEventCh), func() {}).Maybe()

	cfg := Config{
		ControlPlane: mockCP,
		Services:     mode.Services{},
	}

	m := New(cfg)
	m.workflows = workflows
	m.selectedIndex = 0
	m = m.SetSize(100, 40).(Model)

	// Select same index - should be no-op
	m.handleWorkflowSelectionChange(0)
	require.Equal(t, 0, m.selectedIndex)
}

// === Unit Tests: Tree State Caching ===

func TestWorkflowUIState_TreeStateFields_DefaultsToZeroValues(t *testing.T) {
	// Verify that new state has zero values for tree state fields
	state := NewWorkflowUIState()

	require.Equal(t, tree.Direction(""), state.TreeDirection, "TreeDirection should be zero value")
	require.Equal(t, tree.TreeMode(""), state.TreeMode, "TreeMode should be zero value")
	require.Equal(t, "", state.TreeSelectedID, "TreeSelectedID should be empty")
}

func TestModel_SaveEpicTreeState_SavesDirectionModeSelection(t *testing.T) {
	// Verify that saveEpicTreeState saves direction, mode, and selection
	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return([]*controlplane.WorkflowInstance{}, nil).Maybe()

	globalEventCh := make(chan controlplane.ControlPlaneEvent)
	close(globalEventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(globalEventCh), func() {}).Maybe()

	cfg := Config{
		ControlPlane: mockCP,
		Services:     mode.Services{},
	}

	m := New(cfg)
	m = m.SetSize(100, 40).(Model)

	// Create a tree with specific state (DirectionDown shows children)
	issueMap := map[string]*beads.Issue{
		"issue-1": {ID: "issue-1", TitleText: "Root Issue", Children: []string{"issue-2"}},
		"issue-2": {ID: "issue-2", TitleText: "Child Issue", ParentID: "issue-1"},
	}
	m.epicTree = tree.New("issue-1", issueMap, tree.DirectionDown, tree.ModeChildren, nil)

	// Move cursor to second node (child)
	m.epicTree.MoveCursor(1)

	// Save state
	m.saveEpicTreeState("wf-1")

	// Verify state was saved
	state := m.getOrCreateUIState("wf-1")
	require.Equal(t, tree.DirectionDown, state.TreeDirection)
	require.Equal(t, tree.ModeChildren, state.TreeMode)
	require.Equal(t, "issue-2", state.TreeSelectedID)
}

func TestModel_TreeStateRestoredOnReturn(t *testing.T) {
	// Verify that tree state is restored when returning to a workflow.
	// This tests the round-trip: save state -> switch away -> switch back -> state restored.
	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return([]*controlplane.WorkflowInstance{}, nil).Maybe()

	globalEventCh := make(chan controlplane.ControlPlaneEvent)
	close(globalEventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(globalEventCh), func() {}).Maybe()

	cfg := Config{
		ControlPlane: mockCP,
		Services:     mode.Services{},
	}

	m := New(cfg)
	m = m.SetSize(100, 40).(Model)

	// Create two workflows
	workflows := []*controlplane.WorkflowInstance{
		createTestWorkflow("wf-1", "Workflow 1", controlplane.WorkflowRunning),
		createTestWorkflow("wf-2", "Workflow 2", controlplane.WorkflowRunning),
	}
	workflows[0].EpicID = "epic-1"
	workflows[1].EpicID = "epic-2"
	m.workflows = workflows
	m.selectedIndex = 0

	// Create a tree with specific state for wf-1
	issueMap := map[string]*beads.Issue{
		"epic-1":  {ID: "epic-1", TitleText: "Epic 1", Children: []string{"issue-1"}},
		"issue-1": {ID: "issue-1", TitleText: "Issue 1", ParentID: "epic-1"},
	}
	m.epicTree = tree.New("epic-1", issueMap, tree.DirectionDown, tree.ModeChildren, nil)

	// Change direction to Up and select child
	m.epicTree.SetDirection(tree.DirectionUp)
	_ = m.epicTree.Rebuild()
	m.epicTree.MoveCursor(1)

	// Save state before switching
	m.saveEpicTreeState("wf-1")

	// Verify state was saved correctly
	state1 := m.getOrCreateUIState("wf-1")
	require.Equal(t, tree.DirectionUp, state1.TreeDirection)
	require.Equal(t, tree.ModeChildren, state1.TreeMode)

	// Switch to wf-2
	m.handleWorkflowSelectionChange(1)

	// Create different tree state for wf-2
	issueMap2 := map[string]*beads.Issue{
		"epic-2":  {ID: "epic-2", TitleText: "Epic 2", Children: []string{"issue-2"}},
		"issue-2": {ID: "issue-2", TitleText: "Issue 2", ParentID: "epic-2"},
	}
	m.epicTree = tree.New("epic-2", issueMap2, tree.DirectionDown, tree.ModeDeps, nil)
	m.saveEpicTreeState("wf-2")

	// Switch back to wf-1
	m.handleWorkflowSelectionChange(0)

	// Verify wf-1's state is still preserved in cache
	state1After := m.getOrCreateUIState("wf-1")
	require.Equal(t, tree.DirectionUp, state1After.TreeDirection)
	require.Equal(t, tree.ModeChildren, state1After.TreeMode)

	// Verify wf-2's state is also preserved
	state2 := m.getOrCreateUIState("wf-2")
	require.Equal(t, tree.DirectionDown, state2.TreeDirection)
	require.Equal(t, tree.ModeDeps, state2.TreeMode)
}

func TestModel_TreeDirectionPreserved(t *testing.T) {
	// Verify direction enum is preserved exactly (up vs down)
	state := NewWorkflowUIState()

	// Test DirectionDown
	state.TreeDirection = tree.DirectionDown
	require.Equal(t, tree.DirectionDown, state.TreeDirection)

	// Test DirectionUp
	state.TreeDirection = tree.DirectionUp
	require.Equal(t, tree.DirectionUp, state.TreeDirection)
}

func TestModel_TreeModePreserved(t *testing.T) {
	// Verify mode enum is preserved exactly (deps vs children)
	state := NewWorkflowUIState()

	// Test ModeDeps
	state.TreeMode = tree.ModeDeps
	require.Equal(t, tree.ModeDeps, state.TreeMode)

	// Test ModeChildren
	state.TreeMode = tree.ModeChildren
	require.Equal(t, tree.ModeChildren, state.TreeMode)
}

func TestModel_TreeStateEvicted_CleanedUpProperly(t *testing.T) {
	// Verify that tree state is cleaned up when workflow state is evicted
	mockCP := newMockControlPlane()
	mockCP.On("List", mock.Anything, mock.Anything).Return([]*controlplane.WorkflowInstance{}, nil).Maybe()

	globalEventCh := make(chan controlplane.ControlPlaneEvent)
	close(globalEventCh)
	mockCP.On("Subscribe", mock.Anything).Return((<-chan controlplane.ControlPlaneEvent)(globalEventCh), func() {}).Maybe()

	cfg := Config{
		ControlPlane: mockCP,
		Services:     mode.Services{},
	}

	m := New(cfg)
	m = m.SetSize(100, 40).(Model)

	baseTime := time.Now()

	// Create more than maxCachedWorkflows states with staggered timestamps
	// to ensure deterministic eviction (oldest gets evicted first)
	for i := 0; i < maxCachedWorkflows+2; i++ {
		wfID := controlplane.WorkflowID(fmt.Sprintf("wf-%d", i))
		state := m.getOrCreateUIState(wfID)
		state.TreeDirection = tree.DirectionUp
		state.TreeMode = tree.ModeChildren
		state.TreeSelectedID = fmt.Sprintf("issue-%d", i)
		// Set timestamps so earlier workflows are older
		state.LastUpdated = baseTime.Add(time.Duration(i) * time.Second)
	}

	// Verify we don't exceed max cached workflows
	require.LessOrEqual(t, len(m.workflowUIState), maxCachedWorkflows)

	// Verify that evicted workflow states (including tree state) were removed
	// At least some early workflows should have been evicted
	evictedCount := 0
	for i := 0; i < maxCachedWorkflows+2; i++ {
		wfID := controlplane.WorkflowID(fmt.Sprintf("wf-%d", i))
		if _, exists := m.workflowUIState[wfID]; !exists {
			evictedCount++
		}
	}

	require.GreaterOrEqual(t, evictedCount, 2, "At least 2 workflows should have been evicted")
}
