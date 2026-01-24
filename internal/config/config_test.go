package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zjrosen/perles/internal/orchestration/client"
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
	require.Contains(t, err.Error(), "gemini") // Verify gemini is mentioned as valid option
}

func TestValidateOrchestration_ValidGemini(t *testing.T) {
	cfg := OrchestrationConfig{
		Client: "gemini",
		Gemini: GeminiClientConfig{Model: "gemini-2.5-pro"},
	}
	err := ValidateOrchestration(cfg)
	require.NoError(t, err)
}

func TestValidateOrchestration_ValidOpencode(t *testing.T) {
	cfg := OrchestrationConfig{
		Client: "opencode",
	}
	err := ValidateOrchestration(cfg)
	require.NoError(t, err)
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

func TestValidateOrchestration_ValidCoordinatorClient(t *testing.T) {
	clients := []string{"claude", "amp", "codex", "gemini", "opencode"}
	for _, c := range clients {
		t.Run(c, func(t *testing.T) {
			cfg := OrchestrationConfig{CoordinatorClient: c}
			err := ValidateOrchestration(cfg)
			require.NoError(t, err)
		})
	}
}

func TestValidateOrchestration_InvalidCoordinatorClient(t *testing.T) {
	cfg := OrchestrationConfig{CoordinatorClient: "invalid"}
	err := ValidateOrchestration(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "orchestration.coordinator_client must be one of")
	require.Contains(t, err.Error(), "invalid")
}

func TestValidateOrchestration_ValidWorkerClient(t *testing.T) {
	clients := []string{"claude", "amp", "codex", "gemini", "opencode"}
	for _, c := range clients {
		t.Run(c, func(t *testing.T) {
			cfg := OrchestrationConfig{WorkerClient: c}
			err := ValidateOrchestration(cfg)
			require.NoError(t, err)
		})
	}
}

func TestValidateOrchestration_InvalidWorkerClient(t *testing.T) {
	cfg := OrchestrationConfig{WorkerClient: "invalid"}
	err := ValidateOrchestration(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "orchestration.worker_client must be one of")
	require.Contains(t, err.Error(), "invalid")
}

func TestValidateOrchestration_MixedClientConfigs(t *testing.T) {
	cfg := OrchestrationConfig{
		Client:            "claude",
		CoordinatorClient: "amp",
		WorkerClient:      "codex",
	}
	err := ValidateOrchestration(cfg)
	require.NoError(t, err)
}

func TestValidateOrchestration_InvalidClientButValidCoordinatorWorker(t *testing.T) {
	cfg := OrchestrationConfig{
		Client:            "invalid",
		CoordinatorClient: "claude",
		WorkerClient:      "amp",
	}
	err := ValidateOrchestration(cfg)
	require.Error(t, err, "invalid Client should still fail validation")
	require.Contains(t, err.Error(), "orchestration.client")
}

// Tests for GeminiClientConfig

func TestGeminiClientConfig_ZeroValue(t *testing.T) {
	// Test that zero value GeminiClientConfig has expected defaults
	cfg := GeminiClientConfig{}
	require.Empty(t, cfg.Model, "Model zero value should be empty")
}

func TestGeminiClientConfig_WithModel(t *testing.T) {
	cfg := GeminiClientConfig{
		Model: "gemini-2.5-flash",
	}
	require.Equal(t, "gemini-2.5-flash", cfg.Model)
}

func TestOrchestrationConfig_GeminiField(t *testing.T) {
	// Verify OrchestrationConfig includes Gemini field
	cfg := OrchestrationConfig{
		Client: "gemini",
		Gemini: GeminiClientConfig{
			Model: "gemini-2.5-pro",
		},
	}
	require.Equal(t, "gemini-2.5-pro", cfg.Gemini.Model)
}

func TestValidateOrchestration_ValidGeminiModels(t *testing.T) {
	models := []string{"gemini-2.5-pro", "gemini-2.5-flash"}
	for _, model := range models {
		cfg := OrchestrationConfig{
			Client: "gemini",
			Gemini: GeminiClientConfig{Model: model},
		}
		err := ValidateOrchestration(cfg)
		require.NoError(t, err, "model %q should be valid", model)
	}
}

func TestOrchestrationConfig_OpenCodeField(t *testing.T) {
	// Verify OrchestrationConfig includes OpenCode field
	cfg := OrchestrationConfig{
		Client: "opencode",
		OpenCode: OpenCodeClientConfig{
			Model: "anthropic/claude-sonnet-4",
		},
	}
	require.Equal(t, "anthropic/claude-sonnet-4", cfg.OpenCode.Model)
}

func TestValidateOrchestration_ValidOpenCodeModels(t *testing.T) {
	models := []string{"anthropic/claude-opus-4-5", "anthropic/claude-sonnet-4", "openai/gpt-4o"}
	for _, model := range models {
		cfg := OrchestrationConfig{
			Client:   "opencode",
			OpenCode: OpenCodeClientConfig{Model: model},
		}
		err := ValidateOrchestration(cfg)
		require.NoError(t, err, "model %q should be valid", model)
	}
}

func TestDefaults_Orchestration(t *testing.T) {
	cfg := Defaults()

	require.Empty(t, cfg.Orchestration.Client, "legacy Client field should be empty")
	require.Equal(t, "claude", cfg.Orchestration.CoordinatorClient)
	require.Equal(t, "claude", cfg.Orchestration.WorkerClient)
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

// Tests for ThemeConfig.FlattenedColors

func TestThemeConfig_FlattenedColors_Nil(t *testing.T) {
	cfg := ThemeConfig{}
	result := cfg.FlattenedColors()
	require.NotNil(t, result)
	require.Empty(t, result)
}

func TestThemeConfig_FlattenedColors_FlatKeys(t *testing.T) {
	// Already flat keys (quoted in YAML) should pass through
	cfg := ThemeConfig{
		Colors: map[string]any{
			"text.primary": "#FF0000",
			"status.error": "#00FF00",
		},
	}
	result := cfg.FlattenedColors()
	require.Len(t, result, 2)
	require.Equal(t, "#FF0000", result["text.primary"])
	require.Equal(t, "#00FF00", result["status.error"])
}

func TestThemeConfig_FlattenedColors_NestedKeys(t *testing.T) {
	// Nested structure (natural YAML) should be flattened
	cfg := ThemeConfig{
		Colors: map[string]any{
			"text": map[string]any{
				"primary":   "#FF0000",
				"secondary": "#00FF00",
			},
			"status": map[string]any{
				"error": "#0000FF",
			},
		},
	}
	result := cfg.FlattenedColors()
	require.Len(t, result, 3)
	require.Equal(t, "#FF0000", result["text.primary"])
	require.Equal(t, "#00FF00", result["text.secondary"])
	require.Equal(t, "#0000FF", result["status.error"])
}

func TestThemeConfig_FlattenedColors_DeeplyNested(t *testing.T) {
	// Deeply nested structure (e.g., button.primary.bg)
	cfg := ThemeConfig{
		Colors: map[string]any{
			"button": map[string]any{
				"primary": map[string]any{
					"bg":    "#FF0000",
					"focus": "#00FF00",
				},
			},
		},
	}
	result := cfg.FlattenedColors()
	require.Len(t, result, 2)
	require.Equal(t, "#FF0000", result["button.primary.bg"])
	require.Equal(t, "#00FF00", result["button.primary.focus"])
}

func TestThemeConfig_FlattenedColors_Mixed(t *testing.T) {
	// Mix of flat and nested keys
	cfg := ThemeConfig{
		Colors: map[string]any{
			"spinner": "#AABBCC", // Flat (no dots)
			"text": map[string]any{
				"primary": "#FF0000",
			},
		},
	}
	result := cfg.FlattenedColors()
	require.Len(t, result, 2)
	require.Equal(t, "#AABBCC", result["spinner"])
	require.Equal(t, "#FF0000", result["text.primary"])
}

func TestThemeConfig_FlattenedColors_MapAnyAny(t *testing.T) {
	// YAML sometimes produces map[any]any - should be handled
	cfg := ThemeConfig{
		Colors: map[string]any{
			"text": map[any]any{
				"primary": "#FF0000",
			},
		},
	}
	result := cfg.FlattenedColors()
	require.Len(t, result, 1)
	require.Equal(t, "#FF0000", result["text.primary"])
}

// Tests for DisableWorktrees config field

func TestOrchestrationConfig_DisableWorktrees_Default(t *testing.T) {
	// Verify Defaults() returns false for DisableWorktrees (Go's zero value)
	cfg := Defaults()
	require.False(t, cfg.Orchestration.DisableWorktrees, "DisableWorktrees should be false by default")
}

func TestOrchestrationConfig_DisableWorktrees_Explicit(t *testing.T) {
	// Verify explicit true value is preserved when set
	cfg := OrchestrationConfig{
		Client:           "claude",
		DisableWorktrees: true,
	}
	require.True(t, cfg.DisableWorktrees, "DisableWorktrees should preserve explicit true value")
}

func TestOrchestrationConfig_DisableWorktrees_ZeroValue(t *testing.T) {
	// Test that zero value OrchestrationConfig has DisableWorktrees as false
	cfg := OrchestrationConfig{}
	require.False(t, cfg.DisableWorktrees, "DisableWorktrees zero value should be false")
}

// Tests for TracingConfig

func TestTracingConfig_Defaults(t *testing.T) {
	cfg := Defaults()
	tracing := cfg.Orchestration.Tracing

	require.False(t, tracing.Enabled, "Tracing should be disabled by default")
	require.Equal(t, "file", tracing.Exporter, "Default exporter should be 'file'")
	require.Empty(t, tracing.FilePath, "FilePath should be empty in defaults (derived at runtime)")
	require.Equal(t, "localhost:4317", tracing.OTLPEndpoint, "Default OTLP endpoint should be localhost:4317")
	require.Equal(t, 1.0, tracing.SampleRate, "Default sample rate should be 1.0")
}

func TestValidateTracing_Empty(t *testing.T) {
	// Empty config should be valid (disabled by default)
	err := ValidateTracing(TracingConfig{})
	require.NoError(t, err)
}

func TestValidateTracing_DisabledWithEmptyFilePath(t *testing.T) {
	// Disabled tracing with file exporter but no file path is valid
	cfg := TracingConfig{
		Enabled:  false,
		Exporter: "file",
		FilePath: "",
	}
	err := ValidateTracing(cfg)
	require.NoError(t, err)
}

func TestValidateTracing_EnabledFileExporter_RequiresFilePath(t *testing.T) {
	cfg := TracingConfig{
		Enabled:    true,
		Exporter:   "file",
		FilePath:   "",
		SampleRate: 1.0,
	}
	err := ValidateTracing(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "file_path is required when exporter is \"file\"")
}

func TestValidateTracing_EnabledFileExporter_ValidWithFilePath(t *testing.T) {
	cfg := TracingConfig{
		Enabled:    true,
		Exporter:   "file",
		FilePath:   "/tmp/traces.jsonl",
		SampleRate: 1.0,
	}
	err := ValidateTracing(cfg)
	require.NoError(t, err)
}

func TestValidateTracing_EnabledOTLPExporter_RequiresEndpoint(t *testing.T) {
	cfg := TracingConfig{
		Enabled:      true,
		Exporter:     "otlp",
		OTLPEndpoint: "",
		SampleRate:   1.0,
	}
	err := ValidateTracing(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "otlp_endpoint is required when exporter is \"otlp\"")
}

func TestValidateTracing_EnabledOTLPExporter_ValidWithEndpoint(t *testing.T) {
	cfg := TracingConfig{
		Enabled:      true,
		Exporter:     "otlp",
		OTLPEndpoint: "localhost:4317",
		SampleRate:   1.0,
	}
	err := ValidateTracing(cfg)
	require.NoError(t, err)
}

func TestValidateTracing_InvalidSampleRate_TooLow(t *testing.T) {
	cfg := TracingConfig{
		SampleRate: -0.1,
	}
	err := ValidateTracing(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "sample_rate must be between 0.0 and 1.0")
}

func TestValidateTracing_InvalidSampleRate_TooHigh(t *testing.T) {
	cfg := TracingConfig{
		SampleRate: 1.5,
	}
	err := ValidateTracing(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "sample_rate must be between 0.0 and 1.0")
}

func TestValidateTracing_ValidSampleRate_Boundaries(t *testing.T) {
	// Test boundary values
	testCases := []float64{0.0, 0.5, 1.0}
	for _, rate := range testCases {
		cfg := TracingConfig{
			SampleRate: rate,
		}
		err := ValidateTracing(cfg)
		require.NoError(t, err, "sample rate %v should be valid", rate)
	}
}

func TestValidateTracing_InvalidExporter(t *testing.T) {
	cfg := TracingConfig{
		Exporter: "invalid",
	}
	err := ValidateTracing(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exporter must be \"none\", \"file\", \"stdout\", or \"otlp\"")
}

func TestValidateTracing_ValidExporters(t *testing.T) {
	exporters := []string{"none", "file", "stdout", "otlp"}
	for _, exporter := range exporters {
		cfg := TracingConfig{
			Enabled:      false, // Disable to avoid path requirements
			Exporter:     exporter,
			SampleRate:   1.0,
			FilePath:     "/tmp/traces.jsonl", // Provide just in case
			OTLPEndpoint: "localhost:4317",
		}
		err := ValidateTracing(cfg)
		require.NoError(t, err, "exporter %q should be valid", exporter)
	}
}

func TestValidateTracing_StdoutExporter_NoPathRequired(t *testing.T) {
	cfg := TracingConfig{
		Enabled:    true,
		Exporter:   "stdout",
		SampleRate: 1.0,
	}
	err := ValidateTracing(cfg)
	require.NoError(t, err)
}

func TestValidateTracing_NoneExporter_NoPathRequired(t *testing.T) {
	cfg := TracingConfig{
		Enabled:    true,
		Exporter:   "none",
		SampleRate: 1.0,
	}
	err := ValidateTracing(cfg)
	require.NoError(t, err)
}

func TestValidateOrchestration_WithValidTracing(t *testing.T) {
	cfg := OrchestrationConfig{
		Client: "claude",
		Tracing: TracingConfig{
			Enabled:    true,
			Exporter:   "file",
			FilePath:   "/tmp/traces.jsonl",
			SampleRate: 1.0,
		},
	}
	err := ValidateOrchestration(cfg)
	require.NoError(t, err)
}

func TestValidateOrchestration_WithInvalidTracing(t *testing.T) {
	cfg := OrchestrationConfig{
		Client: "claude",
		Tracing: TracingConfig{
			Enabled:    true,
			Exporter:   "file",
			FilePath:   "", // Missing required file path
			SampleRate: 1.0,
		},
	}
	err := ValidateOrchestration(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "file_path is required")
}

func TestDefaultTracesFilePath(t *testing.T) {
	// Just verify it returns a non-empty path (actual path depends on system)
	path := DefaultTracesFilePath()
	require.NotEmpty(t, path, "DefaultTracesFilePath should return a path")
	require.Contains(t, path, "traces.jsonl", "Path should contain traces.jsonl")
	require.Contains(t, path, "perles", "Path should contain perles")
}

func TestTracingConfig_ZeroValue(t *testing.T) {
	// Test that zero value TracingConfig has expected defaults
	cfg := TracingConfig{}
	require.False(t, cfg.Enabled, "Enabled zero value should be false")
	require.Empty(t, cfg.Exporter, "Exporter zero value should be empty")
	require.Empty(t, cfg.FilePath, "FilePath zero value should be empty")
	require.Empty(t, cfg.OTLPEndpoint, "OTLPEndpoint zero value should be empty")
	require.Equal(t, 0.0, cfg.SampleRate, "SampleRate zero value should be 0.0")
}

func TestOrchestrationConfig_TracingField(t *testing.T) {
	// Verify OrchestrationConfig includes Tracing field
	cfg := OrchestrationConfig{
		Client: "claude",
		Tracing: TracingConfig{
			Enabled:    true,
			Exporter:   "stdout",
			SampleRate: 0.5,
		},
	}
	require.True(t, cfg.Tracing.Enabled)
	require.Equal(t, "stdout", cfg.Tracing.Exporter)
	require.Equal(t, 0.5, cfg.Tracing.SampleRate)
}

// Tests for SessionStorageConfig

func TestDefaultSessionStorageBaseDir(t *testing.T) {
	// Verify it returns a path containing .perles/sessions
	path := DefaultSessionStorageBaseDir()
	require.NotEmpty(t, path, "DefaultSessionStorageBaseDir should return a path")
	require.Contains(t, path, ".perles", "Path should contain .perles")
	require.Contains(t, path, "sessions", "Path should contain sessions")
}

func TestSessionStorageConfig_Defaults(t *testing.T) {
	cfg := Defaults()
	storage := cfg.Orchestration.SessionStorage

	// BaseDir should be set to the default
	require.NotEmpty(t, storage.BaseDir, "BaseDir should be set in defaults")
	require.Contains(t, storage.BaseDir, ".perles", "Default BaseDir should contain .perles")
	require.Contains(t, storage.BaseDir, "sessions", "Default BaseDir should contain sessions")

	// ApplicationName should be empty (derived at runtime)
	require.Empty(t, storage.ApplicationName, "ApplicationName should be empty in defaults")
}

func TestSessionStorageConfig_ZeroValue(t *testing.T) {
	cfg := SessionStorageConfig{}
	require.Empty(t, cfg.BaseDir, "BaseDir zero value should be empty")
	require.Empty(t, cfg.ApplicationName, "ApplicationName zero value should be empty")
}

func TestValidateSessionStorage_Empty(t *testing.T) {
	// Empty config should be valid (uses defaults)
	err := ValidateSessionStorage(SessionStorageConfig{})
	require.NoError(t, err)
}

func TestValidateSessionStorage_AbsoluteBaseDir(t *testing.T) {
	// Use a platform-appropriate absolute path
	absPath := "/home/user/.perles/sessions"
	if runtime.GOOS == "windows" {
		absPath = `C:\Users\user\.perles\sessions`
	}
	cfg := SessionStorageConfig{
		BaseDir: absPath,
	}
	err := ValidateSessionStorage(cfg)
	require.NoError(t, err)
}

func TestValidateSessionStorage_RelativeBaseDir(t *testing.T) {
	cfg := SessionStorageConfig{
		BaseDir: "relative/path/sessions",
	}
	err := ValidateSessionStorage(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be an absolute path")
}

func TestValidateSessionStorage_WithApplicationName(t *testing.T) {
	// Use a platform-appropriate absolute path
	absPath := "/home/user/.perles/sessions"
	if runtime.GOOS == "windows" {
		absPath = `C:\Users\user\.perles\sessions`
	}
	cfg := SessionStorageConfig{
		BaseDir:         absPath,
		ApplicationName: "my-project",
	}
	err := ValidateSessionStorage(cfg)
	require.NoError(t, err)
}

func TestValidateOrchestration_WithValidSessionStorage(t *testing.T) {
	// Use a platform-appropriate absolute path
	absPath := "/home/user/.perles/sessions"
	if runtime.GOOS == "windows" {
		absPath = `C:\Users\user\.perles\sessions`
	}
	cfg := OrchestrationConfig{
		Client: "claude",
		SessionStorage: SessionStorageConfig{
			BaseDir:         absPath,
			ApplicationName: "test-app",
		},
	}
	err := ValidateOrchestration(cfg)
	require.NoError(t, err)
}

func TestValidateOrchestration_WithInvalidSessionStorage(t *testing.T) {
	cfg := OrchestrationConfig{
		Client: "claude",
		SessionStorage: SessionStorageConfig{
			BaseDir: "relative/path", // Invalid: not absolute
		},
	}
	err := ValidateOrchestration(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be an absolute path")
}

func TestOrchestrationConfig_SessionStorageField(t *testing.T) {
	// Verify OrchestrationConfig includes SessionStorage field
	cfg := OrchestrationConfig{
		Client: "claude",
		SessionStorage: SessionStorageConfig{
			BaseDir:         "/custom/path/sessions",
			ApplicationName: "custom-app",
		},
	}
	require.Equal(t, "/custom/path/sessions", cfg.SessionStorage.BaseDir)
	require.Equal(t, "custom-app", cfg.SessionStorage.ApplicationName)
}

func TestSessionStorageConfig_CustomBaseDirPreserved(t *testing.T) {
	// Test that a custom BaseDir is preserved (simulating config file loading)
	cfg := SessionStorageConfig{
		BaseDir: "/custom/sessions/path",
	}
	require.Equal(t, "/custom/sessions/path", cfg.BaseDir)
}

func TestSessionStorageConfig_ApplicationNameOverridePreserved(t *testing.T) {
	// Test that ApplicationName override is preserved
	cfg := SessionStorageConfig{
		BaseDir:         "/home/user/.perles/sessions",
		ApplicationName: "my-custom-app-name",
	}
	require.Equal(t, "my-custom-app-name", cfg.ApplicationName)
}

// Tests for SoundConfig

func TestConfig_SoundDefaults(t *testing.T) {
	cfg := Defaults()

	// Verify Sound field exists and has Events map
	require.NotNil(t, cfg.Sound.Events, "Events map should not be nil")

	// Verify all six sound events exist and are disabled by default
	require.False(t, cfg.Sound.Events["review_verdict_approve"].Enabled, "review_verdict_approve should be disabled by default")
	require.False(t, cfg.Sound.Events["review_verdict_deny"].Enabled, "review_verdict_deny should be disabled by default")
	require.False(t, cfg.Sound.Events["chat_welcome"].Enabled, "chat_welcome should be disabled by default")
	require.False(t, cfg.Sound.Events["workflow_complete"].Enabled, "workflow_complete should be disabled by default")
	require.False(t, cfg.Sound.Events["orchestration_welcome"].Enabled, "orchestration_welcome should be disabled by default")
	require.False(t, cfg.Sound.Events["worker_out_of_context"].Enabled, "worker_out_of_context should be disabled by default")
}

func TestConfig_LoadSoundConfig(t *testing.T) {
	// Test that SoundConfig can be created with explicit values
	cfg := SoundConfig{
		Events: map[string]SoundEventConfig{
			"review_verdict_approve": {Enabled: true},
			"review_verdict_deny":    {Enabled: false},
			"custom_sound":           {Enabled: true},
		},
	}

	require.True(t, cfg.Events["review_verdict_approve"].Enabled)
	require.False(t, cfg.Events["review_verdict_deny"].Enabled)
	require.True(t, cfg.Events["custom_sound"].Enabled)
}

func TestConfig_EnableSpecificSound(t *testing.T) {
	// Start with defaults (sounds disabled)
	cfg := Defaults()

	// Verify initial state - all sounds disabled by default
	require.False(t, cfg.Sound.Events["review_verdict_approve"].Enabled)
	require.False(t, cfg.Sound.Events["review_verdict_deny"].Enabled)

	// Enable one specific sound
	cfg.Sound.Events["review_verdict_approve"] = SoundEventConfig{Enabled: true}

	// Verify only the specific sound is enabled
	require.True(t, cfg.Sound.Events["review_verdict_approve"].Enabled, "review_verdict_approve should be enabled")
	require.False(t, cfg.Sound.Events["review_verdict_deny"].Enabled, "review_verdict_deny should remain disabled")
}

func TestSoundConfig_ZeroValue(t *testing.T) {
	// Test that zero value SoundConfig has nil Events
	cfg := SoundConfig{}
	require.Nil(t, cfg.Events, "Events zero value should be nil")
}

func TestSoundConfig_EmptyMap(t *testing.T) {
	// Test that SoundConfig can have an empty Events map (all sounds disabled)
	cfg := SoundConfig{
		Events: map[string]SoundEventConfig{},
	}
	require.NotNil(t, cfg.Events)
	require.Empty(t, cfg.Events)
}

func TestConfig_SoundField(t *testing.T) {
	// Verify Config includes Sound field
	cfg := Config{
		Sound: SoundConfig{
			Events: map[string]SoundEventConfig{
				"test_sound": {Enabled: true},
			},
		},
	}
	require.True(t, cfg.Sound.Events["test_sound"].Enabled)
}

// Tests for SoundEventConfig struct

func TestSoundEventConfig_WithOverrideSounds(t *testing.T) {
	// Test that SoundEventConfig can have multiple override sounds
	cfg := SoundEventConfig{
		Enabled: true,
		OverrideSounds: []string{
			"~/.config/perles/sounds/custom1.wav",
			"~/.config/perles/sounds/custom2.wav",
		},
	}
	require.True(t, cfg.Enabled)
	require.Len(t, cfg.OverrideSounds, 2)
	require.Equal(t, "~/.config/perles/sounds/custom1.wav", cfg.OverrideSounds[0])
	require.Equal(t, "~/.config/perles/sounds/custom2.wav", cfg.OverrideSounds[1])
}

func TestSoundEventConfig_DisabledWithOverrides(t *testing.T) {
	// Test that enabled=false works even with override sounds configured
	cfg := SoundEventConfig{
		Enabled:        false,
		OverrideSounds: []string{"~/.config/perles/sounds/custom.wav"},
	}
	require.False(t, cfg.Enabled)
	require.Len(t, cfg.OverrideSounds, 1)
}

func TestSoundEventConfig_EnabledNoOverrides(t *testing.T) {
	// Test that enabled=true with empty overrides uses default embedded sound
	cfg := SoundEventConfig{
		Enabled:        true,
		OverrideSounds: nil,
	}
	require.True(t, cfg.Enabled)
	require.Nil(t, cfg.OverrideSounds)
}

func TestSoundConfig_FullConfig(t *testing.T) {
	// Test complete SoundConfig with all three events and various override configurations
	cfg := SoundConfig{
		Events: map[string]SoundEventConfig{
			"chat_welcome": {
				Enabled: true,
				OverrideSounds: []string{
					"~/.config/perles/sounds/welcome1.wav",
					"~/.config/perles/sounds/welcome2.wav",
				},
			},
			"review_verdict_approve": {
				Enabled:        true,
				OverrideSounds: nil, // Uses embedded default
			},
			"review_verdict_deny": {
				Enabled: false, // Disabled entirely
			},
		},
	}

	// Verify chat_welcome
	require.True(t, cfg.Events["chat_welcome"].Enabled)
	require.Len(t, cfg.Events["chat_welcome"].OverrideSounds, 2)

	// Verify review_verdict_approve
	require.True(t, cfg.Events["review_verdict_approve"].Enabled)
	require.Nil(t, cfg.Events["review_verdict_approve"].OverrideSounds)

	// Verify review_verdict_deny
	require.False(t, cfg.Events["review_verdict_deny"].Enabled)
}

func TestDefaults_SoundEventConfigValues(t *testing.T) {
	// Verify Defaults() returns correct SoundEventConfig values for all events
	cfg := Defaults()

	// All events should exist in the map
	require.Len(t, cfg.Sound.Events, 7)

	// Check each event has correct default values
	for _, eventName := range []string{"review_verdict_approve", "review_verdict_deny", "chat_welcome", "workflow_complete", "orchestration_welcome", "worker_out_of_context", "user_notification"} {
		eventConfig, exists := cfg.Sound.Events[eventName]
		require.True(t, exists, "Event %q should exist in defaults", eventName)
		require.False(t, eventConfig.Enabled, "Event %q should be disabled by default", eventName)
		require.Nil(t, eventConfig.OverrideSounds, "Event %q should have nil OverrideSounds by default", eventName)
	}
}

// Tests for ValidateSound

func TestValidateSound_NilEvents(t *testing.T) {
	// Empty/nil config should be valid
	err := ValidateSound(SoundConfig{Events: nil})
	require.NoError(t, err)
}

func TestValidateSound_EmptyOverrideSounds(t *testing.T) {
	// Config with empty override_sounds list should pass
	cfg := SoundConfig{
		Events: map[string]SoundEventConfig{
			"chat_welcome": {
				Enabled:        true,
				OverrideSounds: []string{},
			},
		},
	}
	err := ValidateSound(cfg)
	require.NoError(t, err)
}

func TestValidateSound_ValidPathUnderSecurityBoundary(t *testing.T) {
	// Create a temp directory structure that mimics ~/.config/perles/sounds/
	tempDir := t.TempDir()
	soundsDir := filepath.Join(tempDir, ".config", "perles", "sounds")
	require.NoError(t, os.MkdirAll(soundsDir, 0o755))

	// Create a valid WAV file (small file under 1MB)
	wavFile := filepath.Join(soundsDir, "test.wav")
	// Write minimal WAV header + some data (valid header not required for this test)
	require.NoError(t, os.WriteFile(wavFile, make([]byte, 100), 0o644))

	// Use a custom boundary for testing
	err := validateSoundPath(wavFile, "test_event", 0, soundsDir)
	require.NoError(t, err)
}

func TestValidateSound_PathOutsideSecurityBoundary(t *testing.T) {
	// Create two separate directories
	tempDir := t.TempDir()
	soundsDir := filepath.Join(tempDir, "sounds")
	outsideDir := filepath.Join(tempDir, "outside")
	require.NoError(t, os.MkdirAll(soundsDir, 0o755))
	require.NoError(t, os.MkdirAll(outsideDir, 0o755))

	// Create a WAV file outside the security boundary
	wavFile := filepath.Join(outsideDir, "test.wav")
	require.NoError(t, os.WriteFile(wavFile, make([]byte, 100), 0o644))

	// Validate should fail because path is outside boundary
	err := validateSoundPath(wavFile, "test_event", 0, soundsDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "path must be under")
}

func TestValidateSound_PathTraversalRejected(t *testing.T) {
	// Create directory structure
	tempDir := t.TempDir()
	soundsDir := filepath.Join(tempDir, "sounds")
	outsideDir := filepath.Join(tempDir, "outside")
	require.NoError(t, os.MkdirAll(soundsDir, 0o755))
	require.NoError(t, os.MkdirAll(outsideDir, 0o755))

	// Create a WAV file outside the boundary
	wavFile := filepath.Join(outsideDir, "secret.wav")
	require.NoError(t, os.WriteFile(wavFile, make([]byte, 100), 0o644))

	// Try to access via path traversal (../../../etc/passwd pattern)
	traversalPath := filepath.Join(soundsDir, "..", "outside", "secret.wav")

	err := validateSoundPath(traversalPath, "test_event", 0, soundsDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "path must be under")
}

func TestValidateSound_SymlinkOutsideBoundaryRejected(t *testing.T) {
	// Skip on Windows as symlinks require elevated privileges
	if runtime.GOOS == "windows" {
		t.Skip("Skipping symlink test on Windows")
	}

	// Create directory structure
	tempDir := t.TempDir()
	soundsDir := filepath.Join(tempDir, "sounds")
	outsideDir := filepath.Join(tempDir, "outside")
	require.NoError(t, os.MkdirAll(soundsDir, 0o755))
	require.NoError(t, os.MkdirAll(outsideDir, 0o755))

	// Create a WAV file outside the boundary
	realFile := filepath.Join(outsideDir, "secret.wav")
	require.NoError(t, os.WriteFile(realFile, make([]byte, 100), 0o644))

	// Create a symlink inside the boundary pointing to the file outside
	symlinkPath := filepath.Join(soundsDir, "link.wav")
	require.NoError(t, os.Symlink(realFile, symlinkPath))

	// Validation should reject because the real path is outside boundary
	err := validateSoundPath(symlinkPath, "test_event", 0, soundsDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "path must be under")
}

func TestValidateSound_NonWAVExtensionRejected(t *testing.T) {
	tempDir := t.TempDir()
	soundsDir := filepath.Join(tempDir, "sounds")
	require.NoError(t, os.MkdirAll(soundsDir, 0o755))

	// Test various non-WAV extensions
	extensions := []string{".mp3", ".ogg", ".flac", ".m4a", ".aac", ".txt"}
	for _, ext := range extensions {
		t.Run(ext, func(t *testing.T) {
			testFile := filepath.Join(soundsDir, "test"+ext)
			require.NoError(t, os.WriteFile(testFile, make([]byte, 100), 0o644))

			err := validateSoundPath(testFile, "test_event", 0, soundsDir)
			require.Error(t, err)
			require.Contains(t, err.Error(), "only WAV format is supported")
		})
	}
}

func TestValidateSound_MissingFileRejected(t *testing.T) {
	tempDir := t.TempDir()
	soundsDir := filepath.Join(tempDir, "sounds")
	require.NoError(t, os.MkdirAll(soundsDir, 0o755))

	// Reference a file that doesn't exist
	missingFile := filepath.Join(soundsDir, "nonexistent.wav")

	err := validateSoundPath(missingFile, "test_event", 0, soundsDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "file not found")
}

func TestValidateSound_FileOver1MBRejected(t *testing.T) {
	tempDir := t.TempDir()
	soundsDir := filepath.Join(tempDir, "sounds")
	require.NoError(t, os.MkdirAll(soundsDir, 0o755))

	// Create a file larger than 1MB
	largeFile := filepath.Join(soundsDir, "large.wav")
	largeData := make([]byte, maxSoundFileSize+1)
	require.NoError(t, os.WriteFile(largeFile, largeData, 0o644))

	err := validateSoundPath(largeFile, "test_event", 0, soundsDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "file too large")
}

func TestValidateSound_CaseInsensitiveExtension(t *testing.T) {
	tempDir := t.TempDir()
	soundsDir := filepath.Join(tempDir, "sounds")
	require.NoError(t, os.MkdirAll(soundsDir, 0o755))

	// Test various case variations of .wav extension
	extensions := []string{".WAV", ".Wav", ".waV", ".WaV"}
	for _, ext := range extensions {
		t.Run(ext, func(t *testing.T) {
			testFile := filepath.Join(soundsDir, "test"+ext)
			require.NoError(t, os.WriteFile(testFile, make([]byte, 100), 0o644))

			err := validateSoundPath(testFile, "test_event", 0, soundsDir)
			require.NoError(t, err)
		})
	}
}

func TestValidateSound_ExactlyMaxSizePasses(t *testing.T) {
	tempDir := t.TempDir()
	soundsDir := filepath.Join(tempDir, "sounds")
	require.NoError(t, os.MkdirAll(soundsDir, 0o755))

	// Create a file exactly at the size limit
	maxFile := filepath.Join(soundsDir, "maxsize.wav")
	maxData := make([]byte, maxSoundFileSize)
	require.NoError(t, os.WriteFile(maxFile, maxData, 0o644))

	err := validateSoundPath(maxFile, "test_event", 0, soundsDir)
	require.NoError(t, err)
}

func TestValidateSound_IntegrationWithSoundConfig(t *testing.T) {
	// Test the full ValidateSound function with a SoundConfig
	// Events with no override sounds should always pass validation
	cfg := SoundConfig{
		Events: map[string]SoundEventConfig{
			"chat_welcome": {
				Enabled:        true,
				OverrideSounds: nil, // No overrides, uses embedded
			},
			"review_verdict_approve": {
				Enabled:        true,
				OverrideSounds: []string{}, // Empty list, uses embedded
			},
			"review_verdict_deny": {
				Enabled: false,
			},
		},
	}

	// This should pass because there are no override sounds to validate
	err := ValidateSound(cfg)
	require.NoError(t, err)
}

func TestValidateSound_MultipleEventsWithErrors(t *testing.T) {
	tempDir := t.TempDir()
	soundsDir := filepath.Join(tempDir, "sounds")
	require.NoError(t, os.MkdirAll(soundsDir, 0o755))

	// Create one valid and one invalid file
	validFile := filepath.Join(soundsDir, "valid.wav")
	require.NoError(t, os.WriteFile(validFile, make([]byte, 100), 0o644))

	invalidFile := filepath.Join(soundsDir, "invalid.mp3")
	require.NoError(t, os.WriteFile(invalidFile, make([]byte, 100), 0o644))

	// Test that validation catches invalid file in any event
	err := validateSoundPath(invalidFile, "chat_welcome", 0, soundsDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "only WAV format is supported")
	require.Contains(t, err.Error(), "chat_welcome")
}

func TestSoundSecurityBoundary(t *testing.T) {
	// Test that SoundSecurityBoundary returns the expected path
	boundary := SoundSecurityBoundary()

	// Should be non-empty (unless home dir is unavailable)
	if home, err := os.UserHomeDir(); err == nil {
		expected := filepath.Join(home, ".perles", "sounds")
		require.Equal(t, expected, boundary)
	}
}

// Tests for TimeoutsConfig

func TestDefaultTimeoutsConfig(t *testing.T) {
	cfg := DefaultTimeoutsConfig()

	require.Equal(t, 30*time.Second, cfg.WorktreeCreation, "WorktreeCreation should be 30s")
	require.Equal(t, 60*time.Second, cfg.CoordinatorStart, "CoordinatorStart should be 60s")
	require.Equal(t, 30*time.Second, cfg.WorkspaceSetup, "WorkspaceSetup should be 30s")
	require.Equal(t, 120*time.Second, cfg.MaxTotal, "MaxTotal should be 120s")
}

func TestTimeoutsConfig_ZeroValue(t *testing.T) {
	// Test that zero value TimeoutsConfig has expected defaults (all zero durations)
	cfg := TimeoutsConfig{}
	require.Equal(t, time.Duration(0), cfg.WorktreeCreation, "WorktreeCreation zero value should be 0")
	require.Equal(t, time.Duration(0), cfg.CoordinatorStart, "CoordinatorStart zero value should be 0")
	require.Equal(t, time.Duration(0), cfg.WorkspaceSetup, "WorkspaceSetup zero value should be 0")
	require.Equal(t, time.Duration(0), cfg.MaxTotal, "MaxTotal zero value should be 0")
}

func TestDefaults_Timeouts(t *testing.T) {
	cfg := Defaults()

	require.Equal(t, 30*time.Second, cfg.Orchestration.Timeouts.WorktreeCreation, "WorktreeCreation should be 30s")
	require.Equal(t, 60*time.Second, cfg.Orchestration.Timeouts.CoordinatorStart, "CoordinatorStart should be 60s")
	require.Equal(t, 30*time.Second, cfg.Orchestration.Timeouts.WorkspaceSetup, "WorkspaceSetup should be 30s")
	require.Equal(t, 120*time.Second, cfg.Orchestration.Timeouts.MaxTotal, "MaxTotal should be 120s")
}

func TestOrchestrationConfig_TimeoutsField(t *testing.T) {
	// Verify OrchestrationConfig includes Timeouts field
	cfg := OrchestrationConfig{
		Client: "claude",
		Timeouts: TimeoutsConfig{
			WorktreeCreation: 45 * time.Second,
			CoordinatorStart: 90 * time.Second,
			WorkspaceSetup:   15 * time.Second,
			MaxTotal:         180 * time.Second,
		},
	}
	require.Equal(t, 45*time.Second, cfg.Timeouts.WorktreeCreation)
	require.Equal(t, 90*time.Second, cfg.Timeouts.CoordinatorStart)
	require.Equal(t, 15*time.Second, cfg.Timeouts.WorkspaceSetup)
	require.Equal(t, 180*time.Second, cfg.Timeouts.MaxTotal)
}

func TestTimeoutsConfig_PartialValues(t *testing.T) {
	// Test that partial values are preserved (no automatic defaulting in struct)
	cfg := TimeoutsConfig{
		WorktreeCreation: 45 * time.Second,
		// Other fields left as zero
	}
	require.Equal(t, 45*time.Second, cfg.WorktreeCreation)
	require.Equal(t, time.Duration(0), cfg.CoordinatorStart)
	require.Equal(t, time.Duration(0), cfg.WorkspaceSetup)
	require.Equal(t, time.Duration(0), cfg.MaxTotal)
}

func TestTimeoutsConfig_VariousDurationFormats(t *testing.T) {
	// Test that various duration formats work correctly
	testCases := []struct {
		name     string
		duration time.Duration
		expected time.Duration
	}{
		{"30 seconds", 30 * time.Second, 30 * time.Second},
		{"1 minute", 1 * time.Minute, 60 * time.Second},
		{"90 seconds", 90 * time.Second, 90 * time.Second},
		{"2 minutes", 2 * time.Minute, 120 * time.Second},
		{"500 milliseconds", 500 * time.Millisecond, 500 * time.Millisecond},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := TimeoutsConfig{
				WorktreeCreation: tc.duration,
			}
			require.Equal(t, tc.expected, cfg.WorktreeCreation)
		})
	}
}

func TestTimeoutsConfig_CustomValuesPreserved(t *testing.T) {
	// Test that custom values are preserved (simulating config file loading)
	cfg := TimeoutsConfig{
		WorktreeCreation: 45 * time.Second,
		CoordinatorStart: 90 * time.Second,
		WorkspaceSetup:   15 * time.Second,
		MaxTotal:         180 * time.Second,
	}
	require.Equal(t, 45*time.Second, cfg.WorktreeCreation)
	require.Equal(t, 90*time.Second, cfg.CoordinatorStart)
	require.Equal(t, 15*time.Second, cfg.WorkspaceSetup)
	require.Equal(t, 180*time.Second, cfg.MaxTotal)
}

// ============================================================================
// CoordinatorClientType Tests
// ============================================================================

func TestCoordinatorClientType_EmptyConfig(t *testing.T) {
	cfg := OrchestrationConfig{}
	require.Equal(t, client.ClientClaude, cfg.CoordinatorClientType(), "empty config should default to claude")
}

func TestCoordinatorClientType_OnlyClient(t *testing.T) {
	cfg := OrchestrationConfig{Client: "amp"}
	require.Equal(t, client.ClientType("amp"), cfg.CoordinatorClientType(), "should use Client when CoordinatorClient is empty")
}

func TestCoordinatorClientType_CoordinatorClientOverridesClient(t *testing.T) {
	cfg := OrchestrationConfig{
		Client:            "amp",
		CoordinatorClient: "codex",
	}
	require.Equal(t, client.ClientType("codex"), cfg.CoordinatorClientType(), "CoordinatorClient should override Client")
}

func TestCoordinatorClientType_AllClients(t *testing.T) {
	clients := []string{"claude", "amp", "codex", "gemini", "opencode"}
	for _, c := range clients {
		t.Run(c, func(t *testing.T) {
			cfg := OrchestrationConfig{CoordinatorClient: c}
			require.Equal(t, client.ClientType(c), cfg.CoordinatorClientType())
		})
	}
}

// ============================================================================
// WorkerClientType Tests
// ============================================================================

func TestWorkerClientType_EmptyConfig(t *testing.T) {
	cfg := OrchestrationConfig{}
	require.Equal(t, client.ClientClaude, cfg.WorkerClientType(), "empty config should default to claude")
}

func TestWorkerClientType_OnlyClient(t *testing.T) {
	cfg := OrchestrationConfig{Client: "amp"}
	require.Equal(t, client.ClientType("amp"), cfg.WorkerClientType(), "should use Client when WorkerClient is empty")
}

func TestWorkerClientType_WorkerClientOverridesClient(t *testing.T) {
	cfg := OrchestrationConfig{
		Client:       "amp",
		WorkerClient: "codex",
	}
	require.Equal(t, client.ClientType("codex"), cfg.WorkerClientType(), "WorkerClient should override Client")
}

func TestWorkerClientType_AllClients(t *testing.T) {
	clients := []string{"claude", "amp", "codex", "gemini", "opencode"}
	for _, c := range clients {
		t.Run(c, func(t *testing.T) {
			cfg := OrchestrationConfig{WorkerClient: c}
			require.Equal(t, client.ClientType(c), cfg.WorkerClientType())
		})
	}
}

func TestClientType_CoordinatorAndWorkerIndependent(t *testing.T) {
	cfg := OrchestrationConfig{
		CoordinatorClient: "claude",
		WorkerClient:      "amp",
	}
	require.Equal(t, client.ClientType("claude"), cfg.CoordinatorClientType())
	require.Equal(t, client.ClientType("amp"), cfg.WorkerClientType())
}

func TestClientType_FallbackToClientForBoth(t *testing.T) {
	cfg := OrchestrationConfig{Client: "codex"}
	require.Equal(t, client.ClientType("codex"), cfg.CoordinatorClientType())
	require.Equal(t, client.ClientType("codex"), cfg.WorkerClientType())
}

// ============================================================================
// AgentProviders Tests
// ============================================================================

func TestAgentProviders_DefaultConfig(t *testing.T) {
	cfg := OrchestrationConfig{}
	providers := cfg.AgentProviders()

	require.NotNil(t, providers[client.RoleCoordinator])
	require.NotNil(t, providers[client.RoleWorker])
	require.Equal(t, client.ClientClaude, providers[client.RoleCoordinator].Type())
	require.Equal(t, client.ClientClaude, providers[client.RoleWorker].Type())
}

func TestAgentProviders_DifferentClients(t *testing.T) {
	cfg := OrchestrationConfig{
		CoordinatorClient: "claude",
		WorkerClient:      "amp",
		Claude:            ClaudeClientConfig{Model: "opus"},
		Amp:               AmpClientConfig{Model: "sonnet", Mode: "smart"},
	}
	providers := cfg.AgentProviders()

	require.Equal(t, client.ClientClaude, providers[client.RoleCoordinator].Type())
	require.Equal(t, client.ClientType("amp"), providers[client.RoleWorker].Type())
}

func TestAgentProviders_IncludesExtensions(t *testing.T) {
	cfg := OrchestrationConfig{
		CoordinatorClient: "claude",
		WorkerClient:      "claude",
		Claude:            ClaudeClientConfig{Model: "opus"},
	}
	providers := cfg.AgentProviders()

	coordExt := providers[client.RoleCoordinator].Extensions()
	workerExt := providers[client.RoleWorker].Extensions()

	require.Equal(t, "opus", coordExt["claude.model"])
	require.Equal(t, "opus", workerExt["claude.model"])
}

func TestAgentProviders_WorkerUsesWorkerConfig(t *testing.T) {
	cfg := OrchestrationConfig{
		CoordinatorClient: "claude",
		WorkerClient:      "claude",
		Claude:            ClaudeClientConfig{Model: "opus"},
		ClaudeWorker: ClaudeClientConfig{
			Model: "sonnet",
			Env:   map[string]string{"WORKER_KEY": "value"},
		},
	}
	providers := cfg.AgentProviders()

	coordExt := providers[client.RoleCoordinator].Extensions()
	workerExt := providers[client.RoleWorker].Extensions()

	require.Equal(t, "opus", coordExt["claude.model"])
	require.NotContains(t, coordExt, "claude.env")

	require.Equal(t, "sonnet", workerExt["claude.model"])
	require.Equal(t, map[string]string{"WORKER_KEY": "value"}, workerExt["claude.env"])
}

func TestAgentProviders_FallbackToClient(t *testing.T) {
	cfg := OrchestrationConfig{
		Client: "gemini",
		Gemini: GeminiClientConfig{Model: "gemini-2.5-flash"},
	}
	providers := cfg.AgentProviders()

	require.Equal(t, client.ClientType("gemini"), providers[client.RoleCoordinator].Type())
	require.Equal(t, client.ClientType("gemini"), providers[client.RoleWorker].Type())
	require.Equal(t, "gemini-2.5-flash", providers[client.RoleCoordinator].Extensions()["gemini.model"])
	require.Equal(t, "gemini-2.5-flash", providers[client.RoleWorker].Extensions()["gemini.model"])
}

// ============================================================================
// extensionsForClient Tests
// ============================================================================

func TestExtensionsForClient_Claude_EmptyConfig(t *testing.T) {
	cfg := OrchestrationConfig{}
	ext := cfg.extensionsForClient("claude", false)
	require.Empty(t, ext, "empty Claude config should return empty extensions")
}

func TestExtensionsForClient_Claude_WithModel(t *testing.T) {
	cfg := OrchestrationConfig{
		Claude: ClaudeClientConfig{Model: "sonnet"},
	}
	ext := cfg.extensionsForClient("claude", false)
	require.Equal(t, "sonnet", ext["claude.model"])
}

func TestExtensionsForClient_Claude_WithEnv(t *testing.T) {
	cfg := OrchestrationConfig{
		Claude: ClaudeClientConfig{
			Model: "opus",
			Env:   map[string]string{"API_KEY": "secret"},
		},
	}
	ext := cfg.extensionsForClient("claude", false)
	require.Equal(t, "opus", ext["claude.model"])
	require.Equal(t, map[string]string{"API_KEY": "secret"}, ext["claude.env"])
}

func TestExtensionsForClient_Claude_WorkerUsesMainConfig(t *testing.T) {
	cfg := OrchestrationConfig{
		Claude: ClaudeClientConfig{Model: "opus"},
	}
	ext := cfg.extensionsForClient("claude", true)
	require.Equal(t, "opus", ext["claude.model"])
}

func TestExtensionsForClient_Claude_WorkerUsesWorkerConfig(t *testing.T) {
	cfg := OrchestrationConfig{
		Claude: ClaudeClientConfig{Model: "opus"},
		ClaudeWorker: ClaudeClientConfig{
			Model: "sonnet",
			Env:   map[string]string{"WORKER_KEY": "worker-secret"},
		},
	}
	ext := cfg.extensionsForClient("claude", true)
	require.Equal(t, "sonnet", ext["claude.model"])
	require.Equal(t, map[string]string{"WORKER_KEY": "worker-secret"}, ext["claude.env"])
}

func TestExtensionsForClient_Claude_WorkerInheritsModelFromMain(t *testing.T) {
	cfg := OrchestrationConfig{
		Claude: ClaudeClientConfig{Model: "opus"},
		ClaudeWorker: ClaudeClientConfig{
			Model: "", // Empty - should inherit from main
			Env:   map[string]string{"WORKER_KEY": "value"},
		},
	}
	ext := cfg.extensionsForClient("claude", true)
	require.Equal(t, "opus", ext["claude.model"], "worker should inherit model from main claude config")
	require.Equal(t, map[string]string{"WORKER_KEY": "value"}, ext["claude.env"])
}

func TestExtensionsForClient_Claude_WorkerWithoutEnvUsesMainConfig(t *testing.T) {
	cfg := OrchestrationConfig{
		Claude: ClaudeClientConfig{
			Model: "opus",
			Env:   map[string]string{"MAIN_KEY": "main-value"},
		},
		ClaudeWorker: ClaudeClientConfig{
			Model: "sonnet",
			Env:   nil, // No env vars - should use main config
		},
	}
	ext := cfg.extensionsForClient("claude", true)
	require.Equal(t, "opus", ext["claude.model"], "should use main claude config when worker has no env")
	require.Equal(t, map[string]string{"MAIN_KEY": "main-value"}, ext["claude.env"])
}

func TestExtensionsForClient_Claude_CoordinatorIgnoresWorkerConfig(t *testing.T) {
	cfg := OrchestrationConfig{
		Claude: ClaudeClientConfig{Model: "opus"},
		ClaudeWorker: ClaudeClientConfig{
			Model: "sonnet",
			Env:   map[string]string{"WORKER_KEY": "worker-secret"},
		},
	}
	ext := cfg.extensionsForClient("claude", false)
	require.Equal(t, "opus", ext["claude.model"], "coordinator should use main claude config")
	require.NotContains(t, ext, "claude.env", "coordinator should not have worker env")
}

func TestExtensionsForClient_Codex_EmptyConfig(t *testing.T) {
	cfg := OrchestrationConfig{}
	ext := cfg.extensionsForClient("codex", false)
	require.Empty(t, ext, "empty Codex config should return empty extensions")
}

func TestExtensionsForClient_Codex_WithModel(t *testing.T) {
	cfg := OrchestrationConfig{
		Codex: CodexClientConfig{Model: "gpt-5.2-codex"},
	}
	ext := cfg.extensionsForClient("codex", false)
	require.Equal(t, "gpt-5.2-codex", ext["codex.model"])
}

func TestExtensionsForClient_Codex_WorkerSameAsCoordinator(t *testing.T) {
	cfg := OrchestrationConfig{
		Codex: CodexClientConfig{Model: "o4-mini"},
	}
	extCoord := cfg.extensionsForClient("codex", false)
	extWorker := cfg.extensionsForClient("codex", true)
	require.Equal(t, extCoord, extWorker, "Codex extensions should be same for coordinator and worker")
}

func TestExtensionsForClient_Amp_EmptyConfig(t *testing.T) {
	cfg := OrchestrationConfig{}
	ext := cfg.extensionsForClient("amp", false)
	require.Empty(t, ext, "empty Amp config should return empty extensions")
}

func TestExtensionsForClient_Amp_WithModel(t *testing.T) {
	cfg := OrchestrationConfig{
		Amp: AmpClientConfig{Model: "sonnet"},
	}
	ext := cfg.extensionsForClient("amp", false)
	require.Equal(t, "sonnet", ext["amp.model"])
}

func TestExtensionsForClient_Amp_WithMode(t *testing.T) {
	cfg := OrchestrationConfig{
		Amp: AmpClientConfig{Mode: "rush"},
	}
	ext := cfg.extensionsForClient("amp", false)
	require.Equal(t, "rush", ext["amp.mode"])
}

func TestExtensionsForClient_Amp_WithModelAndMode(t *testing.T) {
	cfg := OrchestrationConfig{
		Amp: AmpClientConfig{
			Model: "opus",
			Mode:  "smart",
		},
	}
	ext := cfg.extensionsForClient("amp", false)
	require.Equal(t, "opus", ext["amp.model"])
	require.Equal(t, "smart", ext["amp.mode"])
}

func TestExtensionsForClient_Amp_WorkerSameAsCoordinator(t *testing.T) {
	cfg := OrchestrationConfig{
		Amp: AmpClientConfig{Model: "opus", Mode: "smart"},
	}
	extCoord := cfg.extensionsForClient("amp", false)
	extWorker := cfg.extensionsForClient("amp", true)
	require.Equal(t, extCoord, extWorker, "Amp extensions should be same for coordinator and worker")
}

func TestExtensionsForClient_Gemini_EmptyConfig(t *testing.T) {
	cfg := OrchestrationConfig{}
	ext := cfg.extensionsForClient("gemini", false)
	require.Empty(t, ext, "empty Gemini config should return empty extensions")
}

func TestExtensionsForClient_Gemini_WithModel(t *testing.T) {
	cfg := OrchestrationConfig{
		Gemini: GeminiClientConfig{Model: "gemini-2.5-flash"},
	}
	ext := cfg.extensionsForClient("gemini", false)
	require.Equal(t, "gemini-2.5-flash", ext["gemini.model"])
}

func TestExtensionsForClient_Gemini_WorkerSameAsCoordinator(t *testing.T) {
	cfg := OrchestrationConfig{
		Gemini: GeminiClientConfig{Model: "gemini-3-pro-preview"},
	}
	extCoord := cfg.extensionsForClient("gemini", false)
	extWorker := cfg.extensionsForClient("gemini", true)
	require.Equal(t, extCoord, extWorker, "Gemini extensions should be same for coordinator and worker")
}

func TestExtensionsForClient_OpenCode_EmptyConfig(t *testing.T) {
	cfg := OrchestrationConfig{}
	ext := cfg.extensionsForClient("opencode", false)
	require.Empty(t, ext, "empty OpenCode config should return empty extensions")
}

func TestExtensionsForClient_OpenCode_WithModel(t *testing.T) {
	cfg := OrchestrationConfig{
		OpenCode: OpenCodeClientConfig{Model: "opencode/glm-4.8"},
	}
	ext := cfg.extensionsForClient("opencode", false)
	require.Equal(t, "opencode/glm-4.8", ext["opencode.model"])
}

func TestExtensionsForClient_OpenCode_WorkerSameAsCoordinator(t *testing.T) {
	cfg := OrchestrationConfig{
		OpenCode: OpenCodeClientConfig{Model: "anthropic/claude-opus-4-5"},
	}
	extCoord := cfg.extensionsForClient("opencode", false)
	extWorker := cfg.extensionsForClient("opencode", true)
	require.Equal(t, extCoord, extWorker, "OpenCode extensions should be same for coordinator and worker")
}

func TestExtensionsForClient_UnknownClient(t *testing.T) {
	cfg := OrchestrationConfig{
		Claude: ClaudeClientConfig{Model: "opus"},
		Codex:  CodexClientConfig{Model: "gpt-5.2-codex"},
	}
	ext := cfg.extensionsForClient("unknown-client", false)
	require.Empty(t, ext, "unknown client should return empty extensions")
}

func TestExtensionsForClient_ReturnsNewMap(t *testing.T) {
	cfg := OrchestrationConfig{
		Claude: ClaudeClientConfig{Model: "opus"},
	}
	ext1 := cfg.extensionsForClient("claude", false)
	ext2 := cfg.extensionsForClient("claude", false)
	ext1["new-key"] = "value"
	require.NotContains(t, ext2, "new-key", "should return independent maps")
}
