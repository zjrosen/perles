package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateColumns_Empty(t *testing.T) {
	err := ValidateColumns(nil)
	require.NoError(t, err, "empty columns should be valid (uses defaults)")
}

func TestValidateColumns_Valid(t *testing.T) {
	cols := []ColumnConfig{
		{Name: "Todo", Query: "status = open"},
		{Name: "In Progress", Query: "status = in_progress"},
		{Name: "Done", Query: "status = closed"},
	}
	err := ValidateColumns(cols)
	require.NoError(t, err)
}

func TestValidateColumns_MissingQuery(t *testing.T) {
	cols := []ColumnConfig{
		{Name: "All Issues", Query: ""}, // Missing query
	}
	err := ValidateColumns(cols)
	require.Error(t, err)
	require.Contains(t, err.Error(), "query is required")
}

func TestValidateColumns_ValidComplexQuery(t *testing.T) {
	cols := []ColumnConfig{
		{Name: "Active", Query: "status in (open, in_progress)"},
		{Name: "Done", Query: "status = closed"},
	}
	err := ValidateColumns(cols)
	require.NoError(t, err)
}

func TestValidateColumns_MissingName(t *testing.T) {
	cols := []ColumnConfig{
		{Name: "", Query: "status = open"},
	}
	err := ValidateColumns(cols)
	require.Error(t, err)
	require.Contains(t, err.Error(), "column 0: name is required")
}

func TestValidateColumns_SecondColumnMissingQuery(t *testing.T) {
	cols := []ColumnConfig{
		{Name: "Good", Query: "status = open"},
		{Name: "Bad", Query: ""},
	}
	err := ValidateColumns(cols)
	require.Error(t, err)
	require.Contains(t, err.Error(), "column 1")
	require.Contains(t, err.Error(), "query is required")
}

func TestDefaultColumns(t *testing.T) {
	cols := DefaultColumns()
	require.Len(t, cols, 4)

	require.Equal(t, "Blocked", cols[0].Name)
	require.Equal(t, "status = open and blocked = true", cols[0].Query)

	require.Equal(t, "Ready", cols[1].Name)
	require.Equal(t, "status = open and ready = true", cols[1].Query)

	require.Equal(t, "In Progress", cols[2].Name)
	require.Equal(t, "status = in_progress", cols[2].Query)

	require.Equal(t, "Closed", cols[3].Name)
	require.Equal(t, "status = closed", cols[3].Query)
}

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	require.True(t, cfg.AutoRefresh)
	require.Len(t, cfg.Views, 1)
	require.Equal(t, "Default", cfg.Views[0].Name)
	require.Len(t, cfg.Views[0].Columns, 4)
}

func TestDefaultViews(t *testing.T) {
	views := DefaultViews()
	require.Len(t, views, 1)
	require.Equal(t, "Default", views[0].Name)
	require.Len(t, views[0].Columns, 4)
}

func TestConfig_GetColumns(t *testing.T) {
	cfg := Defaults()
	cols := cfg.GetColumns()
	require.Len(t, cols, 4)
	require.Equal(t, "Blocked", cols[0].Name)
}

func TestConfig_GetColumns_Empty(t *testing.T) {
	cfg := Config{} // No views
	cols := cfg.GetColumns()
	// Should return defaults
	require.Len(t, cols, 4)
}

func TestConfig_GetViews(t *testing.T) {
	cfg := Config{
		Views: []ViewConfig{
			{Name: "Custom", Columns: []ColumnConfig{{Name: "Col1", Query: "status = open"}}},
		},
	}
	views := cfg.GetViews()
	require.Len(t, views, 1)
	require.Equal(t, "Custom", views[0].Name)
}

func TestConfig_GetViews_Empty(t *testing.T) {
	cfg := Config{} // No views
	views := cfg.GetViews()
	// Should return defaults
	require.Len(t, views, 1)
	require.Equal(t, "Default", views[0].Name)
	require.Len(t, views[0].Columns, 4)
}

func TestConfig_SetColumns(t *testing.T) {
	cfg := Defaults()
	newCols := []ColumnConfig{{Name: "Test", Query: "status = open"}}
	cfg.SetColumns(newCols)

	require.Len(t, cfg.Views[0].Columns, 1)
	require.Equal(t, "Test", cfg.Views[0].Columns[0].Name)
}

func TestConfig_SetColumns_NoViews(t *testing.T) {
	cfg := Config{} // No views
	newCols := []ColumnConfig{{Name: "Test", Query: "status = open"}}
	cfg.SetColumns(newCols)

	require.Len(t, cfg.Views, 1)
	require.Equal(t, "Default", cfg.Views[0].Name)
	require.Len(t, cfg.Views[0].Columns, 1)
}

func TestValidateViews_Empty(t *testing.T) {
	err := ValidateViews(nil)
	require.NoError(t, err, "empty views should be valid (uses defaults)")
}

func TestValidateViews_Valid(t *testing.T) {
	views := []ViewConfig{
		{
			Name: "Test",
			Columns: []ColumnConfig{
				{Name: "Open", Query: "status = open"},
			},
		},
	}
	err := ValidateViews(views)
	require.NoError(t, err)
}

func TestValidateViews_MissingName(t *testing.T) {
	views := []ViewConfig{
		{
			Name: "",
			Columns: []ColumnConfig{
				{Name: "Open", Query: "status = open"},
			},
		},
	}
	err := ValidateViews(views)
	require.Error(t, err)
	require.Contains(t, err.Error(), "view 0: name is required")
}

func TestValidateViews_EmptyColumns(t *testing.T) {
	// Empty columns array is valid - will show empty state UI
	views := []ViewConfig{
		{
			Name:    "Empty",
			Columns: []ColumnConfig{},
		},
	}
	err := ValidateViews(views)
	require.NoError(t, err)
}

func TestValidateViews_InvalidColumn(t *testing.T) {
	views := []ViewConfig{
		{
			Name: "Bad",
			Columns: []ColumnConfig{
				{Name: "Missing Query", Query: ""},
			},
		},
	}
	err := ValidateViews(views)
	require.Error(t, err)
	require.Contains(t, err.Error(), "query is required")
}

func TestConfig_GetColumnsForView(t *testing.T) {
	cfg := Config{
		Views: []ViewConfig{
			{Name: "View1", Columns: []ColumnConfig{{Name: "Col1", Query: "q1"}}},
			{Name: "View2", Columns: []ColumnConfig{{Name: "Col2", Query: "q2"}}},
		},
	}

	cols0 := cfg.GetColumnsForView(0)
	require.Len(t, cols0, 1)
	require.Equal(t, "Col1", cols0[0].Name)

	cols1 := cfg.GetColumnsForView(1)
	require.Len(t, cols1, 1)
	require.Equal(t, "Col2", cols1[0].Name)
}

func TestConfig_GetColumnsForView_OutOfRange(t *testing.T) {
	cfg := Config{
		Views: []ViewConfig{
			{Name: "View1", Columns: []ColumnConfig{{Name: "Col1", Query: "q1"}}},
		},
	}

	// Out of range should return defaults
	cols := cfg.GetColumnsForView(5)
	require.Len(t, cols, 4) // DefaultColumns has 4
}

func TestConfig_SetColumnsForView(t *testing.T) {
	cfg := Config{
		Views: []ViewConfig{
			{Name: "View1", Columns: []ColumnConfig{{Name: "Col1", Query: "q1"}}},
			{Name: "View2", Columns: []ColumnConfig{{Name: "Col2", Query: "q2"}}},
		},
	}

	newCols := []ColumnConfig{{Name: "Updated", Query: "updated"}}
	cfg.SetColumnsForView(1, newCols)

	// View1 unchanged
	require.Equal(t, "Col1", cfg.Views[0].Columns[0].Name)
	// View2 updated
	require.Equal(t, "Updated", cfg.Views[1].Columns[0].Name)
}

func TestConfig_SetColumnsForView_OutOfRange(t *testing.T) {
	cfg := Config{
		Views: []ViewConfig{
			{Name: "View1", Columns: []ColumnConfig{{Name: "Col1", Query: "q1"}}},
		},
	}

	newCols := []ColumnConfig{{Name: "Updated", Query: "updated"}}
	cfg.SetColumnsForView(5, newCols) // Out of range - should do nothing

	// Original unchanged
	require.Equal(t, "Col1", cfg.Views[0].Columns[0].Name)
}

// Tests for tree column type support

func TestValidateColumns_TreeType_Valid(t *testing.T) {
	cols := []ColumnConfig{
		{Name: "Dependencies", Type: "tree", IssueID: "bd-123"},
	}
	err := ValidateColumns(cols)
	require.NoError(t, err)
}

func TestValidateColumns_TreeType_MissingIssueID(t *testing.T) {
	cols := []ColumnConfig{
		{Name: "Dependencies", Type: "tree", IssueID: ""},
	}
	err := ValidateColumns(cols)
	require.Error(t, err)
	require.Contains(t, err.Error(), "issue_id is required for tree columns")
}

func TestValidateColumns_BQLType_Explicit(t *testing.T) {
	// Explicit type=bql should work the same as no type
	cols := []ColumnConfig{
		{Name: "Open", Type: "bql", Query: "status = open"},
	}
	err := ValidateColumns(cols)
	require.NoError(t, err)
}

func TestValidateColumns_BQLType_MissingQuery(t *testing.T) {
	cols := []ColumnConfig{
		{Name: "Open", Type: "bql", Query: ""},
	}
	err := ValidateColumns(cols)
	require.Error(t, err)
	require.Contains(t, err.Error(), "query is required for bql columns")
}

func TestValidateColumns_BackwardCompatibility_NoType(t *testing.T) {
	// Configs without Type field should default to bql behavior
	cols := []ColumnConfig{
		{Name: "Todo", Query: "status = open"},
		{Name: "In Progress", Query: "status = in_progress"},
	}
	err := ValidateColumns(cols)
	require.NoError(t, err)
}

func TestValidateColumns_InvalidType(t *testing.T) {
	cols := []ColumnConfig{
		{Name: "Bad", Type: "invalid", Query: "status = open"},
	}
	err := ValidateColumns(cols)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid type \"invalid\"")
}

func TestValidateColumns_MixedTypes(t *testing.T) {
	// Mixed bql and tree columns in the same view
	cols := []ColumnConfig{
		{Name: "Open", Type: "bql", Query: "status = open"},
		{Name: "Dependencies", Type: "tree", IssueID: "bd-123"},
		{Name: "Closed", Query: "status = closed"}, // No type = bql
	}
	err := ValidateColumns(cols)
	require.NoError(t, err)
}

func TestValidateColumns_TreeWithMode(t *testing.T) {
	// TreeMode field is optional (defaults in usage, not validation)
	cols := []ColumnConfig{
		{Name: "Deps Mode", Type: "tree", IssueID: "bd-123", TreeMode: "deps"},
		{Name: "Child Mode", Type: "tree", IssueID: "bd-456", TreeMode: "child"},
		{Name: "Default Mode", Type: "tree", IssueID: "bd-789"}, // No tree_mode
	}
	err := ValidateColumns(cols)
	require.NoError(t, err)
}

// Tests for orchestration config validation

func TestValidateOrchestration_Empty(t *testing.T) {
	// Empty config should be valid (uses defaults)
	err := ValidateOrchestration(OrchestrationConfig{})
	require.NoError(t, err)
}

func TestValidateOrchestration_ValidClaude(t *testing.T) {
	cfg := OrchestrationConfig{
		Client: "claude",
		Claude: ClaudeClientConfig{Model: "sonnet"},
	}
	err := ValidateOrchestration(cfg)
	require.NoError(t, err)
}

func TestValidateOrchestration_ValidAmp(t *testing.T) {
	cfg := OrchestrationConfig{
		Client: "amp",
		Amp:    AmpClientConfig{Model: "opus", Mode: "smart"},
	}
	err := ValidateOrchestration(cfg)
	require.NoError(t, err)
}

func TestValidateOrchestration_InvalidClient(t *testing.T) {
	cfg := OrchestrationConfig{
		Client: "invalid",
	}
	err := ValidateOrchestration(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "orchestration.client must be")
}

func TestValidateOrchestration_ValidClaudeModels(t *testing.T) {
	models := []string{"sonnet", "opus", "haiku"}
	for _, model := range models {
		cfg := OrchestrationConfig{
			Client: "claude",
			Claude: ClaudeClientConfig{Model: model},
		}
		err := ValidateOrchestration(cfg)
		require.NoError(t, err, "model %q should be valid", model)
	}
}

func TestValidateOrchestration_ValidAmpModels(t *testing.T) {
	models := []string{"opus", "sonnet"}
	for _, model := range models {
		cfg := OrchestrationConfig{
			Client: "amp",
			Amp:    AmpClientConfig{Model: model},
		}
		err := ValidateOrchestration(cfg)
		require.NoError(t, err, "model %q should be valid", model)
	}
}

func TestValidateOrchestration_InvalidAmpMode(t *testing.T) {
	cfg := OrchestrationConfig{
		Client: "amp",
		Amp:    AmpClientConfig{Mode: "invalid"},
	}
	err := ValidateOrchestration(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "orchestration.amp.mode must be")
}

func TestValidateOrchestration_ValidAmpModes(t *testing.T) {
	modes := []string{"free", "rush", "smart"}
	for _, mode := range modes {
		cfg := OrchestrationConfig{
			Client: "amp",
			Amp:    AmpClientConfig{Mode: mode},
		}
		err := ValidateOrchestration(cfg)
		require.NoError(t, err, "mode %q should be valid", mode)
	}
}

func TestDefaults_Orchestration(t *testing.T) {
	cfg := Defaults()

	require.Equal(t, "claude", cfg.Orchestration.Client)
	require.Equal(t, "claude-opus-4-5", cfg.Orchestration.Claude.Model)
	require.Equal(t, "opus", cfg.Orchestration.Amp.Model)
	require.Equal(t, "smart", cfg.Orchestration.Amp.Mode)
}

// Tests for workflow config validation

func TestValidateWorkflows_Empty(t *testing.T) {
	// Empty workflows should be valid
	err := ValidateWorkflows(nil)
	require.NoError(t, err)
}

func TestValidateWorkflows_ValidWithNameOnly(t *testing.T) {
	// Name only is valid (for disabling built-ins)
	workflows := []WorkflowConfig{
		{Name: "Debate"},
	}
	err := ValidateWorkflows(workflows)
	require.NoError(t, err)
}

func TestValidateWorkflows_ValidWithAllFields(t *testing.T) {
	enabled := true
	workflows := []WorkflowConfig{
		{
			Name:        "Code Review",
			Description: "Multi-perspective code review",
			Enabled:     &enabled,
		},
	}
	err := ValidateWorkflows(workflows)
	require.NoError(t, err)
}

func TestValidateWorkflows_MultipleWorkflows(t *testing.T) {
	enabled := false
	workflows := []WorkflowConfig{
		{Name: "Code Review"},
		{Name: "Debate", Enabled: &enabled},
		{Name: "Research", Description: "Custom research workflow"},
	}
	err := ValidateWorkflows(workflows)
	require.NoError(t, err)
}

func TestValidateOrchestration_WithValidWorkflows(t *testing.T) {
	cfg := OrchestrationConfig{
		Client: "claude",
		Workflows: []WorkflowConfig{
			{Name: "Code Review"},
		},
	}
	err := ValidateOrchestration(cfg)
	require.NoError(t, err)
}

// Tests for WorkflowConfig.IsEnabled

func TestWorkflowConfig_IsEnabled_NilEnabled(t *testing.T) {
	// nil Enabled should default to true
	wf := WorkflowConfig{Name: "Test"}
	require.True(t, wf.IsEnabled())
}

func TestWorkflowConfig_IsEnabled_True(t *testing.T) {
	enabled := true
	wf := WorkflowConfig{Name: "Test", Enabled: &enabled}
	require.True(t, wf.IsEnabled())
}

func TestWorkflowConfig_IsEnabled_False(t *testing.T) {
	enabled := false
	wf := WorkflowConfig{Name: "Test", Enabled: &enabled}
	require.False(t, wf.IsEnabled())
}

// Tests for VimMode UI config

func TestDefaults_VimModeDisabled(t *testing.T) {
	cfg := Defaults()
	require.False(t, cfg.UI.VimMode, "VimMode should be disabled by default")
}

func TestUIConfig_VimModeExplicit(t *testing.T) {
	// Test that VimMode can be explicitly set to true
	cfg := UIConfig{
		ShowCounts:    true,
		ShowStatusBar: true,
		VimMode:       true,
	}
	require.True(t, cfg.VimMode)
}

func TestUIConfig_VimModeZeroValue(t *testing.T) {
	// Test that zero value UIConfig has VimMode as false
	cfg := UIConfig{}
	require.False(t, cfg.VimMode)
}
