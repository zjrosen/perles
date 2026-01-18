// Package processor provides the FIFO command processor for the v2 orchestration architecture.
// The processor is the heart of v2 - a single-threaded loop that processes commands
// in strict FIFO order, eliminating most lock operations from the system.
package processor

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/orchestration/v2/types"
	"github.com/zjrosen/perles/internal/pubsub"
)

// DefaultQueueCapacity is the default buffer size for the command queue.
const DefaultQueueCapacity = 1000

// CommandHandler is an alias for types.CommandHandler to avoid import cycles.
// See types.CommandHandler for documentation.
type CommandHandler = types.CommandHandler

// HandlerFunc is an alias for types.HandlerFunc to avoid import cycles.
// See types.HandlerFunc for documentation.
type HandlerFunc = types.HandlerFunc

// Option configures the CommandProcessor.
type Option func(*CommandProcessor)

// WithQueueCapacity sets the command queue buffer capacity.
func WithQueueCapacity(capacity int) Option {
	return func(p *CommandProcessor) {
		p.queueCapacity = capacity
	}
}

// WithEventBus sets the event bus for publishing command results.
func WithEventBus(bus *pubsub.Broker[any]) Option {
	return func(p *CommandProcessor) {
		p.eventBus = bus
	}
}

// WithTaskRepository sets the task repository.
func WithTaskRepository(repo repository.TaskRepository) Option {
	return func(p *CommandProcessor) {
		p.taskRepo = repo
	}
}

// WithQueueRepository sets the queue repository.
func WithQueueRepository(repo repository.QueueRepository) Option {
	return func(p *CommandProcessor) {
		p.queueRepo = repo
	}
}

// WithMiddleware adds middleware to be applied to all handlers.
// Middleware is applied in order: first middleware wraps outermost.
func WithMiddleware(middlewares ...Middleware) Option {
	return func(p *CommandProcessor) {
		p.middlewares = append(p.middlewares, middlewares...)
	}
}

// CommandProcessor processes commands sequentially in FIFO order.
// This is the heart of the v2 architecture - single-threaded processing
// eliminates most lock operations while maintaining deterministic execution.
type CommandProcessor struct {
	// Command queue (buffered channel)
	queue         chan queueItem
	queueCapacity int

	// Handler registry
	handlers map[command.CommandType]CommandHandler

	// Repositories (dependency injection)
	taskRepo  repository.TaskRepository
	queueRepo repository.QueueRepository

	// Middleware chain applied to all handlers
	middlewares []Middleware

	// Event publishing
	eventBus *pubsub.Broker[any]

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// State tracking
	running  atomic.Bool
	started  atomic.Bool
	readyCh  chan struct{} // Closed when processor is ready to accept commands
	readyMu  sync.Mutex    // Protects readyCh initialization
	readySet bool          // True after readyCh is closed

	// Metrics
	processedCount atomic.Int64
	errorCount     atomic.Int64
}

// queueItem wraps a command with an optional result channel for SubmitAndWait.
type queueItem struct {
	cmd      command.Command
	resultCh chan *commandResponse // nil for fire-and-forget Submit
}

// commandResponse wraps the result and error for SubmitAndWait.
type commandResponse struct {
	result *command.CommandResult
	err    error
}

// NewCommandProcessor creates a new CommandProcessor with the given options.
func NewCommandProcessor(opts ...Option) *CommandProcessor {
	p := &CommandProcessor{
		queueCapacity: DefaultQueueCapacity,
		handlers:      make(map[command.CommandType]CommandHandler),
		readyCh:       make(chan struct{}),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// RegisterHandler registers a handler for a command type.
// Must be called before Run() is called.
// The handler is wrapped with all configured middleware.
func (p *CommandProcessor) RegisterHandler(cmdType command.CommandType, handler CommandHandler) {
	p.handlers[cmdType] = ChainMiddleware(handler, p.middlewares...)
}

// Run starts the command processing loop.
// This method blocks until the context is cancelled or Stop() is called.
// Run can only be called once - subsequent calls return immediately.
func (p *CommandProcessor) Run(ctx context.Context) {
	// Prevent multiple Run calls
	if !p.started.CompareAndSwap(false, true) {
		return
	}

	// Create cancellable context for shutdown
	p.ctx, p.cancel = context.WithCancel(ctx)

	// Initialize queue
	p.queue = make(chan queueItem, p.queueCapacity)

	// Add to wait group BEFORE setting running to avoid race with Drain()
	p.wg.Add(1)
	p.running.Store(true)

	// Signal that processor is ready to accept commands
	p.readyMu.Lock()
	if !p.readySet {
		close(p.readyCh)
		p.readySet = true
	}
	p.readyMu.Unlock()

	defer func() {
		p.running.Store(false)
		p.wg.Done()
	}()

	// Main processing loop
	for {
		select {
		case <-p.ctx.Done():
			return
		case item, ok := <-p.queue:
			if !ok {
				// Queue closed during Drain
				return
			}
			p.processItem(item)
		}
	}
}

// WaitForReady blocks until the processor is ready to accept commands.
// Returns immediately if the processor is already running.
// Returns an error if the context is cancelled before the processor is ready.
func (p *CommandProcessor) WaitForReady(ctx context.Context) error {
	select {
	case <-p.readyCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Submit adds a command to the queue for asynchronous processing.
// Returns immediately. Returns ErrQueueFull if the queue is at capacity.
func (p *CommandProcessor) Submit(cmd command.Command) error {
	if !p.running.Load() {
		return command.ErrQueueFull
	}

	item := queueItem{
		cmd:      cmd,
		resultCh: nil, // Fire-and-forget
	}

	select {
	case p.queue <- item:
		return nil
	default:
		return command.ErrQueueFull
	}
}

// SubmitAndWait adds a command to the queue and waits for the result.
// Returns the command result or an error. Respects context cancellation.
func (p *CommandProcessor) SubmitAndWait(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	if !p.running.Load() {
		return nil, command.ErrQueueFull
	}

	resultCh := make(chan *commandResponse, 1)
	item := queueItem{
		cmd:      cmd,
		resultCh: resultCh,
	}

	// Try to submit
	select {
	case p.queue <- item:
		// Submitted successfully
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, command.ErrQueueFull
	}

	// Wait for result
	select {
	case resp := <-resultCh:
		return resp.result, resp.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.ctx.Done():
		// Processor is shutting down
		return nil, context.Canceled
	}
}

// Stop cancels the processing context and waits for shutdown.
// Any pending commands in the queue are NOT processed.
func (p *CommandProcessor) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.wg.Wait()
}

// Drain processes all remaining commands in the queue before stopping.
// This provides graceful shutdown - all queued commands are completed.
func (p *CommandProcessor) Drain() {
	if !p.running.Load() {
		return
	}

	// Stop accepting new commands
	p.running.Store(false)

	// Close the queue to signal drain mode
	close(p.queue)

	// Wait for processing loop to finish
	p.wg.Wait()
}

// IsRunning returns true if the processor is currently accepting commands.
func (p *CommandProcessor) IsRunning() bool {
	return p.running.Load()
}

// ProcessedCount returns the total number of commands processed.
func (p *CommandProcessor) ProcessedCount() int64 {
	return p.processedCount.Load()
}

// ErrorCount returns the total number of commands that resulted in errors.
func (p *CommandProcessor) ErrorCount() int64 {
	return p.errorCount.Load()
}

// QueueLength returns the current number of pending commands.
func (p *CommandProcessor) QueueLength() int {
	if p.queue == nil {
		return 0
	}
	return len(p.queue)
}

// processItem handles a single command from the queue.
func (p *CommandProcessor) processItem(item queueItem) {
	result := p.processCommand(item.cmd)

	// Update metrics
	p.processedCount.Add(1)
	if result != nil && !result.Success {
		p.errorCount.Add(1)
	}

	// Send result if caller is waiting
	if item.resultCh != nil {
		item.resultCh <- &commandResponse{
			result: result,
			err:    nil,
		}
		close(item.resultCh)
	}
}

// processCommand executes the command processing pipeline.
// Errors are wrapped in the CommandResult, not returned separately.
func (p *CommandProcessor) processCommand(cmd command.Command) *command.CommandResult {
	// Step 1: Validate the command
	if err := cmd.Validate(); err != nil {
		result := &command.CommandResult{
			Success: false,
			Error:   err,
		}
		p.emitErrorEvent(cmd, err)
		return result
	}

	// Step 2: Route to handler
	handler, ok := p.handlers[cmd.Type()]
	if !ok {
		result := &command.CommandResult{
			Success: false,
			Error:   ErrUnknownCommandType,
		}
		p.emitErrorEvent(cmd, ErrUnknownCommandType)
		return result
	}

	// Step 3: Execute handler
	result, err := handler.Handle(p.ctx, cmd)
	if err != nil {
		// Handler returned an error - wrap it in a result
		result = &command.CommandResult{
			Success: false,
			Error:   err,
		}
		p.emitErrorEvent(cmd, err)
		return result
	}

	// Step 4: Emit events from result
	if result != nil && len(result.Events) > 0 {
		p.emitEvents(result.Events)
	}

	// Step 5: Enqueue follow-up commands
	if result != nil && len(result.FollowUp) > 0 {
		for _, followUp := range result.FollowUp {
			// Submit follow-ups - they go to the end of the queue (FIFO)
			// Use non-blocking submit to avoid deadlock
			select {
			case p.queue <- queueItem{cmd: followUp}:
				// Submitted
			default:
				// Queue full - log but don't fail
				// This shouldn't happen in normal operation
			}
		}
	}

	return result
}

// emitEvents publishes events to the event bus.
func (p *CommandProcessor) emitEvents(events []any) {
	if p.eventBus == nil {
		return
	}
	for _, event := range events {
		// Non-blocking publish
		p.eventBus.Publish(pubsub.UpdatedEvent, event)
	}
}

// emitErrorEvent publishes an error event for command failures.
func (p *CommandProcessor) emitErrorEvent(cmd command.Command, err error) {
	if p.eventBus == nil {
		return
	}
	// Create an error event with command context
	errorEvent := CommandErrorEvent{
		CommandID:   cmd.ID(),
		CommandType: cmd.Type(),
		Error:       err,
	}
	p.eventBus.Publish(pubsub.UpdatedEvent, errorEvent)
}
