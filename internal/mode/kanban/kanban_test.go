package kanban

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"perles/internal/beads"
	"perles/internal/config"
	"perles/internal/mode"
	"perles/internal/ui/details"
	"perles/internal/ui/modal"
	"perles/internal/ui/toaster"
)

// createTestModel creates a minimal Model for testing state transitions.
// It does not require a database connection.
func createTestModel() Model {
	cfg := config.Defaults()
	services := mode.Services{
		Config: &cfg,
	}

	return Model{
		services: services,
		width:    100,
		height:   40,
		view:     ViewBoard,
		toaster:  toaster.Model{},
	}
}

func TestDeleteFlow_CancelReturnsToDetails(t *testing.T) {
	m := createTestModel()

	// Set up delete confirm view
	issue := beads.Issue{
		ID:        "test-123",
		TitleText: "Test Issue",
		Type:      beads.TypeTask,
	}
	m.view = ViewDeleteConfirm
	m.details = details.New(issue, nil).SetSize(100, 40)
	m.selectedIssue = &issue

	// Simulate modal cancel
	m, _ = m.handleModalCancel()

	assert.Equal(t, ViewDetails, m.view, "expected ViewDetails after cancel")
	assert.Nil(t, m.selectedIssue, "expected selectedIssue to be cleared")
}

func TestDeleteFlow_SubmitTriggersDelete(t *testing.T) {
	m := createTestModel()

	// Set up delete confirm view with selected issue
	issue := beads.Issue{
		ID:        "test-123",
		TitleText: "Test Issue",
		Type:      beads.TypeTask,
	}
	m.view = ViewDeleteConfirm
	m.selectedIssue = &issue

	// Simulate modal submit
	m, cmd := m.handleModalSubmit(modal.SubmitMsg{})

	// Should return a delete command
	assert.NotNil(t, cmd, "expected delete command")
	assert.Nil(t, m.selectedIssue, "expected selectedIssue to be cleared")
}

func TestDeleteFlow_IssueDeletedMsgReturnsToBoard(t *testing.T) {
	m := createTestModel()

	// Simulate receiving success message
	msg := issueDeletedMsg{
		issueID: "test-123",
		err:     nil,
	}
	m, _ = m.handleIssueDeleted(msg)

	assert.Equal(t, ViewBoard, m.view, "expected ViewBoard after successful delete")
	assert.True(t, m.toaster.Visible(), "expected toaster to show success message")
}

func TestDeleteFlow_IssueDeletedMsgWithErrorShowsError(t *testing.T) {
	m := createTestModel()

	// Simulate receiving error message
	msg := issueDeletedMsg{
		issueID: "test-123",
		err:     assert.AnError,
	}
	m, _ = m.handleIssueDeleted(msg)

	assert.Equal(t, ViewBoard, m.view, "expected ViewBoard after error")
	assert.Error(t, m.err, "expected error to be set")
	assert.Equal(t, "deleting issue", m.errContext)
}

func TestCreateDeleteModal_RegularIssue(t *testing.T) {
	m := createTestModel()

	issue := &beads.Issue{
		ID:        "test-456",
		TitleText: "Issue to Delete",
		Type:      beads.TypeTask,
	}

	modal, isCascade := m.createDeleteModal(issue)

	assert.NotNil(t, modal)
	assert.False(t, isCascade, "expected non-cascade for regular task")
}

func TestCreateDeleteModal_EpicWithoutChildren(t *testing.T) {
	m := createTestModel()

	issue := &beads.Issue{
		ID:        "epic-1",
		TitleText: "Epic Without Children",
		Type:      beads.TypeEpic,
		Blocks:    []string{}, // No children
	}

	modal, isCascade := m.createDeleteModal(issue)

	assert.NotNil(t, modal)
	assert.False(t, isCascade, "expected non-cascade for epic without children")
}

func TestCreateDeleteModal_EpicWithChildren(t *testing.T) {
	m := createTestModel()

	issue := &beads.Issue{
		ID:        "epic-1",
		TitleText: "Epic With Children",
		Type:      beads.TypeEpic,
		Blocks:    []string{"task-1", "task-2", "task-3"},
	}

	modal, isCascade := m.createDeleteModal(issue)

	assert.NotNil(t, modal)
	assert.True(t, isCascade, "expected cascade for epic with children")
}

func TestDeleteFlow_CascadeSubmit(t *testing.T) {
	m := createTestModel()

	// Set up cascade delete scenario
	issue := beads.Issue{
		ID:        "epic-1",
		TitleText: "Epic With Children",
		Type:      beads.TypeEpic,
		Blocks:    []string{"task-1", "task-2"},
	}
	m.view = ViewDeleteConfirm
	m.selectedIssue = &issue
	m.deleteIsCascade = true

	// Simulate modal submit
	m, cmd := m.handleModalSubmit(modal.SubmitMsg{})

	// Should return a delete command
	assert.NotNil(t, cmd, "expected delete command")
	assert.Nil(t, m.selectedIssue, "expected selectedIssue to be cleared")
	assert.False(t, m.deleteIsCascade, "expected deleteIsCascade to be cleared")
}

func TestDeleteFlow_CancelClearsCascadeFlag(t *testing.T) {
	m := createTestModel()

	issue := beads.Issue{
		ID:        "epic-1",
		TitleText: "Epic",
		Type:      beads.TypeEpic,
	}
	m.view = ViewDeleteConfirm
	m.selectedIssue = &issue
	m.deleteIsCascade = true

	// Simulate cancel
	m, _ = m.handleModalCancel()

	assert.False(t, m.deleteIsCascade, "expected deleteIsCascade to be cleared on cancel")
}

func TestDeleteFlow_SubmitWithNoSelectedIssue(t *testing.T) {
	m := createTestModel()

	// Set up delete confirm view but NO selected issue
	m.view = ViewDeleteConfirm
	m.selectedIssue = nil

	// Simulate modal submit
	m, cmd := m.handleModalSubmit(modal.SubmitMsg{})

	// Should return to board, not crash
	assert.Equal(t, ViewBoard, m.view, "expected ViewBoard when no issue selected")
	assert.Nil(t, cmd, "expected no command when no issue selected")
}
