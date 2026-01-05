package orchestration

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/git"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/amp"
	"github.com/zjrosen/perles/internal/orchestration/client"
	_ "github.com/zjrosen/perles/internal/orchestration/codex" // Register codex client
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/mcp"
	"github.com/zjrosen/perles/internal/orchestration/session"
	v2 "github.com/zjrosen/perles/internal/orchestration/v2"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/process"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
	"github.com/zjrosen/perles/internal/pubsub"
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
	WorkDir     string
	ClientType  string
	CodexModel  string
	ClaudeModel string
	AmpModel    string
	AmpMode     string
	Timeout     time.Duration
	// Worktree configuration
	WorktreeBaseBranch string          // Branch to base worktree on. Empty = skip worktree creation
	GitExecutor        git.GitExecutor // Injected for testability
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

	// Resources created during initialization
	messageRepo    *repository.MemoryMessageRepository // Message repository for inter-agent messaging
	mcpPort        int                                 // Assigned port
	mcpServer      *http.Server
	mcpCoordServer *mcp.CoordinatorServer
	session        *session.Session // Session tracking

	// V2 orchestration infrastructure (created by v2.NewInfrastructure factory)
	// Contains Core.Processor, Core.EventBus, Core.CmdSubmitter, and Repositories.ProcessRepo
	v2Infra *v2.Infrastructure

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
func NewInitializer(cfg InitializerConfig) *Initializer {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	return &Initializer{
		cfg:       cfg,
		phase:     InitNotStarted,
		readyChan: make(chan struct{}),
		broker:    pubsub.NewBroker[InitializerEvent](),
	}
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
func (i *Initializer) run() {
	// Start timeout timer
	timeoutTimer := time.NewTimer(i.cfg.Timeout)
	defer timeoutTimer.Stop()

	// Phase 0: Create worktree if branch specified
	if i.cfg.WorktreeBaseBranch != "" && i.cfg.GitExecutor != nil {
		i.transitionTo(InitCreatingWorktree)
		if err := i.createWorktree(); err != nil {
			i.fail(err)
			return
		}
	}

	// Phase 1: Create workspace
	i.transitionTo(InitCreatingWorkspace)
	if err := i.createWorkspace(); err != nil {
		i.fail(err)
		return
	}

	// Phase 2: Spawn coordinator
	i.transitionTo(InitSpawningCoordinator)
	if err := i.spawnCoordinator(); err != nil {
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

		case <-timeoutTimer.C:
			// Initialization timed out
			i.timeout()
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

// createSession creates a new orchestration session with its directory structure.
// It uses the session ID generated during Start(), creates the session directory,
// and initializes the session tracking object.
func (i *Initializer) createSession() (*session.Session, error) {
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

	sessionDir := filepath.Join(effectiveWorkDir, ".perles", "sessions", sessionID)

	sess, err := session.New(sessionID, sessionDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	log.Debug(log.CatOrch, "Session created", "subsystem", "init", "sessionID", sessionID, "dir", sessionDir)
	return sess, nil
}

// createWorktree creates a git worktree for isolated workspace operation.
// It performs pre-flight checks, determines the worktree path, and creates
// the worktree with an auto-generated or configured branch name.
func (i *Initializer) createWorktree() error {
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

	// Auto-generate branch name using first 8 chars of session ID
	shortID := sessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	newBranch := fmt.Sprintf("perles-session-%s", shortID)

	// Base branch is what the user selected (main, current branch, etc.)
	// Empty means use current HEAD
	baseBranch := i.cfg.WorktreeBaseBranch

	// Create the worktree with new branch based on selected branch
	if err := gitExec.CreateWorktree(path, newBranch, baseBranch); err != nil {
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

// AIClientResult holds the result of createAIClient().
type AIClientResult struct {
	Client     client.HeadlessClient
	Extensions map[string]any
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

// createAIClient creates the AI client and builds the provider-specific extensions map.
// It determines the client type from config (defaulting to Claude), creates the client,
// and populates the extensions map with model and mode settings.
func (i *Initializer) createAIClient() (*AIClientResult, error) {
	// Determine client type, default to Claude if empty
	clientType := client.ClientType(i.cfg.ClientType)
	if clientType == "" {
		clientType = client.ClientClaude
	}

	// Create the AI client
	aiClient, err := client.NewClient(clientType)
	if err != nil {
		return nil, fmt.Errorf("failed to create AI client: %w", err)
	}

	// Build extensions map for provider-specific configuration
	extensions := make(map[string]any)
	switch clientType {
	case client.ClientClaude:
		if i.cfg.ClaudeModel != "" {
			extensions[client.ExtClaudeModel] = i.cfg.ClaudeModel
		}
	case client.ClientCodex:
		if i.cfg.CodexModel != "" {
			extensions[client.ExtCodexModel] = i.cfg.CodexModel
		}
	case client.ClientAmp:
		if i.cfg.AmpModel != "" {
			extensions[client.ExtAmpModel] = i.cfg.AmpModel
		}
		if i.cfg.AmpMode != "" {
			extensions[amp.ExtAmpMode] = i.cfg.AmpMode
		}
	}

	return &AIClientResult{
		Client:     aiClient,
		Extensions: extensions,
	}, nil
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
	AIClient     client.HeadlessClient               // AI client for coordinator server
	MsgRepo      *repository.MemoryMessageRepository // Message repository for coordinator server
	Session      *session.Session                    // Session for reflection writing
	V2Adapter    *adapter.V2Adapter                  // V2 adapter for routing
	TurnEnforcer mcp.ToolCallRecorder                // Turn completion enforcer for workers
	WorkDir      string                              // Working directory
	Extensions   map[string]any                      // Provider-specific extensions
}

// createMCPServer creates the MCP server with HTTP routes for coordinator and worker endpoints.
// It requires a pre-created listener (from createMCPListener) and all dependencies.
func (i *Initializer) createMCPServer(cfg MCPServerConfig) (*MCPServerResult, error) {
	// Validate required config fields
	if cfg.Listener == nil {
		return nil, fmt.Errorf("listener is required")
	}
	if cfg.AIClient == nil {
		return nil, fmt.Errorf("AI client is required")
	}
	if cfg.MsgRepo == nil {
		return nil, fmt.Errorf("message repository is required")
	}

	// Create coordinator server with the dynamic port and v2 adapter
	mcpCoordServer := mcp.NewCoordinatorServerWithV2Adapter(
		cfg.AIClient, cfg.MsgRepo, cfg.WorkDir, cfg.Port, cfg.Extensions,
		beads.NewRealExecutor(cfg.WorkDir), cfg.V2Adapter)

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

// createWorkspace orchestrates the creation of all workspace resources needed for orchestration.
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
func (i *Initializer) createWorkspace() error {
	// Determine effective work directory (use worktree path if created)
	effectiveWorkDir := i.cfg.WorkDir
	i.mu.RLock()
	if i.worktreePath != "" {
		effectiveWorkDir = i.worktreePath
	}
	i.mu.RUnlock()

	// ============================================================
	// Step 1: Create AI client
	// ============================================================
	clientResult, err := i.createAIClient()
	if err != nil {
		return err
	}
	aiClient := clientResult.Client
	extensions := clientResult.Extensions

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
	v2Infra, err := v2.NewInfrastructure(v2.InfrastructureConfig{
		Port:        port,
		AIClient:    aiClient,
		WorkDir:     effectiveWorkDir, // Use worktree path when enabled
		Extensions:  extensions,
		MessageRepo: msgRepo,
		SessionID:   sess.ID,
	})
	if err != nil {
		_ = listenerResult.Listener.Close()
		return fmt.Errorf("creating v2 infrastructure: %w", err)
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
		AIClient:     aiClient,
		MsgRepo:      msgRepo,
		Session:      sess,
		V2Adapter:    v2Infra.Core.Adapter,
		TurnEnforcer: v2Infra.Internal.TurnEnforcer,
		WorkDir:      effectiveWorkDir, // Use worktree path when enabled
		Extensions:   extensions,
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

// spawnCoordinator creates and starts the coordinator using the v2 command processor.
// This submits a SpawnProcessCommand which handles all the AI spawning, registry registration,
// and ProcessRepository updates through the unified command pattern.
func (i *Initializer) spawnCoordinator() error {
	i.mu.RLock()
	v2Infra := i.v2Infra
	i.mu.RUnlock()

	if v2Infra == nil {
		return fmt.Errorf("v2 infrastructure not initialized")
	}

	// Create and submit SpawnProcessCommand for coordinator
	spawnCmd := command.NewSpawnProcessCommand(command.SourceUser, repository.RoleCoordinator)

	// Use SubmitAndWait to ensure coordinator is fully spawned before continuing
	result, err := v2Infra.Core.Processor.SubmitAndWait(i.ctx, spawnCmd)
	if err != nil {
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

// timeout transitions to timed out state and publishes a timeout event.
// If already failed, this is a no-op to preserve the original failure.
func (i *Initializer) timeout() {
	i.mu.Lock()
	// Don't overwrite if already failed - preserve original failure
	if i.phase == InitFailed || i.phase == InitTimedOut {
		i.mu.Unlock()
		return
	}
	i.failedAtPhase = i.phase
	i.phase = InitTimedOut
	i.mu.Unlock()

	log.Error(log.CatOrch, "Initialization timed out", "subsystem", "init", "phase", i.failedAtPhase)

	i.publishEvent(InitializerEvent{
		Type:  InitEventTimedOut,
		Phase: InitTimedOut,
	})
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
func (i *Initializer) cleanupResources() {
	i.mu.Lock()
	mcpServer := i.mcpServer
	v2Infra := i.v2Infra

	i.mcpServer = nil
	i.v2Infra = nil
	i.mu.Unlock()

	if v2Infra != nil {
		v2Infra.Shutdown()
	}

	if mcpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = mcpServer.Shutdown(ctx)
		cancel()
	}
}
