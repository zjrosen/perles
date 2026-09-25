package issueeditor

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	zone "github.com/lrstanley/bubblezone"

	"github.com/zjrosen/perles/internal/task"
	"github.com/zjrosen/perles/internal/ui/shared/formmodal"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	zone.NewGlobal()
	os.Exit(m.Run())
}

// --- BuildUpdateOptions tests ---

func TestBuildUpdateOptions_AllFieldsChanged(t *testing.T) {
	original := &task.Issue{
		TitleText:       "Old Title",
		DescriptionText: "Old Desc",
		Notes:           "Old Notes",
		Priority:        task.PriorityLow,
		Status:          task.StatusOpen,
		Labels:          []string{"old-label"},
	}
	msg := SaveMsg{
		Title:       "New Title",
		Description: "New Desc",
		Notes:       "New Notes",
		Priority:    task.PriorityHigh,
		Status:      task.StatusClosed,
		Labels:      []string{"new-label"},
	}

	opts := msg.BuildUpdateOptions(original)

	require.NotNil(t, opts.Title)
	require.Equal(t, "New Title", *opts.Title)
	require.NotNil(t, opts.Description)
	require.Equal(t, "New Desc", *opts.Description)
	require.NotNil(t, opts.Notes)
	require.Equal(t, "New Notes", *opts.Notes)
	require.NotNil(t, opts.Priority)
	require.Equal(t, task.PriorityHigh, *opts.Priority)
	require.NotNil(t, opts.Status)
	require.Equal(t, task.StatusClosed, *opts.Status)
	require.NotNil(t, opts.Labels)
	require.Equal(t, []string{"new-label"}, *opts.Labels)
}

func TestBuildUpdateOptions_NoFieldsChanged(t *testing.T) {
	original := &task.Issue{
		TitleText:       "Same Title",
		DescriptionText: "Same Desc",
		Notes:           "Same Notes",
		Priority:        task.PriorityMedium,
		Status:          task.StatusOpen,
		Labels:          []string{"bug", "feature"},
	}
	msg := SaveMsg{
		Title:       "Same Title",
		Description: "Same Desc",
		Notes:       "Same Notes",
		Priority:    task.PriorityMedium,
		Status:      task.StatusOpen,
		Labels:      []string{"bug", "feature"},
	}

	opts := msg.BuildUpdateOptions(original)

	require.Nil(t, opts.Title)
	require.Nil(t, opts.Description)
	require.Nil(t, opts.Notes)
	require.Nil(t, opts.Priority)
	require.Nil(t, opts.Status)
	require.Nil(t, opts.Labels)
}

func TestBuildUpdateOptions_SingleFieldChanged(t *testing.T) {
	original := &task.Issue{
		TitleText:       "Original",
		DescriptionText: "Desc",
		Notes:           "Notes",
		Priority:        task.PriorityMedium,
		Status:          task.StatusOpen,
		Labels:          []string{"bug"},
	}
	msg := SaveMsg{
		Title:       "Changed Title",
		Description: "Desc",
		Notes:       "Notes",
		Priority:    task.PriorityMedium,
		Status:      task.StatusOpen,
		Labels:      []string{"bug"},
	}

	opts := msg.BuildUpdateOptions(original)

	require.NotNil(t, opts.Title, "only Title should be non-nil")
	require.Equal(t, "Changed Title", *opts.Title)
	require.Nil(t, opts.Description)
	require.Nil(t, opts.Notes)
	require.Nil(t, opts.Priority)
	require.Nil(t, opts.Status)
	require.Nil(t, opts.Labels)
}

func TestBuildUpdateOptions_NilOriginalFallback(t *testing.T) {
	msg := SaveMsg{
		Title:       "Title",
		Description: "Desc",
		Notes:       "Notes",
		Priority:    task.PriorityHigh,
		Status:      task.StatusInProgress,
		Labels:      []string{"label"},
	}

	opts := msg.BuildUpdateOptions(nil)

	require.NotNil(t, opts.Title)
	require.Equal(t, "Title", *opts.Title)
	require.NotNil(t, opts.Description)
	require.Equal(t, "Desc", *opts.Description)
	require.NotNil(t, opts.Notes)
	require.Equal(t, "Notes", *opts.Notes)
	require.NotNil(t, opts.Priority)
	require.Equal(t, task.PriorityHigh, *opts.Priority)
	require.NotNil(t, opts.Status)
	require.Equal(t, task.StatusInProgress, *opts.Status)
	require.NotNil(t, opts.Labels)
	require.Equal(t, []string{"label"}, *opts.Labels)
}

func TestBuildUpdateOptions_LabelsChanged(t *testing.T) {
	original := &task.Issue{
		TitleText: "T",
		Labels:    []string{"bug", "feature"},
	}
	msg := SaveMsg{
		Title:  "T",
		Labels: []string{"bug", "ui"},
	}

	opts := msg.BuildUpdateOptions(original)

	require.NotNil(t, opts.Labels)
	require.Equal(t, []string{"bug", "ui"}, *opts.Labels)
	require.Nil(t, opts.Title, "Title unchanged")
}

func TestBuildUpdateOptions_LabelsUnchanged(t *testing.T) {
	original := &task.Issue{
		TitleText: "T",
		Labels:    []string{"bug", "feature"},
	}
	msg := SaveMsg{
		Title:  "T",
		Labels: []string{"bug", "feature"},
	}

	opts := msg.BuildUpdateOptions(original)

	require.Nil(t, opts.Labels, "Labels should be nil when unchanged")
}

func TestBuildUpdateOptions_LabelsEmptyVsPopulated(t *testing.T) {
	original := &task.Issue{
		TitleText: "T",
		Labels:    []string{"bug"},
	}
	msg := SaveMsg{
		Title:  "T",
		Labels: []string{},
	}

	opts := msg.BuildUpdateOptions(original)

	require.NotNil(t, opts.Labels, "empty slice vs populated should be detected as change")
	require.Equal(t, []string{}, *opts.Labels)
}

func TestBuildUpdateOptions_ValueTypesUseAddressOfCopy(t *testing.T) {
	original := &task.Issue{
		TitleText: "T",
		Priority:  task.PriorityLow,
		Status:    task.StatusOpen,
	}
	msg := SaveMsg{
		Title:    "T",
		Priority: task.PriorityHigh,
		Status:   task.StatusClosed,
	}

	opts := msg.BuildUpdateOptions(original)

	require.NotNil(t, opts.Priority)
	require.NotNil(t, opts.Status)

	// Verify the pointers point to independent copies, not the SaveMsg fields.
	// Mutating the returned values should not affect msg.
	*opts.Priority = task.PriorityBacklog
	*opts.Status = task.StatusDeferred
	require.Equal(t, task.PriorityHigh, msg.Priority, "mutating opts.Priority must not affect SaveMsg")
	require.Equal(t, task.StatusClosed, msg.Status, "mutating opts.Status must not affect SaveMsg")
}

// testIssue creates a task.Issue for testing with the given parameters.
func testIssue(id string, labels []string, priority task.Priority, status task.Status) task.Issue {
	return task.Issue{
		ID:        id,
		TitleText: "Test Issue Title",
		Type:      task.TypeTask,
		Labels:    labels,
		Priority:  priority,
		Status:    status,
	}
}

// testIssueWithDescription creates a task.Issue with title and description for testing.
func testIssueWithDescription(id, title, description string, labels []string, priority task.Priority, status task.Status) task.Issue {
	return task.Issue{
		ID:              id,
		TitleText:       title,
		DescriptionText: description,
		Type:            task.TypeTask,
		Labels:          labels,
		Priority:        priority,
		Status:          status,
	}
}

// testIssueWithNotes creates a task.Issue with title, description, and notes for testing.
func testIssueWithNotes(id, title, description, notes string, labels []string, priority task.Priority, status task.Status) task.Issue {
	return task.Issue{
		ID:              id,
		TitleText:       title,
		DescriptionText: description,
		Notes:           notes,
		Type:            task.TypeTask,
		Labels:          labels,
		Priority:        priority,
		Status:          status,
	}
}

func findFieldConfig(fields []formmodal.FieldConfig, key string) (formmodal.FieldConfig, bool) {
	for _, field := range fields {
		if field.Key == key {
			return field, true
		}
	}
	return formmodal.FieldConfig{}, false
}

func TestIssueFields_CreateMode_ParentEpicVisibilityFollowsType(t *testing.T) {
	fields := issueFields(task.Issue{Type: task.TypeTask}, nil, false, true)
	parentField, ok := findFieldConfig(fields, "parent_id")
	require.True(t, ok, "create mode should include parent epic field")
	require.NotNil(t, parentField.VisibleWhen, "parent epic field should hide for epic issue types")
	require.True(t, parentField.VisibleWhen(map[string]any{"type": string(task.TypeTask)}))
	require.False(t, parentField.VisibleWhen(map[string]any{"type": string(task.TypeEpic)}))
}

func TestIssueFields_CreateMode_ParentTaskSearchesNonEpics(t *testing.T) {
	fields := issueFields(task.Issue{Type: task.TypeTask}, nil, false, true)
	parentTaskField, ok := findFieldConfig(fields, "parent_task_id")
	require.True(t, ok, "create mode should include parent task field")
	require.Equal(t, formmodal.FieldTypeEpicSearch, parentTaskField.Type)
	require.Equal(t, "type != epic", parentTaskField.SearchTypeFilter, "any non-epic issue can be a parent task")
	require.Equal(t, "Parent Task", parentTaskField.Label)
}

func TestIssueFields_CreateMode_ParentEpicAndTaskAreMutuallyExclusive(t *testing.T) {
	fields := issueFields(task.Issue{Type: task.TypeTask}, nil, false, true)
	epicField, ok := findFieldConfig(fields, "parent_id")
	require.True(t, ok)
	taskField, ok := findFieldConfig(fields, "parent_task_id")
	require.True(t, ok)

	none := map[string]any{"type": string(task.TypeTask), "parent_id": "", "parent_task_id": ""}
	require.True(t, epicField.VisibleWhen(none), "parent epic visible when nothing selected")
	require.True(t, taskField.VisibleWhen(none), "parent task visible when nothing selected")

	epicSelected := map[string]any{"type": string(task.TypeTask), "parent_id": "epic-1", "parent_task_id": ""}
	require.True(t, epicField.VisibleWhen(epicSelected))
	require.False(t, taskField.VisibleWhen(epicSelected), "parent task hidden once an epic is selected")

	taskSelected := map[string]any{"type": string(task.TypeTask), "parent_id": "", "parent_task_id": "task-1"}
	require.False(t, epicField.VisibleWhen(taskSelected), "parent epic hidden once a task is selected")
	require.True(t, taskField.VisibleWhen(taskSelected))

	epicType := map[string]any{"type": string(task.TypeEpic), "parent_id": "", "parent_task_id": ""}
	require.False(t, taskField.VisibleWhen(epicType), "parent task hidden for epic issue types")
}

func TestIssueFields_EditMode_IncludesParentFields(t *testing.T) {
	issue := task.Issue{ID: "task-1", Type: task.TypeTask, ParentID: "epic-9"}
	fields := issueFields(issue, nil, false, false)

	epicField, ok := findFieldConfig(fields, "parent_id")
	require.True(t, ok, "edit mode should include parent epic field")
	require.Equal(t, "epic-9", epicField.InitialValue, "current parent is pre-selected")
	require.Equal(t, `type = epic and id != "task-1"`, epicField.SearchTypeFilter, "issue can't be its own parent")

	taskField, ok := findFieldConfig(fields, "parent_task_id")
	require.True(t, ok, "edit mode should include parent task field")
	require.Equal(t, `type != epic and id != "task-1"`, taskField.SearchTypeFilter, "issue can't be its own parent")
}

func TestIssueFields_EditMode_EpicHasNoParentFields(t *testing.T) {
	fields := issueFields(task.Issue{ID: "epic-1", Type: task.TypeEpic}, nil, false, false)
	_, ok := findFieldConfig(fields, "parent_id")
	require.False(t, ok, "editing an epic should not offer a parent epic")
	_, ok = findFieldConfig(fields, "parent_task_id")
	require.False(t, ok, "editing an epic should not offer a parent task")
}

func TestSaveMsgFromValues_EditMode_ParentHandling(t *testing.T) {
	base := func(extra map[string]any) map[string]any {
		v := map[string]any{
			"title":       "Title",
			"description": "",
			"notes":       "",
			"priority":    "P2",
			"status":      string(task.StatusOpen),
			"labels":      []string{},
		}
		for k, val := range extra {
			v[k] = val
		}
		return v
	}
	issue := task.Issue{ID: "task-1", Type: task.TypeTask, ParentID: "epic-9"}

	msg := saveMsgFromValues(issue, base(map[string]any{"parent_task_id": "task-2"}), false)
	require.Equal(t, "task-2", msg.ParentID, "switching to a parent task")

	msg = saveMsgFromValues(issue, base(map[string]any{"parent_id": "", "parent_task_id": ""}), false)
	require.Empty(t, msg.ParentID, "clearing both fields removes the parent")

	epic := task.Issue{ID: "epic-1", Type: task.TypeEpic, ParentID: "epic-0"}
	msg = saveMsgFromValues(epic, base(nil), false)
	require.Equal(t, "epic-0", msg.ParentID, "epics have no parent fields, so their parent is left unchanged")
}

func TestBuildUpdateOptions_ParentID(t *testing.T) {
	original := &task.Issue{ID: "task-1", ParentID: "epic-9"}

	opts := SaveMsg{ParentID: "epic-9"}.BuildUpdateOptions(original)
	require.Nil(t, opts.ParentID, "unchanged parent should not be sent")

	opts = SaveMsg{ParentID: "task-2"}.BuildUpdateOptions(original)
	require.NotNil(t, opts.ParentID)
	require.Equal(t, "task-2", *opts.ParentID)

	opts = SaveMsg{ParentID: ""}.BuildUpdateOptions(original)
	require.NotNil(t, opts.ParentID, "clearing the parent should be sent")
	require.Empty(t, *opts.ParentID)

	opts = SaveMsg{ParentID: "epic-9"}.BuildUpdateOptions(nil)
	require.Nil(t, opts.ParentID, "create flows set the parent via CreateTask, not UpdateIssue")
}

type fakeQueryExecutor struct {
	issues  []task.Issue
	queries []string
}

func (f *fakeQueryExecutor) Execute(query string) ([]task.Issue, error) {
	f.queries = append(f.queries, query)
	return f.issues, nil
}

func TestEditMode_ResolvesParentIntoMatchingField(t *testing.T) {
	tests := []struct {
		name         string
		parent       task.Issue
		wantEpicID   string
		wantParentID string
	}{
		{
			name:       "epic parent stays in parent epic field",
			parent:     task.Issue{ID: "epic-9", TitleText: "Big Epic", Type: task.TypeEpic},
			wantEpicID: "epic-9",
		},
		{
			name:         "task parent moves to parent task field",
			parent:       task.Issue{ID: "task-9", TitleText: "Parent Task", Type: task.TypeTask},
			wantParentID: "task-9",
		},
		{
			name:         "bug parent moves to parent task field",
			parent:       task.Issue{ID: "bug-9", TitleText: "Parent Bug", Type: task.TypeBug},
			wantParentID: "bug-9",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &fakeQueryExecutor{issues: []task.Issue{tt.parent}}
			issue := task.Issue{ID: "task-1", Type: task.TypeTask, ParentID: tt.parent.ID}
			m := NewWithExecutorAndVimMode(issue, exec, false).SetSize(120, 40)

			cmd := m.resolveParentCmd()
			require.NotNil(t, cmd)
			m, _ = m.Update(cmd())
			require.Equal(t, []string{fmt.Sprintf(`id = "%s"`, tt.parent.ID)}, exec.queries)

			require.Equal(t, tt.wantEpicID, m.form.Value("parent_id"))
			require.Equal(t, tt.wantParentID, m.form.Value("parent_task_id"))
			require.Contains(t, m.View(), tt.parent.TitleText, "resolved parent title is displayed")
		})
	}
}

func TestEditMode_ResolveParentSkippedWithoutParent(t *testing.T) {
	exec := &fakeQueryExecutor{}
	m := NewWithExecutorAndVimMode(task.Issue{ID: "task-1", Type: task.TypeTask}, exec, false)
	require.Nil(t, m.resolveParentCmd())
}

func TestSaveMsgFromValues_CreateMode_ParentIDHandling(t *testing.T) {
	baseValues := map[string]any{
		"title":       "New issue",
		"description": "Desc",
		"notes":       "Notes",
		"priority":    "P2",
		"status":      string(task.StatusOpen),
		"labels":      []string{"bug"},
	}

	taskValues := map[string]any{
		"type":      string(task.TypeBug),
		"parent_id": "epic-123",
	}
	for k, v := range baseValues {
		taskValues[k] = v
	}

	msg := saveMsgFromValues(task.Issue{}, taskValues, true)
	require.Equal(t, "epic-123", msg.ParentID, "non-epic creates should preserve the selected parent epic")

	epicValues := map[string]any{
		"type": string(task.TypeEpic),
	}
	for k, v := range baseValues {
		epicValues[k] = v
	}

	msg = saveMsgFromValues(task.Issue{}, epicValues, true)
	require.Empty(t, msg.ParentID, "epic creates should clear any parent link")

	parentTaskValues := map[string]any{
		"type":           string(task.TypeTask),
		"parent_task_id": "task-456",
	}
	for k, v := range baseValues {
		parentTaskValues[k] = v
	}

	msg = saveMsgFromValues(task.Issue{}, parentTaskValues, true)
	require.Equal(t, "task-456", msg.ParentID, "non-epic creates should use the selected parent task")
}

func TestNew_InitializesFormModalWithCorrectFields(t *testing.T) {
	labels := []string{"bug", "feature"}
	issue := testIssue("test-123", labels, task.PriorityHigh, task.StatusOpen)
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
	opts := priorityListOptions(task.PriorityMedium)

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
		priority      task.Priority
		expectedIndex int
	}{
		{task.PriorityCritical, 0},
		{task.PriorityHigh, 1},
		{task.PriorityMedium, 2},
		{task.PriorityLow, 3},
		{task.PriorityBacklog, 4},
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
	opts := statusListOptions(task.StatusOpen)

	require.Len(t, opts, 5, "expected 5 status options")

	// Verify labels and values
	expectedLabels := []string{"Open", "In Progress", "Closed", "Deferred", "Blocked"}
	expectedValues := []string{"open", "in_progress", "closed", "deferred", "blocked"}

	for i, opt := range opts {
		require.Equal(t, expectedLabels[i], opt.Label, "expected label for option %d", i)
		require.Equal(t, expectedValues[i], opt.Value, "expected value for option %d", i)
	}

	// Verify Open (index 0) is selected
	require.True(t, opts[0].Selected, "expected Open to be selected")
	require.False(t, opts[1].Selected, "expected In Progress to not be selected")
	require.False(t, opts[2].Selected, "expected Closed to not be selected")
	require.False(t, opts[3].Selected, "expected Deferred to not be selected")
	require.False(t, opts[4].Selected, "expected Blocked to not be selected")
}

func TestStatusListOptions_SelectsCorrectStatus(t *testing.T) {
	tests := []struct {
		status        task.Status
		expectedIndex int
	}{
		{task.StatusOpen, 0},
		{task.StatusInProgress, 1},
		{task.StatusClosed, 2},
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
	issue := testIssue("test-123", []string{"existing"}, task.PriorityHigh, task.StatusInProgress)
	m := New(issue)

	// Navigate to submit button and press Enter
	// Tab through Title -> Priority -> Status -> Labels -> Add Label input -> Description -> Notes -> Submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to Priority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to Parent Epic
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to Parent Task
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to Labels
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to Add Label input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to Description
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to Notes
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // to Submit button

	// Press Enter to save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command to be returned")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg, got %T", msg)
	require.Equal(t, "test-123", saveMsg.IssueID, "expected correct issue ID")
	require.Equal(t, "Test Issue Title", saveMsg.Title, "expected correct title")
	require.Equal(t, "", saveMsg.Description, "expected empty description")
	require.Equal(t, task.PriorityHigh, saveMsg.Priority, "expected Priority 1 (High)")
	require.Equal(t, task.StatusInProgress, saveMsg.Status, "expected Status in_progress")
	require.Contains(t, saveMsg.Labels, "existing", "expected existing label")
}

func TestCancelMsg_ProducedOnEsc(t *testing.T) {
	issue := testIssue("test-123", []string{}, task.PriorityMedium, task.StatusOpen)
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
		expected task.Priority
	}{
		{"P0", task.PriorityCritical},
		{"P1", task.PriorityHigh},
		{"P2", task.PriorityMedium},
		{"P3", task.PriorityLow},
		{"P4", task.PriorityBacklog},
		{"invalid", task.PriorityMedium}, // default
		{"P", task.PriorityMedium},       // too short
		{"P99", task.PriorityMedium},     // out of range
		{"", task.PriorityMedium},        // empty
	}

	for _, tc := range tests {
		result := parsePriority(tc.input)
		require.Equal(t, tc.expected, result, "expected %d for input %q", tc.expected, tc.input)
	}
}

func TestNew_EmptyLabels_ProducesValidConfig(t *testing.T) {
	issue := testIssue("test-123", []string{}, task.PriorityMedium, task.StatusOpen)
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
	issue := testIssue("test-123", labels, task.PriorityMedium, task.StatusOpen)
	m := New(issue)

	view := m.View()
	require.Contains(t, view, "hello world", "expected label with spaces")
	require.Contains(t, view, "multi word label", "expected multi-word label")
}

func TestInit_ReturnsNil(t *testing.T) {
	issue := testIssue("test-123", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)
	cmd := m.Init()
	require.Nil(t, cmd, "expected Init to return nil")
}

func TestSetSize_ReturnsNewModel(t *testing.T) {
	issue := testIssue("test-123", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)

	m = m.SetSize(120, 40)
	// Verify it doesn't panic and returns a model
	m2 := m.SetSize(80, 24)
	_ = m2
}

func TestOverlay_RendersOverBackground(t *testing.T) {
	issue := testIssue("test-123", []string{"bug"}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)
	m = m.SetSize(80, 24)

	background := "This is the background content"
	overlay := m.Overlay(background)

	require.Contains(t, overlay, "Edit Issue", "expected modal title in overlay")
}

func TestView_ContainsAllPriorityOptions(t *testing.T) {
	issue := testIssue("test-123", []string{}, task.PriorityCritical, task.StatusOpen)
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
	issue := testIssue("test-123", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)
	view := m.View()

	// All status options should be visible
	require.Contains(t, view, "Open", "expected Open option")
	require.Contains(t, view, "In Progress", "expected In Progress option")
	require.Contains(t, view, "Closed", "expected Closed option")
}

func TestSaveMsg_PriorityChange(t *testing.T) {
	// Start with P0 (Critical)
	issue := testIssue("test-123", []string{}, task.PriorityCritical, task.StatusOpen)
	m := New(issue)

	// Tab to Priority field first (starts on Title)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Navigate down in priority list to P2 (Medium)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // P1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // P2
	// Press Space to confirm selection
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Tab to Parent Epic -> Parent Task -> Status -> Labels -> Add Label input -> Description -> Notes -> Submit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
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
	require.Equal(t, task.PriorityMedium, saveMsg.Priority, "expected P2 (Medium)")
}

func TestSaveMsg_StatusChange(t *testing.T) {
	// Start with Open status
	issue := testIssue("test-123", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)

	// Tab to Status field (Title -> Priority -> Parent Epic -> Parent Task -> Status)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Navigate down in status list to In Progress
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	// Press Space to confirm selection
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Tab to Labels -> Add Label input -> Description -> Notes -> Submit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
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
	require.Equal(t, task.StatusInProgress, saveMsg.Status, "expected in_progress status")
}

func TestSaveMsg_LabelsToggle(t *testing.T) {
	labels := []string{"bug", "feature", "ui"}
	issue := testIssue("test-123", labels, task.PriorityMedium, task.StatusOpen)
	m := New(issue)

	// Tab to Labels (Title -> Priority -> Status -> Labels)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Priority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Parent Epic
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Parent Task
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Labels

	// Toggle off "bug" (first label) with space
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})

	// Tab to Add Label input -> Description -> Notes -> Submit
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
	require.NotContains(t, saveMsg.Labels, "bug", "expected bug to be toggled off")
	require.Contains(t, saveMsg.Labels, "feature", "expected feature to remain")
	require.Contains(t, saveMsg.Labels, "ui", "expected ui to remain")
}

func TestSaveMsg_AddNewLabel(t *testing.T) {
	issue := testIssue("test-123", []string{"existing"}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)

	// Tab to Add Label input (Title -> Priority -> Status -> Labels -> Add Label input)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Priority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Parent Epic
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Parent Task
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Labels
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Add Label input

	// Type new label
	for _, r := range "new-label" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter to add the label
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Tab to Description -> Notes -> Submit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
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

// Tests for title and description fields

func TestNew_InitializesTitleField(t *testing.T) {
	issue := testIssueWithDescription("test-123", "My Custom Title", "", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)

	view := stripZoneMarkers(m.View())
	require.Contains(t, view, "Title", "expected Title field label")
	require.Contains(t, view, "My Custom Title", "expected title value in view")
}

func TestNew_InitializesDescriptionField(t *testing.T) {
	issue := testIssueWithDescription("test-123", "Title", "This is the description", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)

	view := m.View()
	require.Contains(t, view, "Description", "expected Description field label")
	require.Contains(t, view, "This is the description", "expected description value in view")
}

func TestSaveMsg_ContainsTitleValue(t *testing.T) {
	issue := testIssueWithDescription("test-123", "Original Title", "", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)

	// Tab through all fields to Submit button
	// Title -> Priority -> Status -> Labels -> Add Label -> Description -> Notes -> Submit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Priority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Parent Epic
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Parent Task
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Labels
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Add Label input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Description
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Notes
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Submit button

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg, got %T", msg)
	require.Equal(t, "Original Title", saveMsg.Title, "expected title from initial value")
}

func TestSaveMsg_ContainsDescriptionValue(t *testing.T) {
	issue := testIssueWithDescription("test-123", "Title", "Original Description", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)

	// Tab through all fields to Submit button
	// Title -> Priority -> Status -> Labels -> Add Label -> Description -> Notes -> Submit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Priority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Parent Epic
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Parent Task
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Labels
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Add Label input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Description
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Notes
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Submit button

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg, got %T", msg)
	require.Equal(t, "Original Description", saveMsg.Description, "expected description from initial value")
}

func TestView_FieldOrder(t *testing.T) {
	issue := testIssueWithNotes("test-123", "My Title", "My Description", "My Notes", []string{"label1"}, task.PriorityHigh, task.StatusOpen)
	m := New(issue)
	m = m.SetSize(80, 50)

	view := m.View()

	// Verify field order: Title -> Priority -> Status -> Labels -> Description -> Notes
	titleIdx := len(view) - len(view[findIndex(view, "Title"):])
	priorityIdx := len(view) - len(view[findIndex(view, "Priority"):])
	statusIdx := len(view) - len(view[findIndex(view, "Status"):])
	labelsIdx := len(view) - len(view[findIndex(view, "Labels"):])
	descriptionIdx := len(view) - len(view[findIndex(view, "Description"):])
	notesIdx := len(view) - len(view[findIndex(view, "Notes"):])

	require.Less(t, titleIdx, priorityIdx, "Title should come before Priority")
	require.Less(t, priorityIdx, statusIdx, "Priority should come before Status")
	require.Less(t, statusIdx, labelsIdx, "Status should come before Labels")
	require.Less(t, labelsIdx, descriptionIdx, "Labels should come before Description")
	require.Less(t, descriptionIdx, notesIdx, "Description should come before Notes")
}

// findIndex returns the index of the first occurrence of substr in s, or len(s) if not found.
func findIndex(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return len(s)
}

func TestView_ContainsTitleField(t *testing.T) {
	issue := testIssue("test-123", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)
	view := m.View()

	require.Contains(t, view, "Title", "expected Title field in view")
}

func TestView_ContainsDescriptionField(t *testing.T) {
	issue := testIssue("test-123", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)
	view := m.View()

	require.Contains(t, view, "Description", "expected Description field in view")
	require.Contains(t, view, "Ctrl+G for editor", "expected Ctrl+G hint in view")
}

// Tests for Notes field

func TestNew_InitializesNotesField(t *testing.T) {
	issue := testIssueWithNotes("test-123", "Title", "Description", "My notes here", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)

	view := m.View()
	require.Contains(t, view, "Notes", "expected Notes field label")
	require.Contains(t, view, "My notes here", "expected notes value in view")
}

func TestView_ContainsNotesField(t *testing.T) {
	issue := testIssue("test-123", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)
	view := m.View()

	require.Contains(t, view, "Notes", "expected Notes field in view")
	require.Contains(t, view, "Issue notes", "expected Internal notes hint in view")
}

func TestSaveMsg_ContainsNotesValue(t *testing.T) {
	issue := testIssueWithNotes("test-123", "Title", "Description", "Original Notes", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)

	// Tab through all fields to Submit button
	// Title -> Priority -> Status -> Labels -> Add Label input -> Description -> Notes -> Submit
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Priority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Parent Epic
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Parent Task
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Labels
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Add Label input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Description
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Notes
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Submit button

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg, got %T", msg)
	require.Equal(t, "Original Notes", saveMsg.Notes, "expected notes from initial value")
}

func TestIssueeditor_SaveMsg_IncludesNotes(t *testing.T) {
	issue := testIssueWithNotes("test-123", "Title", "Desc", "Test notes content", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)

	// Tab through all fields to Submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Priority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Parent Epic
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Parent Task
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Labels
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Add Label input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Description
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Notes
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Submit button

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg, got %T", msg)
	require.Equal(t, "Test notes content", saveMsg.Notes, "notes field value should appear in SaveMsg")
}

func TestIssueeditor_NotesField_VimEnabled(t *testing.T) {
	// VimEnabled starts in insert mode by default, so we can type directly
	issue := testIssueWithNotes("test-123", "Title", "Desc", "", []string{}, task.PriorityMedium, task.StatusOpen)
	m := NewWithVimMode(issue, true)

	// Tab to Notes field (Title -> Priority -> Status -> Labels -> Add Label input -> Description -> Notes)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Priority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Parent Epic
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Parent Task
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Labels
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Add Label input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Description
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab}) // Notes

	// Type some text (VimEnabled starts in insert mode)
	for _, r := range "vim mode works" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Esc to exit insert mode (verifies vim mode is active)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	// Tab to Submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Save
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	require.NotNil(t, cmd, "expected command")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg, got %T", msg)
	require.Equal(t, "vim mode works", saveMsg.Notes, "vim mode should allow typing in notes field")
}

func TestIssueEditor_TitleField_VimModeDisabled_EscapeCancels(t *testing.T) {
	issue := testIssue("test-title-esc-off", []string{}, task.PriorityMedium, task.StatusOpen)
	m := NewWithVimMode(issue, false).SetSize(120, 40)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	require.NotNil(t, cmd, "expected cancel command when vim mode is disabled")
	msg := cmd()
	_, ok := msg.(CancelMsg)
	require.True(t, ok, "expected CancelMsg when vim mode is disabled, got %T", msg)
}

func TestIssueEditor_TitleField_VimModeEnabled_EscapeStaysInModal(t *testing.T) {
	issue := testIssue("test-title-esc-on", []string{}, task.PriorityCritical, task.StatusOpen)
	m := NewWithVimMode(issue, true).SetSize(120, 40)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		msg := cmd()
		_, isCancel := msg.(CancelMsg)
		require.False(t, isCancel, "Escape should switch title vimtextarea mode, not cancel modal")
	}

	// Verify modal remains active by changing priority and saving.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})                       // Priority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // P1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})                     // select P1
	for range 8 {                                                       // Parent Epic -> Parent Task -> Status -> Labels -> Add Label -> Description -> Notes -> Submit
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd, "expected save command after Esc in vim mode")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg after Esc in vim mode, got %T", msg)
	require.Equal(t, task.PriorityHigh, saveMsg.Priority, "priority change proves modal stayed active after Esc")
}

func TestIssueeditor_EmptyNotes_DisplaysPlaceholder(t *testing.T) {
	issue := testIssue("test-123", []string{}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)

	view := m.View()
	require.Contains(t, view, "Issue notes...", "expected placeholder for empty notes field")
}

// Golden tests for visual regression testing
// Run with -update flag to update golden files: go test -update ./internal/ui/modals/issueeditor/...

func TestIssueEditor_View_Golden(t *testing.T) {
	issue := testIssue("test-123", []string{"bug", "feature"}, task.PriorityHigh, task.StatusOpen)
	m := New(issue)
	m = m.SetSize(80, 50) // Large enough to avoid scrolling
	view := stripZoneMarkers(m.View())

	teatest.RequireEqualOutput(t, []byte(view))
}

func TestIssueEditor_View_EmptyLabels_Golden(t *testing.T) {
	issue := testIssue("test-456", []string{}, task.PriorityMedium, task.StatusInProgress)
	m := New(issue)
	m = m.SetSize(80, 50) // Large enough to avoid scrolling
	view := stripZoneMarkers(m.View())

	teatest.RequireEqualOutput(t, []byte(view))
}

func TestIssueEditor_View_ManyLabels_Golden(t *testing.T) {
	labels := []string{"bug", "feature", "ui", "backend", "api", "database"}
	issue := testIssue("test-789", labels, task.PriorityCritical, task.StatusClosed)
	m := New(issue)
	m = m.SetSize(80, 50) // Large enough to avoid scrolling
	view := stripZoneMarkers(m.View())

	teatest.RequireEqualOutput(t, []byte(view))
}

// stripZoneMarkers removes bubblezone escape sequences from output.
// Zone IDs are global and vary based on test execution order, causing flakiness.
func stripZoneMarkers(s string) string {
	zonePattern := regexp.MustCompile(`\x1b\[\d+z`)
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	cleaned := zonePattern.ReplaceAllString(s, "")
	return ansiPattern.ReplaceAllString(cleaned, "")
}

// Golden tests for two-column layout

func TestIssueEditor_TwoColumn_120x40_Golden(t *testing.T) {
	// Two-column layout is enabled when width >= 100
	issue := testIssueWithNotes("test-layout", "Multi-Column Issue", "This description appears in column 1", "Internal notes here", []string{"bug", "feature"}, task.PriorityHigh, task.StatusOpen)
	m := New(issue)
	m = m.SetSize(120, 40) // Wide enough for two columns
	view := stripZoneMarkers(m.View())

	teatest.RequireEqualOutput(t, []byte(view))
}

func TestIssueEditor_LongTitle_120x40_Golden(t *testing.T) {
	const titleOverflowSentinel = "OVERLAP_SENTINEL"
	const descriptionSentinel = "DESC_SENTINEL"

	issue := testIssueWithNotes(
		"test-long-title",
		titleOverflowSentinel+" This is a deliberately long pre-filled issue title to validate two-column containment behavior in the editor modal",
		descriptionSentinel,
		"Internal notes for long title case",
		[]string{"bug", "ui"},
		task.PriorityHigh,
		task.StatusOpen,
	)
	m := New(issue)
	m = m.SetSize(120, 40)
	view := stripZoneMarkers(m.View())

	teatest.RequireEqualOutput(t, []byte(view))

	var descriptionRow string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, descriptionSentinel) {
			descriptionRow = line
			break
		}
	}
	require.NotEmpty(t, descriptionRow, "expected to find description row containing sentinel text")

	parts := strings.SplitN(descriptionRow, "│   │", 2)
	require.Len(t, parts, 2, "expected two-column row delimiter between Title and Description sections")
	require.NotContains(t, parts[1], titleOverflowSentinel, "title overflow should never appear in Description column")
}

func TestIssueEditor_TitleField_NextFlowViaEnterAndDownArrow(t *testing.T) {
	issue := testIssue("test-title-nav", []string{"bug"}, task.PriorityCritical, task.StatusOpen)
	m := New(issue).SetSize(120, 40)

	// Enter from title should move to Priority (same flow as legacy text input).
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd, "expected Enter on title textarea to emit navigation command")
	m, _ = m.Update(cmd())
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // P1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // P2
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})                     // select P2

	// Continue to submit and save.
	for range 8 { // Parent Epic -> Parent Task -> Status -> Labels -> Add Label -> Description -> Notes -> Submit
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd, "expected save command after Enter navigation from title")
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg, got %T", msg)
	require.Equal(t, task.PriorityMedium, saveMsg.Priority, "priority change proves Enter advanced focus off title field")

	// Down-arrow from title should also move to Priority.
	m = New(issue).SetSize(120, 40)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // P1
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // P2
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})                     // select P2
	for range 8 {                                                       // Parent Epic -> Parent Task -> Status -> Labels -> Add Label -> Description -> Notes -> Submit
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd, "expected save command after Down navigation from title")
	msg = cmd()
	saveMsg, ok = msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg, got %T", msg)
	require.Equal(t, task.PriorityMedium, saveMsg.Priority, "priority change proves Down-arrow advanced focus off title field")
}

func TestIssueEditor_SingleColumn_80x40_Golden(t *testing.T) {
	// Single-column fallback when width < 100
	issue := testIssueWithNotes("test-narrow", "Narrow Issue", "Description in single column", "Notes in single column", []string{"bug"}, task.PriorityMedium, task.StatusInProgress)
	m := New(issue)
	m = m.SetSize(80, 40) // Narrow: single column fallback
	view := stripZoneMarkers(m.View())

	teatest.RequireEqualOutput(t, []byte(view))
}

// Tab order tests verify that Tab/Shift-Tab traverse fields in array order regardless of column

func TestTabOrder_TraversesFieldsInArrayOrder(t *testing.T) {
	// Tab order should be: title -> priority -> parent epic -> parent task -> status -> labels -> add-label-input -> description -> notes -> submit
	issue := testIssueWithNotes("test-tab", "Tab Order Test", "Description", "Notes", []string{"label1"}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)
	m = m.SetSize(120, 40) // Two-column mode

	// Starting position: title field is focused

	// Tab to priority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// Tab to parent epic
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// Tab to parent task
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// Tab to status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// Tab to labels
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// Tab to add label input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// Tab to description
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// Tab to notes
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	// Tab to submit button
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})

	// Save and verify we reached submit
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd, "expected command from submit button")
	msg := cmd()
	_, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg, got %T - Tab order may be incorrect", msg)
}

func TestShiftTabOrder_ReversesCorrectly(t *testing.T) {
	// Shift-Tab from submit should go back through fields in reverse order
	issue := testIssueWithNotes("test-shift-tab", "Shift-Tab Test", "Description", "Notes", []string{"label1"}, task.PriorityMedium, task.StatusOpen)
	m := New(issue)
	m = m.SetSize(120, 40) // Two-column mode

	// Navigate to submit button first
	for i := 0; i < 9; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	}

	// Now Shift-Tab should go back: notes -> description -> add-label -> labels -> status -> parent task -> parent epic -> priority -> title
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // to notes
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // to description
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // to add-label input
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // to labels
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // to status
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // to parent task
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // to parent epic
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // to priority
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // to title

	// Type in title field to verify we're there
	for _, r := range " modified" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Tab forward to submit and save
	for i := 0; i < 9; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg := cmd()
	saveMsg, ok := msg.(SaveMsg)
	require.True(t, ok, "expected SaveMsg")
	require.Contains(t, saveMsg.Title, "modified", "title should have been modified via Shift-Tab navigation")
}

func TestTabOrder_ConsistentBetweenSingleAndTwoColumn(t *testing.T) {
	// Tab order should be identical regardless of column layout mode
	// Use labels to include the add-label input sub-focus in the tab cycle
	issue := testIssueWithNotes("test-consistent", "Consistent Tab", "Desc", "Notes", []string{"label1"}, task.PriorityLow, task.StatusClosed)

	// Test narrow width (single column)
	mNarrow := New(issue)
	mNarrow = mNarrow.SetSize(80, 40)

	// Test wide width (two column)
	mWide := New(issue)
	mWide = mWide.SetSize(120, 40)

	// Both should take the same number of tabs to reach submit
	// title -> priority -> parent epic -> parent task -> status -> labels -> add-label-input -> description -> notes -> submit
	tabsToSubmit := 9

	// Navigate narrow version to submit
	for i := 0; i < tabsToSubmit; i++ {
		mNarrow, _ = mNarrow.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	_, cmdNarrow := mNarrow.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmdNarrow, "narrow: expected command at submit")
	msgNarrow := cmdNarrow()
	_, okNarrow := msgNarrow.(SaveMsg)
	require.True(t, okNarrow, "narrow: expected SaveMsg at submit position")

	// Navigate wide version to submit
	for i := 0; i < tabsToSubmit; i++ {
		mWide, _ = mWide.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	_, cmdWide := mWide.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmdWide, "wide: expected command at submit")
	msgWide := cmdWide()
	_, okWide := msgWide.(SaveMsg)
	require.True(t, okWide, "wide: expected SaveMsg at submit position")
}
