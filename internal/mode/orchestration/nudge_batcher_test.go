package orchestration

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNudgeBatcher_SingleMessage(t *testing.T) {
	var received map[MessageType][]string
	var mu sync.Mutex
	done := make(chan struct{})

	batcher := NewNudgeBatcher(50 * time.Millisecond)
	batcher.SetOnNudge(func(msgs map[MessageType][]string) {
		mu.Lock()
		received = msgs
		mu.Unlock()
		close(done)
	})

	batcher.Add("WORKER.1", WorkerNewMessage)

	// Should not fire immediately
	mu.Lock()
	require.Empty(t, received)
	mu.Unlock()

	// Wait for debounce
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for nudge")
	}

	mu.Lock()
	assert.Equal(t, map[MessageType][]string{WorkerNewMessage: {"WORKER.1"}}, received)
	mu.Unlock()
}

func TestNudgeBatcher_MultipleMessagesBatched(t *testing.T) {
	var received map[MessageType][]string
	var mu sync.Mutex
	done := make(chan struct{})

	batcher := NewNudgeBatcher(50 * time.Millisecond)
	batcher.SetOnNudge(func(msgs map[MessageType][]string) {
		mu.Lock()
		received = msgs
		mu.Unlock()
		close(done)
	})

	batcher.Add("WORKER.1", WorkerNewMessage)
	batcher.Add("WORKER.2", WorkerReady)
	batcher.Add("WORKER.3", WorkerNewMessage)

	// Wait for debounce
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for nudge")
	}

	mu.Lock()
	defer mu.Unlock()

	// Check we have both message types
	assert.Len(t, received, 2)

	// Check WorkerNewMessage group
	assert.Len(t, received[WorkerNewMessage], 2)
	assert.Contains(t, received[WorkerNewMessage], "WORKER.1")
	assert.Contains(t, received[WorkerNewMessage], "WORKER.3")

	// Check WorkerReady group
	assert.Equal(t, []string{"WORKER.2"}, received[WorkerReady])
}

func TestNudgeBatcher_SlidingWindow(t *testing.T) {
	var callCount int
	var mu sync.Mutex
	done := make(chan struct{})

	batcher := NewNudgeBatcher(50 * time.Millisecond)
	batcher.SetOnNudge(func(msgs map[MessageType][]string) {
		mu.Lock()
		callCount++
		mu.Unlock()
		close(done)
	})

	batcher.Add("WORKER.1", WorkerNewMessage)
	time.Sleep(30 * time.Millisecond)
	batcher.Add("WORKER.2", WorkerNewMessage) // Resets timer
	time.Sleep(30 * time.Millisecond)
	batcher.Add("WORKER.3", WorkerNewMessage) // Resets timer again

	// Wait for debounce
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for nudge")
	}

	// Give a little extra time to ensure no second call
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	// Only one callback despite 3 adds
	assert.Equal(t, 1, callCount)
	mu.Unlock()
}

func TestNudgeBatcher_DuplicateSenders(t *testing.T) {
	var received map[MessageType][]string
	var mu sync.Mutex
	done := make(chan struct{})

	batcher := NewNudgeBatcher(50 * time.Millisecond)
	batcher.SetOnNudge(func(msgs map[MessageType][]string) {
		mu.Lock()
		received = msgs
		mu.Unlock()
		close(done)
	})

	batcher.Add("WORKER.1", WorkerNewMessage)
	batcher.Add("WORKER.1", WorkerNewMessage) // Duplicate
	batcher.Add("WORKER.1", WorkerNewMessage) // Duplicate

	// Wait for debounce
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for nudge")
	}

	mu.Lock()
	// Should dedupe to single entry
	assert.Equal(t, map[MessageType][]string{WorkerNewMessage: {"WORKER.1"}}, received)
	mu.Unlock()
}

func TestNudgeBatcher_StopCancelsPending(t *testing.T) {
	var called bool
	var mu sync.Mutex

	batcher := NewNudgeBatcher(100 * time.Millisecond)
	batcher.SetOnNudge(func(msgs map[MessageType][]string) {
		mu.Lock()
		called = true
		mu.Unlock()
	})

	batcher.Add("WORKER.1", WorkerNewMessage)

	// Stop before debounce fires
	time.Sleep(30 * time.Millisecond)
	batcher.Stop()

	// Wait past debounce window
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	assert.False(t, called, "callback should not have been called after Stop()")
	mu.Unlock()
}

func TestNudgeBatcher_NoCallbackIfNoMessages(t *testing.T) {
	var called bool
	var mu sync.Mutex

	batcher := NewNudgeBatcher(50 * time.Millisecond)
	batcher.SetOnNudge(func(msgs map[MessageType][]string) {
		mu.Lock()
		called = true
		mu.Unlock()
	})

	// Don't add any messages

	// Wait past debounce window
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.False(t, called, "callback should not have been called without messages")
	mu.Unlock()
}

func TestNudgeBatcher_SetOnNudge_NilSafe(t *testing.T) {
	// Test that flush doesn't panic if no callback is set
	batcher := NewNudgeBatcher(10 * time.Millisecond)
	// Don't set onNudge callback

	batcher.Add("WORKER.1", WorkerNewMessage)

	// Wait for flush - should not panic
	time.Sleep(50 * time.Millisecond)
}

func TestNudgeBatcher_ConcurrentAdds(t *testing.T) {
	var received map[MessageType][]string
	var mu sync.Mutex
	done := make(chan struct{})

	batcher := NewNudgeBatcher(100 * time.Millisecond)
	batcher.SetOnNudge(func(msgs map[MessageType][]string) {
		mu.Lock()
		received = msgs
		mu.Unlock()
		close(done)
	})

	// Concurrently add from multiple goroutines
	var wg sync.WaitGroup
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			batcher.Add("WORKER."+string(rune('0'+id)), WorkerNewMessage)
		}(i)
	}
	wg.Wait()

	// Wait for debounce
	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timeout waiting for nudge")
	}

	mu.Lock()
	// All 5 workers should be in the WorkerNewMessage group
	assert.Len(t, received[WorkerNewMessage], 5)
	mu.Unlock()
}

func TestNudgeBatcher_MessageTypesTracked(t *testing.T) {
	var received map[MessageType][]string
	var mu sync.Mutex
	done := make(chan struct{})

	batcher := NewNudgeBatcher(50 * time.Millisecond)
	batcher.SetOnNudge(func(msgs map[MessageType][]string) {
		mu.Lock()
		received = msgs
		mu.Unlock()
		close(done)
	})

	// Add messages with different types
	batcher.Add("WORKER.1", WorkerReady)
	batcher.Add("WORKER.2", WorkerNewMessage)

	// Wait for debounce
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for nudge")
	}

	mu.Lock()
	defer mu.Unlock()

	// Verify we have both message types with correct workers
	assert.Len(t, received, 2)
	assert.Equal(t, []string{"WORKER.1"}, received[WorkerReady])
	assert.Equal(t, []string{"WORKER.2"}, received[WorkerNewMessage])
}

func TestNudgeBatcher_LastTypeWins(t *testing.T) {
	var received map[MessageType][]string
	var mu sync.Mutex
	done := make(chan struct{})

	batcher := NewNudgeBatcher(50 * time.Millisecond)
	batcher.SetOnNudge(func(msgs map[MessageType][]string) {
		mu.Lock()
		received = msgs
		mu.Unlock()
		close(done)
	})

	// Same worker sends different types - last one should win
	batcher.Add("WORKER.1", WorkerReady)
	batcher.Add("WORKER.1", WorkerNewMessage)

	// Wait for debounce
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for nudge")
	}

	mu.Lock()
	defer mu.Unlock()

	// Should have only one group with one worker (last type wins)
	assert.Len(t, received, 1)
	assert.Equal(t, []string{"WORKER.1"}, received[WorkerNewMessage])
	assert.Nil(t, received[WorkerReady])
}
