package task

import "time"

// BuildIssue creates a task.Issue with sensible defaults and applies options.
// Defaults: TitleText="Issue {id}", Type=TypeTask, Priority=PriorityMedium, Status=StatusOpen.
//
// Example:
//
//	issue := task.BuildIssue("bd-1",
//	    task.WithTitle("Fix auth bug"),
//	    task.WithType(task.TypeBug),
//	    task.WithPriority(task.PriorityHigh),
//	    task.WithLabels("security", "urgent"),
//	)
func BuildIssue(id string, opts ...IssueOption) Issue {
	issue := Issue{
		ID:        id,
		TitleText: "Issue " + id,
		Status:    StatusOpen,
		Priority:  PriorityMedium,
		Type:      TypeTask,
	}
	for _, opt := range opts {
		opt(&issue)
	}
	return issue
}

// BuildIssuePtr is like BuildIssue but returns a pointer.
func BuildIssuePtr(id string, opts ...IssueOption) *Issue {
	issue := BuildIssue(id, opts...)
	return &issue
}

// IssueOption configures a task.Issue during construction.
type IssueOption func(*Issue)

// WithTitle sets the issue title.
func WithTitle(title string) IssueOption {
	return func(i *Issue) { i.TitleText = title }
}

// WithDescription sets the issue description.
func WithDescription(desc string) IssueOption {
	return func(i *Issue) { i.DescriptionText = desc }
}

// WithDesign sets the issue design field.
func WithDesign(design string) IssueOption {
	return func(i *Issue) { i.Design = design }
}

// WithAcceptanceCriteria sets the acceptance criteria.
func WithAcceptanceCriteria(ac string) IssueOption {
	return func(i *Issue) { i.AcceptanceCriteria = ac }
}

// WithNotes sets the issue notes.
func WithNotes(notes string) IssueOption {
	return func(i *Issue) { i.Notes = notes }
}

// WithStatus sets the issue status.
func WithStatus(s Status) IssueOption {
	return func(i *Issue) { i.Status = s }
}

// WithPriority sets the issue priority.
func WithPriority(p Priority) IssueOption {
	return func(i *Issue) { i.Priority = p }
}

// WithType sets the issue type.
func WithType(t IssueType) IssueOption {
	return func(i *Issue) { i.Type = t }
}

// WithAssignee sets the issue assignee.
func WithAssignee(a string) IssueOption {
	return func(i *Issue) { i.Assignee = a }
}

// WithLabels sets the issue labels.
func WithLabels(labels ...string) IssueOption {
	return func(i *Issue) { i.Labels = labels }
}

// WithParentID sets the parent issue ID.
func WithParentID(id string) IssueOption {
	return func(i *Issue) { i.ParentID = id }
}

// WithChildren sets the child issue IDs.
func WithChildren(ids ...string) IssueOption {
	return func(i *Issue) { i.Children = ids }
}

// WithBlocks sets the issue IDs that this issue blocks.
func WithBlocks(ids ...string) IssueOption {
	return func(i *Issue) { i.Blocks = ids }
}

// WithBlockedBy sets the issue IDs that block this issue.
func WithBlockedBy(ids ...string) IssueOption {
	return func(i *Issue) { i.BlockedBy = ids }
}

// WithDiscoveredFrom sets the discovered-from issue IDs.
func WithDiscoveredFrom(ids ...string) IssueOption {
	return func(i *Issue) { i.DiscoveredFrom = ids }
}

// WithDiscovered sets the discovered issue IDs.
func WithDiscovered(ids ...string) IssueOption {
	return func(i *Issue) { i.Discovered = ids }
}

// WithCreatedAt sets the created_at timestamp.
func WithCreatedAt(t time.Time) IssueOption {
	return func(i *Issue) { i.CreatedAt = t }
}

// WithUpdatedAt sets the updated_at timestamp.
func WithUpdatedAt(t time.Time) IssueOption {
	return func(i *Issue) { i.UpdatedAt = t }
}

// WithClosedAt sets the closed_at timestamp.
func WithClosedAt(t time.Time) IssueOption {
	return func(i *Issue) { i.ClosedAt = t }
}

// WithDeferUntil sets the defer_until timestamp.
func WithDeferUntil(t time.Time) IssueOption {
	return func(i *Issue) { i.DeferUntil = t }
}

// WithCloseReason sets the close reason.
func WithCloseReason(reason string) IssueOption {
	return func(i *Issue) { i.CloseReason = reason }
}

// WithComments sets the issue comments.
func WithComments(comments ...Comment) IssueOption {
	return func(i *Issue) {
		i.Comments = comments
		i.CommentCount = len(comments)
	}
}

// WithCommentCount sets the comment count without populating comments.
func WithCommentCount(n int) IssueOption {
	return func(i *Issue) { i.CommentCount = n }
}

// WithExtension sets a single extension key-value pair.
// Initializes the Extensions map if nil.
func WithExtension(key string, value any) IssueOption {
	return func(i *Issue) {
		if i.Extensions == nil {
			i.Extensions = make(map[string]any)
		}
		i.Extensions[key] = value
	}
}

// WithExtensions sets the entire extensions map.
func WithExtensions(ext map[string]any) IssueOption {
	return func(i *Issue) { i.Extensions = ext }
}
