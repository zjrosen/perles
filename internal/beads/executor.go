package beads

// BeadsExecutor defines the interface for BD command execution.
type BeadsExecutor interface {
	UpdateStatus(issueID string, status Status) error
	UpdatePriority(issueID string, priority Priority) error
	UpdateType(issueID string, issueType IssueType) error
	CloseIssue(issueID, reason string) error
	ReopenIssue(issueID string) error
	DeleteIssues(issueIDs []string) error
	SetLabels(issueID string, labels []string) error
	ShowIssue(issueID string) (*Issue, error)
	AddComment(issueID, author, text string) error
}
