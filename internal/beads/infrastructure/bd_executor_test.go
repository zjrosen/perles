package infrastructure

import (
	"errors"
	"path/filepath"
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

// TestBDExecutor_NewBDExecutor_RelativeBeadsDirIsAbsolute verifies a relative
// beads dir is resolved against perles' cwd, not bd's working directory.
func TestBDExecutor_NewBDExecutor_RelativeBeadsDirIsAbsolute(t *testing.T) {
	executor := NewBDExecutor("/elsewhere", filepath.Join("project", ".beads"))

	want, err := filepath.Abs(filepath.Join("project", ".beads"))
	require.NoError(t, err)
	require.Equal(t, want, executor.beadsDir)
	require.True(t, filepath.IsAbs(executor.beadsDir))

	require.Empty(t, NewBDExecutor("", "").beadsDir, "empty beads dir stays empty (BEADS_DIR not set)")
}

// TestBDExecutor_UpdateTitle_MethodExists verifies UpdateTitle exists with correct signature.
func TestBDExecutor_UpdateTitle_MethodExists(t *testing.T) {
	executor := NewBDExecutor("", "")
	var updateTitleFunc func(issueID, title string) error = executor.UpdateTitle
	require.NotNil(t, updateTitleFunc, "UpdateTitle method should exist")
}

// TestBDExecutor_UpdateDescription_MethodExists verifies UpdateDescription exists with correct signature.
func TestBDExecutor_UpdateDescription_MethodExists(t *testing.T) {
	executor := NewBDExecutor("", "")
	var updateDescFunc func(issueID, description string) error = executor.UpdateDescription
	require.NotNil(t, updateDescFunc, "UpdateDescription method should exist")
}

// TestBDExecutor_UpdateNotes_MethodExists verifies UpdateNotes exists with correct signature.
func TestBDExecutor_UpdateNotes_MethodExists(t *testing.T) {
	executor := NewBDExecutor("", "")
	var updateNotesFunc func(issueID, notes string) error = executor.UpdateNotes
	require.NotNil(t, updateNotesFunc, "UpdateNotes method should exist")
}

// newTestExecutor creates a BDExecutor whose bd runs are answered by fn: a
// nil error is a successful run printing the returned stdout, a non-nil
// error is a failed run (exit 1) whose stderr is the error text.
func newTestExecutor(fn func(args ...string) (string, error)) *BDExecutor {
	return newRawTestExecutor(func(args ...string) bdOutput {
		stdout, err := fn(args...)
		if err != nil {
			return bdFailed(1, "", err.Error())
		}
		return bdOutput{Stdout: stdout}
	})
}

// newRawTestExecutor creates a BDExecutor whose bd runs are answered by fn.
func newRawTestExecutor(fn func(args ...string) bdOutput) *BDExecutor {
	e := NewBDExecutor("", "")
	e.runFunc = fn
	return e
}

// bdFailed is a failed bd run with the given exit code and output.
func bdFailed(code int, stdout, stderr string) bdOutput {
	return bdOutput{Stdout: stdout, Stderr: stderr, ExitCode: code, Err: errors.New("exit status")}
}

// recordCalls returns a run func that records each call's args and answers
// with respond (nil respond means success with empty stdout).
func recordCalls(calls *[][]string, respond func(args []string) bdOutput) func(args ...string) bdOutput {
	return func(args ...string) bdOutput {
		*calls = append(*calls, append([]string(nil), args...))
		if respond == nil {
			return bdOutput{}
		}
		return respond(args)
	}
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
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
	require.Equal(t, []string{"update", "PROJ-1", "--title=New Title", "--json"}, captured)
}

func TestBDExecutor_UpdateIssue_ParentID(t *testing.T) {
	var captured []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		captured = args
		return "", nil
	})

	parent := "PROJ-9"
	require.NoError(t, executor.UpdateIssue("PROJ-1", domain.UpdateIssueOptions{ParentID: &parent}))
	require.Equal(t, []string{"update", "PROJ-1", "--parent=PROJ-9", "--json"}, captured)

	// Empty string removes the parent.
	empty := ""
	require.NoError(t, executor.UpdateIssue("PROJ-1", domain.UpdateIssueOptions{ParentID: &empty}))
	require.Equal(t, []string{"update", "PROJ-1", "--parent=", "--json"}, captured)
}

// TestBDExecutor_UserTextStartingWithDash verifies user text is passed as a
// single --flag=value argument, so bd never parses it as a flag.
func TestBDExecutor_UserTextStartingWithDash(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, nil))

	title := "--force everything"
	desc := "-d not a flag"
	require.NoError(t, executor.UpdateIssue("PROJ-1", domain.UpdateIssueOptions{Title: &title, Description: &desc}))
	require.NoError(t, executor.UpdateTitle("PROJ-1", "-t epic"))
	require.NoError(t, executor.UpdateDescription("PROJ-1", "--json"))
	require.NoError(t, executor.CloseIssue("PROJ-1", "--force"))

	require.Equal(t, []string{"update", "PROJ-1", "--title=--force everything", "--description=-d not a flag", "--json"}, calls[0])
	require.Equal(t, []string{"update", "PROJ-1", "--title=-t epic", "--json"}, calls[1])
	require.Equal(t, []string{"update", "PROJ-1", "--description=--json", "--json"}, calls[2])
	require.Equal(t, []string{"close", "PROJ-1", "--reason=--force", "--json"}, calls[3])
}

func TestBDExecutor_SingleFieldUpdates_UseFlagEqualsValue(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, nil))

	require.NoError(t, executor.UpdateStatus("PROJ-1", domain.StatusInProgress))
	require.NoError(t, executor.UpdatePriority("PROJ-1", domain.PriorityHigh))
	require.NoError(t, executor.UpdateType("PROJ-1", domain.TypeBug))
	require.NoError(t, executor.AddDependency("PROJ-1", "PROJ-2"))

	require.Equal(t, [][]string{
		{"update", "PROJ-1", "--status=in_progress", "--json"},
		{"update", "PROJ-1", "--priority=1", "--json"},
		{"update", "PROJ-1", "--type=bug", "--json"},
		{"dep", "add", "PROJ-1", "PROJ-2", "--type=blocks"},
	}, calls)
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
	require.Equal(t, "c-1", comments[0].ID)
	require.Equal(t, "alice", comments[0].Author)
	require.Equal(t, "First", comments[0].Text)
	require.Equal(t, "bob", comments[1].Author)
	require.Equal(t, "Second", comments[1].Text)
}

func TestBDExecutor_GetComments_EnvelopeAndNull(t *testing.T) {
	executor := newTestExecutor(func(args ...string) (string, error) {
		return `{"schema_version":1,"data":[{"id":7,"author":"alice","text":"Wrapped","created_at":"2026-04-16T12:00:00Z"}]}`, nil
	})
	comments, err := executor.GetComments("PROJ-1")
	require.NoError(t, err)
	require.Len(t, comments, 1)
	require.Equal(t, "7", comments[0].ID, "numeric ids from older bd decode as strings")
	require.Equal(t, "Wrapped", comments[0].Text)

	empty := newTestExecutor(func(args ...string) (string, error) { return "null", nil })
	comments, err = empty.GetComments("PROJ-1")
	require.NoError(t, err)
	require.Empty(t, comments)
}

func TestBDExecutor_AddComment(t *testing.T) {
	var captured []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		captured = args
		return "", nil
	})

	err := executor.AddComment("PROJ-1", "alice", "Great work!")
	require.NoError(t, err)
	require.Equal(t, []string{"comments", "add", "PROJ-1", "--json", "--actor=alice", "--", "Great work!"}, captured)

	require.NoError(t, executor.AddComment("PROJ-1", "", "--looks like a flag"))
	require.Equal(t, []string{"comments", "add", "PROJ-1", "--json", "--", "--looks like a flag"}, captured)
}

// TestBDExecutor_UpdateIssue_MultipleFields verifies correct CLI args with Title + Priority + Labels.
// Labels require a separate bd update call because --set-labels cannot be combined with other flags.
func TestBDExecutor_UpdateIssue_MultipleFields(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, nil))

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
	require.Equal(t, [][]string{
		{"update", "PROJ-2", "--title=Updated Title", "--priority=1", "--json"},
		{"update", "PROJ-2", "--set-labels=bug,urgent", "--json"},
	}, calls)
}

// TestBDExecutor_UpdateIssue_AllFields verifies the call sequence when all
// fields are set: other fields, then notes on their own, then labels.
func TestBDExecutor_UpdateIssue_AllFields(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, nil))

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
	require.Equal(t, [][]string{
		{
			"update", "PROJ-3",
			"--title=Full Update",
			"--description=A new description",
			"--priority=0",
			"--status=in_progress",
			"--assignee=alice",
			"--type=feature",
			"--json",
		},
		{"update", "PROJ-3", "--notes=Some notes", "--json"},
		{"update", "PROJ-3", "--set-labels=feature,v2", "--json"},
	}, calls)
}

// TestBDExecutor_UpdateIssue_NoFieldsSet verifies no-op when all fields are nil (no CLI call).
func TestBDExecutor_UpdateIssue_NoFieldsSet(t *testing.T) {
	called := false
	executor := newTestExecutor(func(args ...string) (string, error) {
		called = true
		return "", nil
	})

	err := executor.UpdateIssue("PROJ-4", domain.UpdateIssueOptions{})
	require.NoError(t, err)
	require.False(t, called, "runBeads should not be called when no fields are set")
}

// TestBDExecutor_UpdateIssue_ErrorPropagation verifies error is returned with issue ID context.
func TestBDExecutor_UpdateIssue_ErrorPropagation(t *testing.T) {
	executor := newTestExecutor(func(args ...string) (string, error) {
		return "", errors.New("connection refused")
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
		"--description=Line 1\nLine 2\nLine 3",
		"--priority=2",
		"--json",
	}, captured)
}

// TestBDExecutor_UpdateIssue_EmptyLabels verifies that clearing all labels
// fetches current labels via ShowIssue and then issues --remove-label for each.
func TestBDExecutor_UpdateIssue_EmptyLabels(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, func(args []string) bdOutput {
		if args[0] == "show" {
			return bdOutput{Stdout: `[{"id":"PROJ-7","title":"T","labels":["bug","urgent"]}]`}
		}
		return bdOutput{}
	}))

	emptyLabels := []string{}
	err := executor.UpdateIssue("PROJ-7", domain.UpdateIssueOptions{Labels: &emptyLabels})
	require.NoError(t, err)
	require.Equal(t, [][]string{
		{"show", "PROJ-7", "--json"},
		{"update", "PROJ-7", "--remove-label=bug", "--remove-label=urgent", "--json"},
	}, calls)
}

// TestBDExecutor_UpdateIssue_EmptyLabels_AlreadyClear verifies no-op when
// clearing labels on an issue that has no labels.
func TestBDExecutor_UpdateIssue_EmptyLabels_AlreadyClear(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, func(args []string) bdOutput {
		if args[0] == "show" {
			return bdOutput{Stdout: `[{"id":"PROJ-7b","title":"T"}]`}
		}
		return bdOutput{}
	}))

	emptyLabels := []string{}
	err := executor.UpdateIssue("PROJ-7b", domain.UpdateIssueOptions{Labels: &emptyLabels})
	require.NoError(t, err)
	require.Equal(t, [][]string{{"show", "PROJ-7b", "--json"}}, calls, "expected only show call, no update needed")
}

// TestBDExecutor_UpdateIssue_LabelsOnly verifies that when only labels are set,
// only the SetLabels call is made (no empty non-label update).
func TestBDExecutor_UpdateIssue_LabelsOnly(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, nil))

	labels := []string{"bug", "critical"}
	err := executor.UpdateIssue("PROJ-8", domain.UpdateIssueOptions{Labels: &labels})
	require.NoError(t, err)
	require.Equal(t, [][]string{{"update", "PROJ-8", "--set-labels=bug,critical", "--json"}}, calls)
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
	err := executor.UpdateIssue("PROJ-9", domain.UpdateIssueOptions{Title: &title, Labels: &labels})
	require.Error(t, err)
	require.Contains(t, err.Error(), "labels")
	require.Equal(t, 2, callCount, "both calls should have been attempted")
}

// TestBDExecutor_UpdateIssue_NilLabels_ClearsAll verifies that nil labels
// (from formmodal when user deselects all) also triggers the clear path.
func TestBDExecutor_UpdateIssue_NilLabels_ClearsAll(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, func(args []string) bdOutput {
		if args[0] == "show" {
			return bdOutput{Stdout: `[{"id":"PROJ-10","title":"T","labels":["old"]}]`}
		}
		return bdOutput{}
	}))

	var nilLabels []string // nil, not []string{}
	err := executor.UpdateIssue("PROJ-10", domain.UpdateIssueOptions{Labels: &nilLabels})
	require.NoError(t, err)
	require.Equal(t, [][]string{
		{"show", "PROJ-10", "--json"},
		{"update", "PROJ-10", "--remove-label=old", "--json"},
	}, calls)
}

// notesOverwriteRefusal is what bd >= 1.3.2 prints (stderr, exit 1) for
// `bd update X --notes=... --json` over existing non-empty notes.
func notesOverwriteRefusal(id string) bdOutput {
	return bdFailed(1, "",
		id+": --notes would replace existing notes; use --force to overwrite (or --append-notes to preserve history)\n"+
			`{"error":"1 of 1 issues failed to update","failed":[{"id":"`+id+`","error":"`+id+`: --notes would replace existing notes; use --force to overwrite (or --append-notes to preserve history)"}],"schema_version":1}`+"\n")
}

func TestBDExecutor_UpdateNotes_RetriesWithForceOnOverwriteRefusal(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, func(args []string) bdOutput {
		if !hasArg(args, "--force") {
			return notesOverwriteRefusal("PROJ-1")
		}
		return bdOutput{Stdout: `[{"id":"PROJ-1"}]`}
	}))

	require.NoError(t, executor.UpdateNotes("PROJ-1", "replacement"))
	require.Equal(t, [][]string{
		{"update", "PROJ-1", "--notes=replacement", "--json"},
		{"update", "PROJ-1", "--notes=replacement", "--force", "--json"},
	}, calls)
}

func TestBDExecutor_UpdateIssue_NotesForcedWhenUnchangedSinceRead(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, func(args []string) bdOutput {
		switch {
		case args[0] == "show":
			return bdOutput{Stdout: `[{"id":"PROJ-1","title":"T","notes":"old notes"}]`}
		case !hasArg(args, "--force"):
			return notesOverwriteRefusal("PROJ-1")
		default:
			return bdOutput{Stdout: `[{"id":"PROJ-1"}]`}
		}
	}))

	notes, expected := "new notes", "old notes"
	err := executor.UpdateIssue("PROJ-1", domain.UpdateIssueOptions{Notes: &notes, ExpectedNotes: &expected})
	require.NoError(t, err)
	require.Equal(t, [][]string{
		{"update", "PROJ-1", "--notes=new notes", "--json"},
		{"show", "PROJ-1", "--json"},
		{"update", "PROJ-1", "--notes=new notes", "--force", "--json"},
	}, calls)
}

func TestBDExecutor_UpdateIssue_NotesNotForcedWhenChangedSinceRead(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, func(args []string) bdOutput {
		if args[0] == "show" {
			// An agent appended to the notes after the editor opened.
			return bdOutput{Stdout: `[{"id":"PROJ-1","title":"T","notes":"old notes\nagent handoff"}]`}
		}
		if hasArg(args, "--force") {
			t.Fatalf("must not force over notes that changed since they were read: %v", args)
		}
		return notesOverwriteRefusal("PROJ-1")
	}))

	notes, expected := "new notes", "old notes"
	err := executor.UpdateIssue("PROJ-1", domain.UpdateIssueOptions{Notes: &notes, ExpectedNotes: &expected})
	require.ErrorIs(t, err, ErrNotesChanged)
	require.Len(t, calls, 2)
}

func TestBDExecutor_UpdateNotes_NoRetryOnOtherErrors(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, func(args []string) bdOutput {
		return bdFailed(1, "", "Error: issue PROJ-1 not found\n")
	}))

	err := executor.UpdateNotes("PROJ-1", "replacement")
	require.Error(t, err)
	require.Len(t, calls, 1, "only the notes-overwrite refusal is retried")
	require.Contains(t, err.Error(), "issue PROJ-1 not found")
}

func TestBDExecutor_UpdateNotes_ForceRetryFailureIsReturned(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, func(args []string) bdOutput {
		if hasArg(args, "--force") {
			return bdFailed(1, "", "Error: template PROJ-1 is read-only\n")
		}
		return notesOverwriteRefusal("PROJ-1")
	}))

	err := executor.UpdateNotes("PROJ-1", "replacement")
	require.Error(t, err)
	require.Len(t, calls, 2, "retried exactly once")
	require.Contains(t, err.Error(), "template PROJ-1 is read-only")
}

func TestBDExecutor_UpdateNotes_ClearUsesClearNotes(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, nil))

	require.NoError(t, executor.UpdateNotes("PROJ-1", ""))
	require.Equal(t, [][]string{{"update", "PROJ-1", "--clear-notes", "--json"}}, calls)
}

func TestBDExecutor_UpdateNotes_ClearFallsBackWhenFlagUnknown(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, func(args []string) bdOutput {
		if hasArg(args, "--clear-notes") {
			// bd <= 1.3.x: cobra rejects the flag before running the command.
			return bdFailed(1, "", "Error: unknown flag: --clear-notes\n")
		}
		return bdOutput{}
	}))

	require.NoError(t, executor.UpdateNotes("PROJ-1", ""))
	require.Equal(t, [][]string{
		{"update", "PROJ-1", "--clear-notes", "--json"},
		{"update", "PROJ-1", "--notes=", "--json"},
	}, calls)
}

func TestBDExecutor_UpdateIssue_NotesSentSeparatelyAfterFields(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, func(args []string) bdOutput {
		if hasArg(args, "--notes=new notes") && !hasArg(args, "--force") {
			return notesOverwriteRefusal("PROJ-2")
		}
		return bdOutput{}
	}))

	status := domain.StatusClosed
	notes := "new notes"
	labels := []string{"done"}
	err := executor.UpdateIssue("PROJ-2", domain.UpdateIssueOptions{Status: &status, Notes: &notes, Labels: &labels})
	require.NoError(t, err)
	require.Equal(t, [][]string{
		{"update", "PROJ-2", "--status=closed", "--json"},
		{"update", "PROJ-2", "--notes=new notes", "--json"},
		{"update", "PROJ-2", "--notes=new notes", "--force", "--json"},
		{"update", "PROJ-2", "--set-labels=done", "--json"},
	}, calls)
	require.False(t, hasArg(calls[0], "--force"), "--force must never accompany the status change")
}

func TestBDExecutor_UpdateIssue_ClearNotes(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, nil))

	empty := ""
	require.NoError(t, executor.UpdateIssue("PROJ-3", domain.UpdateIssueOptions{Notes: &empty}))
	require.Equal(t, [][]string{{"update", "PROJ-3", "--clear-notes", "--json"}}, calls)
}

func TestBDExecutor_UpdateIssue_FieldsErrorSkipsNotes(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, func(args []string) bdOutput {
		return bdFailed(1, "", "Error: invalid status\n")
	}))

	status := domain.Status("bogus")
	notes := "n"
	err := executor.UpdateIssue("PROJ-4", domain.UpdateIssueOptions{Status: &status, Notes: &notes})
	require.Error(t, err)
	require.Len(t, calls, 1)
	require.Contains(t, err.Error(), "saving issue PROJ-4: bd update failed: invalid status")
}

func TestBDExecutor_UpdateIssue_NotesErrorWrapped(t *testing.T) {
	executor := newRawTestExecutor(func(args ...string) bdOutput {
		return bdFailed(1, "", "Error: database is locked\n")
	})

	notes := "n"
	err := executor.UpdateIssue("PROJ-5", domain.UpdateIssueOptions{Notes: &notes})
	require.Error(t, err)
	require.Contains(t, err.Error(), "saving issue PROJ-5 notes: bd update failed: database is locked")
}

// TestBDExecutor_UpdateStatus_CloseRefusalReason verifies the close-policy
// refusal (bd >= 1.3) reaches the caller with bd's reason, not an exit status.
func TestBDExecutor_UpdateStatus_CloseRefusalReason(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, func(args []string) bdOutput {
		return bdFailed(1, "",
			"Error updating perles-abc1: cannot close perles-abc1: 2 open child issue(s); close children first or use --force to override\n"+
				`{"error":"1 of 1 issues failed to update","failed":[{"id":"perles-abc1","error":"updating issue: cannot close perles-abc1: 2 open child issue(s); close children first or use --force to override"}],"schema_version":1}`+"\n")
	}))

	err := executor.UpdateStatus("perles-abc1", domain.StatusClosed)
	require.Error(t, err)
	require.Equal(t, [][]string{{"update", "perles-abc1", "--status=closed", "--json"}}, calls, "no --force retry for status")
	require.Equal(t,
		"bd update failed: Error updating perles-abc1: cannot close perles-abc1: 2 open child issue(s); close children first or use --force to override",
		err.Error())
}

func TestBDExecutor_ReopenIssue_UsesReopen(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, nil))

	require.NoError(t, executor.ReopenIssue("PROJ-1"))
	require.Equal(t, [][]string{{"reopen", "PROJ-1", "--json"}}, calls)
}

func TestBDExecutor_ReopenIssue_FallsBackWhenCommandUnknown(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, func(args []string) bdOutput {
		if args[0] == "reopen" {
			return bdFailed(1, "", "Error: unknown command \"reopen\" for \"bd\"\nRun 'bd --help' for usage.\n")
		}
		return bdOutput{}
	}))

	require.NoError(t, executor.ReopenIssue("PROJ-1"))
	require.Equal(t, [][]string{
		{"reopen", "PROJ-1", "--json"},
		{"update", "PROJ-1", "--status=open", "--json"},
	}, calls)
}

func TestBDExecutor_ReopenIssue_OtherErrorsReturned(t *testing.T) {
	var calls [][]string
	executor := newRawTestExecutor(recordCalls(&calls, func(args []string) bdOutput {
		return bdFailed(1, "", "Error reopening PROJ-1: issue is a template\n")
	}))

	err := executor.ReopenIssue("PROJ-1")
	require.Error(t, err)
	require.Len(t, calls, 1)
	require.Contains(t, err.Error(), "bd reopen failed: Error reopening PROJ-1: issue is a template")
}

func TestBDExecutor_CreateTask_Args(t *testing.T) {
	var captured []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		captured = args
		return `{"id":"PROJ-11","title":"-starts with dash","schema_version":1}`, nil
	})

	result, err := executor.CreateTask("-starts with dash", "Desc", "PROJ-1", "alice", []string{"a", "b"})
	require.NoError(t, err)
	require.Equal(t, domain.CreateResult{ID: "PROJ-11", Title: "-starts with dash"}, result)
	require.Equal(t, []string{
		"create", "--title=-starts with dash", "--type=task",
		"--parent=PROJ-1", "--description=Desc", "--assignee=alice",
		"--label=a", "--label=b", "--json",
	}, captured)

	_, err = executor.CreateTask("Top level", "", "", "", nil)
	require.NoError(t, err)
	require.Equal(t, []string{"create", "--title=Top level", "--type=task", "--json"}, captured,
		"empty parent, description and assignee are omitted")
}

func TestBDExecutor_CreateEpic_ArgsAndEnvelope(t *testing.T) {
	var captured []string
	executor := newTestExecutor(func(args ...string) (string, error) {
		captured = args
		return `{"schema_version":1,"data":{"id":"PROJ-12","title":"Epic"}}`, nil
	})

	result, err := executor.CreateEpic("Epic", "Body", []string{"x"})
	require.NoError(t, err)
	require.Equal(t, domain.CreateResult{ID: "PROJ-12", Title: "Epic"}, result)
	require.Equal(t, []string{"create", "--title=Epic", "--type=epic", "--description=Body", "--label=x", "--json"}, captured)
}

func TestBDExecutor_Create_ParseErrors(t *testing.T) {
	executor := newTestExecutor(func(args ...string) (string, error) { return "not json", nil })
	_, err := executor.CreateTask("T", "", "", "", nil)
	require.ErrorContains(t, err, "failed to parse bd create output")

	noID := newTestExecutor(func(args ...string) (string, error) { return `{"title":"T"}`, nil })
	_, err = noID.CreateEpic("T", "", nil)
	require.ErrorContains(t, err, "no issue id")
}

func TestBDExecutor_ShowIssue_MapsBDShape(t *testing.T) {
	executor := newTestExecutor(func(args ...string) (string, error) {
		require.Equal(t, []string{"show", "PROJ-20", "--json"}, args)
		return showFixtureCurrent, nil
	})

	issue, err := executor.ShowIssue("PROJ-20")
	require.NoError(t, err)
	require.Equal(t, "PROJ-20", issue.ID)
	require.Equal(t, domain.TypeBug, issue.Type)
	require.Equal(t, "PROJ-1", issue.ParentID)
	require.Equal(t, []string{"PROJ-5"}, issue.BlockedBy)
	require.Equal(t, []string{"PROJ-6"}, issue.DiscoveredFrom)
	require.Equal(t, []string{"backend", "urgent"}, issue.Labels)
	require.Equal(t, 3, issue.CommentCount)
}

func TestBDExecutor_ShowIssue_NotFound(t *testing.T) {
	executor := newTestExecutor(func(args ...string) (string, error) { return "[]", nil })
	_, err := executor.ShowIssue("PROJ-404")
	require.ErrorContains(t, err, "issue not found: PROJ-404")
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
