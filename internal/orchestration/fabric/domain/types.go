// Package domain provides core types for the Fabric messaging system.
package domain

import (
	"slices"
	"time"
)

// ThreadType identifies what kind of thread this is.
type ThreadType string

const (
	ThreadChannel  ThreadType = "channel"
	ThreadMessage  ThreadType = "message"
	ThreadArtifact ThreadType = "artifact"
)

// RelationType defines how threads are connected.
type RelationType string

const (
	RelationChildOf    RelationType = "child_of"
	RelationReplyTo    RelationType = "reply_to"
	RelationReferences RelationType = "references"
)

// SubscriptionMode defines how an agent receives notifications.
type SubscriptionMode string

const (
	ModeAll      SubscriptionMode = "all"
	ModeMentions SubscriptionMode = "mentions"
	ModeNone     SubscriptionMode = "none"
)

// MessageKind identifies the purpose of a message.
type MessageKind string

const (
	KindInfo       MessageKind = "info"
	KindRequest    MessageKind = "request"
	KindResponse   MessageKind = "response"
	KindCompletion MessageKind = "completion"
	KindError      MessageKind = "error"
)

// ChannelSlugs defines the fixed channel structure.
const (
	SlugRoot     = "root"
	SlugSystem   = "system"
	SlugTasks    = "tasks"
	SlugPlanning = "planning"
	SlugGeneral  = "general"
	SlugObserver = "observer"
)

// Thread is the universal node in the Fabric graph.
type Thread struct {
	ID        string     `json:"id"`
	Type      ThreadType `json:"type"`
	CreatedAt time.Time  `json:"created_at"`
	CreatedBy string     `json:"created_by"`

	Content string `json:"content,omitempty"`
	Kind    string `json:"kind,omitempty"`

	Slug    string `json:"slug,omitempty"`
	Title   string `json:"title,omitempty"`
	Purpose string `json:"purpose,omitempty"`

	Name       string `json:"name,omitempty"`
	MediaType  string `json:"media_type,omitempty"`
	SizeBytes  int64  `json:"size_bytes,omitempty"`
	StorageURI string `json:"storage_uri,omitempty"`
	Sha256     string `json:"sha256,omitempty"`

	Mentions     []string          `json:"mentions,omitempty"`
	Participants []string          `json:"participants,omitempty"` // Agents participating in this thread (auto-added on mention/reply)
	Meta         map[string]string `json:"meta,omitempty"`

	Seq        int64      `json:"seq"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
}

// IsArchived returns true if this thread has been archived.
func (t *Thread) IsArchived() bool {
	return t.ArchivedAt != nil
}

// HasMention returns true if the given agent is mentioned.
func (t *Thread) HasMention(agentID string) bool {
	return slices.Contains(t.Mentions, agentID)
}

// IsParticipant returns true if the given agent is a participant.
func (t *Thread) IsParticipant(agentID string) bool {
	return slices.Contains(t.Participants, agentID)
}

// AddParticipant adds an agent as a participant if not already present.
func (t *Thread) AddParticipant(agentID string) {
	if !t.IsParticipant(agentID) {
		t.Participants = append(t.Participants, agentID)
	}
}

// AddParticipants adds multiple agents as participants.
func (t *Thread) AddParticipants(agentIDs ...string) {
	for _, id := range agentIDs {
		t.AddParticipant(id)
	}
}

// Dependency represents an edge in the thread graph.
type Dependency struct {
	ThreadID    string       `json:"thread_id"`
	DependsOnID string       `json:"depends_on_id"`
	Relation    RelationType `json:"relation"`
	CreatedAt   time.Time    `json:"created_at"`
}

// Key returns a unique identifier for this dependency edge.
func (d *Dependency) Key() string {
	return d.ThreadID + ":" + d.DependsOnID + ":" + string(d.Relation)
}

// NewDependency creates a new dependency edge.
func NewDependency(threadID, dependsOnID string, relation RelationType) Dependency {
	return Dependency{
		ThreadID:    threadID,
		DependsOnID: dependsOnID,
		Relation:    relation,
		CreatedAt:   time.Now(),
	}
}

// Subscription represents an agent's interest in a channel thread.
type Subscription struct {
	ChannelID string           `json:"channel_id"`
	AgentID   string           `json:"agent_id"`
	Mode      SubscriptionMode `json:"mode"`
	CreatedAt time.Time        `json:"created_at"`
}

// Key returns a unique identifier for this subscription.
func (s *Subscription) Key() string {
	return s.ChannelID + ":" + s.AgentID
}

// Ack tracks which message threads an agent has acknowledged.
type Ack struct {
	ThreadID string    `json:"thread_id"`
	AgentID  string    `json:"agent_id"`
	AckedAt  time.Time `json:"acked_at"`
}

// Key returns a unique identifier for this ack.
func (a *Ack) Key() string {
	return a.ThreadID + ":" + a.AgentID
}

// FixedChannels returns the channel definitions for a new session.
func FixedChannels() []Thread {
	return []Thread{
		{Type: ThreadChannel, Slug: SlugRoot, Title: "Root", Purpose: "Session root channel"},
		{Type: ThreadChannel, Slug: SlugSystem, Title: "System", Purpose: "Worker ready messages, system events"},
		{Type: ThreadChannel, Slug: SlugTasks, Title: "Tasks", Purpose: "Task assignments (each task = a message thread)"},
		{Type: ThreadChannel, Slug: SlugPlanning, Title: "Planning", Purpose: "Strategy, architecture discussions"},
		{Type: ThreadChannel, Slug: SlugGeneral, Title: "General", Purpose: "General coordination chat"},
		{Type: ThreadChannel, Slug: SlugObserver, Title: "Observer", Purpose: "User-to-observer communication"},
	}
}
