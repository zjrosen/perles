package labeleditor

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLabels() []string {
	return []string{"bug", "feature", "ui"}
}

func TestLabelEditor_New(t *testing.T) {
	labels := testLabels()
	m := New("test-123", labels)

	assert.Equal(t, "test-123", m.issueID, "expected issueID to be set")
	assert.Len(t, m.labels, 3, "expected 3 label items")
	assert.Equal(t, labels, m.originalLabels, "expected original labels preserved")
	assert.Equal(t, 0, m.selectedLabel, "expected default selection at 0")
	assert.Equal(t, FieldLabelList, m.focusedField, "expected initial focus on label list")

	// Verify all labels start enabled
	for i, item := range m.labels {
		assert.Equal(t, labels[i], item.name, "expected label name to match")
		assert.True(t, item.enabled, "expected label to be enabled by default")
	}
}

func TestLabelEditor_New_EmptyLabels(t *testing.T) {
	m := New("test-123", []string{})

	assert.Equal(t, "test-123", m.issueID, "expected issueID to be set")
	assert.Len(t, m.labels, 0, "expected empty labels")
	assert.Len(t, m.originalLabels, 0, "expected empty original labels")
}

func TestLabelEditor_AddLabel(t *testing.T) {
	m := New("test-123", []string{"existing"})
	m.focusedField = FieldInput

	// Type a label
	m.input.SetValue("new-label")

	// Press enter on input field
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.Len(t, m.labels, 2, "expected 2 labels after add")
	assert.Equal(t, "new-label", m.labels[1].name, "expected new-label to be added")
	assert.True(t, m.labels[1].enabled, "expected new label to be enabled")
	assert.Equal(t, "", m.input.Value(), "expected input cleared after add")
}

func TestLabelEditor_AddLabel_ViaInput(t *testing.T) {
	m := New("test-123", []string{})
	m.focusedField = FieldInput
	m.input.Focus()

	// Type a label
	m.input.SetValue("test-label")

	// Press enter
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.Len(t, m.labels, 1, "expected 1 label after add")
	assert.Equal(t, "test-label", m.labels[0].name, "expected correct label")
	assert.True(t, m.labels[0].enabled, "expected label to be enabled")
}

func TestLabelEditor_AddLabel_SelectsNewLabel(t *testing.T) {
	// Start with existing labels
	m := New("test-123", testLabels())
	m.focusedField = FieldLabelList

	// Navigate down through list
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, 2, m.selectedLabel, "expected selection at bottom of list")

	// Move to input (j at bottom goes to input)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, FieldInput, m.focusedField, "expected focus on input")

	// Add a new label
	m.input.SetValue("new-label")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// selectedLabel stays at old value until cycling
	assert.Equal(t, 2, m.selectedLabel, "expected selection unchanged after adding")
	assert.Len(t, m.labels, 4, "expected 4 labels total")

	// Cycle back to label list - when going reverse from input, should start at last label
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, FieldLabelList, m.focusedField, "expected focus on label list")
	assert.Equal(t, 3, m.selectedLabel, "expected selection at last label when cycling reverse")
	assert.Equal(t, "new-label", m.labels[m.selectedLabel].name, "expected new label selected")
}

func TestLabelEditor_ToggleLabel_Space(t *testing.T) {
	m := New("test-123", testLabels())
	m.focusedField = FieldLabelList
	m.selectedLabel = 1 // Select "feature"

	// All labels start enabled
	assert.True(t, m.labels[1].enabled, "expected 'feature' to be enabled initially")

	// Press space to toggle off
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	assert.False(t, m.labels[1].enabled, "expected 'feature' to be disabled after space")

	// Press space again to toggle back on
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	assert.True(t, m.labels[1].enabled, "expected 'feature' to be enabled again")

	// Other labels unchanged
	assert.True(t, m.labels[0].enabled, "expected 'bug' still enabled")
	assert.True(t, m.labels[2].enabled, "expected 'ui' still enabled")
}

func TestLabelEditor_ToggleLabel_Enter(t *testing.T) {
	m := New("test-123", testLabels())
	m.focusedField = FieldLabelList
	m.selectedLabel = 0 // Select "bug"

	// Press enter on label list to toggle
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, m.labels[0].enabled, "expected 'bug' to be disabled after enter")

	// Press enter again to toggle back
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, m.labels[0].enabled, "expected 'bug' to be enabled again")
}

func TestLabelEditor_Navigation_JK(t *testing.T) {
	m := New("test-123", testLabels())
	m.focusedField = FieldLabelList

	// Start at 0
	assert.Equal(t, 0, m.selectedLabel, "expected initial selection at 0")

	// Navigate down with 'j'
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, 1, m.selectedLabel, "expected selection at 1 after 'j'")

	// Navigate down with 'j' again
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, 2, m.selectedLabel, "expected selection at 2 after second 'j'")

	// Navigate down at bottom moves to next field (Input)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, FieldInput, m.focusedField, "expected focus to move to input field")

	// j in Input field should NOT navigate - it types 'j' in input
	// Use Tab to move to Done field instead
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FieldDone, m.focusedField, "expected focus to move to done field via tab")

	// k in Done field moves back to Input (but k in Input won't navigate)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, FieldInput, m.focusedField, "expected focus to move to input")

	// Use shift+tab to get back to label list
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, FieldLabelList, m.focusedField, "expected focus to move back to label list")
	// selectedLabel is preserved from before (was at 2)
	assert.Equal(t, 2, m.selectedLabel, "expected selection still at 2")

	// k navigates up through label list
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, 1, m.selectedLabel, "expected selection at 1 after k")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, 0, m.selectedLabel, "expected selection at 0 after k")

	// k at top of label list moves to previous field (Done)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, FieldDone, m.focusedField, "expected focus to wrap to done field")
}

func TestLabelEditor_JKInInput(t *testing.T) {
	m := New("test-123", []string{})
	m.focusedField = FieldInput
	m.input.Focus()

	// j and k should NOT navigate when in input field
	// They should fall through to text input
	origField := m.focusedField
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, origField, m.focusedField, "expected j to not navigate in input field")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	assert.Equal(t, origField, m.focusedField, "expected k to not navigate in input field")
}

func TestLabelEditor_Navigation_CtrlNP(t *testing.T) {
	m := New("test-123", testLabels())
	m.focusedField = FieldLabelList

	// Start at 0
	assert.Equal(t, 0, m.selectedLabel, "expected initial selection at 0")

	// Navigate down with ctrl+n
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	assert.Equal(t, 1, m.selectedLabel, "expected selection at 1 after ctrl+n")

	// Navigate up with ctrl+p
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	assert.Equal(t, 0, m.selectedLabel, "expected selection at 0 after ctrl+p")

	// ctrl+n/ctrl+p should navigate even from input field
	m.focusedField = FieldInput
	m.input.Focus()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	assert.Equal(t, FieldDone, m.focusedField, "expected ctrl+n to navigate from input to done")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	assert.Equal(t, FieldInput, m.focusedField, "expected ctrl+p to navigate from done to input")
}

func TestLabelEditor_Navigation_Tab(t *testing.T) {
	m := New("test-123", testLabels())

	// Start at FieldLabelList
	assert.Equal(t, FieldLabelList, m.focusedField, "expected initial focus on label list")

	// Tab cycles through fields (no Save button anymore)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FieldInput, m.focusedField, "expected focus on input after first tab")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FieldDone, m.focusedField, "expected focus on done after second tab")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	assert.Equal(t, FieldLabelList, m.focusedField, "expected focus to wrap to label list")
}

func TestLabelEditor_Navigation_ShiftTab(t *testing.T) {
	m := New("test-123", testLabels())
	m.focusedField = FieldDone

	// Shift+Tab cycles backward (no Save button anymore)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, FieldInput, m.focusedField, "expected focus on input after shift+tab")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, FieldLabelList, m.focusedField, "expected focus on label list after shift+tab")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	assert.Equal(t, FieldDone, m.focusedField, "expected focus to wrap to done")
}

func TestLabelEditor_DuplicatePrevention(t *testing.T) {
	m := New("test-123", testLabels())
	m.focusedField = FieldInput

	// Try to add existing label
	m.input.SetValue("bug")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.Len(t, m.labels, 3, "expected labels unchanged (duplicate rejected)")
	// Input is NOT cleared when duplicate rejected (user may want to edit typo)
	assert.Equal(t, "bug", m.input.Value(), "expected input preserved on duplicate")

	// Count occurrences of "bug"
	count := 0
	for _, item := range m.labels {
		if item.name == "bug" {
			count++
		}
	}
	assert.Equal(t, 1, count, "expected exactly one 'bug' label")
}

func TestLabelEditor_EmptyInput(t *testing.T) {
	m := New("test-123", testLabels())
	m.focusedField = FieldInput

	// Try to add empty label
	m.input.SetValue("")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.Len(t, m.labels, 3, "expected labels unchanged (empty rejected)")

	// Try with whitespace only
	m.input.SetValue("   ")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.Len(t, m.labels, 3, "expected labels unchanged (whitespace rejected)")
}

func TestLabelEditor_Cancel_Esc(t *testing.T) {
	m := New("test-123", testLabels())

	// Toggle a label off
	m.focusedField = FieldLabelList
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Press Esc
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// Should emit CancelMsg
	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	_, ok := msg.(CancelMsg)
	assert.True(t, ok, "expected CancelMsg to be returned")
}

func TestLabelEditor_Save_OnlyEnabledLabels(t *testing.T) {
	m := New("test-123", testLabels())
	m.focusedField = FieldLabelList

	// Disable "feature" (index 1)
	m.selectedLabel = 1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Switch to Done and save
	m.focusedField = FieldDone
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should emit SaveMsg with only enabled labels
	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	assert.True(t, ok, "expected SaveMsg to be returned")
	assert.Equal(t, "test-123", saveMsg.IssueID, "expected correct issue ID")
	assert.Len(t, saveMsg.Labels, 2, "expected 2 enabled labels")
	assert.Contains(t, saveMsg.Labels, "bug", "expected 'bug' in SaveMsg")
	assert.Contains(t, saveMsg.Labels, "ui", "expected 'ui' in SaveMsg")
	assert.NotContains(t, saveMsg.Labels, "feature", "expected 'feature' NOT in SaveMsg")
}

func TestLabelEditor_Save_WithNewLabel(t *testing.T) {
	m := New("test-123", testLabels())

	// Add a new label
	m.focusedField = FieldInput
	m.input.SetValue("new-label")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Save
	m.focusedField = FieldDone
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should emit SaveMsg with all labels (including new one)
	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	assert.True(t, ok, "expected SaveMsg to be returned")
	assert.Len(t, saveMsg.Labels, 4, "expected 4 labels in SaveMsg")
	assert.Contains(t, saveMsg.Labels, "new-label", "expected new-label in SaveMsg")
}

func TestLabelEditor_Init(t *testing.T) {
	m := New("test-123", testLabels())
	cmd := m.Init()
	assert.Nil(t, cmd, "expected Init to return nil")
}

func TestLabelEditor_SetSize(t *testing.T) {
	m := New("test-123", testLabels())

	m = m.SetSize(120, 40)
	assert.Equal(t, 120, m.width, "expected width to be 120")
	assert.Equal(t, 40, m.height, "expected height to be 40")

	// Verify immutability
	m2 := m.SetSize(80, 24)
	assert.Equal(t, 80, m2.width, "expected new model width to be 80")
	assert.Equal(t, 120, m.width, "expected original model width unchanged")
}

func TestLabelEditor_View(t *testing.T) {
	m := New("test-123", testLabels()).SetSize(80, 24)
	view := m.View()

	// Should contain title
	assert.Contains(t, view, "Edit Labels", "expected view to contain title")

	// Should contain labels
	assert.Contains(t, view, "bug", "expected view to contain 'bug'")
	assert.Contains(t, view, "feature", "expected view to contain 'feature'")
	assert.Contains(t, view, "ui", "expected view to contain 'ui'")

	// Should have checkboxes (all enabled by default)
	assert.Contains(t, view, "[x]", "expected view to contain enabled checkboxes")

	// Should have input hint and Save button
	assert.Contains(t, view, "Enter to add", "expected view to contain input hint")
	assert.Contains(t, view, "Save", "expected view to contain Save button")
}

func TestLabelEditor_View_WithDisabled(t *testing.T) {
	m := New("test-123", testLabels()).SetSize(80, 24)
	m.focusedField = FieldLabelList
	m.selectedLabel = 0

	// Disable first label
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	view := m.View()

	// Should show unchecked checkbox
	assert.Contains(t, view, "[ ]", "expected view to contain disabled checkbox")
}

func TestLabelEditor_View_Empty(t *testing.T) {
	m := New("test-123", []string{}).SetSize(80, 24)
	view := m.View()

	assert.Contains(t, view, "no labels", "expected empty state message")
}

func TestLabelEditor_EnabledLabels(t *testing.T) {
	m := New("test-123", testLabels())

	// All enabled initially
	enabled := m.enabledLabels()
	assert.Len(t, enabled, 3, "expected all 3 labels enabled")

	// Disable one
	m.labels[1].enabled = false
	enabled = m.enabledLabels()
	assert.Len(t, enabled, 2, "expected 2 labels enabled")
	assert.NotContains(t, enabled, "feature", "expected 'feature' not in enabled list")

	// Disable all
	m.labels[0].enabled = false
	m.labels[2].enabled = false
	enabled = m.enabledLabels()
	assert.Len(t, enabled, 0, "expected 0 labels enabled")
}

func TestLabelEditor_ContainsLabel(t *testing.T) {
	m := New("test-123", testLabels())

	assert.True(t, m.containsLabel("bug"), "expected to find 'bug'")
	assert.True(t, m.containsLabel("feature"), "expected to find 'feature'")
	assert.False(t, m.containsLabel("nonexistent"), "expected not to find 'nonexistent'")

	// Should find even disabled labels
	m.labels[0].enabled = false
	assert.True(t, m.containsLabel("bug"), "expected to find disabled 'bug'")
}

func TestLabelEditor_SpaceInInput(t *testing.T) {
	m := New("test-123", []string{})
	m.focusedField = FieldInput
	m.input.Focus()

	// Type "hello world" with space
	m.input.SetValue("hello")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	// Space should be passed to text input, not toggle anything
	// Note: The textinput component handles the actual character insertion
	// This test verifies space doesn't trigger toggle behavior
	assert.Equal(t, FieldInput, m.focusedField, "expected to stay in input field")
	assert.Len(t, m.labels, 0, "expected no labels to be affected by space in input")
}

// TestLabelEditor_View_Golden uses teatest golden file comparison
// Run with -update flag to update golden files: go test -update ./internal/ui/labeleditor/...
func TestLabelEditor_View_Golden(t *testing.T) {
	m := New("test-123", testLabels()).SetSize(80, 24)

	view := m.View()

	teatest.RequireEqualOutput(t, []byte(view))
}
