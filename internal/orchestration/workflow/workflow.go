// Package workflow provides workflow template management for orchestration mode.
// It supports loading and managing both built-in and user-defined workflow templates.
package workflow

// Source indicates where a workflow template originated from.
type Source int

const (
	// SourceBuiltIn indicates a workflow bundled with the application.
	SourceBuiltIn Source = iota
	// SourceUser indicates a workflow from the user's configuration directory.
	SourceUser
)

// String returns a human-readable representation of the Source.
func (s Source) String() string {
	switch s {
	case SourceBuiltIn:
		return "built-in"
	case SourceUser:
		return "user"
	default:
		return "unknown"
	}
}

// Workflow represents a workflow template that can be used in orchestration mode.
type Workflow struct {
	// ID is derived from the filename (e.g., "debate" from "debate.md").
	ID string

	// Name is the human-readable display name from frontmatter.
	Name string

	// Description is a brief description from frontmatter.
	Description string

	// Category is an optional grouping category from frontmatter.
	Category string

	// Workers is the number of workers required by this workflow.
	// A value of 0 (or omitted in frontmatter) indicates lazy spawn mode,
	// where workers are spawned on-demand as needed by the workflow.
	Workers int

	// Content is the full markdown content (including frontmatter).
	Content string

	// Source indicates whether this is a built-in or user-defined workflow.
	Source Source

	// FilePath is the absolute path for user workflows (empty for built-in).
	FilePath string
}
