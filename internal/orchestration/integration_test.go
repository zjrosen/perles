package orchestration_test

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/pubsub"

	"github.com/stretchr/testify/require"
)

// TestIntegration_CoordinatorEventFlow verifies that coordinator events
// flow through the broker to subscribers correctly.
func TestIntegration_CoordinatorEventFlow(t *testing.T) {
	broker := pubsub.NewBroker[events.CoordinatorEvent]()
	defer broker.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to events
	ch := broker.Subscribe(ctx)

	// Simulate coordinator publishing events
	testMetrics := &metrics.TokenMetrics{ContextTokens: 1500, ContextWindow: 200000}
	testEvents := []events.CoordinatorEvent{
		{Type: events.CoordinatorChat, Role: "coordinator", Content: "Starting epic execution"},
		{Type: events.CoordinatorWorking},
		{Type: events.CoordinatorTokenUsage, Metrics: testMetrics},
		{Type: events.CoordinatorReady},
	}

	// Publish events
	for _, evt := range testEvents {
		broker.Publish(pubsub.UpdatedEvent, evt)
	}

	// Verify all events received in order
	for i, expected := range testEvents {
		select {
		case received := <-ch:
			require.Equal(t, expected.Type, received.Payload.Type, "event %d type mismatch", i)
			require.Equal(t, expected.Role, received.Payload.Role, "event %d role mismatch", i)
			require.Equal(t, expected.Content, received.Payload.Content, "event %d content mismatch", i)
			require.Equal(t, expected.Metrics, received.Payload.Metrics, "event %d metrics mismatch", i)
			require.Equal(t, pubsub.UpdatedEvent, received.Type, "wrapper event type should be UpdatedEvent")
			require.False(t, received.Timestamp.IsZero(), "timestamp should be set")
		case <-time.After(100 * time.Millisecond):
			require.Fail(t, "timeout waiting for event", "event %d", i)
		}
	}
}

// TestIntegration_MultipleSubscribers verifies that multiple subscribers
// all receive the same events from a single publish.
func TestIntegration_MultipleSubscribers(t *testing.T) {
	broker := pubsub.NewBroker[events.CoordinatorEvent]()
	defer broker.Close()

	ctx := context.Background()

	// Create 5 subscribers (simulates TUI, logger, metrics collector, etc.)
	const numSubscribers = 5
	channels := make([]<-chan pubsub.Event[events.CoordinatorEvent], numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		channels[i] = broker.Subscribe(ctx)
	}

	require.Equal(t, numSubscribers, broker.SubscriberCount())

	// Publish a chat event
	testEvent := events.CoordinatorEvent{
		Type:    events.CoordinatorChat,
		Role:    "coordinator",
		Content: "Hello from coordinator",
	}
	broker.Publish(pubsub.UpdatedEvent, testEvent)

	// Verify all subscribers received the same event
	for i, ch := range channels {
		select {
		case received := <-ch:
			require.Equal(t, testEvent.Type, received.Payload.Type, "subscriber %d: type mismatch", i)
			require.Equal(t, testEvent.Role, received.Payload.Role, "subscriber %d: role mismatch", i)
			require.Equal(t, testEvent.Content, received.Payload.Content, "subscriber %d: content mismatch", i)
		case <-time.After(100 * time.Millisecond):
			require.Fail(t, "timeout waiting for event", "subscriber %d", i)
		}
	}
}

// TestIntegration_WorkerEventForwarding verifies that worker events
// from the pool are forwarded through the worker broker to TUI subscribers.
func TestIntegration_WorkerEventForwarding(t *testing.T) {
	// Create worker broker (simulates coordinator.Workers())
	workerBroker := pubsub.NewBroker[events.WorkerEvent]()
	defer workerBroker.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to worker events (simulates TUI subscription)
	ch := workerBroker.Subscribe(ctx)

	// Simulate pool events being converted and forwarded by coordinator
	workerMetrics := &metrics.TokenMetrics{ContextTokens: 500, ContextWindow: 200000}
	testEvents := []events.WorkerEvent{
		{Type: events.WorkerSpawned, WorkerID: "worker-1", TaskID: "task-1"},
		{Type: events.WorkerStatusChange, WorkerID: "worker-1", TaskID: "task-1"},
		{Type: events.WorkerOutput, WorkerID: "worker-1", Output: "Starting task execution"},
		{Type: events.WorkerTokenUsage, WorkerID: "worker-1", Metrics: workerMetrics},
		{Type: events.WorkerIncoming, WorkerID: "worker-1", Message: "Message from coordinator"},
	}

	// Publish events (simulates processPoolEvents forwarding)
	for _, evt := range testEvents {
		workerBroker.Publish(pubsub.UpdatedEvent, evt)
	}

	// Verify all events received
	for i, expected := range testEvents {
		select {
		case received := <-ch:
			require.Equal(t, expected.Type, received.Payload.Type, "event %d type mismatch", i)
			require.Equal(t, expected.WorkerID, received.Payload.WorkerID, "event %d workerID mismatch", i)
			require.Equal(t, expected.TaskID, received.Payload.TaskID, "event %d taskID mismatch", i)
			require.Equal(t, expected.Output, received.Payload.Output, "event %d output mismatch", i)
			require.Equal(t, expected.Message, received.Payload.Message, "event %d message mismatch", i)
		case <-time.After(100 * time.Millisecond):
			require.Fail(t, "timeout waiting for event", "event %d", i)
		}
	}
}

// TestIntegration_CleanShutdown verifies that closing brokers properly
// cleans up all subscriber goroutines without leaks.
func TestIntegration_CleanShutdown(t *testing.T) {
	// Record initial goroutine count
	initialGoroutines := runtime.NumGoroutine()

	// Create brokers and subscribers
	coordBroker := pubsub.NewBroker[events.CoordinatorEvent]()
	workerBroker := pubsub.NewBroker[events.WorkerEvent]()

	// Create multiple subscriptions with different contexts
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())

	ch1 := coordBroker.Subscribe(ctx1)
	ch2 := coordBroker.Subscribe(ctx2)
	ch3 := workerBroker.Subscribe(ctx1)
	ch4 := workerBroker.Subscribe(ctx2)

	require.Equal(t, 2, coordBroker.SubscriberCount())
	require.Equal(t, 2, workerBroker.SubscriberCount())

	// Publish some events to ensure goroutines are active
	coordBroker.Publish(pubsub.UpdatedEvent, events.CoordinatorEvent{Type: events.CoordinatorChat})
	workerBroker.Publish(pubsub.UpdatedEvent, events.WorkerEvent{Type: events.WorkerOutput})

	// Drain channels
	<-ch1
	<-ch2
	<-ch3
	<-ch4

	// Cancel contexts to trigger subscription cleanup
	cancel1()
	cancel2()

	// Wait for cleanup goroutines
	time.Sleep(50 * time.Millisecond)

	// Close brokers
	coordBroker.Close()
	workerBroker.Close()

	// Verify all channels are closed
	_, ok1 := <-ch1
	_, ok2 := <-ch2
	_, ok3 := <-ch3
	_, ok4 := <-ch4

	require.False(t, ok1, "ch1 should be closed")
	require.False(t, ok2, "ch2 should be closed")
	require.False(t, ok3, "ch3 should be closed")
	require.False(t, ok4, "ch4 should be closed")

	// Verify subscriber counts are zero
	require.Equal(t, 0, coordBroker.SubscriberCount())
	require.Equal(t, 0, workerBroker.SubscriberCount())

	// Allow time for goroutines to fully exit
	time.Sleep(50 * time.Millisecond)

	// Check for goroutine leaks (allow some tolerance for test framework goroutines)
	finalGoroutines := runtime.NumGoroutine()
	goroutineDiff := finalGoroutines - initialGoroutines
	require.LessOrEqual(t, goroutineDiff, 2, "possible goroutine leak: started with %d, ended with %d", initialGoroutines, finalGoroutines)
}

// TestIntegration_ConcurrentPublishSubscribe verifies thread safety
// under concurrent publish and subscribe operations.
func TestIntegration_ConcurrentPublishSubscribe(t *testing.T) {
	broker := pubsub.NewBroker[events.CoordinatorEvent]()
	defer broker.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		numPublishers  = 5
		numSubscribers = 5
		numEvents      = 100
	)

	var wg sync.WaitGroup

	// Start subscribers
	received := make([]int, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		i := i
		ch := broker.Subscribe(ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
					received[i]++
				case <-time.After(200 * time.Millisecond):
					return
				}
			}
		}()
	}

	// Start publishers concurrently
	for p := 0; p < numPublishers; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := 0; e < numEvents; e++ {
				broker.Publish(pubsub.UpdatedEvent, events.CoordinatorEvent{
					Type:    events.CoordinatorChat,
					Content: "test",
				})
			}
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify each subscriber received events (may drop some due to buffer limits)
	totalPublished := numPublishers * numEvents
	for i, count := range received {
		require.Greater(t, count, 0, "subscriber %d should have received at least some events", i)
		t.Logf("Subscriber %d received %d/%d events (%.1f%%)", i, count, totalPublished, float64(count)/float64(totalPublished)*100)
	}
}

// TestIntegration_ContextCancellationCleanup verifies that context cancellation
// properly removes subscribers without affecting other subscribers.
func TestIntegration_ContextCancellationCleanup(t *testing.T) {
	broker := pubsub.NewBroker[events.CoordinatorEvent]()
	defer broker.Close()

	// Create independent contexts
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2 := context.Background() // Never cancelled

	ch1 := broker.Subscribe(ctx1)
	ch2 := broker.Subscribe(ctx2)

	require.Equal(t, 2, broker.SubscriberCount())

	// Cancel first subscriber's context
	cancel1()
	time.Sleep(30 * time.Millisecond) // Wait for cleanup

	require.Equal(t, 1, broker.SubscriberCount())

	// ch1 should be closed
	_, ok := <-ch1
	require.False(t, ok, "ch1 should be closed after context cancellation")

	// ch2 should still receive events
	broker.Publish(pubsub.UpdatedEvent, events.CoordinatorEvent{Type: events.CoordinatorReady})

	select {
	case event := <-ch2:
		require.Equal(t, events.CoordinatorReady, event.Payload.Type)
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "ch2 should still receive events")
	}
}
