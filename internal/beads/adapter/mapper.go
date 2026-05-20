// Package adapter provides the beads backend adapter that converts between
// beads domain types and the backend-agnostic task DTOs.
package adapter

import (
	domain "github.com/zjrosen/perles/internal/beads/domain"
	"github.com/zjrosen/perles/internal/task"
)

// --- Status ---

// ToTaskStatus converts a beads Status to a task Status.
func ToTaskStatus(s domain.Status) task.Status {
	return task.Status(s)
}

// FromTaskStatus converts a task Status to a beads Status.
func FromTaskStatus(s task.Status) domain.Status {
	return domain.Status(s)
}

// --- Priority ---

// ToTaskPriority converts a beads Priority to a task Priority.
func ToTaskPriority(p domain.Priority) task.Priority {
	return task.Priority(p)
}

// FromTaskPriority converts a task Priority to a beads Priority.
func FromTaskPriority(p task.Priority) domain.Priority {
	return domain.Priority(p)
}

// --- IssueType ---

// ToTaskIssueType converts a beads IssueType to a task IssueType.
func ToTaskIssueType(t domain.IssueType) task.IssueType {
	return task.IssueType(t)
}

// FromTaskIssueType converts a task IssueType to a beads IssueType.
func FromTaskIssueType(t task.IssueType) domain.IssueType {
	return domain.IssueType(t)
}

// --- Comment ---

// ToTaskComment converts a beads Comment to a task Comment.
func ToTaskComment(c domain.Comment) task.Comment {
	return task.Comment{
		ID:        c.ID,
		Author:    c.Author,
		Text:      c.Text,
		CreatedAt: c.CreatedAt,
	}
}

// ToTaskComments converts a slice of beads Comments to task Comments.
func ToTaskComments(comments []domain.Comment) []task.Comment {
	if comments == nil {
		return nil
	}
	result := make([]task.Comment, len(comments))
	for i, c := range comments {
		result[i] = ToTaskComment(c)
	}
	return result
}

// --- Issue ---

// ToTaskIssue converts a beads domain Issue to a task Issue.
// Beads-specific agent fields are stored in Extensions for round-tripping.
func ToTaskIssue(b domain.Issue) task.Issue {
	t := task.Issue{
		ID:                 b.ID,
		TitleText:          b.TitleText,
		DescriptionText:    b.DescriptionText,
		Design:             b.Design,
		AcceptanceCriteria: b.AcceptanceCriteria,
		Notes:              b.Notes,
		Status:             ToTaskStatus(b.Status),
		Priority:           ToTaskPriority(b.Priority),
		Type:               ToTaskIssueType(b.Type),
		Assignee:           b.Assignee,
		Labels:             b.Labels,
		CreatedAt:          b.CreatedAt,
		UpdatedAt:          b.UpdatedAt,
		ClosedAt:           b.ClosedAt,
		DeferUntil:         b.DeferUntil,
		CloseReason:        b.CloseReason,
		ParentID:           b.ParentID,
		BlockedBy:          b.BlockedBy,
		Blocks:             b.Blocks,
		Children:           b.Children,
		DiscoveredFrom:     b.DiscoveredFrom,
		Discovered:         b.Discovered,
		Comments:           ToTaskComments(b.Comments),
		CommentCount:       b.CommentCount,
	}

	// Store beads-specific fields in Extensions
	ext := make(map[string]any)
	if b.Sender != "" {
		ext["sender"] = b.Sender
	}
	if b.Ephemeral {
		ext["ephemeral"] = true
	}
	if b.Pinned != nil {
		ext["pinned"] = *b.Pinned
	}
	if b.IsTemplate != nil {
		ext["is_template"] = *b.IsTemplate
	}
	if b.CreatedBy != "" {
		ext["created_by"] = b.CreatedBy
	}
	if b.MolType != "" {
		ext["mol_type"] = b.MolType
	}
	if len(ext) > 0 {
		t.Extensions = ext
	}

	return t
}

// ToTaskIssues converts a slice of beads Issues to task Issues.
func ToTaskIssues(issues []domain.Issue) []task.Issue {
	if issues == nil {
		return nil
	}
	result := make([]task.Issue, len(issues))
	for i, issue := range issues {
		result[i] = ToTaskIssue(issue)
	}
	return result
}

// --- CreateResult ---

// ToTaskCreateResult converts a beads CreateResult to a task CreateResult.
func ToTaskCreateResult(r domain.CreateResult) task.CreateResult {
	return task.CreateResult{
		ID:    r.ID,
		Title: r.Title,
	}
}

// --- UpdateOptions ---

// FromTaskUpdateOptions converts task UpdateOptions to beads UpdateIssueOptions.
func FromTaskUpdateOptions(o task.UpdateOptions) domain.UpdateIssueOptions {
	var result domain.UpdateIssueOptions
	result.Title = o.Title
	result.Description = o.Description
	result.Notes = o.Notes
	result.Assignee = o.Assignee

	if o.Priority != nil {
		p := domain.Priority(*o.Priority)
		result.Priority = &p
	}
	if o.Status != nil {
		s := domain.Status(*o.Status)
		result.Status = &s
	}
	if o.Labels != nil {
		result.Labels = o.Labels
	}
	if o.Type != nil {
		t := domain.IssueType(*o.Type)
		result.Type = &t
	}

	return result
}
