// Package mcp implements the Model Context Protocol for orchestration tools.
//
// MCP is a standard protocol for AI systems to communicate with external tools.
// This implementation uses JSON-RPC 2.0 over stdio as the transport mechanism.
//
// The coordinator MCP server exposes tools for the orchestrating Claude session
// to manage workers, communicate via message issues, and interact with bd.
package mcp

import (
	"encoding/json"
	"fmt"
)

// ProtocolVersion is the MCP protocol version this implementation supports.
const ProtocolVersion = "2024-11-05"

// JSON-RPC 2.0 version string.
const JSONRPCVersion = "2.0"

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // Can be string, number, or null
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Notification represents a JSON-RPC 2.0 notification (no ID, no response expected).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCError represents a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// Standard JSON-RPC 2.0 error codes.
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// MCP-specific error codes (reserved range: -32000 to -32099).
const (
	ErrCodeToolNotFound     = -32001
	ErrCodeToolExecFailed   = -32002
	ErrCodeResourceNotFound = -32003
)

// NewParseError creates a parse error response.
func NewParseError(data any) *RPCError {
	return &RPCError{Code: ErrCodeParseError, Message: "Parse error", Data: data}
}

// NewInvalidRequest creates an invalid request error.
func NewInvalidRequest(data any) *RPCError {
	return &RPCError{Code: ErrCodeInvalidRequest, Message: "Invalid Request", Data: data}
}

// NewMethodNotFound creates a method not found error.
func NewMethodNotFound(method string) *RPCError {
	return &RPCError{Code: ErrCodeMethodNotFound, Message: "Method not found", Data: method}
}

// NewInvalidParams creates an invalid params error.
func NewInvalidParams(data any) *RPCError {
	return &RPCError{Code: ErrCodeInvalidParams, Message: "Invalid params", Data: data}
}

// NewInternalError creates an internal error.
func NewInternalError(message string) *RPCError {
	return &RPCError{Code: ErrCodeInternalError, Message: message}
}

// NewToolNotFound creates a tool not found error.
func NewToolNotFound(toolName string) *RPCError {
	return &RPCError{Code: ErrCodeToolNotFound, Message: fmt.Sprintf("Unknown tool: %s", toolName), Data: toolName}
}

// NewToolExecFailed creates a tool execution failed error.
func NewToolExecFailed(message string) *RPCError {
	return &RPCError{Code: ErrCodeToolExecFailed, Message: message}
}

// InitializeParams contains the client's initialization parameters.
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapability   `json:"capabilities"`
	ClientInfo      ImplementationInfo `json:"clientInfo"`
}

// InitializeResult contains the server's initialization response.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapability   `json:"capabilities"`
	ServerInfo      ImplementationInfo `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

// ClientCapability describes what a client supports.
type ClientCapability struct {
	Roots       *RootsCapability       `json:"roots,omitempty"`
	Sampling    *SamplingCapability    `json:"sampling,omitempty"`
	Elicitation *ElicitationCapability `json:"elicitation,omitempty"`
}

// ServerCapability describes what a server supports.
type ServerCapability struct {
	Prompts     *PromptsCapability     `json:"prompts,omitempty"`
	Resources   *ResourcesCapability   `json:"resources,omitempty"`
	Tools       *ToolsCapability       `json:"tools,omitempty"`
	Logging     *LoggingCapability     `json:"logging,omitempty"`
	Completions *CompletionsCapability `json:"completions,omitempty"`
}

// RootsCapability indicates filesystem root support.
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// SamplingCapability indicates LLM sampling request support.
type SamplingCapability struct{}

// ElicitationCapability indicates elicitation request support.
type ElicitationCapability struct{}

// PromptsCapability indicates prompt template support.
type PromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ResourcesCapability indicates readable resource support.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// ToolsCapability indicates callable tool support.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// LoggingCapability indicates structured logging support.
type LoggingCapability struct{}

// CompletionsCapability indicates argument autocompletion support.
type CompletionsCapability struct{}

// ImplementationInfo identifies an MCP implementation.
type ImplementationInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// Tool defines an MCP tool that can be called.
type Tool struct {
	Name         string        `json:"name"`
	Title        string        `json:"title,omitempty"`
	Description  string        `json:"description"`
	InputSchema  *InputSchema  `json:"inputSchema"`
	OutputSchema *OutputSchema `json:"outputSchema,omitempty"`
}

// OutputSchema defines the JSON Schema for tool output.
// When provided, servers MUST return structured results conforming to this schema.
type OutputSchema struct {
	Type       string                     `json:"type"`
	Properties map[string]*PropertySchema `json:"properties,omitempty"`
	Required   []string                   `json:"required,omitempty"`
	Items      *PropertySchema            `json:"items,omitempty"` // For array types
}

// InputSchema defines the JSON Schema for tool input.
type InputSchema struct {
	Type       string                     `json:"type"`
	Properties map[string]*PropertySchema `json:"properties,omitempty"`
	Required   []string                   `json:"required,omitempty"`
}

// PropertySchema defines a single property in a schema.
type PropertySchema struct {
	Type        string                     `json:"type"`
	Description string                     `json:"description,omitempty"`
	Properties  map[string]*PropertySchema `json:"properties,omitempty"` // For nested objects
	Items       *PropertySchema            `json:"items,omitempty"`      // For array items
	Required    []string                   `json:"required,omitempty"`   // For object types
}

// ToolsListResult is the response for tools/list.
type ToolsListResult struct {
	Tools      []Tool  `json:"tools"`
	NextCursor *string `json:"nextCursor,omitempty"`
}

// ToolCallParams contains the parameters for a tools/call request.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolCallResult is the response for tools/call.
type ToolCallResult struct {
	Content           []ContentItem `json:"content"`
	IsError           bool          `json:"isError,omitempty"`
	StructuredContent any           `json:"structuredContent,omitempty"`
}

// ContentItem represents a single content item in a tool result.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// Additional fields for image, audio, resource_link can be added as needed
}

// TextContent creates a text content item.
func TextContent(text string) ContentItem {
	return ContentItem{Type: "text", Text: text}
}

// SuccessResult creates a successful tool result with text content.
func SuccessResult(text string) *ToolCallResult {
	return &ToolCallResult{
		Content: []ContentItem{TextContent(text)},
		IsError: false,
	}
}

// ErrorResult creates an error tool result with text content.
func ErrorResult(text string) *ToolCallResult {
	return &ToolCallResult{
		Content: []ContentItem{TextContent(text)},
		IsError: true,
	}
}

// StructuredResult creates a successful tool result with both text content and structured content.
// This is required when a tool defines an outputSchema - the structuredContent field must be populated.
func StructuredResult(textContent string, structured any) *ToolCallResult {
	return &ToolCallResult{
		Content:           []ContentItem{TextContent(textContent)},
		IsError:           false,
		StructuredContent: structured,
	}
}

// NewResponse creates a success response with the given result.
func NewResponse(id json.RawMessage, result any) *Response {
	return &Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Result:  result,
	}
}

// NewErrorResponse creates an error response.
func NewErrorResponse(id json.RawMessage, err *RPCError) *Response {
	return &Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error:   err,
	}
}
