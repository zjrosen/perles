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
	"github.com/zjrosen/perles/internal/orchestration/mcp"
	"github.com/zjrosen/perles/internal/orchestration/session"
	v2 "github.com/zjrosen/perles/internal/orchestration/v2"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/orchestration/v2/prompt/roles"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/orchestration/workflow"
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

	// Stop terminates a workflow and releases all resources.
	// Drains the command processor, finalizes the session, and releases leases.
	// Can be called on any active workflow (Running, Paused).
	Stop(ctx context.Context, inst *WorkflowInstance, opts StopOptions) error
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

	// Flags provides access to feature flags for worktree cleanup behavior.
	// Used to check FlagRemoveWorktree when stopping workflows.
	// If nil, worktree cleanup is skipped.
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
func (s *defaultSupervisor) AllocateResources(ctx context.Context, inst *WorkflowInstance) error {
	// Validate state is Pending
	if inst.State != WorkflowPending {
		return fmt.Errorf("%w: cannot allocate resources for workflow in state %s, expected %s",
			ErrInvalidState, inst.State, WorkflowPending)
	}

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
	if inst.WorktreeEnabled && s.gitExecutorFactory != nil {
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

	// Step 3: Create message repository for this workflow
	messageRepo := repository.NewMemoryMessageRepository()

	// Step 3.5: Create session for this workflow
	workDir := getWorkDir(inst)
	sess, err = s.sessionFactory.Create(session.CreateOptions{
		SessionID: inst.ID.String(),
		WorkDir:   workDir,
	})
	if err != nil {
		cleanup()
		return fmt.Errorf("creating session: %w", err)
	}
	log.Debug(log.CatOrch, "Session created for workflow", "subsystem", "supervisor",
		"workflowID", inst.ID, "sessionDir", sess.Dir)

	// Step 4: Create InfrastructureConfig
	infraCfg := v2.InfrastructureConfig{
		Port:                    port,
		AgentProviders:          s.agentProviders,
		WorkDir:                 workDir,
		BeadsDir:                s.beadsDir,
		MessageRepo:             messageRepo,
		SessionID:               inst.ID.String(),
		SessionDir:              sess.Dir,
		SessionRefNotifier:      sess,
		SessionMetadataProvider: sess,
		SoundService:            s.soundService,
	}

	// Step 5: Create Infrastructure
	infra, err = s.infrastructureFactory.Create(infraCfg)
	if err != nil {
		cleanup()
		return fmt.Errorf("creating infrastructure: %w", err)
	}

	// Step 5.5: Attach session to event brokers for logging
	sess.AttachToBrokers(workflowCtx, nil, messageRepo.Broker(), nil)
	sess.AttachV2EventBus(workflowCtx, infra.Core.EventBus)

	// Step 6: Start infrastructure (command processor)
	if err := infra.Start(workflowCtx); err != nil {
		cleanup()
		return fmt.Errorf("starting infrastructure: %w", err)
	}

	// Create coordinator MCP server with the v2 adapter
	// Note: BeadsDir is empty here; the v2 infrastructure config handles BEADS_DIR for spawned processes
	mcpCoordServer := mcp.NewCoordinatorServerWithV2Adapter(
		messageRepo,
		workDir,
		port,
		infrabeads.NewBDExecutor(workDir, ""),
		infra.Core.Adapter,
	)

	// Attach MCP broker to session for mcp_requests.jsonl logging
	sess.AttachMCPBroker(workflowCtx, mcpCoordServer.Broker())

	// Create worker server cache for /worker/ routes
	workerServers := newWorkerServerCache(messageRepo, nil, infra.Core.Adapter, infra.Internal.TurnEnforcer)

	// Set up HTTP routes
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpCoordServer.ServeHTTP())
	mux.HandleFunc("/worker/", workerServers.ServeHTTP)

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
	inst.MessageRepo = messageRepo
	inst.Session = sess // May be nil if session factory not configured

	return nil
}

// SpawnCoordinator spawns the coordinator process for a workflow.
// Must be called after AllocateResources. Transitions the workflow to Running state.
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

	// Transition to Running state
	if err := inst.TransitionTo(WorkflowRunning); err != nil {
		return fmt.Errorf("transitioning to Running: %w", err)
	}

	return nil
}

// Stop terminates a workflow and releases all resources.
func (s *defaultSupervisor) Stop(ctx context.Context, inst *WorkflowInstance, opts StopOptions) error {
	// Validate workflow can be stopped (Running or Paused)
	if !inst.State.CanTransitionTo(WorkflowStopped) {
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

	// Step 1: Close the session if present (finalize session data before infrastructure shutdown)
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

	// Step 2: Shutdown HTTP server if present
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

	// Step 3: Shutdown infrastructure if present
	if inst.Infrastructure != nil {
		if opts.Force {
			// Force immediate shutdown
			inst.Infrastructure.Drain()
		} else {
			// Graceful shutdown
			inst.Infrastructure.Shutdown()
		}
	}

	// Step 4: Cancel the workflow context
	if inst.Cancel != nil {
		inst.Cancel()
	}

	// Step 5: Remove worktree if present and FlagRemoveWorktree is enabled
	// Pre-conditions: inst.WorktreePath != "" and FlagRemoveWorktree is enabled
	if inst.WorktreePath != "" && s.gitExecutorFactory != nil && s.flags != nil && s.flags.Enabled(flags.FlagRemoveWorktree) {
		gitExec := s.gitExecutorFactory(inst.WorktreePath)
		if err := gitExec.RemoveWorktree(inst.WorktreePath); err != nil {
			// Log warning but don't fail - worktree cleanup is best-effort
			log.Debug(log.CatOrch, "Failed to remove worktree", "subsystem", "supervisor",
				"workflowID", inst.ID, "path", inst.WorktreePath, "error", err)
		} else {
			log.Debug(log.CatOrch, "Worktree cleaned up. Work preserved on branch", "subsystem", "supervisor",
				"workflowID", inst.ID, "branch", inst.WorktreeBranch)
		}
	}

	// Step 6: Transition to Stopped state
	if err := inst.TransitionTo(WorkflowStopped); err != nil {
		return fmt.Errorf("transitioning to Stopped: %w", err)
	}

	// Clear resources
	inst.Infrastructure = nil
	inst.MCPPort = 0
	inst.Ctx = nil
	inst.Cancel = nil
	inst.MCPCoordServer = nil
	inst.MessageRepo = nil

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

// workerServerCache manages worker MCP servers that share the same message store.
// Workers connect via HTTP to /worker/{workerID} and all share the coordinator's
// message repository instance.
type workerServerCache struct {
	msgStore             mcp.MessageStore
	accountabilityWriter mcp.AccountabilityWriter
	v2Adapter            *adapter.V2Adapter
	turnEnforcer         handler.TurnCompletionEnforcer
	servers              map[string]*mcp.WorkerServer
	mu                   sync.RWMutex
}

// newWorkerServerCache creates a new worker server cache.
func newWorkerServerCache(
	msgStore mcp.MessageStore,
	accountabilityWriter mcp.AccountabilityWriter,
	v2Adapter *adapter.V2Adapter,
	turnEnforcer handler.TurnCompletionEnforcer,
) *workerServerCache {
	return &workerServerCache{
		msgStore:             msgStore,
		accountabilityWriter: accountabilityWriter,
		v2Adapter:            v2Adapter,
		turnEnforcer:         turnEnforcer,
		servers:              make(map[string]*mcp.WorkerServer),
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

	ws = mcp.NewWorkerServer(workerID, c.msgStore)
	if c.accountabilityWriter != nil {
		ws.SetAccountabilityWriter(c.accountabilityWriter)
	}
	if c.v2Adapter != nil {
		ws.SetV2Adapter(c.v2Adapter)
	}
	if c.turnEnforcer != nil {
		ws.SetTurnEnforcer(c.turnEnforcer)
	}
	c.servers[workerID] = ws
	log.Debug(log.CatOrch, "Created worker server", "subsystem", "supervisor", "workerID", workerID)
	return ws
}
