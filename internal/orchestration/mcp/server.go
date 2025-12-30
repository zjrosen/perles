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

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/events"
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
}

// ServerOption configures a Server.
type ServerOption func(*Server)

// WithInstructions sets the server instructions sent during initialization.
func WithInstructions(instructions string) ServerOption {
	return func(s *Server) {
		s.instructions = instructions
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
	log.Debug(log.CatMCP, "Registered tool", "name", tool.Name)
}

// Broker returns the MCP event broker for session logging.
func (s *Server) Broker() *pubsub.Broker[events.MCPEvent] {
	return s.broker
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

		// Write JSON-RPC response
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

	// It's a notification - process but don't respond
	s.handleNotification(&req)
	return []byte("{}")
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

	// Capture start time for duration calculation
	startTime := time.Now()
	result, err := handler(s.ctx, p.Arguments)
	duration := time.Since(startTime)

	// Publish MCP event for session logging
	s.publishToolEvent(p.Name, params, result, err, duration)

	if err != nil {
		log.Debug(log.CatMCP, "Tool execution failed", "name", p.Name, "error", err)
		// Return the error as a tool result, not an RPC error
		return ErrorResult(err.Error()), nil
	}

	return result, nil
}

// publishToolEvent publishes an MCPEvent for the tool call.
func (s *Server) publishToolEvent(toolName string, requestParams json.RawMessage, result *ToolCallResult, err error, duration time.Duration) {
	if s.broker == nil {
		return
	}

	evt := events.MCPEvent{
		Timestamp:   time.Now(),
		Method:      "tools/call",
		ToolName:    toolName,
		RequestJSON: requestParams,
		Duration:    duration,
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
