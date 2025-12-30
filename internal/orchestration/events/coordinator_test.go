package events

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorEvent_WithRawJSON(t *testing.T) {
	// Verify event with RawJSON serializes correctly
	rawJSON := []byte(`{"id":"msg_456","content":[{"type":"text","text":"Coordinator response"}]}`)
	event := CoordinatorEvent{
		Type:    CoordinatorChat,
		Role:    "coordinator",
		Content: "Coordinator response",
		RawJSON: rawJSON,
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
	var decoded CoordinatorEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, event.Role, decoded.Role)
	require.Equal(t, event.Content, decoded.Content)
	require.Equal(t, rawJSON, decoded.RawJSON)
}

func TestCoordinatorEvent_WithoutRawJSON(t *testing.T) {
	// Verify event without RawJSON omits the field (omitempty)
	event := CoordinatorEvent{
		Type:    CoordinatorChat,
		Role:    "coordinator",
		Content: "Coordinator response",
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
	var decoded CoordinatorEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, event.Role, decoded.Role)
	require.Nil(t, decoded.RawJSON)
}
