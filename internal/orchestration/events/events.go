// Package events defines typed event structures for the orchestration layer.
// These events are published via the pubsub broker and consumed by the TUI
// and other subscribers.
//
// Event types are organized by source:
//   - CoordinatorEvent: Events from the main coordinator process
//   - WorkerEvent: Events from worker processes in the pool
package events
