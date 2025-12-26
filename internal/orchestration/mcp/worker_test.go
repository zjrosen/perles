package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zjrosen/perles/internal/orchestration/message"
)

// mockMessageStore implements MessageStore for testing.
type mockMessageStore struct {
	entries   []message.Entry
	readState map[string]int
	mu        sync.RWMutex

	// Track method calls for verification
	appendCalls    []appendCall
	unreadForCalls []string
	markReadCalls  []string
}

type appendCall struct {
	From    string
	To      string
	Content string
	Type    message.MessageType
}

func newMockMessageStore() *mockMessageStore {
	return &mockMessageStore{
		entries:        make([]message.Entry, 0),
		readState:      make(map[string]int),
		appendCalls:    make([]appendCall, 0),
		unreadForCalls: make([]string, 0),
		markReadCalls:  make([]string, 0),
	}
}

// addEntry adds a message directly for test setup.
func (m *mockMessageStore) addEntry(from, to, content string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, message.Entry{
		ID:        "test-" + from + "-" + to,
		Timestamp: time.Now(),
		From:      from,
		To:        to,
		Content:   content,
		Type:      message.MessageInfo,
	})
}

// UnreadFor returns all unread messages for the given agent (no recipient filtering).
func (m *mockMessageStore) UnreadFor(agentID string) []message.Entry {
	m.mu.Lock()
	m.unreadForCalls = append(m.unreadForCalls, agentID)
	m.mu.Unlock()

	m.mu.RLock()
	defer m.mu.RUnlock()

	lastRead := m.readState[agentID]
	if lastRead >= len(m.entries) {
		return nil
	}

	// Return all unread entries (no recipient filtering)
	unread := make([]message.Entry, len(m.entries)-lastRead)
	copy(unread, m.entries[lastRead:])
	return unread
}

// MarkRead marks all messages up to now as read by the given agent.
func (m *mockMessageStore) MarkRead(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.markReadCalls = append(m.markReadCalls, agentID)
	m.readState[agentID] = len(m.entries)
}

// Append adds a new message to the log.
func (m *mockMessageStore) Append(from, to, content string, msgType message.MessageType) (*message.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.appendCalls = append(m.appendCalls, appendCall{
		From:    from,
		To:      to,
		Content: content,
		Type:    msgType,
	})

	entry := message.Entry{
		ID:        "test-" + from + "-" + to,
		Timestamp: time.Now(),
		From:      from,
		To:        to,
		Content:   content,
		Type:      msgType,
	}

	m.entries = append(m.entries, entry)
	return &entry, nil
}

// TestWorkerServer_RegistersAllTools verifies all 3 worker tools are registered.
func TestWorkerServer_RegistersAllTools(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	expectedTools := []string{
		"check_messages",
		"post_message",
		"signal_ready",
	}

	for _, toolName := range expectedTools {
		if _, ok := ws.tools[toolName]; !ok {
			t.Errorf("Tool %q not registered", toolName)
		}
		if _, ok := ws.handlers[toolName]; !ok {
			t.Errorf("Handler for %q not registered", toolName)
		}
	}

	if len(ws.tools) != len(expectedTools) {
		t.Errorf("Tool count = %d, want %d", len(ws.tools), len(expectedTools))
	}
}

// TestWorkerServer_ToolSchemas verifies tool schemas are valid.
func TestWorkerServer_ToolSchemas(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	for name, tool := range ws.tools {
		t.Run(name, func(t *testing.T) {
			if tool.Name == "" {
				t.Error("Tool name is empty")
			}
			if tool.Description == "" {
				t.Error("Tool description is empty")
			}
			if tool.InputSchema == nil {
				t.Error("Tool inputSchema is nil")
			}
			if tool.InputSchema != nil && tool.InputSchema.Type != "object" {
				t.Errorf("InputSchema.Type = %q, want %q", tool.InputSchema.Type, "object")
			}
		})
	}
}

// TestWorkerServer_Instructions tests that instructions are set correctly.
func TestWorkerServer_Instructions(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	if ws.instructions == "" {
		t.Error("Instructions should be set")
	}
	if ws.info.Name != "perles-worker" {
		t.Errorf("Server name = %q, want %q", ws.info.Name, "perles-worker")
	}
	if ws.info.Version != "1.0.0" {
		t.Errorf("Server version = %q, want %q", ws.info.Version, "1.0.0")
	}
}

// TestWorkerServer_DifferentWorkerIDs verifies different workers get separate identities.
func TestWorkerServer_DifferentWorkerIDs(t *testing.T) {
	store := newMockMessageStore()
	ws1 := NewWorkerServer("WORKER.1", store)
	ws2 := NewWorkerServer("WORKER.2", store)

	// Test through behavior - send message from each worker
	handler1 := ws1.handlers["post_message"]
	handler2 := ws2.handlers["post_message"]

	_, _ = handler1(context.Background(), json.RawMessage(`{"to": "ALL", "content": "from worker 1"}`))
	_, _ = handler2(context.Background(), json.RawMessage(`{"to": "ALL", "content": "from worker 2"}`))

	// Verify messages were sent with correct worker IDs
	if len(store.appendCalls) != 2 {
		t.Fatalf("Expected 2 append calls, got %d", len(store.appendCalls))
	}
	if store.appendCalls[0].From != "WORKER.1" {
		t.Errorf("First message from = %q, want %q", store.appendCalls[0].From, "WORKER.1")
	}
	if store.appendCalls[1].From != "WORKER.2" {
		t.Errorf("Second message from = %q, want %q", store.appendCalls[1].From, "WORKER.2")
	}
}

// TestWorkerServer_CheckMessagesNoStore tests check_messages when no store is available.
func TestWorkerServer_CheckMessagesNoStore(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)
	handler := ws.handlers["check_messages"]

	_, err := handler(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Error("Expected error when message store is nil")
	}
	if !strings.Contains(err.Error(), "message store not available") {
		t.Errorf("Error should mention 'message store not available', got: %v", err)
	}
}

// TestWorkerServer_CheckMessagesHappyPath tests successful message retrieval.
func TestWorkerServer_CheckMessagesHappyPath(t *testing.T) {
	store := newMockMessageStore()
	store.addEntry(message.ActorCoordinator, "WORKER.1", "Hello worker!")
	store.addEntry(message.ActorCoordinator, "WORKER.1", "Please start task")

	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["check_messages"]

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify UnreadFor was called with correct worker ID
	if len(store.unreadForCalls) != 1 || store.unreadForCalls[0] != "WORKER.1" {
		t.Errorf("UnreadFor not called correctly: %v", store.unreadForCalls)
	}

	// Verify MarkRead was called
	if len(store.markReadCalls) != 1 || store.markReadCalls[0] != "WORKER.1" {
		t.Errorf("MarkRead not called correctly: %v", store.markReadCalls)
	}

	// Verify result contains message count
	if result == nil || len(result.Content) == 0 {
		t.Fatal("Expected result with content")
	}
	text := result.Content[0].Text

	// Parse JSON response
	var response checkMessagesResponse
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if response.UnreadCount != 2 {
		t.Errorf("Expected unread_count=2, got: %d", response.UnreadCount)
	}
	if len(response.Messages) != 2 {
		t.Errorf("Expected 2 messages, got: %d", len(response.Messages))
	}
	if response.Messages[0].Content != "Hello worker!" {
		t.Errorf("Expected first message content 'Hello worker!', got: %s", response.Messages[0].Content)
	}
}

// TestWorkerServer_CheckMessagesNoMessages tests when there are no unread messages.
func TestWorkerServer_CheckMessagesNoMessages(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["check_messages"]

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result == nil || len(result.Content) == 0 {
		t.Fatal("Expected result with content")
	}
	text := result.Content[0].Text

	// Parse JSON response
	var response checkMessagesResponse
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if response.UnreadCount != 0 {
		t.Errorf("Expected unread_count=0, got: %d", response.UnreadCount)
	}
	if len(response.Messages) != 0 {
		t.Errorf("Expected 0 messages, got: %d", len(response.Messages))
	}
}

// TestWorkerServer_CheckMessagesSeesAllMessages tests that workers see all messages.
func TestWorkerServer_CheckMessagesSeesAllMessages(t *testing.T) {
	store := newMockMessageStore()
	// Messages for different workers
	store.addEntry(message.ActorCoordinator, "WORKER.1", "For worker 1")
	store.addEntry(message.ActorCoordinator, "WORKER.2", "For worker 2")
	store.addEntry(message.ActorCoordinator, message.ActorAll, "For everyone")

	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["check_messages"]

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	text := result.Content[0].Text

	// Parse JSON response
	var response checkMessagesResponse
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Workers see ALL messages (no filtering by recipient)
	if response.UnreadCount != 3 {
		t.Errorf("Expected 3 messages, got %d", response.UnreadCount)
	}

	contents := make(map[string]bool)
	for _, msg := range response.Messages {
		contents[msg.Content] = true
	}

	if !contents["For worker 1"] {
		t.Error("Should contain message addressed to WORKER.1")
	}
	if !contents["For everyone"] {
		t.Error("Should contain message addressed to ALL")
	}
	if !contents["For worker 2"] {
		t.Error("Should contain message addressed to WORKER.2 (workers see all messages)")
	}
}

// TestWorkerServer_CheckMessagesReadTracking tests that messages are marked as read.
func TestWorkerServer_CheckMessagesReadTracking(t *testing.T) {
	store := newMockMessageStore()
	store.addEntry(message.ActorCoordinator, "WORKER.1", "First message")

	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["check_messages"]

	// First call should return the message
	result1, _ := handler(context.Background(), json.RawMessage(`{}`))
	var response1 checkMessagesResponse
	if err := json.Unmarshal([]byte(result1.Content[0].Text), &response1); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}
	if response1.UnreadCount != 1 || response1.Messages[0].Content != "First message" {
		t.Error("First call should return the message")
	}

	// Second call should return no new messages
	result2, _ := handler(context.Background(), json.RawMessage(`{}`))
	var response2 checkMessagesResponse
	if err := json.Unmarshal([]byte(result2.Content[0].Text), &response2); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}
	if response2.UnreadCount != 0 {
		t.Errorf("Second call should return 0 unread messages, got: %d", response2.UnreadCount)
	}

	// Add a new message
	store.addEntry(message.ActorCoordinator, "WORKER.1", "Second message")

	// Third call should return only the new message
	result3, _ := handler(context.Background(), json.RawMessage(`{}`))
	var response3 checkMessagesResponse
	if err := json.Unmarshal([]byte(result3.Content[0].Text), &response3); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}
	if response3.UnreadCount != 1 {
		t.Errorf("Third call should return 1 new message, got: %d", response3.UnreadCount)
	}
	if response3.Messages[0].Content != "Second message" {
		t.Error("Third call should return the new message")
	}
}

// TestWorkerServer_SendMessageValidation tests input validation for post_message.
func TestWorkerServer_SendMessageValidation(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)
	handler := ws.handlers["post_message"]

	tests := []struct {
		name    string
		args    string
		wantErr string
	}{
		{
			name:    "missing to",
			args:    `{"content": "hello"}`,
			wantErr: "to is required",
		},
		{
			name:    "missing content",
			args:    `{"to": "COORDINATOR"}`,
			wantErr: "content is required",
		},
		{
			name:    "empty to",
			args:    `{"to": "", "content": "hello"}`,
			wantErr: "to is required",
		},
		{
			name:    "empty content",
			args:    `{"to": "COORDINATOR", "content": ""}`,
			wantErr: "content is required",
		},
		{
			name:    "message store not available",
			args:    `{"to": "COORDINATOR", "content": "hello"}`,
			wantErr: "message store not available",
		},
		{
			name:    "invalid json",
			args:    `not json`,
			wantErr: "invalid arguments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handler(context.Background(), json.RawMessage(tt.args))
			if err == nil {
				t.Error("Expected error but got none")
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Error should contain %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

// TestWorkerServer_SendMessageHappyPath tests successful message sending.
func TestWorkerServer_SendMessageHappyPath(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["post_message"]

	result, err := handler(context.Background(), json.RawMessage(`{"to": "COORDINATOR", "content": "Task complete"}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify Append was called with correct parameters
	if len(store.appendCalls) != 1 {
		t.Fatalf("Expected 1 append call, got %d", len(store.appendCalls))
	}
	call := store.appendCalls[0]
	if call.From != "WORKER.1" {
		t.Errorf("From = %q, want %q", call.From, "WORKER.1")
	}
	if call.To != "COORDINATOR" {
		t.Errorf("To = %q, want %q", call.To, "COORDINATOR")
	}
	if call.Content != "Task complete" {
		t.Errorf("Content = %q, want %q", call.Content, "Task complete")
	}
	if call.Type != message.MessageInfo {
		t.Errorf("Type = %v, want %v", call.Type, message.MessageInfo)
	}

	// Verify success result
	if !strings.Contains(result.Content[0].Text, "Message sent to COORDINATOR") {
		t.Errorf("Result should confirm sending, got: %s", result.Content[0].Text)
	}
}

// TestWorkerServer_SignalReadyValidation tests input validation for signal_ready.
func TestWorkerServer_SignalReadyValidation(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)
	handler := ws.handlers["signal_ready"]

	// signal_ready takes no parameters, so only test message store error
	_, err := handler(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Error("Expected error when message store is nil")
	}
	if !strings.Contains(err.Error(), "message store not available") {
		t.Errorf("Error should contain 'message store not available', got: %v", err)
	}
}

// TestWorkerServer_SignalReadyHappyPath tests successful ready signaling.
func TestWorkerServer_SignalReadyHappyPath(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["signal_ready"]

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify Append was called with correct parameters
	if len(store.appendCalls) != 1 {
		t.Fatalf("Expected 1 append call, got %d", len(store.appendCalls))
	}
	call := store.appendCalls[0]
	if call.From != "WORKER.1" {
		t.Errorf("From = %q, want %q", call.From, "WORKER.1")
	}
	if call.To != message.ActorCoordinator {
		t.Errorf("To = %q, want %q", call.To, message.ActorCoordinator)
	}
	expectedContent := "Worker WORKER.1 ready for task assignment"
	if call.Content != expectedContent {
		t.Errorf("Content = %q, want %q", call.Content, expectedContent)
	}
	if call.Type != message.MessageWorkerReady {
		t.Errorf("Type = %v, want %v", call.Type, message.MessageWorkerReady)
	}

	// Verify success result
	if !strings.Contains(result.Content[0].Text, "Ready signal sent") {
		t.Errorf("Result should confirm signal, got: %s", result.Content[0].Text)
	}
}

// TestWorkerServer_ToolDescriptionsAreHelpful verifies tool descriptions are informative.
func TestWorkerServer_ToolDescriptionsAreHelpful(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tests := []struct {
		toolName      string
		mustContain   []string
		descMinLength int
	}{
		{
			toolName:      "check_messages",
			mustContain:   []string{"message", "unread"},
			descMinLength: 30,
		},
		{
			toolName:      "post_message",
			mustContain:   []string{"message", "coordinator"},
			descMinLength: 30,
		},
		{
			toolName:      "signal_ready",
			mustContain:   []string{"ready", "task", "assignment"},
			descMinLength: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			tool := ws.tools[tt.toolName]
			desc := strings.ToLower(tool.Description)

			if len(tool.Description) < tt.descMinLength {
				t.Errorf("Description too short: %d chars, want at least %d", len(tool.Description), tt.descMinLength)
			}

			for _, keyword := range tt.mustContain {
				if !strings.Contains(desc, keyword) {
					t.Errorf("Description should contain %q: %s", keyword, tool.Description)
				}
			}
		})
	}
}

// TestWorkerServer_InstructionsContainToolNames verifies instructions mention all tools.
func TestWorkerServer_InstructionsContainToolNames(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)
	instructions := strings.ToLower(ws.instructions)

	toolNames := []string{"check_messages", "post_message", "signal_ready"}
	for _, name := range toolNames {
		if !strings.Contains(instructions, name) {
			t.Errorf("Instructions should mention %q", name)
		}
	}
}

// TestWorkerServer_CheckMessagesSchema verifies check_messages tool schema.
func TestWorkerServer_CheckMessagesSchema(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tool, ok := ws.tools["check_messages"]
	if !ok {
		t.Fatal("check_messages tool not registered")
	}

	if len(tool.InputSchema.Required) != 0 {
		t.Error("check_messages should not have required parameters")
	}
}

// TestWorkerServer_SendMessageSchema verifies post_message tool schema.
func TestWorkerServer_SendMessageSchema(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tool, ok := ws.tools["post_message"]
	if !ok {
		t.Fatal("post_message tool not registered")
	}

	if len(tool.InputSchema.Required) != 2 {
		t.Errorf("post_message should have 2 required parameters, got %d", len(tool.InputSchema.Required))
	}

	requiredSet := make(map[string]bool)
	for _, r := range tool.InputSchema.Required {
		requiredSet[r] = true
	}
	if !requiredSet["to"] {
		t.Error("'to' should be required")
	}
	if !requiredSet["content"] {
		t.Error("'content' should be required")
	}
}

// TestWorkerServer_SignalReadySchema verifies signal_ready tool schema.
func TestWorkerServer_SignalReadySchema(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tool, ok := ws.tools["signal_ready"]
	if !ok {
		t.Fatal("signal_ready tool not registered")
	}

	if len(tool.InputSchema.Required) != 0 {
		t.Errorf("signal_ready should have 0 required parameters, got %d", len(tool.InputSchema.Required))
	}
	if len(tool.InputSchema.Properties) != 0 {
		t.Errorf("signal_ready should have 0 properties, got %d", len(tool.InputSchema.Properties))
	}
}
