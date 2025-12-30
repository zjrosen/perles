package orchestration

import (
	"context"
	"fmt"
	"maps"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/amp"
	"github.com/zjrosen/perles/internal/orchestration/client"
	"github.com/zjrosen/perles/internal/orchestration/coordinator"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/mcp"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/pool"
	"github.com/zjrosen/perles/internal/orchestration/session"
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
	WorkDir         string
	ClientType      string
	ClaudeModel     string
	AmpModel        string
	AmpMode         string
	ExpectedWorkers int
	Timeout         time.Duration
}

// InitializerResources holds the resources created during initialization.
// These are transferred to the Model when initialization completes.
type InitializerResources struct {
	AIClient          client.HeadlessClient
	Extensions        map[string]any
	Pool              *pool.WorkerPool
	MessageLog        *message.Issue
	MCPServer         *http.Server
	MCPPort           int // Dynamic port the MCP server is listening on
	Coordinator       *coordinator.Coordinator
	WorkerServerCache *workerServerCache
	Session           *session.Session // Session tracking for this orchestration run
}

// Initializer manages the orchestration initialization lifecycle as a state machine.
// It subscribes to coordinator, worker, and message events to drive phase transitions,
// and publishes high-level events for the TUI to consume.
type Initializer struct {
	// Configuration
	cfg InitializerConfig

	// State (protected by mu)
	phase            InitPhase
	failedAtPhase    InitPhase // The phase we were in when failure/timeout occurred
	workersSpawned   int
	confirmedWorkers map[string]bool
	startTime        time.Time
	err              error

	// Resources created during initialization
	aiClient           client.HeadlessClient
	aiClientExtensions map[string]any
	pool               *pool.WorkerPool
	messageLog         *message.Issue
	mcpListener        net.Listener // Listener for dynamic port
	mcpPort            int          // Assigned port
	mcpServer          *http.Server
	mcpCoordServer     *mcp.CoordinatorServer
	coord              *coordinator.Coordinator
	workerServers      *workerServerCache
	session            *session.Session // Session tracking

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
	if cfg.ExpectedWorkers == 0 {
		cfg.ExpectedWorkers = 4
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 20 * time.Second
	}

	return &Initializer{
		cfg:              cfg,
		phase:            InitNotStarted,
		confirmedWorkers: make(map[string]bool),
		broker:           pubsub.NewBroker[InitializerEvent](),
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

// SpinnerData returns data needed for spinner rendering.
func (i *Initializer) SpinnerData() (phase InitPhase, workersSpawned, expectedWorkers int, confirmedWorkers map[string]bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	// Return a copy of confirmedWorkers to avoid races
	confirmed := make(map[string]bool, len(i.confirmedWorkers))
	maps.Copy(confirmed, i.confirmedWorkers)

	return i.phase, i.workersSpawned, i.cfg.ExpectedWorkers, confirmed
}

// Resources returns the initialized resources.
// Only valid after receiving InitEventReady.
func (i *Initializer) Resources() InitializerResources {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return InitializerResources{
		AIClient:          i.aiClient,
		Extensions:        i.aiClientExtensions,
		Pool:              i.pool,
		MessageLog:        i.messageLog,
		MCPServer:         i.mcpServer,
		MCPPort:           i.mcpPort,
		Coordinator:       i.coord,
		WorkerServerCache: i.workerServers,
		Session:           i.session,
	}
}

// GetCoordinator returns the coordinator if it has been created, nil otherwise.
// This allows the TUI to set up event subscriptions as soon as the coordinator exists.
func (i *Initializer) GetCoordinator() *coordinator.Coordinator {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.coord
}

// GetMessageLog returns the message log if it has been created, nil otherwise.
func (i *Initializer) GetMessageLog() *message.Issue {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.messageLog
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
	i.workersSpawned = 0
	i.confirmedWorkers = make(map[string]bool)
	i.err = nil
	i.started = false
	i.aiClient = nil
	i.aiClientExtensions = nil
	i.pool = nil
	i.messageLog = nil
	i.mcpListener = nil
	i.mcpPort = 0
	i.mcpServer = nil
	i.mcpCoordServer = nil
	i.coord = nil
	i.workerServers = nil
	i.session = nil
	i.mu.Unlock()

	return i.Start()
}

// Cancel stops initialization and cleans up resources.
func (i *Initializer) Cancel() {
	i.mu.Lock()
	if i.cancel != nil {
		i.cancel()
	}
	i.mu.Unlock()

	i.cleanup()
}

// run is the main initialization goroutine.
func (i *Initializer) run() {
	// Start timeout timer
	timeoutTimer := time.NewTimer(i.cfg.Timeout)
	defer timeoutTimer.Stop()

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

	// Attach session to coordinator broker now that coordinator exists
	// (Pool and Message brokers were attached in createWorkspace(), MCP broker attached after mcpCoordServer creation)
	i.mu.RLock()
	sess := i.session
	i.mu.RUnlock()
	if sess != nil {
		sess.AttachCoordinatorBroker(i.ctx, i.coord.Broker())
	}

	// Phase 3+: Event-driven phases
	// Subscribe to all event sources and process them
	coordSub := i.coord.Broker().Subscribe(i.ctx)
	workerSub := i.coord.Workers().Subscribe(i.ctx)
	msgSub := i.messageLog.Broker().Subscribe(i.ctx)

	// Transition to awaiting first message
	i.transitionTo(InitAwaitingFirstMessage)

	for {
		select {
		case <-i.ctx.Done():
			return

		case <-timeoutTimer.C:
			i.timeout()
			return

		case event, ok := <-coordSub:
			if !ok {
				return
			}
			if i.handleCoordinatorEvent(event) {
				return // Ready or terminal state
			}

		case event, ok := <-workerSub:
			if !ok {
				return
			}
			if i.handleWorkerEvent(event) {
				return // Ready or terminal state
			}

		case event, ok := <-msgSub:
			if !ok {
				return
			}
			if i.handleMessageEvent(event) {
				return // Ready or terminal state
			}
		}
	}
}

// createWorkspace creates the AI client, worker pool, message log, and MCP server.
func (i *Initializer) createWorkspace() error {
	// 1. Create AI client
	clientType := client.ClientType(i.cfg.ClientType)
	if clientType == "" {
		clientType = client.ClientClaude
	}

	aiClient, err := client.NewClient(clientType)
	if err != nil {
		return fmt.Errorf("failed to create AI client: %w", err)
	}

	// Build extensions map for provider-specific configuration
	extensions := make(map[string]any)
	switch clientType {
	case client.ClientClaude:
		if i.cfg.ClaudeModel != "" {
			extensions[client.ExtClaudeModel] = i.cfg.ClaudeModel
		}
	case client.ClientAmp:
		if i.cfg.AmpModel != "" {
			extensions[client.ExtAmpModel] = i.cfg.AmpModel
		}
		if i.cfg.AmpMode != "" {
			extensions[amp.ExtAmpMode] = i.cfg.AmpMode
		}
	}

	i.mu.Lock()
	i.aiClient = aiClient
	i.aiClientExtensions = extensions
	i.mu.Unlock()

	// 2. Create worker pool
	workerPool := pool.NewWorkerPool(pool.Config{
		Client: aiClient,
	})
	i.mu.Lock()
	i.pool = workerPool
	i.mu.Unlock()

	// 3. Create message log
	msgLog := message.New()
	i.mu.Lock()
	i.messageLog = msgLog
	i.mu.Unlock()

	// 4. Create session for tracking this orchestration run
	sessionID := uuid.New().String()
	sessionDir := filepath.Join(i.cfg.WorkDir, ".perles", "sessions", sessionID)
	sess, err := session.New(sessionID, sessionDir)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	i.mu.Lock()
	i.session = sess
	i.mu.Unlock()

	log.Debug(log.CatOrch, "Session created", "subsystem", "init", "sessionID", sessionID, "dir", sessionDir)

	// Attach session to pool.Broker() and msgIssue.Broker() immediately
	// (they exist at this point).
	// Broker attachment order:
	// 1. Pool and Message brokers - attached here in createWorkspace() (they exist now)
	// 2. MCP broker - attached below after mcpCoordServer is created
	// 3. Coordinator broker - attached later in run() after spawnCoordinator() completes
	sess.AttachToBrokers(i.ctx, nil, workerPool.Broker(), msgLog.Broker(), nil)

	// 5. Start MCP server with dynamic port
	// Create listener on localhost:0 to get a random available port
	// Using localhost (127.0.0.1) to avoid binding to all interfaces
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to create MCP listener: %w", err)
	}

	// Extract the assigned port
	tcpAddr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return fmt.Errorf("failed to get TCP address from listener")
	}
	port := tcpAddr.Port
	log.Debug(log.CatOrch, "MCP server listening on dynamic port", "subsystem", "init", "port", port)

	// Create coordinator server with the dynamic port
	mcpCoordServer := mcp.NewCoordinatorServer(aiClient, workerPool, msgLog, i.cfg.WorkDir, port, extensions, beads.NewRealExecutor(i.cfg.WorkDir))
	// Pass the coordinator server as the state callback so workers can update coordinator state
	workerServers := newWorkerServerCache(msgLog, mcpCoordServer)

	// Attach session to MCP broker now that mcpCoordServer exists
	sess.AttachMCPBroker(i.ctx, mcpCoordServer.Broker())

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpCoordServer.ServeHTTP())
	mux.HandleFunc("/worker/", workerServers.ServeHTTP)

	httpServer := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	i.mu.Lock()
	i.mcpListener = listener
	i.mcpPort = port
	i.mcpServer = httpServer
	i.mcpCoordServer = mcpCoordServer
	i.workerServers = workerServers
	i.mu.Unlock()

	// Start HTTP server in background using the listener
	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Error(log.CatOrch, "MCP server error", "subsystem", "init", "error", err)
		}
	}()

	return nil
}

// spawnCoordinator creates and starts the coordinator.
func (i *Initializer) spawnCoordinator() error {
	i.mu.RLock()
	aiClient := i.aiClient
	workerPool := i.pool
	msgLog := i.messageLog
	port := i.mcpPort
	i.mu.RUnlock()

	if aiClient == nil || workerPool == nil || msgLog == nil {
		return fmt.Errorf("prerequisites not initialized")
	}

	coordCfg := coordinator.Config{
		WorkDir:      i.cfg.WorkDir,
		Client:       aiClient,
		Pool:         workerPool,
		MessageIssue: msgLog,
		Port:         port,
	}

	coord, err := coordinator.New(coordCfg)
	if err != nil {
		return err
	}

	i.mu.Lock()
	i.coord = coord
	i.mu.Unlock()

	if err := coord.Start(); err != nil {
		return err
	}

	log.Debug(log.CatOrch, "Coordinator started", "subsystem", "init")
	return nil
}

// spawnWorkers spawns all workers programmatically.
// This runs in a goroutine to not block the event loop.
func (i *Initializer) spawnWorkers() {
	i.mu.RLock()
	mcpCoordServer := i.mcpCoordServer
	expected := i.cfg.ExpectedWorkers
	i.mu.RUnlock()

	if mcpCoordServer == nil {
		log.Error(log.CatOrch, "Cannot spawn workers: MCP server not initialized", "subsystem", "init")
		return
	}

	for j := range expected {
		workerID, err := mcpCoordServer.SpawnIdleWorker()
		if err != nil {
			log.Error(log.CatOrch, "Failed to spawn worker", "subsystem", "init", "index", j, "error", err)
			// Continue trying to spawn remaining workers
			continue
		}
		log.Debug(log.CatOrch, "Spawned worker programmatically", "subsystem", "init", "workerID", workerID, "index", j)
	}
}

// handleCoordinatorEvent processes coordinator events.
// Returns true if initialization reached a terminal state.
func (i *Initializer) handleCoordinatorEvent(event pubsub.Event[events.CoordinatorEvent]) bool {
	payload := event.Payload

	i.mu.Lock()
	phase := i.phase
	i.mu.Unlock()

	// Detect first coordinator message for phase transition
	if phase == InitAwaitingFirstMessage && payload.Type == events.CoordinatorChat {
		log.Debug(log.CatOrch, "First coordinator message received, spawning workers", "subsystem", "init")
		i.transitionTo(InitSpawningWorkers)

		// Spawn workers programmatically (don't rely on coordinator LLM)
		go i.spawnWorkers()
	}

	return false
}

// handleWorkerEvent processes worker events.
// Returns true if initialization reached a terminal state.
func (i *Initializer) handleWorkerEvent(event pubsub.Event[events.WorkerEvent]) bool {
	payload := event.Payload

	if payload.Type != events.WorkerSpawned {
		return false
	}

	i.mu.Lock()
	phase := i.phase
	i.workersSpawned++
	spawned := i.workersSpawned
	expected := i.cfg.ExpectedWorkers
	i.mu.Unlock()

	log.Debug(log.CatOrch, "Worker spawned",
		"subsystem", "init",
		"workerID", payload.WorkerID,
		"spawned", spawned,
		"expected", expected)

	// Check if all workers spawned
	if phase == InitSpawningWorkers && spawned >= expected {
		i.transitionTo(InitWorkersReady)
		// Check if we already have enough confirmed workers (race condition handling)
		return i.checkWorkersConfirmed()
	}

	return false
}

// handleMessageEvent processes message events.
// Returns true if initialization reached a terminal state.
func (i *Initializer) handleMessageEvent(event pubsub.Event[message.Event]) bool {
	payload := event.Payload

	if payload.Type != message.EventPosted {
		return false
	}

	entry := payload.Entry

	// Only track worker ready messages
	if entry.Type != message.MessageWorkerReady {
		return false
	}

	i.mu.Lock()
	phase := i.phase

	// Track during both SpawningWorkers and WorkersReady phases (messages may arrive early)
	if phase != InitSpawningWorkers && phase != InitWorkersReady {
		i.mu.Unlock()
		return false
	}

	if !i.confirmedWorkers[entry.From] {
		i.confirmedWorkers[entry.From] = true
		log.Debug(log.CatOrch, "Worker confirmed",
			"subsystem", "init",
			"workerID", entry.From,
			"confirmed", len(i.confirmedWorkers),
			"expected", i.cfg.ExpectedWorkers)
	}
	i.mu.Unlock()

	// Only transition to ready if we're in WorkersReady phase
	return i.checkWorkersConfirmed()
}

// checkWorkersConfirmed checks if all workers are confirmed and transitions to Ready if so.
// Returns true if transitioned to Ready.
func (i *Initializer) checkWorkersConfirmed() bool {
	i.mu.Lock()
	phase := i.phase
	confirmed := len(i.confirmedWorkers)
	expected := i.cfg.ExpectedWorkers
	i.mu.Unlock()

	if phase == InitWorkersReady && confirmed >= expected {
		log.Debug(log.CatOrch, "All workers confirmed, transitioning to ready", "subsystem", "init")
		i.transitionTo(InitReady)
		i.publishEvent(InitializerEvent{
			Type:  InitEventReady,
			Phase: InitReady,
		})
		return true
	}

	return false
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
func (i *Initializer) fail(err error) {
	i.mu.Lock()
	i.failedAtPhase = i.phase
	i.phase = InitFailed
	i.err = err
	i.mu.Unlock()

	log.Error(log.CatOrch, "Initialization failed", "subsystem", "init", "phase", i.failedAtPhase, "error", err)

	i.publishEvent(InitializerEvent{
		Type:  InitEventFailed,
		Phase: InitFailed,
		Error: err,
	})
}

// timeout transitions to timed out state and publishes a timeout event.
func (i *Initializer) timeout() {
	i.mu.Lock()
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

// cleanup releases all resources.
func (i *Initializer) cleanup() {
	i.mu.Lock()
	mcpServer := i.mcpServer
	coord := i.coord
	i.mu.Unlock()

	if mcpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = mcpServer.Shutdown(ctx)
		cancel()
	}

	if coord != nil {
		_ = coord.Cancel()
	}
}
