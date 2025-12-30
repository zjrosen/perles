// Package config provides configuration types and defaults for perles.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zjrosen/perles/internal/log"
)

// ColumnConfig defines a single kanban column.
type ColumnConfig struct {
	Name     string `mapstructure:"name"`
	Type     string `mapstructure:"type"`      // "bql" (default) or "tree"
	Query    string `mapstructure:"query"`     // BQL query for filtering (required when type=bql)
	IssueID  string `mapstructure:"issue_id"`  // Root issue ID (required when type=tree)
	TreeMode string `mapstructure:"tree_mode"` // "deps" (default) or "child" for tree columns
	Color    string `mapstructure:"color"`     // hex color e.g. "#10B981"
}

// ViewConfig defines a named board view with its column configuration.
type ViewConfig struct {
	Name    string         `mapstructure:"name"`
	Columns []ColumnConfig `mapstructure:"columns"`
}

// Config holds all configuration options for perles.
type Config struct {
	BeadsDir      string              `mapstructure:"beads_dir"`
	AutoRefresh   bool                `mapstructure:"auto_refresh"`
	UI            UIConfig            `mapstructure:"ui"`
	Theme         ThemeConfig         `mapstructure:"theme"`
	Views         []ViewConfig        `mapstructure:"views"`
	Orchestration OrchestrationConfig `mapstructure:"orchestration"`
}

// UIConfig holds user interface configuration options.
type UIConfig struct {
	ShowCounts      bool   `mapstructure:"show_counts"`
	ShowStatusBar   bool   `mapstructure:"show_status_bar"`
	MarkdownStyle   string `mapstructure:"markdown_style"` // "dark" (default) or "light"
	VimMode       bool `mapstructure:"vim_mode"` // Enable vim keybindings in text input areas
}

// ThemeConfig holds all theme customization options.
type ThemeConfig struct {
	// Preset loads a built-in theme as the base (optional).
	// Valid values: "default", "catppuccin-mocha", "catppuccin-latte",
	// "dracula", "nord", "high-contrast"
	Preset string `mapstructure:"preset"`

	// Mode forces light or dark mode. If empty, uses terminal detection.
	// Valid values: "light", "dark", ""
	Mode string `mapstructure:"mode"`

	// Colors allows overriding individual color tokens.
	// Keys use dot notation: "text.primary", "status.error", etc.
	Colors map[string]string `mapstructure:"colors"`
}

// OrchestrationConfig holds orchestration mode configuration.
type OrchestrationConfig struct {
	Client    string             `mapstructure:"client"` // "claude" (default) or "amp"
	Claude    ClaudeClientConfig `mapstructure:"claude"`
	Amp       AmpClientConfig    `mapstructure:"amp"`
	Workflows []WorkflowConfig   `mapstructure:"workflows"` // Workflow template configurations
}

// ClaudeClientConfig holds Claude-specific settings.
type ClaudeClientConfig struct {
	Model string `mapstructure:"model"` // sonnet (default), opus, haiku
}

// AmpClientConfig holds Amp-specific settings.
type AmpClientConfig struct {
	Model string `mapstructure:"model"` // opus (default), sonnet
	Mode  string `mapstructure:"mode"`  // free, rush, smart (default)
}

// WorkflowConfig defines configuration for a workflow template.
type WorkflowConfig struct {
	Name        string `mapstructure:"name"`        // Display name for the workflow
	Description string `mapstructure:"description"` // Description shown in picker
	Enabled     *bool  `mapstructure:"enabled"`     // nil = true (default enabled)
}

// IsEnabled returns whether the workflow is enabled (defaults to true if nil).
func (w WorkflowConfig) IsEnabled() bool {
	return w.Enabled == nil || *w.Enabled
}

// DefaultColumns returns the default column configuration matching current behavior.
func DefaultColumns() []ColumnConfig {
	return []ColumnConfig{
		{
			Name:  "Blocked",
			Query: "status = open and blocked = true",
			Color: "#FF8787",
		},
		{
			Name:  "Ready",
			Query: "status = open and ready = true",
			Color: "#73F59F",
		},
		{
			Name:  "In Progress",
			Query: "status = in_progress",
			Color: "#54A0FF",
		},
		{
			Name:  "Closed",
			Query: "status = closed",
			Color: "#BBBBBB",
		},
	}
}

// DefaultViews returns the default view configuration with a single "Default" view.
func DefaultViews() []ViewConfig {
	return []ViewConfig{
		{
			Name:    "Default",
			Columns: DefaultColumns(),
		},
	}
}

// ValidateColumns checks column configuration for errors.
// Returns nil if columns are valid or empty (will use defaults).
func ValidateColumns(cols []ColumnConfig) error {
	if len(cols) == 0 {
		return nil // Will use defaults
	}

	for i, col := range cols {
		if col.Name == "" {
			return fmt.Errorf("column %d: name is required", i)
		}

		// Type-based validation (discriminated union pattern)
		switch col.Type {
		case "", "bql":
			// BQL columns require a query
			if col.Query == "" {
				return fmt.Errorf("column %d (%s): query is required for bql columns", i, col.Name)
			}
		case "tree":
			// Tree columns require an issue ID
			if col.IssueID == "" {
				return fmt.Errorf("column %d (%s): issue_id is required for tree columns", i, col.Name)
			}
			// TreeMode defaults to "deps" (handled in tree column creation, not validation)
		default:
			return fmt.Errorf("column %d (%s): invalid type %q (must be \"bql\" or \"tree\")", i, col.Name, col.Type)
		}
	}
	return nil
}

// ValidateViews checks view configuration for errors.
// Returns nil if views are valid or empty (will use defaults).
func ValidateViews(views []ViewConfig) error {
	if len(views) == 0 {
		return nil // Will use defaults
	}

	for i, view := range views {
		if view.Name == "" {
			return fmt.Errorf("view %d: name is required", i)
		}
		// Empty columns array is valid - will show empty state UI
		if err := ValidateColumns(view.Columns); err != nil {
			return fmt.Errorf("view %d (%s): %w", i, view.Name, err)
		}
	}
	return nil
}

// ValidateOrchestration checks orchestration configuration for errors.
// Returns nil if the configuration is valid (empty values use defaults).
func ValidateOrchestration(orch OrchestrationConfig) error {
	// Validate client type
	if orch.Client != "" && orch.Client != "claude" && orch.Client != "amp" {
		return fmt.Errorf("orchestration.client must be \"claude\" or \"amp\", got %q", orch.Client)
	}

	// Validate Amp mode
	if orch.Amp.Mode != "" {
		switch orch.Amp.Mode {
		case "free", "rush", "smart":
			// Valid
		default:
			return fmt.Errorf("orchestration.amp.mode must be \"free\", \"rush\", or \"smart\", got %q", orch.Amp.Mode)
		}
	}

	// Validate workflows
	if err := ValidateWorkflows(orch.Workflows); err != nil {
		return err
	}

	return nil
}

// ValidateWorkflows checks workflow configurations for errors.
// Returns nil if workflows are valid or empty.
func ValidateWorkflows(workflows []WorkflowConfig) error {
	// Currently no validation required - name is optional (used for matching)
	// and enabled defaults to true
	return nil
}

// GetColumns returns the columns for the first view, or defaults if no views configured.
// This provides backward compatibility during the transition to multi-view support.
func (c Config) GetColumns() []ColumnConfig {
	return c.GetColumnsForView(0)
}

// GetColumnsForView returns the columns for a specific view, or defaults if not found.
// Returns empty slice for views with zero columns (empty state).
func (c Config) GetColumnsForView(viewIndex int) []ColumnConfig {
	if viewIndex >= 0 && viewIndex < len(c.Views) {
		return c.Views[viewIndex].Columns // May be empty slice - that's valid
	}
	return DefaultColumns()
}

// GetViews returns the configured views, or DefaultViews() if none configured.
func (c Config) GetViews() []ViewConfig {
	if len(c.Views) > 0 {
		return c.Views
	}
	return DefaultViews()
}

// SetColumns updates the columns for the first view.
// If no views exist, it creates a default view with the given columns.
// This provides backward compatibility during the transition to multi-view support.
func (c *Config) SetColumns(columns []ColumnConfig) {
	c.SetColumnsForView(0, columns)
}

// SetColumnsForView updates the columns for a specific view.
// If no views exist or viewIndex is out of range, it creates/expands to include the view.
func (c *Config) SetColumnsForView(viewIndex int, columns []ColumnConfig) {
	if len(c.Views) == 0 {
		c.Views = []ViewConfig{{Name: "Default", Columns: columns}}
		return
	}
	if viewIndex < 0 || viewIndex >= len(c.Views) {
		return // Out of range, do nothing
	}
	c.Views[viewIndex].Columns = columns
}

// Defaults returns a Config with sensible default values.
func Defaults() Config {
	return Config{
		AutoRefresh: true,
		UI: UIConfig{
			ShowCounts:    true,
			ShowStatusBar: true,
			MarkdownStyle: "dark",
			VimMode:       false, // Disabled by default for non-vim users
		},
		Theme: ThemeConfig{
			// Default theme uses the "default" preset
			Preset: "",
		},
		Views: DefaultViews(),
		Orchestration: OrchestrationConfig{
			Client: "claude",
			Claude: ClaudeClientConfig{
				Model: "claude-opus-4-5",
			},
			Amp: AmpClientConfig{
				Model: "opus",
				Mode:  "smart",
			},
		},
	}
}

// DefaultConfigTemplate returns the default config as a YAML string with comments.
func DefaultConfigTemplate() string {
	return `# Perles Configuration

# Path to beads database directory (default: current directory)
# beads_dir: /path/to/project

# Auto-refresh when database changes
auto_refresh: true

# UI settings
ui:
  show_counts: true       # Show issue counts in column headers
  show_status_bar: true   # Show status bar at bottom
  # markdown_style: dark  # Markdown rendering style: "dark" (default) or "light"
  vim_mode: false         # Enable vim keybindings in text input areas (orchestration mode)

# Theme configuration
# Use a preset theme or customize individual colors
theme:
  # Use a preset (run 'perles themes' to see available presets):
  # preset: catppuccin-mocha
  #
  # Available presets:
  #   default           - Default perles theme
  #   catppuccin-mocha  - Warm, cozy dark theme
  #   catppuccin-latte  - Warm, cozy light theme
  #   dracula           - Dark theme with vibrant colors
  #   nord              - Arctic, north-bluish palette
  #   high-contrast     - High contrast for accessibility
  #
  # Override specific colors (works with or without preset):
  # colors:
  #   text.primary: "#FFFFFF"
  #   status.error: "#FF0000"
  #   priority.critical: "#FF5555"
  #
  # See all available color tokens with 'perles themes --help' or docs

# Board views - each view is a named collection of columns
# Cycle through views with Shift+J (next) and Shift+K (previous)
views:
  - name: Default
    columns:
      - name: Blocked
        type: bql
        query: "status = open and blocked = true"
        color: "#FF8787"

      - name: Ready
        type: bql
        query: "status = open and ready = true"
        color: "#73F59F"

      - name: In Progress
        type: bql
        query: "status = in_progress"
        color: "#54A0FF"

      - name: Closed
        type: bql
        query: "status = closed"
        color: "#BBBBBB"

# View options:
#   name: Display name for the view (required)
#   columns: List of columns for this view (required)
#
# Column options:
#   name: Display name (required)
#   type: bql or tree
#   query: BQL query (required when type is bql) - see BQL syntax below
#   issue_id: Issue Id (required when type is tree)
#   tree_mode: deps or child (optional when type is tree)
#   color: Hex color for column header
#
# BQL Query Syntax:
#   Fields: type, priority, status, blocked, ready, label, title, id, created, updated
#   Operators: = != < > <= >= ~ (contains) in not-in
#   Examples:
#     status = open
#     type = bug and priority = P0
#     blocked = true
#     label in (urgent, critical)
#     title ~ auth

# Orchestration mode settings
# Configure which AI client to use when entering orchestration mode
orchestration:
  # AI client provider: "claude" (default) or "amp"
  client: claude

  # Claude-specific settings (only used when client: claude)
  claude:
    model: sonnet  # sonnet (default), opus, or haiku

  # Amp-specific settings (only used when client: amp)
  amp:
    model: opus    # opus (default) or sonnet
    mode: smart    # free, rush, or smart (default)

  # Workflow templates (Ctrl+P to open picker in orchestration mode)
  # User workflows are loaded from ~/.perles/workflows/*.md
  # workflows:
  #   # Define a user workflow (loaded from ~/.perles/workflows/)
  #   - name: "Code Review"
  #     description: "Multi-perspective code review"
  #     file: "code_review.md"
  #
  #   # Disable a built-in workflow
  #   - name: "Debate"
  #     enabled: false
  #
  #   # Override name/description of a built-in workflow
  #   - name: "Research Proposal"
  #     description: "Custom description for research workflow"
`
}

// WriteDefaultConfig creates a config file at the given path with default settings and comments.
// Creates the parent directory if it doesn't exist.
func WriteDefaultConfig(configPath string) error {
	log.Debug(log.CatConfig, "Writing default config", "path", configPath)

	// Create parent directory if needed
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		log.ErrorErr(log.CatConfig, "Failed to create config directory", err, "dir", dir)
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Write the template
	if err := os.WriteFile(configPath, []byte(DefaultConfigTemplate()), 0o600); err != nil {
		log.ErrorErr(log.CatConfig, "Failed to write config file", err, "path", configPath)
		return fmt.Errorf("writing config file: %w", err)
	}

	log.Info(log.CatConfig, "Created default config", "path", configPath)
	return nil
}
