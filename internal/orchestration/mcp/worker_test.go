package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/events"
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

// TestWorkerServer_RegistersAllTools verifies all 5 worker tools are registered.
func TestWorkerServer_RegistersAllTools(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	expectedTools := []string{
		"check_messages",
		"post_message",
		"signal_ready",
		"report_implementation_complete",
		"report_review_verdict",
	}

	for _, toolName := range expectedTools {
		_, ok := ws.tools[toolName]
		require.True(t, ok, "Tool %q not registered", toolName)
		_, ok = ws.handlers[toolName]
		require.True(t, ok, "Handler for %q not registered", toolName)
	}

	require.Equal(t, len(expectedTools), len(ws.tools), "Tool count mismatch")
}

// TestWorkerServer_ToolSchemas verifies tool schemas are valid.
func TestWorkerServer_ToolSchemas(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	for name, tool := range ws.tools {
		t.Run(name, func(t *testing.T) {
			require.NotEmpty(t, tool.Name, "Tool name is empty")
			require.NotEmpty(t, tool.Description, "Tool description is empty")
			require.NotNil(t, tool.InputSchema, "Tool inputSchema is nil")
			if tool.InputSchema != nil {
				require.Equal(t, "object", tool.InputSchema.Type, "InputSchema.Type mismatch")
			}
		})
	}
}

// TestWorkerServer_Instructions tests that instructions are set correctly.
func TestWorkerServer_Instructions(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	require.NotEmpty(t, ws.instructions, "Instructions should be set")
	require.Equal(t, "perles-worker", ws.info.Name, "Server name mismatch")
	require.Equal(t, "1.0.0", ws.info.Version, "Server version mismatch")
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
	require.Len(t, store.appendCalls, 2, "Expected 2 append calls")
	require.Equal(t, "WORKER.1", store.appendCalls[0].From, "First message from mismatch")
	require.Equal(t, "WORKER.2", store.appendCalls[1].From, "Second message from mismatch")
}

// TestWorkerServer_CheckMessagesNoStore tests check_messages when no store is available.
func TestWorkerServer_CheckMessagesNoStore(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)
	handler := ws.handlers["check_messages"]

	_, err := handler(context.Background(), json.RawMessage(`{}`))
	require.Error(t, err, "Expected error when message store is nil")
	require.Contains(t, err.Error(), "message store not available", "Error should mention 'message store not available'")
}

// TestWorkerServer_CheckMessagesHappyPath tests successful message retrieval.
func TestWorkerServer_CheckMessagesHappyPath(t *testing.T) {
	store := newMockMessageStore()
	store.addEntry(message.ActorCoordinator, "WORKER.1", "Hello worker!")
	store.addEntry(message.ActorCoordinator, "WORKER.1", "Please start task")

	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["check_messages"]

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "Unexpected error")

	// Verify UnreadFor was called with correct worker ID
	require.Len(t, store.unreadForCalls, 1, "UnreadFor should be called once")
	require.Equal(t, "WORKER.1", store.unreadForCalls[0], "UnreadFor called with wrong worker ID")

	// Verify MarkRead was called
	require.Len(t, store.markReadCalls, 1, "MarkRead should be called once")
	require.Equal(t, "WORKER.1", store.markReadCalls[0], "MarkRead called with wrong worker ID")

	// Verify result contains message count
	require.NotNil(t, result, "Expected result with content")
	require.NotEmpty(t, result.Content, "Expected result with content")
	text := result.Content[0].Text

	// Parse JSON response
	var response checkMessagesResponse
	require.NoError(t, json.Unmarshal([]byte(text), &response), "Failed to parse JSON response")

	require.Equal(t, 2, response.UnreadCount, "Expected unread_count=2")
	require.Len(t, response.Messages, 2, "Expected 2 messages")
	require.Equal(t, "Hello worker!", response.Messages[0].Content, "First message content mismatch")
}

// TestWorkerServer_CheckMessagesNoMessages tests when there are no unread messages.
func TestWorkerServer_CheckMessagesNoMessages(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["check_messages"]

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "Unexpected error")

	require.NotNil(t, result, "Expected result with content")
	require.NotEmpty(t, result.Content, "Expected result with content")
	text := result.Content[0].Text

	// Parse JSON response
	var response checkMessagesResponse
	require.NoError(t, json.Unmarshal([]byte(text), &response), "Failed to parse JSON response")

	require.Equal(t, 0, response.UnreadCount, "Expected unread_count=0")
	require.Empty(t, response.Messages, "Expected 0 messages")
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
	require.NoError(t, err, "Unexpected error")

	text := result.Content[0].Text

	// Parse JSON response
	var response checkMessagesResponse
	require.NoError(t, json.Unmarshal([]byte(text), &response), "Failed to parse JSON response")

	// Workers see ALL messages (no filtering by recipient)
	require.Equal(t, 3, response.UnreadCount, "Expected 3 messages")

	contents := make(map[string]bool)
	for _, msg := range response.Messages {
		contents[msg.Content] = true
	}

	require.True(t, contents["For worker 1"], "Should contain message addressed to WORKER.1")
	require.True(t, contents["For everyone"], "Should contain message addressed to ALL")
	require.True(t, contents["For worker 2"], "Should contain message addressed to WORKER.2 (workers see all messages)")
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
	require.NoError(t, json.Unmarshal([]byte(result1.Content[0].Text), &response1), "Failed to parse JSON response")
	require.Equal(t, 1, response1.UnreadCount, "First call should return 1 message")
	require.Equal(t, "First message", response1.Messages[0].Content, "First call should return the message")

	// Second call should return no new messages
	result2, _ := handler(context.Background(), json.RawMessage(`{}`))
	var response2 checkMessagesResponse
	require.NoError(t, json.Unmarshal([]byte(result2.Content[0].Text), &response2), "Failed to parse JSON response")
	require.Equal(t, 0, response2.UnreadCount, "Second call should return 0 unread messages")

	// Add a new message
	store.addEntry(message.ActorCoordinator, "WORKER.1", "Second message")

	// Third call should return only the new message
	result3, _ := handler(context.Background(), json.RawMessage(`{}`))
	var response3 checkMessagesResponse
	require.NoError(t, json.Unmarshal([]byte(result3.Content[0].Text), &response3), "Failed to parse JSON response")
	require.Equal(t, 1, response3.UnreadCount, "Third call should return 1 new message")
	require.Equal(t, "Second message", response3.Messages[0].Content, "Third call should return the new message")
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
			require.Error(t, err, "Expected error but got none")
			require.Contains(t, err.Error(), tt.wantErr, "Error should contain expected message")
		})
	}
}

// TestWorkerServer_SendMessageHappyPath tests successful message sending.
func TestWorkerServer_SendMessageHappyPath(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["post_message"]

	result, err := handler(context.Background(), json.RawMessage(`{"to": "COORDINATOR", "content": "Task complete"}`))
	require.NoError(t, err, "Unexpected error")

	// Verify Append was called with correct parameters
	require.Len(t, store.appendCalls, 1, "Expected 1 append call")
	call := store.appendCalls[0]
	require.Equal(t, "WORKER.1", call.From, "From mismatch")
	require.Equal(t, "COORDINATOR", call.To, "To mismatch")
	require.Equal(t, "Task complete", call.Content, "Content mismatch")
	require.Equal(t, message.MessageInfo, call.Type, "Type mismatch")

	// Verify success result
	require.Contains(t, result.Content[0].Text, "Message sent to COORDINATOR", "Result should confirm sending")
}

// TestWorkerServer_SignalReadyValidation tests input validation for signal_ready.
func TestWorkerServer_SignalReadyValidation(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)
	handler := ws.handlers["signal_ready"]

	// signal_ready takes no parameters, so only test message store error
	_, err := handler(context.Background(), json.RawMessage(`{}`))
	require.Error(t, err, "Expected error when message store is nil")
	require.Contains(t, err.Error(), "message store not available", "Error should mention 'message store not available'")
}

// TestWorkerServer_SignalReadyHappyPath tests successful ready signaling.
func TestWorkerServer_SignalReadyHappyPath(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["signal_ready"]

	result, err := handler(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err, "Unexpected error")

	// Verify Append was called with correct parameters
	require.Len(t, store.appendCalls, 1, "Expected 1 append call")
	call := store.appendCalls[0]
	require.Equal(t, "WORKER.1", call.From, "From mismatch")
	require.Equal(t, message.ActorCoordinator, call.To, "To mismatch")
	expectedContent := "Worker WORKER.1 ready for task assignment"
	require.Equal(t, expectedContent, call.Content, "Content mismatch")
	require.Equal(t, message.MessageWorkerReady, call.Type, "Type mismatch")

	// Verify success result
	require.Contains(t, result.Content[0].Text, "Ready signal sent", "Result should confirm signal")
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

			require.GreaterOrEqual(t, len(tool.Description), tt.descMinLength, "Description too short: want at least %d chars", tt.descMinLength)

			for _, keyword := range tt.mustContain {
				require.Contains(t, desc, keyword, "Description should contain %q", keyword)
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
		require.Contains(t, instructions, name, "Instructions should mention %q", name)
	}
}

// TestWorkerServer_CheckMessagesSchema verifies check_messages tool schema.
func TestWorkerServer_CheckMessagesSchema(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tool, ok := ws.tools["check_messages"]
	require.True(t, ok, "check_messages tool not registered")

	require.Empty(t, tool.InputSchema.Required, "check_messages should not have required parameters")
}

// TestWorkerServer_SendMessageSchema verifies post_message tool schema.
func TestWorkerServer_SendMessageSchema(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tool, ok := ws.tools["post_message"]
	require.True(t, ok, "post_message tool not registered")

	require.Len(t, tool.InputSchema.Required, 2, "post_message should have 2 required parameters")

	requiredSet := make(map[string]bool)
	for _, r := range tool.InputSchema.Required {
		requiredSet[r] = true
	}
	require.True(t, requiredSet["to"], "'to' should be required")
	require.True(t, requiredSet["content"], "'content' should be required")
}

// TestWorkerServer_SignalReadySchema verifies signal_ready tool schema.
func TestWorkerServer_SignalReadySchema(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tool, ok := ws.tools["signal_ready"]
	require.True(t, ok, "signal_ready tool not registered")

	require.Empty(t, tool.InputSchema.Required, "signal_ready should have 0 required parameters")
	require.Empty(t, tool.InputSchema.Properties, "signal_ready should have 0 properties")
}

// mockStateCallback implements WorkerStateCallback for testing.
type mockStateCallback struct {
	workerPhases map[string]events.WorkerPhase
	calls        []stateCallbackCall
	mu           sync.RWMutex

	// Error injection
	getPhaseError                 error
	onImplementationCompleteError error
	onReviewVerdictError          error
}

type stateCallbackCall struct {
	Method   string
	WorkerID string
	Summary  string
	Verdict  string
	Comments string
}

func newMockStateCallback() *mockStateCallback {
	return &mockStateCallback{
		workerPhases: make(map[string]events.WorkerPhase),
		calls:        make([]stateCallbackCall, 0),
	}
}

func (m *mockStateCallback) setPhase(workerID string, phase events.WorkerPhase) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workerPhases[workerID] = phase
}

func (m *mockStateCallback) GetWorkerPhase(workerID string) (events.WorkerPhase, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, stateCallbackCall{Method: "GetWorkerPhase", WorkerID: workerID})
	if m.getPhaseError != nil {
		return "", m.getPhaseError
	}
	phase, ok := m.workerPhases[workerID]
	if !ok {
		return events.PhaseIdle, nil
	}
	return phase, nil
}

func (m *mockStateCallback) OnImplementationComplete(workerID, summary string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, stateCallbackCall{Method: "OnImplementationComplete", WorkerID: workerID, Summary: summary})
	if m.onImplementationCompleteError != nil {
		return m.onImplementationCompleteError
	}
	// Update phase as coordinator would
	m.workerPhases[workerID] = events.PhaseAwaitingReview
	return nil
}

func (m *mockStateCallback) OnReviewVerdict(workerID, verdict, comments string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, stateCallbackCall{Method: "OnReviewVerdict", WorkerID: workerID, Verdict: verdict, Comments: comments})
	if m.onReviewVerdictError != nil {
		return m.onReviewVerdictError
	}
	// Update phase as coordinator would
	m.workerPhases[workerID] = events.PhaseIdle
	return nil
}

// TestWorkerServer_ReportImplementationComplete_NoCallback tests error when callback not set.
func TestWorkerServer_ReportImplementationComplete_NoCallback(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["report_implementation_complete"]

	_, err := handler(context.Background(), json.RawMessage(`{"summary": "completed feature X"}`))
	require.Error(t, err, "Expected error when callback not configured")
	require.Contains(t, err.Error(), "state callback not configured", "Expected 'state callback not configured' error")
}

// TestWorkerServer_ReportImplementationComplete_MissingSummary tests validation.
func TestWorkerServer_ReportImplementationComplete_MissingSummary(t *testing.T) {
	store := newMockMessageStore()
	callback := newMockStateCallback()
	ws := NewWorkerServer("WORKER.1", store)
	ws.SetStateCallback(callback)
	handler := ws.handlers["report_implementation_complete"]

	_, err := handler(context.Background(), json.RawMessage(`{}`))
	require.Error(t, err, "Expected error for missing summary")
	require.Contains(t, err.Error(), "summary is required", "Expected 'summary is required' error")
}

// TestWorkerServer_ReportImplementationComplete_WrongPhase tests phase validation.
func TestWorkerServer_ReportImplementationComplete_WrongPhase(t *testing.T) {
	store := newMockMessageStore()
	callback := newMockStateCallback()
	callback.setPhase("WORKER.1", events.PhaseIdle) // Not implementing

	ws := NewWorkerServer("WORKER.1", store)
	ws.SetStateCallback(callback)
	handler := ws.handlers["report_implementation_complete"]

	_, err := handler(context.Background(), json.RawMessage(`{"summary": "done"}`))
	require.Error(t, err, "Expected error for wrong phase")
	require.Contains(t, err.Error(), "not in implementing or addressing_feedback phase", "Expected phase error")
}

// TestWorkerServer_ReportImplementationComplete_HappyPath tests successful completion.
func TestWorkerServer_ReportImplementationComplete_HappyPath(t *testing.T) {
	store := newMockMessageStore()
	callback := newMockStateCallback()
	callback.setPhase("WORKER.1", events.PhaseImplementing)

	ws := NewWorkerServer("WORKER.1", store)
	ws.SetStateCallback(callback)
	handler := ws.handlers["report_implementation_complete"]

	result, err := handler(context.Background(), json.RawMessage(`{"summary": "Added feature X with tests"}`))
	require.NoError(t, err, "Unexpected error")

	// Verify callback was called
	require.Len(t, callback.calls, 2, "Expected 2 callback calls (GetWorkerPhase + OnImplementationComplete)")
	// Find the OnImplementationComplete call
	found := false
	for _, call := range callback.calls {
		if call.Method == "OnImplementationComplete" {
			found = true
			require.Equal(t, "WORKER.1", call.WorkerID, "WorkerID mismatch")
			require.Equal(t, "Added feature X with tests", call.Summary, "Summary mismatch")
		}
	}
	require.True(t, found, "OnImplementationComplete callback not called")

	// Verify message was posted to coordinator
	require.Len(t, store.appendCalls, 1, "Expected 1 message posted")
	require.Contains(t, store.appendCalls[0].Content, "Implementation complete", "Message should contain 'Implementation complete'")

	// Verify structured response
	require.NotNil(t, result, "Expected result with content")
	require.NotEmpty(t, result.Content, "Expected result with content")
	require.Contains(t, result.Content[0].Text, "awaiting_review", "Response should contain 'awaiting_review'")
}

// TestWorkerServer_ReportImplementationComplete_AddressingFeedback tests completion from addressing_feedback phase.
func TestWorkerServer_ReportImplementationComplete_AddressingFeedback(t *testing.T) {
	store := newMockMessageStore()
	callback := newMockStateCallback()
	callback.setPhase("WORKER.1", events.PhaseAddressingFeedback)

	ws := NewWorkerServer("WORKER.1", store)
	ws.SetStateCallback(callback)
	handler := ws.handlers["report_implementation_complete"]

	_, err := handler(context.Background(), json.RawMessage(`{"summary": "Fixed review feedback"}`))
	require.NoError(t, err, "Should succeed from addressing_feedback phase")
}

// TestWorkerServer_ReportReviewVerdict_NoCallback tests error when callback not set.
func TestWorkerServer_ReportReviewVerdict_NoCallback(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)
	handler := ws.handlers["report_review_verdict"]

	_, err := handler(context.Background(), json.RawMessage(`{"verdict": "APPROVED", "comments": "LGTM"}`))
	require.Error(t, err, "Expected error when callback not configured")
	require.Contains(t, err.Error(), "state callback not configured", "Expected 'state callback not configured' error")
}

// TestWorkerServer_ReportReviewVerdict_MissingVerdict tests validation.
func TestWorkerServer_ReportReviewVerdict_MissingVerdict(t *testing.T) {
	store := newMockMessageStore()
	callback := newMockStateCallback()
	ws := NewWorkerServer("WORKER.1", store)
	ws.SetStateCallback(callback)
	handler := ws.handlers["report_review_verdict"]

	_, err := handler(context.Background(), json.RawMessage(`{"comments": "LGTM"}`))
	require.Error(t, err, "Expected error for missing verdict")
	require.Contains(t, err.Error(), "verdict is required", "Expected 'verdict is required' error")
}

// TestWorkerServer_ReportReviewVerdict_MissingComments tests validation.
func TestWorkerServer_ReportReviewVerdict_MissingComments(t *testing.T) {
	store := newMockMessageStore()
	callback := newMockStateCallback()
	ws := NewWorkerServer("WORKER.1", store)
	ws.SetStateCallback(callback)
	handler := ws.handlers["report_review_verdict"]

	_, err := handler(context.Background(), json.RawMessage(`{"verdict": "APPROVED"}`))
	require.Error(t, err, "Expected error for missing comments")
	require.Contains(t, err.Error(), "comments is required", "Expected 'comments is required' error")
}

// TestWorkerServer_ReportReviewVerdict_InvalidVerdict tests invalid verdict value.
func TestWorkerServer_ReportReviewVerdict_InvalidVerdict(t *testing.T) {
	store := newMockMessageStore()
	callback := newMockStateCallback()
	callback.setPhase("WORKER.1", events.PhaseReviewing)
	ws := NewWorkerServer("WORKER.1", store)
	ws.SetStateCallback(callback)
	handler := ws.handlers["report_review_verdict"]

	_, err := handler(context.Background(), json.RawMessage(`{"verdict": "MAYBE", "comments": "Not sure"}`))
	require.Error(t, err, "Expected error for invalid verdict")
	require.Contains(t, err.Error(), "must be 'APPROVED' or 'DENIED'", "Expected verdict validation error")
}

// TestWorkerServer_ReportReviewVerdict_WrongPhase tests phase validation.
func TestWorkerServer_ReportReviewVerdict_WrongPhase(t *testing.T) {
	store := newMockMessageStore()
	callback := newMockStateCallback()
	callback.setPhase("WORKER.1", events.PhaseImplementing) // Not reviewing

	ws := NewWorkerServer("WORKER.1", store)
	ws.SetStateCallback(callback)
	handler := ws.handlers["report_review_verdict"]

	_, err := handler(context.Background(), json.RawMessage(`{"verdict": "APPROVED", "comments": "LGTM"}`))
	require.Error(t, err, "Expected error for wrong phase")
	require.Contains(t, err.Error(), "not in reviewing phase", "Expected phase error")
}

// TestWorkerServer_ReportReviewVerdict_Approved tests successful approval.
func TestWorkerServer_ReportReviewVerdict_Approved(t *testing.T) {
	store := newMockMessageStore()
	callback := newMockStateCallback()
	callback.setPhase("WORKER.1", events.PhaseReviewing)

	ws := NewWorkerServer("WORKER.1", store)
	ws.SetStateCallback(callback)
	handler := ws.handlers["report_review_verdict"]

	result, err := handler(context.Background(), json.RawMessage(`{"verdict": "APPROVED", "comments": "Code looks great, tests pass"}`))
	require.NoError(t, err, "Unexpected error")

	// Verify callback was called
	found := false
	for _, call := range callback.calls {
		if call.Method == "OnReviewVerdict" {
			found = true
			require.Equal(t, "APPROVED", call.Verdict, "Verdict mismatch")
			require.Equal(t, "Code looks great, tests pass", call.Comments, "Comments mismatch")
		}
	}
	require.True(t, found, "OnReviewVerdict callback not called")

	// Verify message was posted
	require.Len(t, store.appendCalls, 1, "Expected 1 message posted")
	require.Contains(t, store.appendCalls[0].Content, "Review verdict: APPROVED", "Message should contain verdict")

	// Verify structured response
	require.NotNil(t, result, "Expected result with content")
	require.NotEmpty(t, result.Content, "Expected result with content")
	require.Contains(t, result.Content[0].Text, "APPROVED", "Response should contain 'APPROVED'")
	require.Contains(t, result.Content[0].Text, "idle", "Response should contain 'idle' phase")
}

// TestWorkerServer_ReportReviewVerdict_Denied tests successful denial.
func TestWorkerServer_ReportReviewVerdict_Denied(t *testing.T) {
	store := newMockMessageStore()
	callback := newMockStateCallback()
	callback.setPhase("WORKER.1", events.PhaseReviewing)

	ws := NewWorkerServer("WORKER.1", store)
	ws.SetStateCallback(callback)
	handler := ws.handlers["report_review_verdict"]

	result, err := handler(context.Background(), json.RawMessage(`{"verdict": "DENIED", "comments": "Missing error handling in line 50"}`))
	require.NoError(t, err, "Unexpected error")

	// Verify callback was called with DENIED
	found := false
	for _, call := range callback.calls {
		if call.Method == "OnReviewVerdict" && call.Verdict == "DENIED" {
			found = true
			require.Contains(t, call.Comments, "Missing error handling", "Comments should be passed correctly")
		}
	}
	require.True(t, found, "OnReviewVerdict callback not called with DENIED")

	// Verify structured response contains DENIED
	require.Contains(t, result.Content[0].Text, "DENIED", "Response should contain 'DENIED'")
}

// TestWorkerServer_ReportImplementationCompleteSchema verifies tool schema.
func TestWorkerServer_ReportImplementationCompleteSchema(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tool, ok := ws.tools["report_implementation_complete"]
	require.True(t, ok, "report_implementation_complete tool not registered")

	require.Len(t, tool.InputSchema.Required, 1, "report_implementation_complete should have 1 required parameter")
	require.Equal(t, "summary", tool.InputSchema.Required[0], "Required parameter should be 'summary'")

	_, ok = tool.InputSchema.Properties["summary"]
	require.True(t, ok, "'summary' property should be defined")
}

// TestWorkerServer_ReportReviewVerdictSchema verifies tool schema.
func TestWorkerServer_ReportReviewVerdictSchema(t *testing.T) {
	ws := NewWorkerServer("WORKER.1", nil)

	tool, ok := ws.tools["report_review_verdict"]
	require.True(t, ok, "report_review_verdict tool not registered")

	require.Len(t, tool.InputSchema.Required, 2, "report_review_verdict should have 2 required parameters")

	requiredSet := make(map[string]bool)
	for _, r := range tool.InputSchema.Required {
		requiredSet[r] = true
	}
	require.True(t, requiredSet["verdict"], "'verdict' should be required")
	require.True(t, requiredSet["comments"], "'comments' should be required")

	_, ok = tool.InputSchema.Properties["verdict"]
	require.True(t, ok, "'verdict' property should be defined")
	_, ok = tool.InputSchema.Properties["comments"]
	require.True(t, ok, "'comments' property should be defined")
}
