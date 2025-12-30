package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMCPEvent_Serialization(t *testing.T) {
	// Create an event with all fields populated
	original := MCPEvent{
		Timestamp:    time.Date(2025, 1, 15, 10, 30, 45, 123000000, time.UTC),
		Type:         MCPToolResult,
		Method:       "tools/call",
		ToolName:     "test_tool",
		WorkerID:     "worker-1",
		RequestJSON:  []byte(`{"name":"test_tool","arguments":{"key":"value"}}`),
		ResponseJSON: []byte(`{"content":[{"type":"text","text":"success"}]}`),
		Duration:     150 * time.Millisecond,
	}

	// Marshal to JSON
	data, err := json.Marshal(original)
	require.NoError(t, err, "Failed to marshal MCPEvent")

	// Unmarshal back
	var decoded MCPEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err, "Failed to unmarshal MCPEvent")

	// Verify all fields
	require.Equal(t, original.Timestamp.Unix(), decoded.Timestamp.Unix(), "Timestamp mismatch")
	require.Equal(t, original.Type, decoded.Type, "Type mismatch")
	require.Equal(t, original.Method, decoded.Method, "Method mismatch")
	require.Equal(t, original.ToolName, decoded.ToolName, "ToolName mismatch")
	require.Equal(t, original.WorkerID, decoded.WorkerID, "WorkerID mismatch")
	require.JSONEq(t, string(original.RequestJSON), string(decoded.RequestJSON), "RequestJSON mismatch")
	require.JSONEq(t, string(original.ResponseJSON), string(decoded.ResponseJSON), "ResponseJSON mismatch")
	require.Equal(t, original.Duration, decoded.Duration, "Duration mismatch")
}

func TestMCPEvent_WithError(t *testing.T) {
	// Create an error event
	evt := MCPEvent{
		Timestamp:   time.Now(),
		Type:        MCPError,
		Method:      "tools/call",
		ToolName:    "failing_tool",
		RequestJSON: []byte(`{"name":"failing_tool","arguments":{}}`),
		Error:       "context deadline exceeded",
		Duration:    5 * time.Second,
	}

	// Marshal and unmarshal
	data, err := json.Marshal(evt)
	require.NoError(t, err, "Failed to marshal error event")

	var decoded MCPEvent
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err, "Failed to unmarshal error event")

	// Verify error field
	require.Equal(t, MCPError, decoded.Type, "Type should be error")
	require.Equal(t, "context deadline exceeded", decoded.Error, "Error message mismatch")
	require.Empty(t, decoded.ResponseJSON, "ResponseJSON should be empty for error")
}

func TestMCPEvent_OmitsEmptyFields(t *testing.T) {
	// Create a minimal event (coordinator, no worker, no error)
	evt := MCPEvent{
		Timestamp:    time.Now(),
		Type:         MCPToolResult,
		Method:       "tools/call",
		ToolName:     "simple_tool",
		RequestJSON:  []byte(`{}`),
		ResponseJSON: []byte(`{"content":[]}`),
		Duration:     10 * time.Millisecond,
	}

	data, err := json.Marshal(evt)
	require.NoError(t, err, "Failed to marshal event")

	// Verify omitempty fields are not present
	jsonStr := string(data)
	require.NotContains(t, jsonStr, `"worker_id"`, "worker_id should be omitted when empty")
	require.NotContains(t, jsonStr, `"error"`, "error should be omitted when empty")
}

func TestMCPEventType_Constants(t *testing.T) {
	// Verify the type constants are as expected
	require.Equal(t, MCPEventType("tool_call"), MCPToolCall)
	require.Equal(t, MCPEventType("tool_result"), MCPToolResult)
	require.Equal(t, MCPEventType("error"), MCPError)
}
