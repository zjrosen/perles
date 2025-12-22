package client

// ProcessStatus represents the current state of a headless process.
type ProcessStatus int

const (
	// StatusPending indicates the process has not yet started.
	StatusPending ProcessStatus = iota
	// StatusRunning indicates the process is actively running.
	StatusRunning
	// StatusCompleted indicates the process completed successfully.
	StatusCompleted
	// StatusFailed indicates the process failed with an error.
	StatusFailed
	// StatusCancelled indicates the process was cancelled.
	StatusCancelled
)

// String returns a human-readable string representation of the status.
func (s ProcessStatus) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusRunning:
		return "running"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// IsTerminal returns true if this is a terminal status (completed, failed, or cancelled).
func (s ProcessStatus) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCancelled
}
