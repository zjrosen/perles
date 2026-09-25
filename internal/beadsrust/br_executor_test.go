package beadsrust

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/task"
)

// newTestExecutor creates a BRExecutor with a mock runFunc.
func newTestExecutor(fn func(args ...string) (string, error)) *BRExecutor {
	return &BRExecutor{
		workDir:  "/test",
		beadsDir: "/test/.beads",
		runFunc:  fn,
	}
}

func TestBRExecutor_UpdateStatus(t *testing.T) {
	var capturedArgs []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		capturedArgs = args
		return "{}", nil
	})

	err := executor.UpdateStatus("proj-1", task.StatusInProgress)
	require.NoError(t, err)
	require.Equal(t, []string{"update", "proj-1", "--status", "in_progress", "--json"}, capturedArgs)
}

func TestBRExecutor_UpdatePriority(t *testing.T) {
	var capturedArgs []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		capturedArgs = args
		return "{}", nil
	})

	err := executor.UpdatePriority("proj-1", task.PriorityCritical)
	require.NoError(t, err)
	require.Equal(t, []string{"update", "proj-1", "-p", "0", "--json"}, capturedArgs)
}

func TestBRExecutor_UpdateType(t *testing.T) {
	var capturedArgs []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		capturedArgs = args
		return "{}", nil
	})

	err := executor.UpdateType("proj-1", task.TypeBug)
	require.NoError(t, err)
	require.Equal(t, []string{"update", "proj-1", "-t", "bug", "--json"}, capturedArgs)
}

func TestBRExecutor_UpdateTitle(t *testing.T) {
	var capturedArgs []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		capturedArgs = args
		return "{}", nil
	})

	err := executor.UpdateTitle("proj-1", "New Title")
	require.NoError(t, err)
	require.Equal(t, []string{"update", "proj-1", "--title", "New Title", "--json"}, capturedArgs)
}

func TestBRExecutor_UpdateDescription(t *testing.T) {
	var capturedArgs []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		capturedArgs = args
		return "{}", nil
	})

	err := executor.UpdateDescription("proj-1", "New description")
	require.NoError(t, err)
	require.Equal(t, []string{"update", "proj-1", "--description", "New description", "--json"}, capturedArgs)
}

func TestBRExecutor_UpdateNotes(t *testing.T) {
	var capturedArgs []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		capturedArgs = args
		return "{}", nil
	})

	err := executor.UpdateNotes("proj-1", "New notes")
	require.NoError(t, err)
	require.Equal(t, []string{"update", "proj-1", "--notes", "New notes", "--json"}, capturedArgs)
}

func TestBRExecutor_CloseIssue(t *testing.T) {
	var capturedArgs []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		capturedArgs = args
		return "{}", nil
	})

	err := executor.CloseIssue("proj-1", "Done")
	require.NoError(t, err)
	require.Equal(t, []string{"close", "proj-1", "--force", "--json", "--reason", "Done"}, capturedArgs)
}

func TestBRExecutor_CloseIssue_NoReason(t *testing.T) {
	var capturedArgs []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		capturedArgs = args
		return "{}", nil
	})

	err := executor.CloseIssue("proj-1", "")
	require.NoError(t, err)
	require.Equal(t, []string{"close", "proj-1", "--force", "--json"}, capturedArgs)
}

func TestBRExecutor_ReopenIssue(t *testing.T) {
	var capturedArgs []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		capturedArgs = args
		return "{}", nil
	})

	err := executor.ReopenIssue("proj-1")
	require.NoError(t, err)
	require.Equal(t, []string{"reopen", "proj-1", "--json"}, capturedArgs)
}

func TestBRExecutor_DeleteIssues(t *testing.T) {
	var capturedArgs []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		capturedArgs = args
		return "{}", nil
	})

	err := executor.DeleteIssues([]string{"proj-1", "proj-2"})
	require.NoError(t, err)
	require.Equal(t, []string{"delete", "proj-1", "proj-2", "--force", "--json"}, capturedArgs)
}

func TestBRExecutor_DeleteIssues_Empty(t *testing.T) {
	called := false
	executor := newTestExecutor(func(args ...string) (string, error) {
		called = true
		return "", nil
	})

	err := executor.DeleteIssues(nil)
	require.NoError(t, err)
	require.False(t, called)
}

func TestBRExecutor_SetLabels(t *testing.T) {
	var capturedArgs []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		capturedArgs = args
		return "{}", nil
	})

	err := executor.SetLabels("proj-1", []string{"bug", "urgent"})
	require.NoError(t, err)
	require.Equal(t, []string{"update", "proj-1", "--set-labels", "bug", "--set-labels", "urgent", "--json"}, capturedArgs)
}

func TestBRExecutor_SetLabels_Clear(t *testing.T) {
	calls := 0
	executor := newTestExecutor(func(args ...string) (string, error) {
		calls++
		if calls == 1 {
			// br show call to get current labels.
			return `[{"id":"proj-1","title":"Test","status":"open","issue_type":"task","created_at":"2026-03-06T12:00:00Z","updated_at":"2026-03-06T12:00:00Z","labels":["old-label"]}]`, nil
		}
		// Remove label call.
		require.Contains(t, args, "--remove-label")
		require.Contains(t, args, "old-label")
		return "{}", nil
	})

	err := executor.SetLabels("proj-1", []string{})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}

func TestBRExecutor_AddComment(t *testing.T) {
	var capturedArgs []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		capturedArgs = args
		return "{}", nil
	})

	err := executor.AddComment("proj-1", "alice", "Great work!")
	require.NoError(t, err)
	require.Equal(t, []string{"comments", "add", "proj-1", "Great work!", "--json", "--actor", "alice"}, capturedArgs)
}

func TestBRExecutor_CreateEpic(t *testing.T) {
	executor := newTestExecutor(func(args ...string) (string, error) {
		require.Contains(t, args, "-t")
		require.Contains(t, args, "epic")
		return `{"id":"proj-abc","title":"New Epic"}`, nil
	})

	result, err := executor.CreateEpic("New Epic", "Description", []string{"team-a"})
	require.NoError(t, err)
	require.Equal(t, "proj-abc", result.ID)
	require.Equal(t, "New Epic", result.Title)
}

func TestBRExecutor_CreateTask(t *testing.T) {
	executor := newTestExecutor(func(args ...string) (string, error) {
		require.Contains(t, args, "--parent")
		require.Contains(t, args, "proj-epic")
		return `{"id":"proj-epic.1","title":"New Task"}`, nil
	})

	result, err := executor.CreateTask("New Task", "Do this", "proj-epic", "alice", nil)
	require.NoError(t, err)
	require.Equal(t, "proj-epic.1", result.ID)
}

func TestBRExecutor_AddDependency(t *testing.T) {
	var capturedArgs []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		capturedArgs = args
		return "{}", nil
	})

	err := executor.AddDependency("proj-1", "proj-2")
	require.NoError(t, err)
	require.Equal(t, []string{"dep", "add", "proj-1", "proj-2", "--json"}, capturedArgs)
}

func TestBRExecutor_UpdateIssue_AllFields(t *testing.T) {
	var allCalls [][]string
	executor := newTestExecutor(func(args ...string) (string, error) {
		allCalls = append(allCalls, args)
		if strings.Contains(strings.Join(args, " "), "show") {
			return `[{"id":"proj-1","title":"Test","status":"open","issue_type":"task","created_at":"2026-03-06T12:00:00Z","updated_at":"2026-03-06T12:00:00Z"}]`, nil
		}
		return "{}", nil
	})

	title := "Updated Title"
	desc := "Updated Desc"
	notes := "Updated Notes"
	p := task.PriorityHigh
	s := task.StatusInProgress
	labels := []string{"new-label"}
	assignee := "bob"
	typ := task.TypeBug

	err := executor.UpdateIssue("proj-1", task.UpdateOptions{
		Title:       &title,
		Description: &desc,
		Notes:       &notes,
		Priority:    &p,
		Status:      &s,
		Labels:      &labels,
		Assignee:    &assignee,
		Type:        &typ,
	})
	require.NoError(t, err)

	// First call: non-label update.
	require.GreaterOrEqual(t, len(allCalls), 1)
	firstCall := allCalls[0]
	require.Contains(t, firstCall, "--title")
	require.Contains(t, firstCall, "--description")
	require.Contains(t, firstCall, "--notes")
	require.Contains(t, firstCall, "-p")
	require.Contains(t, firstCall, "--status")
	require.Contains(t, firstCall, "--assignee")
	require.Contains(t, firstCall, "-t")

	// Second call: label update.
	require.GreaterOrEqual(t, len(allCalls), 2)
	secondCall := allCalls[1]
	require.Contains(t, secondCall, "--set-labels")
}

func TestBRExecutor_UpdateIssue_ParentID(t *testing.T) {
	var captured []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		captured = args
		return "{}", nil
	})

	empty := ""
	require.NoError(t, executor.UpdateIssue("proj-1", task.UpdateOptions{ParentID: &empty}))
	require.Equal(t, []string{"update", "proj-1", "--parent", "", "--json"}, captured)
}

func TestBRExecutor_UpdateIssue_NoFields(t *testing.T) {
	called := false
	executor := newTestExecutor(func(args ...string) (string, error) {
		called = true
		return "{}", nil
	})

	err := executor.UpdateIssue("proj-1", task.UpdateOptions{})
	require.NoError(t, err)
	require.False(t, called, "should not call br when no fields set")
}
