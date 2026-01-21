package beads

// CreateResult holds the result of a create operation.
type CreateResult struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// BeadsExecutor defines the interface for BD command execution.
type BeadsExecutor interface {
	// Read operations
	ShowIssue(issueID string) (*Issue, error)

	// Update operations
	UpdateStatus(issueID string, status Status) error
	UpdatePriority(issueID string, priority Priority) error
	UpdateType(issueID string, issueType IssueType) error
	CloseIssue(issueID, reason string) error
	ReopenIssue(issueID string) error
	SetLabels(issueID string, labels []string) error
	AddComment(issueID, author, text string) error

	// Create operations
	CreateEpic(title, description string, labels []string) (CreateResult, error)
	CreateTask(title, description, parentID, assignee string, labels []string) (CreateResult, error)
	DeleteIssues(issueIDs []string) error

	// Dependency operations
	AddDependency(taskID, dependsOnID string) error
}
