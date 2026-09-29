package shared

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/task"
)

// =============================================================================
// IssueContext Tests
// =============================================================================

func TestNewIssueContext(t *testing.T) {
	issue := &task.Issue{
		ID:        "ISSUE-123",
		TitleText: "Fix the bug",
	}

	ctx := NewIssueContext(issue)

	require.Equal(t, "ISSUE-123", ctx.ID)
	require.Equal(t, "Fix the bug", ctx.Title)
	require.Equal(t, "'Fix the bug'", ctx.TitleText)
}

func TestNewIssueContext_NilIssue(t *testing.T) {
	ctx := NewIssueContext(nil)

	require.Equal(t, "", ctx.ID)
	require.Equal(t, "", ctx.Title)
	require.Equal(t, "''", ctx.TitleText, "empty title escapes to an empty single-quoted word")
}

func TestNewIssueContext_TitleWithSpecialChars(t *testing.T) {
	issue := &task.Issue{
		ID:        "TEST-1",
		TitleText: "Fix bug with 'quotes'",
	}

	ctx := NewIssueContext(issue)

	require.Equal(t, "TEST-1", ctx.ID)
	// Title is raw, not escaped - user handles quoting in command template
	require.Equal(t, "Fix bug with 'quotes'", ctx.Title)
	// TitleText is shell-escaped with POSIX single-quote escaping
	require.Equal(t, `'Fix bug with '\''quotes'\'''`, ctx.TitleText)
}

// =============================================================================
// shellQuote Tests
// =============================================================================

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty", input: "", expected: `''`},
		{name: "plain", input: "Fix login", expected: `'Fix login'`},
		{name: "single quote", input: "Don't panic", expected: `'Don'\''t panic'`},
		{name: "only single quote", input: "'", expected: `''\'''`},
		{name: "double quotes", input: `Say "hi"`, expected: `'Say "hi"'`},
		{name: "command substitution", input: "$(rm -rf ~)", expected: `'$(rm -rf ~)'`},
		{name: "backticks", input: "`whoami`", expected: "'`whoami`'"},
		{name: "variable", input: "$HOME", expected: `'$HOME'`},
		{name: "newline", input: "line1\nline2", expected: "'line1\nline2'"},
		{name: "backslash", input: `a\b\`, expected: `'a\b\'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, shellQuote(tt.input))
		})
	}
}

// TestTitleText_ShellRoundTrip renders {{.TitleText}} into a command, runs it
// through sh -c (as ExecuteAction does), and verifies the shell receives the
// exact raw title as a single argument without evaluating any of its content.
func TestTitleText_ShellRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		title string
	}{
		{name: "empty", title: ""},
		{name: "plain", title: "Fix login"},
		{name: "single quote", title: "Don't panic"},
		{name: "quote breakout", title: "'; touch pwned; echo '"},
		{name: "command substitution", title: "$(touch pwned)"},
		{name: "backticks", title: "`touch pwned`"},
		{name: "variables", title: "$HOME and ${PATH}"},
		{name: "newline", title: "first line\ntouch pwned"},
		{name: "separators", title: "a; touch pwned | cat && touch pwned"},
		{name: "double quotes", title: `say "hi" \"there\"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			ctx := NewIssueContext(&task.Issue{ID: "TEST-1", TitleText: tt.title})

			// printf reuses its format for each argument, so word splitting
			// would show up as extra "|" separators in the output.
			rendered, err := renderCommand(`printf '%s|' {{.TitleText}}`, ctx)
			require.NoError(t, err)

			cmd := exec.Command("sh", "-c", rendered)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "command failed: %s", out)
			require.Equal(t, tt.title+"|", string(out))

			_, statErr := os.Stat(filepath.Join(dir, "pwned"))
			require.True(t, os.IsNotExist(statErr), "title content must not be executed by the shell")
		})
	}
}

// =============================================================================
// renderCommand Tests (Migrated from kanban/actions_test.go)
// =============================================================================

func TestRenderCommand(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		ctx      IssueContext
		expected string
		wantErr  bool
	}{
		{
			name:     "no template markers",
			tmpl:     "echo hello",
			ctx:      IssueContext{ID: "TEST-1"},
			expected: "echo hello",
		},
		{
			name:     "template with ID",
			tmpl:     "echo {{.ID}}",
			ctx:      IssueContext{ID: "TEST-123"},
			expected: "echo TEST-123",
		},
		{
			name:     "template with Title",
			tmpl:     "echo {{.Title}}",
			ctx:      IssueContext{Title: "Hello World"},
			expected: "echo Hello World",
		},
		{
			name:     "template with TitleText",
			tmpl:     "echo {{.TitleText}}",
			ctx:      NewIssueContext(&task.Issue{TitleText: "It's done"}),
			expected: `echo 'It'\''s done'`,
		},
		{
			name:    "invalid template field",
			tmpl:    "echo {{.Unknown}}",
			ctx:     IssueContext{ID: "TEST-1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderCommand(tt.tmpl, tt.ctx)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestRenderCommand_TemplateVariables(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		issue    *task.Issue
		expected string
	}{
		{
			name: "both ID and Title populated",
			tmpl: "echo {{.ID}} - {{.Title}}",
			issue: &task.Issue{
				ID:        "PROJ-456",
				TitleText: "Important feature",
			},
			expected: "echo PROJ-456 - Important feature",
		},
		{
			name: "complex command with template - user handles quoting",
			tmpl: `tmux split-window -h "claude 'Work on {{.ID}}: {{.Title}}'"`,
			issue: &task.Issue{
				ID:        "BUG-123",
				TitleText: "Fix login",
			},
			expected: `tmux split-window -h "claude 'Work on BUG-123: Fix login'"`,
		},
		{
			name: "TitleText concatenated with a quoted prefix",
			tmpl: `tmux split-window -h claude "Work on {{.ID}}: "{{.TitleText}}`,
			issue: &task.Issue{
				ID:        "BUG-123",
				TitleText: "Fix user's login",
			},
			expected: `tmux split-window -h claude "Work on BUG-123: "'Fix user'\''s login'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := NewIssueContext(tt.issue)
			result, err := renderCommand(tt.tmpl, ctx)
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// ExecuteAction Tests (Migrated from kanban/actions_test.go)
// =============================================================================

func TestExecuteAction_Success(t *testing.T) {
	issue := &task.Issue{
		ID:        "TEST-123",
		TitleText: "Test issue",
	}
	action := config.ActionConfig{
		Key:         "1",
		Command:     "echo 'hello world'",
		Description: "Test echo",
	}

	cmd := ExecuteAction(action, issue, t.TempDir())
	msg := cmd()

	result, ok := msg.(ActionExecutedMsg)
	require.True(t, ok, "expected ActionExecutedMsg")
	require.NoError(t, result.Err)
	require.Equal(t, "Test echo", result.Name)
}

func TestExecuteAction_TemplateRenderingError(t *testing.T) {
	issue := &task.Issue{
		ID: "TEST-123",
	}
	action := config.ActionConfig{
		Key:         "1",
		Command:     "echo {{.UnknownField}}",
		Description: "Bad template",
	}

	cmd := ExecuteAction(action, issue, t.TempDir())
	msg := cmd()

	result, ok := msg.(ActionExecutedMsg)
	require.True(t, ok, "expected ActionExecutedMsg")
	require.Error(t, result.Err)
	require.Contains(t, result.Err.Error(), "template rendering failed")
}

func TestExecuteAction_StartsCommand(t *testing.T) {
	issue := &task.Issue{
		ID:        "ISSUE-456",
		TitleText: "Fix the bug",
	}
	// Fire-and-forget: we just verify the command starts successfully
	action := config.ActionConfig{
		Key:         "1",
		Command:     "echo {{.ID}}",
		Description: "Template test",
	}

	cmd := ExecuteAction(action, issue, t.TempDir())
	msg := cmd()

	result, ok := msg.(ActionExecutedMsg)
	require.True(t, ok, "expected ActionExecutedMsg")
	require.NoError(t, result.Err)
}

func TestExecuteAction_NilIssue(t *testing.T) {
	action := config.ActionConfig{
		Key:         "1",
		Command:     "echo 'hello'",
		Description: "Nil issue test",
	}

	cmd := ExecuteAction(action, nil, t.TempDir())
	msg := cmd()

	result, ok := msg.(ActionExecutedMsg)
	require.True(t, ok, "expected ActionExecutedMsg")
	require.NoError(t, result.Err, "command should succeed with nil issue")
}

func TestExecuteAction_EmptyCommand(t *testing.T) {
	issue := &task.Issue{
		ID: "TEST-123",
	}
	action := config.ActionConfig{
		Key:         "1",
		Command:     "",
		Description: "Empty command",
	}

	cmd := ExecuteAction(action, issue, t.TempDir())
	msg := cmd()

	result, ok := msg.(ActionExecutedMsg)
	require.True(t, ok, "expected ActionExecutedMsg")
	require.NoError(t, result.Err)
}

// =============================================================================
// MatchUserAction Tests (New tests per task requirements)
// =============================================================================

func TestMatchUserAction_MatchesNumericKeys(t *testing.T) {
	actions := map[string]config.ActionConfig{
		"action-0": {Key: "0", Command: "echo 0", Description: "Zero"},
		"action-1": {Key: "1", Command: "echo 1", Description: "One"},
		"action-5": {Key: "5", Command: "echo 5", Description: "Five"},
		"action-9": {Key: "9", Command: "echo 9", Description: "Nine"},
	}

	tests := []struct {
		key          string
		expectMatch  bool
		expectedName string
	}{
		{"0", true, "action-0"},
		{"1", true, "action-1"},
		{"5", true, "action-5"},
		{"9", true, "action-9"},
		{"2", false, ""},
		{"3", false, ""},
	}

	for _, tt := range tests {
		t.Run("key_"+tt.key, func(t *testing.T) {
			msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)}
			action, name, found := MatchUserAction(msg, actions)

			if tt.expectMatch {
				require.True(t, found, "expected key %s to match", tt.key)
				require.Equal(t, tt.expectedName, name)
				require.Equal(t, tt.key, action.Key)
			} else {
				require.False(t, found, "expected key %s to not match", tt.key)
			}
		})
	}
}

func TestMatchUserAction_NoMatchForNonConfigured(t *testing.T) {
	actions := map[string]config.ActionConfig{
		"action-1": {Key: "1", Command: "echo 1", Description: "One"},
	}

	tests := []struct {
		name string
		key  tea.Key
	}{
		{"letter a", tea.Key{Type: tea.KeyRunes, Runes: []rune("a")}},
		{"letter z", tea.Key{Type: tea.KeyRunes, Runes: []rune("z")}},
		{"ctrl+c", tea.Key{Type: tea.KeyCtrlC}},
		{"enter", tea.Key{Type: tea.KeyEnter}},
		{"escape", tea.Key{Type: tea.KeyEscape}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tea.KeyMsg(tt.key)
			_, _, found := MatchUserAction(msg, actions)
			require.False(t, found, "key should not match user action")
		})
	}
}

func TestMatchUserAction_NilActions(t *testing.T) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")}
	_, _, found := MatchUserAction(msg, nil)
	require.False(t, found, "should return false for nil actions map")
}

func TestMatchUserAction_EmptyActions(t *testing.T) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")}
	actions := map[string]config.ActionConfig{}
	_, _, found := MatchUserAction(msg, actions)
	require.False(t, found, "should return false for empty actions map")
}
