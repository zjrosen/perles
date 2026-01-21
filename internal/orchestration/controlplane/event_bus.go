package controlplane

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/pubsub"
)

// CrossWorkflowEventBus aggregates events from all workflow instances and
// republishes them with workflow context as ControlPlaneEvents. This enables
// unified event handling across multiple concurrent workflows.
//
// The event bus:
// - Attaches to each workflow's internal event bus (Infrastructure.Core.EventBus)
// - Wraps incoming events in ControlPlaneEvent envelopes with workflow context
// - Republishes events to subscribers through a central broker
// - Manages subscription lifecycles with proper cleanup on detach
// LifecycleCallback is invoked when a lifecycle event is detected.
// It provides the workflow instance and the event for the control plane
// to take action (e.g., transitioning workflow state).
type LifecycleCallback func(inst *WorkflowInstance, event ControlPlaneEvent)

type CrossWorkflowEventBus struct {
	// broker is the central pub/sub broker for ControlPlaneEvents.
	// All subscribers receive events from this broker.
	broker *pubsub.Broker[ControlPlaneEvent]

	// subscriptions tracks active workflow subscriptions.
	// Key is WorkflowID, value is cancel function to stop the subscription.
	subscriptions map[WorkflowID]context.CancelFunc

	// lifecycleCallback is invoked for lifecycle events like workflow completion.
	// This allows the control plane to react to events and update workflow state.
	lifecycleCallback LifecycleCallback

	// mu protects subscriptions map.
	mu sync.RWMutex
}

// NewCrossWorkflowEventBus creates a new CrossWorkflowEventBus.
func NewCrossWorkflowEventBus() *CrossWorkflowEventBus {
	return &CrossWorkflowEventBus{
		broker:        pubsub.NewBroker[ControlPlaneEvent](),
		subscriptions: make(map[WorkflowID]context.CancelFunc),
	}
}

// SetLifecycleCallback sets the callback for lifecycle events.
// The callback is invoked synchronously when a lifecycle event is forwarded.
func (b *CrossWorkflowEventBus) SetLifecycleCallback(cb LifecycleCallback) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lifecycleCallback = cb
}

// AttachWorkflow subscribes to a workflow's internal event bus and republishes
// events with workflow context attached. Events from the workflow's
// Infrastructure.Core.EventBus are wrapped in ControlPlaneEvent envelopes
// and published to the central broker.
//
// AttachWorkflow is idempotent - calling it multiple times for the same
// workflow ID will first detach any existing subscription before attaching.
func (b *CrossWorkflowEventBus) AttachWorkflow(inst *WorkflowInstance) {
	if inst == nil || inst.Infrastructure == nil {
		log.Debug(log.CatOrch, "AttachWorkflow skipped - nil instance or infrastructure", "subsystem", "eventbus")
		return
	}
	log.Debug(log.CatOrch, "AttachWorkflow called", "subsystem", "eventbus", "workflowID", inst.ID, "hasEventBus", inst.Infrastructure.Core.EventBus != nil)

	// Detach existing subscription if present (idempotent behavior)
	b.DetachWorkflow(inst.ID)

	// Create cancellable context for this subscription
	ctx, cancel := context.WithCancel(context.Background())

	b.mu.Lock()
	b.subscriptions[inst.ID] = cancel
	b.mu.Unlock()

	// Subscribe to the workflow's internal event bus
	ch := inst.Infrastructure.Core.EventBus.Subscribe(ctx)

	// Start goroutine to forward events with workflow context (with panic recovery)
	log.SafeGo(fmt.Sprintf("eventbus.forwardEvents[%s]", inst.ID), func() {
		b.forwardEvents(ctx, inst, ch)
	})

	// Subscribe to the workflow's MessageRepository broker for inter-agent messages
	if inst.MessageRepo != nil {
		msgCh := inst.MessageRepo.Broker().Subscribe(ctx)
		log.SafeGo(fmt.Sprintf("eventbus.forwardMessageEvents[%s]", inst.ID), func() {
			b.forwardMessageEvents(ctx, inst, msgCh)
		})
	}
}

// forwardEvents reads events from the workflow's event bus channel and
// republishes them as ControlPlaneEvents with workflow context.
func (b *CrossWorkflowEventBus) forwardEvents(
	ctx context.Context,
	inst *WorkflowInstance,
	ch <-chan pubsub.Event[any],
) {
	log.Debug(log.CatOrch, "forwardEvents started", "subsystem", "eventbus", "workflowID", inst.ID)
	for {
		select {
		case <-ctx.Done():
			log.Debug(log.CatOrch, "forwardEvents context done", "subsystem", "eventbus", "workflowID", inst.ID)
			return
		case event, ok := <-ch:
			if !ok {
				// Channel closed, stop forwarding
				log.Debug(log.CatOrch, "forwardEvents channel closed", "subsystem", "eventbus", "workflowID", inst.ID)
				return
			}

			// Classify the event type based on the payload
			eventType := ClassifyEvent(event.Payload)
			log.Debug(log.CatOrch, "forwardEvents received event", "subsystem", "eventbus", "workflowID", inst.ID, "eventType", eventType, "payloadType", fmt.Sprintf("%T", event.Payload))

			// Update workflow heartbeat on any activity
			inst.RecordHeartbeat()

			// Update worker count on spawn/retire events
			switch eventType {
			case EventWorkerSpawned:
				inst.ActiveWorkers++
			case EventWorkerRetired:
				if inst.ActiveWorkers > 0 {
					inst.ActiveWorkers--
				}
			}

			// Create ControlPlaneEvent with workflow context
			cpEvent := ControlPlaneEvent{
				Type:         eventType,
				Timestamp:    time.Now(),
				WorkflowID:   inst.ID,
				TemplateID:   inst.TemplateID,
				WorkflowName: inst.Name,
				State:        inst.State,
				Payload:      event.Payload,
			}

			// Extract process/task IDs from payload if available
			cpEvent = enrichEventFromPayload(cpEvent, event.Payload)

			// Invoke lifecycle callback for lifecycle events (e.g., workflow completion)
			if eventType.IsLifecycleEvent() {
				b.mu.RLock()
				cb := b.lifecycleCallback
				b.mu.RUnlock()
				if cb != nil {
					cb(inst, cpEvent)
				}
			}

			// Publish to central broker
			b.broker.Publish(pubsub.UpdatedEvent, cpEvent)
		}
	}
}

// forwardMessageEvents reads message events from the MessageRepository broker
// and republishes them as ControlPlaneEvents with workflow context.
func (b *CrossWorkflowEventBus) forwardMessageEvents(
	ctx context.Context,
	inst *WorkflowInstance,
	ch <-chan pubsub.Event[message.Event],
) {
	log.Debug(log.CatOrch, "forwardMessageEvents started", "subsystem", "eventbus", "workflowID", inst.ID)
	for {
		select {
		case <-ctx.Done():
			log.Debug(log.CatOrch, "forwardMessageEvents context done", "subsystem", "eventbus", "workflowID", inst.ID)
			return
		case event, ok := <-ch:
			if !ok {
				log.Debug(log.CatOrch, "forwardMessageEvents channel closed", "subsystem", "eventbus", "workflowID", inst.ID)
				return
			}

			log.Debug(log.CatOrch, "forwardMessageEvents received message", "subsystem", "eventbus",
				"workflowID", inst.ID, "from", event.Payload.Entry.From, "to", event.Payload.Entry.To)

			// Create ControlPlaneEvent with message payload
			cpEvent := ControlPlaneEvent{
				Type:         EventMessagePosted,
				Timestamp:    time.Now(),
				WorkflowID:   inst.ID,
				TemplateID:   inst.TemplateID,
				WorkflowName: inst.Name,
				State:        inst.State,
				Payload:      event.Payload, // message.Event containing the Entry
			}

			// Publish to central broker
			b.broker.Publish(pubsub.UpdatedEvent, cpEvent)
		}
	}
}

// enrichEventFromPayload extracts ProcessID and TaskID from the event payload
// if the payload contains these fields.
func enrichEventFromPayload(cpEvent ControlPlaneEvent, payload any) ControlPlaneEvent {
	// Check for ProcessEvent-like payloads with ProcessID field
	if p, ok := payload.(interface{ GetProcessID() string }); ok {
		cpEvent.ProcessID = p.GetProcessID()
	}

	// Check for Task-related payloads
	if t, ok := payload.(interface{ GetTaskID() string }); ok {
		cpEvent.TaskID = t.GetTaskID()
	}

	// Handle events.ProcessEvent specifically since it's a struct with fields
	// Use type assertion on the underlying struct fields
	if pe, ok := payload.(struct {
		ProcessID string
		TaskID    string
	}); ok {
		cpEvent.ProcessID = pe.ProcessID
		cpEvent.TaskID = pe.TaskID
	}

	// Common pattern: payload is events.ProcessEvent with public fields
	// Access via reflection-free type assertion on known fields
	type processEventFields interface {
		GetProcessID() string
		GetTaskID() string
	}
	if fields, ok := payload.(processEventFields); ok {
		cpEvent.ProcessID = fields.GetProcessID()
		cpEvent.TaskID = fields.GetTaskID()
	}

	return cpEvent
}

// DetachWorkflow stops the subscription for a workflow and removes it from
// the subscriptions map. This is safe to call for workflows that are not
// attached (no-op in that case).
func (b *CrossWorkflowEventBus) DetachWorkflow(id WorkflowID) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if cancel, exists := b.subscriptions[id]; exists {
		cancel() // Stop the forwarding goroutine
		delete(b.subscriptions, id)
	}
}

// Publish publishes a ControlPlaneEvent directly to the broker.
// This is used for events that originate from the ControlPlane itself,
// such as workflow lifecycle events (created, started, stopped, etc.)
// rather than from individual workflow instances.
func (b *CrossWorkflowEventBus) Publish(event ControlPlaneEvent) {
	// Ensure timestamp is set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	b.broker.Publish(pubsub.UpdatedEvent, event)
}

// Subscribe returns a channel that receives all ControlPlaneEvents from
// all attached workflows and directly published events. The channel is
// automatically closed when the context is cancelled.
func (b *CrossWorkflowEventBus) Subscribe(ctx context.Context) <-chan pubsub.Event[ControlPlaneEvent] {
	return b.broker.Subscribe(ctx)
}

// Broker returns the underlying pubsub.Broker for advanced use cases.
// This allows external code to use pubsub.ContinuousListener for Bubble Tea
// integration.
func (b *CrossWorkflowEventBus) Broker() *pubsub.Broker[ControlPlaneEvent] {
	return b.broker
}

// SubscriberCount returns the number of active subscribers to the event bus.
func (b *CrossWorkflowEventBus) SubscriberCount() int {
	return b.broker.SubscriberCount()
}

// AttachedWorkflowCount returns the number of workflows currently attached
// to the event bus.
func (b *CrossWorkflowEventBus) AttachedWorkflowCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscriptions)
}

// Close shuts down the event bus, cancelling all workflow subscriptions
// and closing all subscriber channels.
func (b *CrossWorkflowEventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Cancel all workflow subscriptions
	for _, cancel := range b.subscriptions {
		cancel()
	}
	b.subscriptions = make(map[WorkflowID]context.CancelFunc)

	// Close the broker (closes all subscriber channels)
	b.broker.Close()
}

// IsAttached returns true if the given workflow is currently attached
// to the event bus.
func (b *CrossWorkflowEventBus) IsAttached(id WorkflowID) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, exists := b.subscriptions[id]
	return exists
}
