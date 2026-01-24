// Package config provides configuration types and defaults for perles.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
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
	Sound         SoundConfig         `mapstructure:"sound"`
	Flags         map[string]bool     `mapstructure:"flags"`

	// ResolvedBeadsDir is the final resolved beads directory path after applying
	// resolution priority (flag > env var > config > cwd). Used for propagation to agents.
	// This field is not serialized to YAML.
	ResolvedBeadsDir string `mapstructure:"-" yaml:"-"`
}

// UIConfig holds user interface configuration options.
type UIConfig struct {
	ShowCounts    bool   `mapstructure:"show_counts"`
	ShowStatusBar bool   `mapstructure:"show_status_bar"`
	MarkdownStyle string `mapstructure:"markdown_style"` // "dark" (default) or "light"
	VimMode       bool   `mapstructure:"vim_mode"`       // Enable vim keybindings in text input areas
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
	// Supports both nested YAML structure and dot notation.
	// Example YAML:
	//   colors:
	//     text:
	//       primary: "#FF0000"
	// Or quoted dot notation:
	//   colors:
	//     "text.primary": "#FF0000"
	Colors map[string]any `mapstructure:"colors"`
}

// FlattenedColors returns the Colors map flattened to dot-notation keys.
// This handles both nested YAML structures and already-flat keys.
func (t ThemeConfig) FlattenedColors() map[string]string {
	result := make(map[string]string)
	flattenColors("", t.Colors, result)
	return result
}

// flattenColors recursively flattens a nested map into dot-notation keys.
func flattenColors(prefix string, m map[string]any, result map[string]string) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		switch val := v.(type) {
		case string:
			result[key] = val
		case map[string]any:
			flattenColors(key, val, result)
		case map[any]any:
			// YAML sometimes produces map[any]any instead of map[string]any
			converted := make(map[string]any)
			for mk, mv := range val {
				if strKey, ok := mk.(string); ok {
					converted[strKey] = mv
				}
			}
			flattenColors(key, converted, result)
		}
	}
}

// SessionStorageConfig holds session storage location configuration.
type SessionStorageConfig struct {
	// BaseDir is the root directory for session storage.
	// Default: ~/.perles/sessions
	BaseDir string `mapstructure:"base_dir"`

	// ApplicationName identifies the project/application.
	// Default: derived from git remote or directory name
	ApplicationName string `mapstructure:"application_name"`
}

// TimeoutsConfig holds timeout settings for orchestration initialization phases.
type TimeoutsConfig struct {
	// WorktreeCreation is the timeout for git worktree creation.
	// Default: 30 seconds
	WorktreeCreation time.Duration `mapstructure:"worktree_creation"`

	// CoordinatorStart is the timeout for coordinator process startup and first response.
	// Default: 60 seconds (longer for slow API responses)
	CoordinatorStart time.Duration `mapstructure:"coordinator_start"`

	// WorkspaceSetup is the timeout for MCP server, session, and infrastructure setup.
	// Default: 30 seconds
	WorkspaceSetup time.Duration `mapstructure:"workspace_setup"`

	// MaxTotal is the maximum total time allowed for initialization.
	// Acts as a hard safety net; 0 means disabled.
	// Default: 120 seconds
	MaxTotal time.Duration `mapstructure:"max_total"`
}

// DefaultTimeoutsConfig returns the default timeout configuration.
func DefaultTimeoutsConfig() TimeoutsConfig {
	return TimeoutsConfig{
		WorktreeCreation: 30 * time.Second,
		CoordinatorStart: 60 * time.Second,
		WorkspaceSetup:   30 * time.Second,
		MaxTotal:         120 * time.Second,
	}
}

// OrchestrationConfig holds orchestration mode configuration.
type OrchestrationConfig struct {
	Client            string               `mapstructure:"client"`             // "claude" (default), "amp", "codex", or "gemini" - backward compat
	CoordinatorClient string               `mapstructure:"coordinator_client"` // Client for coordinator (overrides Client)
	WorkerClient      string               `mapstructure:"worker_client"`      // Client for workers (overrides Client)
	DisableWorktrees  bool                 `mapstructure:"disable_worktrees"`  // Skip worktree prompt (default: false)
	APIPort           int                  `mapstructure:"api_port"`           // HTTP API port (0 = auto-assign, default: 0)
	Claude            ClaudeClientConfig   `mapstructure:"claude"`
	ClaudeWorker      ClaudeClientConfig   `mapstructure:"claude_worker"` // Worker-specific Claude config (uses claude config if empty)
	Codex             CodexClientConfig    `mapstructure:"codex"`
	Amp               AmpClientConfig      `mapstructure:"amp"`
	Gemini            GeminiClientConfig   `mapstructure:"gemini"`
	OpenCode          OpenCodeClientConfig `mapstructure:"opencode"`
	Workflows         []WorkflowConfig     `mapstructure:"workflows"`       // Workflow template configurations
	Tracing           TracingConfig        `mapstructure:"tracing"`         // Distributed tracing configuration
	SessionStorage    SessionStorageConfig `mapstructure:"session_storage"` // Session storage location configuration
	Timeouts          TimeoutsConfig       `mapstructure:"timeouts"`        // Initialization phase timeout configuration
}

// ClaudeClientConfig holds Claude-specific settings.
type ClaudeClientConfig struct {
	Model string            `mapstructure:"model"` // sonnet (default), opus, haiku
	Env   map[string]string `mapstructure:"env"`   // Custom environment variables (supports ${VAR} expansion)
}

// CodexClientConfig holds Claude-specific settings.
type CodexClientConfig struct {
	Model string `mapstructure:"model"` // gpt-5.2-codex (default), o4-mini
}

// AmpClientConfig holds Amp-specific settings.
type AmpClientConfig struct {
	Model string `mapstructure:"model"` // opus (default), sonnet
	Mode  string `mapstructure:"mode"`  // free, rush, smart (default)
}

// GeminiClientConfig holds Gemini-specific settings.
type GeminiClientConfig struct {
	Model string `mapstructure:"model"` // gemini-3-pro-preview (default), gemini-2.5-flash
}

// OpenCodeClientConfig holds OpenCode-specific settings.
type OpenCodeClientConfig struct {
	Model string `mapstructure:"model"` // anthropic/claude-opus-4-5 (default)
}

// CoordinatorClientType returns the client type for the coordinator.
// Resolution priority: coordinator_client > client > "claude"
func (o OrchestrationConfig) CoordinatorClientType() client.ClientType {
	if o.CoordinatorClient != "" {
		return client.ClientType(o.CoordinatorClient)
	}
	if o.Client != "" {
		return client.ClientType(o.Client)
	}
	return client.ClientClaude
}

// WorkerClientType returns the client type for workers.
// Resolution priority: worker_client > client > "claude"
func (o OrchestrationConfig) WorkerClientType() client.ClientType {
	if o.WorkerClient != "" {
		return client.ClientType(o.WorkerClient)
	}
	if o.Client != "" {
		return client.ClientType(o.Client)
	}
	return client.ClientClaude
}

// AgentProviders returns the AgentProviders map for coordinator and worker roles.
// This is the preferred way to get AI clients for orchestration.
func (o OrchestrationConfig) AgentProviders() client.AgentProviders {
	coordType := o.CoordinatorClientType()
	workerType := o.WorkerClientType()

	return client.AgentProviders{
		client.RoleCoordinator: client.NewAgentProvider(coordType, o.extensionsForClient(coordType, false)),
		client.RoleWorker:      client.NewAgentProvider(workerType, o.extensionsForClient(workerType, true)),
	}
}

// extensionsForClient builds extensions for the given client type.
// If isWorker and client is claude, uses claude_worker config when env is set.
func (o OrchestrationConfig) extensionsForClient(clientType client.ClientType, isWorker bool) map[string]any {
	extensions := make(map[string]any)

	switch clientType {
	case client.ClientClaude:
		cfg := o.Claude
		// For workers, use claude_worker config if it has env vars configured
		if isWorker && len(o.ClaudeWorker.Env) > 0 {
			cfg = o.ClaudeWorker
			// If worker model is empty, inherit from main claude config
			if cfg.Model == "" {
				cfg.Model = o.Claude.Model
			}
		}
		if cfg.Model != "" {
			extensions[client.ExtClaudeModel] = cfg.Model
		}
		if len(cfg.Env) > 0 {
			extensions[client.ExtClaudeEnv] = cfg.Env
		}
	case client.ClientCodex:
		if o.Codex.Model != "" {
			extensions[client.ExtCodexModel] = o.Codex.Model
		}
	case client.ClientAmp:
		if o.Amp.Model != "" {
			extensions[client.ExtAmpModel] = o.Amp.Model
		}
		if o.Amp.Mode != "" {
			// Note: Amp mode key is defined in amp package, but we use the literal here
			// to avoid import cycle. The value is "amp.mode".
			extensions["amp.mode"] = o.Amp.Mode
		}
	case client.ClientGemini:
		if o.Gemini.Model != "" {
			extensions[client.ExtGeminiModel] = o.Gemini.Model
		}
	case client.ClientOpenCode:
		if o.OpenCode.Model != "" {
			extensions[client.ExtOpenCodeModel] = o.OpenCode.Model
		}
	}

	return extensions
}

// WorkflowConfig defines configuration for a workflow template.
type WorkflowConfig struct {
	Name        string `mapstructure:"name"`        // Display name for the workflow
	Description string `mapstructure:"description"` // Description shown in picker
	Enabled     *bool  `mapstructure:"enabled"`     // nil = true (default enabled)
}

// TracingConfig holds distributed tracing configuration for orchestration.
type TracingConfig struct {
	// Enabled controls whether distributed tracing is active.
	// Default: false
	Enabled bool `mapstructure:"enabled"`

	// Exporter selects the trace export backend.
	// Options: "none", "file", "stdout", "otlp"
	// Default: "file"
	Exporter string `mapstructure:"exporter"`

	// FilePath is the output file for "file" exporter.
	// Default: ~/.config/perles/traces/traces.jsonl
	FilePath string `mapstructure:"file_path"`

	// OTLPEndpoint is the collector endpoint for "otlp" exporter.
	// Default: "localhost:4317"
	OTLPEndpoint string `mapstructure:"otlp_endpoint"`

	// SampleRate controls trace sampling (0.0 to 1.0).
	// 1.0 = all traces, 0.1 = 10% of traces
	// Default: 1.0
	SampleRate float64 `mapstructure:"sample_rate"`
}

// IsEnabled returns whether the workflow is enabled (defaults to true if nil).
func (w WorkflowConfig) IsEnabled() bool {
	return w.Enabled == nil || *w.Enabled
}

// SoundEventConfig configures a single sound event with optional override sounds.
type SoundEventConfig struct {
	// Enabled controls whether this sound event plays.
	Enabled bool `mapstructure:"enabled"`

	// OverrideSounds is a list of custom sound file paths to play instead of defaults.
	// If empty or nil, uses the embedded default sound.
	// Multiple paths enable random selection for variety.
	// Paths must be under ~/.perles/sounds/
	OverrideSounds []string `mapstructure:"override_sounds"`
}

// SoundConfig holds audio feedback configuration for orchestration.
type SoundConfig struct {
	// Events maps sound event identifiers to their configuration.
	// Keys are identifiers using underscores (e.g., "review_verdict_approve", "chat_welcome").
	Events map[string]SoundEventConfig `mapstructure:"events"`
}

// DefaultTracesFilePath returns the default path for trace file export.
// Returns ~/.config/perles/traces/traces.jsonl or empty string if home dir unavailable.
func DefaultTracesFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "perles", "traces", "traces.jsonl")
}

// DefaultSessionStorageBaseDir returns the default path for session storage.
// Returns ~/.perles/sessions or empty string if home dir unavailable.
func DefaultSessionStorageBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".perles", "sessions")
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
// allowedClients is the list of valid AI client types for orchestration.
var allowedClients = []string{"claude", "amp", "codex", "gemini", "opencode"}

// isAllowedClient checks if the given client type is in the allowed list.
func isAllowedClient(c string) bool {
	return slices.Contains(allowedClients, c)
}

func ValidateOrchestration(orch OrchestrationConfig) error {
	// Validate client type (legacy field)
	if orch.Client != "" && !isAllowedClient(orch.Client) {
		return fmt.Errorf("orchestration.client must be one of %v, got %q", allowedClients, orch.Client)
	}

	// Validate coordinator_client
	if orch.CoordinatorClient != "" && !isAllowedClient(orch.CoordinatorClient) {
		return fmt.Errorf("orchestration.coordinator_client must be one of %v, got %q", allowedClients, orch.CoordinatorClient)
	}

	// Validate worker_client
	if orch.WorkerClient != "" && !isAllowedClient(orch.WorkerClient) {
		return fmt.Errorf("orchestration.worker_client must be one of %v, got %q", allowedClients, orch.WorkerClient)
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

	// Validate tracing
	if err := ValidateTracing(orch.Tracing); err != nil {
		return err
	}

	// Validate session storage
	if err := ValidateSessionStorage(orch.SessionStorage); err != nil {
		return err
	}

	return nil
}

// maxSoundFileSize is the maximum allowed size for override sound files (1MB).
const maxSoundFileSize = 1 * 1024 * 1024

// SoundSecurityBoundary returns the security boundary directory for sound files.
// All override sound paths must be under this directory.
// Returns ~/.perles/sounds/ or empty string if home dir unavailable.
func SoundSecurityBoundary() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".perles", "sounds")
}

// ValidateSound checks sound configuration for errors.
// Returns nil if the configuration is valid.
// Validates:
// - All override paths are under ~/.perles/sounds/
// - Paths cannot escape the boundary via symlinks or path traversal
// - Only .wav extension is allowed (case-insensitive)
// - Override sound files must exist
// - Override sound files must be <= 1MB
func ValidateSound(sound SoundConfig) error {
	if sound.Events == nil {
		return nil
	}

	boundary := SoundSecurityBoundary()
	if boundary == "" {
		// Cannot validate paths without home directory
		// Only check if there are actual override paths configured
		for eventName, eventConfig := range sound.Events {
			if len(eventConfig.OverrideSounds) > 0 {
				return fmt.Errorf("sound.events.%s: cannot validate override paths (home directory unavailable)", eventName)
			}
		}
		return nil
	}

	for eventName, eventConfig := range sound.Events {
		for i, path := range eventConfig.OverrideSounds {
			if err := validateSoundPath(path, eventName, i, boundary); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateSoundPath validates a single sound file path against security and format requirements.
func validateSoundPath(path, eventName string, index int, boundary string) error {
	// Clean the path first to normalize it
	cleanPath := filepath.Clean(path)

	// Check WAV extension (case-insensitive) before anything else
	ext := filepath.Ext(cleanPath)
	if !strings.EqualFold(ext, ".wav") {
		return fmt.Errorf("sound.events.%s.override_sounds[%d]: only WAV format is supported, got %q", eventName, index, ext)
	}

	// Resolve symlinks to get the real path
	realPath, err := filepath.EvalSymlinks(cleanPath)
	if err != nil {
		// File doesn't exist or symlink is broken
		if os.IsNotExist(err) {
			return fmt.Errorf("sound.events.%s.override_sounds[%d]: file not found: %q", eventName, index, path)
		}
		return fmt.Errorf("sound.events.%s.override_sounds[%d]: cannot resolve path: %w", eventName, index, err)
	}

	// Resolve the boundary path as well to handle platform symlinks
	// (e.g., macOS symlinks /var/folders to /private/var/folders)
	realBoundary, err := filepath.EvalSymlinks(boundary)
	if err != nil {
		// Boundary directory doesn't exist - this is okay, the files just can't be validated
		// But since we have override files, we need the boundary to exist
		return fmt.Errorf("sound.events.%s.override_sounds[%d]: security boundary directory does not exist: %s", eventName, index, boundary)
	}

	// Check security boundary using the resolved real paths
	// This prevents symlink attacks that point outside the boundary
	if !strings.HasPrefix(realPath, realBoundary+string(filepath.Separator)) && realPath != realBoundary {
		return fmt.Errorf("sound.events.%s.override_sounds[%d]: path must be under %s, got %q", eventName, index, boundary, path)
	}

	// File exists (EvalSymlinks succeeded), now check size
	info, err := os.Stat(realPath)
	if err != nil {
		return fmt.Errorf("sound.events.%s.override_sounds[%d]: cannot stat file: %w", eventName, index, err)
	}
	if info.Size() > maxSoundFileSize {
		return fmt.Errorf("sound.events.%s.override_sounds[%d]: file too large: %d bytes (max %d)", eventName, index, info.Size(), maxSoundFileSize)
	}

	return nil
}

// ValidateSessionStorage checks session storage configuration for errors.
// Returns nil if the configuration is valid (empty values use defaults).
func ValidateSessionStorage(storage SessionStorageConfig) error {
	// BaseDir must be absolute if set
	if storage.BaseDir != "" && !filepath.IsAbs(storage.BaseDir) {
		return fmt.Errorf("orchestration.session_storage.base_dir must be an absolute path, got %q", storage.BaseDir)
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

// ValidateTracing checks tracing configuration for errors.
// Returns nil if the configuration is valid (empty values use defaults).
func ValidateTracing(tracing TracingConfig) error {
	// Validate SampleRate is in range [0.0, 1.0]
	if tracing.SampleRate < 0.0 || tracing.SampleRate > 1.0 {
		return fmt.Errorf("orchestration.tracing.sample_rate must be between 0.0 and 1.0, got %v", tracing.SampleRate)
	}

	// Validate Exporter is a valid option
	if tracing.Exporter != "" {
		switch tracing.Exporter {
		case "none", "file", "stdout", "otlp":
			// Valid
		default:
			return fmt.Errorf("orchestration.tracing.exporter must be \"none\", \"file\", \"stdout\", or \"otlp\", got %q", tracing.Exporter)
		}
	}

	// Only validate path requirements when tracing is enabled
	if tracing.Enabled {
		// FilePath is required when Exporter is "file"
		if tracing.Exporter == "file" && tracing.FilePath == "" {
			return fmt.Errorf("orchestration.tracing.file_path is required when exporter is \"file\"")
		}

		// OTLPEndpoint is required when Exporter is "otlp"
		if tracing.Exporter == "otlp" && tracing.OTLPEndpoint == "" {
			return fmt.Errorf("orchestration.tracing.otlp_endpoint is required when exporter is \"otlp\"")
		}
	}

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
			CoordinatorClient: "claude",
			WorkerClient:      "claude",
			Claude: ClaudeClientConfig{
				Model: "claude-opus-4-5",
			},
			Amp: AmpClientConfig{
				Model: "opus",
				Mode:  "smart",
			},
			Codex: CodexClientConfig{
				Model: "gpt-5.2-codex",
			},
			Gemini: GeminiClientConfig{
				Model: "gemini-3-pro-preview",
			},
			Tracing: TracingConfig{
				Enabled:      false,
				Exporter:     "file",
				FilePath:     "", // Derived from config dir at runtime
				OTLPEndpoint: "localhost:4317",
				SampleRate:   1.0,
			},
			SessionStorage: SessionStorageConfig{
				BaseDir:         DefaultSessionStorageBaseDir(),
				ApplicationName: "", // Derived from git remote or directory name
			},
			Timeouts: DefaultTimeoutsConfig(),
		},
		Sound: SoundConfig{
			Events: map[string]SoundEventConfig{
				"review_verdict_approve": {Enabled: false},
				"review_verdict_deny":    {Enabled: false},
				"chat_welcome":           {Enabled: false},
				"workflow_complete":      {Enabled: false},
				"orchestration_welcome":  {Enabled: false},
				"worker_out_of_context":  {Enabled: false},
				"user_notification":      {Enabled: false},
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
  # AI client provider for the coordinator: "claude" (default), "amp", "codex", "opencode", or "opencode"
  coordinator_client: claude

  # AI client provider for the workers: "claude" (default), "amp", "codex", "opencode", or "opencode"
  worker_client: claude

  # Skip worktree prompt and always run in current directory (default: false)
  # disable_worktrees: true

  # Claude-specific settings (only used when client: claude)
  claude:
    model: opus  # sonnet (default), opus, or haiku

  # Codex-specific settings (only used when client: codex)
  codex:
    model: gpt-5.2-codex  # gpt-5.2-codex (default)

  # Amp-specific settings (only used when client: amp)
  amp:
    model: opus    # opus (default) or sonnet
    mode: smart    # free, rush, or smart (default)

  # OpenCode-specific settings (only used when client: opencode)
  opencode:
    model: anthropic/claude-opus-4-5  # anthropic/claude-opus-4-5 (default)

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

  # Sound Notifications
  # Audio feedback for orchestration events. All events are disabled by default.
  # To override the default sounds use the override_sounds for each event
  # Custom sounds must be WAV files located in ~/.perles/sounds/
  sound:
    events:
      # Plays when entering chat mode
      # chat_welcome:
      #   enabled: true
      #   override_sounds:
      #     - ~/.perles/sounds/my-welcome.wav

      # Plays when entering orchestration mode
      # orchestration_welcome:
      #   enabled: true

      # Plays when a workflow completes
      # workflow_complete:
      #   enabled: true

      # Plays when a review is approved
      # review_verdict_approve:
      #   enabled: true

      # Plays when a review is denied
      # review_verdict_deny:
      #   enabled: true
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
