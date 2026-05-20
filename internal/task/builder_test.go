package task

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildIssue_Defaults(t *testing.T) {
	issue := BuildIssue("bd-1")

	require.Equal(t, "bd-1", issue.ID)
	require.Equal(t, "Issue bd-1", issue.TitleText)
	require.Equal(t, StatusOpen, issue.Status)
	require.Equal(t, PriorityMedium, issue.Priority)
	require.Equal(t, TypeTask, issue.Type)
	// All other fields should be zero values
	require.Empty(t, issue.DescriptionText)
	require.Empty(t, issue.Labels)
	require.Empty(t, issue.ParentID)
	require.Empty(t, issue.Children)
	require.Empty(t, issue.BlockedBy)
	require.Empty(t, issue.Blocks)
	require.Nil(t, issue.Extensions)
}

func TestBuildIssue_WithCoreFields(t *testing.T) {
	issue := BuildIssue("bd-42",
		WithTitle("Fix auth bug"),
		WithType(TypeBug),
		WithPriority(PriorityHigh),
		WithStatus(StatusInProgress),
	)

	require.Equal(t, "bd-42", issue.ID)
	require.Equal(t, "Fix auth bug", issue.TitleText)
	require.Equal(t, TypeBug, issue.Type)
	require.Equal(t, PriorityHigh, issue.Priority)
	require.Equal(t, StatusInProgress, issue.Status)
}

func TestBuildIssue_WithAllTextFields(t *testing.T) {
	issue := BuildIssue("bd-1",
		WithTitle("Title"),
		WithDescription("Description"),
		WithDesign("Design doc"),
		WithAcceptanceCriteria("AC"),
		WithNotes("Notes"),
	)

	require.Equal(t, "Title", issue.TitleText)
	require.Equal(t, "Description", issue.DescriptionText)
	require.Equal(t, "Design doc", issue.Design)
	require.Equal(t, "AC", issue.AcceptanceCriteria)
	require.Equal(t, "Notes", issue.Notes)
}

func TestBuildIssue_WithRelationships(t *testing.T) {
	issue := BuildIssue("bd-1",
		WithParentID("epic-1"),
		WithChildren("bd-2", "bd-3"),
		WithBlocks("bd-4"),
		WithBlockedBy("bd-5", "bd-6"),
		WithDiscoveredFrom("bd-7"),
		WithDiscovered("bd-8"),
	)

	require.Equal(t, "epic-1", issue.ParentID)
	require.Equal(t, []string{"bd-2", "bd-3"}, issue.Children)
	require.Equal(t, []string{"bd-4"}, issue.Blocks)
	require.Equal(t, []string{"bd-5", "bd-6"}, issue.BlockedBy)
	require.Equal(t, []string{"bd-7"}, issue.DiscoveredFrom)
	require.Equal(t, []string{"bd-8"}, issue.Discovered)
}

func TestBuildIssue_WithLabelsAndAssignee(t *testing.T) {
	issue := BuildIssue("bd-1",
		WithLabels("security", "urgent"),
		WithAssignee("alice"),
	)

	require.Equal(t, []string{"security", "urgent"}, issue.Labels)
	require.Equal(t, "alice", issue.Assignee)
}

func TestBuildIssue_WithTimestamps(t *testing.T) {
	now := time.Now()
	created := now.Add(-24 * time.Hour)
	closed := now.Add(-1 * time.Hour)
	deferred := now.Add(24 * time.Hour)

	issue := BuildIssue("bd-1",
		WithCreatedAt(created),
		WithUpdatedAt(now),
		WithClosedAt(closed),
		WithDeferUntil(deferred),
		WithCloseReason("Completed"),
	)

	require.Equal(t, created, issue.CreatedAt)
	require.Equal(t, now, issue.UpdatedAt)
	require.Equal(t, closed, issue.ClosedAt)
	require.Equal(t, deferred, issue.DeferUntil)
	require.Equal(t, "Completed", issue.CloseReason)
}

func TestIssue_DisplayStatus(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)

	require.Equal(t, StatusDeferred, BuildIssue("future",
		WithStatus(StatusOpen),
		WithDeferUntil(now.Add(time.Hour)),
	).DisplayStatus(now))
	require.Equal(t, StatusOpen, BuildIssue("past",
		WithStatus(StatusOpen),
		WithDeferUntil(now.Add(-time.Hour)),
	).DisplayStatus(now))
	require.Equal(t, StatusInProgress, BuildIssue("in-progress",
		WithStatus(StatusInProgress),
		WithDeferUntil(now.Add(time.Hour)),
	).DisplayStatus(now))
}

func TestBuildIssue_WithComments(t *testing.T) {
	comments := []Comment{
		{ID: "1", Author: "alice", Text: "First"},
		{ID: "2", Author: "bob", Text: "Second"},
	}
	issue := BuildIssue("bd-1", WithComments(comments...))

	require.Len(t, issue.Comments, 2)
	require.Equal(t, 2, issue.CommentCount)
	require.Equal(t, "alice", issue.Comments[0].Author)
}

func TestBuildIssue_WithCommentCount(t *testing.T) {
	issue := BuildIssue("bd-1", WithCommentCount(5))

	require.Equal(t, 5, issue.CommentCount)
	require.Empty(t, issue.Comments)
}

func TestBuildIssue_WithExtension(t *testing.T) {
	issue := BuildIssue("bd-1",
		WithExtension("pinned", true),
		WithExtension("hook_bead", "hook-1"),
	)

	require.NotNil(t, issue.Extensions)
	require.Equal(t, true, issue.Extensions["pinned"])
	require.Equal(t, "hook-1", issue.Extensions["hook_bead"])
}

func TestBuildIssue_WithExtensions(t *testing.T) {
	ext := map[string]any{"pinned": true, "custom": 42}
	issue := BuildIssue("bd-1", WithExtensions(ext))

	require.Equal(t, ext, issue.Extensions)
}

func TestBuildIssuePtr(t *testing.T) {
	issue := BuildIssuePtr("bd-1", WithTitle("Pointer issue"))

	require.NotNil(t, issue)
	require.Equal(t, "bd-1", issue.ID)
	require.Equal(t, "Pointer issue", issue.TitleText)
}

func TestBuildIssue_OptionsOverrideDefaults(t *testing.T) {
	// Last option wins for the same field
	issue := BuildIssue("bd-1",
		WithTitle("First"),
		WithTitle("Second"),
	)

	require.Equal(t, "Second", issue.TitleText)
}

func TestBuildIssue_NoOptions(t *testing.T) {
	// Should work with zero options
	issue := BuildIssue("minimal")
	require.Equal(t, "minimal", issue.ID)
	require.Equal(t, "Issue minimal", issue.TitleText)
}
