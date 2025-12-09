package testutil

import "time"

// CommentData holds data for a comment to be inserted.
type CommentData struct {
	Author string
	Text   string
}

// Comment creates a CommentData structure.
func Comment(author, text string) CommentData {
	return CommentData{Author: author, Text: text}
}

// issueData holds all data for an issue to be inserted.
type issueData struct {
	id          string
	title       string
	description string
	status      string
	priority    int
	issueType   string
	assignee    *string
	labels      []string
	comments    []CommentData
	createdAt   time.Time
	updatedAt   time.Time
}

// defaultIssue returns an issueData with sensible defaults.
func defaultIssue(id string) issueData {
	now := time.Now()
	return issueData{
		id:        id,
		title:     id, // Default title is the ID
		status:    "open",
		priority:  2,
		issueType: "task",
		createdAt: now,
		updatedAt: now,
	}
}

// IssueOption configures an issue during builder setup.
type IssueOption func(*issueData)

// Title sets the issue title.
func Title(title string) IssueOption {
	return func(i *issueData) { i.title = title }
}

// Description sets the issue description.
func Description(desc string) IssueOption {
	return func(i *issueData) { i.description = desc }
}

// Status sets the issue status.
func Status(status string) IssueOption {
	return func(i *issueData) { i.status = status }
}

// Priority sets the issue priority (0-4).
func Priority(p int) IssueOption {
	return func(i *issueData) { i.priority = p }
}

// IssueType sets the issue type (bug, feature, task, epic, chore).
func IssueType(t string) IssueOption {
	return func(i *issueData) { i.issueType = t }
}

// Assignee sets the issue assignee.
func Assignee(a string) IssueOption {
	return func(i *issueData) { i.assignee = &a }
}

// Labels adds labels to the issue (nested option).
func Labels(labels ...string) IssueOption {
	return func(i *issueData) { i.labels = append(i.labels, labels...) }
}

// Comments adds comments to the issue (nested option).
func Comments(comments ...CommentData) IssueOption {
	return func(i *issueData) { i.comments = append(i.comments, comments...) }
}

// CreatedAt sets the created_at timestamp.
func CreatedAt(t time.Time) IssueOption {
	return func(i *issueData) { i.createdAt = t }
}

// UpdatedAt sets the updated_at timestamp.
func UpdatedAt(t time.Time) IssueOption {
	return func(i *issueData) { i.updatedAt = t }
}
