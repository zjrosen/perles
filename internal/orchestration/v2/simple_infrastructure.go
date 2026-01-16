// Package v2 provides the factory for creating v2 orchestration infrastructure.
package v2

import (
	"context"
	"fmt"
	"time"

	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
)

// ===========================================================================
// Simple Infrastructure (for single-process chat without MCP/tasks)
// ===========================================================================

// SimpleInfrastructureConfig holds configuration for simple chat infrastructure.
type SimpleInfrastructureConfig struct {
	// AgentProvider creates and configures AI processes.
	AgentProvider client.AgentProvider
	// WorkDir is the working directory for the chat session.
	WorkDir string
	// SystemPrompt is the system prompt for the AI assistant.
	SystemPrompt string
	// InitialPrompt is the initial user message to start the conversation (optional).
	InitialPrompt string
}

// Validate checks that all required configuration is provided.
func (c *SimpleInfrastructureConfig) Validate() error {
	if c.AgentProvider == nil {
		return fmt.Errorf("AgentProvider is required")
	}
	if c.WorkDir == "" {
		return fmt.Errorf("work directory is required")
	}
	return nil
}

// SimpleInfrastructure is a minimal infrastructure for single-process chat.
// It reuses core v2 patterns (CommandProcessor, handlers, repositories)
// without MCP server, task management, or multi-worker orchestration.
type SimpleInfrastructure struct {
	// Processor is the FIFO command processor.
	Processor *processor.CommandProcessor
	// EventBus provides pub/sub for process events.
	EventBus *pubsub.Broker[any]
	// ProcessRepo tracks process state (Ready/Working).
	ProcessRepo repository.ProcessRepository
	// QueueRepo stores pending messages when process is Working.
	QueueRepo repository.QueueRepository
	// ProcessRegistry holds live Process instances for runtime operations.
	ProcessRegistry *process.ProcessRegistry
	// CmdSubmitter submits commands to the processor.
	CmdSubmitter process.CommandSubmitter

	// config holds the original configuration for spawning.
	config SimpleInfrastructureConfig
	// ctx is the infrastructure context for lifecycle management.
	ctx context.Context
	// cancel cancels the infrastructure context on shutdown.
	cancel context.CancelFunc
}

// NewSimpleInfrastructure creates minimal infrastructure for single-process chat.
// It reuses the core v2 handlers (SpawnProcess, SendToProcess, DeliverProcessQueued,
// ProcessTurnComplete) without MCP server or task management complexity.
//
// This is ideal for:
//   - Simple chat panels that need one AI conversation
//   - Tool-free AI assistants (no MCP tools)
//   - Lightweight integrations that don't need orchestration
//
// The returned infrastructure must be started with Start() and cleaned up with Shutdown().
func NewSimpleInfrastructure(cfg SimpleInfrastructureConfig) (*SimpleInfrastructure, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid simple infrastructure config: %w", err)
	}

	// Create context with cancel for lifecycle management
	ctx, cancel := context.WithCancel(context.Background())

	// Create repositories (only the ones needed for queue-or-deliver)
	processRepo := repository.NewMemoryProcessRepository()
	queueRepo := repository.NewMemoryQueueRepository(repository.DefaultQueueMaxSize)

	// Create event bus for process events
	eventBus := pubsub.NewBroker[any]()

	// Create middleware (minimal set but includes command log for error visibility)
	loggingMiddleware := processor.NewLoggingMiddleware(processor.LoggingMiddlewareConfig{})
	commandLogMiddleware := processor.NewCommandLogMiddleware(processor.CommandLogMiddlewareConfig{
		EventBus: &eventBusAdapter{broker: eventBus},
	})
	timeoutMiddleware := processor.NewTimeoutMiddleware(processor.TimeoutMiddlewareConfig{
		WarningThreshold: 500 * time.Millisecond,
	})

	// Create command processor with smaller queue for single chat
	cmdProcessor := processor.NewCommandProcessor(
		processor.WithQueueCapacity(100),
		processor.WithEventBus(eventBus),
		processor.WithMiddleware(loggingMiddleware, commandLogMiddleware, timeoutMiddleware),
	)

	// Create process registry for runtime process access
	processRegistry := process.NewProcessRegistry()

	// Create command submitter
	cmdSubmitter := handler.NewProcessorSubmitterAdapter(cmdProcessor)

	// Create simple process spawner (no MCP - port=0, empty MCP config)
	spawner := newSimpleProcessSpawner(cfg.AgentProvider, cfg.WorkDir, cfg.SystemPrompt, cfg.InitialPrompt, cmdSubmitter, eventBus)

	// Create simple message deliverer that spawns session resumes
	deliverer := newSimpleMessageDeliverer(processRegistry, cfg.AgentProvider, cfg.WorkDir)

	// Register only the 4 core handlers needed for chat:
	// 1. SpawnProcess - create the AI process
	cmdProcessor.RegisterHandler(command.CmdSpawnProcess,
		handler.NewSpawnProcessHandler(processRepo, processRegistry,
			handler.WithUnifiedSpawner(spawner)))

	// 2. SendToProcess - queue-or-deliver logic
	cmdProcessor.RegisterHandler(command.CmdSendToProcess,
		handler.NewSendToProcessHandler(processRepo, queueRepo))

	// 3. DeliverProcessQueued - dequeue and deliver
	cmdProcessor.RegisterHandler(command.CmdDeliverProcessQueued,
		handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, processRegistry,
			handler.WithProcessDeliverer(deliverer)))

	// 4. ProcessTurnComplete - update status when AI finishes
	cmdProcessor.RegisterHandler(command.CmdProcessTurnComplete,
		handler.NewProcessTurnCompleteHandler(processRepo, queueRepo))

	return &SimpleInfrastructure{
		Processor:       cmdProcessor,
		EventBus:        eventBus,
		ProcessRepo:     processRepo,
		QueueRepo:       queueRepo,
		ProcessRegistry: processRegistry,
		CmdSubmitter:    cmdSubmitter,
		config:          cfg,
		ctx:             ctx,
		cancel:          cancel,
	}, nil
}

// Start begins the command processor loop and waits for it to be ready.
func (i *SimpleInfrastructure) Start() error {
	go i.Processor.Run(i.ctx)
	if err := i.Processor.WaitForReady(i.ctx); err != nil {
		return fmt.Errorf("waiting for command processor: %w", err)
	}
	return nil
}

// Submit adds a command to the processor queue.
func (i *SimpleInfrastructure) Submit(cmd command.Command) error {
	return i.Processor.Submit(cmd)
}

// Shutdown gracefully stops all processes and drains the command processor.
func (i *SimpleInfrastructure) Shutdown() {
	// Stop all processes first
	if i.ProcessRegistry != nil {
		i.ProcessRegistry.StopAll()
	}
	// Drain processor
	if i.Processor != nil {
		i.Processor.Drain()
	}
	// Cancel context
	if i.cancel != nil {
		i.cancel()
	}
}

// ===========================================================================
// Simple Process Spawner (no MCP)
// ===========================================================================

// simpleProcessSpawner implements UnifiedProcessSpawner for simple chat without MCP.
type simpleProcessSpawner struct {
	provider      client.AgentProvider
	workDir       string
	systemPrompt  string
	initialPrompt string
	submitter     process.CommandSubmitter
	eventBus      *pubsub.Broker[any]
}

func newSimpleProcessSpawner(
	provider client.AgentProvider,
	workDir, systemPrompt, initialPrompt string,
	submitter process.CommandSubmitter,
	eventBus *pubsub.Broker[any],
) *simpleProcessSpawner {
	return &simpleProcessSpawner{
		provider:      provider,
		workDir:       workDir,
		systemPrompt:  systemPrompt,
		initialPrompt: initialPrompt,
		submitter:     submitter,
		eventBus:      eventBus,
	}
}

// SpawnProcess implements UnifiedProcessSpawner.
func (s *simpleProcessSpawner) SpawnProcess(ctx context.Context, id string, role repository.ProcessRole, _ handler.SpawnOptions) (*process.Process, error) {
	// Get the headless client
	aiClient, err := s.provider.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get AI client: %w", err)
	}

	// Simple config: no MCP, custom system prompt
	cfg := client.Config{
		WorkDir:         s.workDir,
		SystemPrompt:    s.systemPrompt,
		Prompt:          s.initialPrompt,
		MCPConfig:       "", // No MCP tools
		SkipPermissions: true,
		DisallowedTools: []string{"AskUserQuestion"},
		Extensions:      s.provider.Extensions(),
	}

	// Spawn the underlying AI process
	headlessProc, err := aiClient.Spawn(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to spawn AI process: %w", err)
	}

	// Create process.Process wrapper
	proc := process.New(id, role, headlessProc, s.submitter, s.eventBus)

	// Start the event loop
	proc.Start()

	return proc, nil
}

// ===========================================================================
// Simple Message Deliverer
// ===========================================================================

// simpleMessageDeliverer implements MessageDeliverer by spawning a session resume.
// Like ProcessSessionDeliverer but simpler (no MCP config needed for simple chat).
type simpleMessageDeliverer struct {
	registry *process.ProcessRegistry
	provider client.AgentProvider
	workDir  string
}

func newSimpleMessageDeliverer(
	registry *process.ProcessRegistry,
	provider client.AgentProvider,
	workDir string,
) *simpleMessageDeliverer {
	return &simpleMessageDeliverer{
		registry: registry,
		provider: provider,
		workDir:  workDir,
	}
}

// Deliver implements MessageDeliverer.
// It spawns a new session with the message as prompt and resumes the process.
func (d *simpleMessageDeliverer) Deliver(_ context.Context, processID, content string) error {
	proc := d.registry.Get(processID)
	if proc == nil {
		return fmt.Errorf("process %s not found in registry", processID)
	}

	// Get session ID from existing process
	sessionID := proc.SessionID()
	if sessionID == "" {
		return fmt.Errorf("process %s has no session ID", processID)
	}

	// Get the headless client
	aiClient, err := d.provider.Client()
	if err != nil {
		return fmt.Errorf("failed to get AI client: %w", err)
	}

	// Spawn/resume session with the message as prompt
	// Use context.Background() because the process lifetime outlives this function
	cfg := client.Config{
		WorkDir:         d.workDir,
		SessionID:       sessionID,
		Prompt:          content,
		MCPConfig:       "", // No MCP tools for simple chat
		SkipPermissions: true,
		DisallowedTools: []string{"AskUserQuestion"},
		Extensions:      d.provider.Extensions(),
	}

	headlessProc, err := aiClient.Spawn(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("failed to resume session for process %s: %w", processID, err)
	}

	// Resume the process with the new HeadlessProcess
	proc.Resume(headlessProc)
	return nil
}
