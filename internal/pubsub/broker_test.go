package pubsub

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBroker_Subscribe(t *testing.T) {
	broker := NewBroker[string]()
	defer broker.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := broker.Subscribe(ctx)

	broker.Publish(UpdatedEvent, "hello")

	select {
	case event := <-ch:
		require.Equal(t, "hello", event.Payload)
		require.Equal(t, UpdatedEvent, event.Type)
		require.False(t, event.Timestamp.IsZero())
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "timeout waiting for event")
	}
}

func TestBroker_MultipleSubscribers(t *testing.T) {
	broker := NewBroker[int]()
	defer broker.Close()

	ctx := context.Background()

	ch1 := broker.Subscribe(ctx)
	ch2 := broker.Subscribe(ctx)
	ch3 := broker.Subscribe(ctx)

	require.Equal(t, 3, broker.SubscriberCount())

	broker.Publish(CreatedEvent, 42)

	// All subscribers should receive the event
	for i, ch := range []<-chan Event[int]{ch1, ch2, ch3} {
		select {
		case event := <-ch:
			require.Equal(t, 42, event.Payload, "subscriber %d", i)
			require.Equal(t, CreatedEvent, event.Type, "subscriber %d", i)
		case <-time.After(100 * time.Millisecond):
			require.Fail(t, "timeout waiting for event", "subscriber %d", i)
		}
	}
}

func TestBroker_ContextCancellation(t *testing.T) {
	broker := NewBroker[string]()
	defer broker.Close()

	ctx, cancel := context.WithCancel(context.Background())

	ch := broker.Subscribe(ctx)
	require.Equal(t, 1, broker.SubscriberCount())

	cancel()
	time.Sleep(20 * time.Millisecond) // Wait for cleanup goroutine

	require.Equal(t, 0, broker.SubscriberCount())

	// Channel should be closed
	_, ok := <-ch
	require.False(t, ok, "channel should be closed")
}

func TestBroker_NonBlocking(t *testing.T) {
	broker := NewBrokerWithBuffer[int](1)
	defer broker.Close()

	ctx := context.Background()

	ch := broker.Subscribe(ctx)

	// Fill buffer
	broker.Publish(UpdatedEvent, 1)

	// These should not block (drop events)
	done := make(chan bool)
	go func() {
		broker.Publish(UpdatedEvent, 2)
		broker.Publish(UpdatedEvent, 3)
		done <- true
	}()

	select {
	case <-done:
		// Success - didn't block
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "Publish blocked")
	}

	// Only first event received (buffer was full for others)
	event := <-ch
	require.Equal(t, 1, event.Payload)
}

func TestBroker_Close(t *testing.T) {
	broker := NewBroker[string]()

	ctx := context.Background()

	ch1 := broker.Subscribe(ctx)
	ch2 := broker.Subscribe(ctx)

	require.Equal(t, 2, broker.SubscriberCount())

	broker.Close()

	// Both channels should be closed
	_, ok1 := <-ch1
	_, ok2 := <-ch2

	require.False(t, ok1, "ch1 should be closed")
	require.False(t, ok2, "ch2 should be closed")

	// Subscriber count should be 0
	require.Equal(t, 0, broker.SubscriberCount())

	// Subscribe after close should return closed channel
	ch3 := broker.Subscribe(ctx)
	_, ok3 := <-ch3
	require.False(t, ok3, "ch3 should be closed immediately")

	// Publish after close should not panic
	broker.Publish(UpdatedEvent, "test") // No panic
}

func TestBroker_CloseIdempotent(t *testing.T) {
	broker := NewBroker[string]()

	ctx := context.Background()
	ch := broker.Subscribe(ctx)

	// Multiple Close() calls should be safe
	broker.Close()
	broker.Close()
	broker.Close()

	// Channel should still be closed
	_, ok := <-ch
	require.False(t, ok, "channel should be closed")
}
