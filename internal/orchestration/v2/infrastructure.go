// Package v2 provides the factory for creating v2 orchestration infrastructure.
// The factory encapsulates all v2 component setup including repositories, command
// processor, handlers, and lifecycle management.
package v2

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"

	appbeads "github.com/zjrosen/perles/internal/beads/application"
	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	fabricsqlite "github.com/zjrosen/perles/internal/infrastructure/sqlite"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/fabric"
	domain "github.com/zjrosen/perles/internal/orchestration/fabric/domain"
	fabricrepo "github.com/zjrosen/perles/internal/orchestration/fabric/repository"
	"github.com/zjrosen/perles/internal/orchestration/tracing"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/integration"
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

// sessionDirProvider implements handler.SessionDirProvider.
// It wraps a static session directory path.
type sessionDirProvider struct {
	sessionDir string
}

// GetSessionDir returns the session directory path.
func (p *sessionDirProvider) GetSessionDir() string {
	return p.sessionDir
}

// FabricBackendConfig controls which backing store is used for Fabric thread
// and dependency repositories. When nil (or DB is nil), repositories use the
// default in-memory implementation. When DB is non-nil, SQLite-backed
// repositories are available, controlled by the feature flags.
type FabricBackendConfig struct {
	// DB is the SQLite database connection. Nil means use memory backend only.
	DB *sql.DB
	// SessionID scopes SQLite repositories to a single orchestration session.
	// Required when DB is non-nil.
	SessionID string
	// DualWriteEnabled causes writes to propagate to both memory and SQLite
	// backends simultaneously. Reads come from whichever backend SQLiteReadEnabled selects.
	DualWriteEnabled bool
	// SQLiteReadEnabled selects SQLite as the read path for thread and dependency
	// queries. When false (even with DualWriteEnabled), reads come from memory.
	SQLiteReadEnabled bool
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
	// CommandPersistenceProvider returns the current CommandWriter for persisting commands.
	// Optional - if nil, commands are not persisted to commands.jsonl.
	CommandPersistenceProvider func() processor.CommandWriter
	// FabricBackend controls the backing store for Fabric thread and dependency repositories.
	// Optional - if nil, uses in-memory repositories (current default behavior).
	FabricBackend *FabricBackendConfig
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
	// FabricService provides the Fabric messaging layer for inter-agent communication.
	// Used by MCP servers to expose fabric_* tools to coordinator and workers.
	FabricService *fabric.Service
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

	// Get observer client and extensions (Observer() falls back to worker if not set)
	observerClient, err := cfg.AgentProviders.Observer().Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get observer client: %w", err)
	}
	observerExtensions := cfg.AgentProviders.Observer().Extensions()

	// Create repositories
	taskRepo := repository.NewMemoryTaskRepository()
	queueRepo := repository.NewMemoryQueueRepository(repository.DefaultQueueMaxSize)
	processRepo := repository.NewMemoryProcessRepository()

	// Create Fabric messaging layer repositories and service
	// Fabric provides graph-based messaging ("Slack for Agents") with channels, threads, and artifacts.
	fabricThreads, fabricDeps := createFabricRepositories(cfg.FabricBackend)
	fabricSubs := fabricrepo.NewMemorySubscriptionRepository()
	fabricAcks := fabricrepo.NewMemoryAckRepository(fabricDeps, fabricThreads, fabricSubs)
	fabricParticipants := fabricrepo.NewMemoryParticipantRepository()
	// Wire participant repo to ack repo for @here inbox expansion
	fabricAcks.SetParticipantRepository(fabricParticipants)
	fabricService := fabric.NewService(fabricThreads, fabricDeps, fabricSubs, fabricAcks, fabricParticipants)

	// Create event bus for v2 command events (TUI subscribes via GetV2EventBus())
	eventBus := pubsub.NewBroker[any]()

	// Create middleware for command processing
	loggingMiddleware := processor.NewLoggingMiddleware(processor.LoggingMiddlewareConfig{})
	commandLogMiddleware := processor.NewCommandLogMiddleware(processor.CommandLogMiddlewareConfig{
		EventBus: &eventBusAdapter{broker: eventBus},
	})
	commandPersistenceMiddleware := processor.NewCommandPersistenceMiddleware(processor.CommandPersistenceMiddlewareConfig{
		WriterProvider: cfg.CommandPersistenceProvider,
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
		processor.WithMiddleware(tracingMiddleware, loggingMiddleware, commandLogMiddleware, commandPersistenceMiddleware, timeoutMiddleware),
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
		observerClient,
		coordinatorExtensions,
		workerExtensions,
		observerExtensions,
		beadsExec,
		cfg.Port,
		eventBus,
		cfg.WorkDir,
		cfg.BeadsDir,
		cfg.SessionDir,
		cfg.Tracer,
		cfg.SessionRefNotifier,
		cfg.SoundService,
		cfg.SessionMetadataProvider,
		cfg.WorkflowStateProvider,
		fabricService,
	)

	// Create command submitter adapter
	cmdSubmitter := handler.NewProcessorSubmitterAdapter(cmdProcessor)

	// Create V2Adapter with repositories for read-only operations
	v2Adapter := adapter.NewV2Adapter(cmdProcessor,
		adapter.WithProcessRepository(processRepo),
		adapter.WithTaskRepository(taskRepo),
		adapter.WithQueueRepository(queueRepo),
		adapter.WithSessionID(cfg.SessionID, cfg.WorkDir, cfg.SessionDir),
	)

	// NOTE: CoordinatorNudger removed - FabricBroker handles @mention notifications

	return &Infrastructure{
		Core: CoreComponents{
			Processor:     cmdProcessor,
			Adapter:       v2Adapter,
			EventBus:      eventBus,
			CmdSubmitter:  cmdSubmitter,
			FabricService: fabricService,
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

	// Initialize Fabric session with fixed channels (system, tasks, planning, general)
	// The coordinator will be the creator of the channel structure.
	// Use lowercase CoordinatorID to match process IDs for subscription matching.
	if i.Core.FabricService != nil {
		if err := i.Core.FabricService.InitSession(repository.CoordinatorID); err != nil {
			return fmt.Errorf("initializing fabric session: %w", err)
		}
	}

	// NOTE: CoordinatorNudger.Start() removed - FabricBroker.Start() is called by Supervisor

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
// NOTE: FabricBroker.Stop() is called by Supervisor before this.
func (i *Infrastructure) Shutdown() {
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
	observerClient client.HeadlessClient,
	coordinatorExtensions map[string]any,
	workerExtensions map[string]any,
	observerExtensions map[string]any,
	beadsExec appbeads.IssueExecutor,
	port int,
	eventBus *pubsub.Broker[any],
	workDir string,
	beadsDir string,
	sessionDir string,
	tracer trace.Tracer,
	sessionRefNotifier handler.SessionRefNotifier,
	soundService sound.SoundService,
	sessionMetadataProvider handler.SessionMetadataProvider,
	workflowStateProvider handler.WorkflowStateProvider,
	fabricService *fabric.Service,
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
		handler.NewMarkTaskCompleteHandler(beadsExec, taskRepo,
			handler.WithMarkTaskCompleteProcessRepo(processRepo)))
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
		SessionDir:            sessionDir,
	})

	// MessageDeliverer for delivering messages to processes via session resume
	// Uses role-based client selection (coordinator vs worker vs observer)
	sessionProvider := handler.NewProcessRegistrySessionProvider(processRegistry, coordinatorClient, workerClient, observerClient, workDir, port)

	messageDeliverer := integration.NewProcessSessionDeliverer(
		sessionProvider,
		coordinatorClient,
		workerClient,
		observerClient,
		processRegistry,
		coordinatorExtensions,
		workerExtensions,
		observerExtensions,
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
		handler.NewStopWorkerHandler(processRepo, taskRepo, queueRepo, processRegistry,
			handler.WithFabricUnsubscriber(fabricService)))
	cmdProcessor.RegisterHandler(command.CmdReplaceProcess,
		handler.NewReplaceProcessHandler(processRepo, processRegistry,
			handler.WithReplaceSpawner(processSpawner),
			handler.WithWorkflowStateProvider(workflowStateProvider),
			handler.WithSessionDirProvider(&sessionDirProvider{sessionDir: sessionDir})))
	cmdProcessor.RegisterHandler(command.CmdPauseProcess,
		handler.NewPauseProcessHandler(processRepo,
			handler.WithPauseRegistry(processRegistry)))
	cmdProcessor.RegisterHandler(command.CmdResumeProcess,
		handler.NewResumeProcessHandler(processRepo, queueRepo))

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

// createFabricRepositories constructs thread and dependency repositories based
// on the backend configuration. When cfg is nil or cfg.DB is nil, in-memory
// repositories are returned (preserving current default behavior). When a DB
// is provided, SQLite-backed repositories are available, gated by feature flags.
//
// Subscriptions, acks, and participants always remain in-memory (Phase 1 scope).
func createFabricRepositories(cfg *FabricBackendConfig) (fabricrepo.ThreadRepository, fabricrepo.DependencyRepository) {
	// Default: memory-only (backward compatible, zero behavior change)
	if cfg == nil || cfg.DB == nil {
		return fabricrepo.NewMemoryThreadRepository(), fabricrepo.NewMemoryDependencyRepository()
	}

	memThreads := fabricrepo.NewMemoryThreadRepository()
	memDeps := fabricrepo.NewMemoryDependencyRepository()

	sqliteThreads := fabricsqlite.NewSQLiteThreadRepository(cfg.DB, cfg.SessionID)
	sqliteDeps := fabricsqlite.NewSQLiteDependencyRepository(cfg.DB, cfg.SessionID)

	if cfg.DualWriteEnabled {
		// Dual-write: writes go to both backends, reads from selected backend
		var readThreads fabricrepo.ThreadRepository = memThreads
		var readDeps fabricrepo.DependencyRepository = memDeps
		if cfg.SQLiteReadEnabled {
			readThreads = sqliteThreads
			readDeps = sqliteDeps
		}
		return &dualWriteThreadRepository{
				primary:   readThreads,
				secondary: secondaryThreadRepo(readThreads, memThreads, sqliteThreads),
			}, &dualWriteDependencyRepository{
				primary:   readDeps,
				secondary: secondaryDepRepo(readDeps, memDeps, sqliteDeps),
			}
	}

	// Non-dual-write: use the selected backend directly
	if cfg.SQLiteReadEnabled {
		return sqliteThreads, sqliteDeps
	}
	return memThreads, memDeps
}

// secondaryThreadRepo returns the backend that is NOT the primary read source.
func secondaryThreadRepo(primary fabricrepo.ThreadRepository, mem, sqlite fabricrepo.ThreadRepository) fabricrepo.ThreadRepository {
	if primary == mem {
		return sqlite
	}
	return mem
}

// secondaryDepRepo returns the backend that is NOT the primary read source.
func secondaryDepRepo(primary fabricrepo.DependencyRepository, mem, sqlite fabricrepo.DependencyRepository) fabricrepo.DependencyRepository {
	if primary == mem {
		return sqlite
	}
	return mem
}

// dualWriteThreadRepository writes to both primary and secondary backends.
// Reads are served exclusively from the primary backend.
type dualWriteThreadRepository struct {
	primary   fabricrepo.ThreadRepository
	secondary fabricrepo.ThreadRepository
}

func (d *dualWriteThreadRepository) Create(thread domain.Thread) (*domain.Thread, error) {
	result, err := d.primary.Create(thread)
	if err != nil {
		return nil, err
	}
	// Best-effort write to secondary with the primary's auto-generated ID/Seq
	// so both backends store the same thread identity.
	_, _ = d.secondary.Create(*result)
	return result, nil
}

func (d *dualWriteThreadRepository) Get(id string) (*domain.Thread, error) {
	return d.primary.Get(id)
}

func (d *dualWriteThreadRepository) GetBySlug(slug string) (*domain.Thread, error) {
	return d.primary.GetBySlug(slug)
}

func (d *dualWriteThreadRepository) List(opts fabricrepo.ListOptions) ([]domain.Thread, error) {
	return d.primary.List(opts)
}

func (d *dualWriteThreadRepository) Update(thread domain.Thread) (*domain.Thread, error) {
	result, err := d.primary.Update(thread)
	if err != nil {
		return nil, err
	}
	_, _ = d.secondary.Update(*result)
	return result, nil
}

func (d *dualWriteThreadRepository) Archive(id string) error {
	err := d.primary.Archive(id)
	if err != nil {
		return err
	}
	_ = d.secondary.Archive(id)
	return nil
}

// dualWriteDependencyRepository writes to both primary and secondary backends.
// Reads are served exclusively from the primary backend.
type dualWriteDependencyRepository struct {
	primary   fabricrepo.DependencyRepository
	secondary fabricrepo.DependencyRepository
}

func (d *dualWriteDependencyRepository) Add(dep domain.Dependency) error {
	err := d.primary.Add(dep)
	if err != nil {
		return err
	}
	_ = d.secondary.Add(dep)
	return nil
}

func (d *dualWriteDependencyRepository) Remove(threadID, dependsOnID string) error {
	err := d.primary.Remove(threadID, dependsOnID)
	if err != nil {
		return err
	}
	_ = d.secondary.Remove(threadID, dependsOnID)
	return nil
}

func (d *dualWriteDependencyRepository) GetParents(threadID string, relation *domain.RelationType) ([]domain.Dependency, error) {
	return d.primary.GetParents(threadID, relation)
}

func (d *dualWriteDependencyRepository) GetChildren(threadID string, relation *domain.RelationType) ([]domain.Dependency, error) {
	return d.primary.GetChildren(threadID, relation)
}

func (d *dualWriteDependencyRepository) GetRoots() ([]string, error) {
	return d.primary.GetRoots()
}
