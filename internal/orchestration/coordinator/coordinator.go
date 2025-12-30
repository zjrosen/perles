// Package coordinator provides the orchestrating agent that manages epic execution.
//
// The Coordinator is an interactive Claude session that receives user guidance
// and spawns worker agents to execute tasks. It communicates with workers via
// MCP tools exposed by perles, and all communication is logged to a message issue.
//
// # Architecture
//
// The Coordinator sits between the user (TUI) and the worker pool:
//
//	User Input -> Coordinator (Claude session) -> MCP Tools -> Worker Pool
//	                    |                              |
//	                    +-- Events <-- TUI updates <---+
//
// # Communication Flow
//
//  1. User types message in TUI
//  2. TUI calls Coordinator.SendUserMessage()
//  3. Coordinator forwards to its Claude session
//  4. Claude may call MCP tools (spawn_worker, post_message, etc.)
//  5. Coordinator emits events for TUI updates
package coordinator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/metrics"
	"github.com/zjrosen/perles/internal/orchestration/pool"
	"github.com/zjrosen/perles/internal/orchestration/queue"

	"github.com/zjrosen/perles/internal/pubsub"
)

// Status represents the coordinator's current state.
type Status int

const (
	// StatusPending means the coordinator has not started yet.
	StatusPending Status = iota
	// StatusStarting means the coordinator is initializing.
	StatusStarting
	// StatusRunning means the coordinator is active and processing.
	StatusRunning
	// StatusPaused means the coordinator is paused by user request.
	StatusPaused
	// StatusStopping means the coordinator is shutting down.
	StatusStopping
	// StatusStopped means the coordinator has stopped.
	StatusStopped
	// StatusFailed means the coordinator encountered an error.
	StatusFailed
)

func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusStarting:
		return "starting"
	case StatusRunning:
		return "running"
	case StatusPaused:
		return "paused"
	case StatusStopping:
		return "stopping"
	case StatusStopped:
		return "stopped"
	case StatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Config holds configuration for creating a Coordinator.
type Config struct {
	// WorkDir is the working directory for Claude processes.
	WorkDir string

	// Client is the headless client for spawning AI processes.
	// This allows injection of different providers (Claude, Amp, etc.)
	// or mock clients for testing.
	Client client.HeadlessClient

	// Pool is the worker pool for spawning workers.
	Pool *pool.WorkerPool

	// MessageIssue is the message issue for inter-agent communication.
	MessageIssue *message.Issue

	// Port is the MCP HTTP server port for config generation.
	Port int

	// Model specifies which AI model to use (sonnet, opus, haiku for Claude).
	// Defaults to sonnet if empty.
	Model string
}

// Coordinator manages an orchestrated execution session.
type Coordinator struct {
	workDir      string
	client       client.HeadlessClient
	pool         *pool.WorkerPool
	messageIssue *message.Issue
	port         int // MCP HTTP server port for config generation
	model        string

	// AI session
	process   client.HeadlessProcess
	sessionID string
	status    atomic.Int32

	// Token usage tracking - accumulates across turns
	cumulativeMetrics *metrics.TokenMetrics

	// Communication - embedded pub/sub broker for coordinator events
	// Worker events are published directly by pool, accessed via Workers()
	broker *pubsub.Broker[events.CoordinatorEvent]

	// Message queue for user messages when coordinator is busy
	// Protected by queueMu for thread-safe access
	messageQueue *queue.MessageQueue
	queueMu      sync.Mutex
	working      bool // True when coordinator is actively processing (between Working and Ready events)

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	wg     sync.WaitGroup
}

// New creates a new Coordinator with the given configuration.
func New(cfg Config) (*Coordinator, error) {
	if cfg.WorkDir == "" {
		return nil, fmt.Errorf("work directory is required")
	}
	if cfg.Client == nil {
		return nil, fmt.Errorf("headless client is required")
	}
	if cfg.Pool == nil {
		return nil, fmt.Errorf("worker pool is required")
	}
	if cfg.MessageIssue == nil {
		return nil, fmt.Errorf("message issue is required")
	}

	model := cfg.Model
	if model == "" {
		model = "sonnet"
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &Coordinator{
		workDir:           cfg.WorkDir,
		client:            cfg.Client,
		pool:              cfg.Pool,
		messageIssue:      cfg.MessageIssue,
		port:              cfg.Port,
		model:             model,
		cumulativeMetrics: &metrics.TokenMetrics{},
		broker:            pubsub.NewBroker[events.CoordinatorEvent](),
		messageQueue:      queue.NewMessageQueue(queue.DefaultMaxSize),
		ctx:               ctx,
		cancel:            cancel,
	}

	c.status.Store(int32(StatusPending))

	return c, nil
}

// Status returns the current coordinator status.
func (c *Coordinator) Status() Status {
	return Status(c.status.Load())
}

// SessionID returns the Claude session ID, or empty if not started.
func (c *Coordinator) SessionID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.sessionID
}

// PID returns the process ID of the coordinator's Claude process, or 0 if not running.
func (c *Coordinator) PID() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.process != nil {
		return c.process.PID()
	}
	return 0
}

// IsRunning returns true if the coordinator is actively processing.
func (c *Coordinator) IsRunning() bool {
	s := c.Status()
	return s == StatusRunning || s == StatusStarting
}

// Workers returns the worker event broker for subscription.
// Subscribers can receive events about worker spawns, status changes, and output.
// This delegates to the pool's broker - pool publishes events.WorkerEvent directly.
func (c *Coordinator) Workers() *pubsub.Broker[events.WorkerEvent] {
	return c.pool.Broker()
}

func (c *Coordinator) Broker() *pubsub.Broker[events.CoordinatorEvent] {
	return c.broker
}

// setStatus atomically updates status and emits an event.
func (c *Coordinator) setStatus(s Status) {
	// Status values are defined constants that always fit in int32
	//nolint:gosec // G115: Status is an enum with a small fixed range (0-6)
	c.status.Store(int32(s))
	c.emitCoordinatorEvent(events.CoordinatorStatusChange, events.CoordinatorEvent{
		Status: statusToEventStatus(s),
	})
}

// emitCoordinatorEvent publishes an event to the embedded broker.
// The eventType is used for the pub/sub Event.Type wrapper, and the
// event.Type should be set in the payload for the specific coordinator event type.
func (c *Coordinator) emitCoordinatorEvent(eventType events.CoordinatorEventType, event events.CoordinatorEvent) {
	event.Type = eventType
	c.broker.Publish(pubsub.UpdatedEvent, event)
}

// statusToEventStatus converts the internal Status type to events.CoordinatorStatus.
func statusToEventStatus(s Status) events.CoordinatorStatus {
	switch s {
	case StatusRunning:
		return events.StatusReady
	case StatusPaused:
		return events.StatusPaused
	case StatusStopped, StatusStopping:
		return events.StatusStopped
	default:
		return events.StatusWorking
	}
}
