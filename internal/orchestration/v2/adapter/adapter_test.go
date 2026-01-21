// Package adapter provides the MCP tool adapter layer for the v2 orchestration architecture.
package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt/roles"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
)

// ===========================================================================
// Test Helpers
// ===========================================================================

// ptr returns a pointer to the given ProcessPhase value.
func ptr(p events.ProcessPhase) *events.ProcessPhase {
	return &p
}

// mockHandler records processed commands and returns configurable results.
type mockHandler struct {
	mu           sync.Mutex
	commands     []command.Command
	returnResult *command.CommandResult
	returnErr    error
	delay        time.Duration
}

func newMockHandler() *mockHandler {
	return &mockHandler{
		returnResult: &command.CommandResult{
			Success: true,
			Data:    "mock_result",
		},
	}
}

func (h *mockHandler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	if h.delay > 0 {
		select {
		case <-time.After(h.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.commands = append(h.commands, cmd)

	if h.returnErr != nil {
		return nil, h.returnErr
	}
	return h.returnResult, nil
}

func (h *mockHandler) getCommands() []command.Command {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]command.Command, len(h.commands))
	copy(result, h.commands)
	return result
}

// mockMessageLog records appended messages for testing.
type mockMessageLog struct {
	mu       sync.Mutex
	messages []mockMessage
	err      error // Error to return from Append
}

type mockMessage struct {
	From    string
	To      string
	Content string
	MsgType message.MessageType
}

func (m *mockMessageLog) Append(from, to, content string, msgType message.MessageType) (*message.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.err != nil {
		return nil, m.err
	}

	m.messages = append(m.messages, mockMessage{
		From:    from,
		To:      to,
		Content: content,
		MsgType: msgType,
	})

	return &message.Entry{
		From:    from,
		To:      to,
		Content: content,
		Type:    msgType,
	}, nil
}

func (m *mockMessageLog) getMessages() []mockMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]mockMessage, len(m.messages))
	copy(result, m.messages)
	return result
}

// mockFullMessageRepository implements MessageRepository for testing.
type mockFullMessageRepository struct {
	mockMessageLog
	entries          []message.Entry
	unreadFor        map[string][]message.Entry
	readAndMarkCalls map[string]int
	count            int
}

func newMockFullMessageRepository() *mockFullMessageRepository {
	return &mockFullMessageRepository{
		mockMessageLog: mockMessageLog{
			messages: make([]mockMessage, 0),
		},
		entries:          make([]message.Entry, 0),
		unreadFor:        make(map[string][]message.Entry),
		readAndMarkCalls: make(map[string]int),
	}
}

func (m *mockFullMessageRepository) Entries() []message.Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]message.Entry, len(m.entries))
	copy(result, m.entries)
	return result
}

func (m *mockFullMessageRepository) ReadAndMark(agentID string) []message.Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readAndMarkCalls[agentID]++
	if unread, ok := m.unreadFor[agentID]; ok {
		result := make([]message.Entry, len(unread))
		copy(result, unread)
		// Clear unread after reading (simulates atomic read-and-mark)
		m.unreadFor[agentID] = nil
		return result
	}
	return []message.Entry{}
}

func (m *mockFullMessageRepository) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count
}

func (m *mockFullMessageRepository) Broker() *pubsub.Broker[message.Event] {
	return nil
}

func (m *mockFullMessageRepository) AppendRestored(entry message.Entry) (*message.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
	m.count++
	return &entry, nil
}

func (m *mockFullMessageRepository) addEntry(entry message.Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry)
	m.count++
}

func (m *mockFullMessageRepository) setUnreadFor(agentID string, entries []message.Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unreadFor[agentID] = entries
}

func (m *mockFullMessageRepository) getReadAndMarkCount(agentID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.readAndMarkCalls[agentID]
}

// testAdapter creates an adapter with a running processor for testing.
func testAdapter(t *testing.T, opts ...Option) (*V2Adapter, *mockHandler, func()) {
	t.Helper()

	handler := newMockHandler()
	p := processor.NewCommandProcessor()

	// Register handlers for all command types
	for _, cmdType := range []command.CommandType{
		command.CmdSpawnProcess,
		command.CmdRetireProcess,
		command.CmdReplaceProcess,
		command.CmdAssignTask,
		command.CmdAssignReview,
		command.CmdApproveCommit,
		command.CmdAssignReviewFeedback,
		command.CmdSendToProcess,
		command.CmdBroadcast,
		command.CmdDeliverProcessQueued,
		command.CmdReportComplete,
		command.CmdReportVerdict,
		command.CmdTransitionPhase,
		command.CmdMarkTaskComplete,
		command.CmdMarkTaskFailed,
		command.CmdStopProcess,
		command.CmdSignalWorkflowComplete,
		command.CmdNotifyUser,
	} {
		p.RegisterHandler(cmdType, handler)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go p.Run(ctx)

	// Wait for processor to start
	require.Eventually(t, func() bool {
		return p.IsRunning()
	}, time.Second, 10*time.Millisecond)

	adapter := NewV2Adapter(p, opts...)

	cleanup := func() {
		cancel()
		p.Stop()
	}

	return adapter, handler, cleanup
}

// toJSON converts a value to json.RawMessage.
func toJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// ===========================================================================
// Constructor Tests
// ===========================================================================

func TestNewV2Adapter(t *testing.T) {
	p := processor.NewCommandProcessor()
	adapter := NewV2Adapter(p)

	require.NotNil(t, adapter)
	assert.Equal(t, DefaultTimeout, adapter.timeout)
}

func TestNewV2Adapter_WithTimeout(t *testing.T) {
	p := processor.NewCommandProcessor()
	customTimeout := 5 * time.Second
	adapter := NewV2Adapter(p, WithTimeout(customTimeout))

	assert.Equal(t, customTimeout, adapter.timeout)
}

// ===========================================================================
// Worker Lifecycle Tests (Batch 1)
// ===========================================================================

func TestHandleSpawnProcess(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		// Set handler to return worker ID
		handler.returnResult = &command.CommandResult{
			Success: true,
			Data:    "worker-123",
		}

		result, err := adapter.HandleSpawnProcess(context.Background(), nil)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "worker-123")

		// Verify command was created correctly
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		assert.Equal(t, command.CmdSpawnProcess, cmds[0].Type())
	})

	t.Run("handler_error_wrapped_in_result", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		// When handler returns an error, processor wraps it in a CommandResult
		// with Success=false, which then gets converted to an MCP error result
		handler.returnErr = errors.New("spawn failed")

		result, err := adapter.HandleSpawnProcess(context.Background(), nil)

		// The processor wraps handler errors in result.Error, not as returned err
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "spawn failed")
	})

	t.Run("result_not_success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		handler.returnResult = &command.CommandResult{
			Success: false,
			Error:   errors.New("capacity exceeded"),
		}

		result, err := adapter.HandleSpawnProcess(context.Background(), nil)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "capacity exceeded")
	})
}

func TestHandleRetireProcess(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"worker_id": "worker-456",
			"reason":    "test reason",
		})

		result, err := adapter.HandleRetireProcess(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "worker-456")
		assert.Contains(t, result.Content[0].Text, "retired")

		// Verify command was created correctly
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		retireCmd, ok := cmds[0].(*command.RetireProcessCommand)
		require.True(t, ok)
		assert.Equal(t, "worker-456", retireCmd.ProcessID)
		assert.Equal(t, "test reason", retireCmd.Reason)
	})

	t.Run("missing_worker_id", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{})

		result, err := adapter.HandleRetireProcess(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "process_id is required")
	})

	t.Run("invalid_json", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		result, err := adapter.HandleRetireProcess(context.Background(), []byte("invalid"))

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid arguments")
	})
}

func TestHandleReplaceProcess(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"worker_id": "worker-789",
			"reason":    "replacing stuck worker",
		})

		result, err := adapter.HandleReplaceProcess(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "worker-789")

		// Verify command
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		replaceCmd, ok := cmds[0].(*command.ReplaceProcessCommand)
		require.True(t, ok)
		assert.Equal(t, "worker-789", replaceCmd.ProcessID)
	})

	t.Run("missing_worker_id", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{"reason": "test"})

		result, err := adapter.HandleReplaceProcess(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "process_id is required")
	})
}

// ===========================================================================
// Messaging Tests (Batch 2)
// ===========================================================================

func TestHandleSendToWorker(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"worker_id": "worker-123",
			"message":   "Hello worker!",
		})

		result, err := adapter.HandleSendToWorker(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "worker-123")

		// Verify command
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
		require.True(t, ok)
		assert.Equal(t, "worker-123", sendCmd.ProcessID)
		assert.Equal(t, "Hello worker!", sendCmd.Content)
	})

	t.Run("missing_worker_id", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{"message": "test"})

		result, err := adapter.HandleSendToWorker(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "process_id is required")
	})

	t.Run("missing_message", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{"worker_id": "worker-123"})

		result, err := adapter.HandleSendToWorker(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "content is required")
	})
}

func TestHandlePostMessage(t *testing.T) {
	t.Run("to_all_broadcasts", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"to":      "ALL",
			"content": "Broadcast message",
		})

		result, err := adapter.HandlePostMessage(context.Background(), args, "sender-id")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "broadcast")

		// Verify broadcast command was created
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		broadcastCmd, ok := cmds[0].(*command.BroadcastCommand)
		require.True(t, ok)
		assert.Equal(t, "Broadcast message", broadcastCmd.Content)
		assert.Contains(t, broadcastCmd.ExcludeWorkers, "sender-id")
	})

	t.Run("to_coordinator_success", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		args := toJSON(t, map[string]string{
			"to":      "COORDINATOR",
			"content": "Message to coordinator",
		})

		result, err := adapter.HandlePostMessage(context.Background(), args, "worker-1")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "Message posted to coordinator")

		// Verify message was appended to the log
		messages := msgRepo.getMessages()
		require.Len(t, messages, 1)
		assert.Equal(t, "worker-1", messages[0].From)
		assert.Equal(t, message.ActorCoordinator, messages[0].To)
		assert.Equal(t, "Message to coordinator", messages[0].Content)
		assert.Equal(t, message.MessageInfo, messages[0].MsgType)
	})

	t.Run("to_coordinator_no_message_log", func(t *testing.T) {
		// Test the error case when MessageLog is not wired
		adapter, _, cleanup := testAdapter(t) // No WithMessageRepository
		defer cleanup()

		args := toJSON(t, map[string]string{
			"to":      "COORDINATOR",
			"content": "Message to coordinator",
		})

		result, err := adapter.HandlePostMessage(context.Background(), args, "worker-1")

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "requires message repository (not wired)")
	})

	t.Run("to_coordinator_append_error", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		msgRepo.err = errors.New("database error")
		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		args := toJSON(t, map[string]string{
			"to":      "COORDINATOR",
			"content": "Message to coordinator",
		})

		result, err := adapter.HandlePostMessage(context.Background(), args, "worker-1")

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to append message to coordinator log")
		assert.Contains(t, err.Error(), "database error")
	})

	t.Run("to_specific_worker", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"to":      "WORKER.5",
			"content": "Direct message",
		})

		result, err := adapter.HandlePostMessage(context.Background(), args, "worker-1")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "WORKER.5")

		// Verify SendToProcess command was created
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		sendCmd, ok := cmds[0].(*command.SendToProcessCommand)
		require.True(t, ok)
		assert.Equal(t, "WORKER.5", sendCmd.ProcessID)
		assert.Equal(t, "Direct message", sendCmd.Content)
	})

	t.Run("missing_to", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{"content": "test"})

		result, err := adapter.HandlePostMessage(context.Background(), args, "sender")

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "to is required")
	})

	t.Run("missing_content", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{"to": "ALL"})

		result, err := adapter.HandlePostMessage(context.Background(), args, "sender")

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "content is required")
	})
}

func TestHandleReadMessageLog(t *testing.T) {
	t.Run("no_repository_configured", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		result, err := adapter.HandleReadMessageLog(context.Background(), nil, "worker-1")

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "message repository not configured for read operations")
	})

	t.Run("read_all_returns_all_entries", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		msgRepo.addEntry(message.Entry{
			ID:      "msg-1",
			From:    "worker-1",
			To:      "COORDINATOR",
			Content: "Hello",
			Type:    message.MessageInfo,
		})
		msgRepo.addEntry(message.Entry{
			ID:      "msg-2",
			From:    "COORDINATOR",
			To:      "worker-1",
			Content: "Hi back",
			Type:    message.MessageResponse,
		})

		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		args := toJSON(t, map[string]bool{"read_all": true})
		result, err := adapter.HandleReadMessageLog(context.Background(), args, "worker-1")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Parse the JSON response (now structured with messageLogResponse)
		var resp messageLogResponse
		err = json.Unmarshal([]byte(result.Content[0].Text), &resp)
		require.NoError(t, err)
		assert.Equal(t, 2, resp.TotalCount)
		assert.Equal(t, 2, resp.ReturnedCount)
		assert.Len(t, resp.Messages, 2)
		assert.Equal(t, "Hello", resp.Messages[0].Content)
		assert.Equal(t, "Hi back", resp.Messages[1].Content)

		// MarkRead should NOT be called when read_all=true
		assert.Equal(t, 0, msgRepo.getReadAndMarkCount("worker-1"))
	})

	t.Run("read_unread_returns_unread_entries", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		unread := []message.Entry{
			{
				ID:      "msg-3",
				From:    "COORDINATOR",
				To:      "worker-1",
				Content: "New task",
				Type:    message.MessageInfo,
			},
		}
		msgRepo.setUnreadFor("worker-1", unread)

		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		// read_all defaults to false
		result, err := adapter.HandleReadMessageLog(context.Background(), nil, "worker-1")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Parse the JSON response (now structured with messageLogResponse)
		var resp messageLogResponse
		err = json.Unmarshal([]byte(result.Content[0].Text), &resp)
		require.NoError(t, err)
		assert.Equal(t, 1, resp.TotalCount)
		assert.Equal(t, 1, resp.ReturnedCount)
		assert.Len(t, resp.Messages, 1)
		assert.Equal(t, "New task", resp.Messages[0].Content)

		// MarkRead should be called when read_all=false
		assert.Equal(t, 1, msgRepo.getReadAndMarkCount("worker-1"))
	})

	t.Run("read_unread_calls_mark_read", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		args := toJSON(t, map[string]bool{"read_all": false})
		_, err := adapter.HandleReadMessageLog(context.Background(), args, "worker-2")

		require.NoError(t, err)
		assert.Equal(t, 1, msgRepo.getReadAndMarkCount("worker-2"))
	})

	t.Run("invalid_json_args", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		result, err := adapter.HandleReadMessageLog(context.Background(), []byte("invalid"), "worker-1")

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid arguments")
	})
}

func TestWithMessageRepository(t *testing.T) {
	t.Run("sets_msgRepo", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		p := processor.NewCommandProcessor()
		adapter := NewV2Adapter(p, WithMessageRepository(msgRepo))

		// msgRepo should be set for read/write operations
		assert.NotNil(t, adapter.msgRepo)
	})

	t.Run("write_operations_work_via_msgRepo", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		// HandleSignalReady uses msgLog.Append
		result, err := adapter.HandleSignalReady(context.Background(), nil, "worker-123")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify message was appended via the msgRepo interface
		messages := msgRepo.getMessages()
		require.Len(t, messages, 1)
		assert.Equal(t, "worker-123", messages[0].From)
	})

}

// ===========================================================================
// Task Assignment Tests (Batch 3-4)
// ===========================================================================

func TestHandleAssignTask(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"worker_id": "worker-123",
			"task_id":   "perles-abc1",
			"summary":   "Implement feature X",
		})

		result, err := adapter.HandleAssignTask(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "perles-abc1")
		assert.Contains(t, result.Content[0].Text, "worker-123")

		// Verify command
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		assignCmd, ok := cmds[0].(*command.AssignTaskCommand)
		require.True(t, ok)
		assert.Equal(t, "worker-123", assignCmd.WorkerID)
		assert.Equal(t, "perles-abc1", assignCmd.TaskID)
		assert.Equal(t, "Implement feature X", assignCmd.Summary)
	})

	t.Run("missing_worker_id", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"task_id": "perles-abc1",
		})

		result, err := adapter.HandleAssignTask(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "worker_id is required")
	})

	t.Run("missing_task_id", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"worker_id": "worker-123",
		})

		result, err := adapter.HandleAssignTask(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "task_id is required")
	})
}

func TestHandleAssignTaskReview(t *testing.T) {
	t.Run("success_defaults_to_complex", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"reviewer_id":    "worker-reviewer",
			"task_id":        "perles-xyz9",
			"implementer_id": "worker-impl",
		})

		result, err := adapter.HandleAssignTaskReview(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "perles-xyz9")
		assert.Contains(t, result.Content[0].Text, "worker-reviewer")

		// Verify command
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		assignCmd, ok := cmds[0].(*command.AssignReviewCommand)
		require.True(t, ok)
		assert.Equal(t, "worker-reviewer", assignCmd.ReviewerID)
		assert.Equal(t, "perles-xyz9", assignCmd.TaskID)
		assert.Equal(t, "worker-impl", assignCmd.ImplementerID)
		assert.Equal(t, command.ReviewTypeComplex, assignCmd.ReviewType)
	})

	t.Run("review_type_simple_passes_simple", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"reviewer_id":    "worker-reviewer",
			"task_id":        "perles-xyz9",
			"implementer_id": "worker-impl",
			"review_type":    "simple",
		})

		result, err := adapter.HandleAssignTaskReview(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify command has ReviewTypeSimple
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		assignCmd, ok := cmds[0].(*command.AssignReviewCommand)
		require.True(t, ok)
		assert.Equal(t, command.ReviewTypeSimple, assignCmd.ReviewType)
	})

	t.Run("review_type_complex_passes_complex", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"reviewer_id":    "worker-reviewer",
			"task_id":        "perles-xyz9",
			"implementer_id": "worker-impl",
			"review_type":    "complex",
		})

		result, err := adapter.HandleAssignTaskReview(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify command has ReviewTypeComplex
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		assignCmd, ok := cmds[0].(*command.AssignReviewCommand)
		require.True(t, ok)
		assert.Equal(t, command.ReviewTypeComplex, assignCmd.ReviewType)
	})

	t.Run("review_type_invalid_defaults_to_complex", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"reviewer_id":    "worker-reviewer",
			"task_id":        "perles-xyz9",
			"implementer_id": "worker-impl",
			"review_type":    "invalid",
		})

		result, err := adapter.HandleAssignTaskReview(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify command defaults to ReviewTypeComplex for invalid value
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		assignCmd, ok := cmds[0].(*command.AssignReviewCommand)
		require.True(t, ok)
		assert.Equal(t, command.ReviewTypeComplex, assignCmd.ReviewType)
	})

	t.Run("missing_reviewer_id", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"task_id":        "perles-xyz9",
			"implementer_id": "worker-impl",
		})

		result, err := adapter.HandleAssignTaskReview(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "reviewer_id is required")
	})

	t.Run("missing_task_id", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"reviewer_id":    "worker-reviewer",
			"implementer_id": "worker-impl",
		})

		result, err := adapter.HandleAssignTaskReview(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "task_id is required")
	})

	t.Run("missing_implementer_id", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"reviewer_id": "worker-reviewer",
			"task_id":     "perles-xyz9",
		})

		result, err := adapter.HandleAssignTaskReview(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "implementer_id is required")
	})
}

func TestHandleAssignReviewFeedback(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"implementer_id": "worker-impl",
			"task_id":        "perles-abc1",
			"feedback":       "Please fix the edge case handling",
		})

		result, err := adapter.HandleAssignReviewFeedback(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "worker-impl")

		// Verify AssignReviewFeedback command was created
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		feedbackCmd, ok := cmds[0].(*command.AssignReviewFeedbackCommand)
		require.True(t, ok)
		assert.Equal(t, "worker-impl", feedbackCmd.ImplementerID)
		assert.Equal(t, "perles-abc1", feedbackCmd.TaskID)
		assert.Equal(t, "Please fix the edge case handling", feedbackCmd.Feedback)
	})

	t.Run("missing_implementer_id", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"task_id":  "perles-abc1",
			"feedback": "feedback",
		})

		result, err := adapter.HandleAssignReviewFeedback(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "implementer_id is required")
	})

	t.Run("missing_task_id", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"implementer_id": "worker-impl",
			"feedback":       "feedback",
		})

		result, err := adapter.HandleAssignReviewFeedback(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "task_id is required")
	})

	t.Run("missing_feedback", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"implementer_id": "worker-impl",
			"task_id":        "perles-abc1",
		})

		result, err := adapter.HandleAssignReviewFeedback(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "feedback is required")
	})
}

func TestHandleApproveCommit(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"implementer_id": "worker-impl",
			"task_id":        "perles-abc1",
			"commit_message": "feat: add new feature",
		})

		result, err := adapter.HandleApproveCommit(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "worker-impl")
		assert.Contains(t, result.Content[0].Text, "perles-abc1")

		// Verify command
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		approveCmd, ok := cmds[0].(*command.ApproveCommitCommand)
		require.True(t, ok)
		assert.Equal(t, "worker-impl", approveCmd.ImplementerID)
		assert.Equal(t, "perles-abc1", approveCmd.TaskID)
	})

	t.Run("missing_implementer_id", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"task_id": "perles-abc1",
		})

		result, err := adapter.HandleApproveCommit(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "implementer_id is required")
	})

	t.Run("missing_task_id", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"implementer_id": "worker-impl",
		})

		result, err := adapter.HandleApproveCommit(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "task_id is required")
	})
}

// ===========================================================================
// State Transition Tests (Batch 5)
// ===========================================================================

func TestHandleSignalReady(t *testing.T) {
	t.Run("posts_message_to_log", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		result, err := adapter.HandleSignalReady(context.Background(), nil, "worker-123")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "worker-123")
		assert.Contains(t, result.Content[0].Text, "ready")

		// Verify message was posted to log
		messages := msgRepo.getMessages()
		require.Len(t, messages, 1)
		assert.Equal(t, "worker-123", messages[0].From)
		assert.Equal(t, message.ActorCoordinator, messages[0].To)
		assert.Equal(t, message.MessageWorkerReady, messages[0].MsgType)
		assert.Contains(t, messages[0].Content, "worker-123")
		assert.Contains(t, messages[0].Content, "ready")
	})

	t.Run("succeeds_without_message_log", func(t *testing.T) {
		// When no message log is configured, signal_ready still succeeds
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		result, err := adapter.HandleSignalReady(context.Background(), nil, "worker-456")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "worker-456")
	})

	t.Run("returns_error_on_log_failure", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		msgRepo.err = errors.New("log write failed")
		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		result, err := adapter.HandleSignalReady(context.Background(), nil, "worker-789")

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to post ready message")
	})
}

func TestHandleReportImplementationComplete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"summary": "Implemented the feature successfully",
		})

		result, err := adapter.HandleReportImplementationComplete(context.Background(), args, "worker-456")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "Implementation complete")

		// Verify command
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		reportCmd, ok := cmds[0].(*command.ReportCompleteCommand)
		require.True(t, ok)
		assert.Equal(t, "worker-456", reportCmd.WorkerID)
		assert.Equal(t, "Implemented the feature successfully", reportCmd.Summary)
	})

	t.Run("invalid_json", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		result, err := adapter.HandleReportImplementationComplete(context.Background(), []byte("invalid"), "worker-456")

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid arguments")
	})

	t.Run("posts_completion_message_to_log", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		args := toJSON(t, map[string]string{
			"summary": "Implemented feature X",
		})

		result, err := adapter.HandleReportImplementationComplete(context.Background(), args, "worker-123")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify message was posted to log
		messages := msgRepo.getMessages()
		require.Len(t, messages, 1)
		assert.Equal(t, "worker-123", messages[0].From)
		assert.Equal(t, message.ActorCoordinator, messages[0].To)
		assert.Equal(t, message.MessageCompletion, messages[0].MsgType)
		assert.Contains(t, messages[0].Content, "Implementation complete")
		assert.Contains(t, messages[0].Content, "Implemented feature X")
	})

	t.Run("posts_completion_message_without_summary", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		args := toJSON(t, map[string]string{})

		result, err := adapter.HandleReportImplementationComplete(context.Background(), args, "worker-123")

		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify message content when no summary
		messages := msgRepo.getMessages()
		require.Len(t, messages, 1)
		assert.Equal(t, "Implementation complete", messages[0].Content)
	})

	t.Run("returns_error_on_log_failure", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		msgRepo.err = errors.New("log write failed")
		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		args := toJSON(t, map[string]string{
			"summary": "Done",
		})

		result, err := adapter.HandleReportImplementationComplete(context.Background(), args, "worker-123")

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to post completion message")
	})
}

func TestHandleReportReviewVerdict(t *testing.T) {
	t.Run("approved", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"verdict":  "APPROVED",
			"comments": "LGTM",
		})

		result, err := adapter.HandleReportReviewVerdict(context.Background(), args, "worker-reviewer")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "APPROVED")

		// Verify command
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		reportCmd, ok := cmds[0].(*command.ReportVerdictCommand)
		require.True(t, ok)
		assert.Equal(t, "worker-reviewer", reportCmd.WorkerID)
		assert.Equal(t, command.VerdictApproved, reportCmd.Verdict)
		assert.Equal(t, "LGTM", reportCmd.Comments)
	})

	t.Run("denied", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"verdict":  "DENIED",
			"comments": "Needs more tests",
		})

		result, err := adapter.HandleReportReviewVerdict(context.Background(), args, "worker-reviewer")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "DENIED")

		// Verify command
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		reportCmd, ok := cmds[0].(*command.ReportVerdictCommand)
		require.True(t, ok)
		assert.Equal(t, command.VerdictDenied, reportCmd.Verdict)
		assert.Equal(t, "Needs more tests", reportCmd.Comments)
	})

	t.Run("missing_verdict", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"comments": "some comments",
		})

		result, err := adapter.HandleReportReviewVerdict(context.Background(), args, "worker-reviewer")

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "verdict is required")
	})

	t.Run("invalid_verdict", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"verdict": "MAYBE",
		})

		result, err := adapter.HandleReportReviewVerdict(context.Background(), args, "worker-reviewer")

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid verdict")
	})

	t.Run("posts_verdict_message_to_log", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		args := toJSON(t, map[string]string{
			"verdict":  "APPROVED",
			"comments": "LGTM, great work!",
		})

		result, err := adapter.HandleReportReviewVerdict(context.Background(), args, "worker-reviewer")

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify message was posted to log
		messages := msgRepo.getMessages()
		require.Len(t, messages, 1)
		assert.Equal(t, "worker-reviewer", messages[0].From)
		assert.Equal(t, message.ActorCoordinator, messages[0].To)
		assert.Equal(t, message.MessageCompletion, messages[0].MsgType)
		assert.Contains(t, messages[0].Content, "Review verdict: APPROVED")
		assert.Contains(t, messages[0].Content, "LGTM, great work!")
	})

	t.Run("posts_verdict_message_without_comments", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		args := toJSON(t, map[string]string{
			"verdict": "DENIED",
		})

		result, err := adapter.HandleReportReviewVerdict(context.Background(), args, "worker-reviewer")

		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify message content when no comments
		messages := msgRepo.getMessages()
		require.Len(t, messages, 1)
		assert.Equal(t, "Review verdict: DENIED", messages[0].Content)
	})

	t.Run("returns_error_on_log_failure", func(t *testing.T) {
		msgRepo := newMockFullMessageRepository()
		msgRepo.err = errors.New("log write failed")
		adapter, _, cleanup := testAdapter(t, WithMessageRepository(msgRepo))
		defer cleanup()

		args := toJSON(t, map[string]string{
			"verdict": "APPROVED",
		})

		result, err := adapter.HandleReportReviewVerdict(context.Background(), args, "worker-reviewer")

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "failed to post verdict message")
	})
}

// ===========================================================================
// BD Integration Tests (Batch 6)
// ===========================================================================

func TestHandleMarkTaskComplete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"task_id": "perles-abc1",
		})

		result, err := adapter.HandleMarkTaskComplete(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "perles-abc1")
		assert.Contains(t, result.Content[0].Text, "completed")

		// Verify command was created correctly
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		markCmd, ok := cmds[0].(*command.MarkTaskCompleteCommand)
		require.True(t, ok)
		assert.Equal(t, "perles-abc1", markCmd.TaskID)
	})

	t.Run("missing_task_id", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{})

		result, err := adapter.HandleMarkTaskComplete(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "task_id is required")
	})

	t.Run("invalid_json", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		result, err := adapter.HandleMarkTaskComplete(context.Background(), []byte("invalid"))

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid arguments")
	})

	t.Run("handler_error_wrapped_in_result", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		handler.returnErr = errors.New("bd update failed")

		args := toJSON(t, map[string]string{
			"task_id": "perles-abc1",
		})

		result, err := adapter.HandleMarkTaskComplete(context.Background(), args)

		// The processor wraps handler errors in result.Error, not as returned err
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "bd update failed")
	})

	t.Run("result_not_success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		handler.returnResult = &command.CommandResult{
			Success: false,
			Error:   errors.New("database error"),
		}

		args := toJSON(t, map[string]string{
			"task_id": "perles-abc1",
		})

		result, err := adapter.HandleMarkTaskComplete(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "database error")
	})
}

func TestHandleMarkTaskFailed(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"task_id": "perles-xyz9",
			"reason":  "Tests failed",
		})

		result, err := adapter.HandleMarkTaskFailed(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "perles-xyz9")
		assert.Contains(t, result.Content[0].Text, "failed")
		assert.Contains(t, result.Content[0].Text, "Tests failed")

		// Verify command was created correctly
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		markCmd, ok := cmds[0].(*command.MarkTaskFailedCommand)
		require.True(t, ok)
		assert.Equal(t, "perles-xyz9", markCmd.TaskID)
		assert.Equal(t, "Tests failed", markCmd.Reason)
	})

	t.Run("missing_task_id", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"reason": "Some reason",
		})

		result, err := adapter.HandleMarkTaskFailed(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "task_id is required")
	})

	t.Run("missing_reason", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{
			"task_id": "perles-xyz9",
		})

		result, err := adapter.HandleMarkTaskFailed(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "reason is required")
	})

	t.Run("invalid_json", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		result, err := adapter.HandleMarkTaskFailed(context.Background(), []byte("invalid"))

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid arguments")
	})

	t.Run("handler_error_wrapped_in_result", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		handler.returnErr = errors.New("bd comment failed")

		args := toJSON(t, map[string]string{
			"task_id": "perles-xyz9",
			"reason":  "Tests failed",
		})

		result, err := adapter.HandleMarkTaskFailed(context.Background(), args)

		// The processor wraps handler errors in result.Error, not as returned err
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "bd comment failed")
	})

	t.Run("result_not_success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		handler.returnResult = &command.CommandResult{
			Success: false,
			Error:   errors.New("database error"),
		}

		args := toJSON(t, map[string]string{
			"task_id": "perles-xyz9",
			"reason":  "Tests failed",
		})

		result, err := adapter.HandleMarkTaskFailed(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "database error")
	})
}

// ===========================================================================
// Timeout Tests
// ===========================================================================

func TestAdapter_Timeout(t *testing.T) {
	t.Run("context_deadline_exceeded", func(t *testing.T) {
		handler := newMockHandler()
		handler.delay = 200 * time.Millisecond

		p := processor.NewCommandProcessor()
		p.RegisterHandler(command.CmdSpawnProcess, handler)

		ctx, cancel := context.WithCancel(context.Background())
		go p.Run(ctx)
		defer func() {
			cancel()
			p.Stop()
		}()

		require.Eventually(t, func() bool {
			return p.IsRunning()
		}, time.Second, 10*time.Millisecond)

		// Create adapter with very short timeout
		adapter := NewV2Adapter(p, WithTimeout(50*time.Millisecond))

		result, err := adapter.HandleSpawnProcess(context.Background(), nil)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "spawn_process command failed")
	})
}

// ===========================================================================
// Queue Full Tests
// ===========================================================================

func TestAdapter_QueueFull(t *testing.T) {
	// Create processor with tiny queue and slow handler
	handler := newMockHandler()
	handler.delay = 500 * time.Millisecond

	p := processor.NewCommandProcessor(processor.WithQueueCapacity(1))
	p.RegisterHandler(command.CmdSpawnProcess, handler)

	ctx, cancel := context.WithCancel(context.Background())
	go p.Run(ctx)
	defer func() {
		cancel()
		p.Stop()
	}()

	require.Eventually(t, func() bool {
		return p.IsRunning()
	}, time.Second, 10*time.Millisecond)

	adapter := NewV2Adapter(p)

	// Submit first command (will be processing)
	go func() {
		_, _ = adapter.HandleSpawnProcess(context.Background(), nil)
	}()

	// Wait a bit for first command to start processing
	time.Sleep(50 * time.Millisecond)

	// Fill the queue
	go func() {
		_, _ = adapter.HandleSpawnProcess(context.Background(), nil)
	}()

	// Wait a bit for queue to fill
	time.Sleep(50 * time.Millisecond)

	// Now the queue should be full
	result, err := adapter.HandleSpawnProcess(context.Background(), nil)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "queue is full")
}

// ===========================================================================
// Integration Tests
// ===========================================================================

func TestAdapter_IntegrationFullMCPCall(t *testing.T) {
	// Test a full MCP call through adapter to handler
	adapter, handler, cleanup := testAdapter(t)
	defer cleanup()

	// Configure handler to return specific result
	handler.returnResult = &command.CommandResult{
		Success: true,
		Data:    "new-worker-id-999",
	}

	// Make the call
	result, err := adapter.HandleSpawnProcess(context.Background(), nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)
	assert.Contains(t, result.Content[0].Text, "new-worker-id-999")

	// Verify command went through processor
	cmds := handler.getCommands()
	require.Len(t, cmds, 1)

	// Verify command properties
	cmd := cmds[0]
	assert.Equal(t, command.CmdSpawnProcess, cmd.Type())
	assert.NotEmpty(t, cmd.ID())
	assert.NotZero(t, cmd.CreatedAt())
}

func TestAdapter_IntegrationMultipleTools(t *testing.T) {
	// Test multiple different tools in sequence
	adapter, handler, cleanup := testAdapter(t)
	defer cleanup()

	// Tool 1: Spawn worker
	handler.returnResult = &command.CommandResult{Success: true, Data: "worker-1"}
	result1, err := adapter.HandleSpawnProcess(context.Background(), nil)
	require.NoError(t, err)
	assert.False(t, result1.IsError)

	// Tool 2: Assign task
	handler.returnResult = &command.CommandResult{Success: true}
	args := toJSON(t, map[string]string{
		"worker_id": "worker-1",
		"task_id":   "perles-abc1",
	})
	result2, err := adapter.HandleAssignTask(context.Background(), args)
	require.NoError(t, err)
	assert.False(t, result2.IsError)

	// Tool 3: Report implementation complete
	handler.returnResult = &command.CommandResult{Success: true}
	args = toJSON(t, map[string]string{
		"summary": "Done",
	})
	result3, err := adapter.HandleReportImplementationComplete(context.Background(), args, "worker-1")
	require.NoError(t, err)
	assert.False(t, result3.IsError)

	// Verify all commands were processed
	cmds := handler.getCommands()
	require.Len(t, cmds, 3)
	assert.Equal(t, command.CmdSpawnProcess, cmds[0].Type())
	assert.Equal(t, command.CmdAssignTask, cmds[1].Type())
	assert.Equal(t, command.CmdReportComplete, cmds[2].Type())
}

// ===========================================================================
// Repository Read Tests (Read-Only Operations)
// ===========================================================================

func TestHandleQueryWorkerState(t *testing.T) {
	t.Run("no_repository_configured", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]string{"worker_id": "worker-1"})
		result, err := adapter.HandleQueryWorkerState(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "process repository not configured")
	})

	t.Run("no_filter_returns_all_workers", func(t *testing.T) {
		// When no filter is provided, should return all active workers
		processRepo := repository.NewMemoryProcessRepository()
		adapter, _, cleanup := testAdapter(t,
			WithProcessRepository(processRepo),
		)
		defer cleanup()

		// Add workers
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-1",
			Role:      repository.RoleWorker,
			Status:    repository.StatusReady,
			Phase:     ptr(events.ProcessPhaseIdle),
			CreatedAt: time.Now(),
		})
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-2",
			Role:      repository.RoleWorker,
			Status:    repository.StatusWorking,
			Phase:     ptr(events.ProcessPhaseImplementing),
			TaskID:    "task-123",
			CreatedAt: time.Now(),
		})

		// No filter args
		result, err := adapter.HandleQueryWorkerState(context.Background(), nil)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Parse response
		var response struct {
			Workers      []map[string]any `json:"workers"`
			ReadyWorkers []string         `json:"ready_workers"`
		}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)
		assert.Len(t, response.Workers, 2)
	})

	t.Run("worker_not_found_returns_empty", func(t *testing.T) {
		// When filtering by worker_id that doesn't exist, returns empty workers array
		processRepo := repository.NewMemoryProcessRepository()
		adapter, _, cleanup := testAdapter(t,
			WithProcessRepository(processRepo),
		)
		defer cleanup()

		args := toJSON(t, map[string]string{"worker_id": "nonexistent"})
		result, err := adapter.HandleQueryWorkerState(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Parse response - should have empty workers array
		var response struct {
			Workers      []map[string]any `json:"workers"`
			ReadyWorkers []string         `json:"ready_workers"`
		}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)
		assert.Empty(t, response.Workers)
		assert.Empty(t, response.ReadyWorkers)
	})

	t.Run("success_basic_worker", func(t *testing.T) {
		processRepo := repository.NewMemoryProcessRepository()

		now := time.Now()
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-123",
			Role:      repository.RoleWorker,
			Status:    repository.StatusReady,
			Phase:     ptr(events.ProcessPhaseIdle),
			SessionID: "session-abc",
			CreatedAt: now,
		})

		adapter, _, cleanup := testAdapter(t,
			WithProcessRepository(processRepo),
		)
		defer cleanup()

		args := toJSON(t, map[string]string{"worker_id": "worker-123"})
		result, err := adapter.HandleQueryWorkerState(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Parse the JSON response - now has workers array
		var response struct {
			Workers      []map[string]any `json:"workers"`
			ReadyWorkers []string         `json:"ready_workers"`
		}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Len(t, response.Workers, 1)
		w := response.Workers[0]
		assert.Equal(t, "worker-123", w["worker_id"])
		assert.Equal(t, "ready", w["status"])
		assert.Equal(t, "idle", w["phase"])
		// started_at should be present (time format HH:MM:SS)
		assert.NotEmpty(t, w["started_at"])

		// Ready worker with no task should be in ready_workers
		assert.Contains(t, response.ReadyWorkers, "worker-123")
	})

	t.Run("success_with_task", func(t *testing.T) {
		processRepo := repository.NewMemoryProcessRepository()

		now := time.Now()
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-456",
			Role:      repository.RoleWorker,
			Status:    repository.StatusWorking,
			Phase:     ptr(events.ProcessPhaseImplementing),
			TaskID:    "task-xyz",
			SessionID: "session-def",
			CreatedAt: now,
		})

		adapter, _, cleanup := testAdapter(t,
			WithProcessRepository(processRepo),
		)
		defer cleanup()

		args := toJSON(t, map[string]string{"worker_id": "worker-456"})
		result, err := adapter.HandleQueryWorkerState(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Parse the JSON response
		var response struct {
			Workers      []map[string]any `json:"workers"`
			ReadyWorkers []string         `json:"ready_workers"`
		}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Len(t, response.Workers, 1)
		w := response.Workers[0]
		assert.Equal(t, "worker-456", w["worker_id"])
		assert.Equal(t, "working", w["status"])
		assert.Equal(t, "implementing", w["phase"])
		assert.Equal(t, "task-xyz", w["task_id"])

		// Worker with task should NOT be in ready_workers
		assert.Empty(t, response.ReadyWorkers)
	})

	t.Run("invalid_json_args", func(t *testing.T) {
		processRepo := repository.NewMemoryProcessRepository()
		adapter, _, cleanup := testAdapter(t,
			WithProcessRepository(processRepo),
		)
		defer cleanup()

		result, err := adapter.HandleQueryWorkerState(context.Background(), []byte("invalid"))

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid arguments")
	})
}

func TestHandleQueryWorkerState_WithTaskFilter(t *testing.T) {
	t.Run("filters_by_task_id", func(t *testing.T) {
		processRepo := repository.NewMemoryProcessRepository()

		// Add workers with different tasks
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-1",
			Role:      repository.RoleWorker,
			Status:    repository.StatusWorking,
			Phase:     ptr(events.ProcessPhaseImplementing),
			TaskID:    "task-abc",
			CreatedAt: time.Now(),
		})
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-2",
			Role:      repository.RoleWorker,
			Status:    repository.StatusWorking,
			Phase:     ptr(events.ProcessPhaseImplementing),
			TaskID:    "task-xyz",
			CreatedAt: time.Now(),
		})
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-3",
			Role:      repository.RoleWorker,
			Status:    repository.StatusReady,
			Phase:     ptr(events.ProcessPhaseIdle),
			CreatedAt: time.Now(),
		})

		adapter, _, cleanup := testAdapter(t,
			WithProcessRepository(processRepo),
		)
		defer cleanup()

		args := toJSON(t, map[string]string{"task_id": "task-abc"})
		result, err := adapter.HandleQueryWorkerState(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Parse response - should only have worker-1
		var response struct {
			Workers      []map[string]any `json:"workers"`
			ReadyWorkers []string         `json:"ready_workers"`
		}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Len(t, response.Workers, 1)
		assert.Equal(t, "worker-1", response.Workers[0]["worker_id"])
		assert.Equal(t, "task-abc", response.Workers[0]["task_id"])
	})

	t.Run("task_id_not_found_returns_empty", func(t *testing.T) {
		processRepo := repository.NewMemoryProcessRepository()

		_ = processRepo.Save(&repository.Process{
			ID:        "worker-1",
			Role:      repository.RoleWorker,
			Status:    repository.StatusWorking,
			Phase:     ptr(events.ProcessPhaseImplementing),
			TaskID:    "task-abc",
			CreatedAt: time.Now(),
		})

		adapter, _, cleanup := testAdapter(t,
			WithProcessRepository(processRepo),
		)
		defer cleanup()

		args := toJSON(t, map[string]string{"task_id": "nonexistent-task"})
		result, err := adapter.HandleQueryWorkerState(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Parse response - should be empty
		var response struct {
			Workers      []map[string]any `json:"workers"`
			ReadyWorkers []string         `json:"ready_workers"`
		}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)
		assert.Empty(t, response.Workers)
	})
}

func TestHandleQueryWorkerState_ReturnsReadyWorkers(t *testing.T) {
	t.Run("includes_ready_workers_with_no_task", func(t *testing.T) {
		processRepo := repository.NewMemoryProcessRepository()

		// Add mix of ready and busy workers
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-1",
			Role:      repository.RoleWorker,
			Status:    repository.StatusReady,
			Phase:     ptr(events.ProcessPhaseIdle),
			CreatedAt: time.Now(),
		})
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-2",
			Role:      repository.RoleWorker,
			Status:    repository.StatusWorking,
			Phase:     ptr(events.ProcessPhaseImplementing),
			TaskID:    "task-123",
			CreatedAt: time.Now(),
		})
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-3",
			Role:      repository.RoleWorker,
			Status:    repository.StatusReady,
			Phase:     ptr(events.ProcessPhaseIdle),
			CreatedAt: time.Now(),
		})

		adapter, _, cleanup := testAdapter(t,
			WithProcessRepository(processRepo),
		)
		defer cleanup()

		result, err := adapter.HandleQueryWorkerState(context.Background(), nil)

		require.NoError(t, err)
		require.NotNil(t, result)

		var response struct {
			Workers      []map[string]any `json:"workers"`
			ReadyWorkers []string         `json:"ready_workers"`
		}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		// Should have all 3 workers
		assert.Len(t, response.Workers, 3)

		// Only worker-1 and worker-3 should be in ready_workers
		assert.Len(t, response.ReadyWorkers, 2)
		assert.Contains(t, response.ReadyWorkers, "worker-1")
		assert.Contains(t, response.ReadyWorkers, "worker-3")
		assert.NotContains(t, response.ReadyWorkers, "worker-2")
	})

	t.Run("ready_worker_with_task_not_in_ready_workers", func(t *testing.T) {
		processRepo := repository.NewMemoryProcessRepository()

		// Worker that is Ready status but has a task assigned
		// (edge case - should not be in ready_workers)
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-1",
			Role:      repository.RoleWorker,
			Status:    repository.StatusReady,
			Phase:     ptr(events.ProcessPhaseIdle),
			TaskID:    "task-123", // Has task assigned
			CreatedAt: time.Now(),
		})

		adapter, _, cleanup := testAdapter(t,
			WithProcessRepository(processRepo),
		)
		defer cleanup()

		result, err := adapter.HandleQueryWorkerState(context.Background(), nil)

		require.NoError(t, err)
		require.NotNil(t, result)

		var response struct {
			Workers      []map[string]any `json:"workers"`
			ReadyWorkers []string         `json:"ready_workers"`
		}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		// Worker should be in workers list
		assert.Len(t, response.Workers, 1)

		// But NOT in ready_workers since it has a task
		assert.Empty(t, response.ReadyWorkers)
	})
}

func TestHandleQueryWorkerState_MatchesCoordinatorFormat(t *testing.T) {
	// Verify response format includes all required fields for comprehensive worker state queries
	processRepo := repository.NewMemoryProcessRepository()

	specificTime := time.Date(2025, 12, 31, 14, 30, 45, 0, time.UTC)
	_ = processRepo.Save(&repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     ptr(events.ProcessPhaseImplementing),
		TaskID:    "task-123",
		SessionID: "session-abc",
		CreatedAt: specificTime,
		Metrics: &metrics.TokenMetrics{
			TokensUsed:  50000,
			TotalTokens: 200000,
		},
	})

	adapter, _, cleanup := testAdapter(t,
		WithProcessRepository(processRepo),
	)
	defer cleanup()

	result, err := adapter.HandleQueryWorkerState(context.Background(), nil)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Parse response
	var response struct {
		Workers      []map[string]any `json:"workers"`
		ReadyWorkers []string         `json:"ready_workers"`
	}
	err = json.Unmarshal([]byte(result.Content[0].Text), &response)
	require.NoError(t, err)

	require.Len(t, response.Workers, 1)
	w := response.Workers[0]

	// Verify core field names
	assert.Equal(t, "worker-1", w["worker_id"]) // worker_id not id
	assert.Equal(t, "working", w["status"])
	assert.Equal(t, "implementing", w["phase"])
	assert.Equal(t, "task-123", w["task_id"])
	assert.Equal(t, "2025-12-31T14:30:45Z", w["started_at"]) // ISO 8601 format
	assert.Equal(t, "50k/200k (25%)", w["context_usage"])    // formatted usage
	assert.NotContains(t, w, "id")                           // should NOT have "id" field

	// Verify restored fields are present
	assert.Equal(t, "session-abc", w["session_id"])          // session_id restored
	assert.Equal(t, "2025-12-31T14:30:45Z", w["created_at"]) // created_at restored (ISO format)
	// Note: queue_size is omitempty, so it won't appear when 0 (and no queue repo configured)
}

func TestHandleQueryWorkerState_IncludesTaskAssignmentDetails(t *testing.T) {
	// Verify that task assignment details are populated from task repository
	processRepo := repository.NewMemoryProcessRepository()
	taskRepo := repository.NewMemoryTaskRepository()

	now := time.Now()
	taskStarted := now.Add(-30 * time.Minute)

	// Create worker with task
	_ = processRepo.Save(&repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusWorking,
		Phase:     ptr(events.ProcessPhaseReviewing),
		TaskID:    "task-123",
		SessionID: "session-abc",
		CreatedAt: now,
	})

	// Create task assignment with reviewer
	_ = taskRepo.Save(&repository.TaskAssignment{
		TaskID:      "task-123",
		Implementer: "worker-1",
		Reviewer:    "worker-2",
		Status:      repository.TaskInReview,
		StartedAt:   taskStarted,
	})

	adapter, _, cleanup := testAdapter(t,
		WithProcessRepository(processRepo),
		WithTaskRepository(taskRepo),
	)
	defer cleanup()

	result, err := adapter.HandleQueryWorkerState(context.Background(), nil)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Parse response
	var response struct {
		Workers      []map[string]any `json:"workers"`
		ReadyWorkers []string         `json:"ready_workers"`
	}
	err = json.Unmarshal([]byte(result.Content[0].Text), &response)
	require.NoError(t, err)

	require.Len(t, response.Workers, 1)
	w := response.Workers[0]

	// Verify task assignment details are populated
	assert.Equal(t, "in_review", w["task_status"])                                      // task status from repository
	assert.Equal(t, "worker-2", w["reviewer_id"])                                       // reviewer ID from task assignment
	assert.Equal(t, taskStarted.Format("2006-01-02T15:04:05Z07:00"), w["task_started"]) // task started timestamp
}

func TestHandleQueryWorkerState_IncludesRetiredAt(t *testing.T) {
	// Verify that retired_at is included when worker is retired
	processRepo := repository.NewMemoryProcessRepository()

	createdAt := time.Now().Add(-1 * time.Hour)
	retiredAt := time.Now().Add(-10 * time.Minute)

	_ = processRepo.Save(&repository.Process{
		ID:        "worker-1",
		Status:    repository.StatusRetired,
		Phase:     ptr(events.ProcessPhaseIdle),
		CreatedAt: createdAt,
		RetiredAt: retiredAt,
	})

	adapter, _, cleanup := testAdapter(t,
		WithProcessRepository(processRepo),
	)
	defer cleanup()

	// Need to use List() which returns all workers, ActiveWorkers() excludes retired
	// Actually let me check if retired workers show up
	result, err := adapter.HandleQueryWorkerState(context.Background(), nil)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Parse response
	var response struct {
		Workers      []map[string]any `json:"workers"`
		ReadyWorkers []string         `json:"ready_workers"`
	}
	err = json.Unmarshal([]byte(result.Content[0].Text), &response)
	require.NoError(t, err)

	// Note: ActiveWorkers() may not return retired workers
	// If test shows 0 workers, this is expected behavior
	if len(response.Workers) > 0 {
		w := response.Workers[0]
		assert.Equal(t, retiredAt.Format("2006-01-02T15:04:05Z07:00"), w["retired_at"])
	}
}

func TestHandleQueryWorkerState_IncludesQueueSize(t *testing.T) {
	// Verify that queue_size is populated from queue repository
	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(100) // maxSize of 100

	_ = processRepo.Save(&repository.Process{
		ID:        "worker-1",
		Role:      repository.RoleWorker,
		Status:    repository.StatusReady,
		Phase:     ptr(events.ProcessPhaseIdle),
		CreatedAt: time.Now(),
	})

	// Add messages to queue
	queue := queueRepo.GetOrCreate("worker-1")
	_ = queue.Enqueue("message 1", repository.SenderUser)
	_ = queue.Enqueue("message 2", repository.SenderUser)
	_ = queue.Enqueue("message 3", repository.SenderUser)

	adapter, _, cleanup := testAdapter(t,
		WithProcessRepository(processRepo),
		WithQueueRepository(queueRepo),
	)
	defer cleanup()

	result, err := adapter.HandleQueryWorkerState(context.Background(), nil)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Parse response
	var response struct {
		Workers      []map[string]any `json:"workers"`
		ReadyWorkers []string         `json:"ready_workers"`
	}
	err = json.Unmarshal([]byte(result.Content[0].Text), &response)
	require.NoError(t, err)

	require.Len(t, response.Workers, 1)
	w := response.Workers[0]

	// queue_size should be 3
	assert.Equal(t, float64(3), w["queue_size"]) // JSON numbers are float64
}

// ===========================================================================
// Worker Control Tests
// ===========================================================================

func TestAdapter_HandleStopProcess_SubmitsCommand(t *testing.T) {
	adapter, handler, cleanup := testAdapter(t)
	defer cleanup()

	err := adapter.HandleStopProcess("worker-123", false, "test reason")

	require.NoError(t, err)

	// Wait for command to be processed (Submit is async)
	require.Eventually(t, func() bool {
		return len(handler.getCommands()) == 1
	}, time.Second, 10*time.Millisecond)

	// Verify command was created correctly
	cmds := handler.getCommands()
	require.Len(t, cmds, 1)
	stopCmd, ok := cmds[0].(*command.StopProcessCommand)
	require.True(t, ok)
	assert.Equal(t, "worker-123", stopCmd.ProcessID)
	assert.False(t, stopCmd.Force)
	assert.Equal(t, "test reason", stopCmd.Reason)
	assert.Equal(t, command.SourceMCPTool, stopCmd.Source())
}

func TestAdapter_HandleStopProcess_ValidationError(t *testing.T) {
	adapter, handler, cleanup := testAdapter(t)
	defer cleanup()

	err := adapter.HandleStopProcess("", false, "reason")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid stop process command")
	assert.Contains(t, err.Error(), "process_id is required")

	// Verify no command was submitted
	cmds := handler.getCommands()
	assert.Len(t, cmds, 0)
}

func TestAdapter_HandleStopProcess_ForceFlag(t *testing.T) {
	adapter, handler, cleanup := testAdapter(t)
	defer cleanup()

	err := adapter.HandleStopProcess("worker-456", true, "emergency stop")

	require.NoError(t, err)

	// Wait for command to be processed (Submit is async)
	require.Eventually(t, func() bool {
		return len(handler.getCommands()) == 1
	}, time.Second, 10*time.Millisecond)

	// Verify force flag is passed through
	cmds := handler.getCommands()
	require.Len(t, cmds, 1)
	stopCmd, ok := cmds[0].(*command.StopProcessCommand)
	require.True(t, ok)
	assert.Equal(t, "worker-456", stopCmd.ProcessID)
	assert.True(t, stopCmd.Force)
	assert.Equal(t, "emergency stop", stopCmd.Reason)
}

// ===========================================================================
// Agent Type Tests
// ===========================================================================

func TestHandleSpawnProcess_WithAgentType(t *testing.T) {
	t.Run("agent_type_implementer_parsed_correctly", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		// Set handler to return worker ID
		handler.returnResult = &command.CommandResult{
			Success: true,
			Data:    "worker-123",
		}

		args := toJSON(t, map[string]string{
			"agent_type": "implementer",
		})

		result, err := adapter.HandleSpawnProcess(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		// Verify command was created with correct AgentType
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		spawnCmd, ok := cmds[0].(*command.SpawnProcessCommand)
		require.True(t, ok)
		assert.Equal(t, "implementer", string(spawnCmd.AgentType))
	})

	t.Run("agent_type_reviewer_parsed_correctly", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		handler.returnResult = &command.CommandResult{
			Success: true,
			Data:    "worker-456",
		}

		args := toJSON(t, map[string]string{
			"agent_type": "reviewer",
		})

		result, err := adapter.HandleSpawnProcess(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		spawnCmd, ok := cmds[0].(*command.SpawnProcessCommand)
		require.True(t, ok)
		assert.Equal(t, "reviewer", string(spawnCmd.AgentType))
	})

	t.Run("agent_type_researcher_parsed_correctly", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		handler.returnResult = &command.CommandResult{
			Success: true,
			Data:    "worker-789",
		}

		args := toJSON(t, map[string]string{
			"agent_type": "researcher",
		})

		result, err := adapter.HandleSpawnProcess(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)

		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		spawnCmd, ok := cmds[0].(*command.SpawnProcessCommand)
		require.True(t, ok)
		assert.Equal(t, "researcher", string(spawnCmd.AgentType))
	})
}

func TestHandleSpawnProcess_InvalidAgentType_ReturnsError(t *testing.T) {
	adapter, handler, cleanup := testAdapter(t)
	defer cleanup()

	args := toJSON(t, map[string]string{
		"agent_type": "invalid_type",
	})

	result, err := adapter.HandleSpawnProcess(context.Background(), args)

	// Should return ErrAgentTypeNotFound
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentTypeNotFound)
	assert.Nil(t, result)

	// Verify no command was sent
	cmds := handler.getCommands()
	assert.Len(t, cmds, 0)
}

func TestHandleSpawnProcess_NoAgentType_UsesGeneric(t *testing.T) {
	adapter, handler, cleanup := testAdapter(t)
	defer cleanup()

	handler.returnResult = &command.CommandResult{
		Success: true,
		Data:    "worker-generic",
	}

	// No agent_type in args (nil args)
	result, err := adapter.HandleSpawnProcess(context.Background(), nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	// Verify command was created with generic (empty string) AgentType
	cmds := handler.getCommands()
	require.Len(t, cmds, 1)
	spawnCmd, ok := cmds[0].(*command.SpawnProcessCommand)
	require.True(t, ok)
	assert.Equal(t, "", string(spawnCmd.AgentType)) // AgentTypeGeneric is empty string
}

func TestHandleSpawnProcess_EmptyAgentType_UsesGeneric(t *testing.T) {
	adapter, handler, cleanup := testAdapter(t)
	defer cleanup()

	handler.returnResult = &command.CommandResult{
		Success: true,
		Data:    "worker-generic",
	}

	// Empty string agent_type
	args := toJSON(t, map[string]string{
		"agent_type": "",
	})

	result, err := adapter.HandleSpawnProcess(context.Background(), args)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	// Verify command was created with generic AgentType
	cmds := handler.getCommands()
	require.Len(t, cmds, 1)
	spawnCmd, ok := cmds[0].(*command.SpawnProcessCommand)
	require.True(t, ok)
	assert.Equal(t, "", string(spawnCmd.AgentType))
}

func TestHandleSpawnProcess_ValidatesEnumValues(t *testing.T) {
	tests := []struct {
		name       string
		agentType  string
		shouldFail bool
	}{
		{"implementer_valid", "implementer", false},
		{"reviewer_valid", "reviewer", false},
		{"researcher_valid", "researcher", false},
		{"empty_valid", "", false},
		{"unknown_invalid", "unknown", true},
		{"uppercase_invalid", "IMPLEMENTER", true},
		{"shell_injection_invalid", "implementer;rm -rf /", true},
		{"path_traversal_invalid", "../../../etc/passwd", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adapter, handler, cleanup := testAdapter(t)
			defer cleanup()

			handler.returnResult = &command.CommandResult{
				Success: true,
				Data:    "worker-test",
			}

			args := toJSON(t, map[string]string{
				"agent_type": tc.agentType,
			})

			result, err := adapter.HandleSpawnProcess(context.Background(), args)

			if tc.shouldFail {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrAgentTypeNotFound)
				assert.Nil(t, result)
				// No command should be sent for invalid types
				cmds := handler.getCommands()
				assert.Len(t, cmds, 0)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.False(t, result.IsError)
			}
		})
	}
}

func TestHandleQueryWorkerState_IncludesAgentType(t *testing.T) {
	t.Run("returns_agent_type_for_specialized_worker", func(t *testing.T) {
		processRepo := repository.NewMemoryProcessRepository()

		_ = processRepo.Save(&repository.Process{
			ID:        "worker-1",
			Role:      repository.RoleWorker,
			Status:    repository.StatusReady,
			Phase:     ptr(events.ProcessPhaseIdle),
			AgentType: roles.AgentTypeImplementer,
			CreatedAt: time.Now(),
		})

		adapter, _, cleanup := testAdapter(t,
			WithProcessRepository(processRepo),
		)
		defer cleanup()

		result, err := adapter.HandleQueryWorkerState(context.Background(), nil)

		require.NoError(t, err)
		require.NotNil(t, result)

		// Parse response
		var response struct {
			Workers []map[string]any `json:"workers"`
		}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Len(t, response.Workers, 1)
		w := response.Workers[0]
		assert.Equal(t, "implementer", w["agent_type"])
	})

	t.Run("returns_generic_for_unspecialized_worker", func(t *testing.T) {
		processRepo := repository.NewMemoryProcessRepository()

		// AgentType empty string (generic)
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-1",
			Role:      repository.RoleWorker,
			Status:    repository.StatusReady,
			Phase:     ptr(events.ProcessPhaseIdle),
			AgentType: roles.AgentTypeGeneric, // Generic
			CreatedAt: time.Now(),
		})

		adapter, _, cleanup := testAdapter(t,
			WithProcessRepository(processRepo),
		)
		defer cleanup()

		result, err := adapter.HandleQueryWorkerState(context.Background(), nil)

		require.NoError(t, err)
		require.NotNil(t, result)

		var response struct {
			Workers []map[string]any `json:"workers"`
		}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Len(t, response.Workers, 1)
		w := response.Workers[0]
		// AgentType.String() returns "generic" for empty string
		assert.Equal(t, "generic", w["agent_type"])
	})

	t.Run("returns_all_agent_types_correctly", func(t *testing.T) {
		processRepo := repository.NewMemoryProcessRepository()

		_ = processRepo.Save(&repository.Process{
			ID:        "worker-1",
			Role:      repository.RoleWorker,
			Status:    repository.StatusReady,
			Phase:     ptr(events.ProcessPhaseIdle),
			AgentType: roles.AgentTypeImplementer,
			CreatedAt: time.Now(),
		})
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-2",
			Role:      repository.RoleWorker,
			Status:    repository.StatusReady,
			Phase:     ptr(events.ProcessPhaseIdle),
			AgentType: roles.AgentTypeReviewer,
			CreatedAt: time.Now(),
		})
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-3",
			Role:      repository.RoleWorker,
			Status:    repository.StatusReady,
			Phase:     ptr(events.ProcessPhaseIdle),
			AgentType: roles.AgentTypeResearcher,
			CreatedAt: time.Now(),
		})
		_ = processRepo.Save(&repository.Process{
			ID:        "worker-4",
			Role:      repository.RoleWorker,
			Status:    repository.StatusReady,
			Phase:     ptr(events.ProcessPhaseIdle),
			AgentType: roles.AgentTypeGeneric, // generic
			CreatedAt: time.Now(),
		})

		adapter, _, cleanup := testAdapter(t,
			WithProcessRepository(processRepo),
		)
		defer cleanup()

		result, err := adapter.HandleQueryWorkerState(context.Background(), nil)

		require.NoError(t, err)
		require.NotNil(t, result)

		var response struct {
			Workers []map[string]any `json:"workers"`
		}
		err = json.Unmarshal([]byte(result.Content[0].Text), &response)
		require.NoError(t, err)

		require.Len(t, response.Workers, 4)

		// Build map of worker_id -> agent_type for easier assertions
		agentTypes := make(map[string]string)
		for _, w := range response.Workers {
			agentTypes[w["worker_id"].(string)] = w["agent_type"].(string)
		}

		assert.Equal(t, "implementer", agentTypes["worker-1"])
		assert.Equal(t, "reviewer", agentTypes["worker-2"])
		assert.Equal(t, "researcher", agentTypes["worker-3"])
		assert.Equal(t, "generic", agentTypes["worker-4"])
	})
}

// ===========================================================================
// Session Directory Tests (perles-x4f4.7)
// ===========================================================================

func TestWithSessionID_SetsAllThreeFields(t *testing.T) {
	// Create a processor with a mock handler
	proc := processor.NewCommandProcessor()
	mockHandler := newMockHandler()
	proc.RegisterHandler(command.CmdGenerateAccountabilitySummary, mockHandler)

	adapter := NewV2Adapter(proc,
		WithSessionID("test-session-123", "/work/dir", "/home/user/.perles/sessions/myapp/2026-01-11/test-session-123"),
	)

	// Verify all three fields are set correctly
	assert.Equal(t, "test-session-123", adapter.sessionID)
	assert.Equal(t, "/work/dir", adapter.workDir)
	assert.Equal(t, "/home/user/.perles/sessions/myapp/2026-01-11/test-session-123", adapter.sessionDir)
}

func TestAdapter_StoresSessionDirectory(t *testing.T) {
	proc := processor.NewCommandProcessor()

	// Test with centralized storage path
	adapter := NewV2Adapter(proc,
		WithSessionID("session-abc", "/project/root", "/Users/test/.perles/sessions/myapp/2026-01-11/session-abc"),
	)

	assert.Equal(t, "session-abc", adapter.sessionID)
	assert.Equal(t, "/project/root", adapter.workDir)
	assert.Equal(t, "/Users/test/.perles/sessions/myapp/2026-01-11/session-abc", adapter.sessionDir)
}

func TestHandleGenerateAccountabilitySummary_UsesStoredSessionDir(t *testing.T) {
	t.Run("uses stored sessionDir directly", func(t *testing.T) {
		proc := processor.NewCommandProcessor()

		// Create a handler that captures the command to verify session dir
		var capturedCmd command.Command
		handler := &mockHandler{
			returnResult: &command.CommandResult{Success: true},
		}
		proc.RegisterHandler(command.CmdGenerateAccountabilitySummary, handler)

		// Start processor
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go proc.Run(ctx)
		require.NoError(t, proc.WaitForReady(ctx))

		expectedSessionDir := "/home/user/.perles/sessions/myapp/2026-01-11/test-session"
		adapter := NewV2Adapter(proc,
			WithSessionID("test-session", "/work/dir", expectedSessionDir),
		)

		args := json.RawMessage(`{"worker_id": "worker-1"}`)
		_, err := adapter.HandleGenerateAccountabilitySummary(context.Background(), args)
		require.NoError(t, err)

		// Wait for handler to process command
		time.Sleep(50 * time.Millisecond)

		// Verify the command was captured
		handler.mu.Lock()
		if len(handler.commands) > 0 {
			capturedCmd = handler.commands[0]
		}
		handler.mu.Unlock()

		require.NotNil(t, capturedCmd)
		summaryCmd, ok := capturedCmd.(*command.GenerateAccountabilitySummaryCommand)
		require.True(t, ok, "expected GenerateAccountabilitySummaryCommand")
		assert.Equal(t, expectedSessionDir, summaryCmd.SessionDir)
	})

	t.Run("returns error when sessionDir not configured", func(t *testing.T) {
		proc := processor.NewCommandProcessor()
		// No sessionDir configured
		adapter := NewV2Adapter(proc)

		args := json.RawMessage(`{"worker_id": "worker-1"}`)
		_, err := adapter.HandleGenerateAccountabilitySummary(context.Background(), args)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "session directory not configured")
	})

	t.Run("no path reconstruction from workDir", func(t *testing.T) {
		// This test verifies that the old path reconstruction pattern
		// (workDir/.perles/sessions/sessionID) is NOT used
		proc := processor.NewCommandProcessor()

		handler := &mockHandler{
			returnResult: &command.CommandResult{Success: true},
		}
		proc.RegisterHandler(command.CmdGenerateAccountabilitySummary, handler)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go proc.Run(ctx)
		require.NoError(t, proc.WaitForReady(ctx))

		// Set a sessionDir that's different from what path reconstruction would produce
		// If the code was still reconstructing, it would use "/work/.perles/sessions/sess-123"
		centralizedSessionDir := "/home/user/.perles/sessions/myproject/2026-01-11/sess-123"
		adapter := NewV2Adapter(proc,
			WithSessionID("sess-123", "/work", centralizedSessionDir),
		)

		args := json.RawMessage(`{"worker_id": "worker-1"}`)
		_, err := adapter.HandleGenerateAccountabilitySummary(context.Background(), args)
		require.NoError(t, err)

		// Wait for handler
		time.Sleep(50 * time.Millisecond)

		handler.mu.Lock()
		var capturedCmd command.Command
		if len(handler.commands) > 0 {
			capturedCmd = handler.commands[0]
		}
		handler.mu.Unlock()

		require.NotNil(t, capturedCmd)
		summaryCmd, ok := capturedCmd.(*command.GenerateAccountabilitySummaryCommand)
		require.True(t, ok)

		// Should use the stored sessionDir, NOT reconstruct from workDir
		assert.Equal(t, centralizedSessionDir, summaryCmd.SessionDir)
		assert.NotEqual(t, "/work/.perles/sessions/sess-123", summaryCmd.SessionDir)
	})
}

// ===========================================================================
// Workflow Lifecycle Tests
// ===========================================================================

func TestHandleSignalWorkflowComplete(t *testing.T) {
	t.Run("success_with_required_fields_only", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]any{
			"status":  "success",
			"summary": "All tasks completed successfully",
		})

		result, err := adapter.HandleSignalWorkflowComplete(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "Workflow marked as success")

		// Verify command was created correctly
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		workflowCmd, ok := cmds[0].(*command.SignalWorkflowCompleteCommand)
		require.True(t, ok)
		assert.Equal(t, command.WorkflowStatusSuccess, workflowCmd.Status)
		assert.Equal(t, "All tasks completed successfully", workflowCmd.Summary)
		assert.Empty(t, workflowCmd.EpicID)
		assert.Equal(t, 0, workflowCmd.TasksClosed)
	})

	t.Run("success_with_all_fields", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]any{
			"status":       "partial",
			"summary":      "Completed 3 of 5 tasks",
			"epic_id":      "epic-123",
			"tasks_closed": 3,
		})

		result, err := adapter.HandleSignalWorkflowComplete(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "Workflow marked as partial")
		assert.Contains(t, result.Content[0].Text, "epic: epic-123")
		assert.Contains(t, result.Content[0].Text, "3 tasks closed")

		// Verify command was created correctly
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		workflowCmd, ok := cmds[0].(*command.SignalWorkflowCompleteCommand)
		require.True(t, ok)
		assert.Equal(t, command.WorkflowStatusPartial, workflowCmd.Status)
		assert.Equal(t, "Completed 3 of 5 tasks", workflowCmd.Summary)
		assert.Equal(t, "epic-123", workflowCmd.EpicID)
		assert.Equal(t, 3, workflowCmd.TasksClosed)
	})

	t.Run("success_aborted_status", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]any{
			"status":  "aborted",
			"summary": "Workflow aborted due to blocking issue",
		})

		result, err := adapter.HandleSignalWorkflowComplete(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "Workflow marked as aborted")

		// Verify command
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		workflowCmd, ok := cmds[0].(*command.SignalWorkflowCompleteCommand)
		require.True(t, ok)
		assert.Equal(t, command.WorkflowStatusAborted, workflowCmd.Status)
	})

	t.Run("invalid_json", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		result, err := adapter.HandleSignalWorkflowComplete(context.Background(), []byte("invalid"))

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid arguments")
	})

	t.Run("missing_status", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]any{
			"summary": "All tasks completed",
		})

		result, err := adapter.HandleSignalWorkflowComplete(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "status is required")
	})

	t.Run("missing_summary", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]any{
			"status": "success",
		})

		result, err := adapter.HandleSignalWorkflowComplete(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "summary is required")
	})

	t.Run("invalid_status", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]any{
			"status":  "invalid_status",
			"summary": "Some summary",
		})

		result, err := adapter.HandleSignalWorkflowComplete(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "signal_workflow_complete command validation failed")
		assert.Contains(t, err.Error(), "status must be success, partial, or aborted")
	})

	t.Run("optional_epic_id_only", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]any{
			"status":  "success",
			"summary": "Epic completed",
			"epic_id": "epic-456",
		})

		result, err := adapter.HandleSignalWorkflowComplete(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "epic: epic-456")
		assert.NotContains(t, result.Content[0].Text, "tasks closed")

		// Verify command
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		workflowCmd, ok := cmds[0].(*command.SignalWorkflowCompleteCommand)
		require.True(t, ok)
		assert.Equal(t, "epic-456", workflowCmd.EpicID)
		assert.Equal(t, 0, workflowCmd.TasksClosed)
	})

	t.Run("optional_tasks_closed_only", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]any{
			"status":       "success",
			"summary":      "Tasks completed",
			"tasks_closed": 5,
		})

		result, err := adapter.HandleSignalWorkflowComplete(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "5 tasks closed")
		assert.NotContains(t, result.Content[0].Text, "epic:")

		// Verify command
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		workflowCmd, ok := cmds[0].(*command.SignalWorkflowCompleteCommand)
		require.True(t, ok)
		assert.Empty(t, workflowCmd.EpicID)
		assert.Equal(t, 5, workflowCmd.TasksClosed)
	})

	t.Run("handler_error", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		handler.returnErr = errors.New("handler failed")

		args := toJSON(t, map[string]any{
			"status":  "success",
			"summary": "Some summary",
		})

		result, err := adapter.HandleSignalWorkflowComplete(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "handler failed")
	})

	t.Run("result_not_success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		handler.returnResult = &command.CommandResult{
			Success: false,
			Error:   errors.New("workflow completion failed"),
		}

		args := toJSON(t, map[string]any{
			"status":  "success",
			"summary": "Some summary",
		})

		result, err := adapter.HandleSignalWorkflowComplete(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "workflow completion failed")
	})
}

// ===========================================================================
// HandleNotifyUser Tests
// ===========================================================================

func TestHandleNotifyUser(t *testing.T) {
	t.Run("success_with_message_only", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]any{
			"message": "Human review required",
		})

		result, err := adapter.HandleNotifyUser(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "User has been notified")

		// Verify command was created correctly
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		notifyCmd, ok := cmds[0].(*command.NotifyUserCommand)
		require.True(t, ok)
		assert.Equal(t, "Human review required", notifyCmd.Message)
		assert.Empty(t, notifyCmd.Phase)
		assert.Empty(t, notifyCmd.TaskID)
	})

	t.Run("success_with_all_fields", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]any{
			"message": "Please review the research findings",
			"phase":   "clarification-review",
			"task_id": "perles-abc.1",
		})

		result, err := adapter.HandleNotifyUser(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.False(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "User notified for phase: clarification-review")

		// Verify command was created correctly
		cmds := handler.getCommands()
		require.Len(t, cmds, 1)
		notifyCmd, ok := cmds[0].(*command.NotifyUserCommand)
		require.True(t, ok)
		assert.Equal(t, "Please review the research findings", notifyCmd.Message)
		assert.Equal(t, "clarification-review", notifyCmd.Phase)
		assert.Equal(t, "perles-abc.1", notifyCmd.TaskID)
	})

	t.Run("invalid_json", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		result, err := adapter.HandleNotifyUser(context.Background(), []byte("invalid"))

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "invalid arguments")
	})

	t.Run("missing_message", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]any{
			"phase": "some-phase",
		})

		result, err := adapter.HandleNotifyUser(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "message is required")
	})

	t.Run("empty_message", func(t *testing.T) {
		adapter, _, cleanup := testAdapter(t)
		defer cleanup()

		args := toJSON(t, map[string]any{
			"message": "",
			"phase":   "some-phase",
		})

		result, err := adapter.HandleNotifyUser(context.Background(), args)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "message is required")
	})

	t.Run("handler_error", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		handler.returnErr = errors.New("handler failed")

		args := toJSON(t, map[string]any{
			"message": "Test message",
		})

		result, err := adapter.HandleNotifyUser(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "handler failed")
	})

	t.Run("result_not_success", func(t *testing.T) {
		adapter, handler, cleanup := testAdapter(t)
		defer cleanup()

		handler.returnResult = &command.CommandResult{
			Success: false,
			Error:   errors.New("notification failed"),
		}

		args := toJSON(t, map[string]any{
			"message": "Test message",
		})

		result, err := adapter.HandleNotifyUser(context.Background(), args)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, result.IsError)
		assert.Contains(t, result.Content[0].Text, "notification failed")
	})
}
