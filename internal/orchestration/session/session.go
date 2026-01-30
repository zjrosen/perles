// Package session provides session tracking for orchestration mode.
// It persists conversation history, inter-agent messages, and operational logs
// to disk in a UUID-based session folder structure.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/orchestration/v2/processor"
	"github.com/zjrosen/perles/internal/orchestration/workflow"
	"github.com/zjrosen/perles/internal/pubsub"
	"github.com/zjrosen/perles/internal/ui/shared/chatrender"
)

// Status represents the session lifecycle state.
type Status string

const (
	// StatusRunning means the session is active.
	StatusRunning Status = "running"
	// StatusCompleted means the session ended normally.
	StatusCompleted Status = "completed"
	// StatusFailed means the session ended due to an error.
	StatusFailed Status = "failed"
	// StatusTimedOut means the session ended due to timeout.
	StatusTimedOut Status = "timed_out"
)

// String returns the string representation of the status.
func (s Status) String() string {
	return string(s)
}

// Session represents an active orchestration session with file handles.
// All writes are centralized through this struct to avoid concurrent file access issues.
type Session struct {
	// ID is the unique session identifier (UUID).
	ID string

	// Dir is the full path to the session folder (.perles/sessions/{id}).
	Dir string

	// StartTime is when the session was created.
	StartTime time.Time

	// Status is the current session state.
	Status Status

	// BufferedWriters for log files.
	coordRaw         *BufferedWriter            // coordinator/raw.jsonl
	coordMessages    *BufferedWriter            // coordinator/messages.jsonl (structured chat)
	observerMessages *BufferedWriter            // observer/messages.jsonl (structured chat)
	workerRaws       map[string]*BufferedWriter // workerID -> workers/{id}/raw.jsonl
	workerMessages   map[string]*BufferedWriter // workerID -> workers/{id}/messages.jsonl
	messageLog       *BufferedWriter            // messages.jsonl (inter-agent messages)
	mcpLog           *BufferedWriter            // mcp_requests.jsonl
	commandLog       *BufferedWriter            // commands.jsonl (V2 command processor events)

	// Metadata for tracking workers and token usage.
	workers               []WorkerMetadata
	tokenUsage            TokenUsageSummary // Aggregate of all processes (computed)
	coordinatorTokenUsage TokenUsageSummary // Coordinator's cumulative usage

	// Session resumption fields.
	coordinatorSessionRef string
	resumable             bool

	// Application context fields (set via options).
	applicationName string
	workDir         string
	datePartition   string
	workflowID      string

	// pathBuilder is used for constructing session index paths.
	// Set via WithPathBuilder option.
	pathBuilder *SessionPathBuilder

	// Workflow state for persistence across coordinator refresh cycles.
	activeWorkflowState *workflow.WorkflowState

	// Synchronization.
	mu     sync.Mutex
	closed bool
}

// SessionOption is a functional option for configuring a Session.
type SessionOption func(*Session)

// WithWorkDir sets the project working directory for the session.
// This preserves the actual project location even when using git worktrees.
func WithWorkDir(dir string) SessionOption {
	return func(s *Session) {
		s.workDir = dir
	}
}

// WithApplicationName sets the application name for the session.
// Used for organizing sessions in centralized storage.
func WithApplicationName(name string) SessionOption {
	return func(s *Session) {
		s.applicationName = name
	}
}

// WithDatePartition sets the date partition (YYYY-MM-DD format) for the session.
// Used for organizing sessions by date in centralized storage.
func WithDatePartition(date string) SessionOption {
	return func(s *Session) {
		s.datePartition = date
	}
}

// WithWorkflowID sets the workflow ID for the session.
// Enables frontend to route API calls to the correct active workflow.
func WithWorkflowID(id string) SessionOption {
	return func(s *Session) {
		s.workflowID = id
	}
}

// WithPathBuilder sets the SessionPathBuilder for constructing index paths.
// This enables writing to both application-level and global session indexes.
func WithPathBuilder(pb *SessionPathBuilder) SessionOption {
	return func(s *Session) {
		s.pathBuilder = pb
	}
}

// Directory and file constants for session folder structure.
const (
	// Directory names.
	coordinatorDir = "coordinator"
	observerDir    = "observer"
	workersDir     = "workers"

	// File names.
	rawJSONLFile              = "raw.jsonl"
	messagesJSONLFile         = "messages.jsonl" // Inter-agent messages (root level)
	chatMessagesFile          = "messages.jsonl" // Chat messages (coordinator/worker directories)
	mcpRequestsFile           = "mcp_requests.jsonl"
	commandsFile              = "commands.jsonl"
	summaryFile               = "summary.md"
	accountabilitySummaryFile = "accountability_summary.md"
)

// New creates a new session with the given ID and directory.
// It creates the complete folder structure and initializes the session with status=running.
//
// Optional SessionOption functions can be passed to configure the session with additional
// application context (e.g., WithWorkDir, WithApplicationName, WithDatePartition).
//
// The folder structure created:
//
//	{dir}/
//	├── metadata.json                # Session metadata with status=running
//	├── coordinator/
//	│   ├── messages.jsonl           # Coordinator chat messages (structured JSONL)
//	│   └── raw.jsonl                # Raw Claude API JSON responses
//	├── workers/                     # Worker directories created on demand
//	├── messages.jsonl               # Inter-agent message log
//	├── mcp_requests.jsonl           # MCP tool call requests/responses
//	└── summary.md                   # Post-session summary (created on close)
func New(id, dir string, opts ...SessionOption) (*Session, error) {
	// Create the main session directory
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("creating session directory: %w", err)
	}

	// Validate write permissions by creating a test file
	testFile := filepath.Join(dir, ".write_test")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil { //nolint:gosec // G306: test file is immediately deleted
		return nil, fmt.Errorf("session directory not writable: %w", err)
	}
	_ = os.Remove(testFile)

	// Create coordinator directory
	coordPath := filepath.Join(dir, coordinatorDir)
	if err := os.MkdirAll(coordPath, 0750); err != nil {
		return nil, fmt.Errorf("creating coordinator directory: %w", err)
	}

	// Create workers directory (empty initially, subdirs created on demand)
	workersPath := filepath.Join(dir, workersDir)
	if err := os.MkdirAll(workersPath, 0750); err != nil {
		return nil, fmt.Errorf("creating workers directory: %w", err)
	}

	// Create observer directory
	observerPath := filepath.Join(dir, observerDir)
	if err := os.MkdirAll(observerPath, 0750); err != nil {
		return nil, fmt.Errorf("creating observer directory: %w", err)
	}

	// Create coordinator raw.jsonl with BufferedWriter
	coordRawPath := filepath.Join(coordPath, rawJSONLFile)
	coordRawFile, err := os.OpenFile(coordRawPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		return nil, fmt.Errorf("creating coordinator raw.jsonl: %w", err)
	}
	coordRaw := NewBufferedWriter(coordRawFile)

	// Create coordinator messages.jsonl with BufferedWriter (structured chat messages)
	coordMsgsPath := filepath.Join(coordPath, chatMessagesFile)
	coordMsgsFile, err := os.OpenFile(coordMsgsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		_ = coordRaw.Close()
		return nil, fmt.Errorf("creating coordinator messages.jsonl: %w", err)
	}
	coordMessages := NewBufferedWriter(coordMsgsFile)

	// Create observer messages.jsonl with BufferedWriter (structured chat messages)
	observerMsgsPath := filepath.Join(observerPath, chatMessagesFile)
	observerMsgsFile, err := os.OpenFile(observerMsgsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		return nil, fmt.Errorf("creating observer messages.jsonl: %w", err)
	}
	observerMessages := NewBufferedWriter(observerMsgsFile)

	// Create messages.jsonl with BufferedWriter (inter-agent messages)
	messagesPath := filepath.Join(dir, messagesJSONLFile)
	messageLogFile, err := os.OpenFile(messagesPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		_ = observerMessages.Close()
		return nil, fmt.Errorf("creating messages.jsonl: %w", err)
	}
	messageLog := NewBufferedWriter(messageLogFile)

	// Create mcp_requests.jsonl with BufferedWriter
	mcpPath := filepath.Join(dir, mcpRequestsFile)
	mcpLogFile, err := os.OpenFile(mcpPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		_ = observerMessages.Close()
		_ = messageLog.Close()
		return nil, fmt.Errorf("creating mcp_requests.jsonl: %w", err)
	}
	mcpLog := NewBufferedWriter(mcpLogFile)

	// Create commands.jsonl with BufferedWriter
	commandsPath := filepath.Join(dir, commandsFile)
	commandsLogFile, err := os.OpenFile(commandsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		_ = observerMessages.Close()
		_ = messageLog.Close()
		_ = mcpLog.Close()
		return nil, fmt.Errorf("creating commands.jsonl: %w", err)
	}
	commandLog := NewBufferedWriter(commandsLogFile)

	startTime := time.Now()

	// Create session struct first so we can apply options.
	// Note: tokenUsage is initialized to zero for new sessions since there is no prior
	// metadata. For resumed sessions (via Reopen), tokenUsage IS loaded from the prior
	// metadata to preserve accumulated totals. The updateTokenUsage() method then
	// accumulates turn costs (not cumulative costs) from processes, so there is no
	// double-counting on resume - see setMetrics() in process.go for the turn-cost
	// publishing semantics that make this safe.
	sess := &Session{
		ID:               id,
		Dir:              dir,
		StartTime:        startTime,
		Status:           StatusRunning,
		coordRaw:         coordRaw,
		coordMessages:    coordMessages,
		observerMessages: observerMessages,
		workerRaws:       make(map[string]*BufferedWriter),
		workerMessages:   make(map[string]*BufferedWriter),
		messageLog:       messageLog,
		mcpLog:           mcpLog,
		commandLog:       commandLog,
		workers:          []WorkerMetadata{},
		tokenUsage:       TokenUsageSummary{},
		closed:           false,
	}

	// Apply any provided options to set application context fields
	for _, opt := range opts {
		opt(sess)
	}

	// Create initial metadata with status=running and application context fields
	meta := &Metadata{
		SessionID:       id,
		StartTime:       startTime,
		Status:          StatusRunning,
		SessionDir:      dir,
		Workers:         []WorkerMetadata{},
		ApplicationName: sess.applicationName,
		WorkDir:         sess.workDir,
		DatePartition:   sess.datePartition,
		WorkflowID:      sess.workflowID,
	}

	if err := meta.Save(dir); err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		_ = messageLog.Close()
		_ = mcpLog.Close()
		_ = commandLog.Close()
		return nil, fmt.Errorf("saving initial metadata: %w", err)
	}

	return sess, nil
}

// Reopen reopens an existing session directory for continued writing.
// This is used for session resumption - it opens existing JSONL files in append mode
// so new messages continue writing to the same files without overwriting.
//
// Unlike New(), Reopen:
//   - Does NOT create directories (they must already exist)
//   - Does NOT create/overwrite metadata.json
//   - Opens files in append mode to continue from existing content
//   - Loads existing worker metadata to preserve the workers list
//
// The sessionDir must be an existing session directory with a valid metadata.json.
// Worker files (under workers/{id}/) are still created on-demand when workers are spawned.
func Reopen(sessionID, sessionDir string, opts ...SessionOption) (*Session, error) {
	// Verify the session directory exists
	info, err := os.Stat(sessionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session directory does not exist: %s", sessionDir)
		}
		return nil, fmt.Errorf("checking session directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("session path is not a directory: %s", sessionDir)
	}

	// Load metadata to get existing workers list and other session state.
	// Per reviewer recommendation: We need to restore workers slice so that
	// AddWorker/UpdateWorker calls don't corrupt the session state.
	meta, err := Load(sessionDir)
	if err != nil {
		return nil, fmt.Errorf("loading session metadata: %w", err)
	}

	// Open coordinator/raw.jsonl in append mode
	coordPath := filepath.Join(sessionDir, coordinatorDir)
	coordRawPath := filepath.Join(coordPath, rawJSONLFile)
	coordRawFile, err := os.OpenFile(coordRawPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted sessionDir parameter
	if err != nil {
		return nil, fmt.Errorf("reopening coordinator raw.jsonl: %w", err)
	}
	coordRaw := NewBufferedWriter(coordRawFile)

	// Open coordinator/messages.jsonl in append mode
	coordMsgsPath := filepath.Join(coordPath, chatMessagesFile)
	coordMsgsFile, err := os.OpenFile(coordMsgsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted sessionDir parameter
	if err != nil {
		_ = coordRaw.Close()
		return nil, fmt.Errorf("reopening coordinator messages.jsonl: %w", err)
	}
	coordMessages := NewBufferedWriter(coordMsgsFile)

	// Create observer directory if it doesn't exist (for sessions created before observer support)
	observerPath := filepath.Join(sessionDir, observerDir)
	if err := os.MkdirAll(observerPath, 0750); err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		return nil, fmt.Errorf("creating observer directory: %w", err)
	}

	// Open observer/messages.jsonl in append mode
	observerMsgsPath := filepath.Join(observerPath, chatMessagesFile)
	observerMsgsFile, err := os.OpenFile(observerMsgsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted sessionDir parameter
	if err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		return nil, fmt.Errorf("reopening observer messages.jsonl: %w", err)
	}
	observerMessages := NewBufferedWriter(observerMsgsFile)

	// Open messages.jsonl (inter-agent messages) in append mode
	messagesPath := filepath.Join(sessionDir, messagesJSONLFile)
	messageLogFile, err := os.OpenFile(messagesPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted sessionDir parameter
	if err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		_ = observerMessages.Close()
		return nil, fmt.Errorf("reopening messages.jsonl: %w", err)
	}
	messageLog := NewBufferedWriter(messageLogFile)

	// Open mcp_requests.jsonl in append mode
	mcpPath := filepath.Join(sessionDir, mcpRequestsFile)
	mcpLogFile, err := os.OpenFile(mcpPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted sessionDir parameter
	if err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		_ = observerMessages.Close()
		_ = messageLog.Close()
		return nil, fmt.Errorf("reopening mcp_requests.jsonl: %w", err)
	}
	mcpLog := NewBufferedWriter(mcpLogFile)

	// Open commands.jsonl in append mode
	commandsPath := filepath.Join(sessionDir, commandsFile)
	commandsLogFile, err := os.OpenFile(commandsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted sessionDir parameter
	if err != nil {
		_ = coordRaw.Close()
		_ = coordMessages.Close()
		_ = observerMessages.Close()
		_ = messageLog.Close()
		_ = mcpLog.Close()
		return nil, fmt.Errorf("reopening commands.jsonl: %w", err)
	}
	commandLog := NewBufferedWriter(commandsLogFile)

	// Create session with current time as start time for this resumed session.
	// Token usage metrics ARE loaded from prior metadata (meta.TokenUsage) to preserve
	// accumulated totals from before the session was paused. This does NOT cause
	// double-counting because:
	// 1. Processes publish turn costs (not cumulative) via setMetrics() in process.go
	// 2. updateTokenUsage() accumulates these turn costs with +=
	// 3. The prior total from metadata serves as the correct starting point
	// Example: Prior session had $1.50 cost. Resumed session receives turn costs of
	// $0.10, $0.20. Final total = $1.50 + $0.10 + $0.20 = $1.80 (correct).
	sess := &Session{
		ID:               sessionID,
		Dir:              sessionDir,
		StartTime:        time.Now(),
		Status:           StatusRunning,
		coordRaw:         coordRaw,
		coordMessages:    coordMessages,
		observerMessages: observerMessages,
		workerRaws:       make(map[string]*BufferedWriter),
		workerMessages:   make(map[string]*BufferedWriter),
		messageLog:       messageLog,
		mcpLog:           mcpLog,
		commandLog:       commandLog,
		// Restore workers from metadata to preserve existing worker list
		workers:               meta.Workers,
		tokenUsage:            meta.TokenUsage,            // Load prior aggregate - see comment above for why this is safe
		coordinatorTokenUsage: meta.CoordinatorTokenUsage, // Load prior coordinator usage
		coordinatorSessionRef: meta.CoordinatorSessionRef,
		resumable:             meta.Resumable,
		applicationName:       meta.ApplicationName,
		workDir:               meta.WorkDir,
		datePartition:         meta.DatePartition,
		workflowID:            meta.WorkflowID,
		closed:                false,
	}

	// Apply any provided options
	for _, opt := range opts {
		opt(sess)
	}

	return sess, nil
}

// WriteCoordinatorMessage writes a structured chat message to coordinator/messages.jsonl.
// The message is serialized to JSON and appended as a single JSONL line.
func (s *Session) WriteCoordinatorMessage(msg chatrender.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling coordinator message: %w", err)
	}

	// Append newline for JSONL format
	data = append(data, '\n')
	return s.coordMessages.Write(data)
}

// WriteObserverMessage writes a structured chat message to observer/messages.jsonl.
// The message is serialized to JSON and appended as a single JSONL line.
func (s *Session) WriteObserverMessage(msg chatrender.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling observer message: %w", err)
	}

	// Append newline for JSONL format
	data = append(data, '\n')
	return s.observerMessages.Write(data)
}

// WriteWorkerMessage writes a structured chat message to workers/{workerID}/messages.jsonl.
// Lazy-creates worker subdirectory and messages.jsonl file if needed.
func (s *Session) WriteWorkerMessage(workerID string, msg chatrender.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	// Get or create worker messages writer
	writer, err := s.getOrCreateWorkerMessages(workerID)
	if err != nil {
		return err
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling worker message: %w", err)
	}

	// Append newline for JSONL format
	data = append(data, '\n')
	return writer.Write(data)
}

// WriteCoordinatorRawJSON appends raw JSON to coordinator/raw.jsonl.
// The rawJSON should be a single JSON object (one line in JSONL format).
func (s *Session) WriteCoordinatorRawJSON(timestamp time.Time, rawJSON []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	// Ensure the line ends with newline
	data := rawJSON
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}

	return s.coordRaw.Write(data)
}

// WriteWorkerRawJSON appends raw JSON to workers/{workerID}/raw.jsonl.
// Lazy-creates worker subdirectory and raw.jsonl file if needed.
func (s *Session) WriteWorkerRawJSON(workerID string, timestamp time.Time, rawJSON []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	// Get or create worker raw log
	writer, err := s.getOrCreateWorkerRaw(workerID)
	if err != nil {
		return err
	}

	// Ensure the line ends with newline
	data := rawJSON
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}

	return writer.Write(data)
}

// WriteMessage appends a message entry to messages.jsonl in JSONL format.
func (s *Session) WriteMessage(entry message.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling message entry: %w", err)
	}

	// Append newline for JSONL format
	data = append(data, '\n')
	return s.messageLog.Write(data)
}

// WriteMCPEvent appends an MCP event to mcp_requests.jsonl in JSONL format.
func (s *Session) WriteMCPEvent(event events.MCPEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling MCP event: %w", err)
	}

	// Append newline for JSONL format
	data = append(data, '\n')
	return s.mcpLog.Write(data)
}

// CommandEvent is an alias for processor.CommandEvent to avoid import cycles in tests.
// The canonical definition is in internal/orchestration/v2/processor/middleware.go.
type CommandEvent = processor.CommandEvent

// WriteCommandEvent appends a V2 command event to commands.jsonl in JSONL format.
// This method implements processor.CommandWriter interface.
func (s *Session) WriteCommandEvent(event processor.CommandEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling command event: %w", err)
	}

	// Append newline for JSONL format
	data = append(data, '\n')
	return s.commandLog.Write(data)
}

// Close finalizes the session, flushes all BufferedWriters, updates metadata, and closes file handles.
// After Close returns, no more writes are accepted.
func (s *Session) Close(status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}
	s.closed = true
	s.Status = status

	var firstErr error

	// Close all worker raw writers
	for _, w := range s.workerRaws {
		if err := w.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Close all worker message writers
	for _, w := range s.workerMessages {
		if err := w.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Close coordinator writers
	if err := s.coordRaw.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := s.coordMessages.Close(); err != nil && firstErr == nil {
		firstErr = err
	}

	// Close observer writer
	if s.observerMessages != nil {
		if err := s.observerMessages.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Close message, MCP, and command logs
	if err := s.messageLog.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := s.mcpLog.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := s.commandLog.Close(); err != nil && firstErr == nil {
		firstErr = err
	}

	// Update metadata with end time, final status, workers, and token usage
	meta, err := Load(s.Dir)
	if err != nil {
		// If we can't load metadata, create a new one with application context
		meta = &Metadata{
			SessionID:       s.ID,
			StartTime:       s.StartTime,
			Status:          status,
			SessionDir:      s.Dir,
			Workers:         []WorkerMetadata{},
			ApplicationName: s.applicationName,
			WorkDir:         s.workDir,
			DatePartition:   s.datePartition,
			WorkflowID:      s.workflowID,
		}
	}
	meta.EndTime = time.Now()
	meta.Status = status
	meta.Workers = s.workers
	meta.TokenUsage = s.tokenUsage

	if err := meta.Save(s.Dir); err != nil && firstErr == nil {
		firstErr = err
	}

	// Generate summary.md
	if err := s.generateSummary(meta); err != nil && firstErr == nil {
		firstErr = err
	}

	// Update sessions.json index
	if err := s.updateSessionIndex(meta); err != nil && firstErr == nil {
		firstErr = err
	}

	return firstErr
}

// getOrCreateWorkerRaw returns the BufferedWriter for a worker's raw.jsonl,
// creating the worker directory and file if needed.
// Caller must hold s.mu.
func (s *Session) getOrCreateWorkerRaw(workerID string) (*BufferedWriter, error) {
	if writer, ok := s.workerRaws[workerID]; ok {
		return writer, nil
	}

	// Create worker directory
	workerPath := filepath.Join(s.Dir, workersDir, workerID)
	if err := os.MkdirAll(workerPath, 0750); err != nil {
		return nil, fmt.Errorf("creating worker directory: %w", err)
	}

	// Create raw.jsonl
	rawPath := filepath.Join(workerPath, rawJSONLFile)
	file, err := os.OpenFile(rawPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted workerID parameter
	if err != nil {
		return nil, fmt.Errorf("creating worker raw.jsonl: %w", err)
	}

	writer := NewBufferedWriter(file)
	s.workerRaws[workerID] = writer
	return writer, nil
}

// getOrCreateWorkerMessages returns the BufferedWriter for a worker's messages.jsonl,
// creating the worker directory and file if needed.
// Caller must hold s.mu.
func (s *Session) getOrCreateWorkerMessages(workerID string) (*BufferedWriter, error) {
	if writer, ok := s.workerMessages[workerID]; ok {
		return writer, nil
	}

	// Create worker directory
	workerPath := filepath.Join(s.Dir, workersDir, workerID)
	if err := os.MkdirAll(workerPath, 0750); err != nil {
		return nil, fmt.Errorf("creating worker directory: %w", err)
	}

	// Create messages.jsonl
	msgsPath := filepath.Join(workerPath, chatMessagesFile)
	file, err := os.OpenFile(msgsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted workerID parameter
	if err != nil {
		return nil, fmt.Errorf("creating worker messages.jsonl: %w", err)
	}

	writer := NewBufferedWriter(file)
	s.workerMessages[workerID] = writer
	return writer, nil
}

// generateSummary writes a summary.md file for the session.
func (s *Session) generateSummary(meta *Metadata) error {
	summaryPath := filepath.Join(s.Dir, summaryFile)

	duration := meta.EndTime.Sub(meta.StartTime)

	var content string
	content = "# Session Summary\n\n"
	content += fmt.Sprintf("**Session ID:** %s\n\n", meta.SessionID)
	content += fmt.Sprintf("**Status:** %s\n\n", meta.Status)
	content += fmt.Sprintf("**Start Time:** %s\n\n", meta.StartTime.Format(time.RFC3339))
	content += fmt.Sprintf("**End Time:** %s\n\n", meta.EndTime.Format(time.RFC3339))
	content += fmt.Sprintf("**Duration:** %s\n\n", duration.Round(time.Second))

	if len(meta.Workers) > 0 {
		content += "## Workers\n\n"
		for _, w := range meta.Workers {
			content += fmt.Sprintf("- **%s**: Spawned at %s", w.ID, w.SpawnedAt.Format(time.RFC3339))
			if !w.RetiredAt.IsZero() {
				content += fmt.Sprintf(", Retired at %s", w.RetiredAt.Format(time.RFC3339))
			}
			if w.FinalPhase != "" {
				content += fmt.Sprintf(" (Final phase: %s)", w.FinalPhase)
			}
			content += "\n"
		}
		content += "\n"
	}

	if meta.TokenUsage.TotalOutputTokens > 0 || meta.TokenUsage.TotalCostUSD > 0 {
		content += "## Token Usage\n\n"
		content += fmt.Sprintf("- **Output Tokens:** %d\n", meta.TokenUsage.TotalOutputTokens)
		if meta.TokenUsage.TotalCostUSD > 0 {
			content += fmt.Sprintf("- **Total Cost:** $%.2f\n", meta.TokenUsage.TotalCostUSD)
		}
		content += "\n"
	}

	return os.WriteFile(summaryPath, []byte(content), 0600)
}

// updateSessionIndex appends this session's entry to the session index files.
//
// When a pathBuilder is configured, the session writes to TWO indexes:
//   - Application index: {baseDir}/{appName}/sessions.json (per-application sessions)
//   - Global index: {baseDir}/sessions.json (all sessions across applications)
//
// When no pathBuilder is configured (legacy mode), writes only to the parent directory:
//   - Legacy index: {parent of session dir}/sessions.json
//
// Uses atomic rename to avoid race conditions on both files.
func (s *Session) updateSessionIndex(meta *Metadata) error {
	// Build accountability summary path relative to session dir if it exists
	var accountabilitySummaryPath string
	summaryPath := filepath.Join(s.Dir, accountabilitySummaryFile)
	if _, statErr := os.Stat(summaryPath); statErr == nil {
		accountabilitySummaryPath = summaryPath
	}

	// Create entry for this session with all metadata fields
	entry := SessionIndexEntry{
		ID:                        s.ID,
		StartTime:                 meta.StartTime,
		EndTime:                   meta.EndTime,
		Status:                    meta.Status,
		SessionDir:                s.Dir,
		AccountabilitySummaryPath: accountabilitySummaryPath,
		WorkerCount:               len(meta.Workers),
		ApplicationName:           s.applicationName,
		WorkDir:                   s.workDir,
		DatePartition:             s.datePartition,
	}

	// If pathBuilder is configured, write to both application and global indexes
	if s.pathBuilder != nil {
		// Update application-level index
		if err := s.updateApplicationIndex(entry); err != nil {
			return fmt.Errorf("updating application index: %w", err)
		}

		// Update global index
		if err := s.updateGlobalIndex(entry); err != nil {
			return fmt.Errorf("updating global index: %w", err)
		}

		return nil
	}

	// Legacy mode: write only to parent directory index
	indexPath := filepath.Join(filepath.Dir(s.Dir), "sessions.json")
	index, err := LoadSessionIndex(indexPath)
	if err != nil {
		return fmt.Errorf("loading session index: %w", err)
	}

	index.Sessions = upsertSessionEntry(index.Sessions, entry)

	if err := SaveSessionIndex(indexPath, index); err != nil {
		return fmt.Errorf("saving session index: %w", err)
	}

	return nil
}

// updateApplicationIndex updates the per-application sessions.json index.
// Path: {baseDir}/{appName}/sessions.json
func (s *Session) updateApplicationIndex(entry SessionIndexEntry) error {
	indexPath := s.pathBuilder.ApplicationIndexPath()

	// Load existing index or create empty one
	appIndex, err := LoadApplicationIndex(indexPath)
	if err != nil {
		return fmt.Errorf("loading application index: %w", err)
	}

	// Set the application name if not already set
	if appIndex.ApplicationName == "" {
		appIndex.ApplicationName = s.applicationName
	}

	// Update existing entry or append new one
	appIndex.Sessions = upsertSessionEntry(appIndex.Sessions, entry)

	// Save with atomic rename
	if err := SaveApplicationIndex(indexPath, appIndex); err != nil {
		return fmt.Errorf("saving application index: %w", err)
	}

	return nil
}

// updateGlobalIndex updates the global sessions.json index.
// Path: {baseDir}/sessions.json
func (s *Session) updateGlobalIndex(entry SessionIndexEntry) error {
	indexPath := s.pathBuilder.IndexPath()

	// Load existing index or create empty one
	globalIndex, err := LoadSessionIndex(indexPath)
	if err != nil {
		return fmt.Errorf("loading global index: %w", err)
	}

	// Update existing entry or append new one
	globalIndex.Sessions = upsertSessionEntry(globalIndex.Sessions, entry)

	// Save with atomic rename
	if err := SaveSessionIndex(indexPath, globalIndex); err != nil {
		return fmt.Errorf("saving global index: %w", err)
	}

	return nil
}

// upsertSessionEntry updates an existing entry with the same ID, or appends if not found.
// This handles resumed sessions correctly by updating in place rather than duplicating.
func upsertSessionEntry(sessions []SessionIndexEntry, entry SessionIndexEntry) []SessionIndexEntry {
	for i, existing := range sessions {
		if existing.ID == entry.ID {
			sessions[i] = entry
			return sessions
		}
	}
	return append(sessions, entry)
}

// AttachToBrokers subscribes to all event brokers and spawns goroutines to stream events to disk.
//
// Each broker spawns a dedicated goroutine that reads events from the subscription channel
// and writes them to the appropriate log files via the Write* methods.
//
// Note: Coordinator events should be received via AttachV2EventBus which handles both
// coordinator and worker ProcessEvents from the unified v2EventBus.
//
// Context cancellation stops all subscriber goroutines cleanly.
func (s *Session) AttachToBrokers(
	ctx context.Context,
	processBroker *pubsub.Broker[events.ProcessEvent],
	msgBroker *pubsub.Broker[message.Event],
	mcpBroker *pubsub.Broker[events.MCPEvent],
) {
	// Attach process broker for worker events
	if processBroker != nil {
		s.attachProcessBroker(ctx, processBroker)
	}

	// Attach message broker
	if msgBroker != nil {
		s.attachMessageBroker(ctx, msgBroker)
	}

	// Attach MCP broker
	if mcpBroker != nil {
		s.AttachMCPBroker(ctx, mcpBroker)
	}
}

// handleCoordinatorProcessEvent processes a coordinator ProcessEvent and writes to appropriate logs.
// This replaces the legacy handleCoordinatorEvent function that used CoordinatorEvent type.
//
// Event type mapping from legacy to v2:
//   - CoordinatorChat → ProcessOutput
//   - CoordinatorTokenUsage → ProcessTokenUsage
//   - CoordinatorError → ProcessError
//   - CoordinatorStatusChange/Ready/Working → ProcessStatusChange/Ready/Working
func (s *Session) handleCoordinatorProcessEvent(event events.ProcessEvent) {
	now := time.Now().UTC()

	switch event.Type {
	case events.ProcessOutput:
		// Detect tool calls by 🔧 prefix
		isToolCall := strings.HasPrefix(event.Output, "🔧")
		msg := chatrender.Message{
			Role:       string(event.Role),
			Content:    event.Output,
			IsToolCall: isToolCall,
			Timestamp:  &now,
		}
		if err := s.WriteCoordinatorMessage(msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write coordinator message", "error", err)
		}
		// Write raw JSON if present
		if len(event.RawJSON) > 0 {
			if err := s.WriteCoordinatorRawJSON(now, event.RawJSON); err != nil {
				log.Warn(log.CatOrch, "Session: failed to write coordinator raw JSON", "error", err)
			}
		}

	case events.ProcessIncoming:
		// User input - role is "user"
		msg := chatrender.Message{
			Role:      "user",
			Content:   event.Message,
			Timestamp: &now,
		}
		if err := s.WriteCoordinatorMessage(msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write coordinator incoming message", "error", err)
		}

	case events.ProcessTokenUsage:
		// Update coordinator token usage in metadata
		if event.Metrics != nil {
			s.updateTokenUsage("coordinator", event.Metrics.TokensUsed, event.Metrics.OutputTokens, event.Metrics.TotalCostUSD)
		}

	case events.ProcessError:
		// Write error as system message
		errMsg := "unknown error"
		if event.Error != nil {
			errMsg = event.Error.Error()
		}
		msg := chatrender.Message{
			Role:      "system",
			Content:   "Error: " + errMsg,
			Timestamp: &now,
		}
		if err := s.WriteCoordinatorMessage(msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write coordinator error message", "error", err)
		}

	case events.ProcessStatusChange, events.ProcessReady, events.ProcessWorking:
		// Status changes are not user-visible chat - skip writing to messages.jsonl
	}
}

// handleObserverProcessEvent processes an observer ProcessEvent and writes to appropriate logs.
func (s *Session) handleObserverProcessEvent(event events.ProcessEvent) {
	now := time.Now().UTC()

	switch event.Type {
	case events.ProcessOutput:
		// Detect tool calls by 🔧 prefix
		isToolCall := strings.HasPrefix(event.Output, "🔧")
		msg := chatrender.Message{
			Role:       string(event.Role),
			Content:    event.Output,
			IsToolCall: isToolCall,
			Timestamp:  &now,
		}
		if err := s.WriteObserverMessage(msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write observer message", "error", err)
		}

	case events.ProcessIncoming:
		// Incoming message to observer
		msg := chatrender.Message{
			Role:      event.Sender,
			Content:   event.Message,
			Timestamp: &now,
		}
		if err := s.WriteObserverMessage(msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write observer incoming message", "error", err)
		}

	case events.ProcessTokenUsage:
		// Update observer token usage in metadata
		if event.Metrics != nil {
			s.updateTokenUsage("observer", event.Metrics.TokensUsed, event.Metrics.OutputTokens, event.Metrics.TotalCostUSD)
		}

	case events.ProcessError:
		// Write error as system message
		errMsg := "unknown error"
		if event.Error != nil {
			errMsg = event.Error.Error()
		}
		msg := chatrender.Message{
			Role:      "system",
			Content:   "Error: " + errMsg,
			Timestamp: &now,
		}
		if err := s.WriteObserverMessage(msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write observer error message", "error", err)
		}

	case events.ProcessStatusChange, events.ProcessReady, events.ProcessWorking:
		// Status changes are not user-visible chat - skip writing to messages.jsonl
	}
}

// attachProcessBroker subscribes to the process event broker for worker events.
//
// The subscriber goroutine handles:
//   - ProcessOutput: writes to worker messages.jsonl and raw.jsonl (if RawJSON present)
//   - ProcessSpawned: updates metadata workers list
//   - ProcessStatusChange: updates worker metadata
//   - ProcessTokenUsage: updates metadata token counts
//   - ProcessError: writes error to worker messages.jsonl
func (s *Session) attachProcessBroker(ctx context.Context, broker *pubsub.Broker[events.ProcessEvent]) {
	sub := broker.Subscribe(ctx)

	log.SafeGo("session-process-broker", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok {
					return
				}
				// Only handle worker events, skip coordinator events
				if ev.Payload.IsWorker() {
					s.handleProcessEvent(ev.Payload)
				}
			}
		}
	})

	log.Debug(log.CatOrch, "Session attached to process broker", "sessionID", s.ID)
}

// AttachV2EventBus subscribes to the unified v2EventBus for all process events.
// This handles coordinator, observer, and worker events via the unified ProcessEvent type.
//
// The subscriber goroutine type-asserts to ProcessEvent and routes events based on Role:
// - RoleCoordinator: routes to handleCoordinatorProcessEvent
// - RoleObserver: routes to handleObserverProcessEvent
// - RoleWorker: routes to handleProcessEvent
//
// This replaces the legacy AttachCoordinatorBroker method that used CoordinatorEvent type.
func (s *Session) AttachV2EventBus(ctx context.Context, broker *pubsub.Broker[any]) {
	sub := broker.Subscribe(ctx)

	log.SafeGo("session-v2-event-bus", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok {
					return
				}
				// Type-assert to ProcessEvent and route based on role
				if processEvent, isProcess := ev.Payload.(events.ProcessEvent); isProcess {
					if processEvent.IsCoordinator() {
						s.handleCoordinatorProcessEvent(processEvent)
					} else if processEvent.IsObserver() {
						s.handleObserverProcessEvent(processEvent)
					} else if processEvent.IsWorker() {
						s.handleProcessEvent(processEvent)
					}
				}
				// Other event types from v2EventBus are ignored by session logger
			}
		}
	})

	log.Debug(log.CatOrch, "Session attached to v2EventBus", "sessionID", s.ID)
}

// handleProcessEvent processes a process event (worker) and writes to appropriate logs.
func (s *Session) handleProcessEvent(event events.ProcessEvent) {
	now := time.Now().UTC()
	workerID := event.ProcessID

	switch event.Type {
	case events.ProcessSpawned:
		// Add worker to metadata - use session's workDir (same for all processes currently)
		s.addWorker(workerID, now, s.workDir)
		// Log the spawn event as a system message
		msg := chatrender.Message{
			Role:      "system",
			Content:   "Worker spawned",
			Timestamp: &now,
		}
		if err := s.WriteWorkerMessage(workerID, msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write worker spawn message", "error", err, "workerID", workerID)
		}

	case events.ProcessOutput:
		// Detect tool calls by 🔧 prefix
		isToolCall := strings.HasPrefix(event.Output, "🔧")
		msg := chatrender.Message{
			Role:       "assistant",
			Content:    event.Output,
			IsToolCall: isToolCall,
			Timestamp:  &now,
		}
		if err := s.WriteWorkerMessage(workerID, msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write worker output message", "error", err, "workerID", workerID)
		}
		// Write raw JSON if present
		if len(event.RawJSON) > 0 {
			if err := s.WriteWorkerRawJSON(workerID, now, event.RawJSON); err != nil {
				log.Warn(log.CatOrch, "Session: failed to write worker raw JSON", "error", err, "workerID", workerID)
			}
		}

	case events.ProcessStatusChange:
		// Update worker phase in metadata
		var phaseStr string
		if event.Phase != nil {
			phaseStr = string(*event.Phase)
		}
		s.updateProcessPhase(workerID, phaseStr)
		// If worker is retired, record retirement time
		if event.Status == events.ProcessStatusRetired {
			s.retireWorker(workerID, now, phaseStr)
		}
		// Status changes are not user-visible chat - skip writing to messages.jsonl

	case events.ProcessTokenUsage:
		// Update worker token usage in metadata
		if event.Metrics != nil {
			s.updateTokenUsage(workerID, event.Metrics.TokensUsed, event.Metrics.OutputTokens, event.Metrics.TotalCostUSD)
		}

	case events.ProcessIncoming:
		// Preserve sender role (default to "coordinator" for coordinator→worker messages)
		role := event.Sender
		if role == "" {
			role = "coordinator"
		}
		msg := chatrender.Message{
			Role:      role,
			Content:   event.Message,
			Timestamp: &now,
		}
		if err := s.WriteWorkerMessage(workerID, msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write worker incoming message", "error", err, "workerID", workerID)
		}

	case events.ProcessError:
		// Write error as system message
		errMsg := "unknown error"
		if event.Error != nil {
			errMsg = event.Error.Error()
		}
		msg := chatrender.Message{
			Role:      "system",
			Content:   "Error: " + errMsg,
			Timestamp: &now,
		}
		if err := s.WriteWorkerMessage(workerID, msg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write worker error message", "error", err, "workerID", workerID)
		}
	}
}

// attachMessageBroker subscribes to the message event broker.
//
// The subscriber goroutine handles:
//   - EventPosted: writes the message to messages.jsonl
func (s *Session) attachMessageBroker(ctx context.Context, broker *pubsub.Broker[message.Event]) {
	sub := broker.Subscribe(ctx)

	log.SafeGo("session-message-broker", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok {
					return
				}
				s.handleMessageEvent(ev.Payload)
			}
		}
	})

	log.Debug(log.CatOrch, "Session attached to message broker", "sessionID", s.ID)
}

// handleMessageEvent processes a message event and writes to messages.jsonl.
func (s *Session) handleMessageEvent(event message.Event) {
	if event.Type == message.EventPosted {
		if err := s.WriteMessage(event.Entry); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write message", "error", err, "messageID", event.Entry.ID)
		}
	}
}

// AttachMCPBroker subscribes to the MCP event broker for late binding.
// This is useful when the MCP server starts after the session is created.
//
// The subscriber goroutine handles:
//   - MCPToolCall/MCPToolResult/MCPError: writes to mcp_requests.jsonl
func (s *Session) AttachMCPBroker(ctx context.Context, broker *pubsub.Broker[events.MCPEvent]) {
	sub := broker.Subscribe(ctx)

	log.SafeGo("session-mcp-broker", func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok {
					return
				}
				s.handleMCPEvent(ev.Payload)
			}
		}
	})

	log.Debug(log.CatOrch, "Session attached to MCP broker", "sessionID", s.ID)
}

// handleMCPEvent processes an MCP event and writes to mcp_requests.jsonl.
func (s *Session) handleMCPEvent(event events.MCPEvent) {
	if err := s.WriteMCPEvent(event); err != nil {
		log.Warn(log.CatOrch, "Session: failed to write MCP event", "error", err, "toolName", event.ToolName)
	}
}

// updateTokenUsage atomically updates per-process token usage and recomputes session totals.
//
// Parameters:
//   - processID: identifies which process (coordinator or worker-X) the usage is for
//   - contextTokens: current context window usage (replaces previous value, not accumulated)
//   - outputTokens: output tokens generated this turn (accumulated)
//   - costUSD: turn cost (accumulated)
//
// IMPORTANT: outputTokens and costUSD must be turn-based (not cumulative) from each process.
// Process.setMetrics() publishes turn cost via m.TotalCostUSD, which we accumulate here.
func (s *Session) updateTokenUsage(processID string, contextTokens, outputTokens int, costUSD float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	// Update per-process usage
	// Only update ContextTokens when we have actual token data (> 0).
	// Cost-only events (e.g., result events) have TokensUsed=0 and should not
	// reset the previously tracked context window size.
	if processID == "coordinator" {
		if contextTokens > 0 {
			s.coordinatorTokenUsage.ContextTokens = contextTokens
		}
		s.coordinatorTokenUsage.TotalOutputTokens += outputTokens
		s.coordinatorTokenUsage.TotalCostUSD += costUSD
	} else {
		// Find and update worker usage
		for i := range s.workers {
			if s.workers[i].ID == processID {
				if contextTokens > 0 {
					s.workers[i].TokenUsage.ContextTokens = contextTokens
				}
				s.workers[i].TokenUsage.TotalOutputTokens += outputTokens
				s.workers[i].TokenUsage.TotalCostUSD += costUSD
				break
			}
		}
	}

	// Recompute session totals from all processes
	s.recomputeTokenUsageLocked()

	// Persist to disk so token usage survives crashes/restarts
	if err := s.saveMetadataLocked(); err != nil {
		log.Warn(log.CatOrch, "Session: failed to persist token usage", "error", err)
	}
}

// recomputeTokenUsageLocked recalculates session totals from coordinator + all workers.
// Caller must hold s.mu.
func (s *Session) recomputeTokenUsageLocked() {
	total := TokenUsageSummary{
		ContextTokens:     s.coordinatorTokenUsage.ContextTokens,
		TotalOutputTokens: s.coordinatorTokenUsage.TotalOutputTokens,
		TotalCostUSD:      s.coordinatorTokenUsage.TotalCostUSD,
	}

	for _, w := range s.workers {
		total.ContextTokens += w.TokenUsage.ContextTokens
		total.TotalOutputTokens += w.TokenUsage.TotalOutputTokens
		total.TotalCostUSD += w.TokenUsage.TotalCostUSD
	}

	s.tokenUsage = total
}

// addWorker adds a new worker to the session's metadata.
// workDir is captured at spawn time; sessionRef is set later via SetWorkerSessionRef.
func (s *Session) addWorker(workerID string, spawnedAt time.Time, workDir string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if worker already exists
	for i := range s.workers {
		if s.workers[i].ID == workerID {
			return // Already tracked
		}
	}

	s.workers = append(s.workers, WorkerMetadata{
		ID:        workerID,
		SpawnedAt: spawnedAt,
		WorkDir:   workDir,
	})
}

// updateProcessPhase updates a worker's current phase in the metadata.
func (s *Session) updateProcessPhase(workerID, phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.workers {
		if s.workers[i].ID == workerID {
			s.workers[i].FinalPhase = phase
			return
		}
	}
}

// retireWorker marks a worker as retired with the retirement time and final phase.
func (s *Session) retireWorker(workerID string, retiredAt time.Time, finalPhase string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.workers {
		if s.workers[i].ID == workerID {
			s.workers[i].RetiredAt = retiredAt
			s.workers[i].FinalPhase = finalPhase
			return
		}
	}
}

// WriteWorkerAccountabilitySummary writes a worker's accountability summary to their session directory.
// Creates the worker directory if it doesn't exist (follows getOrCreateWorkerLog pattern).
// Returns the full path where the summary was saved.
// Note: taskID is embedded in the YAML frontmatter of the content, not passed as parameter.
func (s *Session) WriteWorkerAccountabilitySummary(workerID string, content []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return "", os.ErrClosed
	}

	// Ensure worker directory exists (lazy creation)
	workerPath := filepath.Join(s.Dir, workersDir, workerID)
	if err := os.MkdirAll(workerPath, 0750); err != nil {
		return "", fmt.Errorf("creating worker directory: %w", err)
	}

	// Write accountability summary file (overwrites if exists - latest summary wins)
	summaryPath := filepath.Join(workerPath, accountabilitySummaryFile)
	if err := os.WriteFile(summaryPath, content, 0600); err != nil {
		return "", fmt.Errorf("writing accountability summary file: %w", err)
	}

	log.Debug(log.CatOrch, "Wrote worker accountability summary", "workerID", workerID, "path", summaryPath)

	return summaryPath, nil
}

// SetCoordinatorSessionRef sets the coordinator's headless session reference.
// This should be called after the coordinator's first successful turn.
// Immediately persists metadata to ensure crash resilience.
func (s *Session) SetCoordinatorSessionRef(ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	s.coordinatorSessionRef = ref
	return s.saveMetadataLocked()
}

// SetWorkerSessionRef sets a worker's headless session reference.
// Should be called after the worker's first successful turn.
// Immediately persists metadata to ensure crash resilience.
func (s *Session) SetWorkerSessionRef(workerID, ref, workDir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	// Find worker and update
	for i := range s.workers {
		if s.workers[i].ID == workerID {
			s.workers[i].HeadlessSessionRef = ref
			s.workers[i].WorkDir = workDir
			return s.saveMetadataLocked()
		}
	}

	return fmt.Errorf("worker not found: %s", workerID)
}

// MarkResumable marks the session as resumable.
// Called after coordinator session ref is captured.
func (s *Session) MarkResumable() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	s.resumable = true
	return s.saveMetadataLocked()
}

// saveMetadataLocked persists metadata to disk.
// Caller must hold s.mu.
func (s *Session) saveMetadataLocked() error {
	meta, err := Load(s.Dir)
	if err != nil {
		// Create fresh metadata if file doesn't exist or is corrupted
		meta = &Metadata{
			SessionID:       s.ID,
			StartTime:       s.StartTime,
			Status:          s.Status,
			SessionDir:      s.Dir,
			ApplicationName: s.applicationName,
			WorkDir:         s.workDir,
			DatePartition:   s.datePartition,
			WorkflowID:      s.workflowID,
		}
	}

	// Update with current in-memory state
	meta.CoordinatorSessionRef = s.coordinatorSessionRef
	meta.Resumable = s.resumable
	meta.Workers = s.workers
	meta.TokenUsage = s.tokenUsage
	meta.CoordinatorTokenUsage = s.coordinatorTokenUsage
	meta.WorkflowID = s.workflowID

	return meta.Save(s.Dir)
}

// NotifySessionRef implements the SessionRefNotifier interface.
// Called by ProcessTurnCompleteHandler after a process's first successful turn.
func (s *Session) NotifySessionRef(processID, sessionRef, workDir string) error {
	if processID == "coordinator" {
		if err := s.SetCoordinatorSessionRef(sessionRef); err != nil {
			return err
		}
		return s.MarkResumable()
	}

	// Worker session ref
	return s.SetWorkerSessionRef(processID, sessionRef, workDir)
}

// GetWorkflowCompletedAt returns the workflow completion timestamp from session metadata.
// Returns zero time if workflow has not been completed.
// Implements handler.SessionMetadataProvider interface.
func (s *Session) GetWorkflowCompletedAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return time.Time{}
	}

	meta, err := Load(s.Dir)
	if err != nil {
		return time.Time{}
	}
	return meta.WorkflowCompletedAt
}

// UpdateWorkflowCompletion updates the workflow completion fields in session metadata.
// If WorkflowCompletedAt is already set (non-zero), the timestamp is preserved for idempotency.
// Implements handler.SessionMetadataProvider interface.
func (s *Session) UpdateWorkflowCompletion(status, summary string, completedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	meta, err := Load(s.Dir)
	if err != nil {
		// Create fresh metadata if file doesn't exist or is corrupted
		meta = &Metadata{
			SessionID:       s.ID,
			StartTime:       s.StartTime,
			Status:          s.Status,
			SessionDir:      s.Dir,
			ApplicationName: s.applicationName,
			WorkDir:         s.workDir,
			DatePartition:   s.datePartition,
			WorkflowID:      s.workflowID,
		}
	}

	// Update workflow completion fields
	meta.WorkflowCompletionStatus = status
	meta.WorkflowSummary = summary
	meta.WorkflowCompletedAt = completedAt

	return meta.Save(s.Dir)
}

// SetActiveWorkflowState caches workflow state in memory and persists it to disk.
// The state is written to {session_dir}/workflow_state.json.
func (s *Session) SetActiveWorkflowState(state *workflow.WorkflowState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	// Cache in memory
	s.activeWorkflowState = state

	// Persist to disk
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling workflow state: %w", err)
	}

	statePath := filepath.Join(s.Dir, workflow.WorkflowStateFilename)
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		return fmt.Errorf("writing workflow state file: %w", err)
	}

	log.Debug(log.CatOrch, "Session: persisted workflow state", "sessionID", s.ID, "workflowID", state.WorkflowID)

	return nil
}

// GetActiveWorkflowState returns the cached workflow state if available,
// otherwise loads it from disk. Returns nil if no workflow is active.
func (s *Session) GetActiveWorkflowState() (*workflow.WorkflowState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil, os.ErrClosed
	}

	// Return cached value if available
	if s.activeWorkflowState != nil {
		return s.activeWorkflowState, nil
	}

	// Try to load from disk
	statePath := filepath.Join(s.Dir, workflow.WorkflowStateFilename)
	data, err := os.ReadFile(statePath) //nolint:gosec // G304: path is constructed from trusted s.Dir
	if err != nil {
		if os.IsNotExist(err) {
			// No workflow state file - this is normal when no workflow is active
			return nil, nil
		}
		return nil, fmt.Errorf("reading workflow state file: %w", err)
	}

	var state workflow.WorkflowState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshaling workflow state: %w", err)
	}

	// Cache the loaded state
	s.activeWorkflowState = &state

	return &state, nil
}

// ClearActiveWorkflowState clears the memory cache and deletes the workflow state file from disk.
func (s *Session) ClearActiveWorkflowState() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	// Clear memory cache
	s.activeWorkflowState = nil

	// Delete file from disk
	statePath := filepath.Join(s.Dir, workflow.WorkflowStateFilename)
	if err := os.Remove(statePath); err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist - this is fine
			return nil
		}
		return fmt.Errorf("removing workflow state file: %w", err)
	}

	log.Debug(log.CatOrch, "Session: cleared workflow state", "sessionID", s.ID)

	return nil
}
