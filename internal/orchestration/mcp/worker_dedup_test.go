package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWorkerServer_PostMessage_Deduplication verifies that duplicate messages
// from the same worker are deduplicated, resulting in only one actual Append call.
func TestWorkerServer_PostMessage_Deduplication(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)

	ctx := context.Background()
	messageArgs := `{"to": "COORDINATOR", "content": "test message content"}`

	// First send should succeed and trigger actual append
	result1, err := ws.handlePostMessage(ctx, json.RawMessage(messageArgs))
	require.NoError(t, err)
	require.NotNil(t, result1)
	require.True(t, strings.Contains(result1.Content[0].Text, "Message sent"),
		"First send should succeed with 'Message sent' response")

	// Immediate duplicate should also return success (idempotent)
	result2, err := ws.handlePostMessage(ctx, json.RawMessage(messageArgs))
	require.NoError(t, err)
	require.NotNil(t, result2)
	require.True(t, strings.Contains(result2.Content[0].Text, "Message sent"),
		"Duplicate send should still return success (idempotent)")

	// CRITICAL: Verify Append was only called once
	require.Len(t, store.appendCalls, 1,
		"Append should only be called once despite duplicate post_message calls")
}

// TestWorkerServer_PostMessage_DifferentMessages verifies that different messages
// from the same worker are NOT deduplicated.
func TestWorkerServer_PostMessage_DifferentMessages(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)

	ctx := context.Background()

	// Send first message
	args1 := `{"to": "COORDINATOR", "content": "first message"}`
	result1, err := ws.handlePostMessage(ctx, json.RawMessage(args1))
	require.NoError(t, err)
	require.NotNil(t, result1)

	// Send different message - should NOT be deduplicated
	args2 := `{"to": "COORDINATOR", "content": "second message"}`
	result2, err := ws.handlePostMessage(ctx, json.RawMessage(args2))
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Both messages should trigger appends (different content = different hash)
	require.Len(t, store.appendCalls, 2,
		"Different messages should both trigger Append calls")

	// Verify both messages were stored
	require.Equal(t, "first message", store.appendCalls[0].Content)
	require.Equal(t, "second message", store.appendCalls[1].Content)
}

// TestWorkerServer_PostMessage_DifferentRecipients verifies that the same message
// to different recipients IS deduplicated (dedup is based on worker+content, not recipient).
func TestWorkerServer_PostMessage_DifferentRecipients(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)

	ctx := context.Background()
	sameContent := "identical message content"

	// Send message to COORDINATOR
	args1 := `{"to": "COORDINATOR", "content": "` + sameContent + `"}`
	result1, err := ws.handlePostMessage(ctx, json.RawMessage(args1))
	require.NoError(t, err)
	require.NotNil(t, result1)

	// Send same message to ALL - should be deduplicated since dedup is worker+content based
	args2 := `{"to": "ALL", "content": "` + sameContent + `"}`
	result2, err := ws.handlePostMessage(ctx, json.RawMessage(args2))
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Only the first should trigger append (same worker + same content = duplicate)
	require.Len(t, store.appendCalls, 1,
		"Same message to different recipients should be deduplicated (based on worker+content)")
}

// TestWorkerServer_PostMessage_DifferentWorkers verifies that the same message
// from different workers is NOT deduplicated.
func TestWorkerServer_PostMessage_DifferentWorkers(t *testing.T) {
	store := newMockMessageStore()

	// Create two different worker servers with the same store
	ws1 := NewWorkerServer("WORKER.1", store)
	ws2 := NewWorkerServer("WORKER.2", store)

	ctx := context.Background()
	sameArgs := `{"to": "COORDINATOR", "content": "identical message"}`

	// Send from worker 1
	result1, err := ws1.handlePostMessage(ctx, json.RawMessage(sameArgs))
	require.NoError(t, err)
	require.NotNil(t, result1)

	// Send same message from worker 2 - should NOT be deduplicated
	result2, err := ws2.handlePostMessage(ctx, json.RawMessage(sameArgs))
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Both should trigger appends (different worker = different hash key)
	require.Len(t, store.appendCalls, 2,
		"Same message from different workers should both trigger Append calls")

	// Verify both workers sent their messages
	require.Equal(t, "WORKER.1", store.appendCalls[0].From)
	require.Equal(t, "WORKER.2", store.appendCalls[1].From)
}

// TestWorkerServer_PostMessage_Deduplication_Concurrent verifies that concurrent
// duplicate sends still only result in one actual append.
func TestWorkerServer_PostMessage_Deduplication_Concurrent(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)

	ctx := context.Background()
	messageArgs := `{"to": "COORDINATOR", "content": "concurrent test message"}`

	// Launch multiple concurrent sends of the same message
	const numGoroutines = 10
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ws.handlePostMessage(ctx, json.RawMessage(messageArgs))
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		require.NoError(t, err, "Unexpected error from concurrent send")
	}

	// CRITICAL: Despite 10 concurrent sends of identical message,
	// only ONE should have triggered an actual Append
	require.Len(t, store.appendCalls, 1,
		"Concurrent duplicate sends should only result in one Append call")
}

// TestWorkerServer_SignalReady_NotDeduplicated verifies that signal_ready
// calls are NOT deduplicated (ready signals should always go through).
func TestWorkerServer_SignalReady_NotDeduplicated(t *testing.T) {
	store := newMockMessageStore()
	ws := NewWorkerServer("WORKER.1", store)

	ctx := context.Background()

	// Send first ready signal
	result1, err := ws.handleSignalReady(ctx, json.RawMessage(`{}`))
	require.NoError(t, err)
	require.NotNil(t, result1)

	// Send ready signal again - should NOT be deduplicated
	result2, err := ws.handleSignalReady(ctx, json.RawMessage(`{}`))
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Both signals should go through (ready signals are not deduplicated)
	require.Len(t, store.appendCalls, 2,
		"Ready signals should NOT be deduplicated - both should trigger Append")
}
