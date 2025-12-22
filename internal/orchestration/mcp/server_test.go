package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	s := NewServer("test-server", "1.0.0")
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if s.info.Name != "test-server" {
		t.Errorf("info.Name = %q, want %q", s.info.Name, "test-server")
	}
	if s.info.Version != "1.0.0" {
		t.Errorf("info.Version = %q, want %q", s.info.Version, "1.0.0")
	}
}

func TestNewServerWithInstructions(t *testing.T) {
	s := NewServer("test", "1.0.0", WithInstructions("Use these tools"))
	if s.instructions != "Use these tools" {
		t.Errorf("instructions = %q, want %q", s.instructions, "Use these tools")
	}
}

func TestRegisterTool(t *testing.T) {
	s := NewServer("test", "1.0.0")

	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: &InputSchema{Type: "object"},
	}

	handler := func(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		return SuccessResult("ok"), nil
	}

	s.RegisterTool(tool, handler)

	// Verify tool was registered
	if _, ok := s.tools["test_tool"]; !ok {
		t.Error("Tool was not registered")
	}
	if _, ok := s.handlers["test_tool"]; !ok {
		t.Error("Handler was not registered")
	}
}

func TestServerInitialize(t *testing.T) {
	s := NewServer("test-server", "2.0.0", WithInstructions("Test instructions"))

	// Build initialize request
	initReq := Request{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params: json.RawMessage(`{
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {"name": "test-client", "version": "1.0.0"}
		}`),
	}
	reqData, _ := json.Marshal(initReq)

	// Create pipes for communication
	input := bytes.NewReader(append(reqData, '\n'))
	output := &bytes.Buffer{}

	// Run server in goroutine
	done := make(chan error, 1)
	go func() {
		done <- s.Serve(input, output)
	}()

	// Wait for response
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}

	// Parse response
	respData := output.Bytes()
	if len(respData) == 0 {
		t.Fatal("No response received")
	}

	var resp Response
	if err := json.Unmarshal(respData, &resp); err != nil {
		t.Fatalf("Failed to parse response: %v (data: %s)", err, string(respData))
	}

	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}

	// Verify the result contains server info
	resultData, _ := json.Marshal(resp.Result)
	var initResult InitializeResult
	if err := json.Unmarshal(resultData, &initResult); err != nil {
		t.Fatalf("Failed to parse InitializeResult: %v", err)
	}

	if initResult.ProtocolVersion != ProtocolVersion {
		t.Errorf("ProtocolVersion = %q, want %q", initResult.ProtocolVersion, ProtocolVersion)
	}
	if initResult.ServerInfo.Name != "test-server" {
		t.Errorf("ServerInfo.Name = %q, want %q", initResult.ServerInfo.Name, "test-server")
	}
	if initResult.Instructions != "Test instructions" {
		t.Errorf("Instructions = %q, want %q", initResult.Instructions, "Test instructions")
	}
}

func TestServerToolsList(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Register a test tool
	s.RegisterTool(Tool{
		Name:        "tool_a",
		Description: "Tool A",
		InputSchema: &InputSchema{Type: "object"},
	}, func(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		return SuccessResult("a"), nil
	})

	s.RegisterTool(Tool{
		Name:        "tool_b",
		Description: "Tool B",
		InputSchema: &InputSchema{Type: "object"},
	}, func(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		return SuccessResult("b"), nil
	})

	// Build tools/list request
	listReq := Request{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`2`),
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}
	reqData, _ := json.Marshal(listReq)

	input := bytes.NewReader(append(reqData, '\n'))
	output := &bytes.Buffer{}

	done := make(chan error, 1)
	go func() {
		done <- s.Serve(input, output)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}

	var resp Response
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}

	resultData, _ := json.Marshal(resp.Result)
	var listResult ToolsListResult
	if err := json.Unmarshal(resultData, &listResult); err != nil {
		t.Fatalf("Failed to parse ToolsListResult: %v", err)
	}

	if len(listResult.Tools) != 2 {
		t.Errorf("Tools length = %d, want 2", len(listResult.Tools))
	}
}

func TestServerToolsCall(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Register a tool that echoes its input
	s.RegisterTool(Tool{
		Name:        "echo",
		Description: "Echoes input",
		InputSchema: &InputSchema{
			Type: "object",
			Properties: map[string]*PropertySchema{
				"message": {Type: "string", Description: "Message to echo"},
			},
			Required: []string{"message"},
		},
	}, func(_ context.Context, args json.RawMessage) (*ToolCallResult, error) {
		var input struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, err
		}
		return SuccessResult("Echo: " + input.Message), nil
	})

	// Build tools/call request
	callReq := Request{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`3`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "echo", "arguments": {"message": "hello"}}`),
	}
	reqData, _ := json.Marshal(callReq)

	input := bytes.NewReader(append(reqData, '\n'))
	output := &bytes.Buffer{}

	done := make(chan error, 1)
	go func() {
		done <- s.Serve(input, output)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}

	var resp Response
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}

	resultData, _ := json.Marshal(resp.Result)
	var callResult ToolCallResult
	if err := json.Unmarshal(resultData, &callResult); err != nil {
		t.Fatalf("Failed to parse ToolCallResult: %v", err)
	}

	if callResult.IsError {
		t.Error("Expected success result")
	}
	if len(callResult.Content) != 1 {
		t.Fatalf("Content length = %d, want 1", len(callResult.Content))
	}
	if callResult.Content[0].Text != "Echo: hello" {
		t.Errorf("Content[0].Text = %q, want %q", callResult.Content[0].Text, "Echo: hello")
	}
}

func TestServerToolNotFound(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Call a non-existent tool
	callReq := Request{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`4`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "nonexistent", "arguments": {}}`),
	}
	reqData, _ := json.Marshal(callReq)

	input := bytes.NewReader(append(reqData, '\n'))
	output := &bytes.Buffer{}

	done := make(chan error, 1)
	go func() {
		done <- s.Serve(input, output)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}

	var resp Response
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("Expected error for nonexistent tool")
	}
	if resp.Error.Code != ErrCodeToolNotFound {
		t.Errorf("Error.Code = %d, want %d", resp.Error.Code, ErrCodeToolNotFound)
	}
}

func TestServerMethodNotFound(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Call a non-existent method
	req := Request{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`5`),
		Method:  "unknown/method",
		Params:  json.RawMessage(`{}`),
	}
	reqData, _ := json.Marshal(req)

	input := bytes.NewReader(append(reqData, '\n'))
	output := &bytes.Buffer{}

	done := make(chan error, 1)
	go func() {
		done <- s.Serve(input, output)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}

	var resp Response
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("Expected error for unknown method")
	}
	if resp.Error.Code != ErrCodeMethodNotFound {
		t.Errorf("Error.Code = %d, want %d", resp.Error.Code, ErrCodeMethodNotFound)
	}
}

func TestServerNotification(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Send initialized notification (no ID, no response expected)
	notification := Notification{
		JSONRPC: JSONRPCVersion,
		Method:  "notifications/initialized",
	}
	notifData, _ := json.Marshal(notification)

	input := bytes.NewReader(append(notifData, '\n'))
	output := &bytes.Buffer{}

	done := make(chan error, 1)
	go func() {
		done <- s.Serve(input, output)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}

	// No response should be sent for notifications
	if output.Len() > 0 {
		t.Errorf("Unexpected response for notification: %s", output.String())
	}

	// Verify initialized flag was set
	s.mu.RLock()
	initialized := s.initialized
	s.mu.RUnlock()

	if !initialized {
		t.Error("Server should be marked as initialized")
	}
}

func TestServerPing(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Send ping request
	pingReq := Request{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`"ping-1"`),
		Method:  "ping",
	}
	reqData, _ := json.Marshal(pingReq)

	input := bytes.NewReader(append(reqData, '\n'))
	output := &bytes.Buffer{}

	done := make(chan error, 1)
	go func() {
		done <- s.Serve(input, output)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}

	var resp Response
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("Unexpected error: %v", resp.Error)
	}
	// Ping should return empty object
	if resp.Result == nil {
		t.Error("Expected non-nil result for ping")
	}
}

func TestServerStop(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Use a pipe for input so we can control it
	pr, pw := io.Pipe()
	output := &bytes.Buffer{}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = s.Serve(pr, output)
	}()

	// Stop the server
	s.Stop()

	// Close the pipe to unblock the scanner
	pw.Close()

	// Wait for serve to return
	wg.Wait()
}

func TestServerParseError(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Send invalid JSON
	input := strings.NewReader("not valid json\n")
	output := &bytes.Buffer{}

	done := make(chan error, 1)
	go func() {
		done <- s.Serve(input, output)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}

	var resp Response
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("Expected parse error")
	}
	if resp.Error.Code != ErrCodeParseError {
		t.Errorf("Error.Code = %d, want %d", resp.Error.Code, ErrCodeParseError)
	}
}

func TestServerToolHandlerError(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Register a tool that returns an error
	s.RegisterTool(Tool{
		Name:        "failing_tool",
		Description: "Always fails",
		InputSchema: &InputSchema{Type: "object"},
	}, func(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		return nil, context.DeadlineExceeded
	})

	callReq := Request{
		JSONRPC: JSONRPCVersion,
		ID:      json.RawMessage(`6`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "failing_tool", "arguments": {}}`),
	}
	reqData, _ := json.Marshal(callReq)

	input := bytes.NewReader(append(reqData, '\n'))
	output := &bytes.Buffer{}

	done := make(chan error, 1)
	go func() {
		done <- s.Serve(input, output)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}

	var resp Response
	if err := json.Unmarshal(output.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Tool errors are returned as successful responses with isError=true
	if resp.Error != nil {
		t.Fatalf("Unexpected RPC error: %v", resp.Error)
	}

	resultData, _ := json.Marshal(resp.Result)
	var callResult ToolCallResult
	if err := json.Unmarshal(resultData, &callResult); err != nil {
		t.Fatalf("Failed to parse ToolCallResult: %v", err)
	}

	if !callResult.IsError {
		t.Error("Expected IsError to be true for tool error")
	}
}

func TestServerMultipleRequests(t *testing.T) {
	s := NewServer("test", "1.0.0")

	s.RegisterTool(Tool{
		Name:        "counter",
		Description: "Returns a count",
		InputSchema: &InputSchema{Type: "object"},
	}, func(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		return SuccessResult("counted"), nil
	})

	// Build multiple requests
	var requests []byte
	for i := 1; i <= 3; i++ {
		req := Request{
			JSONRPC: JSONRPCVersion,
			ID:      json.RawMessage(string(rune('0' + i))),
			Method:  "tools/call",
			Params:  json.RawMessage(`{"name": "counter", "arguments": {}}`),
		}
		reqData, _ := json.Marshal(req)
		requests = append(requests, reqData...)
		requests = append(requests, '\n')
	}

	input := bytes.NewReader(requests)
	output := &bytes.Buffer{}

	done := make(chan error, 1)
	go func() {
		done <- s.Serve(input, output)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
	}

	// Should have 3 responses
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("Response count = %d, want 3 (output: %s)", len(lines), output.String())
	}
}
