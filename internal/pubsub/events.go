// Package pubsub provides a generic publish/subscribe event system.
package pubsub

import (
	"context"
	"time"
)

// EventType represents the type of event being published.
type EventType string

const (
	CreatedEvent EventType = "created"
	UpdatedEvent EventType = "updated"
	DeletedEvent EventType = "deleted"
)

// Event represents a published event with a typed payload.
type Event[T any] struct {
	Type      EventType
	Payload   T
	Timestamp time.Time
}

// Subscriber provides a subscription channel for events.
type Subscriber[T any] interface {
	Subscribe(ctx context.Context) <-chan Event[T]
}

// Publisher allows publishing events with a typed payload.
type Publisher[T any] interface {
	Publish(eventType EventType, payload T)
}
