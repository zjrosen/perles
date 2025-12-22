package orchestration

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNudgeBatcher_SingleMessage(t *testing.T) {
	var received []string
	var mu sync.Mutex
	done := make(chan struct{})

	batcher := NewNudgeBatcher(50 * time.Millisecond)
	batcher.SetOnNudge(func(ids []string) {
		mu.Lock()
		received = ids
		mu.Unlock()
		close(done)
	})

	batcher.Add("WORKER.1")

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
	assert.Equal(t, []string{"WORKER.1"}, received)
	mu.Unlock()
}

func TestNudgeBatcher_MultipleMessagesBatched(t *testing.T) {
	var received []string
	var mu sync.Mutex
	done := make(chan struct{})

	batcher := NewNudgeBatcher(50 * time.Millisecond)
	batcher.SetOnNudge(func(ids []string) {
		mu.Lock()
		received = ids
		mu.Unlock()
		close(done)
	})

	batcher.Add("WORKER.1")
	batcher.Add("WORKER.2")
	batcher.Add("WORKER.3")

	// Wait for debounce
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for nudge")
	}

	mu.Lock()
	// All three in single batch
	assert.Len(t, received, 3)
	assert.Contains(t, received, "WORKER.1")
	assert.Contains(t, received, "WORKER.2")
	assert.Contains(t, received, "WORKER.3")
	mu.Unlock()
}

func TestNudgeBatcher_SlidingWindow(t *testing.T) {
	var callCount int
	var mu sync.Mutex
	done := make(chan struct{})

	batcher := NewNudgeBatcher(50 * time.Millisecond)
	batcher.SetOnNudge(func(ids []string) {
		mu.Lock()
		callCount++
		mu.Unlock()
		close(done)
	})

	batcher.Add("WORKER.1")
	time.Sleep(30 * time.Millisecond)
	batcher.Add("WORKER.2") // Resets timer
	time.Sleep(30 * time.Millisecond)
	batcher.Add("WORKER.3") // Resets timer again

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
	var received []string
	var mu sync.Mutex
	done := make(chan struct{})

	batcher := NewNudgeBatcher(50 * time.Millisecond)
	batcher.SetOnNudge(func(ids []string) {
		mu.Lock()
		received = ids
		mu.Unlock()
		close(done)
	})

	batcher.Add("WORKER.1")
	batcher.Add("WORKER.1") // Duplicate
	batcher.Add("WORKER.1") // Duplicate

	// Wait for debounce
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for nudge")
	}

	mu.Lock()
	// Should dedupe to single entry
	assert.Equal(t, []string{"WORKER.1"}, received)
	mu.Unlock()
}

func TestNudgeBatcher_StopCancelsPending(t *testing.T) {
	var called bool
	var mu sync.Mutex

	batcher := NewNudgeBatcher(100 * time.Millisecond)
	batcher.SetOnNudge(func(ids []string) {
		mu.Lock()
		called = true
		mu.Unlock()
	})

	batcher.Add("WORKER.1")

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
	batcher.SetOnNudge(func(ids []string) {
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

	batcher.Add("WORKER.1")

	// Wait for flush - should not panic
	time.Sleep(50 * time.Millisecond)
}

func TestNudgeBatcher_ConcurrentAdds(t *testing.T) {
	var received []string
	var mu sync.Mutex
	done := make(chan struct{})

	batcher := NewNudgeBatcher(100 * time.Millisecond)
	batcher.SetOnNudge(func(ids []string) {
		mu.Lock()
		received = ids
		mu.Unlock()
		close(done)
	})

	// Concurrently add from multiple goroutines
	var wg sync.WaitGroup
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			batcher.Add("WORKER." + string(rune('0'+id)))
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
	assert.Len(t, received, 5)
	mu.Unlock()
}
