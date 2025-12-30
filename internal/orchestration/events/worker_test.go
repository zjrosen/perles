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
