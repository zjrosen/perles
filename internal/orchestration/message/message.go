// Package message provides inter-agent communication for orchestration mode.
package message

import (
	"fmt"
	"time"
)

// MessageType categorizes the kind of message being sent.
type MessageType string

const (
	// MessageInfo is for status updates and informational messages.
	MessageInfo MessageType = "info"

	// MessageRequest indicates the sender is asking for something.
	MessageRequest MessageType = "request"

	// MessageResponse is a reply to a previous request.
	MessageResponse MessageType = "response"

	// MessageCompletion indicates a task or unit of work is done.
	MessageCompletion MessageType = "completion"

	// MessageError indicates something went wrong.
	MessageError MessageType = "error"
)

// Entry represents a single message in the communication log.
type Entry struct {
	// ID is a unique identifier for this message (uuid).
	ID string `json:"id"`

	// Timestamp when the message was created.
	Timestamp time.Time `json:"timestamp"`

	// From identifies the sender: "COORDINATOR", "WORKER.1", "USER", etc.
	From string `json:"from"`

	// To identifies the recipient: "ALL", "COORDINATOR", "WORKER.2", etc.
	To string `json:"to"`

	// Content is the message body.
	Content string `json:"content"`

	// Type categorizes the message purpose.
	Type MessageType `json:"type"`

	// ReadBy tracks which agents have seen this message.
	ReadBy []string `json:"read_by,omitempty"`
}

// Common sender/recipient identifiers.
const (
	// ActorCoordinator is the orchestrating agent.
	ActorCoordinator = "COORDINATOR"

	// ActorUser is the human user.
	ActorUser = "USER"

	// ActorAll broadcasts to all agents.
	ActorAll = "ALL"
)

// WorkerID returns the standard worker identifier for a given index.
// Worker indices are 1-based: WORKER.1, WORKER.2, etc.
func WorkerID(index int) string {
	return fmt.Sprintf("WORKER.%d", index)
}

// EventType identifies the kind of message event.
type EventType string

const (
	// EventPosted is emitted when a new message is appended to the log.
	EventPosted EventType = "posted"
)

// Event represents an event from the message system.
type Event struct {
	// Type identifies the kind of event.
	Type EventType

	// Entry is the message that was posted.
	Entry Entry
}
