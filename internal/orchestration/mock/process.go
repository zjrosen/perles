package mock

import (
	"sync"
	"time"

	"perles/internal/orchestration/client"
)

// Process is a mock implementation of client.HeadlessProcess for testing.
// It provides methods to inject events and errors, and control process lifecycle.
type Process struct {
	events    chan client.OutputEvent
	errors    chan error
	sessionID string
	workDir   string
	pid       int
	status    client.ProcessStatus
	done      chan struct{}
	waitErr   error
	mu        sync.RWMutex
}

// NewProcess creates a new mock process with buffered channels.
func NewProcess() *Process {
	return &Process{
		events: make(chan client.OutputEvent, 100),
		errors: make(chan error, 10),
		status: client.StatusRunning,
		done:   make(chan struct{}),
	}
}

// NewProcessWithConfig creates a new mock process configured from client.Config.
func NewProcessWithConfig(cfg client.Config) *Process {
	p := NewProcess()
	p.workDir = cfg.WorkDir
	p.sessionID = cfg.SessionID
	return p
}

// Events returns the events channel.
func (p *Process) Events() <-chan client.OutputEvent {
	return p.events
}

// Errors returns the errors channel.
func (p *Process) Errors() <-chan error {
	return p.errors
}

// SessionRef returns the session reference.
func (p *Process) SessionRef() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sessionID
}

// Status returns the current process status.
func (p *Process) Status() client.ProcessStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// IsRunning returns true if the process is running.
func (p *Process) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status == client.StatusRunning
}

// WorkDir returns the working directory.
func (p *Process) WorkDir() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.workDir
}

// PID returns the mock process ID.
func (p *Process) PID() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.pid
}

// Cancel terminates the mock process.
func (p *Process) Cancel() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.status == client.StatusRunning {
		p.status = client.StatusCancelled
		close(p.events)
		close(p.errors)
		close(p.done)
	}
	return nil
}

// Wait blocks until the process completes.
func (p *Process) Wait() error {
	<-p.done
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.waitErr
}

// --- Control Methods for Tests ---

// SendEvent sends an event to the events channel.
// It's safe to call from tests to simulate AI output.
func (p *Process) SendEvent(event client.OutputEvent) {
	p.mu.RLock()
	status := p.status
	p.mu.RUnlock()

	if status == client.StatusRunning {
		p.events <- event
	}
}

// SendError sends an error to the errors channel.
func (p *Process) SendError(err error) {
	p.mu.RLock()
	status := p.status
	p.mu.RUnlock()

	if status == client.StatusRunning {
		select {
		case p.errors <- err:
		default:
			// Drop if buffer full
		}
	}
}

// Complete marks the process as successfully completed.
// It closes the event channel and signals completion.
func (p *Process) Complete() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.status == client.StatusRunning {
		p.status = client.StatusCompleted
		close(p.events)
		close(p.errors)
		close(p.done)
	}
}

// Fail marks the process as failed with the given error.
func (p *Process) Fail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.status == client.StatusRunning {
		p.status = client.StatusFailed
		p.waitErr = err
		close(p.events)
		close(p.errors)
		close(p.done)
	}
}

// SetSessionID sets the session ID (useful before process starts).
func (p *Process) SetSessionID(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sessionID = id
}

// SetWorkDir sets the working directory.
func (p *Process) SetWorkDir(dir string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.workDir = dir
}

// SetPID sets the mock process ID.
func (p *Process) SetPID(pid int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pid = pid
}

// Done returns a channel that's closed when the process completes.
// Useful for tests that need to wait for completion.
func (p *Process) Done() <-chan struct{} {
	return p.done
}

// --- Helper Methods for Common Event Types ---

// SendInitEvent sends a system init event with the given session ID and working directory.
func (p *Process) SendInitEvent(sessionID, workDir string) {
	p.mu.Lock()
	p.sessionID = sessionID
	p.workDir = workDir
	p.mu.Unlock()

	p.SendEvent(client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: sessionID,
		WorkDir:   workDir,
		Timestamp: time.Now(),
	})
}

// SendTextEvent sends an assistant text message event.
func (p *Process) SendTextEvent(text string) {
	p.SendEvent(client.OutputEvent{
		Type:      client.EventAssistant,
		Timestamp: time.Now(),
		Message: &client.MessageContent{
			Role: "assistant",
			Content: []client.ContentBlock{
				{Type: "text", Text: text},
			},
		},
	})
}

// SendToolUseEvent sends a tool use event.
func (p *Process) SendToolUseEvent(toolID, toolName string, input []byte) {
	p.SendEvent(client.OutputEvent{
		Type:      client.EventAssistant,
		Timestamp: time.Now(),
		Message: &client.MessageContent{
			Role: "assistant",
			Content: []client.ContentBlock{
				{
					Type:  "tool_use",
					ID:    toolID,
					Name:  toolName,
					Input: input,
				},
			},
		},
	})
}

// SendToolResultEvent sends a tool result event.
func (p *Process) SendToolResultEvent(toolID, toolName, output string) {
	p.SendEvent(client.OutputEvent{
		Type:      client.EventToolResult,
		Timestamp: time.Now(),
		Tool: &client.ToolContent{
			ID:     toolID,
			Name:   toolName,
			Output: output,
		},
	})
}

// SendResultEvent sends a successful result event with token usage.
func (p *Process) SendResultEvent(inputTokens, outputTokens int, costUSD float64) {
	p.SendEvent(client.OutputEvent{
		Type:      client.EventResult,
		Timestamp: time.Now(),
		Usage: &client.UsageInfo{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
		TotalCostUSD: costUSD,
	})
}

// SendErrorResultEvent sends an error result event.
func (p *Process) SendErrorResultEvent(errMsg string) {
	p.SendEvent(client.OutputEvent{
		Type:          client.EventResult,
		Timestamp:     time.Now(),
		IsErrorResult: true,
		Result:        errMsg,
	})
}
