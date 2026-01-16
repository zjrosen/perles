package config

import (
	"runtime"
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

	// Verify Sound field exists and has EnabledSounds map
	require.NotNil(t, cfg.Sound.EnabledSounds, "EnabledSounds map should not be nil")

	// Verify both review_verdict sounds are disabled by default
	require.False(t, cfg.Sound.EnabledSounds["review_verdict_approve"], "review_verdict_approve should be disabled by default")
	require.False(t, cfg.Sound.EnabledSounds["review_verdict_deny"], "review_verdict_deny should be disabled by default")
}

func TestConfig_LoadSoundConfig(t *testing.T) {
	// Test that SoundConfig can be created with explicit values
	cfg := SoundConfig{
		EnabledSounds: map[string]bool{
			"review_verdict_approve": true,
			"review_verdict_deny":    false,
			"custom_sound":           true,
		},
	}

	require.True(t, cfg.EnabledSounds["review_verdict_approve"])
	require.False(t, cfg.EnabledSounds["review_verdict_deny"])
	require.True(t, cfg.EnabledSounds["custom_sound"])
}

func TestConfig_EnableSpecificSound(t *testing.T) {
	// Start with defaults (sounds disabled)
	cfg := Defaults()

	// Verify initial state - all sounds disabled by default
	require.False(t, cfg.Sound.EnabledSounds["review_verdict_approve"])
	require.False(t, cfg.Sound.EnabledSounds["review_verdict_deny"])

	// Enable one specific sound
	cfg.Sound.EnabledSounds["review_verdict_approve"] = true

	// Verify only the specific sound is enabled
	require.True(t, cfg.Sound.EnabledSounds["review_verdict_approve"], "review_verdict_approve should be enabled")
	require.False(t, cfg.Sound.EnabledSounds["review_verdict_deny"], "review_verdict_deny should remain disabled")
}

func TestSoundConfig_ZeroValue(t *testing.T) {
	// Test that zero value SoundConfig has nil EnabledSounds
	cfg := SoundConfig{}
	require.Nil(t, cfg.EnabledSounds, "EnabledSounds zero value should be nil")
}

func TestSoundConfig_EmptyMap(t *testing.T) {
	// Test that SoundConfig can have an empty map (all sounds disabled)
	cfg := SoundConfig{
		EnabledSounds: map[string]bool{},
	}
	require.NotNil(t, cfg.EnabledSounds)
	require.Empty(t, cfg.EnabledSounds)
}

func TestConfig_SoundField(t *testing.T) {
	// Verify Config includes Sound field
	cfg := Config{
		Sound: SoundConfig{
			EnabledSounds: map[string]bool{
				"test_sound": true,
			},
		},
	}
	require.True(t, cfg.Sound.EnabledSounds["test_sound"])
}
