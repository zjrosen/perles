package client

// HeadlessProcess represents a running headless AI session.
// Implementations provide access to the event stream, process lifecycle,
// and session metadata.
type HeadlessProcess interface {
	// Events returns a channel of parsed output events.
	// The channel is closed when the process completes.
	Events() <-chan OutputEvent

	// Errors returns a channel of process errors.
	// Non-blocking; errors are dropped if the channel is full.
	Errors() <-chan error

	// SessionRef returns the session reference (session ID, thread ID, etc).
	// May be empty until the init event is received.
	SessionRef() string

	// Status returns the current process status.
	Status() ProcessStatus

	// IsRunning returns true if the process is actively running.
	IsRunning() bool

	// WorkDir returns the working directory of the process.
	WorkDir() string

	// PID returns the OS process ID, or 0 if not running.
	PID() int

	// Cancel terminates the process.
	Cancel() error

	// Wait blocks until the process completes.
	Wait() error
}
