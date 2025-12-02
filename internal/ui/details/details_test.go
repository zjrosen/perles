package details

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"

	"perles/internal/beads"

	tea "github.com/charmbracelet/bubbletea"
)

// mockLoader implements DependencyLoader for testing with predefined issues.
type mockLoader struct {
	issues map[string]beads.Issue
}

func (m *mockLoader) ListIssuesByIds(ids []string) ([]beads.Issue, error) {
	var result []beads.Issue
	for _, id := range ids {
		if issue, ok := m.issues[id]; ok {
			result = append(result, issue)
		}
	}
	return result, nil
}

func TestDetails_New(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityHigh,
		Status:    beads.StatusOpen,
	}
	m := New(issue, nil)
	assert.Equal(t, "test-1", m.issue.ID)
}

func TestDetails_SetSize(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)
	assert.Equal(t, 100, m.width)
	assert.Equal(t, 40, m.height)
	assert.True(t, m.ready, "expected model to be ready after SetSize")
}

func TestDetails_View_NotReady(t *testing.T) {
	issue := beads.Issue{ID: "test-1", TitleText: "Test"}
	m := New(issue, nil)
	// Without SetSize, ready is false
	view := m.View()
	assert.Equal(t, "Loading...", view, "expected 'Loading...' when not ready")
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
	m := New(issue, nil)
	m = m.SetSize(100, 40)
	view := m.View()

	assert.Contains(t, view, "test-1", "expected view to contain issue ID")
	assert.Contains(t, view, "Test Issue", "expected view to contain title")
}

func TestDetails_View_WithDescription(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: "This is a detailed description",
		CreatedAt:       time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)
	view := m.View()

	// Description is rendered with markdown styling (no "Description:" header)
	assert.Contains(t, view, "detailed description", "expected view to contain description text")
}

func TestDetails_View_WithDependencies(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		BlockedBy: []string{"blocker-1", "blocker-2"},
		Blocks:    []string{"downstream-1"},
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)
	view := m.View()

	// Dependencies now render in right column with section headers (no colon)
	assert.Contains(t, view, "Blocked by", "expected view to contain 'Blocked by' section")
	assert.Contains(t, view, "Blocks", "expected view to contain 'Blocks' section")
	assert.Contains(t, view, "blocker-1", "expected view to contain blocker ID")
}

func TestDetails_View_WithLabels(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Labels:    []string{"bug", "urgent"},
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)
	view := m.View()

	// Check for label values (displayed in right column in two-column layout)
	assert.Contains(t, view, "bug", "expected view to contain label 'bug'")
	assert.Contains(t, view, "urgent", "expected view to contain label 'urgent'")
}

func TestDetails_Update_ScrollDown(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: strings.Repeat("Long content line\n", 100),
		CreatedAt:       time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 20) // Small height to enable scrolling

	initialOffset := m.viewport.YOffset
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Greater(t, m.viewport.YOffset, initialOffset, "expected viewport to scroll down on 'j' key")
}

func TestDetails_Update_ScrollUp(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: strings.Repeat("Long content line\n", 100),
		CreatedAt:       time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 20)

	// Scroll down first
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	afterDown := m.viewport.YOffset

	// Then scroll up
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Less(t, m.viewport.YOffset, afterDown, "expected viewport to scroll up on 'k' key")
}

func TestDetails_Update_GotoTop(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: strings.Repeat("Long content line\n", 100),
		CreatedAt:       time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 20)

	// Scroll down
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Go to top
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	assert.Equal(t, 0, m.viewport.YOffset, "expected viewport at top after 'g'")
}

func TestDetails_Update_GotoBottom(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: strings.Repeat("Long content line\n", 100),
		CreatedAt:       time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 20)

	// Go to bottom
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	// Should be at or near bottom
	assert.NotEqual(t, 0, m.viewport.YOffset, "expected viewport to scroll to bottom on 'G'")
}

func TestDetails_SetSize_TwiceUpdatesViewport(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)
	m = m.SetSize(80, 30) // Resize

	assert.Equal(t, 80, m.width, "expected width 80 after resize")
	assert.Equal(t, 30, m.height, "expected height 30 after resize")
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
		m := New(issue, nil)
		m = m.SetSize(100, 40)
		view := m.View()
		assert.NotEmpty(t, view, "expected non-empty view for type %s", issueType)
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
		m := New(issue, nil)
		m = m.SetSize(100, 40)
		view := m.View()
		assert.NotEmpty(t, view, "expected non-empty view for priority %d", priority)
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
	m := New(issue, nil)
	m = m.SetSize(100, 40)
	view := m.View()

	// Content should be preserved in rendered output
	assert.Contains(t, view, "Heading", "expected view to contain 'Heading'")
	assert.Contains(t, view, "bold", "expected view to contain 'bold'")
	assert.Contains(t, view, "Item 1", "expected view to contain 'Item 1'")
}

func TestDetails_View_MarkdownCodeBlock(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: "```go\nfunc example() {\n    return\n}\n```",
		CreatedAt:       time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)
	view := m.View()

	// Code content should be preserved
	assert.Contains(t, view, "func", "expected view to contain 'func'")
	assert.Contains(t, view, "example", "expected view to contain 'example'")
}

func TestDetails_RendererInitialization(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test",
		DescriptionText: "Some content",
		CreatedAt:       time.Now(),
	}
	m := New(issue, nil)

	// Before SetSize, mdRenderer should be nil
	assert.Nil(t, m.mdRenderer, "expected mdRenderer to be nil before SetSize")

	m = m.SetSize(100, 40)

	// After SetSize, mdRenderer should be initialized
	assert.NotNil(t, m.mdRenderer, "expected mdRenderer to be initialized after SetSize")
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
	m := New(issue, nil)

	// Width below minTwoColumnWidth (80) should use single-column
	m = m.SetSize(70, 40)
	view := m.View()

	// Single-column layout should have type indicator in title (column list style)
	assert.Contains(t, view, "[T]", "expected single-column view to contain type indicator in title")
	assert.Contains(t, view, "[P1]", "expected single-column view to contain priority indicator in title")
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
	m := New(issue, nil)

	// Width at or above minTwoColumnWidth (80) should use two-column
	m = m.SetSize(100, 40)
	view := m.View()

	// Two-column layout should NOT have inline metadata in header
	// Instead, metadata appears in right column without colons
	hasInlineMetadata := strings.Contains(view, "Type:") && strings.Contains(view, "Priority:") && strings.Contains(view, "Status:")
	assert.False(t, hasInlineMetadata, "expected two-column view to NOT have inline metadata in header")

	// Right column should show metadata values
	assert.Contains(t, view, "Priority", "expected two-column view to contain Priority label")
}

func TestDetails_EmptyDescription(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)
	view := m.View()

	// Should render without errors
	assert.Contains(t, view, "test-1", "expected view to contain issue ID")
}

func TestDetails_NoLabels(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Labels:    []string{},
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)
	view := m.View()

	// Should render without errors
	assert.Contains(t, view, "test-1", "expected view to contain issue ID")
}

func TestDetails_ManyLabels(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Labels:    []string{"label1", "label2", "label3", "label4", "label5"},
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)
	view := m.View()

	// All labels should be visible
	for _, label := range issue.Labels {
		assert.Contains(t, view, label, "expected view to contain label '%s'", label)
	}
}

func TestDetails_LongDependencyList(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		BlockedBy: []string{"dep-1", "dep-2", "dep-3", "dep-4", "dep-5"},
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)
	view := m.View()

	// All dependencies should be visible
	for _, dep := range issue.BlockedBy {
		assert.Contains(t, view, dep, "expected view to contain dependency '%s'", dep)
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
	m := New(issue, nil)

	// Start with wide terminal (two-column)
	m = m.SetSize(120, 40)
	wideView := m.View()

	// Resize to narrow terminal (single-column)
	m = m.SetSize(60, 40)
	narrowView := m.View()

	// Both views should render without errors
	assert.Contains(t, wideView, "test-1", "expected wide view to contain issue ID")
	assert.Contains(t, narrowView, "test-1", "expected narrow view to contain issue ID")

	// Narrow view should have type indicator in title (single-column uses column list style)
	assert.Contains(t, narrowView, "[T]", "expected narrow view to contain type indicator")
}

// Tests for inline metadata editing

func TestDetails_FocusPane_InitiallyContent(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	assert.Equal(t, FocusContent, m.focusPane, "expected initial focus on content pane")
}

func TestDetails_FocusPane_SwitchToMetadata(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Press 'l' to switch to metadata pane
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, FocusMetadata, m.focusPane, "expected focus on metadata pane after 'l'")
}

func TestDetails_FocusPane_SwitchBackToContent(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Switch to metadata
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, FocusMetadata, m.focusPane)

	// Press 'h' to switch back to content pane
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	assert.Equal(t, FocusContent, m.focusPane, "expected focus on content pane after 'h'")
}

func TestDetails_FocusPane_LDoesNothingOnMetadata(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Switch to metadata
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	// Press 'l' again - should stay on metadata (no further pane to the right)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, FocusMetadata, m.focusPane, "expected to stay on metadata pane")
}

func TestDetails_FocusPane_HDoesNothingOnContent(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Press 'h' while on content - should stay on content (no further pane to the left)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	assert.Equal(t, FocusContent, m.focusPane, "expected to stay on content pane")
}

func TestDetails_FieldNavigation_InitiallyPriority(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	assert.Equal(t, FieldPriority, m.selectedField, "expected initial selection on Priority field")
}

func TestDetails_FieldNavigation_Down(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Switch to metadata pane
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, FieldPriority, m.selectedField, "expected initial selection on Priority")

	// Press 'j' to move down to Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, FieldStatus, m.selectedField, "expected selection on Status field after 'j'")
}

func TestDetails_FieldNavigation_Up(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Switch to metadata pane
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	// Move down to Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, FieldStatus, m.selectedField, "expected Status field")

	// Press 'k' to move up to Priority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, FieldPriority, m.selectedField, "expected selection on Priority field after 'k'")
}

func TestDetails_FieldNavigation_WrapAroundDown(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Switch to metadata pane
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	// Move down 2 times (Priority -> Status -> Priority)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, FieldPriority, m.selectedField, "expected wrap-around to Priority field")
}

func TestDetails_FieldNavigation_WrapAroundUp(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Switch to metadata pane
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	// Press 'k' while on Priority (should wrap to Status)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, FieldStatus, m.selectedField, "expected wrap-around to Status field")
}

func TestDetails_FieldNavigation_JKScrollsViewportWhenContentFocused(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: strings.Repeat("Long content line\n", 100),
		CreatedAt:       time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 20)

	// Ensure we're on content pane (default)
	assert.Equal(t, FocusContent, m.focusPane)

	initialOffset := m.viewport.YOffset
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Greater(t, m.viewport.YOffset, initialOffset, "expected viewport to scroll when content focused")
}

func TestDetails_FieldNavigation_JKDoesNotScrollWhenMetadataFocused(t *testing.T) {
	issue := beads.Issue{
		ID:              "test-1",
		TitleText:       "Test Issue",
		DescriptionText: strings.Repeat("Long content line\n", 100),
		CreatedAt:       time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 20)

	// Switch to metadata pane
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	initialOffset := m.viewport.YOffset
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, initialOffset, m.viewport.YOffset, "expected viewport NOT to scroll when metadata focused")
}

func TestDetails_OpenPicker_Priority(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Priority:  beads.PriorityHigh, // P1
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Switch to metadata pane
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	// Press Enter on Priority field
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should return a command that produces OpenPriorityPickerMsg
	msg := cmd()
	priorityMsg, ok := msg.(OpenPriorityPickerMsg)
	assert.True(t, ok, "expected OpenPriorityPickerMsg")
	assert.Equal(t, "test-1", priorityMsg.IssueID)
	assert.Equal(t, beads.PriorityHigh, priorityMsg.Current, "expected current priority P1")
}

func TestDetails_OpenPicker_Status(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Status:    beads.StatusOpen,
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Switch to metadata pane and move to Status field (one j from Priority)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})

	// Press Enter on Status field
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	msg := cmd()
	statusMsg, ok := msg.(OpenStatusPickerMsg)
	assert.True(t, ok, "expected OpenStatusPickerMsg")
	assert.Equal(t, "test-1", statusMsg.IssueID)
	assert.Equal(t, beads.StatusOpen, statusMsg.Current, "expected current status open")
}

func TestDetails_EnterDoesNothingOnContentPane(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Ensure on content pane
	assert.Equal(t, FocusContent, m.focusPane)

	// Press Enter - should return nil command
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Nil(t, cmd, "expected nil command when Enter pressed on content pane")
}

func TestDetails_MetadataColumnShowsSelectionIndicator(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Type:      beads.TypeTask,
		Priority:  beads.PriorityHigh,
		Status:    beads.StatusOpen,
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// When content focused, no selection indicator
	view := m.View()
	assert.NotContains(t, view, "> Priority", "expected no selection indicator when content focused")

	// Switch to metadata pane
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	view = m.View()
	assert.Contains(t, view, ">", "expected selection indicator when metadata focused")
}

func TestDetails_DeleteKey_EmitsDeleteIssueMsg(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Type:      beads.TypeTask,
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Press 'd' to request deletion
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	// Should return a command that produces DeleteIssueMsg
	assert.NotNil(t, cmd, "expected command from 'd' key")
	msg := cmd()
	deleteMsg, ok := msg.(DeleteIssueMsg)
	assert.True(t, ok, "expected DeleteIssueMsg")
	assert.Equal(t, "test-1", deleteMsg.IssueID)
	assert.Equal(t, beads.TypeTask, deleteMsg.IssueType)
}

func TestDetails_DeleteKey_EpicType(t *testing.T) {
	issue := beads.Issue{
		ID:        "epic-1",
		TitleText: "Epic Issue",
		Type:      beads.TypeEpic,
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Press 'd' to request deletion
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	msg := cmd()
	deleteMsg, ok := msg.(DeleteIssueMsg)
	assert.True(t, ok, "expected DeleteIssueMsg")
	assert.Equal(t, "epic-1", deleteMsg.IssueID)
	assert.Equal(t, beads.TypeEpic, deleteMsg.IssueType, "expected epic type for cascade handling")
}

func TestDetails_FooterShowsDeleteKeybinding(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)
	view := m.View()

	assert.Contains(t, view, "[d] Delete Issue", "expected footer to show delete keybinding")
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
	m := New(issue, nil).SetSize(120, 30)

	// Switch to metadata pane so we see the selection indicator
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

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
	m := New(issue, nil).SetSize(120, 30)

	// Switch to metadata pane so we see the selection indicator
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// TestDetails_View_Golden_WithLoadedDependencies tests rendering with fully loaded dependency data.
// Uses mockLoader to provide full issue details for dependencies.
// Run with -update flag to update golden files: go test -update ./internal/ui/details/...
func TestDetails_View_Golden_WithLoadedDependencies(t *testing.T) {
	// Create mock loader with dependency issue data
	loader := &mockLoader{
		issues: map[string]beads.Issue{
			"bug-101": {
				ID:        "bug-101",
				TitleText: "Critical login failure",
				Type:      beads.TypeBug,
				Priority:  beads.PriorityCritical,
				Status:    beads.StatusOpen,
			},
			"task-201": {
				ID:        "task-201",
				TitleText: "Update documentation",
				Type:      beads.TypeTask,
				Priority:  beads.PriorityLow,
				Status:    beads.StatusOpen,
			},
			"feature-301": {
				ID:        "feature-301",
				TitleText: "New dashboard widget",
				Type:      beads.TypeFeature,
				Priority:  beads.PriorityHigh,
				Status:    beads.StatusInProgress,
			},
		},
	}

	issue := beads.Issue{
		ID:              "task-500",
		TitleText:       "Integration task",
		DescriptionText: "This task shows fully loaded dependency rendering.",
		Type:            beads.TypeTask,
		Priority:        beads.PriorityMedium,
		Status:          beads.StatusOpen,
		BlockedBy:       []string{"bug-101"},
		Blocks:          []string{"task-201", "feature-301"},
		CreatedAt:       time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC),
	}
	m := New(issue, loader).SetSize(120, 30)

	// Switch to metadata pane so we see the selection indicator
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})

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
	m := New(issue, nil).SetSize(180, 25)

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
	m := New(issue, nil).SetSize(180, 25)

	view := m.View()
	teatest.RequireEqualOutput(t, []byte(view))
}

// Tests for dependency navigation (Phase 3 & 4)

func TestDetails_DependencyNavigation_JFromStatusToDependency(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		BlockedBy: []string{"dep-1", "dep-2"},
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Switch to metadata pane
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, FocusMetadata, m.focusPane)
	assert.Equal(t, FieldPriority, m.selectedField)

	// Move down to Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, FieldStatus, m.selectedField)

	// Move down to Dependencies
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, FieldDependency, m.selectedField)
	assert.Equal(t, 0, m.selectedDependency, "expected first dependency selected")
}

func TestDetails_DependencyNavigation_CycleThroughDependencies(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		BlockedBy: []string{"dep-1", "dep-2", "dep-3"},
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Navigate to FieldDependency
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // Priority -> Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // Status -> Dependency[0]
	assert.Equal(t, FieldDependency, m.selectedField)
	assert.Equal(t, 0, m.selectedDependency)

	// Cycle through dependencies
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, 1, m.selectedDependency)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, 2, m.selectedDependency)

	// Wrap around to Priority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, FieldPriority, m.selectedField)
}

func TestDetails_DependencyNavigation_KFromDependencyToStatus(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		BlockedBy: []string{"dep-1"},
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Navigate to FieldDependency
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // Priority -> Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // Status -> Dependency[0]
	assert.Equal(t, FieldDependency, m.selectedField)

	// k should go back to Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, FieldStatus, m.selectedField)
}

func TestDetails_DependencyNavigation_NoDependencies(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Navigate through metadata
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, FieldPriority, m.selectedField)

	// j should go Status -> Priority (wrap, no dependencies)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, FieldStatus, m.selectedField)

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, FieldPriority, m.selectedField, "expected wrap to Priority when no dependencies")
}

func TestDetails_EnterOnDependency_EmitsNavigateMsg(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		BlockedBy: []string{"target-dep"},
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Navigate to FieldDependency
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // Priority -> Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // Status -> Dependency[0]
	assert.Equal(t, FieldDependency, m.selectedField)

	// Press Enter
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.NotNil(t, cmd, "expected command from Enter on dependency")

	msg := cmd()
	navMsg, ok := msg.(NavigateToDependencyMsg)
	assert.True(t, ok, "expected NavigateToDependencyMsg")
	assert.Equal(t, "target-dep", navMsg.IssueID)
}

func TestDetails_EnterOnPriorityStillOpensPicker(t *testing.T) {
	issue := beads.Issue{
		ID:        "test-1",
		TitleText: "Test Issue",
		Priority:  beads.PriorityHigh,
		BlockedBy: []string{"dep-1"}, // Has dependencies but we're on Priority
		CreatedAt: time.Now(),
	}
	m := New(issue, nil)
	m = m.SetSize(100, 40)

	// Switch to metadata pane, stay on Priority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, FieldPriority, m.selectedField)

	// Press Enter - should open priority picker, not navigate
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(OpenPriorityPickerMsg)
	assert.True(t, ok, "expected OpenPriorityPickerMsg when Enter on Priority field")
}
