// Package v2 provides the factory for creating v2 orchestration infrastructure.
// The factory encapsulates all v2 component setup including repositories, command
// processor, handlers, and lifecycle management.
package v2

import (
	"context"
	"fmt"
	"time"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/integration"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
)

// eventBusAdapter adapts pubsub.Broker to the processor.EventPublisher interface.
// This is needed because pubsub.EventType and processor.EventPublisher use different
// type signatures (pubsub uses a typed EventType string, processor uses plain string).
type eventBusAdapter struct {
	broker *pubsub.Broker[any]
}

// Publish implements processor.EventPublisher.
func (a *eventBusAdapter) Publish(eventType string, payload any) {
	a.broker.Publish(pubsub.EventType(eventType), payload)
}

// InfrastructureConfig holds configuration for creating V2 infrastructure.
type InfrastructureConfig struct {
	// Port is the MCP server port for process communication.
	Port int
	// AIClient is the headless AI client for spawning processes.
	AIClient client.HeadlessClient
	// WorkDir is the working directory for the orchestration session.
	WorkDir string
	// Extensions contains provider-specific configuration (e.g., model settings).
	Extensions map[string]any
	// MessageRepo is the message repository for inter-agent messaging.
	MessageRepo repository.MessageRepository
	// SessionID is the session identifier for accountability summary generation.
	SessionID string
}

// Validate checks that all required configuration is provided.
func (c *InfrastructureConfig) Validate() error {
	if c.Port == 0 {
		return fmt.Errorf("port is required")
	}
	if c.AIClient == nil {
		return fmt.Errorf("AI client is required")
	}
	if c.MessageRepo == nil {
		return fmt.Errorf("message repository is required")
	}
	if c.WorkDir == "" {
		return fmt.Errorf("work directory is required")
	}
	return nil
}

// Infrastructure holds all v2 orchestration components.
type Infrastructure struct {
	// Core components
	Core CoreComponents

	// Repositories for state management
	Repositories RepositoryComponents

	// Internal components (not exposed externally)
	Internal InternalComponents

	// config holds the original configuration for lifecycle operations
	config InfrastructureConfig
}

// CoreComponents holds the core v2 infrastructure pieces.
type CoreComponents struct {
	// Processor is the FIFO command processor.
	Processor *processor.CommandProcessor
	// Adapter bridges MCP tool calls to v2 commands.
	Adapter *adapter.V2Adapter
	// EventBus provides pub/sub for v2 orchestration events.
	EventBus *pubsub.Broker[any]
	// CmdSubmitter submits commands to the processor (fire-and-forget).
	CmdSubmitter process.CommandSubmitter
}

// RepositoryComponents holds all repository instances.
type RepositoryComponents struct {
	// ProcessRepo tracks process state (coordinator + workers).
	ProcessRepo repository.ProcessRepository
	// TaskRepo tracks task assignments.
	TaskRepo repository.TaskRepository
	// QueueRepo tracks per-worker message queues.
	QueueRepo repository.QueueRepository
}

// InternalComponents holds internal infrastructure not exposed externally.
type InternalComponents struct {
	// ProcessRegistry holds live Process instances for runtime operations.
	ProcessRegistry *process.ProcessRegistry
	// TurnEnforcer tracks MCP tool calls during worker turns for enforcement.
	TurnEnforcer handler.TurnCompletionEnforcer
}

// NewInfrastructure creates all v2 orchestration infrastructure components.
// This factory encapsulates the complex setup of repositories, processor, handlers,
// and adapter that was previously inline in initializer.go.
//
// The returned Infrastructure must be started with Start() before use and
// cleaned up with Drain() when shutting down.
func NewInfrastructure(cfg InfrastructureConfig) (*Infrastructure, error) {
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid infrastructure config: %w", err)
	}

	// Create repositories
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(repository.DefaultQueueMaxSize)
	processRepo := repository.NewMemoryProcessRepository()

	// Create event bus for v2 command events (TUI subscribes via GetV2EventBus())
	eventBus := pubsub.NewBroker[any]()

	// Create middleware for command processing
	loggingMiddleware := processor.NewLoggingMiddleware(processor.LoggingMiddlewareConfig{})
	commandLogMiddleware := processor.NewCommandLogMiddleware(processor.CommandLogMiddlewareConfig{
		EventBus: &eventBusAdapter{broker: eventBus},
	})
	timeoutMiddleware := processor.NewTimeoutMiddleware(processor.TimeoutMiddlewareConfig{
		WarningThreshold: 500 * time.Millisecond,
	})

	// Create command processor with event bus for TUI event propagation
	cmdProcessor := processor.NewCommandProcessor(
		processor.WithQueueCapacity(1000),
		processor.WithTaskRepository(taskRepo),
		processor.WithQueueRepository(queueRepo),
		processor.WithEventBus(eventBus),
		processor.WithMiddleware(loggingMiddleware, commandLogMiddleware, timeoutMiddleware),
	)

	// Create unified ProcessRegistry for coordinator and workers
	processRegistry := process.NewProcessRegistry()

	// Create turn completion enforcer for tracking worker tool calls
	turnEnforcer := handler.NewTurnCompletionTracker()

	// Create BDTaskExecutor for syncing v2 state changes to BD tracker
	beadsExec := beads.NewRealExecutor(cfg.WorkDir)

	// Register all command handlers
	registerHandlers(cmdProcessor, processRepo, taskRepo, queueRepo, processRegistry, turnEnforcer,
		cfg.AIClient, cfg.Extensions, beadsExec, cfg.Port, eventBus, cfg.WorkDir)

	// Create command submitter adapter
	cmdSubmitter := handler.NewProcessorSubmitterAdapter(cmdProcessor)

	// Create V2Adapter with repositories for read-only operations
	v2Adapter := adapter.NewV2Adapter(cmdProcessor,
		adapter.WithProcessRepository(processRepo),
		adapter.WithTaskRepository(taskRepo),
		adapter.WithQueueRepository(queueRepo),
		adapter.WithMessageRepository(cfg.MessageRepo),
		adapter.WithSessionID(cfg.SessionID, cfg.WorkDir),
	)

	return &Infrastructure{
		Core: CoreComponents{
			Processor:    cmdProcessor,
			Adapter:      v2Adapter,
			EventBus:     eventBus,
			CmdSubmitter: cmdSubmitter,
		},
		Repositories: RepositoryComponents{
			ProcessRepo: processRepo,
			TaskRepo:    taskRepo,
			QueueRepo:   queueRepo,
		},
		Internal: InternalComponents{
			ProcessRegistry: processRegistry,
			TurnEnforcer:    turnEnforcer,
		},
		config: cfg,
	}, nil
}

// Start begins the command processor loop and waits for it to be ready.
// This must be called before submitting any commands.
func (i *Infrastructure) Start(ctx context.Context) error {
	// Start processor loop in background
	go i.Core.Processor.Run(ctx)

	// Wait for processor to be ready
	if err := i.Core.Processor.WaitForReady(ctx); err != nil {
		return fmt.Errorf("waiting for command processor: %w", err)
	}

	return nil
}

// Drain gracefully shuts down the command processor, processing all remaining
// commands in the queue before stopping.
func (i *Infrastructure) Drain() {
	if i.Core.Processor != nil {
		i.Core.Processor.Drain()
	}
}

// Shutdown stops all processes and drains the command processor.
// This is the recommended way to cleanly shut down the infrastructure.
func (i *Infrastructure) Shutdown() {
	// Stop all processes first (coordinator and workers)
	if i.Internal.ProcessRegistry != nil {
		i.Internal.ProcessRegistry.StopAll()
	}
	// Then drain processor to complete in-flight commands
	i.Drain()
}

// registerHandlers registers all command handlers with the command processor.
// This includes task assignment, state transition, BD task status, and process handlers.
//
// Handler groups:
//   - Task Assignment (4): AssignTask, AssignReview, ApproveCommit, AssignReviewFeedback
//   - State Transition (4): ReportComplete, ReportVerdict, TransitionPhase, ProcessTurnComplete
//   - BD Task Status (2): MarkTaskComplete, MarkTaskFailed
//   - Process Management (7): SpawnProcess, SendToProcess, DeliverProcessQueued,
//     RetireProcess, StopProcess, ReplaceProcess
func registerHandlers(
	cmdProcessor *processor.CommandProcessor,
	processRepo repository.ProcessRepository,
	taskRepo repository.TaskRepository,
	queueRepo repository.QueueRepository,
	processRegistry *process.ProcessRegistry,
	turnEnforcer handler.TurnCompletionEnforcer,
	aiClient client.HeadlessClient,
	extensions map[string]any,
	beadsExec beads.BeadsExecutor,
	port int,
	eventBus *pubsub.Broker[any],
	workDir string,
) {
	// Create shared infrastructure components
	cmdSubmitter := handler.NewProcessorSubmitterAdapter(cmdProcessor)

	// ============================================================
	// Task Assignment handlers (4)
	// ============================================================
	cmdProcessor.RegisterHandler(command.CmdAssignTask,
		handler.NewAssignTaskHandler(processRepo, taskRepo,
			handler.WithBDExecutor(beadsExec),
			handler.WithQueueRepository(queueRepo)))
	cmdProcessor.RegisterHandler(command.CmdAssignReview,
		handler.NewAssignReviewHandler(processRepo, taskRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdApproveCommit,
		handler.NewApproveCommitHandler(processRepo, taskRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdAssignReviewFeedback,
		handler.NewAssignReviewFeedbackHandler(processRepo, taskRepo, queueRepo))

	// ============================================================
	// State Transition handlers (4)
	// ============================================================
	cmdProcessor.RegisterHandler(command.CmdReportComplete,
		handler.NewReportCompleteHandler(processRepo, taskRepo, queueRepo,
			handler.WithReportCompleteBDExecutor(beadsExec)))
	cmdProcessor.RegisterHandler(command.CmdReportVerdict,
		handler.NewReportVerdictHandler(processRepo, taskRepo, queueRepo,
			handler.WithReportVerdictBDExecutor(beadsExec)))
	cmdProcessor.RegisterHandler(command.CmdTransitionPhase,
		handler.NewTransitionPhaseHandler(processRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdProcessTurnComplete,
		handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
			handler.WithProcessTurnEnforcer(turnEnforcer)))

	// ============================================================
	// BD Task Status handlers (2)
	// ============================================================
	cmdProcessor.RegisterHandler(command.CmdMarkTaskComplete,
		handler.NewMarkTaskCompleteHandler(beadsExec))
	cmdProcessor.RegisterHandler(command.CmdMarkTaskFailed,
		handler.NewMarkTaskFailedHandler(beadsExec))

	// ============================================================
	// Process Management handlers (7)
	// ============================================================

	// Create process spawner with the actual port
	processSpawner := handler.NewUnifiedProcessSpawner(handler.UnifiedSpawnerConfig{
		Client:     aiClient,
		WorkDir:    workDir,
		Port:       port,
		Extensions: extensions,
		Submitter:  cmdSubmitter,
		EventBus:   eventBus,
	})

	// MessageDeliverer for delivering messages to processes via session resume
	sessionProvider := handler.NewProcessRegistrySessionProvider(processRegistry, aiClient, workDir, port)
	messageDeliverer := integration.NewProcessSessionDeliverer(sessionProvider, aiClient, processRegistry)

	cmdProcessor.RegisterHandler(command.CmdSpawnProcess,
		handler.NewSpawnProcessHandler(processRepo, processRegistry,
			handler.WithUnifiedSpawner(processSpawner),
			handler.WithTurnEnforcer(turnEnforcer)))
	cmdProcessor.RegisterHandler(command.CmdSendToProcess,
		handler.NewSendToProcessHandler(processRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdDeliverProcessQueued,
		handler.NewDeliverProcessQueuedHandler(processRepo, queueRepo, processRegistry,
			handler.WithProcessDeliverer(messageDeliverer),
			handler.WithDeliverTurnEnforcer(turnEnforcer)))
	cmdProcessor.RegisterHandler(command.CmdRetireProcess,
		handler.NewRetireProcessHandler(processRepo, processRegistry,
			handler.WithRetireTurnEnforcer(turnEnforcer)))
	cmdProcessor.RegisterHandler(command.CmdStopProcess,
		handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, processRegistry))
	cmdProcessor.RegisterHandler(command.CmdReplaceProcess,
		handler.NewReplaceProcessHandler(processRepo, processRegistry,
			handler.WithReplaceSpawner(processSpawner)))

	// ============================================================
	// Aggregation handlers (1)
	// ============================================================
	cmdProcessor.RegisterHandler(command.CmdGenerateAccountabilitySummary,
		handler.NewGenerateAccountabilitySummaryHandler(processRepo, queueRepo))
}
