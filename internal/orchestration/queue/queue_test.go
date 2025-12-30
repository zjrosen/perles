package queue

import (
	"sync"
	"testing"
	"time"
)

func TestQueue_NewMessageQueue(t *testing.T) {
	tests := []struct {
		name            string
		maxSize         int
		expectedMaxSize int
	}{
		{
			name:            "positive max size",
			maxSize:         50,
			expectedMaxSize: 50,
		},
		{
			name:            "zero uses default",
			maxSize:         0,
			expectedMaxSize: DefaultMaxSize,
		},
		{
			name:            "negative uses default",
			maxSize:         -10,
			expectedMaxSize: DefaultMaxSize,
		},
		{
			name:            "default max size constant",
			maxSize:         DefaultMaxSize,
			expectedMaxSize: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewMessageQueue(tt.maxSize)
			if q == nil {
				t.Fatal("NewMessageQueue returned nil")
			}
			if q.maxSize != tt.expectedMaxSize {
				t.Errorf("expected maxSize %d, got %d", tt.expectedMaxSize, q.maxSize)
			}
			if q.Len() != 0 {
				t.Errorf("new queue should be empty, got len %d", q.Len())
			}
		})
	}
}

func TestQueue_FIFO(t *testing.T) {
	q := NewMessageQueue(10)

	// Enqueue messages in order
	messages := []QueuedMessage{
		{ID: "1", Content: "first", From: "COORDINATOR"},
		{ID: "2", Content: "second", From: "USER"},
		{ID: "3", Content: "third", From: "WORKER.1"},
	}

	for _, msg := range messages {
		if err := q.Enqueue(msg); err != nil {
			t.Fatalf("Enqueue failed: %v", err)
		}
	}

	if q.Len() != 3 {
		t.Errorf("expected len 3, got %d", q.Len())
	}

	// Dequeue and verify FIFO order
	for i, expected := range messages {
		msg, ok := q.Dequeue()
		if !ok {
			t.Fatalf("Dequeue %d returned not ok", i)
		}
		if msg.ID != expected.ID {
			t.Errorf("Dequeue %d: expected ID %s, got %s", i, expected.ID, msg.ID)
		}
		if msg.Content != expected.Content {
			t.Errorf("Dequeue %d: expected Content %s, got %s", i, expected.Content, msg.Content)
		}
		if msg.From != expected.From {
			t.Errorf("Dequeue %d: expected From %s, got %s", i, expected.From, msg.From)
		}
	}

	if q.Len() != 0 {
		t.Errorf("queue should be empty after dequeuing all, got len %d", q.Len())
	}
}

func TestQueue_MaxSize(t *testing.T) {
	maxSize := 3
	q := NewMessageQueue(maxSize)

	// Fill the queue to capacity
	for i := 0; i < maxSize; i++ {
		msg := QueuedMessage{ID: string(rune('a' + i)), Content: "msg"}
		if err := q.Enqueue(msg); err != nil {
			t.Fatalf("Enqueue %d failed unexpectedly: %v", i, err)
		}
	}

	if q.Len() != maxSize {
		t.Errorf("expected len %d, got %d", maxSize, q.Len())
	}

	// Attempt to enqueue when full - should return ErrQueueFull
	msg := QueuedMessage{ID: "overflow", Content: "should fail"}
	err := q.Enqueue(msg)
	if err != ErrQueueFull {
		t.Errorf("expected ErrQueueFull, got %v", err)
	}

	// Queue length should remain at max
	if q.Len() != maxSize {
		t.Errorf("queue len should still be %d after failed enqueue, got %d", maxSize, q.Len())
	}

	// Dequeue one and try again - should succeed
	_, ok := q.Dequeue()
	if !ok {
		t.Fatal("Dequeue should succeed")
	}

	err = q.Enqueue(msg)
	if err != nil {
		t.Errorf("Enqueue after dequeue should succeed, got %v", err)
	}
}

func TestQueue_EmptyDequeue(t *testing.T) {
	q := NewMessageQueue(10)

	// Dequeue from empty queue
	msg, ok := q.Dequeue()
	if ok {
		t.Error("Dequeue from empty queue should return false")
	}
	if msg.ID != "" || msg.Content != "" || msg.From != "" {
		t.Error("Dequeue from empty queue should return zero value")
	}

	// Add and remove a message, then dequeue again
	q.Enqueue(QueuedMessage{ID: "temp", Content: "temp"})
	q.Dequeue()

	msg, ok = q.Dequeue()
	if ok {
		t.Error("Dequeue from emptied queue should return false")
	}
	if msg.ID != "" {
		t.Error("Dequeue from emptied queue should return zero value")
	}
}

func TestQueue_Peek(t *testing.T) {
	q := NewMessageQueue(10)

	// Peek empty queue
	msg, ok := q.Peek()
	if ok {
		t.Error("Peek on empty queue should return false")
	}
	if msg.ID != "" {
		t.Error("Peek on empty queue should return zero value")
	}

	// Add messages
	first := QueuedMessage{ID: "first", Content: "first content", From: "COORDINATOR"}
	second := QueuedMessage{ID: "second", Content: "second content", From: "USER"}

	q.Enqueue(first)
	q.Enqueue(second)

	// Peek should return first without removing
	msg, ok = q.Peek()
	if !ok {
		t.Error("Peek should return true when queue has items")
	}
	if msg.ID != first.ID {
		t.Errorf("Peek should return first item, got ID %s", msg.ID)
	}
	if q.Len() != 2 {
		t.Error("Peek should not remove items from queue")
	}

	// Peek again - should return same item
	msg2, ok := q.Peek()
	if !ok || msg2.ID != first.ID {
		t.Error("Multiple peeks should return same item")
	}

	// Dequeue and peek should return second item
	q.Dequeue()
	msg, ok = q.Peek()
	if !ok || msg.ID != second.ID {
		t.Errorf("After dequeue, peek should return second item, got %s", msg.ID)
	}
}

func TestQueue_Drain(t *testing.T) {
	q := NewMessageQueue(10)

	// Drain empty queue
	result := q.Drain()
	if len(result) != 0 {
		t.Error("Drain on empty queue should return empty slice")
	}
	if q.Len() != 0 {
		t.Error("Queue should be empty after drain")
	}

	// Add messages
	messages := []QueuedMessage{
		{ID: "1", Content: "first"},
		{ID: "2", Content: "second"},
		{ID: "3", Content: "third"},
	}
	for _, msg := range messages {
		q.Enqueue(msg)
	}

	// Drain should return all messages
	result = q.Drain()
	if len(result) != len(messages) {
		t.Errorf("Drain should return %d messages, got %d", len(messages), len(result))
	}

	// Verify order preserved
	for i, msg := range result {
		if msg.ID != messages[i].ID {
			t.Errorf("Drain[%d]: expected ID %s, got %s", i, messages[i].ID, msg.ID)
		}
	}

	// Queue should be empty after drain
	if q.Len() != 0 {
		t.Errorf("Queue should be empty after drain, got len %d", q.Len())
	}

	// Drain again should return empty slice
	result = q.Drain()
	if len(result) != 0 {
		t.Error("Second drain should return empty slice")
	}
}

func TestQueue_Concurrent(t *testing.T) {
	q := NewMessageQueue(1000)

	const numGoroutines = 10
	const numOpsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2) // Half enqueue, half dequeue

	// Start enqueue goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				msg := QueuedMessage{
					ID:         string(rune('0' + id)),
					Content:    "test",
					EnqueuedAt: time.Now(),
				}
				_ = q.Enqueue(msg) // Ignore full errors
			}
		}(i)
	}

	// Start dequeue goroutines
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				_, _ = q.Dequeue()
			}
		}()
	}

	wg.Wait()

	// Queue should be in valid state - length should be non-negative
	if q.Len() < 0 {
		t.Errorf("Queue length should be non-negative, got %d", q.Len())
	}
}

func TestQueue_ConcurrentEnqueueDequeue(t *testing.T) {
	q := NewMessageQueue(50)

	const iterations = 1000

	var wg sync.WaitGroup
	wg.Add(4)

	// Rapid enqueue
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			msg := QueuedMessage{ID: "enq1", Content: "from goroutine 1"}
			_ = q.Enqueue(msg)
		}
	}()

	// Rapid enqueue
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			msg := QueuedMessage{ID: "enq2", Content: "from goroutine 2"}
			_ = q.Enqueue(msg)
		}
	}()

	// Rapid dequeue
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_, _ = q.Dequeue()
		}
	}()

	// Rapid Len/Peek interleaved
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = q.Len()
			_, _ = q.Peek()
		}
	}()

	wg.Wait()

	// Drain remaining and verify valid state
	remaining := q.Drain()
	for _, msg := range remaining {
		if msg.ID != "enq1" && msg.ID != "enq2" {
			t.Errorf("Unexpected message ID in queue: %s", msg.ID)
		}
	}
}

func TestQueue_QueuedMessageFields(t *testing.T) {
	q := NewMessageQueue(10)

	now := time.Now()
	msg := QueuedMessage{
		ID:         "test-id-123",
		Content:    "Hello, worker!",
		From:       "COORDINATOR",
		EnqueuedAt: now,
		Priority:   1,
	}

	err := q.Enqueue(msg)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	dequeued, ok := q.Dequeue()
	if !ok {
		t.Fatal("Dequeue failed")
	}

	// Verify all fields are preserved
	if dequeued.ID != msg.ID {
		t.Errorf("ID mismatch: expected %s, got %s", msg.ID, dequeued.ID)
	}
	if dequeued.Content != msg.Content {
		t.Errorf("Content mismatch: expected %s, got %s", msg.Content, dequeued.Content)
	}
	if dequeued.From != msg.From {
		t.Errorf("From mismatch: expected %s, got %s", msg.From, dequeued.From)
	}
	if !dequeued.EnqueuedAt.Equal(msg.EnqueuedAt) {
		t.Errorf("EnqueuedAt mismatch: expected %v, got %v", msg.EnqueuedAt, dequeued.EnqueuedAt)
	}
	if dequeued.Priority != msg.Priority {
		t.Errorf("Priority mismatch: expected %d, got %d", msg.Priority, dequeued.Priority)
	}
}

func TestQueue_DrainReturnsNewSlice(t *testing.T) {
	q := NewMessageQueue(10)

	q.Enqueue(QueuedMessage{ID: "1"})
	q.Enqueue(QueuedMessage{ID: "2"})

	drained := q.Drain()

	// Modify drained slice
	drained[0].ID = "modified"

	// Enqueue new items
	q.Enqueue(QueuedMessage{ID: "3"})

	// New items should not be affected by modification
	msg, _ := q.Dequeue()
	if msg.ID != "3" {
		t.Errorf("Queue internal state was corrupted by drain result modification")
	}
}
