package issueeditor

import (
	"os"
	"regexp"
	"testing"

	zone "github.com/lrstanley/bubblezone"

	beads "github.com/zjrosen/perles/internal/beads/domain"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	zone.NewGlobal()
	os.Exit(m.Run())
}

// testIssue creates a beads.Issue for testing with the given parameters.
func testIssue(id string, labels []string, priority beads.Priority, status beads.Status) beads.Issue {
	return beads.Issue{
		ID:        id,
		TitleText: "Test Issue Title",
		Type:      beads.TypeTask,
		Labels:    labels,
		Priority:  priority,
		Status:    status,
	}
}

func TestNew_InitializesFormModalWithCorrectFields(t *testing.T) {
	labels := []string{"bug", "feature"}
	issue := testIssue("test-123", labels, beads.PriorityHigh, beads.StatusOpen)
	m := New(issue)

	require.Equal(t, "test-123", m.issue.ID, "expected issue ID to be set")

	// Verify the view contains all three sections
	view := m.View()
	require.Contains(t, view, "Edit Issue", "expected title")
	require.Contains(t, view, "Priority", "expected Priority field")
	require.Contains(t, view, "Status", "expected Status field")
	require.Contains(t, view, "Labels", "expected Labels field")
}

func TestPriorityListOptions_Returns5Options(t *testing.T) {
	opts := priorityListOptions(beads.PriorityMedium)

	require.Len(t, opts, 5, "expected 5 priority options")

	// Verify labels and values
	expectedLabels := []string{
		"P0 - Critical",
		"P1 - High",
		"P2 - Medium",
		"P3 - Low",
		"P4 - Backlog",
	}
	expectedValues := []string{"P0", "P1", "P2", "P3", "P4"}

	for i, opt := range opts {
		require.Equal(t, expectedLabels[i], opt.Label, "expected label for option %d", i)
		require.Equal(t, expectedValues[i], opt.Value, "expected value for option %d", i)
	}

	// Verify P2 (Medium, index 2) is selected
	for i, opt := range opts {
		if i == 2 {
			require.True(t, opt.Selected, "expected P2 to be selected")
		} else {
			require.False(t, opt.Selected, "expected P%d to not be selected", i)
		}
	}
}

func TestPriorityListOptions_SelectsCorrectPriority(t *testing.T) {
	tests := []struct {
		priority      beads.Priority
		expectedIndex int
	}{
		{beads.PriorityCritical, 0},
		{beads.PriorityHigh, 1},
		{beads.PriorityMedium, 2},
		{beads.PriorityLow, 3},
		{beads.PriorityBacklog, 4},
	}

	for _, tc := range tests {
		opts := priorityListOptions(tc.priority)
		for i, opt := range opts {
			if i == tc.expectedIndex {
				require.True(t, opt.Selected, "expected index %d to be selected for priority %d", tc.expectedIndex, tc.priority)
			} else {
				require.False(t, opt.Selected, "expected index %d to not be selected for priority %d", i, tc.priority)
			}
		}
	}
}

func TestStatusListOptions_Returns3Options(t *testing.T) {
	opts := statusListOptions(beads.StatusOpen)

	require.Len(t, opts, 3, "expected 3 status options")

	// Verify labels and values
	expectedLabels := []string{"Open", "In Progress", "Closed"}
	expectedValues := []string{"open", "in_progress", "closed"}

	for i, opt := range opts {
		require.Equal(t, expectedLabels[i], opt.Label, "expected label for option %d", i)
		require.Equal(t, expectedValues[i], opt.Value, "expected value for option %d", i)
	}

	// Verify Open (index 0) is selected
	require.True(t, opts[0].Selected, "expected Open to be selected")
	require.False(t, opts[1].Selected, "expected In Progress to not be selected")
	require.False(t, opts[2].Selected, "expected Closed to not be selected")
}

func TestStatusListOptions_SelectsCorrectStatus(t *testing.T) {
	tests := []struct {
		status        beads.Status
		expectedIndex int
	}{
		{beads.StatusOpen, 0},
		{beads.StatusInProgress, 1},
		{beads.StatusClosed, 2},
	}

	for _, tc := range tests {
		opts := statusListOptions(tc.status)
		for i, opt := range opts {
			if i == tc.expectedIndex {
				require.True(t, opt.Selected, "expected index %d to be selected for status %s", tc.expectedIndex, tc.status)
			} else {
				require.False(t, opt.Selected, "expected index %d to not be selected for status %s", i, tc.status)
			}
		}
	}
}

func TestLabelsListOptions_MarksAllSelected(t *testing.T) {
	labels := []string{"bug", "feature", "enhancement"}
	opts := labelsListOptions(labels)

	require.Len(t, opts, 3, "expected 3 label options")

	for i, opt := range opts {
		require.Equal(t, labels[i], opt.Label, "expected label at index %d", i)
		require.Equal(t, labels[i], opt.Value, "expected value at index %d", i)
		require.True(t, opt.Selected, "expected option %d to be selected", i)
	}
}

func TestLabelsListOptions_EmptyLabels(t *testing.T) {
	opts := labelsListOptions([]string{})
	require.Len(t, opts, 0, "expected empty options slice")
	require.NotNil(t, opts, "expected non-nil slice")
}

func TestSaveMsg_ContainsCorrectParsedValues(t *testing.T) {
	issue := testIssue("test-123", []string{"existing"}, beads.PriorityHigh, beads.StatusInProgress)
	m := New(issue)

	// Navigate to submit button and press Enter
	// Tab through Priority -> Status -> Labels -> Add Label input -> Submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to Labels
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to Add Label input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to Submit button

	// Press Enter to save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg, got %T", msg)
	require.Equal(t, "test-123", saveMsg.IssueID, "expected correct issue ID")
	require.Equal(t, beads.PriorityHigh, saveMsg.Priority, "expected Priority 1 (High)")
	require.Equal(t, beads.StatusInProgress, saveMsg.Status, "expected Status in_progress")
	require.Contains(t, saveMsg.Labels, "existing", "expected existing label")
}

func TestCancelMsg_ProducedOnEsc(t *testing.T) {
	issue := testIssue("test-123", []string{}, beads.PriorityMedium, beads.StatusOpen)
	m := New(issue)

	// Press Esc
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	_, ok := msg.(CancelMsg)
	require.True(t, ok, "expected CancelMsg, got %T", msg)
}

func TestParsePriority(t *testing.T) {
	tests := []struct {
		input    string
		expected beads.Priority
	}{
		{"P0", beads.PriorityCritical},
		{"P1", beads.PriorityHigh},
		{"P2", beads.PriorityMedium},
		{"P3", beads.PriorityLow},
		{"P4", beads.PriorityBacklog},
		{"invalid", beads.PriorityMedium}, // default
		{"P", beads.PriorityMedium},       // too short
		{"P99", beads.PriorityMedium},     // out of range
		{"", beads.PriorityMedium},        // empty
	}

	for _, tc := range tests {
		result := parsePriority(tc.input)
		require.Equal(t, tc.expected, result, "expected %d for input %q", tc.expected, tc.input)
	}
}

func TestNew_EmptyLabels_ProducesValidConfig(t *testing.T) {
	issue := testIssue("test-123", []string{}, beads.PriorityMedium, beads.StatusOpen)
	m := New(issue)

	// View should still render without errors
	view := m.View()
	require.Contains(t, view, "Edit Issue", "expected title in view")
	require.Contains(t, view, "Labels", "expected Labels section")
	// Empty list shows "no items" state
	require.Contains(t, view, "no items", "expected empty state message")
}

func TestNew_LabelsWithSpaces(t *testing.T) {
	labels := []string{"hello world", "multi word label"}
	issue := testIssue("test-123", labels, beads.PriorityMedium, beads.StatusOpen)
	m := New(issue)

	view := m.View()
	require.Contains(t, view, "hello world", "expected label with spaces")
	require.Contains(t, view, "multi word label", "expected multi-word label")
}

func TestInit_ReturnsNil(t *testing.T) {
	issue := testIssue("test-123", []string{}, beads.PriorityMedium, beads.StatusOpen)
	m := New(issue)
	cmd := m.Init()
	require.Nil(t, cmd, "expected Init to return nil")
}

func TestSetSize_ReturnsNewModel(t *testing.T) {
	issue := testIssue("test-123", []string{}, beads.PriorityMedium, beads.StatusOpen)
	m := New(issue)

	m = m.SetSize(120, 40)
	// Verify it doesn't panic and returns a model
	m2 := m.SetSize(80, 24)
	_ = m2
}

func TestOverlay_RendersOverBackground(t *testing.T) {
	issue := testIssue("test-123", []string{"bug"}, beads.PriorityMedium, beads.StatusOpen)
	m := New(issue)
	m = m.SetSize(80, 24)

	background := "This is the background content"
	overlay := m.Overlay(background)

	require.Contains(t, overlay, "Edit Issue", "expected modal title in overlay")
}

func TestView_ContainsAllPriorityOptions(t *testing.T) {
	issue := testIssue("test-123", []string{}, beads.PriorityCritical, beads.StatusOpen)
	m := New(issue)
	view := m.View()

	// All priority options should be visible
	require.Contains(t, view, "P0 - Critical", "expected P0 option")
	require.Contains(t, view, "P1 - High", "expected P1 option")
	require.Contains(t, view, "P2 - Medium", "expected P2 option")
	require.Contains(t, view, "P3 - Low", "expected P3 option")
	require.Contains(t, view, "P4 - Backlog", "expected P4 option")
}

func TestView_ContainsAllStatusOptions(t *testing.T) {
	issue := testIssue("test-123", []string{}, beads.PriorityMedium, beads.StatusOpen)
	m := New(issue)
	view := m.View()

	// All status options should be visible
	require.Contains(t, view, "Open", "expected Open option")
	require.Contains(t, view, "In Progress", "expected In Progress option")
	require.Contains(t, view, "Closed", "expected Closed option")
}

func TestSaveMsg_PriorityChange(t *testing.T) {
	// Start with P0 (Critical)
	issue := testIssue("test-123", []string{}, beads.PriorityCritical, beads.StatusOpen)
	m := New(issue)

	// Navigate down in priority list to P2 (Medium)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // P1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // P2
	// Press Space to confirm selection
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Tab to Status -> Labels -> Add Label input -> Submit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg")
	require.Equal(t, beads.PriorityMedium, saveMsg.Priority, "expected P2 (Medium)")
}

func TestSaveMsg_StatusChange(t *testing.T) {
	// Start with Open status
	issue := testIssue("test-123", []string{}, beads.PriorityMedium, beads.StatusOpen)
	m := New(issue)

	// Tab to Status field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Navigate down in status list to In Progress
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	// Press Space to confirm selection
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Tab to Labels -> Add Label input -> Submit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg")
	require.Equal(t, beads.StatusInProgress, saveMsg.Status, "expected in_progress status")
}

func TestSaveMsg_LabelsToggle(t *testing.T) {
	labels := []string{"bug", "feature", "ui"}
	issue := testIssue("test-123", labels, beads.PriorityMedium, beads.StatusOpen)
	m := New(issue)

	// Tab to Status, then Labels
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Labels

	// Toggle off "bug" (first label) with space
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Tab to Add Label input -> Submit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg")
	require.NotContains(t, saveMsg.Labels, "bug", "expected bug to be toggled off")
	require.Contains(t, saveMsg.Labels, "feature", "expected feature to remain")
	require.Contains(t, saveMsg.Labels, "ui", "expected ui to remain")
}

func TestSaveMsg_AddNewLabel(t *testing.T) {
	issue := testIssue("test-123", []string{"existing"}, beads.PriorityMedium, beads.StatusOpen)
	m := New(issue)

	// Tab to Status, Labels, then Add Label input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Labels
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Add Label input

	// Type new label
	for _, r := range "new-label" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter to add the label
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Tab to Submit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg")
	require.Contains(t, saveMsg.Labels, "existing", "expected existing label")
	require.Contains(t, saveMsg.Labels, "new-label", "expected new label to be added")
}

// Golden tests for visual regression testing
// Run with -update flag to update golden files: go test -update ./internal/ui/modals/issueeditor/...

func TestIssueEditor_View_Golden(t *testing.T) {
	issue := testIssue("test-123", []string{"bug", "feature"}, beads.PriorityHigh, beads.StatusOpen)
	m := New(issue)
	m = m.SetSize(80, 50) // Large enough to avoid scrolling
	view := stripZoneMarkers(m.View())

	teatest.RequireEqualOutput(t, []byte(view))
}

func TestIssueEditor_View_EmptyLabels_Golden(t *testing.T) {
	issue := testIssue("test-456", []string{}, beads.PriorityMedium, beads.StatusInProgress)
	m := New(issue)
	m = m.SetSize(80, 50) // Large enough to avoid scrolling
	view := stripZoneMarkers(m.View())

	teatest.RequireEqualOutput(t, []byte(view))
}

func TestIssueEditor_View_ManyLabels_Golden(t *testing.T) {
	labels := []string{"bug", "feature", "ui", "backend", "api", "database"}
	issue := testIssue("test-789", labels, beads.PriorityCritical, beads.StatusClosed)
	m := New(issue)
	m = m.SetSize(80, 50) // Large enough to avoid scrolling
	view := stripZoneMarkers(m.View())

	teatest.RequireEqualOutput(t, []byte(view))
}

// stripZoneMarkers removes bubblezone escape sequences from output.
// Zone IDs are global and vary based on test execution order, causing flakiness.
func stripZoneMarkers(s string) string {
	zonePattern := regexp.MustCompile(`\x1b\[\d+z`)
	return zonePattern.ReplaceAllString(s, "")
}
