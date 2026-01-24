package orchestration

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"go.opentelemetry.io/otel/trace"

	infrabeads "github.com/zjrosen/perles/internal/beads/infrastructure"
	"github.com/zjrosen/perles/internal/config"
	appgit "github.com/zjrosen/perles/internal/git/application"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/amp"      // Register amp client
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/codex"    // Register codex client
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/gemini"   // Register gemini client
	_ "github.com/zjrosen/perles/internal/orchestration/client/providers/opencode" // Register opencode client
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/mcp"
	"github.com/zjrosen/perles/internal/orchestration/session"
	"github.com/zjrosen/perles/internal/orchestration/tracing"
	v2 "github.com/zjrosen/perles/internal/orchestration/v2"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/sound"
)

// InitializerEventType represents the type of event emitted by the Initializer.
type InitializerEventType int

const (
	// InitEventPhaseChanged indicates a phase transition occurred.
	InitEventPhaseChanged InitializerEventType = iota
	// InitEventReady indicates initialization completed successfully.
	InitEventReady
	// InitEventFailed indicates initialization failed with an error.
	InitEventFailed
	// InitEventTimedOut indicates initialization timed out.
	InitEventTimedOut
)

// InitializerEvent represents events emitted by the Initializer.
type InitializerEvent struct {
	Type  InitializerEventType
	Phase InitPhase // Current phase after transition
	Error error     // Non-nil for Failed events
}

// InitializerConfig holds configuration for creating an Initializer.
type InitializerConfig struct {
	WorkDir string
	// BeadsDir is the resolved beads directory path for propagation to spawned processes.
	// When set, spawned AI processes receive BEADS_DIR environment variable.
	BeadsDir string
	// AgentProviders maps roles to their AI client providers.
	AgentProviders client.AgentProviders
	Timeouts       config.TimeoutsConfig
	// Worktree configuration
	WorktreeBaseBranch string             // Branch to base worktree on. Empty = skip worktree creation
	WorktreeBranchName string             // Optional custom branch name (empty = auto-generate)
	GitExecutor        appgit.GitExecutor // Injected for testability
	// Tracing configuration
	TracingConfig config.TracingConfig // Distributed tracing settings
	// Session storage configuration
	SessionStorage config.SessionStorageConfig // Centralized session storage settings
	// Session restoration configuration
	RestoredSession *session.ResumableSession // Set to restore from a previous session
	// Sound service (pre-configured with flags and enabled sounds)
	SoundService sound.SoundService
}

// getAgentProviders returns the AgentProviders map.
// Panics if AgentProviders is nil or missing RoleCoordinator.
func (c *InitializerConfig) getAgentProviders() client.AgentProviders {
	if c.AgentProviders == nil {
		panic("InitializerConfig.AgentProviders is required")
	}
	if _, ok := c.AgentProviders[client.RoleCoordinator]; !ok {
		panic("InitializerConfig.AgentProviders must contain RoleCoordinator")
	}
	return c.AgentProviders
}

// InitializerResources holds the resources created during initialization.
// These are transferred to the Model when initialization completes.
type InitializerResources struct {
	MessageRepo    repository.MessageRepository // Message repository for inter-agent messaging
	MCPServer      *http.Server
	MCPPort        int                    // Dynamic port the MCP server is listening on
	Session        *session.Session       // Session tracking for this orchestration run
	MCPCoordServer *mcp.CoordinatorServer // MCP coordinator server for direct worker messaging
	V2Infra        *v2.Infrastructure     // V2 orchestration infrastructure (owns process lifecycle)
}

// Initializer manages the orchestration initialization lifecycle as a state machine.
// It subscribes to coordinator, worker, and message events to drive phase transitions,
// and publishes high-level events for the TUI to consume.
type Initializer struct {
	// Configuration
	cfg InitializerConfig

	// State (protected by mu)
	phase         InitPhase
	failedAtPhase InitPhase // The phase we were in when failure/timeout occurred
	startTime     time.Time
	err           error
	readyChan     chan struct{} // Closed when initialization completes successfully

	// Worktree state (set during createWorktree phase)
	worktreePath   string // Path to created worktree (empty if disabled)
	worktreeBranch string // Branch name used for worktree
	sessionID      string // Session ID for branch naming (set during Start)
	sessionDir     string // Session directory path (set during createSession)

	// Resources created during initialization
	messageRepo    *repository.MemoryMessageRepository // Message repository for inter-agent messaging
	mcpPort        int                                 // Assigned port
	mcpServer      *http.Server
	mcpCoordServer *mcp.CoordinatorServer
	session        *session.Session // Session tracking

	// V2 orchestration infrastructure (created by v2.NewInfrastructure factory)
	// Contains Core.Processor, Core.EventBus, Core.CmdSubmitter, and Repositories.ProcessRepo
	v2Infra *v2.Infrastructure

	// Tracing infrastructure (created in createWorkspace when enabled)
	tracingProvider *tracing.Provider // Manages tracer provider lifecycle
	tracer          trace.Tracer      // Active tracer for span creation

	// Event broker for publishing state changes to TUI
	broker *pubsub.Broker[InitializerEvent]

	// Context for managing goroutine lifecycle
	ctx    context.Context
	cancel context.CancelFunc

	// Synchronization
	mu      sync.RWMutex
	started bool
}

// NewInitializer creates a new Initializer with the given configuration.
// Applies defaults from config.DefaultTimeoutsConfig() for any zero-value timeout fields.
func NewInitializer(cfg InitializerConfig) *Initializer {
	// Apply defaults for zero-value timeout fields
	defaults := config.DefaultTimeoutsConfig()
	if cfg.Timeouts.WorktreeCreation == 0 {
		cfg.Timeouts.WorktreeCreation = defaults.WorktreeCreation
	}
	if cfg.Timeouts.CoordinatorStart == 0 {
		cfg.Timeouts.CoordinatorStart = defaults.CoordinatorStart
	}
	if cfg.Timeouts.WorkspaceSetup == 0 {
		cfg.Timeouts.WorkspaceSetup = defaults.WorkspaceSetup
	}
	if cfg.Timeouts.MaxTotal == 0 {
		cfg.Timeouts.MaxTotal = defaults.MaxTotal
	}

	return &Initializer{
		cfg:       cfg,
		phase:     InitNotStarted,
		readyChan: make(chan struct{}),
		broker:    pubsub.NewBroker[InitializerEvent](),
	}
}

// InitializerConfigBuilder provides a fluent builder pattern for InitializerConfig.
// It separates static configuration (from Model) from runtime-only fields that must be set separately.
type InitializerConfigBuilder struct {
	cfg InitializerConfig
}

// NewInitializerConfigFromModel creates an InitializerConfigBuilder populated with
// configuration from a Model's state. This centralizes the transformation from Model
// state to InitializerConfig.
//
// Runtime-only fields (GitExecutor, RestoredSession, SoundService, Timeout) must be set
// separately using the builder methods, as they contain runtime dependencies or computed values.
//
// Example:
//
//	cfg := NewInitializerConfigFromModel(m).
//	    WithTimeout(timeout).
//	    WithGitExecutor(m.gitExecutor).
//	    WithSoundService(m.services.Sounds).
//	    Build()
func NewInitializerConfigFromModel(
	workDir string,
	beadsDir string,
	agentProviders client.AgentProviders,
	worktreeBaseBranch string,
	worktreeCustomBranch string,
	tracingConfig config.TracingConfig,
	sessionStorageConfig config.SessionStorageConfig,
) *InitializerConfigBuilder {
	return &InitializerConfigBuilder{
		cfg: InitializerConfig{
			WorkDir:            workDir,
			BeadsDir:           beadsDir,
			AgentProviders:     agentProviders,
			WorktreeBaseBranch: worktreeBaseBranch,
			WorktreeBranchName: worktreeCustomBranch,
			TracingConfig:      tracingConfig,
			SessionStorage:     sessionStorageConfig,
		},
	}
}

// WithTimeouts sets all timeout configuration values.
// If not set, NewInitializer applies defaults from config.DefaultTimeoutsConfig().
func (b *InitializerConfigBuilder) WithTimeouts(timeouts config.TimeoutsConfig) *InitializerConfigBuilder {
	b.cfg.Timeouts = timeouts
	return b
}

// WithTimeout sets the CoordinatorStart timeout for backwards compatibility.
// New code should prefer WithTimeouts() to set all timeout values.
// If not set, defaults are applied in NewInitializer.
func (b *InitializerConfigBuilder) WithTimeout(timeout time.Duration) *InitializerConfigBuilder {
	b.cfg.Timeouts.CoordinatorStart = timeout
	return b
}

// WithGitExecutor sets the git executor for worktree operations.
func (b *InitializerConfigBuilder) WithGitExecutor(executor appgit.GitExecutor) *InitializerConfigBuilder {
	b.cfg.GitExecutor = executor
	return b
}

// WithRestoredSession sets the session to restore from.
func (b *InitializerConfigBuilder) WithRestoredSession(session *session.ResumableSession) *InitializerConfigBuilder {
	b.cfg.RestoredSession = session
	return b
}

// WithSoundService sets the sound service for notifications.
func (b *InitializerConfigBuilder) WithSoundService(svc sound.SoundService) *InitializerConfigBuilder {
	b.cfg.SoundService = svc
	return b
}

// Build returns the completed InitializerConfig.
func (b *InitializerConfigBuilder) Build() InitializerConfig {
	return b.cfg
}

// Broker returns the event broker for subscribing to state changes.
func (i *Initializer) Broker() *pubsub.Broker[InitializerEvent] {
	return i.broker
}

// Phase returns the current initialization phase.
func (i *Initializer) Phase() InitPhase {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.phase
}

// Error returns the error if initialization failed.
func (i *Initializer) Error() error {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.err
}

// FailedAtPhase returns the phase at which initialization failed or timed out.
// Only meaningful when Phase() returns InitFailed or InitTimedOut.
func (i *Initializer) FailedAtPhase() InitPhase {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.failedAtPhase
}

// Timeouts returns the timeout configuration used by this Initializer.
// Useful for displaying phase-specific timeout durations in error messages.
func (i *Initializer) Timeouts() config.TimeoutsConfig {
	return i.cfg.Timeouts
}

// WorktreePath returns the path to the created worktree, or empty string if disabled.
func (i *Initializer) WorktreePath() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.worktreePath
}

// WorktreeBranch returns the branch name used for the worktree, or empty string if disabled.
func (i *Initializer) WorktreeBranch() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.worktreeBranch
}

// SessionDir returns the session directory path, or empty string if not yet created.
func (i *Initializer) SessionDir() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.sessionDir
}

// SpinnerData returns data needed for spinner rendering.
func (i *Initializer) SpinnerData() InitPhase {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.phase
}

// Resources returns the initialized resources.
// Only valid after receiving InitEventReady.
func (i *Initializer) Resources() InitializerResources {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return InitializerResources{
		MessageRepo:    i.messageRepo,
		MCPServer:      i.mcpServer,
		MCPPort:        i.mcpPort,
		Session:        i.session,
		MCPCoordServer: i.mcpCoordServer,
		V2Infra:        i.v2Infra,
	}
}

// GetMessageRepo returns the message repository if it has been created, nil otherwise.
// The returned repository provides inter-agent messaging with pub/sub broker support.
func (i *Initializer) GetMessageRepo() repository.MessageRepository {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.messageRepo
}

// GetV2EventBus returns the v2 event bus for TUI subscription, nil if not yet created.
// The event bus is created during createWorkspace() and can be subscribed to for
// receiving v2 orchestration events (WorkerEvent, CommandErrorEvent, etc.).
func (i *Initializer) GetV2EventBus() *pubsub.Broker[any] {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.v2Infra == nil {
		return nil
	}
	return i.v2Infra.Core.EventBus
}

// GetCmdSubmitter returns the command submitter for v2 command submission.
// Returns nil if not yet created. Used by TUI to submit v2 commands directly.
func (i *Initializer) GetCmdSubmitter() process.CommandSubmitter {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.v2Infra == nil {
		return nil
	}
	return i.v2Infra.Core.CmdSubmitter
}

// GetProcessRepository returns the process repository for unified state management.
// Returns nil if not yet created.
// Deprecated: Use GetV2Infra().Repositories.ProcessRepo instead.
func (i *Initializer) GetProcessRepository() repository.ProcessRepository {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.v2Infra == nil {
		return nil
	}
	return i.v2Infra.Repositories.ProcessRepo
}

// GetV2Infra returns the v2 orchestration infrastructure.
// Returns nil if not yet created.
func (i *Initializer) GetV2Infra() *v2.Infrastructure {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.v2Infra
}

// Start begins the initialization process.
// This runs asynchronously; progress is communicated via the event broker.
func (i *Initializer) Start() error {
	i.mu.Lock()
	if i.started {
		i.mu.Unlock()
		return fmt.Errorf("initializer already started")
	}
	i.started = true
	i.ctx, i.cancel = context.WithCancel(context.Background())
	i.startTime = time.Now()
	// Generate session ID early for worktree branch naming
	i.sessionID = uuid.New().String()
	i.mu.Unlock()

	go i.run()
	return nil
}

// Retry restarts initialization from the beginning.
func (i *Initializer) Retry() error {
	i.Cancel()

	i.mu.Lock()
	i.phase = InitNotStarted
	i.failedAtPhase = InitNotStarted
	i.err = nil
	i.started = false
	i.readyChan = make(chan struct{}) // Reset ready channel
	i.messageRepo = nil
	i.mcpPort = 0
	i.mcpServer = nil
	i.mcpCoordServer = nil
	i.session = nil
	i.v2Infra = nil
	// Reset worktree state
	i.worktreePath = ""
	i.worktreeBranch = ""
	i.sessionID = ""
	i.sessionDir = ""
	// Reset tracing state
	i.tracingProvider = nil
	i.tracer = nil
	i.mu.Unlock()

	return i.Start()
}

// Cancel stops initialization and cleans up resources.
// This method is safe to call multiple times - subsequent calls are no-ops.
func (i *Initializer) Cancel() {
	// Copy cancel func under lock to call it outside the lock
	i.mu.Lock()
	cancel := i.cancel
	i.cancel = nil // Clear to ensure idempotency (only cancel once)
	i.mu.Unlock()

	// Signal cancellation to running goroutines
	if cancel != nil {
		cancel()
	}

	// Clean up all resources (idempotent)
	i.cleanupResources()
}

// run is the main initialization goroutine.
// With lazy spawning, initialization completes when coordinator's first turn finishes.
//
// Timeout architecture:
// - Each phase (worktree, workspace, coordinator) has its own context.WithTimeout()
// - A parallel maxTotalTimer enforces a hard ceiling on total initialization time
// - The maxTotalTimer goroutine listens on readyChan to exit cleanly on success
func (i *Initializer) run() {
	// Start maxTotalTimer as a safety net running in parallel with per-phase timeouts.
	// This provides hard-cut semantics: if MaxTotal fires, initialization fails immediately.
	maxTotalTimer := time.NewTimer(i.cfg.Timeouts.MaxTotal)
	defer maxTotalTimer.Stop()

	// Launch goroutine to monitor maxTotalTimer.
	// This goroutine exits cleanly when:
	// 1. maxTotalTimer fires (calls i.timeout())
	// 2. readyChan is closed (initialization succeeded)
	// 3. ctx is cancelled (initialization failed or was cancelled)
	go func() {
		select {
		case <-maxTotalTimer.C:
			// MaxTotal safety net fired - hard-cut timeout
			i.timeoutWithMaxTotal()
		case <-i.readyChan:
			// Initialization completed successfully - exit cleanly
		case <-i.ctx.Done():
			// Context cancelled - exit cleanly
		}
	}()

	// Phase 0: Create worktree if branch specified
	if i.cfg.WorktreeBaseBranch != "" && i.cfg.GitExecutor != nil {
		i.transitionTo(InitCreatingWorktree)
		worktreeCtx, worktreeCancel := context.WithTimeout(i.ctx, i.cfg.Timeouts.WorktreeCreation)
		err := i.createWorktreeWithContext(worktreeCtx)
		worktreeCancel()
		if err != nil {
			i.fail(err)
			return
		}
	}

	// Phase 1: Create workspace
	i.transitionTo(InitCreatingWorkspace)
	workspaceCtx, workspaceCancel := context.WithTimeout(i.ctx, i.cfg.Timeouts.WorkspaceSetup)
	err := i.createWorkspaceWithContext(workspaceCtx)
	workspaceCancel()
	if err != nil {
		i.fail(err)
		return
	}

	// For resumed sessions, skip spawning coordinator and waiting for first message.
	// The coordinator and workers are already restored in the ProcessRepository.
	// Just transition directly to ready.
	if i.cfg.RestoredSession != nil {
		log.Debug(log.CatOrch, "Session resume: skipping coordinator spawn, transitioning to ready",
			"subsystem", "init",
			"sessionID", i.cfg.RestoredSession.Metadata.SessionID)
		i.transitionTo(InitReady)
		i.publishEvent(InitializerEvent{
			Type:  InitEventReady,
			Phase: InitReady,
		})
		close(i.readyChan)
		return
	}

	// Phase 2+3: Spawn coordinator and await first message
	// These phases share the CoordinatorStart timeout since they're both about
	// getting the coordinator up and responding.
	coordCtx, coordCancel := context.WithTimeout(i.ctx, i.cfg.Timeouts.CoordinatorStart)
	defer coordCancel()

	// Phase 2: Spawn coordinator (new sessions only)
	i.transitionTo(InitSpawningCoordinator)
	if err := i.spawnCoordinatorWithContext(coordCtx); err != nil {
		i.fail(err)
		return
	}

	// Phase 3: Event-driven - wait for coordinator's first turn to complete
	// Subscribe to v2EventBus for coordinator process events
	v2Sub := i.v2Infra.Core.EventBus.Subscribe(i.ctx)

	// Transition to awaiting first message
	i.transitionTo(InitAwaitingFirstMessage)

	for {
		select {
		case <-i.ctx.Done():
			// Context cancelled - clean exit
			return

		case <-coordCtx.Done():
			// Coordinator timeout exceeded
			if coordCtx.Err() == context.DeadlineExceeded {
				i.fail(fmt.Errorf("coordinator start timed out after %v", i.cfg.Timeouts.CoordinatorStart))
			}
			return

		case <-i.readyChan:
			// Initialization completed successfully
			return

		case event, ok := <-v2Sub:
			if !ok {
				return
			}
			// Coordinator ProcessReady triggers transition to InitReady
			i.handleV2Event(event)
		}
	}
}

// createSession creates a new orchestration session with its directory structure,
// or reopens an existing session if RestoredSession is configured.
//
// For new sessions:
// - Uses the session ID generated during Start()
// - Creates the session directory in the centralized storage location
// - Initializes the session tracking object with application context metadata
//
// For restored sessions (RestoredSession is set):
// - Uses session.Reopen() to continue writing to existing session files
// - Passes SessionID and SessionDir from RestoredSession.Metadata
// - Applies the same SessionOptions as normal creation
func (i *Initializer) createSession() (*session.Session, error) {
	// Check if we're restoring from a previous session
	if i.cfg.RestoredSession != nil && i.cfg.RestoredSession.Metadata != nil {
		return i.reopenSession()
	}

	// Use the session ID generated during Start()
	i.mu.RLock()
	sessionID := i.sessionID
	i.mu.RUnlock()

	// Determine effective work directory (use worktree path if created)
	effectiveWorkDir := i.cfg.WorkDir
	i.mu.RLock()
	if i.worktreePath != "" {
		effectiveWorkDir = i.worktreePath
	}
	i.mu.RUnlock()

	// Build centralized session path using SessionPathBuilder
	// Derive application name from git remote or directory basename
	appName := i.cfg.SessionStorage.ApplicationName
	if appName == "" {
		appName = session.DeriveApplicationName(effectiveWorkDir, i.cfg.GitExecutor)
	}

	pathBuilder := session.NewSessionPathBuilder(
		i.cfg.SessionStorage.BaseDir,
		appName,
	)

	now := time.Now()
	sessionDir := pathBuilder.SessionDir(sessionID, now)
	datePartition := now.Format("2006-01-02")

	// Create session with application context options
	sess, err := session.New(sessionID, sessionDir,
		session.WithWorkDir(effectiveWorkDir),
		session.WithApplicationName(pathBuilder.ApplicationName()),
		session.WithDatePartition(datePartition),
		session.WithPathBuilder(pathBuilder),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Store sessionDir for later reference (e.g., V2 adapter accountability summaries)
	i.mu.Lock()
	i.sessionDir = sessionDir
	i.mu.Unlock()

	log.Debug(log.CatOrch, "Session created", "subsystem", "init",
		"sessionID", sessionID, "dir", sessionDir,
		"app", pathBuilder.ApplicationName(), "date", datePartition)
	return sess, nil
}

// reopenSession reopens an existing session directory for continued writing.
// Called when RestoredSession is configured, enabling session resumption.
// Uses session.Reopen() to open existing files in append mode so new messages
// continue writing to the same JSONL files without overwriting.
func (i *Initializer) reopenSession() (*session.Session, error) {
	meta := i.cfg.RestoredSession.Metadata
	sessionID := meta.SessionID
	sessionDir := meta.SessionDir

	// Determine effective work directory (use worktree path if created)
	effectiveWorkDir := i.cfg.WorkDir
	i.mu.RLock()
	if i.worktreePath != "" {
		effectiveWorkDir = i.worktreePath
	}
	i.mu.RUnlock()

	// Build path builder for consistency (needed for index updates)
	appName := i.cfg.SessionStorage.ApplicationName
	if appName == "" {
		appName = session.DeriveApplicationName(effectiveWorkDir, i.cfg.GitExecutor)
	}

	pathBuilder := session.NewSessionPathBuilder(
		i.cfg.SessionStorage.BaseDir,
		appName,
	)

	// Reopen the existing session in append mode
	sess, err := session.Reopen(sessionID, sessionDir,
		session.WithWorkDir(effectiveWorkDir),
		session.WithApplicationName(pathBuilder.ApplicationName()),
		session.WithDatePartition(meta.DatePartition),
		session.WithPathBuilder(pathBuilder),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to reopen session: %w", err)
	}

	// Override the generated session ID with the restored one
	i.mu.Lock()
	i.sessionID = sessionID
	i.sessionDir = sessionDir
	i.mu.Unlock()

	log.Debug(log.CatOrch, "Session reopened for resumption", "subsystem", "init",
		"sessionID", sessionID, "dir", sessionDir,
		"app", pathBuilder.ApplicationName())
	return sess, nil
}

// createWorktreeWithContext creates a git worktree for isolated workspace operation
// with context-based timeout/cancellation support.
// It performs pre-flight checks, determines the worktree path, and creates
// the worktree with an auto-generated or configured branch name.
// Returns descriptive error if context deadline is exceeded.
func (i *Initializer) createWorktreeWithContext(ctx context.Context) error {
	gitExec := i.cfg.GitExecutor

	// Pre-flight check: ensure we're in a git repository
	if !gitExec.IsGitRepo() {
		return fmt.Errorf("not a git repository")
	}

	// Prune stale worktrees before creating a new one
	if err := gitExec.PruneWorktrees(); err != nil {
		log.Warn(log.CatOrch, "Failed to prune worktrees", "subsystem", "init", "error", err)
		// Continue anyway - pruning is a best-effort cleanup
	}

	// Get the session ID for path and branch generation
	i.mu.RLock()
	sessionID := i.sessionID
	i.mu.RUnlock()

	// Determine worktree path
	path, err := gitExec.DetermineWorktreePath(sessionID)
	if err != nil {
		return fmt.Errorf("failed to determine worktree path: %w", err)
	}

	// Use custom branch name if provided, otherwise auto-generate
	var newBranch string
	if i.cfg.WorktreeBranchName != "" {
		newBranch = i.cfg.WorktreeBranchName
	} else {
		shortID := sessionID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		newBranch = fmt.Sprintf("perles-session-%s", shortID)
	}

	// Base branch is what the user selected (main, current branch, etc.)
	// Empty means use current HEAD
	baseBranch := i.cfg.WorktreeBaseBranch

	// Create the worktree with new branch based on selected branch
	// Use context-aware method for timeout support
	if err := gitExec.CreateWorktreeWithContext(ctx, path, newBranch, baseBranch); err != nil {
		// Check if this was a timeout and provide descriptive error
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("worktree creation timed out after %v: %w", i.cfg.Timeouts.WorktreeCreation, err)
		}
		return fmt.Errorf("failed to create worktree: %w", err)
	}

	// Store worktree state
	i.mu.Lock()
	i.worktreePath = path
	i.worktreeBranch = newBranch
	i.mu.Unlock()

	log.Info(log.CatOrch, "Worktree created", "subsystem", "init", "path", path, "branch", newBranch, "basedOn", baseBranch)
	return nil
}

// MCPServerResult holds the result of createMCPServer().
// It encapsulates all components needed for MCP communication.
type MCPServerResult struct {
	Server      *http.Server           // HTTP server serving MCP endpoints
	Port        int                    // Dynamic port the server is listening on
	Listener    net.Listener           // TCP listener for the HTTP server
	CoordServer *mcp.CoordinatorServer // MCP coordinator server for direct worker messaging
	WorkerCache *workerServerCache     // Worker server cache for worker endpoints
}

// MCPListenerResult holds the result of createMCPListener().
type MCPListenerResult struct {
	Listener net.Listener // TCP listener for the HTTP server
	Port     int          // Dynamic port assigned by the OS
}

// createMCPListener creates a TCP listener on localhost:0 to get a random available port.
// This is separated from createMCPServer() to allow early listener creation and port discovery.
func (i *Initializer) createMCPListener() (*MCPListenerResult, error) {
	// Using localhost (127.0.0.1) to avoid binding to all interfaces
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP listener: %w", err)
	}

	// Extract the assigned port
	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return nil, fmt.Errorf("failed to get TCP address from listener")
	}
	port := tcpAddr.Port
	log.Debug(log.CatOrch, "MCP listener created on dynamic port", "subsystem", "init", "port", port)

	return &MCPListenerResult{
		Listener: listener,
		Port:     port,
	}, nil
}

// MCPServerConfig holds the configuration for createMCPServer().
type MCPServerConfig struct {
	Listener     net.Listener                        // Pre-created TCP listener
	Port         int                                 // Port the listener is bound to
	MsgRepo      *repository.MemoryMessageRepository // Message repository for coordinator server
	Session      *session.Session                    // Session for reflection writing
	V2Adapter    *adapter.V2Adapter                  // V2 adapter for routing
	TurnEnforcer mcp.ToolCallRecorder                // Turn completion enforcer for workers
	WorkDir      string                              // Working directory
	BeadsDir     string                              // Path to .beads directory for BEADS_DIR env var
	Tracer       trace.Tracer                        // Tracer for distributed tracing (optional)
}

// createMCPServer creates the MCP server with HTTP routes for coordinator and worker endpoints.
// It requires a pre-created listener (from createMCPListener) and all dependencies.
func (i *Initializer) createMCPServer(cfg MCPServerConfig) (*MCPServerResult, error) {
	// Validate required config fields
	if cfg.Listener == nil {
		return nil, fmt.Errorf("listener is required")
	}
	if cfg.MsgRepo == nil {
		return nil, fmt.Errorf("message repository is required")
	}

	// Create coordinator server with the dynamic port and v2 adapter
	mcpCoordServer := mcp.NewCoordinatorServerWithV2Adapter(
		cfg.MsgRepo,
		cfg.WorkDir,
		cfg.Port,
		infrabeads.NewBDExecutor(cfg.WorkDir, cfg.BeadsDir),
		cfg.V2Adapter,
	)

	// Set tracer for distributed tracing if provided
	if cfg.Tracer != nil {
		mcpCoordServer.SetTracer(cfg.Tracer)
	}

	// Pass the session as the reflection writer so workers can save reflections
	// Pass the v2Adapter so all worker servers route through v2
	// Pass the turnEnforcer to track tool calls during worker turns
	workerServers := newWorkerServerCache(cfg.MsgRepo, cfg.Session, cfg.V2Adapter, cfg.TurnEnforcer)

	// Set up HTTP routes
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpCoordServer.ServeHTTP())
	mux.HandleFunc("/worker/", workerServers.ServeHTTP)

	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return &MCPServerResult{
		Server:      httpServer,
		Port:        cfg.Port,
		Listener:    cfg.Listener,
		CoordServer: mcpCoordServer,
		WorkerCache: workerServers,
	}, nil
}

// createWorkspaceWithContext orchestrates the creation of all workspace resources needed for orchestration
// with context-based timeout/cancellation support.
// It composes the extracted helper methods (createAIClient, createSession, createMCPListener,
// createMCPServer) with V2 infrastructure setup in a clear sequence.
//
// Orchestration steps:
//  1. Create AI client (Claude or Amp)
//  2. Create message repository for inter-agent messaging
//  3. Create session for tracking this orchestration run
//  4. Create MCP listener on dynamic port (needed before V2 for port)
//  5. Create V2 orchestration infrastructure (processor, handlers, adapter)
//  6. Create MCP server with HTTP routes
//  7. Start HTTP server in background
//
// Error handling: Each step properly cleans up resources from previous steps on failure.
// Returns descriptive error if context deadline is exceeded.
func (i *Initializer) createWorkspaceWithContext(ctx context.Context) error {
	// Check context before starting
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("workspace setup timed out after %v", i.cfg.Timeouts.WorkspaceSetup)
		}
		return ctx.Err()
	}
	// Determine effective work directory (use worktree path if created)
	effectiveWorkDir := i.cfg.WorkDir
	i.mu.RLock()
	if i.worktreePath != "" {
		effectiveWorkDir = i.worktreePath
	}
	i.mu.RUnlock()

	// ============================================================
	// Step 0: Create tracing provider (if enabled)
	// Must happen first so tracer is available for all components
	// ============================================================
	var tracer trace.Tracer
	tracingCfg := i.cfg.TracingConfig
	if tracingCfg.Enabled {
		// Convert config.TracingConfig to tracing.Config
		// Apply default file path if not specified
		filePath := tracingCfg.FilePath
		if filePath == "" && tracingCfg.Exporter == "file" {
			filePath = config.DefaultTracesFilePath()
		}

		provider, err := tracing.NewProvider(tracing.Config{
			Enabled:      tracingCfg.Enabled,
			Exporter:     tracingCfg.Exporter,
			FilePath:     filePath,
			OTLPEndpoint: tracingCfg.OTLPEndpoint,
			SampleRate:   tracingCfg.SampleRate,
			ServiceName:  "perles-orchestrator",
		})
		if err != nil {
			return fmt.Errorf("creating tracing provider: %w", err)
		}

		// Store provider for cleanup and get tracer
		i.mu.Lock()
		i.tracingProvider = provider
		i.tracer = provider.Tracer()
		i.mu.Unlock()
		tracer = provider.Tracer()

		log.Debug(log.CatOrch, "Tracing provider created", "subsystem", "init", "exporter", tracingCfg.Exporter)
	}

	// ============================================================
	// Step 1: Get providers from config
	// ============================================================
	agentProviders := i.cfg.getAgentProviders()

	// ============================================================
	// Step 2: Create message repository for inter-agent messaging
	// ============================================================
	msgRepo := repository.NewMemoryMessageRepository()
	i.mu.Lock()
	i.messageRepo = msgRepo
	i.mu.Unlock()

	// ============================================================
	// Step 3: Create session for tracking this orchestration run
	// ============================================================
	sess, err := i.createSession()
	if err != nil {
		return err
	}
	i.mu.Lock()
	i.session = sess
	i.mu.Unlock()

	// ============================================================
	// Step 4: Create MCP listener on dynamic port
	// This must happen before V2 infrastructure so handlers know the port
	// ============================================================
	listenerResult, err := i.createMCPListener()
	if err != nil {
		return err
	}
	port := listenerResult.Port

	// ============================================================
	// Step 5: Create V2 orchestration infrastructure using factory
	// ============================================================
	// Read stored session directory for V2 adapter accountability summaries
	i.mu.RLock()
	sessionDir := i.sessionDir
	i.mu.RUnlock()

	v2Infra, err := v2.NewInfrastructure(v2.InfrastructureConfig{
		Port:                    port,
		AgentProviders:          agentProviders,
		WorkDir:                 effectiveWorkDir, // Use worktree path when enabled
		BeadsDir:                i.cfg.BeadsDir,   // Propagated to spawned AI processes as BEADS_DIR env var
		MessageRepo:             msgRepo,
		SessionID:               sess.ID,
		SessionDir:              sessionDir, // Centralized session storage path
		Tracer:                  tracer,     // nil when tracing disabled - middleware handles this gracefully
		SessionRefNotifier:      sess,       // Session implements SessionRefNotifier for crash-resilient resumption
		SoundService:            i.cfg.SoundService,
		SessionMetadataProvider: sess, // Session implements SessionMetadataProvider for workflow completion
		WorkflowStateProvider:   sess, // Session implements WorkflowStateProvider for auto-refresh workflow continuation
	})
	if err != nil {
		_ = listenerResult.Listener.Close()
		return fmt.Errorf("creating v2 infrastructure: %w", err)
	}

	// ============================================================
	// Step 5a: Restore repositories from session if resuming
	// ============================================================
	if i.cfg.RestoredSession != nil {
		// Restore ProcessRepository with coordinator and workers from session
		if err := session.RestoreProcessRepository(v2Infra.Repositories.ProcessRepo, i.cfg.RestoredSession); err != nil {
			_ = listenerResult.Listener.Close()
			v2Infra.Drain()
			return fmt.Errorf("restoring process repository: %w", err)
		}
		log.Debug(log.CatOrch, "Restored ProcessRepository from session", "subsystem", "init",
			"activeWorkers", len(i.cfg.RestoredSession.ActiveWorkers),
			"retiredWorkers", len(i.cfg.RestoredSession.RetiredWorkers))

		// Restore ProcessRegistry with dormant processes for coordinator and active workers
		// This enables Resume() to work when messages are delivered
		if err := session.RestoreProcessRegistry(
			v2Infra.Internal.ProcessRegistry,
			i.cfg.RestoredSession,
			v2Infra.Core.CmdSubmitter,
			v2Infra.Core.EventBus,
		); err != nil {
			_ = listenerResult.Listener.Close()
			v2Infra.Drain()
			return fmt.Errorf("restoring process registry: %w", err)
		}
		log.Debug(log.CatOrch, "Restored ProcessRegistry with dormant processes", "subsystem", "init",
			"coordinator", true,
			"activeWorkers", len(i.cfg.RestoredSession.ActiveWorkers))

		// Restore MessageRepository with inter-agent messages from session
		if err := session.RestoreMessageRepository(msgRepo, i.cfg.RestoredSession.InterAgentMessages); err != nil {
			_ = listenerResult.Listener.Close()
			v2Infra.Drain()
			return fmt.Errorf("restoring message repository: %w", err)
		}
		log.Debug(log.CatOrch, "Restored MessageRepository from session", "subsystem", "init",
			"messageCount", len(i.cfg.RestoredSession.InterAgentMessages))
	}

	// Attach session to brokers for event logging
	// Note: MCP broker attached later after mcpCoordServer is created
	sess.AttachToBrokers(i.ctx, nil, msgRepo.Broker(), nil)
	sess.AttachV2EventBus(i.ctx, v2Infra.Core.EventBus)

	// Start the V2 infrastructure (processor loop)
	if err := v2Infra.Start(i.ctx); err != nil {
		_ = listenerResult.Listener.Close()
		v2Infra.Drain()
		return fmt.Errorf("starting v2 infrastructure: %w", err)
	}

	log.Debug(log.CatOrch, "V2 orchestration infrastructure initialized", "subsystem", "init")

	// Store V2 infrastructure reference
	i.mu.Lock()
	i.v2Infra = v2Infra
	i.mu.Unlock()

	// ============================================================
	// Step 6: Create MCP server with HTTP routes
	// ============================================================
	mcpResult, err := i.createMCPServer(MCPServerConfig{
		Listener:     listenerResult.Listener,
		Port:         port,
		MsgRepo:      msgRepo,
		Session:      sess,
		V2Adapter:    v2Infra.Core.Adapter,
		TurnEnforcer: v2Infra.Internal.TurnEnforcer,
		WorkDir:      effectiveWorkDir, // Use worktree path when enabled
		BeadsDir:     i.cfg.BeadsDir,   // Propagate beads directory for BEADS_DIR env var
		Tracer:       tracer,           // nil when tracing disabled - server handles this gracefully
	})
	if err != nil {
		_ = listenerResult.Listener.Close()
		v2Infra.Drain()
		return err
	}

	// Attach session to MCP broker for event logging
	sess.AttachMCPBroker(i.ctx, mcpResult.CoordServer.Broker())

	// Store MCP resources (workerServers is not stored; it's only used by HTTP handler)
	i.mu.Lock()
	i.mcpPort = port
	i.mcpServer = mcpResult.Server
	i.mcpCoordServer = mcpResult.CoordServer
	i.mu.Unlock()

	// ============================================================
	// Step 7: Start HTTP server in background
	// ============================================================
	go func() {
		if err := mcpResult.Server.Serve(mcpResult.Listener); err != nil && err != http.ErrServerClosed {
			log.Error(log.CatOrch, "MCP server error", "subsystem", "init", "error", err)
		}
	}()

	return nil
}

// spawnCoordinatorWithContext creates and starts the coordinator using the v2 command processor
// with context-based timeout/cancellation support.
// This submits a SpawnProcessCommand which handles all the AI spawning, registry registration,
// and ProcessRepository updates through the unified command pattern.
//
// Note: This method is only called for new sessions. Resumed sessions skip coordinator
// spawning entirely in the run() method since the coordinator is already restored.
// Returns descriptive error if context deadline is exceeded.
func (i *Initializer) spawnCoordinatorWithContext(ctx context.Context) error {
	i.mu.RLock()
	v2Infra := i.v2Infra
	i.mu.RUnlock()

	if v2Infra == nil {
		return fmt.Errorf("v2 infrastructure not initialized")
	}

	// Create and submit SpawnProcessCommand for coordinator
	spawnCmd := command.NewSpawnProcessCommand(command.SourceUser, repository.RoleCoordinator)

	// Use SubmitAndWait to ensure coordinator is fully spawned before continuing
	// Pass the context for timeout support
	result, err := v2Infra.Core.Processor.SubmitAndWait(ctx, spawnCmd)
	if err != nil {
		// Check if this was a timeout and provide descriptive error
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("coordinator spawn timed out after %v: %w", i.cfg.Timeouts.CoordinatorStart, err)
		}
		return fmt.Errorf("spawning coordinator via command: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("spawn coordinator command failed: %w", result.Error)
	}

	log.Debug(log.CatOrch, "Coordinator spawned via v2 command processor", "subsystem", "init")

	return nil
}

// handleV2Event processes v2 orchestration events from the unified v2EventBus.
// This drives phase transitions based on coordinator ProcessReady events.
// With lazy spawning, initialization completes when coordinator's first turn finishes.
func (i *Initializer) handleV2Event(event pubsub.Event[any]) {
	if payload, ok := event.Payload.(events.ProcessEvent); ok {
		if payload.Role == events.RoleCoordinator {
			i.handleCoordinatorProcessEvent(payload)
		}
	}
}

// handleCoordinatorProcessEvent processes coordinator events from the v2EventBus.
// Drives phase transitions when coordinator's first turn completes (ProcessReady).
// This is more reliable than waiting for ProcessOutput because it's triggered by
// the authoritative status transition, not dependent on LLM generating output.
func (i *Initializer) handleCoordinatorProcessEvent(payload events.ProcessEvent) {
	i.mu.Lock()
	phase := i.phase
	i.mu.Unlock()

	// Handle error events (e.g., turn.failed from Codex with usage limit errors)
	if payload.Type == events.ProcessError {
		if payload.Error != nil {
			i.fail(payload.Error)
		} else {
			i.fail(fmt.Errorf("coordinator process error"))
		}
		return
	}

	// Detect coordinator's first turn completing for phase transition
	// ProcessReady is emitted when ProcessTurnCompleteHandler transitions status to Ready
	// This is authoritative - the coordinator's first turn has fully completed
	// With lazy spawning, we transition directly to InitReady (no workers spawn during init)
	if phase == InitAwaitingFirstMessage && payload.Type == events.ProcessReady {
		log.Debug(log.CatOrch, "coordinator first turn complete (ProcessReady), initialization ready", "subsystem", "init")
		i.transitionTo(InitReady)
		i.publishEvent(InitializerEvent{
			Type:  InitEventReady,
			Phase: InitReady,
		})
		// Signal run() loop to exit
		close(i.readyChan)
	}
}

// transitionTo updates the phase and publishes a phase change event.
func (i *Initializer) transitionTo(phase InitPhase) {
	i.mu.Lock()
	oldPhase := i.phase
	i.phase = phase
	i.mu.Unlock()

	log.Debug(log.CatOrch, "Phase transition", "subsystem", "init", "from", oldPhase, "to", phase)

	i.publishEvent(InitializerEvent{
		Type:  InitEventPhaseChanged,
		Phase: phase,
	})
}

// fail transitions to failed state and publishes a failed event.
// If already failed, this is a no-op to preserve the original failure phase.
// Cancels the context to stop the run() loop and prevent timeout from firing.
func (i *Initializer) fail(err error) {
	i.mu.Lock()
	// Don't overwrite if already failed - preserve original failure phase
	if i.phase == InitFailed || i.phase == InitTimedOut {
		i.mu.Unlock()
		return
	}
	i.failedAtPhase = i.phase
	i.phase = InitFailed
	i.err = err
	cancel := i.cancel
	i.mu.Unlock()

	log.Error(log.CatOrch, "Initialization failed", "subsystem", "init", "phase", i.failedAtPhase, "error", err)

	i.publishEvent(InitializerEvent{
		Type:  InitEventFailed,
		Phase: InitFailed,
		Error: err,
	})

	// Cancel context to stop run() loop and prevent timeout from firing
	if cancel != nil {
		cancel()
	}
}

// timeoutWithMaxTotal transitions to timed out state when MaxTotal timer fires.
// The error indicates both the current phase AND that max total was exceeded.
// If already failed, this is a no-op to preserve the original failure.
// This method also cancels the context to stop the run() loop.
func (i *Initializer) timeoutWithMaxTotal() {
	i.mu.Lock()
	// Don't overwrite if already failed - preserve original failure
	if i.phase == InitFailed || i.phase == InitTimedOut {
		i.mu.Unlock()
		return
	}
	i.failedAtPhase = i.phase
	i.phase = InitTimedOut
	// Set error to indicate both phase and max total exceeded
	i.err = fmt.Errorf("max total timeout (%v) exceeded during %v phase", i.cfg.Timeouts.MaxTotal, i.failedAtPhase)
	cancel := i.cancel
	i.mu.Unlock()

	log.Error(log.CatOrch, "Initialization timed out (max total exceeded)", "subsystem", "init",
		"phase", i.failedAtPhase, "maxTotal", i.cfg.Timeouts.MaxTotal)

	i.publishEvent(InitializerEvent{
		Type:  InitEventTimedOut,
		Phase: InitTimedOut,
		Error: i.err,
	})

	// Cancel context to stop run() loop
	if cancel != nil {
		cancel()
	}
}

// publishEvent publishes an event to subscribers.
func (i *Initializer) publishEvent(event InitializerEvent) {
	i.broker.Publish(pubsub.UpdatedEvent, event)
}

// cleanupResources releases all resources in reverse order of creation.
// This method is idempotent - safe to call multiple times without side effects.
//
// Cleanup order (reverse of creation in createWorkspace):
//  1. Stop all processes and drain command processor (via v2Infra.Shutdown())
//  2. Stop deduplication middleware (created with v2 infrastructure)
//  3. Shutdown MCP server with timeout (HTTP server started last in createWorkspace)
//  4. Shutdown tracing provider to flush pending spans
func (i *Initializer) cleanupResources() {
	i.mu.Lock()
	mcpServer := i.mcpServer
	v2Infra := i.v2Infra
	tracingProvider := i.tracingProvider
	sess := i.session
	phase := i.phase

	i.mcpServer = nil
	i.v2Infra = nil
	i.tracingProvider = nil
	i.tracer = nil
	i.session = nil
	i.mu.Unlock()

	if v2Infra != nil {
		v2Infra.Shutdown()
	}

	if mcpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = mcpServer.Shutdown(ctx)
		cancel()
	}

	// Close session only if initialization never completed (InitReady).
	// Once initialization completes, the Model takes ownership via Resources()
	// and is responsible for closing the session with the appropriate status
	// via Model.Cleanup() -> determineSessionStatus().
	//
	// If we didn't reach InitReady, the Model never got the session, so we must
	// close it here to release file handles
	if sess != nil && phase != InitReady {
		_ = sess.Close(session.StatusFailed)
	}

	// Shutdown tracing provider last to ensure all spans are flushed
	if tracingProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = tracingProvider.Shutdown(ctx)
		cancel()
	}
}
