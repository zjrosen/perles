package mcp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/v2/adapter"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/v2/repository"
)

// ===========================================================================
// V2 Test Helpers
// ===========================================================================

// testV2Handler is a controllable v2 handler for testing.
type testV2Handler struct {
	mu       sync.Mutex
	results  map[command.CommandType]*command.CommandResult
	commands []command.Command
}

func newTestV2Handler() *testV2Handler {
	return &testV2Handler{
		results:  make(map[command.CommandType]*command.CommandResult),
		commands: make([]command.Command, 0),
	}
}

func (h *testV2Handler) SetResult(result *command.CommandResult) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Set as default result for any command type
	h.results[command.CmdSpawnProcess] = result
	h.results[command.CmdRetireProcess] = result
	h.results[command.CmdReplaceProcess] = result
	h.results[command.CmdAssignTask] = result
	h.results[command.CmdSendToProcess] = result
	h.results[command.CmdBroadcast] = result
	h.results[command.CmdAssignReview] = result
	h.results[command.CmdReportComplete] = result
	h.results[command.CmdReportVerdict] = result
	h.results[command.CmdMarkTaskComplete] = result
	h.results[command.CmdMarkTaskFailed] = result
	h.results[command.CmdApproveCommit] = result
	h.results[command.CmdStopProcess] = result
	h.results[command.CmdSignalWorkflowComplete] = result
}

func (h *testV2Handler) GetCommands() []command.Command {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]command.Command, len(h.commands))
	copy(result, h.commands)
	return result
}

// Handle implements processor.CommandHandler
func (h *testV2Handler) Handle(ctx context.Context, cmd command.Command) (*command.CommandResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.commands = append(h.commands, cmd)
	if result, ok := h.results[cmd.Type()]; ok {
		return result, nil
	}
	return &command.CommandResult{Success: true}, nil
}

// injectV2AdapterToCoordinator sets up a v2 adapter with a test handler for testing.
func injectV2AdapterToCoordinator(t *testing.T, cs *CoordinatorServer) (*testV2Handler, func()) {
	t.Helper()

	handler := newTestV2Handler()

	// Create processor with handler
	proc := processor.NewCommandProcessor(
		processor.WithQueueCapacity(100),
		processor.WithTaskRepository(repository.NewMemoryTaskRepository()),
		processor.WithQueueRepository(repository.NewMemoryQueueRepository(100)),
	)

	// Register the handler for all command types
	proc.RegisterHandler(command.CmdSpawnProcess, handler)
	proc.RegisterHandler(command.CmdRetireProcess, handler)
	proc.RegisterHandler(command.CmdReplaceProcess, handler)
	proc.RegisterHandler(command.CmdAssignTask, handler)
	proc.RegisterHandler(command.CmdSendToProcess, handler)
	proc.RegisterHandler(command.CmdBroadcast, handler)
	proc.RegisterHandler(command.CmdAssignReview, handler)
	proc.RegisterHandler(command.CmdReportComplete, handler)
	proc.RegisterHandler(command.CmdReportVerdict, handler)
	proc.RegisterHandler(command.CmdMarkTaskComplete, handler)
	proc.RegisterHandler(command.CmdMarkTaskFailed, handler)
	proc.RegisterHandler(command.CmdApproveCommit, handler)
	proc.RegisterHandler(command.CmdAssignReviewFeedback, handler)
	proc.RegisterHandler(command.CmdStopProcess, handler)
	proc.RegisterHandler(command.CmdSignalWorkflowComplete, handler)

	// Start processor in background
	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	// Wait for processor to be ready
	time.Sleep(10 * time.Millisecond)

	// Create and set adapter
	v2Adapter := adapter.NewV2Adapter(proc,
		adapter.WithProcessRepository(repository.NewMemoryProcessRepository()),
		adapter.WithTaskRepository(repository.NewMemoryTaskRepository()),
		adapter.WithQueueRepository(repository.NewMemoryQueueRepository(100)),
		adapter.WithTimeout(5*time.Second),
	)
	cs.v2Adapter = v2Adapter

	return handler, func() {
		cancel()
		proc.Stop()
	}
}

// TestCoordinatorServerWrapper wraps CoordinatorServer with test-specific helpers.
type TestCoordinatorServerWrapper struct {
	*CoordinatorServer
	ProcessRepo repository.ProcessRepository
	cleanup     func()
}

// Close cleans up the test server resources.
func (w *TestCoordinatorServerWrapper) Close() {
	if w.cleanup != nil {
		w.cleanup()
	}
}

// NewTestCoordinatorServer creates a coordinator server with v2 adapter for testing.
func NewTestCoordinatorServer(t *testing.T) *TestCoordinatorServerWrapper {
	t.Helper()

	msgLog := repository.NewMemoryMessageRepository()
	cs := NewCoordinatorServer(
		msgLog,
		"/tmp/test",
		8765,
		mocks.NewMockIssueExecutor(t),
	)

	// Create shared worker repository
	processRepo := repository.NewMemoryProcessRepository()

	// Create processor with handler
	handler := newTestV2Handler()
	proc := processor.NewCommandProcessor(
		processor.WithQueueCapacity(100),
		processor.WithTaskRepository(repository.NewMemoryTaskRepository()),
		processor.WithQueueRepository(repository.NewMemoryQueueRepository(100)),
	)

	// Register the handler for all command types
	proc.RegisterHandler(command.CmdSpawnProcess, handler)
	proc.RegisterHandler(command.CmdRetireProcess, handler)
	proc.RegisterHandler(command.CmdReplaceProcess, handler)
	proc.RegisterHandler(command.CmdAssignTask, handler)
	proc.RegisterHandler(command.CmdSendToProcess, handler)
	proc.RegisterHandler(command.CmdBroadcast, handler)
	proc.RegisterHandler(command.CmdAssignReview, handler)
	proc.RegisterHandler(command.CmdReportComplete, handler)
	proc.RegisterHandler(command.CmdReportVerdict, handler)
	proc.RegisterHandler(command.CmdMarkTaskComplete, handler)
	proc.RegisterHandler(command.CmdMarkTaskFailed, handler)
	proc.RegisterHandler(command.CmdApproveCommit, handler)
	proc.RegisterHandler(command.CmdAssignReviewFeedback, handler)
	proc.RegisterHandler(command.CmdStopProcess, handler)
	proc.RegisterHandler(command.CmdSignalWorkflowComplete, handler)

	// Start processor in background
	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	// Wait for processor to be ready
	time.Sleep(10 * time.Millisecond)

	// Create and set adapter with same processRepo
	v2Adapter := adapter.NewV2Adapter(proc,
		adapter.WithProcessRepository(processRepo),
		adapter.WithTaskRepository(repository.NewMemoryTaskRepository()),
		adapter.WithQueueRepository(repository.NewMemoryQueueRepository(100)),
		adapter.WithTimeout(5*time.Second),
	)
	cs.v2Adapter = v2Adapter

	return &TestCoordinatorServerWrapper{
		CoordinatorServer: cs,
		ProcessRepo:       processRepo,
		cleanup: func() {
			cancel()
			proc.Stop()
		},
	}
}

// requireV2CommandReceived verifies that a command of the given type was received.
func requireV2CommandReceived(t *testing.T, handler *testV2Handler, cmdType command.CommandType) {
	t.Helper()
	cmds := handler.GetCommands()
	for _, cmd := range cmds {
		if cmd.Type() == cmdType {
			return
		}
	}
	require.Failf(t, "command not received", "expected command type %v, got %v", cmdType, cmds)
}

// ===========================================================================
// Worker Test Helpers
// ===========================================================================

// TestWorkerServerWrapper wraps WorkerServer with test-specific helpers.
type TestWorkerServerWrapper struct {
	*WorkerServer
	V2Handler *testV2Handler
	cleanup   func()
}

// Close cleans up the test server resources.
func (w *TestWorkerServerWrapper) Close() {
	if w.cleanup != nil {
		w.cleanup()
	}
}

// NewTestWorkerServer creates a worker server with v2 adapter for testing.
func NewTestWorkerServer(t *testing.T, workerID string, store MessageStore) *TestWorkerServerWrapper {
	t.Helper()

	ws := NewWorkerServer(workerID, store)

	// Create shared worker repository with a mock worker
	processRepo := repository.NewMemoryProcessRepository()

	// Create processor with handler
	handler := newTestV2Handler()
	proc := processor.NewCommandProcessor(
		processor.WithQueueCapacity(100),
		processor.WithTaskRepository(repository.NewMemoryTaskRepository()),
		processor.WithQueueRepository(repository.NewMemoryQueueRepository(100)),
	)

	// Register the handler for all command types
	proc.RegisterHandler(command.CmdSpawnProcess, handler)
	proc.RegisterHandler(command.CmdRetireProcess, handler)
	proc.RegisterHandler(command.CmdReplaceProcess, handler)
	proc.RegisterHandler(command.CmdAssignTask, handler)
	proc.RegisterHandler(command.CmdSendToProcess, handler)
	proc.RegisterHandler(command.CmdBroadcast, handler)
	proc.RegisterHandler(command.CmdAssignReview, handler)
	proc.RegisterHandler(command.CmdReportComplete, handler)
	proc.RegisterHandler(command.CmdReportVerdict, handler)
	proc.RegisterHandler(command.CmdMarkTaskComplete, handler)
	proc.RegisterHandler(command.CmdMarkTaskFailed, handler)
	proc.RegisterHandler(command.CmdApproveCommit, handler)
	proc.RegisterHandler(command.CmdAssignReviewFeedback, handler)
	proc.RegisterHandler(command.CmdStopProcess, handler)
	proc.RegisterHandler(command.CmdSignalWorkflowComplete, handler)

	// Start processor in background
	ctx, cancel := context.WithCancel(context.Background())
	go proc.Run(ctx)

	// Wait for processor to be ready
	time.Sleep(10 * time.Millisecond)

	// Create and set adapter with same processRepo
	v2Adapter := adapter.NewV2Adapter(proc,
		adapter.WithProcessRepository(processRepo),
		adapter.WithTaskRepository(repository.NewMemoryTaskRepository()),
		adapter.WithQueueRepository(repository.NewMemoryQueueRepository(100)),
		adapter.WithTimeout(5*time.Second),
	)
	ws.v2Adapter = v2Adapter

	return &TestWorkerServerWrapper{
		WorkerServer: ws,
		V2Handler:    handler,
		cleanup: func() {
			cancel()
			proc.Stop()
		},
	}
}
