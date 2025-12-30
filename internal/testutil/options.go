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
	id           string
	title        string
	description  string
	status       string
	priority     int
	issueType    string
	assignee     *string
	sender       string
	ephemeral    bool
	pinned       *bool
	isTemplate   *bool
	labels       []string
	comments     []CommentData
	createdAt    time.Time
	createdBy    string
	updatedAt    time.Time
	closedAt     *time.Time
	deletedAt    *time.Time
	hookBead     string
	roleBead     string
	agentState   string
	lastActivity *time.Time
	roleType     string
	rig          string
	molType      string
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

// Status sets the issue status. If status is "closed", automatically sets closedAt to now.
func Status(status string) IssueOption {
	return func(i *issueData) {
		i.status = status
		if status == "closed" && i.closedAt == nil {
			now := time.Now()
			i.closedAt = &now
		}
	}
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

// CreatedBy sets the created_by field for the issue.
func CreatedBy(s string) IssueOption {
	return func(i *issueData) { i.createdBy = s }
}

// UpdatedAt sets the updated_at timestamp.
func UpdatedAt(t time.Time) IssueOption {
	return func(i *issueData) { i.updatedAt = t }
}

// ClosedAt sets the closed_at timestamp explicitly.
func ClosedAt(t time.Time) IssueOption {
	return func(i *issueData) { i.closedAt = &t }
}

// DeletedAt sets the deleted_at timestamp for soft-deleted issues.
func DeletedAt(t time.Time) IssueOption {
	return func(i *issueData) { i.deletedAt = &t }
}

// Sender sets the sender field for the issue.
func Sender(s string) IssueOption {
	return func(i *issueData) { i.sender = s }
}

// Ephemeral sets the ephemeral flag for the issue.
func Ephemeral(e bool) IssueOption {
	return func(i *issueData) { i.ephemeral = e }
}

// Pinned sets the pinned flag for the issue.
func Pinned(p bool) IssueOption {
	return func(i *issueData) { i.pinned = &p }
}

// IsTemplate sets the is_template flag for the issue.
func IsTemplate(t bool) IssueOption {
	return func(i *issueData) { i.isTemplate = &t }
}

// HookBead sets the hook_bead field for the issue.
func HookBead(s string) IssueOption {
	return func(i *issueData) { i.hookBead = s }
}

// RoleBead sets the role_bead field for the issue.
func RoleBead(s string) IssueOption {
	return func(i *issueData) { i.roleBead = s }
}

// AgentState sets the agent_state field for the issue.
func AgentState(s string) IssueOption {
	return func(i *issueData) { i.agentState = s }
}

// LastActivity sets the last_activity timestamp for the issue.
func LastActivity(t time.Time) IssueOption {
	return func(i *issueData) { i.lastActivity = &t }
}

// RoleType sets the role_type field for the issue.
func RoleType(s string) IssueOption {
	return func(i *issueData) { i.roleType = s }
}

// Rig sets the rig field for the issue.
func Rig(s string) IssueOption {
	return func(i *issueData) { i.rig = s }
}

// MolType sets the mol_type field for the issue.
func MolType(s string) IssueOption {
	return func(i *issueData) { i.molType = s }
}
