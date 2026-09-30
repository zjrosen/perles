package session

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/client/providers/amp"
	"github.com/zjrosen/perles/internal/orchestration/client/providers/claude"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/pubsub"
)

// TestCostFlowEndToEnd_Claude verifies that cost data flows correctly from raw Claude JSON
// through the parser → process event → session → metadata.json pipeline.
// This test uses real JSON from the Claude provider testdata, not synthetic events.
//
// Key verification points:
// 1. Cost from result events is extracted (TotalCostUSD field)
// 2. Cost is correctly accumulated in session metadata
// 3. Output tokens from assistant events are accumulated
func TestCostFlowEndToEnd_Claude(t *testing.T) {
	// Setup session
	baseDir := t.TempDir()
	sessionID := "test-cost-flow-claude"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Setup event bus
	v2EventBus := pubsub.NewBroker[any]()
	defer v2EventBus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session.AttachV2EventBus(ctx, v2EventBus)

	// Load and parse Claude testdata
	testdataPath := filepath.Join("..", "client", "providers", "claude", "testdata", "events.jsonl")
	data, err := os.ReadFile(testdataPath)
	require.NoError(t, err, "Failed to read Claude testdata")

	parser := claude.NewParser()

	// Track what we expect to find
	var expectedCost float64
	var expectedOutputTokens int

	// Parse each line and simulate the process flow
	lines := splitJSONL(data)
	for i, line := range lines {
		if len(line) == 0 {
			continue
		}

		outputEvent, err := parser.ParseEvent(line)
		require.NoError(t, err, "Failed to parse line %d: %s", i+1, string(line))

		// Convert OutputEvent to ProcessEvent based on event type
		// This simulates what v2/process/process.go does in handleOutputEvent()

		// Handle assistant events (contain usage/token info)
		if outputEvent.Type == client.EventAssistant && outputEvent.Usage != nil {
			expectedOutputTokens += outputEvent.Usage.OutputTokens

			v2EventBus.Publish(pubsub.UpdatedEvent, events.NewProcessEvent(events.ProcessTokenUsage, "coordinator", events.RoleCoordinator).
				WithMetrics(&metrics.TokenMetrics{
					TokensUsed:   outputEvent.Usage.TokensUsed,
					TotalTokens:  outputEvent.Usage.TotalTokens,
					OutputTokens: outputEvent.Usage.OutputTokens,
					TotalCostUSD: 0, // Cost comes from result events
				}))
		}

		// Handle result events (contain cost info when Usage is nil)
		// This is the critical path being tested: cost extraction from result events
		if outputEvent.Type == client.EventResult && outputEvent.TotalCostUSD > 0 {
			expectedCost += outputEvent.TotalCostUSD

			// This simulates publishCostEvent() which is called when result event
			// has TotalCostUSD > 0 and Usage == nil
			v2EventBus.Publish(pubsub.UpdatedEvent, events.NewProcessEvent(events.ProcessTokenUsage, "coordinator", events.RoleCoordinator).
				WithMetrics(&metrics.TokenMetrics{
					TurnCostUSD:  outputEvent.TotalCostUSD,
					TotalCostUSD: outputEvent.TotalCostUSD,
				}))
		}
	}

	waitForTokenUsage(t, session, expectedCost, expectedOutputTokens)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify metadata has correct cost
	meta, err := Load(sessionDir)
	require.NoError(t, err)

	// PRIMARY VERIFICATION: Cost extracted from result events is accumulated correctly
	require.Greater(t, meta.TokenUsage.TotalCostUSD, 0.0, "Cost should be greater than zero")
	require.InDelta(t, expectedCost, meta.TokenUsage.TotalCostUSD, 0.0001,
		"Cost should match expected value from testdata (expected: %f, got: %f)",
		expectedCost, meta.TokenUsage.TotalCostUSD)

	// Claude testdata has total_cost_usd: 0.0123 in result event (line 5)
	require.InDelta(t, 0.0123, meta.TokenUsage.TotalCostUSD, 0.0001,
		"Cost should be 0.0123 from Claude testdata result event")

	// Verify output tokens accumulated correctly
	require.Equal(t, expectedOutputTokens, meta.TokenUsage.TotalOutputTokens,
		"Output tokens should match sum of all assistant event tokens")

	// Note: Input tokens are intentionally not verified here because:
	// 1. Session replaces (not accumulates) input tokens with each event
	// 2. Cost-only events have TokensUsed=0, which resets input tokens
	// This is documented behavior - input tokens represent context size, not cumulative
}

// TestCostFlowEndToEnd_Amp verifies that cost data flows correctly from raw Amp JSON
// through the parser → process event → session → metadata.json pipeline.
// This test uses real JSON from the Amp provider testdata, not synthetic events.
//
// Key verification points:
// 1. Cost from result events is extracted (TotalCostUSD field)
// 2. Cost is correctly accumulated in session metadata
// 3. Output tokens from assistant events are accumulated
func TestCostFlowEndToEnd_Amp(t *testing.T) {
	// Setup session
	baseDir := t.TempDir()
	sessionID := "test-cost-flow-amp"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Setup event bus
	v2EventBus := pubsub.NewBroker[any]()
	defer v2EventBus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session.AttachV2EventBus(ctx, v2EventBus)

	// Load and parse Amp testdata
	testdataPath := filepath.Join("..", "client", "providers", "amp", "testdata", "events.jsonl")
	data, err := os.ReadFile(testdataPath)
	require.NoError(t, err, "Failed to read Amp testdata")

	parser := amp.NewParser()

	// Track what we expect to find
	var expectedCost float64
	var expectedOutputTokens int

	// Parse each line and simulate the process flow
	lines := splitJSONL(data)
	for i, line := range lines {
		if len(line) == 0 {
			continue
		}

		outputEvent, err := parser.ParseEvent(line)
		require.NoError(t, err, "Failed to parse line %d: %s", i+1, string(line))

		// Handle assistant events (contain usage/token info for Amp)
		if outputEvent.Type == client.EventAssistant && outputEvent.Usage != nil {
			expectedOutputTokens += outputEvent.Usage.OutputTokens

			v2EventBus.Publish(pubsub.UpdatedEvent, events.NewProcessEvent(events.ProcessTokenUsage, "coordinator", events.RoleCoordinator).
				WithMetrics(&metrics.TokenMetrics{
					TokensUsed:   outputEvent.Usage.TokensUsed,
					TotalTokens:  outputEvent.Usage.TotalTokens,
					OutputTokens: outputEvent.Usage.OutputTokens,
					TotalCostUSD: 0, // Cost comes from result events
				}))
		}

		// Handle result events (contain cost info)
		// This is the critical path being tested: cost extraction from result events
		if outputEvent.Type == client.EventResult && outputEvent.TotalCostUSD > 0 {
			expectedCost += outputEvent.TotalCostUSD

			// This simulates publishCostEvent()
			v2EventBus.Publish(pubsub.UpdatedEvent, events.NewProcessEvent(events.ProcessTokenUsage, "coordinator", events.RoleCoordinator).
				WithMetrics(&metrics.TokenMetrics{
					TurnCostUSD:  outputEvent.TotalCostUSD,
					TotalCostUSD: outputEvent.TotalCostUSD,
				}))
		}
	}

	waitForTokenUsage(t, session, expectedCost, expectedOutputTokens)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify metadata has correct cost
	meta, err := Load(sessionDir)
	require.NoError(t, err)

	// PRIMARY VERIFICATION: Cost extracted from result events is accumulated correctly
	require.Greater(t, meta.TokenUsage.TotalCostUSD, 0.0, "Cost should be greater than zero")
	require.InDelta(t, expectedCost, meta.TokenUsage.TotalCostUSD, 0.0001,
		"Cost should match expected value from testdata (expected: %f, got: %f)",
		expectedCost, meta.TokenUsage.TotalCostUSD)

	// Amp testdata has total_cost_usd: 0.0123 in the success result event (line 5)
	require.InDelta(t, 0.0123, meta.TokenUsage.TotalCostUSD, 0.0001,
		"Cost should be 0.0123 from Amp testdata result event")

	// Verify output tokens accumulated correctly
	require.Equal(t, expectedOutputTokens, meta.TokenUsage.TotalOutputTokens,
		"Output tokens should match sum of all assistant event tokens")

	// Note: Input tokens are intentionally not verified here because:
	// 1. Session replaces (not accumulates) input tokens with each event
	// 2. Cost-only events have TokensUsed=0, which resets input tokens
	// This is documented behavior - input tokens represent context size, not cumulative
}

// TestMultiProcessCostAggregation verifies that costs from multiple processes
// (coordinator + workers) are correctly aggregated into the session total.
// This tests the multi-process cost flow: each process publishes turn costs,
// and the session accumulates them all into TotalCostUSD.
func TestMultiProcessCostAggregation(t *testing.T) {
	// Setup session
	baseDir := t.TempDir()
	sessionID := "test-multi-process-cost"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Setup event bus
	v2EventBus := pubsub.NewBroker[any]()
	defer v2EventBus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session.AttachV2EventBus(ctx, v2EventBus)

	// Add workers to the session first (workers must exist before their token usage can be tracked)
	now := time.Now()
	session.addWorker("worker-1", now, "/project")
	session.addWorker("worker-2", now, "/project")

	// Define costs for each process
	coordinatorCost := 0.05
	worker1Cost := 0.02
	worker2Cost := 0.03
	expectedTotalCost := coordinatorCost + worker1Cost + worker2Cost // 0.10

	// Simulate coordinator token usage
	v2EventBus.Publish(pubsub.UpdatedEvent, events.NewProcessEvent(events.ProcessTokenUsage, "coordinator", events.RoleCoordinator).
		WithMetrics(&metrics.TokenMetrics{
			TokensUsed:   10000,
			OutputTokens: 500,
			TotalCostUSD: coordinatorCost,
		}))

	// Simulate worker-1 token usage
	v2EventBus.Publish(pubsub.UpdatedEvent, events.NewProcessEvent(events.ProcessTokenUsage, "worker-1", events.RoleWorker).
		WithMetrics(&metrics.TokenMetrics{
			TokensUsed:   8000,
			OutputTokens: 300,
			TotalCostUSD: worker1Cost,
		}))

	// Simulate worker-2 token usage
	v2EventBus.Publish(pubsub.UpdatedEvent, events.NewProcessEvent(events.ProcessTokenUsage, "worker-2", events.RoleWorker).
		WithMetrics(&metrics.TokenMetrics{
			TokensUsed:   12000,
			OutputTokens: 400,
			TotalCostUSD: worker2Cost,
		}))

	waitForTokenUsage(t, session, expectedTotalCost, 500+300+400)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify metadata has correct aggregated cost
	meta, err := Load(sessionDir)
	require.NoError(t, err)

	// Verify total cost equals sum of all process costs
	require.InDelta(t, expectedTotalCost, meta.TokenUsage.TotalCostUSD, 0.0001,
		"Total cost should be sum of coordinator + worker costs (expected: %f, got: %f)",
		expectedTotalCost, meta.TokenUsage.TotalCostUSD)

	// Verify individual cost values
	require.InDelta(t, 0.10, meta.TokenUsage.TotalCostUSD, 0.0001,
		"Total cost should be exactly $0.10 (0.05 + 0.02 + 0.03)")

	// Verify no double accumulation - if there was double accumulation,
	// we'd see a value significantly higher than 0.10
	require.Less(t, meta.TokenUsage.TotalCostUSD, 0.15,
		"Cost should not be inflated by double accumulation")

	// Verify token counts (output tokens accumulate, input tokens replace to highest)
	// Input tokens should be from the last process (worker-2 has highest at 12000,
	// but the replacement logic means it depends on event order)
	expectedTotalOutputTokens := 500 + 300 + 400 // All output tokens accumulate
	require.Equal(t, expectedTotalOutputTokens, meta.TokenUsage.TotalOutputTokens,
		"Output tokens should be sum of all processes")
}

// TestMultiProcessCostAggregation_MultiTurn verifies that multi-turn scenarios
// don't cause double accumulation. Each turn's cost should be added once.
func TestMultiProcessCostAggregation_MultiTurn(t *testing.T) {
	// Setup session
	baseDir := t.TempDir()
	sessionID := "test-multi-turn-cost"
	sessionDir := filepath.Join(baseDir, "session")

	session, err := New(sessionID, sessionDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close(StatusCompleted) })

	// Setup event bus
	v2EventBus := pubsub.NewBroker[any]()
	defer v2EventBus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session.AttachV2EventBus(ctx, v2EventBus)

	// Simulate 5 turns with known costs
	turnCosts := []float64{0.01, 0.02, 0.015, 0.01, 0.025}
	expectedTotalCost := 0.0
	for _, cost := range turnCosts {
		expectedTotalCost += cost
	}
	// expectedTotalCost = 0.08

	// Publish each turn's cost
	for i, cost := range turnCosts {
		v2EventBus.Publish(pubsub.UpdatedEvent, events.NewProcessEvent(events.ProcessTokenUsage, "coordinator", events.RoleCoordinator).
			WithMetrics(&metrics.TokenMetrics{
				TokensUsed:   (i + 1) * 1000,
				OutputTokens: 100,
				TotalCostUSD: cost, // Turn cost, not cumulative!
			}))
	}

	waitForTokenUsage(t, session, expectedTotalCost, len(turnCosts)*100)

	// Close session to flush buffers
	err = session.Close(StatusCompleted)
	require.NoError(t, err)

	// Verify metadata has correct aggregated cost
	meta, err := Load(sessionDir)
	require.NoError(t, err)

	// Verify total equals exact sum of turn costs (no double accumulation)
	require.InDelta(t, expectedTotalCost, meta.TokenUsage.TotalCostUSD, 0.0001,
		"Total cost should equal exact sum of turn costs (expected: %f, got: %f)",
		expectedTotalCost, meta.TokenUsage.TotalCostUSD)

	// Verify the value is 0.08
	require.InDelta(t, 0.08, meta.TokenUsage.TotalCostUSD, 0.0001,
		"Total cost should be exactly $0.08")

	// If double accumulation occurred, we'd see something like:
	// Turn 1: session = 0.01
	// Turn 2: session += 0.02 + 0.01 (if cumulative) = 0.04 (wrong)
	// etc.
	// But with turn-cost semantics, we just get the sum: 0.08
	require.Less(t, meta.TokenUsage.TotalCostUSD, 0.10,
		"Cost should not be inflated by double accumulation")

	// Verify output tokens accumulated correctly (5 turns * 100 = 500)
	require.Equal(t, 500, meta.TokenUsage.TotalOutputTokens,
		"Output tokens should be sum of all turns")
}

// waitForTokenUsage waits until the session has applied the published token
// usage events, i.e. its running totals reach the expected cost and output
// tokens. AttachV2EventBus applies events on its own goroutine and Close drops
// any that arrive after it, so asserting right after publishing is racy. The
// wait is non-fatal: if the totals never converge (a real aggregation bug),
// the test still fails, and the metadata assertions report the actual values.
func waitForTokenUsage(t *testing.T, s *Session, wantCostUSD float64, wantOutputTokens int) {
	t.Helper()
	assert.Eventually(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.tokenUsage.TotalOutputTokens == wantOutputTokens &&
			math.Abs(s.tokenUsage.TotalCostUSD-wantCostUSD) < 1e-9
	}, 10*time.Second, time.Millisecond, "session did not apply all published token usage events")
}

// splitJSONL splits a byte slice into lines, handling JSONL format.
func splitJSONL(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
