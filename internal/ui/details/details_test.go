package details

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/beads"
	"github.com/zjrosen/perles/internal/mocks"
)

// stripANSI removes ANSI escape codes from a string for easier testing.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

func createTestModel(t *testing.T, issue beads.Issue) Model {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{}, nil).Maybe()

	mockClient := mocks.NewMockBeadsClient(t)
	mockClient.EXPECT().GetComments(mock.Anything).
		Return([]beads.Comment{}, nil)

	return New(issue, mockExecutor, mockClient)
}

func TestDetails_New(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityHigh,
		Status:    beads.StatusOpen,
	}
	m := createTestModel(t, issue)
	require.Equal(t, "test-1", m.issue.ID)
}

func TestDetails_SetSize(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
	}

	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)
	require.Equal(t, 100, m.width)
	require.Equal(t, 40, m.height)
	require.True(t, m.ready, "expected model to be ready after SetSize")
}

func TestDetails_View_NotReady(t *testing.T) {
	issue := beads.Issue{ID: "test-1", TitleText: "Test"}
	m := createTestModel(t, issue)
	// Without SetSize, ready is false
	view := m.View()
	require.Equal(t, "Loading...", view, "expected 'Loading...' when not ready")
}

func TestDetails_View_Ready(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityHigh,
		Status:    beads.StatusOpen,
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)
	view := m.View()

	require.Contains(t, view, "test-1", "expected view to contain issue ID")
	require.Contains(t, view, "Test Issue", "expected view to contain title")
}

func TestDetails_View_WithDescription(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: "This is a detailed description",
		CreatedAt:       time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)
	view := m.View()

	// Description is rendered with markdown styling (strip ANSI for checking)
	stripped := stripANSI(view)
	require.Contains(t, stripped, "detailed description", "expected view to contain description text")
}

func TestDetails_View_WithExtraFields(t *testing.T) {
	issue := beads.Issue{
		ID:                 "test-1",
		TitleText:          "Test Issue",
		DescriptionText:    "Description content",
		AcceptanceCriteria: "- Criteria 1\n- Criteria 2",
		Design:             "Design document link",
		Notes:              "Some notes",
		CreatedAt:          time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)
	view := m.View()
	stripped := stripANSI(view)

	// Check for headers and content
	require.Contains(t, stripped, "Acceptance Criteria")
	require.Contains(t, stripped, "Criteria 1")
	require.Contains(t, stripped, "Design")
	require.Contains(t, stripped, "Design document link")
	require.Contains(t, stripped, "Notes")
	require.Contains(t, stripped, "Some notes")
}

func TestDetails_View_WithNoExtraFields(t *testing.T) {
	issue := beads.Issue{
		ID:                 "test-1",
		TitleText:          "Test Issue",
		DescriptionText:    "Description content",
		AcceptanceCriteria: "",
		Design:             "",
		Notes:              "",
		CreatedAt:          time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)
	view := m.View()
	stripped := stripANSI(view)

	// Check for headers and content
	require.NotContains(t, stripped, "Acceptance Criteria")
	require.NotContains(t, stripped, "Criteria 1")
	require.NotContains(t, stripped, "Design")
	require.NotContains(t, stripped, "Design document link")
	require.NotContains(t, stripped, "Notes")
	require.NotContains(t, stripped, "Some notes")
}

func TestDetails_View_WithDependencies(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		BlockedBy: []string{"blocker-1", "blocker-2"},
		Blocks:    []string{"downstream-1"},
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(170, 40) // Width >= 115 for two-column layout with dependencies
	view := m.View()

	// Dependencies now render in right column with section headers (no colon)
	require.Contains(t, view, "Blocked by", "expected view to contain 'Blocked by' section")
	require.Contains(t, view, "Blocks", "expected view to contain 'Blocks' section")
	require.Contains(t, view, "blocker-1", "expected view to contain blocker ID")
}

func TestDetails_View_WithLabels(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Labels:    []string{"bug", "urgent"},
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)
	view := m.View()

	// Check for label values (displayed in right column in two-column layout)
	require.Contains(t, view, "bug", "expected view to contain label 'bug'")
	require.Contains(t, view, "urgent", "expected view to contain label 'urgent'")
}

func TestDetails_Update_ScrollDown(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: strings.Repeat("Long content line\n", 100),
		CreatedAt:       time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 20) // Small height to enable scrolling

	initialOffset := m.viewport.YOffset
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Greater(t, m.viewport.YOffset, initialOffset, "expected viewport to scroll down on 'j' key")
}

func TestDetails_Update_ScrollUp(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: strings.Repeat("Long content line\n", 100),
		CreatedAt:       time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 20)

	// Scroll down first
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	afterDown := m.viewport.YOffset

	// Then scroll up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Less(t, m.viewport.YOffset, afterDown, "expected viewport to scroll up on 'k' key")
}

func TestDetails_Update_GotoTop(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: strings.Repeat("Long content line\n", 100),
		CreatedAt:       time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 20)

	// Scroll down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Go to top
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	require.Equal(t, 0, m.viewport.YOffset, "expected viewport at top after 'g'")
}

func TestDetails_Update_GotoBottom(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: strings.Repeat("Long content line\n", 100),
		CreatedAt:       time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 20)

	// Go to bottom
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	// Should be at or near bottom
	require.NotEqual(t, 0, m.viewport.YOffset, "expected viewport to scroll to bottom on 'G'")
}

func TestDetails_SetSize_TwiceUpdatesViewport(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)
	m = m.SetSize(80, 30) // Resize

	require.Equal(t, 80, m.width, "expected width 80 after resize")
	require.Equal(t, 30, m.height, "expected height 30 after resize")
}

func TestDetails_View_AllTypes(t *testing.T) {
	types := []beads.IssueType{
		beads.TypeBug,
		beads.TypeFeature,
		beads.TypeTask,
		beads.TypeEpic,
		beads.TypeChore,
	}

	for _, issueType := range types {
		issue := beads.Issue{
			ID:        "test-1",
			TitleText: "Test",
			Type:      issueType,
			CreatedAt: time.Now(),
		}
		m := createTestModel(t, issue)
		m = m.SetSize(100, 40)
		view := m.View()
		require.NotEmpty(t, view, "expected non-empty view for type %s", issueType)
	}
}

func TestDetails_View_AllPriorities(t *testing.T) {
	priorities := []beads.Priority{
		beads.PriorityCritical,
		beads.PriorityHigh,
		beads.PriorityMedium,
		beads.PriorityLow,
		beads.PriorityBacklog,
	}

	for _, priority := range priorities {
		issue := beads.Issue{
			ID:        "test-1",
			TitleText: "Test",
			Priority:  priority,
			CreatedAt: time.Now(),
		}
		m := createTestModel(t, issue)
		m = m.SetSize(100, 40)
		view := m.View()
		require.NotEmpty(t, view, "expected non-empty view for priority %d", priority)
	}
}

func TestDetails_View_MarkdownDescription(t *testing.T) {
	// Test that markdown content is rendered (content preserved)
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: "# Heading\n\nThis is **bold** and *italic* text.\n\n- Item 1\n- Item 2",
		CreatedAt:       time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)
	view := m.View()

	// Content should be preserved in rendered output (strip ANSI for checking)
	stripped := stripANSI(view)
	require.Contains(t, stripped, "Heading", "expected view to contain 'Heading'")
	require.Contains(t, stripped, "bold", "expected view to contain 'bold'")
	require.Contains(t, stripped, "Item 1", "expected view to contain 'Item 1'")
}

func TestDetails_View_MarkdownCodeBlock(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: "```go\nfunc example() {\n    return\n}\n```",
		CreatedAt:       time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)
	view := m.View()

	// Code content should be preserved (strip ANSI for checking)
	stripped := stripANSI(view)
	require.Contains(t, stripped, "func", "expected view to contain 'func'")
	require.Contains(t, stripped, "example", "expected view to contain 'example'")
}

func TestDetails_RendererInitialization(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test",
		DescriptionText: "Some content",
		CreatedAt:       time.Now(),
	}
	m := createTestModel(t, issue)

	// Before SetSize, mdRenderer should be nil
	require.Nil(t, m.mdRenderer, "expected mdRenderer to be nil before SetSize")

	m = m.SetSize(100, 40)

	// After SetSize, mdRenderer should be initialized
	require.NotNil(t, m.mdRenderer, "expected mdRenderer to be initialized after SetSize")
}

func TestDetails_SingleColumnFallback(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityHigh,
		Status:    beads.StatusOpen,
		Labels:    []string{"test-label"},
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)

	// Width below minTwoColumnWidth (80) should use single-column
	m = m.SetSize(70, 40)
	view := m.View()

	// Single-column layout should have type indicator in title (column list style)
	require.Contains(t, view, "[T]", "expected single-column view to contain type indicator in title")
	require.Contains(t, view, "[P1]", "expected single-column view to contain priority indicator in title")
}

func TestDetails_TwoColumnLayout(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityHigh,
		Status:    beads.StatusOpen,
		Labels:    []string{"test-label"},
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)

	// Width at or above minTwoColumnWidth (115) should use two-column
	m = m.SetSize(170, 40)
	view := m.View()

	// Two-column layout should NOT have inline metadata in header
	// Instead, metadata appears in right column without colons
	hasInlineMetadata := strings.Contains(view, "Type:") && strings.Contains(view, "Priority:") && strings.Contains(view, "Status:")
	require.False(t, hasInlineMetadata, "expected two-column view to NOT have inline metadata in header")

	// Right column should show metadata values
	require.Contains(t, view, "Priority", "expected two-column view to contain Priority label")
}

func TestDetails_EmptyDescription(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)
	view := m.View()

	// Should render without errors
	require.Contains(t, view, "test-1", "expected view to contain issue ID")
}

func TestDetails_NoLabels(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Labels:    []string{},
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)
	view := m.View()

	// Should render without errors
	require.Contains(t, view, "test-1", "expected view to contain issue ID")
}

func TestDetails_ManyLabels(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Labels:    []string{"label1", "label2", "label3", "label4", "label5"},
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)
	view := m.View()

	// All labels should be visible
	for _, label := range issue.Labels {
		require.Contains(t, view, label, "expected view to contain label '%s'", label)
	}
}

func TestDetails_LongDependencyList(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		BlockedBy: []string{"dep-1", "dep-2", "dep-3", "dep-4", "dep-5"},
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(170, 40) // Width >= 115 for two-column layout with dependencies
	view := m.View()

	// All dependencies should be visible
	for _, dep := range issue.BlockedBy {
		require.Contains(t, view, dep, "expected view to contain dependency '%s'", dep)
	}
}

func TestDetails_TerminalResize(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		Type:            beads.TypeTask,
		Priority:        beads.PriorityHigh,
		DescriptionText: "Some description content",
		Labels:          []string{"test"},
		CreatedAt:       time.Now(),
	}
	m := createTestModel(t, issue)

	// Start with wide terminal (two-column)
	m = m.SetSize(120, 40)
	wideView := m.View()

	// Resize to narrow terminal (single-column)
	m = m.SetSize(60, 40)
	narrowView := m.View()

	// Both views should render without errors
	require.Contains(t, wideView, "test-1", "expected wide view to contain issue ID")
	require.Contains(t, narrowView, "test-1", "expected narrow view to contain issue ID")

	// Narrow view should have type indicator in title (single-column uses column list style)
	require.Contains(t, narrowView, "[T]", "expected narrow view to contain type indicator")
}

// Tests for scrolling behavior

func TestDetails_JKScrollsViewport(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: strings.Repeat("Long content line\n", 100),
		CreatedAt:       time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 20)

	initialOffset := m.viewport.YOffset
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Greater(t, m.viewport.YOffset, initialOffset, "expected viewport to scroll down on 'j'")

	// Scroll back up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, initialOffset, m.viewport.YOffset, "expected viewport to scroll back up on 'k'")
}

func TestDetails_DependencyNavigation_LToFocusDeps(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		BlockedBy: []string{"dep-1", "dep-2", "dep-3"},
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)

	// Initially on content pane
	require.Equal(t, FocusContent, m.focusPane, "expected content pane initially")

	// Press 'l' to focus dependencies
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, FocusMetadata, m.focusPane, "expected metadata pane after 'l'")
	require.Equal(t, 0, m.selectedDependency, "expected first dependency selected")

	// Press 'j' to navigate to next dependency
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.selectedDependency, "expected second dependency")

	// Press 'k' to go back
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 0, m.selectedDependency, "expected first dependency")

	// Wrap around with 'k'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	require.Equal(t, 2, m.selectedDependency, "expected wrap to last dependency")

	// Press 'h' to return to content
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	require.Equal(t, FocusContent, m.focusPane, "expected content pane after 'h'")
}

func TestDetails_DependencyNavigation_EnterNavigates(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		BlockedBy: []string{"target-dep"},
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)

	// Focus dependencies with 'l'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, FocusMetadata, m.focusPane)
	require.Equal(t, 0, m.selectedDependency)

	// Press Enter
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd, "expected command from Enter on dependency")

	msg := cmd()
	navMsg, ok := msg.(NavigateToDependencyMsg)
	require.True(t, ok, "expected NavigateToDependencyMsg")
	require.Equal(t, "target-dep", navMsg.IssueID)
}

func TestDetails_DependencyNavigation_EnterNoOpOnContentPane(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		BlockedBy: []string{"dep-1"},
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)

	// Stay on content pane (don't press 'l')
	require.Equal(t, FocusContent, m.focusPane)

	// Press Enter - should return nil command
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.Nil(t, cmd, "expected nil command when on content pane")
}

func TestDetails_DependencyNavigation_LNoOpWithoutDeps(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		// No dependencies
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)

	// Press 'l' - should stay on content (no deps to focus)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, FocusContent, m.focusPane, "expected to stay on content when no deps")
}

// TestDetails_DependencyNavigation_MixedCategories_CorrectOrder tests that
// selectedDependency index correctly maps to the visually displayed dependency.
// This is a regression test for a bug where the dependencies slice order
// (children→blocked_by→blocks→related) didn't match the render order
// (blocked_by→blocks→children→related), causing Enter to navigate to wrong issue.
func TestDetails_DependencyNavigation_MixedCategories_CorrectOrder(t *testing.T) {
	// Create issue with multiple dependency categories
	// The render order is: blocked_by, blocks, children, related
	// So visually: blocker-1, blocker-2, child-1, child-2, child-3
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Children:  []string{"child-1", "child-2", "child-3"},
		BlockedBy: []string{"blocker-1", "blocker-2"},
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)

	// Focus dependencies
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	require.Equal(t, FocusMetadata, m.focusPane)

	// Index 0 should be first blocked_by (blocker-1), NOT first child
	require.Equal(t, 0, m.selectedDependency)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd()
	navMsg, ok := msg.(NavigateToDependencyMsg)
	require.True(t, ok, "expected NavigateToDependencyMsg")
	require.Equal(t, "blocker-1", navMsg.IssueID, "index 0 should be first blocked_by")

	// Navigate to index 1 (second blocked_by)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 1, m.selectedDependency)
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg = cmd()
	navMsg, ok = msg.(NavigateToDependencyMsg)
	require.True(t, ok)
	require.Equal(t, "blocker-2", navMsg.IssueID, "index 1 should be second blocked_by")

	// Navigate to index 2 (first child - after all blocked_by)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 2, m.selectedDependency)
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg = cmd()
	navMsg, ok = msg.(NavigateToDependencyMsg)
	require.True(t, ok)
	require.Equal(t, "child-1", navMsg.IssueID, "index 2 should be first child")

	// Navigate to index 4 (third child)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	require.Equal(t, 4, m.selectedDependency)
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg = cmd()
	navMsg, ok = msg.(NavigateToDependencyMsg)
	require.True(t, ok)
	require.Equal(t, "child-3", navMsg.IssueID, "index 4 should be third child")
}

func TestDetails_DeleteKey_EmitsDeleteIssueMsg(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Type:      beads.TypeTask,
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)

	// Press 'ctrl+d' to request deletion
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	// Should return a command that produces DeleteIssueMsg
	require.NotNil(t, cmd, "expected command from 'ctrl+d' key")
	msg := cmd()
	deleteMsg, ok := msg.(DeleteIssueMsg)
	require.True(t, ok, "expected DeleteIssueMsg")
	require.Equal(t, "test-1", deleteMsg.IssueID)
	require.Equal(t, beads.TypeTask, deleteMsg.IssueType)
}

func TestDetails_DeleteKey_EpicType(t *testing.T) {
	issue := beads.Issue{
		ID:        "epic-1",
		TitleText: "Epic Issue",
		Type:      beads.TypeEpic,
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)

	// Press 'ctrl+d' to request deletion
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})

	msg := cmd()
	deleteMsg, ok := msg.(DeleteIssueMsg)
	require.True(t, ok, "expected DeleteIssueMsg")
	require.Equal(t, "epic-1", deleteMsg.IssueID)
	require.Equal(t, beads.TypeEpic, deleteMsg.IssueType, "expected epic type for cascade handling")
}

func TestDetails_FooterShowsDeleteKeybinding(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 40)
	view := m.View()

	require.Contains(t, view, "[ctrl+d] Delete Issue", "expected footer to show delete keybinding")
}

// TestDetails_View_Golden uses teatest golden file comparison.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-123",
		TitleText:       "Test Issue Title",
		DescriptionText: "This is the issue description.\n\nIt has multiple paragraphs.",
		Type:            beads.TypeTask,
		Priority:        beads.PriorityHigh,
		Status:          beads.StatusOpen,
		Labels:          []string{"backend", "urgent"},
		CreatedAt:       time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2024, 1, 20, 14, 45, 0, 0, time.UTC),
	}
	m := createTestModel(t, issue).SetSize(120, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestDetails_View_Golden_WithDependencies tests rendering with blocked_by and blocks.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden_WithDependencies(t *testing.T) {
	issue := beads.Issue{
		ID:              "epic-456",
		TitleText:       "Epic with Dependencies",
		DescriptionText: "This epic has both blockers and downstream dependencies.",
		Type:            beads.TypeEpic,
		Priority:        beads.PriorityCritical,
		Status:          beads.StatusInProgress,
		Labels:          []string{"ui", "phase-2"},
		BlockedBy:       []string{"task-100", "task-101"},
		Blocks:          []string{"task-200", "task-201", "task-202"},
		CreatedAt:       time.Date(2024, 2, 1, 9, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2024, 2, 15, 16, 30, 0, 0, time.UTC),
	}
	m := createTestModel(t, issue).SetSize(120, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestDetails_View_Golden_WithLoadedDependencies tests rendering with fully loaded dependency data.
// Uses MockBQLExecutor to provide full issue details for dependencies.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden_WithLoadedDependencies(t *testing.T) {
	mockExecutor := mocks.NewMockBQLExecutor(t)
	mockExecutor.EXPECT().Execute(mock.Anything).Return([]beads.Issue{
		{
			ID:        "bug-101",
			TitleText: "Critical login failure",
			Type:      beads.TypeBug,
			Priority:  beads.PriorityCritical,
			Status:    beads.StatusOpen,
		},
		{
			ID:        "task-201",
			TitleText: "Update documentation",
			Type:      beads.TypeTask,
			Priority:  beads.PriorityLow,
			Status:    beads.StatusOpen,
		},
		{
			ID:        "feature-301",
			TitleText: "New dashboard widget",
			Type:      beads.TypeFeature,
			Priority:  beads.PriorityHigh,
			Status:    beads.StatusInProgress,
		},
	}, nil)

	mockClient := mocks.NewMockBeadsClient(t)
	mockClient.EXPECT().GetComments(mock.Anything).
		Return([]beads.Comment{}, nil)

	issue := beads.Issue{
		ID:              "task-500",
		TitleText:       "Integration task",
		DescriptionText: "This task shows fully loaded dependency rendering.",
		Type:            beads.TypeTask,
		Priority:        beads.PriorityMedium,
		Status:          beads.StatusOpen,
		BlockedBy:       []string{"bug-101"},
		Blocks:          []string{"task-201", "feature-301"},
		Children:        []string{"task-201", "feature-301"},
		CreatedAt:       time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC),
	}
	m := New(issue, mockExecutor, mockClient).SetSize(120, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestDetails_View_Golden_Wide tests wide two-column layout with footer visible.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden_Wide(t *testing.T) {
	issue := beads.Issue{
		ID:              "wide-test",
		TitleText:       "Wide Layout Test Issue",
		DescriptionText: "Testing that footer is visible in wide two-column layout.",
		Type:            beads.TypeFeature,
		Priority:        beads.PriorityMedium,
		Status:          beads.StatusOpen,
		Labels:          []string{"test"},
		CreatedAt:       time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
	}
	m := createTestModel(t, issue).SetSize(180, 25)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestDetails_View_Golden_WrappingTitle tests that divider is continuous when title wraps.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden_WrappingTitle(t *testing.T) {
	issue := beads.Issue{
		ID:              "wrap-test",
		TitleText:       "This is a very long title that should wrap to multiple lines in the two-column layout to test divider continuity",
		DescriptionText: "Short description.",
		Type:            beads.TypeBug,
		Priority:        beads.PriorityHigh,
		Status:          beads.StatusInProgress,
		CreatedAt:       time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC),
	}
	m := createTestModel(t, issue).SetSize(180, 25)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestDetails_View_Golden_WithAssignee tests rendering with assignee field populated.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden_WithAssignee(t *testing.T) {
	issue := beads.Issue{
		ID:              "assigned-task",
		TitleText:       "Task with Assignee",
		DescriptionText: "This task is assigned to a specific user.",
		Type:            beads.TypeTask,
		Priority:        beads.PriorityHigh,
		Status:          beads.StatusInProgress,
		Assignee:        "coding-agent",
		Labels:          []string{"wip"},
		CreatedAt:       time.Date(2024, 4, 1, 9, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2024, 4, 1, 10, 30, 0, 0, time.UTC),
	}
	m := createTestModel(t, issue).SetSize(120, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestDetails_View_Golden_WithComments tests rendering with comments loaded.
// Uses MockCommentLoader to provide comments for the issue.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden_WithComments(t *testing.T) {
	commentLoader := mocks.NewMockBeadsClient(t)
	commentLoader.EXPECT().GetComments("commented-task").Return([]beads.Comment{
		{
			ID:        1,
			Author:    "alice",
			Text:      "First comment on this task.",
			CreatedAt: time.Date(2024, 4, 2, 14, 30, 0, 0, time.UTC),
		},
		{
			ID:        2,
			Author:    "bob",
			Text:      "Second comment with some feedback.",
			CreatedAt: time.Date(2024, 4, 2, 15, 45, 0, 0, time.UTC),
		},
	}, nil)

	issue := beads.Issue{
		ID:              "commented-task",
		TitleText:       "Task with Comments",
		DescriptionText: "This task has comments below the description.",
		Type:            beads.TypeTask,
		Priority:        beads.PriorityMedium,
		Status:          beads.StatusOpen,
		CreatedAt:       time.Date(2024, 4, 1, 9, 0, 0, 0, time.UTC),
	}
	m := New(issue, nil, commentLoader).SetSize(120, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestDetails_View_Golden_WithAssigneeAndComments tests rendering with both assignee and comments.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden_WithAssigneeAndComments(t *testing.T) {
	commentLoader := mocks.NewMockBeadsClient(t)
	commentLoader.EXPECT().GetComments("full-task").Return([]beads.Comment{
		{
			ID:        1,
			Author:    "code-reviewer",
			Text:      "APPROVED: Implementation looks good.",
			CreatedAt: time.Date(2024, 4, 3, 16, 0, 0, 0, time.UTC),
		},
	}, nil)

	issue := beads.Issue{
		ID:              "full-task",
		TitleText:       "Complete Task with All Fields",
		DescriptionText: "This task demonstrates all metadata: assignee, comments, labels, and timestamps.",
		Type:            beads.TypeTask,
		Priority:        beads.PriorityCritical,
		Status:          beads.StatusClosed,
		Assignee:        "coding-agent",
		Labels:          []string{"reviewed", "approved"},
		CreatedAt:       time.Date(2024, 4, 1, 9, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2024, 4, 3, 17, 0, 0, 0, time.UTC),
	}
	m := New(issue, nil, commentLoader).SetSize(120, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestDetails_View_Golden_WithLongComment tests that long comments wrap correctly.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden_WithLongComment(t *testing.T) {
	commentLoader := mocks.NewMockBeadsClient(t)
	commentLoader.EXPECT().GetComments("long-comment-task").Return([]beads.Comment{
		{
			ID:        1,
			Author:    "reviewer",
			Text:      "This is a very long comment that should wrap to multiple lines within the content column. It contains enough text to demonstrate that the word wrapping is working correctly and that long comments don't overflow past the column boundary.",
			CreatedAt: time.Date(2024, 4, 5, 10, 0, 0, 0, time.UTC),
		},
	}, nil)

	issue := beads.Issue{
		ID:              "long-comment-task",
		TitleText:       "Task with Long Comment",
		DescriptionText: "Testing comment text wrapping.",
		Type:            beads.TypeTask,
		Priority:        beads.PriorityMedium,
		Status:          beads.StatusOpen,
		CreatedAt:       time.Date(2024, 4, 5, 9, 0, 0, 0, time.UTC),
	}
	m := New(issue, nil, commentLoader).SetSize(120, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestDetails_View_Golden_WithCommentsError tests that error message is shown when comments fail to load.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden_WithCommentsError(t *testing.T) {
	commentLoader := mocks.NewMockBeadsClient(t)
	commentLoader.EXPECT().GetComments("error-task").Return(nil, errors.New("database connection failed"))

	issue := beads.Issue{
		ID:              "error-task",
		TitleText:       "Task with Comments Error",
		DescriptionText: "This task should show an error message for comments.",
		Type:            beads.TypeTask,
		Priority:        beads.PriorityMedium,
		Status:          beads.StatusOpen,
		CreatedAt:       time.Date(2024, 4, 10, 9, 0, 0, 0, time.UTC),
	}
	m := New(issue, nil, commentLoader).SetSize(120, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestDetails_View_Golden_WithExtraFields tests rendering with acceptance criteria, design, and notes fields.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden_WithExtraFields(t *testing.T) {
	issue := beads.Issue{
		ID:                 "extra-fields-task",
		TitleText:          "Task with Extra Fields",
		DescriptionText:    "This task demonstrates the extra fields: acceptance criteria, design, and notes.",
		Type:               beads.TypeFeature,
		Priority:           beads.PriorityHigh,
		Status:             beads.StatusOpen,
		AcceptanceCriteria: "- [ ] Users can log in with email/password\n- [ ] Users can reset password via email\n- [ ] Session expires after 24 hours",
		Design:             "See design doc at: https://example.com/design/auth-flow",
		Notes:              "This feature requires coordination with the backend team.\nTarget release: v2.0",
		Labels:             []string{"feature", "auth"},
		CreatedAt:          time.Date(2024, 5, 1, 9, 0, 0, 0, time.UTC),
		UpdatedAt:          time.Date(2024, 5, 2, 14, 30, 0, 0, time.UTC),
	}
	m := createTestModel(t, issue).SetSize(120, 40)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestDetails_View_Golden_NoExtraFields tests that section headers don't render when extra fields are empty.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden_NoExtraFields(t *testing.T) {
	issue := beads.Issue{
		ID:              "no-extra-task",
		TitleText:       "Task without Extra Fields",
		DescriptionText: "This task has no acceptance criteria, design, or notes fields.",
		Type:            beads.TypeTask,
		Priority:        beads.PriorityMedium,
		Status:          beads.StatusOpen,
		// AcceptanceCriteria, Design, Notes are all empty
		Labels:    []string{"simple"},
		CreatedAt: time.Date(2024, 5, 10, 9, 0, 0, 0, time.UTC),
	}
	m := createTestModel(t, issue).SetSize(120, 40)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestDetails_View_Golden_WithClosedAt tests rendering with Closed timestamp and Duration.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden_WithClosedAt(t *testing.T) {
	issue := beads.Issue{
		ID:              "closed-task",
		TitleText:       "Completed Task with Duration",
		DescriptionText: "This closed task shows the Closed timestamp and Duration fields.",
		Type:            beads.TypeTask,
		Priority:        beads.PriorityHigh,
		Status:          beads.StatusClosed,
		Labels:          []string{"done"},
		CreatedAt:       time.Date(2024, 6, 1, 9, 0, 0, 0, time.UTC),
		UpdatedAt:       time.Date(2024, 6, 3, 14, 30, 0, 0, time.UTC),
		ClosedAt:        time.Date(2024, 6, 3, 14, 30, 0, 0, time.UTC),
	}
	m := createTestModel(t, issue).SetSize(120, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestFormatDuration tests the formatDuration helper function.
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{
			name:     "zero duration",
			duration: 0,
			want:     "0m",
		},
		{
			name:     "minutes only",
			duration: 15 * time.Minute,
			want:     "15m",
		},
		{
			name:     "one hour exactly",
			duration: time.Hour,
			want:     "1h 0m",
		},
		{
			name:     "hours and minutes",
			duration: 2*time.Hour + 30*time.Minute,
			want:     "2h 30m",
		},
		{
			name:     "one day exactly",
			duration: 24 * time.Hour,
			want:     "1d 0h",
		},
		{
			name:     "days and hours",
			duration: 3*24*time.Hour + 4*time.Hour,
			want:     "3d 4h",
		},
		{
			name:     "complex duration",
			duration: 5*24*time.Hour + 12*time.Hour + 30*time.Minute,
			want:     "5d 12h",
		},
		{
			name:     "less than a minute rounds to 0m",
			duration: 30 * time.Second,
			want:     "0m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.duration)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestDetails_YOffset_GetterSetter(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: strings.Repeat("Long content line\n", 100), // Lots of content to enable scrolling
		CreatedAt:       time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 20) // Small height to enable scrolling (matching existing test pattern)

	// Initial offset is 0
	require.Equal(t, 0, m.YOffset(), "initial YOffset should be 0")

	// Scroll down using key (like TestDetails_Update_ScrollDown)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	offset1 := m.viewport.YOffset // Direct access like existing tests
	require.Greater(t, offset1, 0, "should have scrolled down")

	// Verify YOffset() getter returns same value
	require.Equal(t, offset1, m.YOffset(), "YOffset() should return viewport.YOffset")

	// Test SetYOffset to restore position to top
	m = m.SetYOffset(0)
	require.Equal(t, 0, m.YOffset(), "YOffset should be 0 after SetYOffset(0)")

	// Restore to previous offset
	m = m.SetYOffset(offset1)
	require.Equal(t, offset1, m.YOffset(), "YOffset should be restored to previous value")
}

func TestDetails_SetSize_PreservesScroll(t *testing.T) {
	// Create issue with enough content to scroll
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: strings.Repeat("Long content line\n", 100),
		CreatedAt:       time.Now(),
	}
	m := createTestModel(t, issue)
	m = m.SetSize(100, 20) // Small height to enable scrolling

	// Scroll down using key presses (like TestDetails_Update_ScrollDown)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	scrolledOffset := m.viewport.YOffset
	require.Greater(t, scrolledOffset, 0, "should have scrolled down")

	// Resize - scroll should be preserved (no GotoTop in else branch)
	m = m.SetSize(120, 25)
	require.Equal(t, scrolledOffset, m.YOffset(), "scroll should be preserved on resize")
}

func TestDetails_SetSize_InitializedAtTop(t *testing.T) {
	// Create issue with long content
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: strings.Repeat("Line\n", 50),
		CreatedAt:       time.Now(),
	}

	// New model starts at top on first SetSize (initialization)
	m := createTestModel(t, issue).SetSize(80, 20)
	require.Equal(t, 0, m.YOffset(), "new model should start at top")
}

// TestDetails_View_Golden_LongLabels tests that long labels wrap within the metadata column bounds.
// This is a regression test for labels overflowing into the left content column.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden_LongLabels(t *testing.T) {
	issue := beads.Issue{
		ID:              "sesh-r3z",
		TitleText:       "Spec: Embed Children in Session List",
		DescriptionText: "## Overview\n\nAdd `--include-children` flag to `session:list`.",
		Type:            beads.TypeFeature,
		Priority:        beads.PriorityMedium,
		Status:          beads.StatusOpen,
		// Labels longer than metadataColWidth (34) minus indent (2) = 32 chars available
		// "spec:018-embed-children-list" is 28 chars - fits
		// "very-long-label-that-exceeds-column-bounds" is 42 chars - should wrap
		Labels:    []string{"spec:018-embed-children-list", "very-long-label-that-exceeds-column-bounds"},
		CreatedAt: time.Date(2025, 12, 17, 16, 24, 59, 0, time.UTC),
	}
	m := createTestModel(t, issue).SetSize(120, 30)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}
