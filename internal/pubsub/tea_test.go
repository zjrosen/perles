package pubsub

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListenCmd_ReceivesEvent(t *testing.T) {
	broker := NewBroker[string]()
	defer broker.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := broker.Subscribe(ctx)

	// Publish an event
	broker.Publish(UpdatedEvent, "hello world")

	// Create the command and execute it
	cmd := ListenCmd(ctx, ch)
	msg := cmd()

	// Should receive the event as tea.Msg
	event, ok := msg.(Event[string])
	require.True(t, ok, "msg should be Event[string]")
	require.Equal(t, "hello world", event.Payload)
	require.Equal(t, UpdatedEvent, event.Type)
}

func TestListenCmd_ContextCancelled(t *testing.T) {
	broker := NewBroker[string]()
	defer broker.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ch := broker.Subscribe(ctx)

	// Cancel context before executing command
	cancel()
	time.Sleep(20 * time.Millisecond) // Wait for cleanup

	// Execute command - should return nil due to cancelled context
	cmd := ListenCmd(ctx, ch)
	msg := cmd()

	require.Nil(t, msg, "should return nil when context cancelled")
}

func TestListenCmd_ChannelClosed(t *testing.T) {
	// Create a channel and close it immediately
	ch := make(chan Event[string])
	close(ch)

	ctx := context.Background()

	// Execute command - should return nil due to closed channel
	cmd := ListenCmd(ctx, ch)
	msg := cmd()

	require.Nil(t, msg, "should return nil when channel closed")
}

func TestContinuousListener_Listen(t *testing.T) {
	broker := NewBroker[int]()
	defer broker.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener := NewContinuousListener(ctx, broker)

	// Publish multiple events
	broker.Publish(CreatedEvent, 1)
	broker.Publish(UpdatedEvent, 2)
	broker.Publish(DeletedEvent, 3)

	// First Listen() call should return cmd that receives first event
	cmd := listener.Listen()
	msg := cmd()

	event, ok := msg.(Event[int])
	require.True(t, ok, "msg should be Event[int]")
	require.Equal(t, 1, event.Payload)
	require.Equal(t, CreatedEvent, event.Type)

	// Second Listen() call should return cmd that receives second event
	cmd = listener.Listen()
	msg = cmd()

	event, ok = msg.(Event[int])
	require.True(t, ok, "msg should be Event[int]")
	require.Equal(t, 2, event.Payload)
	require.Equal(t, UpdatedEvent, event.Type)

	// Third Listen() call should return cmd that receives third event
	cmd = listener.Listen()
	msg = cmd()

	event, ok = msg.(Event[int])
	require.True(t, ok, "msg should be Event[int]")
	require.Equal(t, 3, event.Payload)
	require.Equal(t, DeletedEvent, event.Type)
}
