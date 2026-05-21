package domain

import "time"

// Status represents the issue lifecycle state.
type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusClosed     Status = "closed"
	StatusDeferred   Status = "deferred"
	StatusBlocked    Status = "blocked"
)

// Priority levels (0-4, lower is more urgent).
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
	TypeMolecule  IssueType = "molecule"
	TypeConvoy    IssueType = "convoy"
	TypeAgent     IssueType = "agent"
)

// Comment represents a comment on an issue.
type Comment struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// Issue represents a beads issue.
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
	Sender             string    `json:"sender,omitempty"`
	Ephemeral          bool      `json:"ephemeral,omitempty"`
	Pinned             *bool     `json:"pinned,omitempty"`
	IsTemplate         *bool     `json:"is_template,omitempty"`
	Labels             []string  `json:"labels"`
	CreatedAt          time.Time `json:"created_at"`
	CreatedBy          string    `json:"created_by,omitempty"`
	UpdatedAt          time.Time `json:"updated_at"`
	ClosedAt           time.Time `json:"closed_at"`
	DeferUntil         time.Time `json:"defer_until,omitempty"`
	CloseReason        string    `json:"close_reason,omitempty"`

	MolType string `json:"mol_type,omitempty"`

	// Dependency tracking
	BlockedBy      []string `json:"blocked_by"`
	Blocks         []string `json:"blocks"`
	Children       []string `json:"children"`
	DiscoveredFrom []string `json:"discovered_from"`
	Discovered     []string `json:"discovered"`
	ParentID       string   `json:"parent_id"`

	// Comments (populated on demand)
	Comments []Comment `json:"comments,omitempty"`

	// CommentCount is populated by BQL queries for display without loading full comments
	CommentCount int `json:"comment_count,omitempty"`
}

// CreateResult holds the result of a create operation.
type CreateResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// UpdateIssueOptions specifies which fields to update on an issue.
// Nil pointer fields are skipped (not sent to bd CLI).
// This enables a single bd update call with only changed fields.
type UpdateIssueOptions struct {
	Title       *string
	Description *string
	Notes       *string
	Priority    *Priority
	Status      *Status
	Labels      *[]string  // nil = unchanged, &[]string{} = clear all
	Assignee    *string    // proactive; not used by current editor
	Type        *IssueType // proactive; not used by current editor
}
