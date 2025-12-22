package pubsub

import (
	"context"
	"sync"
	"time"
)

const defaultBufferSize = 64

// Broker is a generic pub/sub event broker.
// It allows multiple subscribers to receive events published by publishers.
type Broker[T any] struct {
	subs       map[chan Event[T]]struct{}
	mu         sync.RWMutex
	done       chan struct{}
	bufferSize int
}

// NewBroker creates a new broker with the default buffer size (64).
func NewBroker[T any]() *Broker[T] {
	return NewBrokerWithBuffer[T](defaultBufferSize)
}

// NewBrokerWithBuffer creates a new broker with a custom buffer size.
func NewBrokerWithBuffer[T any](size int) *Broker[T] {
	return &Broker[T]{
		subs:       make(map[chan Event[T]]struct{}),
		done:       make(chan struct{}),
		bufferSize: size,
	}
}

// Subscribe creates a new subscription channel.
// The channel is automatically closed when ctx is cancelled.
func (b *Broker[T]) Subscribe(ctx context.Context) <-chan Event[T] {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if broker is closed
	select {
	case <-b.done:
		ch := make(chan Event[T])
		close(ch)
		return ch
	default:
	}

	sub := make(chan Event[T], b.bufferSize)
	b.subs[sub] = struct{}{}

	// Cleanup goroutine
	go func() {
		<-ctx.Done()
		b.mu.Lock()
		defer b.mu.Unlock()

		select {
		case <-b.done:
			return // Already closed
		default:
		}

		delete(b.subs, sub)
		close(sub)
	}()

	return sub
}

// Publish sends an event to all subscribers.
// Non-blocking: drops events if subscriber channel is full.
func (b *Broker[T]) Publish(eventType EventType, payload T) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	select {
	case <-b.done:
		return
	default:
	}

	event := Event[T]{
		Type:      eventType,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	for sub := range b.subs {
		select {
		case sub <- event:
			// Delivered
		default:
			// Channel full - drop to prevent blocking
		}
	}
}

// Close shuts down the broker and all subscriber channels.
func (b *Broker[T]) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	select {
	case <-b.done:
		return // Already closed
	default:
	}

	close(b.done)
	for sub := range b.subs {
		close(sub)
	}
	b.subs = nil
}

// SubscriberCount returns the number of active subscribers.
func (b *Broker[T]) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}
