package events

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkerPhase_Values(t *testing.T) {
	// Verify all WorkerPhase constants have correct string values
	tests := []struct {
		phase    WorkerPhase
		expected string
	}{
		{PhaseIdle, "idle"},
		{PhaseImplementing, "implementing"},
		{PhaseAwaitingReview, "awaiting_review"},
		{PhaseReviewing, "reviewing"},
		{PhaseAddressingFeedback, "addressing_feedback"},
		{PhaseCommitting, "committing"},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			require.Equal(t, tt.expected, string(tt.phase))
		})
	}
}

func TestWorkerPhase_AllPhasesAreDefined(t *testing.T) {
	// Verify we have exactly 6 phases as specified in the proposal
	phases := []WorkerPhase{
		PhaseIdle,
		PhaseImplementing,
		PhaseAwaitingReview,
		PhaseReviewing,
		PhaseAddressingFeedback,
		PhaseCommitting,
	}

	// Each phase should be distinct
	seen := make(map[WorkerPhase]bool)
	for _, phase := range phases {
		require.False(t, seen[phase], "Duplicate phase: %s", phase)
		seen[phase] = true
	}

	require.Len(t, phases, 6, "Expected exactly 6 workflow phases")
}

func TestWorkerEvent_HasPhaseField(t *testing.T) {
	// Verify WorkerEvent can carry phase information
	event := WorkerEvent{
		Type:     WorkerStatusChange,
		WorkerID: "worker-1",
		TaskID:   "task-123",
		Status:   WorkerWorking,
		Phase:    PhaseImplementing,
	}

	require.Equal(t, PhaseImplementing, event.Phase)
	require.Equal(t, WorkerWorking, event.Status)
}

func TestWorkerEvent_WithRawJSON(t *testing.T) {
	// Verify event with RawJSON serializes correctly
	rawJSON := []byte(`{"id":"msg_123","content":[{"type":"text","text":"Hello"}]}`)
	event := WorkerEvent{
		Type:     WorkerOutput,
		WorkerID: "worker-1",
		TaskID:   "task-123",
		Output:   "Hello",
		RawJSON:  rawJSON,
	}

	// Serialize to JSON
	data, err := json.Marshal(event)
	require.NoError(t, err)

	// Verify RawJSON is included in serialized output
	var unmarshaled map[string]interface{}
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)
	require.Contains(t, unmarshaled, "raw_json")

	// Deserialize back to struct
	var decoded WorkerEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, event.WorkerID, decoded.WorkerID)
	require.Equal(t, event.Output, decoded.Output)
	require.Equal(t, rawJSON, decoded.RawJSON)
}

func TestWorkerEvent_WithoutRawJSON(t *testing.T) {
	// Verify event without RawJSON omits the field (omitempty)
	event := WorkerEvent{
		Type:     WorkerOutput,
		WorkerID: "worker-1",
		TaskID:   "task-123",
		Output:   "Hello",
		// RawJSON is nil
	}

	// Serialize to JSON
	data, err := json.Marshal(event)
	require.NoError(t, err)

	// Verify raw_json is not present in output (omitempty)
	var unmarshaled map[string]interface{}
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)
	require.NotContains(t, unmarshaled, "raw_json")

	// Deserialize back to struct
	var decoded WorkerEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, event.WorkerID, decoded.WorkerID)
	require.Nil(t, decoded.RawJSON)
}

func TestWorkerQueueChanged_EventType(t *testing.T) {
	// Verify WorkerQueueChanged event type is defined correctly
	require.Equal(t, WorkerEventType("queue_changed"), WorkerQueueChanged)

	// Verify it's distinct from other event types
	eventTypes := []WorkerEventType{
		WorkerSpawned,
		WorkerOutput,
		WorkerStatusChange,
		WorkerTokenUsage,
		WorkerIncoming,
		WorkerError,
		WorkerQueueChanged,
	}

	seen := make(map[WorkerEventType]bool)
	for _, et := range eventTypes {
		require.False(t, seen[et], "Duplicate event type: %s", et)
		seen[et] = true
	}

	require.Len(t, eventTypes, 7, "Expected exactly 7 event types")
}

func TestWorkerEvent_QueueCount(t *testing.T) {
	// Verify QueueCount field is populated correctly for queue_changed events
	event := WorkerEvent{
		Type:       WorkerQueueChanged,
		WorkerID:   "worker-1",
		QueueCount: 5,
	}

	require.Equal(t, WorkerQueueChanged, event.Type)
	require.Equal(t, "worker-1", event.WorkerID)
	require.Equal(t, 5, event.QueueCount)
}

func TestWorkerEvent_QueueCount_Serialization(t *testing.T) {
	// Verify QueueCount serializes correctly when non-zero
	event := WorkerEvent{
		Type:       WorkerQueueChanged,
		WorkerID:   "worker-1",
		QueueCount: 3,
	}

	// Serialize to JSON
	data, err := json.Marshal(event)
	require.NoError(t, err)

	// Verify queue_count is present
	var unmarshaled map[string]interface{}
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)
	require.Contains(t, unmarshaled, "queue_count")
	require.Equal(t, float64(3), unmarshaled["queue_count"]) // JSON numbers are float64

	// Deserialize back to struct
	var decoded WorkerEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, event.WorkerID, decoded.WorkerID)
	require.Equal(t, event.QueueCount, decoded.QueueCount)
}

func TestWorkerEvent_QueueCount_OmittedWhenZero(t *testing.T) {
	// Verify QueueCount is omitted when zero (omitempty)
	event := WorkerEvent{
		Type:       WorkerStatusChange,
		WorkerID:   "worker-1",
		QueueCount: 0, // Zero value
	}

	// Serialize to JSON
	data, err := json.Marshal(event)
	require.NoError(t, err)

	// Verify queue_count is not present (omitempty)
	var unmarshaled map[string]interface{}
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)
	require.NotContains(t, unmarshaled, "queue_count")
}
