package infrastructure

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	appbeads "github.com/zjrosen/perles/internal/beads/application"
	domain "github.com/zjrosen/perles/internal/beads/domain"
)

// TestBDExecutor_ImplementsIssueExecutor verifies BDExecutor implements IssueExecutor.
func TestBDExecutor_ImplementsIssueExecutor(t *testing.T) {
	var _ appbeads.IssueExecutor = (*BDExecutor)(nil)
}

func TestBDExecutor_ImplementsCommentReader(t *testing.T) {
	var _ appbeads.CommentReader = (*BDExecutor)(nil)
}

// TestBDExecutor_NewBDExecutor tests the constructor.
func TestBDExecutor_NewBDExecutor(t *testing.T) {
	workDir := "/some/work/dir"
	beadsDir := "/some/beads/dir"
	executor := NewBDExecutor(workDir, beadsDir)

	require.NotNil(t, executor, "NewBDExecutor returned nil")
	require.Equal(t, workDir, executor.workDir)
	require.Equal(t, beadsDir, executor.beadsDir)
}

// TestBDExecutor_UpdateTitle_MethodExists verifies UpdateTitle exists with correct signature.
// This is a compile-time check that ensures the method is implemented.
func TestBDExecutor_UpdateTitle_MethodExists(t *testing.T) {
	executor := NewBDExecutor("", "")

	// Verify the method exists and has the correct signature.
	// We call it with empty values - it will fail due to missing bd CLI,
	// but this confirms the method exists and compiles.
	var updateTitleFunc func(issueID, title string) error = executor.UpdateTitle

	require.NotNil(t, updateTitleFunc, "UpdateTitle method should exist")
}

// TestBDExecutor_UpdateDescription_MethodExists verifies UpdateDescription exists with correct signature.
func TestBDExecutor_UpdateDescription_MethodExists(t *testing.T) {
	executor := NewBDExecutor("", "")

	var updateDescFunc func(issueID, description string) error = executor.UpdateDescription

	require.NotNil(t, updateDescFunc, "UpdateDescription method should exist")
}

// TestBDExecutor_MethodSignatureConsistency verifies UpdateTitle has same signature as UpdateDescription.
func TestBDExecutor_MethodSignatureConsistency(t *testing.T) {
	executor := NewBDExecutor("", "")

	// Both methods should have signature: func(string, string) error
	var titleFunc func(string, string) error = executor.UpdateTitle
	var descFunc func(string, string) error = executor.UpdateDescription

	require.NotNil(t, titleFunc, "UpdateTitle should have signature func(string, string) error")
	require.NotNil(t, descFunc, "UpdateDescription should have signature func(string, string) error")
}

// TestBDExecutor_UpdateNotes_MethodExists verifies UpdateNotes exists with correct signature.
func TestBDExecutor_UpdateNotes_MethodExists(t *testing.T) {
	executor := NewBDExecutor("", "")

	var updateNotesFunc func(issueID, notes string) error = executor.UpdateNotes

	require.NotNil(t, updateNotesFunc, "UpdateNotes method should exist")
}

// TestBDExecutor_UpdateNotes_MethodSignatureConsistency verifies UpdateNotes has same signature as UpdateDescription.
func TestBDExecutor_UpdateNotes_MethodSignatureConsistency(t *testing.T) {
	executor := NewBDExecutor("", "")

	// Both methods should have signature: func(string, string) error
	var notesFunc func(string, string) error = executor.UpdateNotes
	var descFunc func(string, string) error = executor.UpdateDescription

	require.NotNil(t, notesFunc, "UpdateNotes should have signature func(string, string) error")
	require.NotNil(t, descFunc, "UpdateDescription should have signature func(string, string) error")
}

// newTestExecutor creates a BDExecutor with a runFunc that captures the args.
func newTestExecutor(fn func(args ...string) (string, error)) *BDExecutor {
	e := NewBDExecutor("", "")
	e.runFunc = fn
	return e
}

// TestBDExecutor_UpdateIssue_SingleField verifies correct CLI args when only Title is set.
func TestBDExecutor_UpdateIssue_SingleField(t *testing.T) {
	var captured []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		captured = args
		return "", nil
	})

	title := "New Title"
	opts := domain.UpdateIssueOptions{Title: &title}

	err := executor.UpdateIssue("PROJ-1", opts)
	require.NoError(t, err)
	require.Equal(t, []string{"update", "PROJ-1", "--title", "New Title", "--json"}, captured)
}

func TestBDExecutor_UpdateIssue_ParentID(t *testing.T) {
	var captured []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		captured = args
		return "", nil
	})

	parent := "PROJ-9"
	require.NoError(t, executor.UpdateIssue("PROJ-1", domain.UpdateIssueOptions{ParentID: &parent}))
	require.Equal(t, []string{"update", "PROJ-1", "--parent", "PROJ-9", "--json"}, captured)

	// Empty string removes the parent.
	empty := ""
	require.NoError(t, executor.UpdateIssue("PROJ-1", domain.UpdateIssueOptions{ParentID: &empty}))
	require.Equal(t, []string{"update", "PROJ-1", "--parent", "", "--json"}, captured)
}

func TestBDExecutor_GetComments(t *testing.T) {
	executor := newTestExecutor(func(args ...string) (string, error) {
		require.Equal(t, []string{"comments", "PROJ-1", "--json"}, args)
		return `[
			{"id":"c-1","author":"alice","text":"First","created_at":"2026-04-16T12:00:00Z"},
			{"id":"c-2","author":"bob","text":"Second","created_at":"2026-04-16T12:01:00Z"}
		]`, nil
	})

	comments, err := executor.GetComments("PROJ-1")
	require.NoError(t, err)
	require.Len(t, comments, 2)
	require.Equal(t, "alice", comments[0].Author)
	require.Equal(t, "First", comments[0].Text)
	require.Equal(t, "bob", comments[1].Author)
	require.Equal(t, "Second", comments[1].Text)
}

func TestBDExecutor_AddComment(t *testing.T) {
	var captured []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		captured = args
		return "", nil
	})

	err := executor.AddComment("PROJ-1", "alice", "Great work!")
	require.NoError(t, err)
	require.Equal(t, []string{"comments", "add", "PROJ-1", "Great work!", "--json", "--actor", "alice"}, captured)
}

// TestBDExecutor_UpdateIssue_MultipleFields verifies correct CLI args with Title + Priority + Labels.
// Labels require a separate bd update call because --set-labels cannot be combined with other flags.
func TestBDExecutor_UpdateIssue_MultipleFields(t *testing.T) {
	var calls [][]string
	executor := newTestExecutor(func(args ...string) (string, error) {
		cp := make([]string, len(args))
		copy(cp, args)
		calls = append(calls, cp)
		return "", nil
	})

	title := "Updated Title"
	priority := domain.PriorityHigh
	labels := []string{"bug", "urgent"}
	opts := domain.UpdateIssueOptions{
		Title:    &title,
		Priority: &priority,
		Labels:   &labels,
	}

	err := executor.UpdateIssue("PROJ-2", opts)
	require.NoError(t, err)
	require.Len(t, calls, 2, "expected two bd calls: one for fields, one for labels")
	require.Equal(t, []string{
		"update", "PROJ-2",
		"--title", "Updated Title",
		"--priority", "1",
		"--json",
	}, calls[0])
	require.Equal(t, []string{
		"update", "PROJ-2",
		"--set-labels", "bug,urgent",
		"--json",
	}, calls[1])
}

// TestBDExecutor_UpdateIssue_AllFields verifies complete arg list when all fields are set.
// Labels are sent in a separate bd update call.
func TestBDExecutor_UpdateIssue_AllFields(t *testing.T) {
	var calls [][]string
	executor := newTestExecutor(func(args ...string) (string, error) {
		cp := make([]string, len(args))
		copy(cp, args)
		calls = append(calls, cp)
		return "", nil
	})

	title := "Full Update"
	description := "A new description"
	notes := "Some notes"
	priority := domain.PriorityCritical
	status := domain.StatusInProgress
	labels := []string{"feature", "v2"}
	assignee := "alice"
	issueType := domain.TypeFeature
	opts := domain.UpdateIssueOptions{
		Title:       &title,
		Description: &description,
		Notes:       &notes,
		Priority:    &priority,
		Status:      &status,
		Labels:      &labels,
		Assignee:    &assignee,
		Type:        &issueType,
	}

	err := executor.UpdateIssue("PROJ-3", opts)
	require.NoError(t, err)
	require.Len(t, calls, 2, "expected two bd calls: one for fields, one for labels")
	require.Equal(t, []string{
		"update", "PROJ-3",
		"--title", "Full Update",
		"--description", "A new description",
		"--notes", "Some notes",
		"--priority", "0",
		"--status", "in_progress",
		"--assignee", "alice",
		"--type", "feature",
		"--json",
	}, calls[0])
	require.Equal(t, []string{
		"update", "PROJ-3",
		"--set-labels", "feature,v2",
		"--json",
	}, calls[1])
}

// TestBDExecutor_UpdateIssue_NoFieldsSet verifies no-op when all fields are nil (no CLI call).
func TestBDExecutor_UpdateIssue_NoFieldsSet(t *testing.T) {
	called := false
	executor := newTestExecutor(func(args ...string) (string, error) {
		called = true
		return "", nil
	})

	opts := domain.UpdateIssueOptions{}

	err := executor.UpdateIssue("PROJ-4", opts)
	require.NoError(t, err)
	require.False(t, called, "runBeads should not be called when no fields are set")
}

// TestBDExecutor_UpdateIssue_ErrorPropagation verifies error is returned with issue ID context.
func TestBDExecutor_UpdateIssue_ErrorPropagation(t *testing.T) {
	executor := newTestExecutor(func(args ...string) (string, error) {
		return "", errors.New("bd update failed: connection refused")
	})

	title := "Will Fail"
	opts := domain.UpdateIssueOptions{Title: &title}

	err := executor.UpdateIssue("PROJ-5", opts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PROJ-5")
	require.Contains(t, err.Error(), "saving issue")
	require.Contains(t, err.Error(), "bd update failed: connection refused")
}

// TestBDExecutor_UpdateIssue_MultilineDescription verifies multiline description with combined flags.
func TestBDExecutor_UpdateIssue_MultilineDescription(t *testing.T) {
	var captured []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		captured = args
		return "", nil
	})

	description := "Line 1\nLine 2\nLine 3"
	priority := domain.PriorityMedium
	opts := domain.UpdateIssueOptions{
		Description: &description,
		Priority:    &priority,
	}

	err := executor.UpdateIssue("PROJ-6", opts)
	require.NoError(t, err)
	require.Equal(t, []string{
		"update", "PROJ-6",
		"--description", "Line 1\nLine 2\nLine 3",
		"--priority", "2",
		"--json",
	}, captured)
	// Verify the multiline string is passed as a single argument
	require.Equal(t, "Line 1\nLine 2\nLine 3", captured[3])
}

// TestBDExecutor_UpdateIssue_EmptyLabels verifies that clearing all labels
// fetches current labels via ShowIssue and then issues --remove-label for each.
func TestBDExecutor_UpdateIssue_EmptyLabels(t *testing.T) {
	var calls [][]string
	executor := newTestExecutor(func(args ...string) (string, error) {
		cp := make([]string, len(args))
		copy(cp, args)
		calls = append(calls, cp)
		// ShowIssue call returns an issue with existing labels
		if len(args) > 0 && args[0] == "show" {
			return `[{"id":"PROJ-7","title":"T","labels":["bug","urgent"]}]`, nil
		}
		return "", nil
	})

	emptyLabels := []string{}
	opts := domain.UpdateIssueOptions{Labels: &emptyLabels}

	err := executor.UpdateIssue("PROJ-7", opts)
	require.NoError(t, err)
	require.Len(t, calls, 2, "expected show + remove-label calls")
	require.Equal(t, []string{"show", "PROJ-7", "--json"}, calls[0])
	require.Equal(t, []string{
		"update", "PROJ-7",
		"--remove-label", "bug",
		"--remove-label", "urgent",
		"--json",
	}, calls[1])
}

// TestBDExecutor_UpdateIssue_EmptyLabels_AlreadyClear verifies no-op when
// clearing labels on an issue that has no labels.
func TestBDExecutor_UpdateIssue_EmptyLabels_AlreadyClear(t *testing.T) {
	var calls [][]string
	executor := newTestExecutor(func(args ...string) (string, error) {
		cp := make([]string, len(args))
		copy(cp, args)
		calls = append(calls, cp)
		if len(args) > 0 && args[0] == "show" {
			return `[{"id":"PROJ-7b","title":"T"}]`, nil
		}
		return "", nil
	})

	emptyLabels := []string{}
	opts := domain.UpdateIssueOptions{Labels: &emptyLabels}

	err := executor.UpdateIssue("PROJ-7b", opts)
	require.NoError(t, err)
	require.Len(t, calls, 1, "expected only show call, no update needed")
	require.Equal(t, []string{"show", "PROJ-7b", "--json"}, calls[0])
}

// TestBDExecutor_UpdateIssue_LabelsOnly verifies that when only labels are set,
// only the SetLabels call is made (no empty non-label update).
func TestBDExecutor_UpdateIssue_LabelsOnly(t *testing.T) {
	var calls [][]string
	executor := newTestExecutor(func(args ...string) (string, error) {
		cp := make([]string, len(args))
		copy(cp, args)
		calls = append(calls, cp)
		return "", nil
	})

	labels := []string{"bug", "critical"}
	opts := domain.UpdateIssueOptions{Labels: &labels}

	err := executor.UpdateIssue("PROJ-8", opts)
	require.NoError(t, err)
	require.Len(t, calls, 1, "labels-only update should produce exactly one bd call")
	require.Equal(t, []string{
		"update", "PROJ-8",
		"--set-labels", "bug,critical",
		"--json",
	}, calls[0])
}

// TestBDExecutor_UpdateIssue_LabelsErrorReturned verifies that non-label
// fields are saved even if the labels call fails (they are independent calls).
func TestBDExecutor_UpdateIssue_LabelsErrorReturned(t *testing.T) {
	callCount := 0
	executor := newTestExecutor(func(args ...string) (string, error) {
		callCount++
		if callCount == 2 {
			return "", errors.New("set-labels failed")
		}
		return "", nil
	})

	title := "Saved OK"
	labels := []string{"bug"}
	opts := domain.UpdateIssueOptions{
		Title:  &title,
		Labels: &labels,
	}

	err := executor.UpdateIssue("PROJ-9", opts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "labels")
	require.Equal(t, 2, callCount, "both calls should have been attempted")
}

// TestBDExecutor_UpdateIssue_NilLabels_ClearsAll verifies that nil labels
// (from formmodal when user deselects all) also triggers the clear path.
func TestBDExecutor_UpdateIssue_NilLabels_ClearsAll(t *testing.T) {
	var calls [][]string
	executor := newTestExecutor(func(args ...string) (string, error) {
		cp := make([]string, len(args))
		copy(cp, args)
		calls = append(calls, cp)
		if len(args) > 0 && args[0] == "show" {
			return `[{"id":"PROJ-10","title":"T","labels":["old"]}]`, nil
		}
		return "", nil
	})

	var nilLabels []string // nil, not []string{}
	opts := domain.UpdateIssueOptions{Labels: &nilLabels}

	err := executor.UpdateIssue("PROJ-10", opts)
	require.NoError(t, err)
	require.Len(t, calls, 2, "expected show + remove-label calls")
	require.Equal(t, []string{"show", "PROJ-10", "--json"}, calls[0])
	require.Equal(t, []string{
		"update", "PROJ-10",
		"--remove-label", "old",
		"--json",
	}, calls[1])
}

// TestUpdateIssueOptions_ZeroValue verifies that the zero value has all nil fields.
func TestUpdateIssueOptions_ZeroValue(t *testing.T) {
	var opts domain.UpdateIssueOptions
	require.Nil(t, opts.Title)
	require.Nil(t, opts.Description)
	require.Nil(t, opts.Notes)
	require.Nil(t, opts.Priority)
	require.Nil(t, opts.Status)
	require.Nil(t, opts.Labels)
	require.Nil(t, opts.Assignee)
	require.Nil(t, opts.Type)
}
