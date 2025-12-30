package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRPCError_Error(t *testing.T) {
	err := &RPCError{Code: -32600, Message: "Invalid Request"}
	got := err.Error()
	want := "RPC error -32600: Invalid Request"
	require.Equal(t, want, got, "Error() mismatch")
}

func TestErrorConstructors(t *testing.T) {
	tests := []struct {
		name     string
		err      *RPCError
		wantCode int
	}{
		{"ParseError", NewParseError("bad json"), ErrCodeParseError},
		{"InvalidRequest", NewInvalidRequest(nil), ErrCodeInvalidRequest},
		{"MethodNotFound", NewMethodNotFound("unknown"), ErrCodeMethodNotFound},
		{"InvalidParams", NewInvalidParams("missing field"), ErrCodeInvalidParams},
		{"InternalError", NewInternalError("server error"), ErrCodeInternalError},
		{"ToolNotFound", NewToolNotFound("bad_tool"), ErrCodeToolNotFound},
		{"ToolExecFailed", NewToolExecFailed("exec failed"), ErrCodeToolExecFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantCode, tt.err.Code, "Code mismatch")
		})
	}
}

func TestTextContent(t *testing.T) {
	content := TextContent("hello world")
	require.Equal(t, "text", content.Type, "Type mismatch")
	require.Equal(t, "hello world", content.Text, "Text mismatch")
}

func TestSuccessResult(t *testing.T) {
	result := SuccessResult("task completed")
	require.False(t, result.IsError, "IsError should be false for success")
	require.Len(t, result.Content, 1, "Content length mismatch")
	require.Equal(t, "task completed", result.Content[0].Text, "Text mismatch")
}

func TestErrorResult(t *testing.T) {
	result := ErrorResult("something failed")
	require.True(t, result.IsError, "IsError should be true for error result")
	require.Len(t, result.Content, 1, "Content length mismatch")
	require.Equal(t, "something failed", result.Content[0].Text, "Text mismatch")
}

func TestNewResponse(t *testing.T) {
	id := json.RawMessage(`1`)
	resp := NewResponse(id, map[string]string{"key": "value"})

	require.Equal(t, JSONRPCVersion, resp.JSONRPC, "JSONRPC mismatch")
	require.Equal(t, "1", string(resp.ID), "ID mismatch")
	require.Nil(t, resp.Error, "Error should be nil for success response")
}

func TestNewErrorResponse(t *testing.T) {
	id := json.RawMessage(`"req-123"`)
	rpcErr := NewMethodNotFound("unknown_method")
	resp := NewErrorResponse(id, rpcErr)

	require.Equal(t, JSONRPCVersion, resp.JSONRPC, "JSONRPC mismatch")
	require.Equal(t, `"req-123"`, string(resp.ID), "ID mismatch")
	require.NotNil(t, resp.Error, "Error should not be nil")
	require.Equal(t, ErrCodeMethodNotFound, resp.Error.Code, "Error.Code mismatch")
}

func TestRequestSerialization(t *testing.T) {
	req := Request{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`1`),
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}

	data, err := json.Marshal(req)
	require.NoError(t, err, "Marshal failed")

	var parsed Request
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err, "Unmarshal failed")

	require.Equal(t, req.Method, parsed.Method, "Method mismatch")
}

func TestToolSerialization(t *testing.T) {
	tool := Tool{
		Name:        "test_tool",
		Title:       "Test Tool",
		Description: "A test tool for testing",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"arg1": {Type: "string", Description: "First argument"},
				"arg2": {Type: "number", Description: "Second argument"},
			},
			Required: []string{"arg1"},
		},
	}

	data, err := json.Marshal(tool)
	require.NoError(t, err, "Marshal failed")

	var parsed Tool
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err, "Unmarshal failed")

	require.Equal(t, tool.Name, parsed.Name, "Name mismatch")
	require.Equal(t, tool.Description, parsed.Description, "Description mismatch")
	require.Len(t, parsed.InputSchema.Properties, 2, "Properties length mismatch")
	require.Len(t, parsed.InputSchema.Required, 1, "Required length mismatch")
}

func TestInitializeResultSerialization(t *testing.T) {
	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ServerCapability{
			Tools: &ToolsCapability{ListChanged: false},
		},
		ServerInfo: ImplementationInfo{
			Name:    "test-server",
			Version: "1.0.0",
		},
		Instructions: "Use these tools wisely",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err, "Marshal failed")

	var parsed InitializeResult
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err, "Unmarshal failed")

	require.Equal(t, ProtocolVersion, parsed.ProtocolVersion, "ProtocolVersion mismatch")
	require.Equal(t, "test-server", parsed.ServerInfo.Name, "ServerInfo.Name mismatch")
	require.NotNil(t, parsed.Capabilities.Tools, "Capabilities.Tools should not be nil")
}

func TestToolCallParamsSerialization(t *testing.T) {
	params := ToolCallParams{
		Name:      "spawn_worker",
		Arguments: json.RawMessage(`{"task_id": "perles-abc", "prompt": "Do the thing"}`),
	}

	data, err := json.Marshal(params)
	require.NoError(t, err, "Marshal failed")

	var parsed ToolCallParams
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err, "Unmarshal failed")

	require.Equal(t, params.Name, parsed.Name, "Name mismatch")
}

func TestToolCallResultSerialization(t *testing.T) {
	result := &ToolCallResult{
		Content: []ContentItem{
			{Type: "text", Text: "Worker spawned: worker-1"},
		},
		IsError: false,
	}

	data, err := json.Marshal(result)
	require.NoError(t, err, "Marshal failed")

	var parsed ToolCallResult
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err, "Unmarshal failed")

	require.False(t, parsed.IsError, "IsError should be false")
	require.Len(t, parsed.Content, 1, "Content length mismatch")
	require.Equal(t, "text", parsed.Content[0].Type, "Content[0].Type mismatch")
}
