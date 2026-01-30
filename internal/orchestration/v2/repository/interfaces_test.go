package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===========================================================================
// TaskAssignment Entity Tests
// ===========================================================================

func TestTaskAssignment_FieldAccess(t *testing.T) {
	startedAt := time.Now()
	reviewStartedAt := startedAt.Add(30 * time.Minute)

	task := &TaskAssignment{
		TaskID:          "perles-abc.1",
		Implementer:     "worker-1",
		Reviewer:        "worker-2",
		Status:          TaskInReview,
		StartedAt:       startedAt,
		ReviewStartedAt: reviewStartedAt,
	}

	assert.Equal(t, "perles-abc.1", task.TaskID)
	assert.Equal(t, "worker-1", task.Implementer)
	assert.Equal(t, "worker-2", task.Reviewer)
	assert.Equal(t, TaskInReview, task.Status)
	assert.Equal(t, startedAt, task.StartedAt)
	assert.Equal(t, reviewStartedAt, task.ReviewStartedAt)
}

func TestTaskAssignment_StatusValues(t *testing.T) {
	tests := []struct {
		name   string
		status TaskStatus
		want   string
	}{
		{"Implementing", TaskImplementing, "implementing"},
		{"InReview", TaskInReview, "in_review"},
		{"Approved", TaskApproved, "approved"},
		{"Denied", TaskDenied, "denied"},
		{"Committing", TaskCommitting, "committing"},
		{"Completed", TaskCompleted, "completed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &TaskAssignment{Status: tt.status}
			assert.Equal(t, tt.status, task.Status)
			assert.Equal(t, tt.want, string(task.Status))
		})
	}
}

// ===========================================================================
// QueueEntry Tests
// ===========================================================================

func TestQueueEntry_FieldAccess(t *testing.T) {
	ts := time.Now()
	entry := QueueEntry{
		Content:   "Hello, worker!",
		Timestamp: ts,
	}

	assert.Equal(t, "Hello, worker!", entry.Content)
	assert.Equal(t, ts, entry.Timestamp)
}

func TestQueueEntry_TimestampSetOnCreation(t *testing.T) {
	// When creating a QueueEntry via Enqueue, timestamp should be set
	q := NewMessageQueue("worker-1", 10)

	before := time.Now()
	err := q.Enqueue("test message", SenderUser)
	after := time.Now()

	require.NoError(t, err)

	entry, ok := q.Dequeue()
	require.True(t, ok)

	// Timestamp should be between before and after
	assert.True(t, !entry.Timestamp.Before(before), "timestamp should be >= before")
	assert.True(t, !entry.Timestamp.After(after), "timestamp should be <= after")
}

// ===========================================================================
// MessageQueue Entity Tests
// ===========================================================================

func TestMessageQueue_NewMessageQueue(t *testing.T) {
	q := NewMessageQueue("worker-1", 100)

	assert.Equal(t, "worker-1", q.WorkerID)
	assert.Equal(t, 0, q.Size())
	assert.Equal(t, 100, q.MaxSize())
	assert.True(t, q.IsEmpty())
}

func TestMessageQueue_Enqueue_AddsEntry(t *testing.T) {
	q := NewMessageQueue("worker-1", 10)

	err := q.Enqueue("first message", SenderUser)
	require.NoError(t, err)
	assert.Equal(t, 1, q.Size())
	assert.False(t, q.IsEmpty())

	err = q.Enqueue("second message", SenderUser)
	require.NoError(t, err)
	assert.Equal(t, 2, q.Size())
}

func TestMessageQueue_Dequeue_ReturnsFIFOOrder(t *testing.T) {
	q := NewMessageQueue("worker-1", 10)

	// Enqueue multiple messages
	require.NoError(t, q.Enqueue("first", SenderUser))
	require.NoError(t, q.Enqueue("second", SenderUser))
	require.NoError(t, q.Enqueue("third", SenderUser))

	// Dequeue should return in FIFO order
	entry, ok := q.Dequeue()
	require.True(t, ok)
	assert.Equal(t, "first", entry.Content)

	entry, ok = q.Dequeue()
	require.True(t, ok)
	assert.Equal(t, "second", entry.Content)

	entry, ok = q.Dequeue()
	require.True(t, ok)
	assert.Equal(t, "third", entry.Content)

	// Queue should now be empty
	entry, ok = q.Dequeue()
	assert.False(t, ok)
	assert.Nil(t, entry)
}

func TestMessageQueue_Dequeue_EmptyQueue(t *testing.T) {
	q := NewMessageQueue("worker-1", 10)

	entry, ok := q.Dequeue()
	assert.False(t, ok)
	assert.Nil(t, entry)
}

func TestMessageQueue_Drain_ReturnsAllAndEmpties(t *testing.T) {
	q := NewMessageQueue("worker-1", 10)

	// Enqueue multiple messages
	require.NoError(t, q.Enqueue("first", SenderUser))
	require.NoError(t, q.Enqueue("second", SenderUser))
	require.NoError(t, q.Enqueue("third", SenderUser))
	assert.Equal(t, 3, q.Size())

	// Drain should return all entries
	entries := q.Drain()
	assert.Len(t, entries, 3)
	assert.Equal(t, "first", entries[0].Content)
	assert.Equal(t, "second", entries[1].Content)
	assert.Equal(t, "third", entries[2].Content)

	// Queue should now be empty
	assert.Equal(t, 0, q.Size())
	assert.True(t, q.IsEmpty())

	// Drain again should return empty slice
	entries = q.Drain()
	assert.Len(t, entries, 0)
}

func TestMessageQueue_Drain_EmptyQueue(t *testing.T) {
	q := NewMessageQueue("worker-1", 10)

	entries := q.Drain()
	assert.Len(t, entries, 0)
	assert.NotNil(t, entries) // Should return empty slice, not nil
}

func TestMessageQueue_RespectsMaxSize(t *testing.T) {
	q := NewMessageQueue("worker-1", 3)

	// Fill the queue to capacity
	require.NoError(t, q.Enqueue("first", SenderUser))
	require.NoError(t, q.Enqueue("second", SenderUser))
	require.NoError(t, q.Enqueue("third", SenderUser))
	assert.Equal(t, 3, q.Size())

	// Next enqueue should fail with ErrQueueFull
	err := q.Enqueue("fourth", SenderUser)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrQueueFull))

	// Size should still be 3
	assert.Equal(t, 3, q.Size())
}

func TestMessageQueue_UnlimitedCapacity(t *testing.T) {
	// maxSize of 0 means unlimited
	q := NewMessageQueue("worker-1", 0)

	// Should be able to enqueue many messages
	for i := 0; i < 1000; i++ {
		err := q.Enqueue("message", SenderUser)
		require.NoError(t, err)
	}

	assert.Equal(t, 1000, q.Size())
}

func TestMessageQueue_Size(t *testing.T) {
	q := NewMessageQueue("worker-1", 10)

	assert.Equal(t, 0, q.Size())

	require.NoError(t, q.Enqueue("one", SenderUser))
	assert.Equal(t, 1, q.Size())

	require.NoError(t, q.Enqueue("two", SenderUser))
	assert.Equal(t, 2, q.Size())

	q.Dequeue()
	assert.Equal(t, 1, q.Size())

	q.Drain()
	assert.Equal(t, 0, q.Size())
}

func TestMessageQueue_IsEmpty(t *testing.T) {
	q := NewMessageQueue("worker-1", 10)

	assert.True(t, q.IsEmpty())

	require.NoError(t, q.Enqueue("message", SenderUser))
	assert.False(t, q.IsEmpty())

	q.Dequeue()
	assert.True(t, q.IsEmpty())
}

// ===========================================================================
// Error Sentinel Value Tests
// ===========================================================================

func TestErrorSentinels_HaveCorrectMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"ErrTaskNotFound", ErrTaskNotFound, "task not found"},
		{"ErrQueueFull", ErrQueueFull, "message queue is full"},
		{"ErrProcessNotFound", ErrProcessNotFound, "process not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Error())
		})
	}
}

func TestErrorSentinels_ErrorsIs(t *testing.T) {
	// Verify errors.Is works correctly for sentinel errors
	assert.True(t, errors.Is(ErrTaskNotFound, ErrTaskNotFound))
	assert.True(t, errors.Is(ErrQueueFull, ErrQueueFull))
	assert.True(t, errors.Is(ErrProcessNotFound, ErrProcessNotFound))

	// Verify they are distinct
	assert.False(t, errors.Is(ErrProcessNotFound, ErrTaskNotFound))
	assert.False(t, errors.Is(ErrTaskNotFound, ErrQueueFull))
	assert.False(t, errors.Is(ErrQueueFull, ErrProcessNotFound))
}

// ===========================================================================
// ProcessStatus Tests
// ===========================================================================

func TestProcessStatus_Values(t *testing.T) {
	tests := []struct {
		name   string
		status ProcessStatus
		want   string
	}{
		{"Pending", StatusPending, "pending"},
		{"Starting", StatusStarting, "starting"},
		{"Ready", StatusReady, "ready"},
		{"Working", StatusWorking, "working"},
		{"Paused", StatusPaused, "paused"},
		{"Retired", StatusRetired, "retired"},
		{"Failed", StatusFailed, "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, string(tt.status))
		})
	}
}

func TestProcessStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		name       string
		status     ProcessStatus
		isTerminal bool
	}{
		{"Pending is not terminal", StatusPending, false},
		{"Starting is not terminal", StatusStarting, false},
		{"Ready is not terminal", StatusReady, false},
		{"Working is not terminal", StatusWorking, false},
		{"Paused is not terminal", StatusPaused, false},
		{"Retired is terminal", StatusRetired, true},
		{"Failed is terminal", StatusFailed, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isTerminal, tt.status.IsTerminal())
		})
	}
}

func TestProcessStatus_StatusPaused_StringRepresentation(t *testing.T) {
	// Verify StatusPaused renders correctly as string
	status := StatusPaused
	assert.Equal(t, "paused", string(status))

	// Verify it can be compared correctly
	assert.Equal(t, ProcessStatus("paused"), status)
}

func TestProcessStatus_StatusPaused_IsValidValue(t *testing.T) {
	// Verify StatusPaused is a distinct, valid value
	validStatuses := []ProcessStatus{
		StatusPending,
		StatusStarting,
		StatusReady,
		StatusWorking,
		StatusPaused,
		StatusRetired,
		StatusFailed,
	}

	// Count occurrences of StatusPaused
	count := 0
	for _, s := range validStatuses {
		if s == StatusPaused {
			count++
		}
	}
	assert.Equal(t, 1, count, "StatusPaused should appear exactly once in valid statuses")

	// Verify StatusPaused is distinct from all other statuses
	assert.NotEqual(t, StatusPaused, StatusPending)
	assert.NotEqual(t, StatusPaused, StatusStarting)
	assert.NotEqual(t, StatusPaused, StatusReady)
	assert.NotEqual(t, StatusPaused, StatusWorking)
	assert.NotEqual(t, StatusPaused, StatusRetired)
	assert.NotEqual(t, StatusPaused, StatusFailed)
}

// ===========================================================================
// ObserverID Constant Tests
// ===========================================================================

func TestObserverID_Constant(t *testing.T) {
	// Verify ObserverID constant equals "observer"
	assert.Equal(t, "observer", ObserverID)
}

func TestRoleObserver_Alias(t *testing.T) {
	// Verify RoleObserver alias works correctly
	assert.Equal(t, "observer", string(RoleObserver))
}
