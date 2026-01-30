// Package controlplane provides Supervisor for managing workflow lifecycle operations.
package controlplane

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/flags"
	appgit "github.com/zjrosen/perles/internal/git/application"
	domaingit "github.com/zjrosen/perles/internal/git/domain"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/fabric"
	fabricpersist "github.com/zjrosen/perles/internal/orchestration/fabric/persistence"
	"github.com/zjrosen/perles/internal/orchestration/mcp"
	"github.com/zjrosen/perles/internal/orchestration/session"
	v2 "github.com/zjrosen/perles/internal/orchestration/v2"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt/roles"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/orchestration/workflow"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/sound"
)

// ErrInvalidState is returned when a workflow is in an invalid state for the operation.
var ErrInvalidState = fmt.Errorf("invalid workflow state")

// StopOptions configures the behavior of the Stop operation.
type StopOptions struct {
	// Reason describes why the workflow is being stopped.
	Reason string
	// GracePeriod is the time to wait for graceful shutdown before forcing.
	GracePeriod time.Duration
	// Force skips graceful shutdown and immediately terminates.
	Force bool
}

// Supervisor handles workflow lifecycle operations.
// It manages starting, pausing, resuming, and stopping workflow instances.
type Supervisor interface {
	// AllocateResources prepares a workflow for execution.
	// Creates infrastructure, MCP server, session, and stores resources on the instance.
	// After this call succeeds, inst.Infrastructure is set and event buses can be attached.
	// Returns an error if the workflow is not in Pending state.
	AllocateResources(ctx context.Context, inst *WorkflowInstance) error

	// SpawnCoordinator spawns the coordinator process for a workflow.
	// Must be called after AllocateResources. Transitions the workflow to Running state.
	// Returns an error if resources have not been allocated (inst.Infrastructure is nil).
	SpawnCoordinator(ctx context.Context, inst *WorkflowInstance) error

	// Pause suspends a running workflow, stopping all processes and clearing queues.
	// The infrastructure remains allocated for potential resumption.
	// Returns ErrInvalidState if the workflow is not in Running state.
	Pause(ctx context.Context, inst *WorkflowInstance) error

	// Resume restarts a paused workflow by respawning the coordinator.
	// Sends a system message to the coordinator with pause context.
	// Returns ErrInvalidState if the workflow is not in Paused state.
	Resume(ctx context.Context, inst *WorkflowInstance) error

	// Shutdown terminates a workflow and releases all resources.
	// Drains the command processor, finalizes the session, and releases leases.
	// Can be called on any active workflow (Running, Paused, Pending).
	Shutdown(ctx context.Context, inst *WorkflowInstance, opts StopOptions) error
}

// InfrastructureFactory creates v2.Infrastructure instances.
// This interface enables testing by allowing mock implementations.
type InfrastructureFactory interface {
	// Create creates a new infrastructure with the given configuration.
	Create(cfg v2.InfrastructureConfig) (*v2.Infrastructure, error)
}

// DefaultInfrastructureFactory is the production implementation that uses v2.NewInfrastructure.
type DefaultInfrastructureFactory struct{}

// Create implements InfrastructureFactory.
func (f *DefaultInfrastructureFactory) Create(cfg v2.InfrastructureConfig) (*v2.Infrastructure, error) {
	return v2.NewInfrastructure(cfg)
}

// ListenerFactory creates net.Listener instances for MCP HTTP servers.
// This interface enables testing by allowing mock implementations.
type ListenerFactory interface {
	// Create creates a TCP listener on the given address.
	Create(address string) (net.Listener, error)
}

// DefaultListenerFactory is the production implementation that uses net.Listen.
type DefaultListenerFactory struct{}

// Create implements ListenerFactory.
func (f *DefaultListenerFactory) Create(address string) (net.Listener, error) {
	return net.Listen("tcp", address)
}

// Default values for worktree configuration.
const (
	// DefaultWorktreeTimeout is the default timeout for worktree creation operations.
	// This matches the single-workflow orchestration mode timeout.
	DefaultWorktreeTimeout = 30 * time.Second
)

// SupervisorConfig configures the Supervisor.
type SupervisorConfig struct {
	// AgentProviders maps roles to their AI client providers.
	// Must contain at least RoleCoordinator. RoleWorker falls back to coordinator if not set.
	AgentProviders client.AgentProviders
	// InfrastructureFactory creates v2 infrastructure instances.
	// If nil, DefaultInfrastructureFactory is used.
	InfrastructureFactory InfrastructureFactory
	// ListenerFactory creates TCP listeners for MCP HTTP servers.
	// If nil, DefaultListenerFactory is used.
	ListenerFactory ListenerFactory
	// WorkflowRegistry provides access to workflow templates.
	// If nil, template content will not be prepended to the initial goal.
	WorkflowRegistry *workflow.Registry

	// GitExecutorFactory creates GitExecutor instances for worktree operations.
	// The factory receives the working directory path and returns a GitExecutor.
	// If nil, worktree creation is disabled even when WorktreeEnabled is true.
	GitExecutorFactory func(workDir string) appgit.GitExecutor

	// WorktreeTimeout is the timeout for worktree creation operations.
	// If zero, defaults to DefaultWorktreeTimeout (30s).
	WorktreeTimeout time.Duration

	// Flags provides access to feature flags.
	// If nil, flag-dependent behavior uses safe defaults.
	Flags *flags.Registry

	// SessionFactory creates session instances for workflow tracking.
	// Required - sessions persist workflow logs to ~/.perles/sessions/.
	SessionFactory *session.Factory

	// SoundService provides audio feedback for orchestration events.
	// Optional - if nil, uses NoopSoundService (no audio).
	SoundService sound.SoundService

	// BeadsDir is the resolved path to the beads database directory.
	// When set, spawned processes receive BEADS_DIR environment variable.
	BeadsDir string
}

// defaultSupervisor is the default implementation of Supervisor.
type defaultSupervisor struct {
	agentProviders        client.AgentProviders
	infrastructureFactory InfrastructureFactory
	listenerFactory       ListenerFactory
	workflowRegistry      *workflow.Registry
	gitExecutorFactory    func(workDir string) appgit.GitExecutor
	worktreeTimeout       time.Duration
	flags                 *flags.Registry
	sessionFactory        *session.Factory
	soundService          sound.SoundService
	beadsDir              string
}

// NewSupervisor creates a new Supervisor with the given configuration.
func NewSupervisor(cfg SupervisorConfig) (Supervisor, error) {
	// Validate AgentProviders
	if cfg.AgentProviders == nil {
		return nil, fmt.Errorf("AgentProviders is required")
	}
	if _, ok := cfg.AgentProviders[client.RoleCoordinator]; !ok {
		return nil, fmt.Errorf("AgentProviders must contain RoleCoordinator")
	}
	if cfg.SessionFactory == nil {
		return nil, fmt.Errorf("SessionFactory is required")
	}

	infraFactory := cfg.InfrastructureFactory
	if infraFactory == nil {
		infraFactory = &DefaultInfrastructureFactory{}
	}

	listenerFactory := cfg.ListenerFactory
	if listenerFactory == nil {
		listenerFactory = &DefaultListenerFactory{}
	}

	// Apply default values for worktree configuration
	worktreeTimeout := cfg.WorktreeTimeout
	if worktreeTimeout == 0 {
		worktreeTimeout = DefaultWorktreeTimeout
	}

	return &defaultSupervisor{
		agentProviders:        cfg.AgentProviders,
		infrastructureFactory: infraFactory,
		listenerFactory:       listenerFactory,
		workflowRegistry:      cfg.WorkflowRegistry,
		gitExecutorFactory:    cfg.GitExecutorFactory,
		worktreeTimeout:       worktreeTimeout,
		flags:                 cfg.Flags,
		sessionFactory:        cfg.SessionFactory,
		soundService:          cfg.SoundService,
		beadsDir:              cfg.BeadsDir,
	}, nil
}

// AllocateResources prepares a workflow for execution.
// Creates infrastructure, MCP server, session, and stores resources on the instance.
// After this call succeeds, inst.Infrastructure is set and event buses can be attached.
//
// This method accepts workflows in Pending state (new workflows) or Paused state (cold resume).
// For cold resume (Paused state with SessionDir set), it reopens the existing session directory
// instead of creating a new one, preserving message history and coordinator session refs.
func (s *defaultSupervisor) AllocateResources(ctx context.Context, inst *WorkflowInstance) error {
	// Validate state is Pending (new workflow) or Paused (cold resume from SQLite)
	if inst.State != WorkflowPending && inst.State != WorkflowPaused {
		return fmt.Errorf("%w: cannot allocate resources for workflow in state %s, expected %s or %s",
			ErrInvalidState, inst.State, WorkflowPending, WorkflowPaused)
	}

	// Detect cold resume: Paused state with existing SessionDir
	coldResume := inst.State == WorkflowPaused && inst.SessionDir != ""

	// Create a cancellable context for this workflow's lifecycle.
	// IMPORTANT: Use context.Background() instead of ctx to ensure the workflow
	// continues running after the API request completes. The passed ctx may be
	// an HTTP request context that gets cancelled when the request returns.
	// The workflow's lifecycle is managed via the cancel function stored in inst.Cancel.
	workflowCtx, cancel := context.WithCancel(context.Background())

	// Track resources for cleanup on error
	var (
		infra        *v2.Infrastructure
		httpServer   *http.Server
		listener     net.Listener
		worktreePath string
		gitExec      appgit.GitExecutor
		sess         *session.Session
	)

	// Cleanup function for error cases
	cleanup := func() {
		if httpServer != nil {
			_ = httpServer.Close()
		}
		if listener != nil {
			_ = listener.Close()
		}
		if infra != nil {
			infra.Shutdown()
		}
		// Close session to release file handles
		if sess != nil {
			_ = sess.Close(session.StatusFailed)
		}
		// Clean up worktree if it was created
		if worktreePath != "" && gitExec != nil {
			_ = gitExec.RemoveWorktree(worktreePath)
		}
		cancel()
	}

	// Step 0: Create worktree if enabled (fail fast - before any other resources)
	// For cold resume, skip worktree creation if WorktreePath is already set and exists.
	if inst.WorktreeEnabled && s.gitExecutorFactory != nil {
		// Check if worktree already exists (cold resume case)
		worktreeExists := false
		if inst.WorktreePath != "" {
			if info, err := os.Stat(inst.WorktreePath); err == nil && info.IsDir() {
				worktreeExists = true
				log.Debug(log.CatOrch, "Using existing worktree for cold resume", "subsystem", "supervisor",
					"workflowID", inst.ID, "path", inst.WorktreePath, "branch", inst.WorktreeBranch)
			}
		}

		if !worktreeExists {
			// Determine the work directory to use for git operations
			workDir := inst.WorkDir
			if workDir == "" {
				if wd, err := os.Getwd(); err == nil {
					workDir = wd
				}
			}

			// Create GitExecutor for the work directory
			gitExec = s.gitExecutorFactory(workDir)

			// Prune stale worktree references
			_ = gitExec.PruneWorktrees() // Best-effort, don't fail on prune errors

			// Determine worktree path
			path, err := gitExec.DetermineWorktreePath(inst.ID.String())
			if err != nil {
				cancel()
				return fmt.Errorf("determining worktree path: %w", err)
			}

			// Generate branch name: use custom if provided, otherwise auto-generate
			branchName := inst.WorktreeBranchName
			if branchName == "" {
				// Auto-generate branch name using first 8 chars of workflow ID
				shortID := inst.ID.String()
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				branchName = fmt.Sprintf("perles-workflow-%s", shortID)
			}

			// Create worktree with timeout context
			worktreeCtx, worktreeCancel := context.WithTimeout(ctx, s.worktreeTimeout)
			err = gitExec.CreateWorktreeWithContext(worktreeCtx, path, branchName, inst.WorktreeBaseBranch)
			worktreeCancel()

			if err != nil {
				cancel()
				// Wrap known error types for user-friendly messages
				if errors.Is(err, domaingit.ErrBranchAlreadyCheckedOut) {
					return fmt.Errorf("creating worktree: branch '%s' is already checked out in another worktree: %w", branchName, err)
				}
				if errors.Is(err, domaingit.ErrPathAlreadyExists) {
					return fmt.Errorf("creating worktree: path '%s' already exists: %w", path, err)
				}
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, domaingit.ErrWorktreeTimeout) {
					return fmt.Errorf("creating worktree: operation timed out after %v: %w", s.worktreeTimeout, err)
				}
				return fmt.Errorf("creating worktree: %w", err)
			}

			// Update instance fields with worktree information
			worktreePath = path // Track for cleanup on subsequent failure
			inst.WorktreePath = path
			inst.WorktreeBranch = branchName
			inst.WorkDir = path // Override WorkDir to use the worktree path

			log.Debug(log.CatOrch, "Worktree created", "subsystem", "supervisor",
				"workflowID", inst.ID, "path", path, "branch", branchName)
		}
	}

	// Step 1: Create TCP listener for MCP HTTP server (OS assigns available port)
	var err error
	listener, err = s.listenerFactory.Create("127.0.0.1:0")
	if err != nil {
		cancel()
		return fmt.Errorf("creating MCP listener: %w", err)
	}

	// Get the actual port assigned by the OS
	port := listener.Addr().(*net.TCPAddr).Port
	log.Debug(log.CatOrch, "MCP listener created", "subsystem", "supervisor", "port", port, "workflowID", inst.ID)

	// Step 3: Create or reopen session for this workflow
	workDir := getWorkDir(inst)
	if coldResume && inst.SessionDir != "" {
		// Cold resume: reopen existing session directory to preserve message history
		sess, err = session.Reopen(inst.ID.String(), inst.SessionDir)
		if err != nil {
			cleanup()
			return fmt.Errorf("reopening session for cold resume: %w", err)
		}
		log.Debug(log.CatOrch, "Session reopened for cold resume", "subsystem", "supervisor",
			"workflowID", inst.ID, "sessionDir", sess.Dir)
	} else {
		// New workflow: create fresh session
		sess, err = s.sessionFactory.Create(session.CreateOptions{
			SessionID:  inst.ID.String(),
			WorkDir:    workDir,
			WorkflowID: inst.ID.String(),
		})
		if err != nil {
			cleanup()
			return fmt.Errorf("creating session: %w", err)
		}
		// Store session directory for persistence
		inst.SessionDir = sess.Dir
		log.Debug(log.CatOrch, "Session created for workflow", "subsystem", "supervisor",
			"workflowID", inst.ID, "sessionDir", sess.Dir)
	}

	// Step 4: Create InfrastructureConfig
	infraCfg := v2.InfrastructureConfig{
		Port:                    port,
		AgentProviders:          s.agentProviders,
		WorkDir:                 workDir,
		BeadsDir:                s.beadsDir,
		SessionID:               inst.ID.String(),
		SessionDir:              sess.Dir,
		SessionRefNotifier:      sess,
		SessionMetadataProvider: sess,
		SoundService:            s.soundService,
		CommandPersistenceProvider: func() processor.CommandWriter {
			return sess
		},
	}

	// Step 5: Create Infrastructure
	infra, err = s.infrastructureFactory.Create(infraCfg)
	if err != nil {
		cleanup()
		return fmt.Errorf("creating infrastructure: %w", err)
	}

	// Step 5.5: Attach session to event brokers for logging
	sess.AttachV2EventBus(workflowCtx, infra.Core.EventBus)

	// Step 5.6: Create Fabric event logger and broker for mention-based notifications
	var fabricLogger *fabricpersist.EventLogger
	var fabricBroker *fabric.Broker

	if infra.Core.FabricService != nil {
		// Create event logger (persists fabric.jsonl to session directory)
		fabricLogger, err = fabricpersist.NewEventLogger(sess.Dir)
		if err != nil {
			cleanup()
			return fmt.Errorf("creating fabric event logger: %w", err)
		}

		// Create broker for batching @mention notifications (replaces CoordinatorNudger)
		fabricBroker = fabric.NewBroker(fabric.BrokerConfig{
			CmdSubmitter:  infra.Core.CmdSubmitter,
			Subscriptions: infra.Core.FabricService.SubscriptionRepository(),
			SlugLookup:    infra.Core.FabricService,
		})

		// Create forwarder that publishes fabric events to the control plane event bus.
		// This enables the dashboard to receive fabric events for the message log.
		fabricForwarder := func(event fabric.Event) {
			infra.Core.EventBus.Publish(pubsub.UpdatedEvent, event)
		}

		// Wire all three handlers to FabricService using ChainHandler:
		// 1. fabricLogger - persists events to fabric.jsonl
		// 2. fabricBroker - handles @mention notifications
		// 3. fabricForwarder - publishes events to control plane event bus for dashboard
		infra.Core.FabricService.SetEventHandler(
			fabricpersist.ChainHandler(fabricLogger.HandleEvent, fabricBroker.HandleEvent, fabricForwarder),
		)

		// Start the broker's event loop
		fabricBroker.Start()
	}

	// Step 6: Start infrastructure (command processor)
	if err := infra.Start(workflowCtx); err != nil {
		cleanup()
		return fmt.Errorf("starting infrastructure: %w", err)
	}

	// Create coordinator MCP server with the v2 adapter
	// Note: BeadsDir is empty here; the v2 infrastructure config handles BEADS_DIR for spawned processes
	mcpCoordServer := mcp.NewCoordinatorServerWithV2Adapter(
		workDir,
		port,
		infrabeads.NewBDExecutor(workDir, ""),
		infra.Core.Adapter,
	)

	// Wire Fabric messaging tools to coordinator MCP server
	if infra.Core.FabricService != nil {
		mcpCoordServer.SetFabricService(infra.Core.FabricService)
	}

	// Attach MCP broker to session for mcp_requests.jsonl logging
	sess.AttachMCPBroker(workflowCtx, mcpCoordServer.Broker())

	// Create worker server cache for /worker/ routes
	// Pass sess as AccountabilityWriter so workers can persist their accountability summaries
	workerServers := newWorkerServerCache(sess, infra.Core.Adapter, infra.Internal.TurnEnforcer, infra.Core.FabricService, sess, workflowCtx)

	// Create observer MCP server (singleton - one observer per workflow)
	observerServer := mcp.NewObserverServer(repository.ObserverID)
	if infra.Core.FabricService != nil {
		observerServer.SetFabricService(infra.Core.FabricService)
	}

	// Attach observer MCP broker to session for mcp_requests.jsonl logging
	sess.AttachMCPBroker(workflowCtx, observerServer.Broker())

	// Set up HTTP routes
	// IMPORTANT: Route registration order matters!
	// 1. MCP routes first (/mcp, /worker/, /observer)
	// 2. API routes second (/api/*)
	// 3. SPA catch-all LAST (/) - serves index.html for client-side routing
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpCoordServer.ServeHTTP())
	mux.HandleFunc("/worker/", workerServers.ServeHTTP)
	mux.Handle("/observer", observerServer.ServeHTTP())

	httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start HTTP server in background
	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Error(log.CatOrch, "MCP server error", "subsystem", "supervisor", "workflowID", inst.ID, "error", err)
		}
	}()
	log.Debug(log.CatOrch, "MCP HTTP server started", "subsystem", "supervisor", "port", port, "workflowID", inst.ID)

	// Store resources in instance BEFORE spawning coordinator.
	// This allows ControlPlane to attach event buses before coordinator spawn.
	inst.Infrastructure = infra
	inst.MCPPort = port
	inst.Ctx = workflowCtx
	inst.Cancel = cancel
	inst.HTTPServer = httpServer
	inst.MCPCoordServer = mcpCoordServer
	inst.Session = sess // May be nil if session factory not configured
	inst.FabricBroker = fabricBroker
	inst.FabricLogger = fabricLogger

	// For cold resume: restore ProcessRepository and ProcessRegistry from session data.
	// This populates the coordinator and worker processes so Resume() can find them.
	if coldResume && inst.SessionDir != "" {
		if err := s.restoreProcessStateFromSession(inst, workflowCtx); err != nil {
			// Log but don't fail - we can still try to resume even if restore fails
			log.Debug(log.CatOrch, "Failed to restore process state from session (will spawn fresh)",
				"subsystem", "supervisor", "workflowID", inst.ID, "error", err)
		}
	}

	return nil
}

// SpawnCoordinator spawns the coordinator process for a workflow.
// Must be called after AllocateResources. Transitions the workflow to Running state.
// If observer is enabled, also spawns the observer sequentially after coordinator.
func (s *defaultSupervisor) SpawnCoordinator(ctx context.Context, inst *WorkflowInstance) error {
	// Validate resources have been allocated
	if inst.Infrastructure == nil {
		return fmt.Errorf("%w: AllocateResources must be called before SpawnCoordinator", ErrInvalidState)
	}
	if inst.State != WorkflowPending {
		return fmt.Errorf("%w: cannot spawn coordinator for workflow in state %s, expected %s",
			ErrInvalidState, inst.State, WorkflowPending)
	}

	// Spawn coordinator
	spawnCmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleCoordinator, command.WithWorkflowConfig(&roles.WorkflowConfig{
		// TODO we need to figure out what we want to do here, currently
		// the InitialPrompt being used should is really the system prompt and the initial prompt
		// should simply being giving it an Epic ID to work on. We will have to likely update the
		// WorkflowSpec to include a SystemPrompt and InitialPrompt and update the web and tui clients
		// as well.
		// SystemPromptOverride: "",
		InitialPromptOverride: inst.InitialPrompt,
	}))
	result, err := inst.Infrastructure.Core.Processor.SubmitAndWait(inst.Ctx, spawnCmd)
	if err != nil {
		return fmt.Errorf("spawning coordinator: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("spawn coordinator failed: %w", result.Error)
	}

	// Spawn observer after coordinator (sequential, not parallel)
	// Uses fail-open error handling: log warnings but don't fail workflow
	s.spawnObserver(ctx, inst)

	// Transition to Running state
	if err := inst.TransitionTo(WorkflowRunning); err != nil {
		return fmt.Errorf("transitioning to Running: %w", err)
	}

	return nil
}

// spawnObserver spawns the observer process for a workflow if enabled.
// Uses fail-open error handling: logs warnings but returns nil on error
// to avoid blocking the workflow if observer fails to spawn.
func (s *defaultSupervisor) spawnObserver(_ context.Context, inst *WorkflowInstance) {
	// Check if observer is enabled (RoleObserver present in agentProviders)
	if _, ok := s.agentProviders[client.RoleObserver]; !ok {
		log.Debug(log.CatOrch, "Observer disabled, skipping spawn", "subsystem", "supervisor", "workflowID", inst.ID)
		return
	}

	// Validate infrastructure is available
	if inst.Infrastructure == nil || inst.Infrastructure.Core.Processor == nil {
		log.Warn(log.CatOrch, "Cannot spawn observer: infrastructure not available",
			"subsystem", "supervisor", "workflowID", inst.ID)
		return
	}

	// Spawn observer
	spawnCmd := command.NewSpawnProcessCommand(command.SourceInternal, repository.RoleObserver)
	result, err := inst.Infrastructure.Core.Processor.SubmitAndWait(inst.Ctx, spawnCmd)
	if err != nil {
		log.Warn(log.CatOrch, "Failed to spawn observer (fail-open)",
			"subsystem", "supervisor", "workflowID", inst.ID, "error", err)
		return
	}
	if !result.Success {
		log.Warn(log.CatOrch, "Observer spawn command failed (fail-open)",
			"subsystem", "supervisor", "workflowID", inst.ID, "error", result.Error)
		return
	}

	log.Debug(log.CatOrch, "Observer spawned successfully", "subsystem", "supervisor", "workflowID", inst.ID)
}

// Pause suspends a running workflow, stopping all processes and clearing queues.
// The infrastructure remains allocated for potential resumption.
func (s *defaultSupervisor) Pause(ctx context.Context, inst *WorkflowInstance) error {
	// Validate workflow is in Running state
	if inst.State != WorkflowRunning {
		return fmt.Errorf("%w: cannot pause workflow in state %s", ErrInvalidState, inst.State)
	}

	// Transition to Paused state
	if err := inst.TransitionTo(WorkflowPaused); err != nil {
		return fmt.Errorf("transitioning to paused: %w", err)
	}

	// Stop infrastructure components but preserve the infrastructure for resume
	if inst.Infrastructure != nil {
		// NOTE: FabricBroker.Stop() not called here - broker continues to exist
		// but paused processes won't receive nudges anyway

		// Clear all message queues to provide a clean slate for resume
		if inst.Infrastructure.Repositories.QueueRepo != nil {
			inst.Infrastructure.Repositories.QueueRepo.ClearAll()
		}

		// Pause all processes via commands (transitions status and stops AI subprocesses)
		// This ensures proper status transitions and event emissions
		if inst.Infrastructure.Repositories.ProcessRepo != nil {
			processes := inst.Infrastructure.Repositories.ProcessRepo.List()
			for _, proc := range processes {
				// Skip already paused or terminal processes
				if proc.Status == repository.StatusPaused || proc.Status.IsTerminal() {
					continue
				}

				pauseCmd := command.NewPauseProcessCommand(command.SourceInternal, proc.ID, "workflow paused")
				_, _ = inst.Infrastructure.Core.Processor.SubmitAndWait(inst.Ctx, pauseCmd)
				// Continue pausing other processes even if one fails
			}
		}
	}

	// Record when the workflow was paused
	inst.PausedAt = time.Now()

	return nil
}

// Resume restarts a paused workflow by sending a resume message to the coordinator.
// The coordinator process is already in Ready state (preserved during pause),
// so sending a message triggers the delivery flow which spawns a new AI session.
func (s *defaultSupervisor) Resume(ctx context.Context, inst *WorkflowInstance) error {
	// Validate workflow is in Paused state
	if inst.State != WorkflowPaused {
		return fmt.Errorf("%w: cannot resume workflow in state %s", ErrInvalidState, inst.State)
	}

	// Transition to Running state first
	if err := inst.TransitionTo(WorkflowRunning); err != nil {
		return fmt.Errorf("transitioning to running: %w", err)
	}

	// NOTE: FabricBroker restart not needed - it was never stopped during pause

	// Resume all processes via commands (transitions Paused -> Ready)
	// Resume workers first, then coordinator, so coordinator can see worker availability
	if inst.Infrastructure != nil && inst.Infrastructure.Repositories.ProcessRepo != nil {
		processes := inst.Infrastructure.Repositories.ProcessRepo.List()

		// Resume workers first (non-coordinator)
		for _, proc := range processes {
			if proc.ID == repository.CoordinatorID {
				continue // Resume coordinator last
			}
			// Only resume paused processes
			if proc.Status != repository.StatusPaused {
				continue
			}

			resumeCmd := command.NewResumeProcessCommand(command.SourceInternal, proc.ID)
			_, _ = inst.Infrastructure.Core.Processor.SubmitAndWait(inst.Ctx, resumeCmd)
			// Continue resuming other processes even if one fails
		}

		// Resume coordinator last
		resumeCmd := command.NewResumeProcessCommand(command.SourceInternal, repository.CoordinatorID)
		if result, err := inst.Infrastructure.Core.Processor.SubmitAndWait(inst.Ctx, resumeCmd); err != nil {
			_ = inst.TransitionTo(WorkflowPaused)
			return fmt.Errorf("resuming coordinator: %w", err)
		} else if !result.Success {
			_ = inst.TransitionTo(WorkflowPaused)
			return fmt.Errorf("resuming coordinator: %w", result.Error)
		}
	}

	// Send system message with pause context - this triggers the delivery flow
	// which spawns a new AI session and attaches it to the existing coordinator process
	if err := s.sendResumeContextMessage(inst); err != nil {
		_ = inst.TransitionTo(WorkflowPaused)
		return fmt.Errorf("sending resume message: %w", err)
	}

	return nil
}

// restoreProcessStateFromSession loads session metadata and restores ProcessRepository
// and ProcessRegistry from the persisted session data. This enables cold resume by
// populating the coordinator and worker processes so Resume() can find them.
func (s *defaultSupervisor) restoreProcessStateFromSession(inst *WorkflowInstance, _ context.Context) error {
	// Load metadata directly (without resumable validation)
	metadata, err := session.Load(inst.SessionDir)
	if err != nil {
		return fmt.Errorf("loading session metadata: %w", err)
	}

	// Build a minimal ResumableSession for the restore functions
	// We only need metadata for process restoration, not messages
	resumableSession := &session.ResumableSession{
		Metadata:       metadata,
		ActiveWorkers:  []session.WorkerMetadata{},
		RetiredWorkers: []session.WorkerMetadata{},
	}

	// Partition workers into active and retired
	for _, worker := range metadata.Workers {
		if worker.RetiredAt.IsZero() {
			resumableSession.ActiveWorkers = append(resumableSession.ActiveWorkers, worker)
		} else {
			resumableSession.RetiredWorkers = append(resumableSession.RetiredWorkers, worker)
		}
	}

	// Restore ProcessRepository (used by command handlers)
	if inst.Infrastructure.Repositories.ProcessRepo != nil {
		if err := session.RestoreProcessRepository(inst.Infrastructure.Repositories.ProcessRepo, resumableSession); err != nil {
			return fmt.Errorf("restoring process repository: %w", err)
		}
		log.Debug(log.CatOrch, "Restored ProcessRepository from session",
			"subsystem", "supervisor", "workflowID", inst.ID,
			"coordinatorSessionRef", metadata.CoordinatorSessionRef,
			"activeWorkers", len(resumableSession.ActiveWorkers),
			"retiredWorkers", len(resumableSession.RetiredWorkers))
	}

	// Restore ProcessRegistry (used by delivery handler to find live processes)
	if inst.Infrastructure.Internal.ProcessRegistry != nil {
		// Create a submitter adapter that wraps the processor
		var submitter process.CommandSubmitter
		if inst.Infrastructure.Core.Processor != nil {
			submitter = &processorSubmitterAdapter{processor: inst.Infrastructure.Core.Processor}
		}
		if err := session.RestoreProcessRegistry(
			inst.Infrastructure.Internal.ProcessRegistry,
			resumableSession,
			submitter,
			inst.Infrastructure.Core.EventBus,
		); err != nil {
			return fmt.Errorf("restoring process registry: %w", err)
		}
		log.Debug(log.CatOrch, "Restored ProcessRegistry from session",
			"subsystem", "supervisor", "workflowID", inst.ID)
	}

	return nil
}

// sendResumeContextMessage sends a system message to the coordinator explaining the pause context.
// This triggers the delivery flow which spawns a new AI session and attaches it to the coordinator.
func (s *defaultSupervisor) sendResumeContextMessage(inst *WorkflowInstance) error {
	if inst.Infrastructure == nil {
		return fmt.Errorf("infrastructure not available")
	}

	message := `[SYSTEM] Automatic System Message 

The workflow was paused by the user and has now been resumed. We are re-orienting to the workflow we were working on.

Diagnose using these tools:
1. Use query_workflow_state to check worker statuses
2. Use fabric_inbox to check for unread messages

Based on what you find:
- If workers are still in "working" state → No action needed, they're actively processing
- If waiting for user input or action → You MUST call the notify_user to alert the user, then end your turn
- If workers are idle/stuck → if they were supposed to be working on a task investigate and determine if we need to send a message to the worker.

If you are still unsure how to proceed then you MUST call the notify_user tool and summarize your findings so the user can help unblock you.`

	// Submit send-to-process command for the coordinator
	sendCmd := command.NewSendToProcessCommand(
		command.SourceInternal,
		repository.CoordinatorID,
		message,
	)

	// Wait for the command to complete - this ensures the message is queued
	// and the delivery flow is triggered
	result, err := inst.Infrastructure.Core.Processor.SubmitAndWait(inst.Ctx, sendCmd)
	if err != nil {
		return fmt.Errorf("submitting send command: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("send command failed: %w", result.Error)
	}

	return nil
}

// Shutdown terminates a workflow and releases all resources.
func (s *defaultSupervisor) Shutdown(ctx context.Context, inst *WorkflowInstance, opts StopOptions) error {
	// Validate workflow can be stopped (Running or Paused can transition to Failed)
	if !inst.State.CanTransitionTo(WorkflowFailed) {
		return fmt.Errorf("%w: cannot stop workflow in state %s", ErrInvalidState, inst.State)
	}

	// Step 0: Check for uncommitted changes in worktree (if applicable)
	// Pre-conditions: inst.WorktreePath != "" and opts.Force == false
	if inst.WorktreePath != "" && !opts.Force && s.gitExecutorFactory != nil {
		gitExec := s.gitExecutorFactory(inst.WorktreePath)
		hasUncommitted, err := gitExec.HasUncommittedChanges()
		if err != nil {
			// Log but don't fail - if we can't check, proceed with stop
			log.Debug(log.CatOrch, "Failed to check uncommitted changes", "subsystem", "supervisor",
				"workflowID", inst.ID, "error", err)
		} else if hasUncommitted {
			return fmt.Errorf("%w: please commit, stash, or force stop to discard changes", ErrUncommittedChanges)
		}
	}

	// Step 1: Stop Fabric broker and close logger (before session close to ensure all events are flushed)
	if inst.FabricBroker != nil {
		inst.FabricBroker.Stop()
		inst.FabricBroker = nil
	}
	if inst.FabricLogger != nil {
		if err := inst.FabricLogger.Close(); err != nil {
			log.Debug(log.CatOrch, "Failed to close fabric logger", "subsystem", "supervisor",
				"workflowID", inst.ID, "error", err)
		}
		inst.FabricLogger = nil
	}

	// Step 2: Close the session if present (finalize session data before infrastructure shutdown)
	if inst.Session != nil {
		// Determine session status based on workflow state
		var sessionStatus session.Status
		if opts.Force {
			sessionStatus = session.StatusFailed
		} else {
			sessionStatus = session.StatusCompleted
		}
		// Close session - errors are logged but don't block shutdown
		if closeErr := inst.Session.Close(sessionStatus); closeErr != nil {
			// Log but don't fail - session data may be incomplete but workflow should still stop
			_ = closeErr // TODO: Log when debug logging is available
		}
		inst.Session = nil
	}

	// Step 3: Shutdown HTTP server if present
	if inst.HTTPServer != nil {
		if opts.Force {
			// Force immediate close
			_ = inst.HTTPServer.Close()
		} else {
			// Graceful shutdown with timeout
			shutdownCtx, cancel := context.WithTimeout(ctx, opts.GracePeriod)
			defer cancel()
			_ = inst.HTTPServer.Shutdown(shutdownCtx)
		}
		inst.HTTPServer = nil
	}

	// Step 4: Shutdown infrastructure if present
	if inst.Infrastructure != nil {
		if opts.Force {
			// Force immediate shutdown
			inst.Infrastructure.Drain()
		} else {
			// Graceful shutdown
			inst.Infrastructure.Shutdown()
		}
	}

	// Step 5: Cancel the workflow context
	if inst.Cancel != nil {
		inst.Cancel()
	}

	// Step 6: Transition to Failed state (user-initiated stop is treated as failure)
	if err := inst.TransitionTo(WorkflowFailed); err != nil {
		return fmt.Errorf("transitioning to Failed: %w", err)
	}

	// Clear resources
	inst.Infrastructure = nil
	inst.MCPPort = 0
	inst.Ctx = nil
	inst.Cancel = nil
	inst.MCPCoordServer = nil

	return nil
}

// getWorkDir returns the effective working directory for a workflow.
// Returns the WorkDir from the instance, or current working directory as fallback.
func getWorkDir(inst *WorkflowInstance) string {
	if inst.WorkDir != "" {
		return inst.WorkDir
	}
	// Fallback to current working directory
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return ""
}

// processorSubmitterAdapter adapts CommandProcessor to process.CommandSubmitter interface.
// The processor.Submit returns error but process.CommandSubmitter.Submit doesn't.
type processorSubmitterAdapter struct {
	processor *processor.CommandProcessor
}

// Submit implements process.CommandSubmitter.
func (a *processorSubmitterAdapter) Submit(cmd command.Command) {
	_ = a.processor.Submit(cmd) // Ignore error - fire-and-forget for turn completion
}

// workerServerCache manages worker MCP servers.
// Workers connect via HTTP to /worker/{workerID}.
type workerServerCache struct {
	accountabilityWriter mcp.AccountabilityWriter
	v2Adapter            *adapter.V2Adapter
	turnEnforcer         handler.TurnCompletionEnforcer
	fabricService        *fabric.Service
	servers              map[string]*mcp.WorkerServer
	mu                   sync.RWMutex

	// For attaching worker MCP brokers to session logging
	session     *session.Session
	workflowCtx context.Context
}

// newWorkerServerCache creates a new worker server cache.
func newWorkerServerCache(
	accountabilityWriter mcp.AccountabilityWriter,
	v2Adapter *adapter.V2Adapter,
	turnEnforcer handler.TurnCompletionEnforcer,
	fabricService *fabric.Service,
	sess *session.Session,
	workflowCtx context.Context,
) *workerServerCache {
	return &workerServerCache{
		accountabilityWriter: accountabilityWriter,
		v2Adapter:            v2Adapter,
		turnEnforcer:         turnEnforcer,
		fabricService:        fabricService,
		servers:              make(map[string]*mcp.WorkerServer),
		session:              sess,
		workflowCtx:          workflowCtx,
	}
}

// ServeHTTP handles HTTP requests for worker MCP endpoints.
func (c *workerServerCache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract worker ID from path: /worker/{workerID}
	workerID := strings.TrimPrefix(r.URL.Path, "/worker/")
	if workerID == "" {
		http.Error(w, "worker ID required in path", http.StatusBadRequest)
		return
	}

	ws := c.getOrCreate(workerID)
	ws.ServeHTTP().ServeHTTP(w, r)
}

// getOrCreate returns an existing worker server or creates a new one.
func (c *workerServerCache) getOrCreate(workerID string) *mcp.WorkerServer {
	c.mu.RLock()
	ws, ok := c.servers[workerID]
	c.mu.RUnlock()
	if ok {
		return ws
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if ws, ok := c.servers[workerID]; ok {
		return ws
	}

	ws = mcp.NewWorkerServer(workerID)
	if c.accountabilityWriter != nil {
		ws.SetAccountabilityWriter(c.accountabilityWriter)
	}
	if c.v2Adapter != nil {
		ws.SetV2Adapter(c.v2Adapter)
	}
	if c.turnEnforcer != nil {
		ws.SetTurnEnforcer(c.turnEnforcer)
	}
	if c.fabricService != nil {
		ws.SetFabricService(c.fabricService)
	}

	// Attach worker MCP broker to session for mcp_requests.jsonl logging
	if c.session != nil && c.workflowCtx != nil {
		c.session.AttachMCPBroker(c.workflowCtx, ws.Broker())
	}

	c.servers[workerID] = ws
	log.Debug(log.CatOrch, "Created worker server", "subsystem", "supervisor", "workerID", workerID)
	return ws
}
