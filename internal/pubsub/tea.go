package pubsub

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// ListenCmd creates a Bubble Tea command that listens for events on a channel.
// Returns the event as a tea.Msg when received.
// Returns nil if the context is cancelled or the channel is closed.
func ListenCmd[T any](ctx context.Context, ch <-chan Event[T]) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-ch:
			if !ok {
				return nil // Channel closed
			}
			return event
		}
	}
}

// ContinuousListener maintains subscription state for the Bubble Tea update loop.
// It wraps a broker subscription and provides a Listen method that returns
// a tea.Cmd for receiving events.
type ContinuousListener[T any] struct {
	ctx context.Context
	ch  <-chan Event[T]
}

// NewContinuousListener creates a new listener that subscribes to the broker.
// The subscription is automatically cleaned up when the context is cancelled.
func NewContinuousListener[T any](ctx context.Context, broker *Broker[T]) *ContinuousListener[T] {
	return &ContinuousListener[T]{
		ctx: ctx,
		ch:  broker.Subscribe(ctx),
	}
}

// Listen returns a tea.Cmd that waits for the next event.
// Call this method in your Update function after handling an event
// to continue receiving events.
func (l *ContinuousListener[T]) Listen() tea.Cmd {
	return ListenCmd(l.ctx, l.ch)
}
