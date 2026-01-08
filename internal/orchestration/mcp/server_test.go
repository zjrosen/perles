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

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/tracing"
)

func TestNewServer(t *testing.T) {
	s := NewServer("test-server", "1.0.0")
	require.NotNil(t, s, "NewServer returned nil")
	require.Equal(t, "test-server", s.info.Name, "info.Name mismatch")
	require.Equal(t, "1.0.0", s.info.Version, "info.Version mismatch")
}

func TestNewServerWithInstructions(t *testing.T) {
	s := NewServer("test", "1.0.0", WithInstructions("Use these tools"))
	require.Equal(t, "Use these tools", s.instructions, "instructions mismatch")
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
	_, toolOk := s.tools["test_tool"]
	require.True(t, toolOk, "Tool was not registered")
	_, handlerOk := s.handlers["test_tool"]
	require.True(t, handlerOk, "Handler was not registered")
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
	require.NotEmpty(t, respData, "No response received")

	var resp Response
	require.NoError(t, json.Unmarshal(respData, &resp), "Failed to parse response (data: %s)", string(respData))

	require.Nil(t, resp.Error, "Unexpected error: %v", resp.Error)

	// Verify the result contains server info
	resultData, _ := json.Marshal(resp.Result)
	var initResult InitializeResult
	require.NoError(t, json.Unmarshal(resultData, &initResult), "Failed to parse InitializeResult")

	require.Equal(t, ProtocolVersion, initResult.ProtocolVersion, "ProtocolVersion mismatch")
	require.Equal(t, "test-server", initResult.ServerInfo.Name, "ServerInfo.Name mismatch")
	require.Equal(t, "Test instructions", initResult.Instructions, "Instructions mismatch")
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
	require.NoError(t, json.Unmarshal(output.Bytes(), &resp), "Failed to parse response")

	require.Nil(t, resp.Error, "Unexpected error: %v", resp.Error)

	resultData, _ := json.Marshal(resp.Result)
	var listResult ToolsListResult
	require.NoError(t, json.Unmarshal(resultData, &listResult), "Failed to parse ToolsListResult")

	require.Len(t, listResult.Tools, 2, "Tools length mismatch")
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
	require.NoError(t, json.Unmarshal(output.Bytes(), &resp), "Failed to parse response")

	require.Nil(t, resp.Error, "Unexpected error: %v", resp.Error)

	resultData, _ := json.Marshal(resp.Result)
	var callResult ToolCallResult
	require.NoError(t, json.Unmarshal(resultData, &callResult), "Failed to parse ToolCallResult")

	require.False(t, callResult.IsError, "Expected success result")
	require.Len(t, callResult.Content, 1, "Content length mismatch")
	require.Equal(t, "Echo: hello", callResult.Content[0].Text, "Content[0].Text mismatch")
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
	require.NoError(t, json.Unmarshal(output.Bytes(), &resp), "Failed to parse response")

	require.NotNil(t, resp.Error, "Expected error for nonexistent tool")
	require.Equal(t, ErrCodeToolNotFound, resp.Error.Code, "Error.Code mismatch")
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
	require.NoError(t, json.Unmarshal(output.Bytes(), &resp), "Failed to parse response")

	require.NotNil(t, resp.Error, "Expected error for unknown method")
	require.Equal(t, ErrCodeMethodNotFound, resp.Error.Code, "Error.Code mismatch")
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
	require.Empty(t, output.Bytes(), "Unexpected response for notification")

	// Verify initialized flag was set
	s.mu.RLock()
	initialized := s.initialized
	s.mu.RUnlock()

	require.True(t, initialized, "Server should be marked as initialized")
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
	require.NoError(t, json.Unmarshal(output.Bytes(), &resp), "Failed to parse response")

	require.Nil(t, resp.Error, "Unexpected error: %v", resp.Error)
	// Ping should return empty object
	require.NotNil(t, resp.Result, "Expected non-nil result for ping")
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
	require.NoError(t, json.Unmarshal(output.Bytes(), &resp), "Failed to parse response")

	require.NotNil(t, resp.Error, "Expected parse error")
	require.Equal(t, ErrCodeParseError, resp.Error.Code, "Error.Code mismatch")
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
	require.NoError(t, json.Unmarshal(output.Bytes(), &resp), "Failed to parse response")

	// Tool errors are returned as successful responses with isError=true
	require.Nil(t, resp.Error, "Unexpected RPC error: %v", resp.Error)

	resultData, _ := json.Marshal(resp.Result)
	var callResult ToolCallResult
	require.NoError(t, json.Unmarshal(resultData, &callResult), "Failed to parse ToolCallResult")

	require.True(t, callResult.IsError, "Expected IsError to be true for tool error")
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
	require.Len(t, lines, 3, "Response count mismatch")
}

func TestServer_MCPBroker_Publishes(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Register a simple tool
	s.RegisterTool(Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: &InputSchema{Type: "object"},
	}, func(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		return SuccessResult("ok"), nil
	})

	// Subscribe to the broker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eventCh := s.Broker().Subscribe(ctx)

	// Make a tool call via handleToolsCall
	params := json.RawMessage(`{"name": "test_tool", "arguments": {"key": "value"}}`)
	result, rpcErr := s.handleToolsCall(params)
	require.Nil(t, rpcErr, "Unexpected RPC error")
	require.NotNil(t, result, "Expected result")

	// Wait for event
	select {
	case event := <-eventCh:
		require.Equal(t, "tools/call", event.Payload.Method, "Method mismatch")
		require.Equal(t, "test_tool", event.Payload.ToolName, "ToolName mismatch")
		require.Contains(t, string(event.Payload.RequestJSON), "test_tool", "RequestJSON should contain tool name")
		require.Contains(t, string(event.Payload.ResponseJSON), "content", "ResponseJSON should contain content")
		require.Empty(t, event.Payload.Error, "Error should be empty for success")
	case <-time.After(100 * time.Millisecond):
		require.FailNow(t, "Timeout waiting for MCP event")
	}
}

func TestServer_MCPBroker_CapturesDuration(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Register a tool that takes some time
	s.RegisterTool(Tool{
		Name:        "slow_tool",
		Description: "A slow tool",
		InputSchema: &InputSchema{Type: "object"},
	}, func(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		time.Sleep(50 * time.Millisecond)
		return SuccessResult("done"), nil
	})

	// Subscribe to the broker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eventCh := s.Broker().Subscribe(ctx)

	// Make a tool call
	params := json.RawMessage(`{"name": "slow_tool", "arguments": {}}`)
	result, rpcErr := s.handleToolsCall(params)
	require.Nil(t, rpcErr, "Unexpected RPC error")
	require.NotNil(t, result, "Expected result")

	// Wait for event and verify duration
	select {
	case event := <-eventCh:
		require.GreaterOrEqual(t, event.Payload.Duration.Milliseconds(), int64(50), "Duration should be at least 50ms")
		require.Less(t, event.Payload.Duration.Milliseconds(), int64(200), "Duration should be reasonable")
	case <-time.After(500 * time.Millisecond):
		require.FailNow(t, "Timeout waiting for MCP event")
	}
}

func TestServer_MCPBroker_CapturesError(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Register a tool that fails
	s.RegisterTool(Tool{
		Name:        "failing_tool",
		Description: "A failing tool",
		InputSchema: &InputSchema{Type: "object"},
	}, func(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		return nil, context.DeadlineExceeded
	})

	// Subscribe to the broker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eventCh := s.Broker().Subscribe(ctx)

	// Make a tool call that will fail
	params := json.RawMessage(`{"name": "failing_tool", "arguments": {}}`)
	_, _ = s.handleToolsCall(params)

	// Wait for event and verify error
	select {
	case event := <-eventCh:
		require.Equal(t, "error", string(event.Payload.Type), "Type should be error")
		require.Equal(t, "context deadline exceeded", event.Payload.Error, "Error message mismatch")
		require.Equal(t, "failing_tool", event.Payload.ToolName, "ToolName mismatch")
	case <-time.After(100 * time.Millisecond):
		require.FailNow(t, "Timeout waiting for MCP event")
	}
}

func TestServer_Broker_ReturnsNonNil(t *testing.T) {
	s := NewServer("test", "1.0.0")
	require.NotNil(t, s.Broker(), "Broker should not be nil")
}

func TestServer_ExtractsTraceIDFromArguments(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Register a tool that returns the context trace ID
	var capturedTraceID string
	s.RegisterTool(Tool{
		Name:        "trace_test",
		Description: "Test tool for trace ID extraction",
		InputSchema: &InputSchema{Type: "object"},
	}, func(ctx context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		// Extract trace ID from context using tracing helper
		capturedTraceID = tracing.TraceIDFromContext(ctx)
		return SuccessResult("ok"), nil
	})

	// Test with trace_id in arguments
	params := json.RawMessage(`{"name": "trace_test", "arguments": {"trace_id": "test-trace-123"}}`)
	_, rpcErr := s.handleToolsCall(params)
	require.Nil(t, rpcErr, "Unexpected RPC error")
	require.Equal(t, "test-trace-123", capturedTraceID, "Trace ID should be extracted from arguments")
}

func TestServer_ExtractsTraceContextFromArguments(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Register a tool that returns the context trace ID
	var capturedTraceID string
	s.RegisterTool(Tool{
		Name:        "trace_context_test",
		Description: "Test tool for trace context extraction",
		InputSchema: &InputSchema{Type: "object"},
	}, func(ctx context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		// Extract trace ID from context using tracing helper
		capturedTraceID = tracing.TraceIDFromContext(ctx)
		return SuccessResult("ok"), nil
	})

	// Test with trace_context object in arguments
	params := json.RawMessage(`{"name": "trace_context_test", "arguments": {"trace_context": {"trace_id": "context-trace-456"}}}`)
	_, rpcErr := s.handleToolsCall(params)
	require.Nil(t, rpcErr, "Unexpected RPC error")
	require.Equal(t, "context-trace-456", capturedTraceID, "Trace ID should be extracted from trace_context object")
}

func TestServer_WorksWithoutTraceID(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Register a tool
	var handlerCalled bool
	s.RegisterTool(Tool{
		Name:        "no_trace_test",
		Description: "Test tool without trace ID",
		InputSchema: &InputSchema{Type: "object"},
	}, func(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		handlerCalled = true
		return SuccessResult("ok"), nil
	})

	// Test without trace_id in arguments (backwards compatibility)
	params := json.RawMessage(`{"name": "no_trace_test", "arguments": {"foo": "bar"}}`)
	result, rpcErr := s.handleToolsCall(params)
	require.Nil(t, rpcErr, "Unexpected RPC error")
	require.NotNil(t, result, "Result should not be nil")
	require.True(t, handlerCalled, "Handler should be called")
}

func TestServer_MCPBroker_CapturesTraceID(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Register a simple tool
	s.RegisterTool(Tool{
		Name:        "trace_event_test",
		Description: "Test tool for trace ID in events",
		InputSchema: &InputSchema{Type: "object"},
	}, func(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		return SuccessResult("ok"), nil
	})

	// Subscribe to the broker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eventCh := s.Broker().Subscribe(ctx)

	// Make a tool call with trace_id
	params := json.RawMessage(`{"name": "trace_event_test", "arguments": {"trace_id": "event-trace-789"}}`)
	_, rpcErr := s.handleToolsCall(params)
	require.Nil(t, rpcErr, "Unexpected RPC error")

	// Wait for event and verify trace ID
	select {
	case event := <-eventCh:
		require.Equal(t, "event-trace-789", event.Payload.TraceID, "TraceID should be captured in event")
		require.Equal(t, "trace_event_test", event.Payload.ToolName, "ToolName should match")
	case <-time.After(100 * time.Millisecond):
		require.FailNow(t, "Timeout waiting for MCP event")
	}
}

func TestServer_WithCallerInfo(t *testing.T) {
	s := NewServer("test", "1.0.0", WithCallerInfo("worker", "worker-1"))

	require.Equal(t, "worker", s.callerRole, "callerRole should be set")
	require.Equal(t, "worker-1", s.callerID, "callerID should be set")
}

func TestServer_IncludesTraceIDInStructuredResult(t *testing.T) {
	s := NewServer("test", "1.0.0")

	// Register a tool that returns structured content
	s.RegisterTool(Tool{
		Name:        "structured_trace_test",
		Description: "Test tool returning structured content",
		InputSchema: &InputSchema{Type: "object"},
	}, func(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		return StructuredResult("ok", map[string]any{"data": "test"}), nil
	})

	// Make a tool call with trace_id
	params := json.RawMessage(`{"name": "structured_trace_test", "arguments": {"trace_id": "structured-trace-001"}}`)
	result, rpcErr := s.handleToolsCall(params)
	require.Nil(t, rpcErr, "Unexpected RPC error")

	// Verify trace_id is included in structured content
	toolResult, ok := result.(*ToolCallResult)
	require.True(t, ok, "Result should be ToolCallResult")
	require.NotNil(t, toolResult.StructuredContent, "StructuredContent should not be nil")

	structContent, ok := toolResult.StructuredContent.(map[string]any)
	require.True(t, ok, "StructuredContent should be a map")
	require.Equal(t, "structured-trace-001", structContent["trace_id"], "trace_id should be included in structured content")
}

func TestServer_SpanCreationWithTracer(t *testing.T) {
	// Create a test tracer provider to capture spans
	provider, err := tracing.NewProvider(tracing.Config{
		Enabled:     true,
		Exporter:    "none", // No export, just create spans
		ServiceName: "test-mcp-server",
	})
	require.NoError(t, err, "Failed to create tracing provider")
	defer func() { _ = provider.Shutdown(context.Background()) }()

	s := NewServer("test", "1.0.0",
		WithTracer(provider.Tracer()),
		WithCallerInfo("worker", "worker-1"),
	)

	// Register a simple tool
	s.RegisterTool(Tool{
		Name:        "span_test",
		Description: "Test tool for span creation",
		InputSchema: &InputSchema{Type: "object"},
	}, func(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		return SuccessResult("ok"), nil
	})

	// Make a tool call
	params := json.RawMessage(`{"name": "span_test", "arguments": {"trace_id": "span-test-trace-001"}}`)
	result, rpcErr := s.handleToolsCall(params)
	require.Nil(t, rpcErr, "Unexpected RPC error")
	require.NotNil(t, result, "Result should not be nil")
}

func TestServer_SpanCreationOnError(t *testing.T) {
	// Create a test tracer provider
	provider, err := tracing.NewProvider(tracing.Config{
		Enabled:     true,
		Exporter:    "none",
		ServiceName: "test-mcp-server",
	})
	require.NoError(t, err, "Failed to create tracing provider")
	defer func() { _ = provider.Shutdown(context.Background()) }()

	s := NewServer("test", "1.0.0", WithTracer(provider.Tracer()))

	// Register a tool that fails
	s.RegisterTool(Tool{
		Name:        "failing_span_test",
		Description: "Test tool that fails",
		InputSchema: &InputSchema{Type: "object"},
	}, func(_ context.Context, _ json.RawMessage) (*ToolCallResult, error) {
		return nil, context.DeadlineExceeded
	})

	// Make a tool call that fails
	params := json.RawMessage(`{"name": "failing_span_test", "arguments": {}}`)
	result, rpcErr := s.handleToolsCall(params)
	require.Nil(t, rpcErr, "RPC error should be nil (tool errors are returned as results)")
	require.NotNil(t, result, "Result should not be nil")
	toolResult := result.(*ToolCallResult)
	require.True(t, toolResult.IsError, "Result should indicate error")
}
