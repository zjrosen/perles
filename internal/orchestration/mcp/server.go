package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/tracing"
	"github.com/zjrosen/perles/internal/pubsub"
)

// ToolHandler is a function that handles a tool call.
// It receives the parsed arguments and returns a result or error.
type ToolHandler func(ctx context.Context, args json.RawMessage) (*ToolCallResult, error)

// Server implements an MCP server over stdio.
type Server struct {
	info         ImplementationInfo
	instructions string
	tools        map[string]Tool
	handlers     map[string]ToolHandler

	reader io.Reader
	writer io.Writer

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex

	initialized bool

	// broker publishes MCPEvent for session logging
	broker *pubsub.Broker[events.MCPEvent]

	// tracer is used for distributed tracing of MCP tool calls.
	// When set, spans are created for each tool call with attributes
	// like mcp.tool.name, mcp.request.id, mcp.caller.role, mcp.caller.id.
	tracer trace.Tracer

	// callerRole identifies the role of the caller (coordinator or worker).
	// Used as the mcp.caller.role span attribute.
	callerRole string

	// callerID identifies the specific caller (e.g., worker-1, coordinator).
	// Used as the mcp.caller.id span attribute.
	callerID string
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithInstructions sets the server instructions sent during initialization.
func WithInstructions(instructions string) ServerOption {
	return func(s *Server) {
		s.instructions = instructions
	}
}

// WithTracer sets the tracer for distributed tracing of MCP tool calls.
// When set, spans are created for each tool call with standard MCP attributes.
func WithTracer(tracer trace.Tracer) ServerOption {
	return func(s *Server) {
		s.tracer = tracer
	}
}

// WithCallerInfo sets the caller role and ID for span attributes.
// Role should be "coordinator" or "worker", and ID should be the specific identifier.
func WithCallerInfo(role, id string) ServerOption {
	return func(s *Server) {
		s.callerRole = role
		s.callerID = id
	}
}

// NewServer creates a new MCP server.
func NewServer(name, version string, opts ...ServerOption) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		info: ImplementationInfo{
			Name:    name,
			Version: version,
		},
		tools:    make(map[string]Tool),
		handlers: make(map[string]ToolHandler),
		ctx:      ctx,
		cancel:   cancel,
		broker:   pubsub.NewBrokerWithBuffer[events.MCPEvent](128),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// RegisterTool registers a tool with its handler.
func (s *Server) RegisterTool(tool Tool, handler ToolHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[tool.Name] = tool
	s.handlers[tool.Name] = handler
}

// Broker returns the MCP event broker for session logging.
func (s *Server) Broker() *pubsub.Broker[events.MCPEvent] {
	return s.broker
}

// GetHandler returns the handler for the given tool name.
// Returns the handler and true if found, nil and false otherwise.
func (s *Server) GetHandler(name string) (ToolHandler, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.handlers[name]
	return h, ok
}

// Serve starts the server, reading from stdin and writing to stdout.
func (s *Server) Serve(stdin io.Reader, stdout io.Writer) error {
	s.mu.Lock()
	s.reader = stdin
	s.writer = stdout
	s.mu.Unlock()

	return s.run()
}

// ServeHTTP starts the server as an HTTP endpoint for MCP-over-HTTP transport.
// Returns the HTTP handler for use with http.Server.
func (s *Server) ServeHTTP() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read JSON-RPC request
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request", http.StatusBadRequest)
			return
		}

		// Handle the request (reuse existing handleRequest logic)
		response := s.handleRequestBytes(body)

		// Per JSON-RPC 2.0 spec: notifications MUST NOT receive a response.
		// Return 204 No Content for notifications (nil response).
		if response == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Write JSON-RPC response for requests
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(response); err != nil {
			log.Debug(log.CatMCP, "Failed to write response", "error", err)
		}
	})
}

// handleRequestBytes processes a single JSON-RPC request and returns the response bytes.
// Used by HTTP transport to handle synchronous request/response.
func (s *Server) handleRequestBytes(body []byte) []byte {
	var req Request
	if err := json.Unmarshal(body, &req); err != nil {
		errResp := NewErrorResponse(nil, NewParseError(err.Error()))
		data, _ := json.Marshal(errResp)
		return data
	}

	// Check if it's a request or notification
	if len(req.ID) > 0 && string(req.ID) != "null" {
		// It's a request - process and build response
		var result any
		var rpcErr *RPCError

		switch req.Method {
		case "initialize":
			result, rpcErr = s.handleInitialize(req.Params)
		case "tools/list":
			result, rpcErr = s.handleToolsList(req.Params)
		case "tools/call":
			result, rpcErr = s.handleToolsCall(req.Params)
		case "ping":
			result = struct{}{}
		default:
			rpcErr = NewMethodNotFound(req.Method)
		}

		var resp *Response
		if rpcErr != nil {
			resp = NewErrorResponse(req.ID, rpcErr)
		} else {
			resp = NewResponse(req.ID, result)
		}

		data, _ := json.Marshal(resp)
		return data
	}

	// It's a notification - process but don't respond per JSON-RPC 2.0 spec.
	// Return nil to signal the HTTP handler to send 204 No Content.
	s.handleNotification(&req)
	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() {
	s.cancel()
	s.wg.Wait()
}

// run is the main server loop.
func (s *Server) run() error {
	scanner := bufio.NewScanner(s.reader)
	// Increase buffer for large messages
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		log.Debug(log.CatMCP, "Received message", "raw", string(line))

		// Check if it's a request or notification
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, NewParseError(err.Error()))
			continue
		}

		// Handle the request - check if ID is present (not nil and not empty)
		// json.RawMessage is []byte, so we check length to distinguish requests from notifications
		if len(req.ID) > 0 && string(req.ID) != "null" {
			// It's a request, needs a response
			s.handleRequest(&req)
		} else {
			// It's a notification (no ID or ID is null)
			s.handleNotification(&req)
		}

		// Check if context is done
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}
	}

	if err := scanner.Err(); err != nil {
		log.Debug(log.CatMCP, "Scanner error", "error", err)
		return fmt.Errorf("reading input: %w", err)
	}

	return nil
}

// handleRequest processes a JSON-RPC request and sends a response.
func (s *Server) handleRequest(req *Request) {
	log.Debug(log.CatMCP, "Handling request", "method", req.Method)

	var result any
	var rpcErr *RPCError

	switch req.Method {
	case "initialize":
		result, rpcErr = s.handleInitialize(req.Params)

	case "tools/list":
		result, rpcErr = s.handleToolsList(req.Params)

	case "tools/call":
		result, rpcErr = s.handleToolsCall(req.Params)

	case "ping":
		result = struct{}{}

	default:
		rpcErr = NewMethodNotFound(req.Method)
	}

	if rpcErr != nil {
		s.sendError(req.ID, rpcErr)
	} else {
		s.sendResult(req.ID, result)
	}
}

// handleNotification processes a JSON-RPC notification (no response needed).
func (s *Server) handleNotification(req *Request) {
	log.Debug(log.CatMCP, "Handling notification", "method", req.Method)

	switch req.Method {
	case "notifications/initialized":
		s.mu.Lock()
		s.initialized = true
		s.mu.Unlock()
		log.Debug(log.CatMCP, "Client initialized")

	case "notifications/cancelled":
		// Handle cancellation if needed
		log.Debug(log.CatMCP, "Request cancelled")

	default:
		// Unknown notifications are ignored per spec
		log.Debug(log.CatMCP, "Unknown notification", "method", req.Method)
	}
}

// handleInitialize processes the initialize request.
func (s *Server) handleInitialize(params json.RawMessage) (any, *RPCError) {
	var p InitializeParams
	if params != nil {
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, NewInvalidParams(err.Error())
		}
	}

	log.Debug(log.CatMCP, "Initialize request",
		"clientVersion", p.ProtocolVersion,
		"clientName", p.ClientInfo.Name)

	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ServerCapability{
			Tools: &ToolsCapability{
				ListChanged: false, // We don't emit list change notifications
			},
		},
		ServerInfo:   s.info,
		Instructions: s.instructions,
	}

	return result, nil
}

// handleToolsList returns the list of available tools.
func (s *Server) handleToolsList(_ json.RawMessage) (any, *RPCError) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]Tool, 0, len(s.tools))
	for _, tool := range s.tools {
		tools = append(tools, tool)
	}

	return ToolsListResult{Tools: tools}, nil
}

// handleToolsCall invokes a tool and returns its result.
func (s *Server) handleToolsCall(params json.RawMessage) (any, *RPCError) {
	var p ToolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, NewInvalidParams(err.Error())
	}

	s.mu.RLock()
	handler, ok := s.handlers[p.Name]
	s.mu.RUnlock()

	if !ok {
		return nil, NewToolNotFound(p.Name)
	}

	log.Debug(log.CatMCP, "Calling tool", "name", p.Name)

	// Extract trace context from arguments if present (backwards compatible)
	traceID := s.extractTraceID(p.Arguments)

	// Set up context with trace ID if available
	ctx := s.ctx
	if traceID != "" {
		ctx = tracing.ContextWithTraceID(ctx, traceID)
	}

	// Create span for tool execution if tracer is configured
	var span trace.Span
	if s.tracer != nil {
		spanName := tracing.SpanPrefixMCP + p.Name
		var spanOpts []trace.SpanStartOption
		spanOpts = append(spanOpts, trace.WithSpanKind(trace.SpanKindServer))

		ctx, span = s.tracer.Start(ctx, spanName, spanOpts...)
		defer span.End()

		// Set span attributes using constants from tracing package
		span.SetAttributes(
			attribute.String(tracing.AttrMCPToolName, p.Name),
		)

		// Add caller info if available
		if s.callerRole != "" {
			span.SetAttributes(attribute.String(tracing.AttrMCPCallerRole, s.callerRole))
		}
		if s.callerID != "" {
			span.SetAttributes(attribute.String(tracing.AttrMCPCallerID, s.callerID))
		}

		// Add trace ID to span attributes if available
		if traceID != "" {
			span.SetAttributes(attribute.String("trace_id", traceID))
		}
	}

	// Capture start time for duration calculation
	startTime := time.Now()
	result, err := handler(ctx, p.Arguments)
	duration := time.Since(startTime)

	// Record outcome in span if tracing is enabled
	if span != nil {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
	}

	// Publish MCP event for session logging
	s.publishToolEvent(p.Name, params, result, err, duration, traceID)

	if err != nil {
		log.Debug(log.CatMCP, "Tool execution failed", "name", p.Name, "error", err)
		// Return the error as a tool result, not an RPC error
		return ErrorResult(err.Error()), nil
	}

	// Include trace_id in result if one was active (for correlation)
	if traceID != "" && result != nil {
		result = s.includeTraceIDInResult(result, traceID)
	}

	return result, nil
}

// extractTraceID extracts the trace_id or trace_context from tool arguments.
// This allows workers to propagate trace context back to the coordinator.
// Returns empty string if no trace context is present (backwards compatible).
func (s *Server) extractTraceID(args json.RawMessage) string {
	if args == nil {
		return ""
	}

	// Parse arguments to look for trace_id field
	var argMap map[string]json.RawMessage
	if err := json.Unmarshal(args, &argMap); err != nil {
		return ""
	}

	// Check for trace_id first (simple string)
	if traceIDRaw, ok := argMap["trace_id"]; ok {
		var traceID string
		if err := json.Unmarshal(traceIDRaw, &traceID); err == nil && traceID != "" {
			return traceID
		}
	}

	// Check for trace_context (object with trace_id field)
	if tcRaw, ok := argMap["trace_context"]; ok {
		var tc struct {
			TraceID string `json:"trace_id"`
		}
		if err := json.Unmarshal(tcRaw, &tc); err == nil && tc.TraceID != "" {
			return tc.TraceID
		}
	}

	return ""
}

// includeTraceIDInResult adds the trace_id to the result content for correlation.
// This allows workers to receive and propagate the trace context.
func (s *Server) includeTraceIDInResult(result *ToolCallResult, traceID string) *ToolCallResult {
	if result == nil || traceID == "" {
		return result
	}

	// If there's structured content, add trace_id to it
	if result.StructuredContent != nil {
		// Try to add trace_id to structured content if it's a map
		if m, ok := result.StructuredContent.(map[string]any); ok {
			m["trace_id"] = traceID
			return result
		}
	}

	// For text-only results, append trace context info to the last content item
	// We don't modify text content to avoid confusing the AI - trace_id is primarily
	// for structured responses or can be extracted from context
	return result
}

// publishToolEvent publishes an MCPEvent for the tool call.
func (s *Server) publishToolEvent(toolName string, requestParams json.RawMessage, result *ToolCallResult, err error, duration time.Duration, traceID string) {
	if s.broker == nil {
		return
	}

	evt := events.MCPEvent{
		Timestamp:   time.Now(),
		Method:      "tools/call",
		ToolName:    toolName,
		RequestJSON: requestParams,
		Duration:    duration,
		TraceID:     traceID,
	}

	// Serialize response if present
	if result != nil {
		if respJSON, marshalErr := json.Marshal(result); marshalErr == nil {
			evt.ResponseJSON = respJSON
		}
	}

	// Set event type and error based on outcome
	if err != nil {
		evt.Type = events.MCPError
		evt.Error = err.Error()
	} else {
		evt.Type = events.MCPToolResult
	}

	s.broker.Publish(pubsub.CreatedEvent, evt)
}

// sendResult sends a success response.
func (s *Server) sendResult(id json.RawMessage, result any) {
	resp := NewResponse(id, result)
	s.send(resp)
}

// sendError sends an error response.
func (s *Server) sendError(id json.RawMessage, err *RPCError) {
	resp := NewErrorResponse(id, err)
	s.send(resp)
}

// send marshals and writes a response to stdout.
func (s *Server) send(resp *Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Debug(log.CatMCP, "Failed to marshal response", "error", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writer == nil {
		return
	}

	// MCP uses newline-delimited JSON
	data = append(data, '\n')
	if _, err := s.writer.Write(data); err != nil {
		log.Debug(log.CatMCP, "Failed to write response", "error", err)
	}

	log.Debug(log.CatMCP, "Sent response", "raw", string(data[:len(data)-1]))
}
