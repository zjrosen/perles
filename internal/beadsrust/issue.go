// Package beadsrust provides the beads_rust backend adapter for perles.
// It integrates with the br CLI tool, which is a Rust implementation of
// the beads issue tracker. Unlike the Go beads backend, beads_rust is
// SQLite-only (no Dolt support) and all operations go through the br CLI.
package beadsrust

import "time"

// Issue represents a beads_rust issue as returned by br show --json.
// Field names match the br JSON output format.
type Issue struct {
	ID                 string     `json:"id"`
	Title              string     `json:"title"`
	Description        string     `json:"description,omitempty"`
	Design             string     `json:"design,omitempty"`
	AcceptanceCriteria string     `json:"acceptance_criteria,omitempty"`
	Notes              string     `json:"notes,omitempty"`
	Status             string     `json:"status"`
	Priority           int        `json:"priority"`
	IssueType          string     `json:"issue_type"`
	Assignee           string     `json:"assignee,omitempty"`
	Owner              string     `json:"owner,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	CreatedBy          string     `json:"created_by,omitempty"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ClosedAt           *time.Time `json:"closed_at,omitempty"`
	DeferUntil         *time.Time `json:"defer_until,omitempty"`
	CloseReason        string     `json:"close_reason,omitempty"`
	Parent             string     `json:"parent,omitempty"`
	SourceRepo         string     `json:"source_repo,omitempty"`
	ExternalRef        string     `json:"external_ref,omitempty"`
	CompactionLevel    int        `json:"compaction_level,omitempty"`
	OriginalSize       int        `json:"original_size,omitempty"`

	// Boolean fields are int (0/1) in br JSON output.
	Pinned     int `json:"pinned,omitempty"`
	Ephemeral  int `json:"ephemeral,omitempty"`
	IsTemplate int `json:"is_template,omitempty"`

	// Relationships
	Labels       []string  `json:"labels,omitempty"`
	Comments     []Comment `json:"comments,omitempty"`
	Dependencies []DepRef  `json:"dependencies,omitempty"` // issues this depends ON
	Dependents   []DepRef  `json:"dependents,omitempty"`   // issues that depend ON this

	// Counts (present in list output, not show)
	DependencyCount int `json:"dependency_count,omitempty"`
	DependentCount  int `json:"dependent_count,omitempty"`
}

// DepRef is a dependency reference as returned in br show --json
// dependencies/dependents arrays.
type DepRef struct {
	ID             string `json:"id"`
	Title          string `json:"title,omitempty"`
	Status         string `json:"status,omitempty"`
	Priority       int    `json:"priority,omitempty"`
	DependencyType string `json:"dependency_type"`
}

// Comment represents a comment on an issue as returned by br show --json.
type Comment struct {
	ID        string    `json:"id"`
	IssueID   string    `json:"issue_id,omitempty"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateResult holds the result of a br create command.
type CreateResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
