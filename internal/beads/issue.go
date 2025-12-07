// Package beads provides data access to the beads issue tracker.
package beads

import "time"

// Status represents the issue lifecycle state.
type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusClosed     Status = "closed"
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
	TypeBug     IssueType = "bug"
	TypeFeature IssueType = "feature"
	TypeTask    IssueType = "task"
	TypeEpic    IssueType = "epic"
	TypeChore   IssueType = "chore"
)

// Comment represents a comment on an issue.
type Comment struct {
	ID        int       `json:"id"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// Issue represents a beads issue.
type Issue struct {
	ID              string    `json:"id"`
	TitleText       string    `json:"title"`
	DescriptionText string    `json:"description"`
	Status          Status    `json:"status"`
	Priority        Priority  `json:"priority"`
	Type            IssueType `json:"type"`
	Assignee        string    `json:"assignee"`
	Labels          []string  `json:"labels"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// Dependency tracking
	BlockedBy []string `json:"blocked_by"`
	Blocks    []string `json:"blocks"`
	Children  []string `json:"children"`
	Related   []string `json:"related"`
	ParentID  string   `json:"parent_id"`

	// Comments (populated on demand)
	Comments []Comment `json:"comments,omitempty"`
}

// Title implements list.Item interface.
func (i Issue) Title() string {
	return i.ID + " " + i.TitleText
}

// Description implements list.Item interface.
func (i Issue) Description() string {
	return string(i.Type) + " - P" + string(rune('0'+i.Priority))
}

// FilterValue implements list.Item for bubbles list component.
func (i Issue) FilterValue() string {
	return i.TitleText
}
