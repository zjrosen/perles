package shared

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"perles/internal/beads"
	"perles/internal/mocks"
)

func TestGetAllDescendants_NoChildren(t *testing.T) {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{
		{ID: "epic-1", Type: beads.TypeEpic},
	}, nil)

	result := GetAllDescendants(mockExecutor, "epic-1")

	require.Len(t, result, 1)
	require.Equal(t, "epic-1", result[0].ID)
}

func TestGetAllDescendants_WithChildren(t *testing.T) {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{
		{ID: "epic-1", Type: beads.TypeEpic},
		{ID: "task-1", Type: beads.TypeTask},
		{ID: "task-2", Type: beads.TypeTask},
	}, nil)

	result := GetAllDescendants(mockExecutor, "epic-1")

	require.Len(t, result, 3, "should have 3 issues (root + 2 children)")
	ids := make([]string, len(result))
	for i, issue := range result {
		ids[i] = issue.ID
	}
	require.Contains(t, ids, "epic-1")
	require.Contains(t, ids, "task-1")
	require.Contains(t, ids, "task-2")
}

func TestGetAllDescendants_NestedChildren(t *testing.T) {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{
		{ID: "epic-1", Type: beads.TypeEpic},
		{ID: "sub-epic-1", Type: beads.TypeEpic},
		{ID: "task-1", Type: beads.TypeTask},
		{ID: "task-2", Type: beads.TypeTask},
		{ID: "grandchild-1", Type: beads.TypeTask},
	}, nil)

	result := GetAllDescendants(mockExecutor, "epic-1")

	require.Len(t, result, 5, "should have 5 issues (root + 2 children + 2 grandchildren)")
	ids := make([]string, len(result))
	for i, issue := range result {
		ids[i] = issue.ID
	}
	require.Contains(t, ids, "epic-1")
	require.Contains(t, ids, "sub-epic-1")
	require.Contains(t, ids, "task-1")
	require.Contains(t, ids, "task-2")
	require.Contains(t, ids, "grandchild-1")
}

func TestGetAllDescendants_BQLError(t *testing.T) {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return(nil, errors.New("BQL query failed"))

	result := GetAllDescendants(mockExecutor, "epic-1")

	require.Nil(t, result, "BQL error should return nil")
}

func TestGetAllDescendants_EmptyBQLResult(t *testing.T) {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{}, nil)

	result := GetAllDescendants(mockExecutor, "epic-1")

	require.Empty(t, result, "empty BQL result should return empty slice")
}

func TestCreateDeleteModal_RegularIssue_ReturnsCorrectIDs(t *testing.T) {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	// No expectations needed - Execute won't be called for non-epic

	issue := &beads.Issue{
		ID:        "task-1",
		TitleText: "Test Task",
		Type:      beads.TypeTask,
	}

	modal, isCascade, issueIDs := CreateDeleteModal(issue, mockExecutor)

	require.NotNil(t, modal)
	require.False(t, isCascade, "regular issue should not be cascade")
	require.Equal(t, []string{"task-1"}, issueIDs, "should return single-element slice")
}

func TestCreateDeleteModal_EpicWithChildren_ReturnsAllDescendants(t *testing.T) {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{
		{ID: "epic-1", Type: beads.TypeEpic, TitleText: "Root Epic"},
		{ID: "task-1", Type: beads.TypeTask, TitleText: "Child Task 1"},
		{ID: "task-2", Type: beads.TypeTask, TitleText: "Child Task 2"},
	}, nil)

	issue := &beads.Issue{
		ID:        "epic-1",
		TitleText: "Root Epic",
		Type:      beads.TypeEpic,
		Children:  []string{"task-1", "task-2"},
	}

	modal, isCascade, issueIDs := CreateDeleteModal(issue, mockExecutor)

	require.NotNil(t, modal)
	require.True(t, isCascade, "epic with children should be cascade")
	require.Len(t, issueIDs, 3, "should return all 3 IDs")
	require.Contains(t, issueIDs, "epic-1")
	require.Contains(t, issueIDs, "task-1")
	require.Contains(t, issueIDs, "task-2")
}

func TestCreateDeleteModal_EpicWithNestedChildren_ReturnsAllDescendants(t *testing.T) {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{
		{ID: "epic-1", Type: beads.TypeEpic, TitleText: "Root Epic"},
		{ID: "sub-epic-1", Type: beads.TypeEpic, TitleText: "Sub Epic"},
		{ID: "task-1", Type: beads.TypeTask, TitleText: "Child Task"},
		{ID: "grandchild-1", Type: beads.TypeTask, TitleText: "Grandchild Task"},
	}, nil)

	issue := &beads.Issue{
		ID:        "epic-1",
		TitleText: "Root Epic",
		Type:      beads.TypeEpic,
		Children:  []string{"sub-epic-1", "task-1"}, // Immediate children
	}

	modal, isCascade, issueIDs := CreateDeleteModal(issue, mockExecutor)

	require.NotNil(t, modal)
	require.True(t, isCascade)
	require.Len(t, issueIDs, 4, "should return all 4 IDs including grandchild")
	require.Contains(t, issueIDs, "epic-1")
	require.Contains(t, issueIDs, "sub-epic-1")
	require.Contains(t, issueIDs, "task-1")
	require.Contains(t, issueIDs, "grandchild-1")
}

func TestCreateDeleteModal_EpicWithoutChildren_NotCascade(t *testing.T) {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	// No expectations needed - Execute won't be called for epic without children

	issue := &beads.Issue{
		ID:        "epic-1",
		TitleText: "Empty Epic",
		Type:      beads.TypeEpic,
		Children:  []string{}, // No children
	}

	modal, isCascade, issueIDs := CreateDeleteModal(issue, mockExecutor)

	require.NotNil(t, modal)
	require.False(t, isCascade, "epic without children should not be cascade")
	require.Equal(t, []string{"epic-1"}, issueIDs)
}
