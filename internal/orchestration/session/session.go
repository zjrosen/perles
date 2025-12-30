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
	"sync"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/message"
	"github.com/zjrosen/perles/internal/pubsub"
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
	coordLog   *BufferedWriter            // coordinator/output.log
	coordRaw   *BufferedWriter            // coordinator/raw.jsonl
	workerLogs map[string]*BufferedWriter // workerID -> workers/{id}/output.log
	workerRaws map[string]*BufferedWriter // workerID -> workers/{id}/raw.jsonl
	messageLog *BufferedWriter            // messages.jsonl
	mcpLog     *BufferedWriter            // mcp_requests.jsonl

	// Metadata for tracking workers and token usage.
	workers    []WorkerMetadata
	tokenUsage TokenUsageSummary

	// Synchronization.
	mu     sync.Mutex
	closed bool
}

// Directory and file constants for session folder structure.
const (
	// Directory names.
	coordinatorDir = "coordinator"
	workersDir     = "workers"

	// File names.
	outputLogFile     = "output.log"
	rawJSONLFile      = "raw.jsonl"
	messagesJSONLFile = "messages.jsonl"
	mcpRequestsFile   = "mcp_requests.jsonl"
	summaryFile       = "summary.md"
)

// New creates a new session with the given ID and directory.
// It creates the complete folder structure and initializes the session with status=running.
//
// The folder structure created:
//
//	{dir}/
//	├── metadata.json                # Session metadata with status=running
//	├── coordinator/
//	│   ├── output.log               # Coordinator conversation stream
//	│   └── raw.jsonl                # Raw Claude API JSON responses
//	├── workers/                     # Worker directories created on demand
//	├── messages.jsonl               # Inter-agent message log
//	├── mcp_requests.jsonl           # MCP tool call requests/responses
//	└── summary.md                   # Post-session summary (created on close)
func New(id, dir string) (*Session, error) {
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

	// Create coordinator output.log with BufferedWriter
	coordLogPath := filepath.Join(coordPath, outputLogFile)
	coordLogFile, err := os.OpenFile(coordLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		return nil, fmt.Errorf("creating coordinator output log: %w", err)
	}
	coordLog := NewBufferedWriter(coordLogFile)

	// Create coordinator raw.jsonl with BufferedWriter
	coordRawPath := filepath.Join(coordPath, rawJSONLFile)
	coordRawFile, err := os.OpenFile(coordRawPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		_ = coordLog.Close()
		return nil, fmt.Errorf("creating coordinator raw.jsonl: %w", err)
	}
	coordRaw := NewBufferedWriter(coordRawFile)

	// Create messages.jsonl with BufferedWriter
	messagesPath := filepath.Join(dir, messagesJSONLFile)
	messageLogFile, err := os.OpenFile(messagesPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		_ = coordLog.Close()
		_ = coordRaw.Close()
		return nil, fmt.Errorf("creating messages.jsonl: %w", err)
	}
	messageLog := NewBufferedWriter(messageLogFile)

	// Create mcp_requests.jsonl with BufferedWriter
	mcpPath := filepath.Join(dir, mcpRequestsFile)
	mcpLogFile, err := os.OpenFile(mcpPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted dir parameter
	if err != nil {
		_ = coordLog.Close()
		_ = coordRaw.Close()
		_ = messageLog.Close()
		return nil, fmt.Errorf("creating mcp_requests.jsonl: %w", err)
	}
	mcpLog := NewBufferedWriter(mcpLogFile)

	startTime := time.Now()

	// Create initial metadata with status=running
	meta := &Metadata{
		SessionID: id,
		StartTime: startTime,
		Status:    StatusRunning,
		WorkDir:   dir,
		Workers:   []WorkerMetadata{},
	}

	if err := meta.Save(dir); err != nil {
		_ = coordLog.Close()
		_ = coordRaw.Close()
		_ = messageLog.Close()
		_ = mcpLog.Close()
		return nil, fmt.Errorf("saving initial metadata: %w", err)
	}

	return &Session{
		ID:         id,
		Dir:        dir,
		StartTime:  startTime,
		Status:     StatusRunning,
		coordLog:   coordLog,
		coordRaw:   coordRaw,
		workerLogs: make(map[string]*BufferedWriter),
		workerRaws: make(map[string]*BufferedWriter),
		messageLog: messageLog,
		mcpLog:     mcpLog,
		workers:    []WorkerMetadata{},
		tokenUsage: TokenUsageSummary{},
		closed:     false,
	}, nil
}

// WriteCoordinatorEvent writes a coordinator event to the coordinator output.log.
// Format: {ISO8601_timestamp} [{role}] {content}\n
func (s *Session) WriteCoordinatorEvent(timestamp time.Time, role, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	line := fmt.Sprintf("%s [%s] %s\n", timestamp.Format(time.RFC3339), role, content)
	return s.coordLog.Write([]byte(line))
}

// WriteWorkerEvent writes a worker event to the worker's output.log.
// Lazy-creates worker subdirectory and files if needed.
// Format: {ISO8601_timestamp} {content}\n
func (s *Session) WriteWorkerEvent(workerID string, timestamp time.Time, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return os.ErrClosed
	}

	// Get or create worker log
	writer, err := s.getOrCreateWorkerLog(workerID)
	if err != nil {
		return err
	}

	line := fmt.Sprintf("%s %s\n", timestamp.Format(time.RFC3339), content)
	return writer.Write([]byte(line))
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

	// Close all worker log writers
	for _, w := range s.workerLogs {
		if err := w.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Close all worker raw writers
	for _, w := range s.workerRaws {
		if err := w.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Close coordinator logs
	if err := s.coordLog.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := s.coordRaw.Close(); err != nil && firstErr == nil {
		firstErr = err
	}

	// Close message and MCP logs
	if err := s.messageLog.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := s.mcpLog.Close(); err != nil && firstErr == nil {
		firstErr = err
	}

	// Update metadata with end time, final status, workers, and token usage
	meta, err := Load(s.Dir)
	if err != nil {
		// If we can't load metadata, create a new one
		meta = &Metadata{
			SessionID: s.ID,
			StartTime: s.StartTime,
			Status:    status,
			WorkDir:   s.Dir,
			Workers:   []WorkerMetadata{},
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

	return firstErr
}

// getOrCreateWorkerLog returns the BufferedWriter for a worker's output.log,
// creating the worker directory and file if needed.
// Caller must hold s.mu.
func (s *Session) getOrCreateWorkerLog(workerID string) (*BufferedWriter, error) {
	if writer, ok := s.workerLogs[workerID]; ok {
		return writer, nil
	}

	// Create worker directory
	workerPath := filepath.Join(s.Dir, workersDir, workerID)
	if err := os.MkdirAll(workerPath, 0750); err != nil {
		return nil, fmt.Errorf("creating worker directory: %w", err)
	}

	// Create output.log
	logPath := filepath.Join(workerPath, outputLogFile)
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) //nolint:gosec // G304: path is constructed from trusted workerID parameter
	if err != nil {
		return nil, fmt.Errorf("creating worker output log: %w", err)
	}

	writer := NewBufferedWriter(file)
	s.workerLogs[workerID] = writer
	return writer, nil
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

	if meta.TokenUsage.TotalInputTokens > 0 || meta.TokenUsage.TotalOutputTokens > 0 {
		content += "## Token Usage\n\n"
		content += fmt.Sprintf("- **Input Tokens:** %d\n", meta.TokenUsage.TotalInputTokens)
		content += fmt.Sprintf("- **Output Tokens:** %d\n", meta.TokenUsage.TotalOutputTokens)
		if meta.TokenUsage.TotalCostUSD > 0 {
			content += fmt.Sprintf("- **Total Cost:** $%.2f\n", meta.TokenUsage.TotalCostUSD)
		}
		content += "\n"
	}

	return os.WriteFile(summaryPath, []byte(content), 0600)
}

// AttachToBrokers subscribes to all event brokers and spawns goroutines to stream events to disk.
// The coordBroker can be nil if the coordinator hasn't started yet - use AttachCoordinatorBroker later.
//
// Each broker spawns a dedicated goroutine that reads events from the subscription channel
// and writes them to the appropriate log files via the Write* methods.
//
// Context cancellation stops all subscriber goroutines cleanly.
func (s *Session) AttachToBrokers(
	ctx context.Context,
	coordBroker *pubsub.Broker[events.CoordinatorEvent],
	workerBroker *pubsub.Broker[events.WorkerEvent],
	msgBroker *pubsub.Broker[message.Event],
	mcpBroker *pubsub.Broker[events.MCPEvent],
) {
	// Attach coordinator broker if provided
	if coordBroker != nil {
		s.AttachCoordinatorBroker(ctx, coordBroker)
	}

	// Attach worker broker
	if workerBroker != nil {
		s.attachWorkerBroker(ctx, workerBroker)
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

// AttachCoordinatorBroker subscribes to the coordinator event broker for late binding.
// This is useful when the coordinator starts after the session is created.
//
// The subscriber goroutine handles:
//   - CoordinatorChat: writes to coordinator output.log and raw.jsonl (if RawJSON present)
//   - CoordinatorTokenUsage: updates metadata token counts
//   - CoordinatorError: writes error to log
func (s *Session) AttachCoordinatorBroker(ctx context.Context, broker *pubsub.Broker[events.CoordinatorEvent]) {
	sub := broker.Subscribe(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok {
					return
				}
				s.handleCoordinatorEvent(ev.Payload)
			}
		}
	}()

	log.Debug(log.CatOrch, "Session attached to coordinator broker", "sessionID", s.ID)
}

// handleCoordinatorEvent processes a coordinator event and writes to appropriate logs.
func (s *Session) handleCoordinatorEvent(event events.CoordinatorEvent) {
	now := time.Now()

	switch event.Type {
	case events.CoordinatorChat:
		// Write to coordinator output.log
		if err := s.WriteCoordinatorEvent(now, event.Role, event.Content); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write coordinator event", "error", err)
		}
		// Write raw JSON if present
		if len(event.RawJSON) > 0 {
			if err := s.WriteCoordinatorRawJSON(now, event.RawJSON); err != nil {
				log.Warn(log.CatOrch, "Session: failed to write coordinator raw JSON", "error", err)
			}
		}

	case events.CoordinatorTokenUsage:
		// Update token usage in metadata
		if event.Metrics != nil {
			s.updateTokenUsage(event.Metrics.InputTokens, event.Metrics.OutputTokens, event.Metrics.TotalCostUSD)
		}

	case events.CoordinatorError:
		// Write error to coordinator output.log
		errMsg := "unknown error"
		if event.Error != nil {
			errMsg = event.Error.Error()
		}
		if err := s.WriteCoordinatorEvent(now, "error", errMsg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write coordinator error", "error", err)
		}

	case events.CoordinatorStatusChange, events.CoordinatorReady, events.CoordinatorWorking:
		// Status changes are informational - optionally log them
		if err := s.WriteCoordinatorEvent(now, "status", string(event.Status)); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write coordinator status", "error", err)
		}
	}
}

// attachWorkerBroker subscribes to the worker event broker.
//
// The subscriber goroutine handles:
//   - WorkerOutput: writes to worker output.log and raw.jsonl (if RawJSON present)
//   - WorkerSpawned: updates metadata workers list
//   - WorkerStatusChange/WorkerRetired: updates worker metadata
//   - WorkerTokenUsage: updates metadata token counts
//   - WorkerError: writes error to worker log
func (s *Session) attachWorkerBroker(ctx context.Context, broker *pubsub.Broker[events.WorkerEvent]) {
	sub := broker.Subscribe(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok {
					return
				}
				s.handleWorkerEvent(ev.Payload)
			}
		}
	}()

	log.Debug(log.CatOrch, "Session attached to worker broker", "sessionID", s.ID)
}

// handleWorkerEvent processes a worker event and writes to appropriate logs.
func (s *Session) handleWorkerEvent(event events.WorkerEvent) {
	now := time.Now()

	switch event.Type {
	case events.WorkerSpawned:
		// Add worker to metadata
		s.addWorker(event.WorkerID, now)
		// Log the spawn event
		if err := s.WriteWorkerEvent(event.WorkerID, now, "Worker spawned"); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write worker spawn event", "error", err, "workerID", event.WorkerID)
		}

	case events.WorkerOutput:
		// Write to worker output.log
		if err := s.WriteWorkerEvent(event.WorkerID, now, event.Output); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write worker output", "error", err, "workerID", event.WorkerID)
		}
		// Write raw JSON if present
		if len(event.RawJSON) > 0 {
			if err := s.WriteWorkerRawJSON(event.WorkerID, now, event.RawJSON); err != nil {
				log.Warn(log.CatOrch, "Session: failed to write worker raw JSON", "error", err, "workerID", event.WorkerID)
			}
		}

	case events.WorkerStatusChange:
		// Update worker phase in metadata
		s.updateWorkerPhase(event.WorkerID, string(event.Phase))
		// If worker is retired, record retirement time
		if event.Status == events.WorkerRetired {
			s.retireWorker(event.WorkerID, now, string(event.Phase))
		}
		// Log the status change
		statusMsg := fmt.Sprintf("Status: %s, Phase: %s", event.Status.String(), event.Phase)
		if err := s.WriteWorkerEvent(event.WorkerID, now, statusMsg); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write worker status change", "error", err, "workerID", event.WorkerID)
		}

	case events.WorkerTokenUsage:
		// Update token usage in metadata
		if event.Metrics != nil {
			s.updateTokenUsage(event.Metrics.InputTokens, event.Metrics.OutputTokens, event.Metrics.TotalCostUSD)
		}

	case events.WorkerIncoming:
		// Log incoming message notification
		if err := s.WriteWorkerEvent(event.WorkerID, now, fmt.Sprintf("Incoming message: %s", event.Message)); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write worker incoming message", "error", err, "workerID", event.WorkerID)
		}

	case events.WorkerError:
		// Write error to worker log
		errMsg := "unknown error"
		if event.Error != nil {
			errMsg = event.Error.Error()
		}
		if err := s.WriteWorkerEvent(event.WorkerID, now, fmt.Sprintf("Error: %s", errMsg)); err != nil {
			log.Warn(log.CatOrch, "Session: failed to write worker error", "error", err, "workerID", event.WorkerID)
		}
	}
}

// attachMessageBroker subscribes to the message event broker.
//
// The subscriber goroutine handles:
//   - EventPosted: writes the message to messages.jsonl
func (s *Session) attachMessageBroker(ctx context.Context, broker *pubsub.Broker[message.Event]) {
	sub := broker.Subscribe(ctx)

	go func() {
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
	}()

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

	go func() {
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
	}()

	log.Debug(log.CatOrch, "Session attached to MCP broker", "sessionID", s.ID)
}

// handleMCPEvent processes an MCP event and writes to mcp_requests.jsonl.
func (s *Session) handleMCPEvent(event events.MCPEvent) {
	if err := s.WriteMCPEvent(event); err != nil {
		log.Warn(log.CatOrch, "Session: failed to write MCP event", "error", err, "toolName", event.ToolName)
	}
}

// updateTokenUsage atomically updates the session's token usage counters.
func (s *Session) updateTokenUsage(inputTokens, outputTokens int, costUSD float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokenUsage.TotalInputTokens += inputTokens
	s.tokenUsage.TotalOutputTokens += outputTokens
	s.tokenUsage.TotalCostUSD += costUSD
}

// addWorker adds a new worker to the session's metadata.
func (s *Session) addWorker(workerID string, spawnedAt time.Time) {
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
	})
}

// updateWorkerPhase updates a worker's current phase in the metadata.
func (s *Session) updateWorkerPhase(workerID, phase string) {
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
