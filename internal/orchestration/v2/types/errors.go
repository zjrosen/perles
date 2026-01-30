// Package types provides shared types and error sentinels for the v2 orchestration architecture.
package types

import (
	"errors"
	"fmt"
)

// ===========================================================================
// Process Lifecycle Errors
// ===========================================================================

// Note: ErrProcessNotFound is defined in repository.ErrProcessNotFound to avoid import cycles.
// Handlers should use handler.ErrProcessNotFound which re-exports it.

// ErrProcessRetired is returned when trying to operate on a retired process.
var ErrProcessRetired = errors.New("process is retired")

// ErrCoordinatorExists is returned when trying to spawn a coordinator that already exists.
var ErrCoordinatorExists = errors.New("coordinator already exists")

// ErrObserverExists is returned when trying to spawn an observer that already exists.
var ErrObserverExists = errors.New("observer already exists")

// ErrMaxProcessesReached is returned when trying to spawn at max capacity.
var ErrMaxProcessesReached = errors.New("maximum processes reached")

// ErrNotSpawning is returned when WorkerSpawned is called for a process not in spawning state.
var ErrNotSpawning = errors.New("process is not in spawning state")

// ErrAlreadyRetired is returned when trying to retire an already retired process.
var ErrAlreadyRetired = errors.New("process is already retired")

// ===========================================================================
// Queue Errors
// ===========================================================================

// ErrQueueEmpty is returned when trying to deliver from an empty queue.
var ErrQueueEmpty = errors.New("message queue is empty")

// Note: ErrQueueFull is defined in repository.ErrQueueFull to avoid import cycles.

// Note: ErrCommandQueueFull is defined in command.ErrQueueFull to avoid import cycles.

// ===========================================================================
// Task Errors
// ===========================================================================

// Note: ErrTaskNotFound is defined in repository.ErrTaskNotFound to avoid import cycles.

// ErrTaskNotApproved is returned when trying to commit a task that hasn't been approved.
var ErrTaskNotApproved = errors.New("task has not been approved")

// ErrNoTaskAssigned is returned when trying to transition a process with no assigned task.
var ErrNoTaskAssigned = errors.New("process has no task assigned")

// ===========================================================================
// Process State Errors
// ===========================================================================

// ErrProcessNotReady is returned when a process is not in ready status.
var ErrProcessNotReady = errors.New("process is not ready")

// ErrProcessNotIdle is returned when a process is not in idle phase.
var ErrProcessNotIdle = errors.New("process is not in idle phase")

// ErrProcessAlreadyAssigned is returned when a process already has a task assigned.
var ErrProcessAlreadyAssigned = errors.New("process already has a task assigned")

// ErrProcessNotImplementing is returned when trying to report completion for a process not implementing.
var ErrProcessNotImplementing = errors.New("process is not in implementing phase")

// ErrProcessNotReviewing is returned when trying to report verdict for a process not reviewing.
var ErrProcessNotReviewing = errors.New("process is not in reviewing phase")

// ErrProcessNotAwaitingReview is returned when a process is not awaiting review.
var ErrProcessNotAwaitingReview = errors.New("process is not awaiting review")

// ErrProcessNotImplementer is returned when a process is not the implementer of the task.
var ErrProcessNotImplementer = errors.New("process is not the implementer of the task")

// ===========================================================================
// Validation Errors
// ===========================================================================

// ErrInvalidVerdict is returned when a verdict value is not APPROVED or DENIED.
var ErrInvalidVerdict = errors.New("invalid verdict: must be APPROVED or DENIED")

// ErrInvalidPhaseTransition is returned when attempting an invalid state machine transition.
var ErrInvalidPhaseTransition = errors.New("invalid phase transition")

// ErrReviewerIsImplementer is returned when trying to assign a reviewer who is also the implementer.
var ErrReviewerIsImplementer = errors.New("reviewer cannot be the same as implementer")

// ===========================================================================
// Processor Errors
// ===========================================================================

// ErrUnknownCommandType is returned when no handler is registered for a command type.
var ErrUnknownCommandType = errors.New("unknown command type")

// ErrProcessorNotRunning is returned when submitting to a stopped processor.
var ErrProcessorNotRunning = errors.New("processor is not running")

// ErrHandlerNotFound is returned when a handler is not registered for a command type.
var ErrHandlerNotFound = errors.New("handler not found for command type")

// ErrDuplicateCommand is returned when a duplicate command is detected within the TTL window.
var ErrDuplicateCommand = fmt.Errorf("duplicate command detected within TTL window")
