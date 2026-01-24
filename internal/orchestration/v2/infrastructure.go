// Package v2 provides the factory for creating v2 orchestration infrastructure.
// The factory encapsulates all v2 component setup including repositories, command
// processor, handlers, and lifecycle management.
package v2

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/tracing"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/integration"
	"github.com/zjrosen/perles/internal/orchestration/v2/nudger"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/sound"
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
	// AgentProviders maps roles to their AI client providers.
	// Must contain at least RoleCoordinator. RoleWorker falls back to coordinator if not set.
	AgentProviders client.AgentProviders
	// WorkDir is the working directory for the orchestration session.
	WorkDir string
	// BeadsDir is the path to the beads database directory.
	// When set, spawned processes receive BEADS_DIR environment variable.
	BeadsDir string
	// MessageRepo is the message repository for inter-agent messaging.
	MessageRepo repository.MessageRepository
	// SessionID is the session identifier for accountability summary generation.
	SessionID string
	// SessionDir is the directory where session files are stored.
	// For centralized storage: ~/.perles/sessions/{app}/{date}/{id}/
	SessionDir string
	// Tracer is the OpenTelemetry tracer for distributed tracing (optional).
	// When provided, TracingMiddleware will be registered in the command processor.
	Tracer trace.Tracer
	// SessionRefNotifier is called when a process's session reference is captured.
	// Used to persist session refs for crash-resilient session resumption.
	// Optional - if nil, session ref capture is skipped.
	SessionRefNotifier handler.SessionRefNotifier
	// SoundService provides audio feedback for orchestration events.
	// Optional - if nil, uses NoopSoundService (no audio).
	SoundService sound.SoundService
	// SessionMetadataProvider provides access to session metadata for workflow completion.
	// Optional - if nil, workflow completion status is not persisted to session metadata.
	SessionMetadataProvider handler.SessionMetadataProvider
	// WorkflowStateProvider provides workflow state for coordinator replacement.
	// Optional - if nil, auto-refresh uses standard replace prompt instead of workflow continuation.
	WorkflowStateProvider handler.WorkflowStateProvider
	// NudgeDebounce is the debounce duration for coordinator nudges.
	// Defaults to nudger.DefaultDebounce (1 second) if zero.
	NudgeDebounce time.Duration
}

// Validate checks that all required configuration is provided.
func (c *InfrastructureConfig) Validate() error {
	if c.Port == 0 {
		return fmt.Errorf("port is required")
	}
	if c.AgentProviders == nil {
		return fmt.Errorf("AgentProviders is required")
	}
	if _, ok := c.AgentProviders[client.RoleCoordinator]; !ok {
		return fmt.Errorf("AgentProviders must contain RoleCoordinator")
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
	// CoordinatorNudger batches worker messages and sends consolidated nudges.
	CoordinatorNudger *nudger.CoordinatorNudger
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

	// Get coordinator client and extensions
	coordinatorClient, err := cfg.AgentProviders.Coordinator().Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get coordinator client: %w", err)
	}
	coordinatorExtensions := cfg.AgentProviders.Coordinator().Extensions()

	// Get worker client and extensions (Worker() falls back to coordinator if not set)
	workerClient, err := cfg.AgentProviders.Worker().Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get worker client: %w", err)
	}
	workerExtensions := cfg.AgentProviders.Worker().Extensions()

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
	tracingMiddleware := tracing.NewTracingMiddleware(tracing.TracingMiddlewareConfig{
		Tracer: cfg.Tracer,
	})

	// Create command processor with event bus for TUI event propagation
	cmdProcessor := processor.NewCommandProcessor(
		processor.WithQueueCapacity(1000),
		processor.WithTaskRepository(taskRepo),
		processor.WithQueueRepository(queueRepo),
		processor.WithEventBus(eventBus),
		processor.WithMiddleware(tracingMiddleware, loggingMiddleware, commandLogMiddleware, timeoutMiddleware),
	)

	// Create unified ProcessRegistry for coordinator and workers
	processRegistry := process.NewProcessRegistry()

	// Create turn completion enforcer for tracking worker tool calls
	turnEnforcer := handler.NewTurnCompletionTracker()

	// Create BDTaskExecutor for syncing v2 state changes to BD tracker
	beadsExec := infrabeads.NewBDExecutor(cfg.WorkDir, cfg.BeadsDir)

	// Register all command handlers
	registerHandlers(
		cmdProcessor,
		processRepo,
		taskRepo,
		queueRepo,
		processRegistry,
		turnEnforcer,
		coordinatorClient,
		workerClient,
		coordinatorExtensions,
		workerExtensions,
		beadsExec,
		cfg.Port,
		eventBus,
		cfg.WorkDir,
		cfg.BeadsDir,
		cfg.Tracer,
		cfg.SessionRefNotifier,
		cfg.SoundService,
		cfg.SessionMetadataProvider,
		cfg.WorkflowStateProvider,
	)

	// Create command submitter adapter
	cmdSubmitter := handler.NewProcessorSubmitterAdapter(cmdProcessor)

	// Create V2Adapter with repositories for read-only operations
	v2Adapter := adapter.NewV2Adapter(cmdProcessor,
		adapter.WithProcessRepository(processRepo),
		adapter.WithTaskRepository(taskRepo),
		adapter.WithQueueRepository(queueRepo),
		adapter.WithMessageRepository(cfg.MessageRepo),
		adapter.WithSessionID(cfg.SessionID, cfg.WorkDir, cfg.SessionDir),
	)

	// Create CoordinatorNudger for batching worker messages
	coordNudger := nudger.New(nudger.Config{
		Debounce:     cfg.NudgeDebounce, // Uses nudger.DefaultDebounce if zero
		MsgBroker:    cfg.MessageRepo.Broker(),
		CmdSubmitter: cmdSubmitter,
	})

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
			ProcessRegistry:   processRegistry,
			TurnEnforcer:      turnEnforcer,
			CoordinatorNudger: coordNudger,
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

	// Start coordinator nudger after processor is ready
	// (nudger submits commands, so processor must be running first)
	if i.Internal.CoordinatorNudger != nil {
		i.Internal.CoordinatorNudger.Start()
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
	// Stop nudger first (it may submit final commands that need processing)
	if i.Internal.CoordinatorNudger != nil {
		i.Internal.CoordinatorNudger.Stop()
	}
	// Stop all processes (coordinator and workers)
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
	coordinatorClient client.HeadlessClient,
	workerClient client.HeadlessClient,
	coordinatorExtensions map[string]any,
	workerExtensions map[string]any,
	beadsExec appbeads.IssueExecutor,
	port int,
	eventBus *pubsub.Broker[any],
	workDir string,
	beadsDir string,
	tracer trace.Tracer,
	sessionRefNotifier handler.SessionRefNotifier,
	soundService sound.SoundService,
	sessionMetadataProvider handler.SessionMetadataProvider,
	workflowStateProvider handler.WorkflowStateProvider,
) {
	// Create shared infrastructure components
	cmdSubmitter := handler.NewProcessorSubmitterAdapter(cmdProcessor)

	// Use NoopSoundService if none provided
	if soundService == nil {
		soundService = sound.NoopSoundService{}
	}

	// ============================================================
	// Task Assignment handlers (4)
	// ============================================================
	cmdProcessor.RegisterHandler(command.CmdAssignTask,
		handler.NewAssignTaskHandler(processRepo, taskRepo,
			handler.WithBDExecutor(beadsExec),
			handler.WithQueueRepository(queueRepo),
			handler.WithAssignTaskTracer(tracer)))
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
			handler.WithReportVerdictBDExecutor(beadsExec),
			handler.WithReportVerdictTracer(tracer),
			handler.WithReportVerdictSoundService(soundService)))
	cmdProcessor.RegisterHandler(command.CmdTransitionPhase,
		handler.NewTransitionPhaseHandler(processRepo, queueRepo))
	cmdProcessor.RegisterHandler(command.CmdProcessTurnComplete,
		handler.NewProcessTurnCompleteHandler(processRepo, queueRepo,
			handler.WithProcessTurnEnforcer(turnEnforcer),
			handler.WithTurnCompleteProcessRegistry(processRegistry),
			handler.WithSessionRefNotifier(sessionRefNotifier),
			handler.WithProcessTurnSoundService(soundService)))

	// ============================================================
	// BD Task Status handlers (2)
	// ============================================================
	cmdProcessor.RegisterHandler(command.CmdMarkTaskComplete,
		handler.NewMarkTaskCompleteHandler(beadsExec, taskRepo))
	cmdProcessor.RegisterHandler(command.CmdMarkTaskFailed,
		handler.NewMarkTaskFailedHandler(beadsExec))

	// ============================================================
	// Process Management handlers (7)
	// ============================================================

	// Create process spawner with separate coordinator/worker clients
	processSpawner := handler.NewUnifiedProcessSpawner(handler.UnifiedSpawnerConfig{
		CoordinatorClient:     coordinatorClient,
		WorkerClient:          workerClient,
		CoordinatorExtensions: coordinatorExtensions,
		WorkerExtensions:      workerExtensions,
		WorkDir:               workDir,
		Port:                  port,
		Submitter:             cmdSubmitter,
		EventBus:              eventBus,
		BeadsDir:              beadsDir,
	})

	// MessageDeliverer for delivering messages to processes via session resume
	// Uses role-based client selection (coordinator vs worker)
	sessionProvider := handler.NewProcessRegistrySessionProvider(processRegistry, coordinatorClient, workerClient, workDir, port)

	messageDeliverer := integration.NewProcessSessionDeliverer(
		sessionProvider,
		coordinatorClient,
		workerClient,
		processRegistry,
		coordinatorExtensions,
		workerExtensions,
		integration.WithBeadsDir(beadsDir),
	)

	cmdProcessor.RegisterHandler(command.CmdSpawnProcess,
		handler.NewSpawnProcessHandler(processRepo, processRegistry,
			handler.WithUnifiedSpawner(processSpawner),
			handler.WithTurnEnforcer(turnEnforcer),
			handler.WithSpawnProcessTracer(tracer)))
	cmdProcessor.RegisterHandler(command.CmdSendToProcess,
		handler.NewSendToProcessHandler(processRepo, queueRepo,
			handler.WithSendToProcessTracer(tracer)))
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
			handler.WithReplaceSpawner(processSpawner),
			handler.WithWorkflowStateProvider(workflowStateProvider)))

	// ============================================================
	// Aggregation handlers (1)
	// ============================================================
	cmdProcessor.RegisterHandler(command.CmdGenerateAccountabilitySummary,
		handler.NewGenerateAccountabilitySummaryHandler(processRepo, queueRepo))

	// ============================================================
	// Workflow Completion handlers (1)
	// ============================================================
	cmdProcessor.RegisterHandler(command.CmdSignalWorkflowComplete,
		handler.NewSignalWorkflowCompleteHandler(
			handler.WithSessionMetadataProvider(sessionMetadataProvider),
			handler.WithWorkflowSoundService(soundService)))

	// ============================================================
	// User Interaction handlers (1)
	// ============================================================
	cmdProcessor.RegisterHandler(command.CmdNotifyUser,
		handler.NewNotifyUserHandler(
			handler.WithNotifyUserSoundService(soundService)))
}
