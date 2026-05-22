// Package task defines backend-agnostic DTOs and interfaces for issue tracking.
// All UI, mode, and orchestration code consumes these types instead of beads-specific types.
// Backend implementations (beads, future linear/github/etc.) adapt their native types to these DTOs.
package task

import "time"

// Status represents the lifecycle state of an issue.
type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusClosed     Status = "closed"
	StatusDeferred   Status = "deferred"
	StatusBlocked    Status = "blocked"
)

// Priority represents issue urgency (0 = critical, 4 = backlog).
type Priority int

const (
	PriorityCritical Priority = 0
	PriorityHigh     Priority = 1
	PriorityMedium   Priority = 2
	PriorityLow      Priority = 3
	PriorityBacklog  Priority = 4
)

// IssueType categorizes the nature of work.
type IssueType string

const (
	TypeBug       IssueType = "bug"
	TypeFeature   IssueType = "feature"
	TypeTask      IssueType = "task"
	TypeEpic      IssueType = "epic"
	TypeChore     IssueType = "chore"
	TypeMilestone IssueType = "milestone"
	TypeStory     IssueType = "story"
	TypeSpike     IssueType = "spike"
)

// Comment represents a comment on an issue.
type Comment struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// Issue is the canonical task DTO used throughout the application.
// Backend implementations map their native types to this struct.
type Issue struct {
	ID                 string    `json:"id"`
	TitleText          string    `json:"title"`
	DescriptionText    string    `json:"description"`
	Design             string    `json:"design"`
	AcceptanceCriteria string    `json:"acceptance_criteria"`
	Notes              string    `json:"notes"`
	Status             Status    `json:"status"`
	Priority           Priority  `json:"priority"`
	Type               IssueType `json:"type"`
	Assignee           string    `json:"assignee"`
	Labels             []string  `json:"labels"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	ClosedAt           time.Time `json:"closed_at"`
	DeferUntil         time.Time `json:"defer_until,omitzero"`
	CloseReason        string    `json:"close_reason,omitempty"`
	ParentID           string    `json:"parent_id"`

	// Dependency tracking
	BlockedBy      []string `json:"blocked_by"`
	Blocks         []string `json:"blocks"`
	Children       []string `json:"children"`
	DiscoveredFrom []string `json:"discovered_from"`
	Discovered     []string `json:"discovered"`

	// Comments (populated on demand)
	Comments     []Comment `json:"comments,omitempty"`
	CommentCount int       `json:"comment_count,omitempty"`

	// Extensions allows backends to carry provider-specific data without
	// polluting the shared type. For example, beads agent fields (HookBead,
	// RoleBead, etc.) are stored here for round-tripping. UI code should
	// not read these directly.
	Extensions map[string]any `json:"extensions,omitempty"`
}

// DisplayStatus returns the user-visible status without changing backend semantics.
func (i Issue) DisplayStatus(now time.Time) Status {
	if i.Status == StatusOpen && !i.DeferUntil.IsZero() && i.DeferUntil.After(now) {
		return StatusDeferred
	}
	return i.Status
}

// CreateResult holds the result of a create operation.
type CreateResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// UpdateOptions specifies which fields to update on an issue.
// Nil pointer fields are skipped (not sent to the backend).
type UpdateOptions struct {
	Title       *string
	Description *string
	Notes       *string
	Priority    *Priority
	Status      *Status
	Labels      *[]string  // nil = unchanged, &[]string{} = clear all
	Assignee    *string    // nil = unchanged
	Type        *IssueType // nil = unchanged
}
