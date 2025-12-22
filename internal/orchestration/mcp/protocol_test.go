package mcp

import (
	"encoding/json"
	"testing"
)

func TestRPCError_Error(t *testing.T) {
	err := &RPCError{Code: -32600, Message: "Invalid Request"}
	got := err.Error()
	want := "RPC error -32600: Invalid Request"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
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
			if tt.err.Code != tt.wantCode {
				t.Errorf("Code = %d, want %d", tt.err.Code, tt.wantCode)
			}
		})
	}
}

func TestTextContent(t *testing.T) {
	content := TextContent("hello world")
	if content.Type != "text" {
		t.Errorf("Type = %q, want %q", content.Type, "text")
	}
	if content.Text != "hello world" {
		t.Errorf("Text = %q, want %q", content.Text, "hello world")
	}
}

func TestSuccessResult(t *testing.T) {
	result := SuccessResult("task completed")
	if result.IsError {
		t.Error("IsError should be false for success")
	}
	if len(result.Content) != 1 {
		t.Fatalf("Content length = %d, want 1", len(result.Content))
	}
	if result.Content[0].Text != "task completed" {
		t.Errorf("Text = %q, want %q", result.Content[0].Text, "task completed")
	}
}

func TestErrorResult(t *testing.T) {
	result := ErrorResult("something failed")
	if !result.IsError {
		t.Error("IsError should be true for error result")
	}
	if len(result.Content) != 1 {
		t.Fatalf("Content length = %d, want 1", len(result.Content))
	}
	if result.Content[0].Text != "something failed" {
		t.Errorf("Text = %q, want %q", result.Content[0].Text, "something failed")
	}
}

func TestNewResponse(t *testing.T) {
	id := json.RawMessage(`1`)
	resp := NewResponse(id, map[string]string{"key": "value"})

	if resp.JSONRPC != JSONRPCVersion {
		t.Errorf("JSONRPC = %q, want %q", resp.JSONRPC, JSONRPCVersion)
	}
	if string(resp.ID) != "1" {
		t.Errorf("ID = %q, want %q", string(resp.ID), "1")
	}
	if resp.Error != nil {
		t.Error("Error should be nil for success response")
	}
}

func TestNewErrorResponse(t *testing.T) {
	id := json.RawMessage(`"req-123"`)
	rpcErr := NewMethodNotFound("unknown_method")
	resp := NewErrorResponse(id, rpcErr)

	if resp.JSONRPC != JSONRPCVersion {
		t.Errorf("JSONRPC = %q, want %q", resp.JSONRPC, JSONRPCVersion)
	}
	if string(resp.ID) != `"req-123"` {
		t.Errorf("ID = %q, want %q", string(resp.ID), `"req-123"`)
	}
	if resp.Error == nil {
		t.Error("Error should not be nil")
	}
	if resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("Error.Code = %d, want %d", resp.Error.Code, ErrCodeMethodNotFound)
	}
}

func TestRequestSerialization(t *testing.T) {
	req := Request{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`1`),
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed Request
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.Method != req.Method {
		t.Errorf("Method = %q, want %q", parsed.Method, req.Method)
	}
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
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed Tool
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.Name != tool.Name {
		t.Errorf("Name = %q, want %q", parsed.Name, tool.Name)
	}
	if parsed.Description != tool.Description {
		t.Errorf("Description = %q, want %q", parsed.Description, tool.Description)
	}
	if len(parsed.InputSchema.Properties) != 2 {
		t.Errorf("Properties length = %d, want 2", len(parsed.InputSchema.Properties))
	}
	if len(parsed.InputSchema.Required) != 1 {
		t.Errorf("Required length = %d, want 1", len(parsed.InputSchema.Required))
	}
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
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed InitializeResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.ProtocolVersion != ProtocolVersion {
		t.Errorf("ProtocolVersion = %q, want %q", parsed.ProtocolVersion, ProtocolVersion)
	}
	if parsed.ServerInfo.Name != "test-server" {
		t.Errorf("ServerInfo.Name = %q, want %q", parsed.ServerInfo.Name, "test-server")
	}
	if parsed.Capabilities.Tools == nil {
		t.Error("Capabilities.Tools should not be nil")
	}
}

func TestToolCallParamsSerialization(t *testing.T) {
	params := ToolCallParams{
		Name:      "spawn_worker",
		Arguments: json.RawMessage(`{"task_id": "perles-abc", "prompt": "Do the thing"}`),
	}

	data, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed ToolCallParams
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.Name != params.Name {
		t.Errorf("Name = %q, want %q", parsed.Name, params.Name)
	}
}

func TestToolCallResultSerialization(t *testing.T) {
	result := &ToolCallResult{
		Content: []ContentItem{
			{Type: "text", Text: "Worker spawned: worker-1"},
		},
		IsError: false,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var parsed ToolCallResult
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.IsError {
		t.Error("IsError should be false")
	}
	if len(parsed.Content) != 1 {
		t.Fatalf("Content length = %d, want 1", len(parsed.Content))
	}
	if parsed.Content[0].Type != "text" {
		t.Errorf("Content[0].Type = %q, want %q", parsed.Content[0].Type, "text")
	}
}
