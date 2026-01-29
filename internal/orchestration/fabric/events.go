package fabric

import (
	"time"

	"github.com/zjrosen/perles/internal/orchestration/fabric/domain"
)

// EventType identifies fabric events.
type EventType string

const (
	EventChannelCreated  EventType = "channel.created"
	EventChannelArchived EventType = "channel.archived"
	EventMessagePosted   EventType = "message.posted"
	EventReplyPosted     EventType = "reply.posted"
	EventArtifactAdded   EventType = "artifact.added"
	EventSubscribed      EventType = "subscribed"
	EventUnsubscribed    EventType = "unsubscribed"
	EventAcked           EventType = "acked"
)

// Event is published when something happens in Fabric.
type Event struct {
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`

	// Context
	ChannelID string `json:"channel_id,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	ParentID  string `json:"parent_id,omitempty"` // For reply.posted events

	// ChannelSlug is the human-readable channel name (tasks, planning, general, system).
	// Populated at emission time to enable direct display without runtime lookups.
	ChannelSlug string `json:"channel_slug,omitempty"`

	// Payloads (at most one is set)
	Thread       *domain.Thread       `json:"thread,omitempty"`
	Subscription *domain.Subscription `json:"subscription,omitempty"`
	Mentions     []string             `json:"mentions,omitempty"`
	Participants []string             `json:"participants,omitempty"` // Parent thread participants for reply events
}

// NewChannelCreatedEvent creates an event for channel creation.
func NewChannelCreatedEvent(channel *domain.Thread) Event {
	return Event{
		Type:        EventChannelCreated,
		Timestamp:   time.Now(),
		ChannelID:   channel.ID,
		ChannelSlug: channel.Slug,
		Thread:      channel,
	}
}

// NewMessagePostedEvent creates an event for a new message.
func NewMessagePostedEvent(message *domain.Thread, channelID, channelSlug string) Event {
	return Event{
		Type:        EventMessagePosted,
		Timestamp:   time.Now(),
		ChannelID:   channelID,
		ChannelSlug: channelSlug,
		Thread:      message,
		Mentions:    message.Mentions,
	}
}

// NewReplyPostedEvent creates an event for a reply.
// parentParticipants are the participants of the parent thread who should be notified.
func NewReplyPostedEvent(reply *domain.Thread, channelID, channelSlug, parentID string, parentParticipants []string) Event {
	return Event{
		Type:         EventReplyPosted,
		Timestamp:    time.Now(),
		ChannelID:    channelID,
		ChannelSlug:  channelSlug,
		ParentID:     parentID,
		Thread:       reply,
		Mentions:     reply.Mentions,
		Participants: parentParticipants,
	}
}

// NewArtifactAddedEvent creates an event for an artifact attachment.
func NewArtifactAddedEvent(artifact *domain.Thread, targetID string) Event {
	return Event{
		Type:      EventArtifactAdded,
		Timestamp: time.Now(),
		ChannelID: targetID,
		Thread:    artifact,
	}
}

// NewSubscribedEvent creates an event for a subscription.
func NewSubscribedEvent(sub *domain.Subscription, channelSlug string) Event {
	return Event{
		Type:         EventSubscribed,
		Timestamp:    time.Now(),
		ChannelID:    sub.ChannelID,
		ChannelSlug:  channelSlug,
		AgentID:      sub.AgentID,
		Subscription: sub,
	}
}

// NewUnsubscribedEvent creates an event for an unsubscription.
func NewUnsubscribedEvent(channelID, channelSlug, agentID string) Event {
	return Event{
		Type:        EventUnsubscribed,
		Timestamp:   time.Now(),
		ChannelID:   channelID,
		ChannelSlug: channelSlug,
		AgentID:     agentID,
	}
}

// NewAckedEvent creates an event for message acknowledgment.
func NewAckedEvent(agentID string, threadIDs []string) Event {
	return Event{
		Type:      EventAcked,
		Timestamp: time.Now(),
		AgentID:   agentID,
		Mentions:  threadIDs, // Reuse Mentions field for thread IDs
	}
}

// NewChannelArchivedEvent creates an event for channel archival.
func NewChannelArchivedEvent(channelID, channelSlug string) Event {
	return Event{
		Type:        EventChannelArchived,
		Timestamp:   time.Now(),
		ChannelID:   channelID,
		ChannelSlug: channelSlug,
	}
}
