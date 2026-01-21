package controlplane

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/events"
	v2 "github.com/zjrosen/perles/internal/orchestration/v2"
	"github.com/zjrosen/perles/internal/pubsub"
)

// createTestWorkflowWithEventBus creates a WorkflowInstance with a functional
// Infrastructure containing an EventBus. This is the minimal setup needed for
// testing the CrossWorkflowEventBus.
func createTestWorkflowWithEventBus(id WorkflowID, name string) *WorkflowInstance {
	eventBus := pubsub.NewBroker[any]()
	return &WorkflowInstance{
		ID:         id,
		TemplateID: "test-template.md",
		Name:       name,
		State:      WorkflowRunning,

		Infrastructure: &v2.Infrastructure{
			Core: v2.CoreComponents{
				EventBus: eventBus,
			},
		},
	}
}

func TestNewCrossWorkflowEventBus(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	require.NotNil(t, bus)
	require.NotNil(t, bus.broker)
	require.NotNil(t, bus.subscriptions)
	require.Equal(t, 0, bus.AttachedWorkflowCount())
	require.Equal(t, 0, bus.SubscriberCount())
}

func TestCrossWorkflowEventBus_AttachWorkflow(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	inst := createTestWorkflowWithEventBus(WorkflowID("wf-1"), "Test Workflow")

	// Attach the workflow
	bus.AttachWorkflow(inst)

	// Verify attachment
	require.Equal(t, 1, bus.AttachedWorkflowCount())
	require.True(t, bus.IsAttached(inst.ID))
}

func TestCrossWorkflowEventBus_AttachWorkflow_NilInstance(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	// Should not panic with nil instance
	bus.AttachWorkflow(nil)
	require.Equal(t, 0, bus.AttachedWorkflowCount())
}

func TestCrossWorkflowEventBus_AttachWorkflow_NilInfrastructure(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	inst := &WorkflowInstance{
		ID:             WorkflowID("wf-1"),
		Name:           "Test",
		Infrastructure: nil,
	}

	// Should not panic with nil infrastructure
	bus.AttachWorkflow(inst)
	require.Equal(t, 0, bus.AttachedWorkflowCount())
}

func TestCrossWorkflowEventBus_AttachWorkflow_Idempotent(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	inst := createTestWorkflowWithEventBus(WorkflowID("wf-1"), "Test Workflow")

	// Attach multiple times
	bus.AttachWorkflow(inst)
	bus.AttachWorkflow(inst)
	bus.AttachWorkflow(inst)

	// Should still only have one attachment
	require.Equal(t, 1, bus.AttachedWorkflowCount())
}

func TestCrossWorkflowEventBus_DetachWorkflow(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	inst := createTestWorkflowWithEventBus(WorkflowID("wf-1"), "Test Workflow")

	bus.AttachWorkflow(inst)
	require.True(t, bus.IsAttached(inst.ID))

	bus.DetachWorkflow(inst.ID)
	require.False(t, bus.IsAttached(inst.ID))
	require.Equal(t, 0, bus.AttachedWorkflowCount())
}

func TestCrossWorkflowEventBus_DetachWorkflow_NonExistent(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	// Should not panic when detaching non-existent workflow
	bus.DetachWorkflow(WorkflowID("non-existent"))
	require.Equal(t, 0, bus.AttachedWorkflowCount())
}

func TestCrossWorkflowEventBus_EventsWrappedWithContext(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	inst := createTestWorkflowWithEventBus(WorkflowID("wf-1"), "Feature: Auth")
	bus.AttachWorkflow(inst)

	// Subscribe to the cross-workflow bus
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := bus.Subscribe(ctx)

	// Publish an event to the workflow's internal event bus
	processEvent := events.ProcessEvent{
		Type:      events.ProcessSpawned,
		Role:      events.RoleCoordinator,
		ProcessID: "coord-1",
	}
	inst.Infrastructure.Core.EventBus.Publish(pubsub.UpdatedEvent, processEvent)

	// Receive the wrapped event
	select {
	case event := <-ch:
		// Verify event is wrapped with workflow context
		require.Equal(t, EventCoordinatorSpawned, event.Payload.Type)
		require.Equal(t, inst.ID, event.Payload.WorkflowID)
		require.Equal(t, inst.TemplateID, event.Payload.TemplateID)
		require.Equal(t, inst.Name, event.Payload.WorkflowName)
		require.Equal(t, inst.State, event.Payload.State)
		require.Equal(t, inst.State, event.Payload.State)
		require.NotZero(t, event.Payload.Timestamp)
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}
}

func TestCrossWorkflowEventBus_MultipleWorkflows(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	inst1 := createTestWorkflowWithEventBus(WorkflowID("wf-1"), "Workflow 1")
	inst2 := createTestWorkflowWithEventBus(WorkflowID("wf-2"), "Workflow 2")

	bus.AttachWorkflow(inst1)
	bus.AttachWorkflow(inst2)

	require.Equal(t, 2, bus.AttachedWorkflowCount())
	require.True(t, bus.IsAttached(inst1.ID))
	require.True(t, bus.IsAttached(inst2.ID))

	// Subscribe to the cross-workflow bus
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := bus.Subscribe(ctx)

	// Publish events from both workflows
	inst1.Infrastructure.Core.EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessSpawned,
		Role:      events.RoleCoordinator,
		ProcessID: "coord-1",
	})
	inst2.Infrastructure.Core.EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessSpawned,
		Role:      events.RoleWorker,
		ProcessID: "worker-1",
	})

	// Collect events
	receivedEvents := make([]ControlPlaneEvent, 0, 2)
	for i := 0; i < 2; i++ {
		select {
		case event := <-ch:
			receivedEvents = append(receivedEvents, event.Payload)
		case <-ctx.Done():
			t.Fatalf("timeout waiting for event %d", i+1)
		}
	}

	// Verify we received events from both workflows
	workflowIDs := make(map[WorkflowID]bool)
	for _, e := range receivedEvents {
		workflowIDs[e.WorkflowID] = true
	}
	require.True(t, workflowIDs[inst1.ID], "should have received event from workflow 1")
	require.True(t, workflowIDs[inst2.ID], "should have received event from workflow 2")
}

func TestCrossWorkflowEventBus_Publish(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	// Subscribe first
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := bus.Subscribe(ctx)

	// Publish a control plane event directly
	event := ControlPlaneEvent{
		Type:         EventWorkflowCreated,
		WorkflowID:   WorkflowID("wf-new"),
		WorkflowName: "New Workflow",
		State:        WorkflowPending,
	}
	bus.Publish(event)

	// Receive the event
	select {
	case received := <-ch:
		require.Equal(t, EventWorkflowCreated, received.Payload.Type)
		require.Equal(t, WorkflowID("wf-new"), received.Payload.WorkflowID)
		require.NotZero(t, received.Payload.Timestamp)
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}
}

func TestCrossWorkflowEventBus_PublishSetsTimestamp(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := bus.Subscribe(ctx)

	// Publish event without timestamp
	event := ControlPlaneEvent{
		Type:       EventWorkflowStarted,
		WorkflowID: WorkflowID("wf-1"),
	}
	bus.Publish(event)

	select {
	case received := <-ch:
		require.NotZero(t, received.Payload.Timestamp, "timestamp should be set automatically")
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}
}

func TestCrossWorkflowEventBus_MultipleSubscribers(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	// Create multiple subscribers
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch1 := bus.Subscribe(ctx)
	ch2 := bus.Subscribe(ctx)
	ch3 := bus.Subscribe(ctx)

	require.Equal(t, 3, bus.SubscriberCount())

	// Publish an event
	event := ControlPlaneEvent{
		Type:       EventWorkflowStarted,
		WorkflowID: WorkflowID("wf-1"),
	}
	bus.Publish(event)

	// All subscribers should receive the event
	for i, ch := range []<-chan pubsub.Event[ControlPlaneEvent]{ch1, ch2, ch3} {
		select {
		case received := <-ch:
			require.Equal(t, EventWorkflowStarted, received.Payload.Type)
		case <-ctx.Done():
			t.Fatalf("timeout waiting for event on subscriber %d", i+1)
		}
	}
}

func TestCrossWorkflowEventBus_SubscriberFromMultipleWorkflows(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	// Create multiple workflows
	workflows := make([]*WorkflowInstance, 3)
	for i := 0; i < 3; i++ {
		workflows[i] = createTestWorkflowWithEventBus(
			WorkflowID(string(rune('a'+i))),
			"Workflow "+string(rune('A'+i)),
		)
		bus.AttachWorkflow(workflows[i])
	}

	// Subscribe to all events
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := bus.Subscribe(ctx)

	// Publish events from all workflows
	for i, wf := range workflows {
		wf.Infrastructure.Core.EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
			Type:      events.ProcessOutput,
			Role:      events.RoleCoordinator,
			ProcessID: "coord-" + string(rune('1'+i)),
			Output:    "output from workflow " + string(rune('A'+i)),
		})
	}

	// Collect all events
	receivedWorkflows := make(map[WorkflowID]int)
	for i := 0; i < 3; i++ {
		select {
		case event := <-ch:
			receivedWorkflows[event.Payload.WorkflowID]++
		case <-ctx.Done():
			t.Fatalf("timeout waiting for event %d", i+1)
		}
	}

	// Verify we received one event from each workflow
	for _, wf := range workflows {
		require.Equal(t, 1, receivedWorkflows[wf.ID], "should receive exactly one event from workflow %s", wf.ID)
	}
}

func TestCrossWorkflowEventBus_DetachStopsSubscription(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	inst := createTestWorkflowWithEventBus(WorkflowID("wf-1"), "Test Workflow")
	bus.AttachWorkflow(inst)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ch := bus.Subscribe(ctx)

	// Detach the workflow
	bus.DetachWorkflow(inst.ID)

	// Give the detach time to take effect
	time.Sleep(50 * time.Millisecond)

	// Publish an event to the workflow's internal bus
	inst.Infrastructure.Core.EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessSpawned,
		Role:      events.RoleCoordinator,
		ProcessID: "coord-1",
	})

	// Should NOT receive the event since workflow is detached
	select {
	case <-ch:
		t.Fatal("should not receive event after detach")
	case <-time.After(100 * time.Millisecond):
		// Expected - no event received
	}
}

func TestCrossWorkflowEventBus_DetachDuringEventFlow(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	inst := createTestWorkflowWithEventBus(WorkflowID("wf-1"), "Test Workflow")
	bus.AttachWorkflow(inst)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := bus.Subscribe(ctx)

	// Start publishing events in a goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			inst.Infrastructure.Core.EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
				Type:      events.ProcessOutput,
				Role:      events.RoleCoordinator,
				ProcessID: "coord-1",
				Output:    "event " + string(rune('0'+i%10)),
			})
			time.Sleep(time.Millisecond)
		}
	}()

	// Detach during event flow - should not crash
	time.Sleep(20 * time.Millisecond)
	bus.DetachWorkflow(inst.ID)

	// Wait for publishing to complete
	wg.Wait()

	// Drain any remaining events - should not panic
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer drainCancel()

	for {
		select {
		case <-ch:
			// Drain events
		case <-drainCtx.Done():
			return
		}
	}
}

func TestCrossWorkflowEventBus_Close(t *testing.T) {
	bus := NewCrossWorkflowEventBus()

	inst1 := createTestWorkflowWithEventBus(WorkflowID("wf-1"), "Workflow 1")
	inst2 := createTestWorkflowWithEventBus(WorkflowID("wf-2"), "Workflow 2")

	bus.AttachWorkflow(inst1)
	bus.AttachWorkflow(inst2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := bus.Subscribe(ctx)

	// Close the bus
	bus.Close()

	// Verify all workflows are detached
	require.Equal(t, 0, bus.AttachedWorkflowCount())

	// Verify subscriber channel is closed
	_, ok := <-ch
	require.False(t, ok, "subscriber channel should be closed")
}

func TestCrossWorkflowEventBus_Broker(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	broker := bus.Broker()
	require.NotNil(t, broker)
	require.Same(t, bus.broker, broker)
}

func TestCrossWorkflowEventBus_ConcurrentAttachDetach(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	var wg sync.WaitGroup
	workflows := make([]*WorkflowInstance, 10)
	for i := 0; i < 10; i++ {
		workflows[i] = createTestWorkflowWithEventBus(
			WorkflowID(string(rune('a'+i))),
			"Workflow "+string(rune('A'+i)),
		)
	}

	// Concurrent attach/detach operations
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			bus.AttachWorkflow(workflows[i%10])
		}(i)
		go func(i int) {
			defer wg.Done()
			bus.DetachWorkflow(workflows[i%10].ID)
		}(i)
	}

	wg.Wait()

	// Should not panic and should have valid state
	count := bus.AttachedWorkflowCount()
	require.GreaterOrEqual(t, count, 0)
	require.LessOrEqual(t, count, 10)
}

func TestCrossWorkflowEventBus_EventClassification(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	inst := createTestWorkflowWithEventBus(WorkflowID("wf-1"), "Test Workflow")
	bus.AttachWorkflow(inst)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := bus.Subscribe(ctx)

	testCases := []struct {
		name         string
		event        events.ProcessEvent
		expectedType EventType
	}{
		{
			name: "CoordinatorSpawned",
			event: events.ProcessEvent{
				Type:      events.ProcessSpawned,
				Role:      events.RoleCoordinator,
				ProcessID: "coord-1",
			},
			expectedType: EventCoordinatorSpawned,
		},
		{
			name: "WorkerSpawned",
			event: events.ProcessEvent{
				Type:      events.ProcessSpawned,
				Role:      events.RoleWorker,
				ProcessID: "worker-1",
			},
			expectedType: EventWorkerSpawned,
		},
		{
			name: "WorkerOutput",
			event: events.ProcessEvent{
				Type:      events.ProcessOutput,
				Role:      events.RoleWorker,
				ProcessID: "worker-1",
				Output:    "implementing...",
			},
			expectedType: EventWorkerOutput,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			inst.Infrastructure.Core.EventBus.Publish(pubsub.UpdatedEvent, tc.event)

			select {
			case received := <-ch:
				require.Equal(t, tc.expectedType, received.Payload.Type)
			case <-ctx.Done():
				t.Fatal("timeout waiting for event")
			}
		})
	}
}

func TestCrossWorkflowEventBus_ActiveWorkerCount(t *testing.T) {
	bus := NewCrossWorkflowEventBus()
	defer bus.Close()

	inst := createTestWorkflowWithEventBus(WorkflowID("wf-worker-count"), "Worker Count Test")
	require.Equal(t, 0, inst.ActiveWorkers, "initial ActiveWorkers should be 0")

	bus.AttachWorkflow(inst)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := bus.Subscribe(ctx)

	// Spawn first worker
	inst.Infrastructure.Core.EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessSpawned,
		Role:      events.RoleWorker,
		ProcessID: "worker-1",
	})
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}
	require.Equal(t, 1, inst.ActiveWorkers, "ActiveWorkers should be 1 after first spawn")

	// Spawn second worker
	inst.Infrastructure.Core.EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessSpawned,
		Role:      events.RoleWorker,
		ProcessID: "worker-2",
	})
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}
	require.Equal(t, 2, inst.ActiveWorkers, "ActiveWorkers should be 2 after second spawn")

	// Retire first worker
	inst.Infrastructure.Core.EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		Role:      events.RoleWorker,
		ProcessID: "worker-1",
		Status:    events.ProcessStatusRetired,
	})
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}
	require.Equal(t, 1, inst.ActiveWorkers, "ActiveWorkers should be 1 after retire")

	// Retire second worker
	inst.Infrastructure.Core.EventBus.Publish(pubsub.UpdatedEvent, events.ProcessEvent{
		Type:      events.ProcessStatusChange,
		Role:      events.RoleWorker,
		ProcessID: "worker-2",
		Status:    events.ProcessStatusRetired,
	})
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}
	require.Equal(t, 0, inst.ActiveWorkers, "ActiveWorkers should be 0 after all retired")
}
