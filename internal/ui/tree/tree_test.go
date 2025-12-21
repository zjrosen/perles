package tree

import (
	"testing"
	"time"

	"perles/internal/beads"
	"perles/internal/mocks"
	"perles/internal/mode/shared"

	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
)

// testClockTime is a fixed time for deterministic test output.
var testClockTime = time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)

// testCreatedAt is 2 days before testClockTime
var testCreatedAt = time.Date(2025, 1, 13, 12, 0, 0, 0, time.UTC)

// newTestClock creates a MockClock that always returns testClockTime.
func newTestClock(t *testing.T) shared.Clock {
	clock := mocks.NewMockClock(t)
	clock.EXPECT().Now().Return(testClockTime).Maybe()
	return clock
}

func makeTestIssueMap() map[string]*beads.Issue {
	return map[string]*beads.Issue{
		"epic-1": {
			ID:           "epic-1",
			TitleText:    "Epic One",
			Status:       beads.StatusOpen,
			Type:         beads.TypeEpic,
			Priority:     beads.PriorityHigh,
			Children:     []string{"task-1", "task-2"},
			CreatedAt:    testCreatedAt,
			CommentCount: 3,
		},
		"task-1": {
			ID:           "task-1",
			TitleText:    "Task One",
			Status:       beads.StatusClosed,
			Type:         beads.TypeTask,
			Priority:     beads.PriorityCritical,
			ParentID:     "epic-1",
			CreatedAt:    testCreatedAt,
			CommentCount: 0,
		},
		"task-2": {
			ID:           "task-2",
			TitleText:    "Task Two",
			Status:       beads.StatusOpen,
			Type:         beads.TypeTask,
			Priority:     beads.PriorityMedium,
			ParentID:     "epic-1",
			Children:     []string{"subtask-1"},
			CreatedAt:    testCreatedAt,
			CommentCount: 1,
		},
		"subtask-1": {
			ID:           "subtask-1",
			TitleText:    "Subtask One",
			Status:       beads.StatusInProgress,
			Type:         beads.TypeTask,
			Priority:     beads.PriorityMedium,
			ParentID:     "task-2",
			CreatedAt:    testCreatedAt,
			CommentCount: 5,
		},
	}
}

func TestNew_Basic(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	require.NotNil(t, m)
	require.NotNil(t, m.root)
	require.Equal(t, "epic-1", m.root.Issue.ID)
	require.Equal(t, DirectionDown, m.direction)
	require.Equal(t, "epic-1", m.originalID)
	require.Equal(t, 0, m.cursor)
	require.Len(t, m.nodes, 4) // epic-1, task-1, task-2, subtask-1
}

func TestNew_InvalidRoot(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("nonexistent", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	require.NotNil(t, m)
	require.Nil(t, m.root)
	require.Empty(t, m.nodes)
}

func TestSetSize(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	m.SetSize(80, 24)
	require.Equal(t, 80, m.width)
	require.Equal(t, 24, m.height)
}

func TestMoveCursor_Basic(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	require.Equal(t, 0, m.cursor)

	m.MoveCursor(1)
	require.Equal(t, 1, m.cursor)

	m.MoveCursor(1)
	require.Equal(t, 2, m.cursor)

	m.MoveCursor(-1)
	require.Equal(t, 1, m.cursor)
}

func TestMoveCursor_Bounds(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	// Try to go above top
	m.MoveCursor(-10)
	require.Equal(t, 0, m.cursor)

	// Try to go below bottom
	m.MoveCursor(100)
	require.Equal(t, 3, m.cursor) // Last node (4 nodes, index 3)

	// Still at bottom after trying to go further
	m.MoveCursor(1)
	require.Equal(t, 3, m.cursor)
}

func TestSelectedNode(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	node := m.SelectedNode()
	require.NotNil(t, node)
	require.Equal(t, "epic-1", node.Issue.ID)

	m.MoveCursor(1)
	node = m.SelectedNode()
	require.Equal(t, "task-1", node.Issue.ID)
}

func TestSelectedNode_Empty(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("nonexistent", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	node := m.SelectedNode()
	require.Nil(t, node)
}

func TestRoot(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	root := m.Root()
	require.NotNil(t, root)
	require.Equal(t, "epic-1", root.Issue.ID)
}

func TestDirection(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	require.Equal(t, DirectionDown, m.Direction())

	m.SetDirection(DirectionUp)
	require.Equal(t, DirectionUp, m.Direction())
}

func TestRefocus_AndGoBack(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	// Refocus on task-2
	err := m.Refocus("task-2")
	require.NoError(t, err)
	require.Equal(t, "task-2", m.root.Issue.ID)
	require.Len(t, m.rootStack, 1)
	require.Equal(t, "epic-1", m.rootStack[0])

	// Go back
	needsRequery, _ := m.GoBack()
	require.False(t, needsRequery)
	require.Equal(t, "epic-1", m.root.Issue.ID)
	require.Empty(t, m.rootStack)
}

func TestRefocus_MultipleAndGoToOriginal(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	// Refocus twice
	_ = m.Refocus("task-2")
	_ = m.Refocus("subtask-1")
	require.Len(t, m.rootStack, 2)
	require.Equal(t, "subtask-1", m.root.Issue.ID)

	// Go to original
	err := m.GoToOriginal()
	require.NoError(t, err)
	require.Equal(t, "epic-1", m.root.Issue.ID)
	require.Empty(t, m.rootStack)
}

func TestGoBack_EmptyStack_NoParent(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	// GoBack on empty stack with no parent should do nothing
	needsRequery, parentID := m.GoBack()
	require.False(t, needsRequery)
	require.Empty(t, parentID)
	require.Equal(t, "epic-1", m.root.Issue.ID)
}

func TestGoBack_EmptyStack_WithParentInMap(t *testing.T) {
	issueMap := makeTestIssueMap()
	// Start directly on task-1 which has parent epic-1
	m := New("task-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	require.Equal(t, "task-1", m.root.Issue.ID)
	require.Empty(t, m.rootStack)

	// GoBack should navigate to parent (epic-1 is in the map)
	needsRequery, _ := m.GoBack()
	require.False(t, needsRequery)
	require.Equal(t, "epic-1", m.root.Issue.ID)
}

func TestGoBack_EmptyStack_ParentNotInMap(t *testing.T) {
	// Create a minimal issue map without the parent
	issueMap := map[string]*beads.Issue{
		"task-1": {
			ID:        "task-1",
			TitleText: "Task One",
			Status:    beads.StatusOpen,
			ParentID:  "missing-parent",
			CreatedAt: testCreatedAt,
		},
	}
	m := New("task-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	// GoBack should signal re-query needed when parent not in map
	needsRequery, parentID := m.GoBack()
	require.True(t, needsRequery)
	require.Equal(t, "missing-parent", parentID)
	// Root should be unchanged
	require.Equal(t, "task-1", m.root.Issue.ID)
}

func TestView_Basic(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))
	m.SetSize(80, 24)

	view := m.View()

	// Should contain issue IDs
	require.Contains(t, view, "epic-1")
	require.Contains(t, view, "task-1")
	require.Contains(t, view, "task-2")
	require.Contains(t, view, "subtask-1")

	// Should contain selection indicator
	require.Contains(t, view, ">")
}

func TestView_UpDirection(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("task-1", issueMap, DirectionUp, ModeDeps, newTestClock(t))
	m.SetSize(80, 24)

	// Direction should be up (parent container uses this for border title)
	require.Equal(t, DirectionUp, m.Direction())

	// View should still render the tree nodes
	view := m.View()
	require.Contains(t, view, "task-1")
}

func TestView_TreeBranches(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))
	m.SetSize(80, 24)

	view := m.View()

	// Should contain tree branch characters
	require.Contains(t, view, "├─")
	require.Contains(t, view, "└─")
}

func TestView_StatusIndicators(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))
	m.SetSize(80, 24)

	view := m.View()

	// Should have status indicators
	require.Contains(t, view, "✓") // closed
	require.Contains(t, view, "○") // open
	require.Contains(t, view, "●") // in_progress
}

func TestView_Empty(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("nonexistent", issueMap, DirectionDown, ModeDeps, newTestClock(t))

	view := m.View()
	require.Contains(t, view, "No tree data")
}

// Golden tests for tree UI rendering
// Run with -update flag to update golden files: go test -update ./internal/ui/tree/...

// TestView_Golden_Basic tests the basic tree view rendering with multiple nodes.
func TestView_Golden_Basic(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))
	m.SetSize(100, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestView_Golden_UpDirection tests tree view with up direction.
func TestView_Golden_UpDirection(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("subtask-1", issueMap, DirectionUp, ModeDeps, newTestClock(t))
	m.SetSize(100, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestView_Golden_CursorMoved tests tree view with cursor on different node.
func TestView_Golden_CursorMoved(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("epic-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))
	m.SetSize(100, 30)

	// Move cursor to task-2 (index 2)
	m.MoveCursor(2)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestView_Golden_Empty tests tree view with no data.
func TestView_Golden_Empty(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("nonexistent", issueMap, DirectionDown, ModeDeps, newTestClock(t))
	m.SetSize(100, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestView_Golden_LeafNode tests tree view when root has no children.
func TestView_Golden_LeafNode(t *testing.T) {
	issueMap := makeTestIssueMap()
	m := New("task-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))
	m.SetSize(100, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestView_Golden_NarrowWidth tests tree view with narrow width and long title.
// At width 60, there's enough room for metadata.
func TestView_Golden_NarrowWidth(t *testing.T) {
	issueMap := map[string]*beads.Issue{
		"long-1": {
			ID:           "long-1",
			TitleText:    "This is a very long title that should definitely be truncated to fit",
			Status:       beads.StatusOpen,
			Type:         beads.TypeEpic,
			Priority:     beads.PriorityHigh,
			CreatedAt:    testCreatedAt,
			CommentCount: 3,
		},
	}
	m := New("long-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))
	m.SetSize(60, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestView_Golden_VeryNarrowWidth tests tree view with very narrow width.
// Metadata should be hidden when there's not enough room.
func TestView_Golden_VeryNarrowWidth(t *testing.T) {
	issueMap := map[string]*beads.Issue{
		"long-1": {
			ID:           "long-1",
			TitleText:    "This is a very long title that should definitely be truncated to fit",
			Status:       beads.StatusOpen,
			Type:         beads.TypeEpic,
			Priority:     beads.PriorityHigh,
			CreatedAt:    testCreatedAt,
			CommentCount: 3,
		},
	}
	m := New("long-1", issueMap, DirectionDown, ModeDeps, newTestClock(t))
	m.SetSize(40, 30) // Very narrow - metadata should be hidden

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
