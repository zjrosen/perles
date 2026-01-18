package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/orchestration/client"
)

// Process represents a headless Gemini CLI process.
// Process implements client.HeadlessProcess by embedding BaseProcess.
type Process struct {
	*client.BaseProcess
}

// ErrTimeout is returned when a Gemini process exceeds its configured timeout.
var ErrTimeout = fmt.Errorf("gemini process timed out")

// ErrNoAuth is returned when no valid authentication is found.
var ErrNoAuth = fmt.Errorf("gemini: no authentication found - set GEMINI_API_KEY, GOOGLE_API_KEY, or run 'gemini auth' for OAuth")

// ErrNotFound is returned when the gemini executable cannot be found.
var ErrNotFound = fmt.Errorf("gemini: executable not found - install with 'npm install -g @anthropic-ai/claude-code-gemini' or ensure 'gemini' is in PATH")

// validateAuth checks for valid Gemini authentication.
// It checks in order: OAuth token file, GEMINI_API_KEY env, GOOGLE_API_KEY env.
func validateAuth() error {
	// Check for OAuth token at ~/.gemini/mcp-oauth-tokens-v2.json (Gemini CLI OAuth storage)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		tokenPath := filepath.Join(homeDir, ".gemini", "mcp-oauth-tokens-v2.json")
		if _, err := os.Stat(tokenPath); err == nil {
			log.Debug(log.CatOrch, "Found OAuth token", "subsystem", "gemini", "path", tokenPath)
			return nil
		}
	}

	// Check for GEMINI_API_KEY environment variable
	if os.Getenv("GEMINI_API_KEY") != "" {
		log.Debug(log.CatOrch, "Found GEMINI_API_KEY", "subsystem", "gemini")
		return nil
	}

	// Check for GOOGLE_API_KEY environment variable
	if os.Getenv("GOOGLE_API_KEY") != "" {
		log.Debug(log.CatOrch, "Found GOOGLE_API_KEY", "subsystem", "gemini")
		return nil
	}

	return ErrNoAuth
}

// findExecutable locates the gemini executable.
// It checks in order: ~/.npm/bin/gemini, /usr/local/bin/gemini, then exec.LookPath.
func findExecutable() (string, error) {
	// On Windows, executables need .exe extension
	execName := "gemini"
	if os.PathSeparator == '\\' {
		execName = "gemini.exe"
	}

	// Check ~/.npm/bin/gemini first
	homeDir, err := os.UserHomeDir()
	if err == nil {
		npmPath := filepath.Join(homeDir, ".npm", "bin", execName)
		if _, err := os.Stat(npmPath); err == nil {
			log.Debug(log.CatOrch, "Found gemini at npm path", "subsystem", "gemini", "path", npmPath)
			return npmPath, nil
		}
	}

	// Check /usr/local/bin/gemini
	localPath := "/usr/local/bin/gemini"
	if _, err := os.Stat(localPath); err == nil {
		log.Debug(log.CatOrch, "Found gemini at local bin", "subsystem", "gemini", "path", localPath)
		return localPath, nil
	}

	// Fall back to exec.LookPath
	path, err := exec.LookPath("gemini")
	if err == nil {
		log.Debug(log.CatOrch, "Found gemini via PATH", "subsystem", "gemini", "path", path)
		return path, nil
	}

	return "", ErrNotFound
}

// setupMCPConfig creates or updates the .gemini/settings.json file with MCP configuration.
// If cfg.MCPConfig is empty, it's a no-op.
// The function merges mcpServers entries without overwriting other settings.
func setupMCPConfig(cfg Config) error {
	// No-op if MCPConfig is empty
	if cfg.MCPConfig == "" {
		return nil
	}

	// Parse the provided MCP config to extract mcpServers
	var mcpConfig map[string]any
	if err := json.Unmarshal([]byte(cfg.MCPConfig), &mcpConfig); err != nil {
		return fmt.Errorf("failed to parse MCPConfig JSON: %w", err)
	}

	// Ensure .gemini directory exists
	geminiDir := filepath.Join(cfg.WorkDir, ".gemini")
	if err := os.MkdirAll(geminiDir, 0750); err != nil {
		return fmt.Errorf("failed to create .gemini directory: %w", err)
	}

	settingsPath := filepath.Join(geminiDir, "settings.json")

	// Read existing settings.json if it exists
	existingSettings := make(map[string]any)
	existingData, err := os.ReadFile(settingsPath) //#nosec G304 -- path is constructed from validated config
	if err == nil {
		// File exists, parse it
		if err := json.Unmarshal(existingData, &existingSettings); err != nil {
			return fmt.Errorf("failed to parse existing settings.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		// Some other error reading the file
		return fmt.Errorf("failed to read settings.json: %w", err)
	}
	// If file doesn't exist, existingSettings remains an empty map

	// Get or create mcpServers map in existing settings
	var existingMCPServers map[string]any
	if existing, ok := existingSettings["mcpServers"]; ok {
		existingMCPServers, ok = existing.(map[string]any)
		if !ok {
			return fmt.Errorf("existing mcpServers is not a valid object")
		}
	} else {
		existingMCPServers = make(map[string]any)
	}

	// Merge new mcpServers into existing
	if newMCPServers, ok := mcpConfig["mcpServers"]; ok {
		newServers, ok := newMCPServers.(map[string]any)
		if !ok {
			return fmt.Errorf("mcpServers in MCPConfig is not a valid object")
		}
		maps.Copy(existingMCPServers, newServers)
	}

	// Update settings with merged mcpServers
	existingSettings["mcpServers"] = existingMCPServers

	// Write the merged settings back with proper formatting
	outputData, err := json.MarshalIndent(existingSettings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, outputData, 0600); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	log.Debug(log.CatOrch, "Wrote MCP config to settings.json", "subsystem", "gemini", "path", settingsPath)
	return nil
}

// extractSession extracts the session ID from an init event.
func extractSession(event client.OutputEvent, _ []byte) string {
	if event.Type == client.EventSystem && event.SubType == "init" && event.SessionID != "" {
		return event.SessionID
	}
	return ""
}

// Spawn creates and starts a new headless Gemini process.
// Context is used for cancellation and timeout control.
func Spawn(ctx context.Context, cfg Config) (*Process, error) {
	return spawnProcess(ctx, cfg, false)
}

// Resume continues an existing Gemini session using --resume flag.
func Resume(ctx context.Context, sessionID string, cfg Config) (*Process, error) {
	cfg.SessionID = sessionID
	return spawnProcess(ctx, cfg, true)
}

// spawnProcess is the internal implementation for both Spawn and Resume.
func spawnProcess(ctx context.Context, cfg Config, _ bool) (*Process, error) {
	// Validate authentication first
	if err := validateAuth(); err != nil {
		return nil, err
	}

	// Find the gemini executable
	execPath, err := findExecutable()
	if err != nil {
		return nil, err
	}

	// Setup MCP configuration (creates/updates .gemini/settings.json)
	if err := setupMCPConfig(cfg); err != nil {
		return nil, fmt.Errorf("failed to setup MCP config: %w", err)
	}

	var procCtx context.Context
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		procCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	} else {
		procCtx, cancel = context.WithCancel(ctx)
	}

	args := buildArgs(cfg)
	log.Debug(log.CatOrch, "Spawning gemini process", "subsystem", "gemini", "args", strings.Join(args, " "), "workDir", cfg.WorkDir)

	// #nosec G204 -- args are built from Config struct, not user input
	cmd := exec.CommandContext(procCtx, execPath, args...)
	cmd.Dir = cfg.WorkDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Create the Gemini process with embedded BaseProcess
	p := &Process{}

	// Create BaseProcess with Gemini-specific hooks
	bp := client.NewBaseProcess(
		procCtx,
		cancel,
		cmd,
		stdout,
		stderr,
		cfg.WorkDir,
		client.WithParseEventFunc(ParseEvent),
		client.WithSessionExtractor(extractSession),
		client.WithStderrCapture(true),
		client.WithProviderName("gemini"),
	)
	p.BaseProcess = bp

	if err := cmd.Start(); err != nil {
		cancel()
		log.Debug(log.CatOrch, "Failed to start gemini process", "subsystem", "gemini", "error", err)
		return nil, fmt.Errorf("failed to start gemini process: %w", err)
	}

	log.Debug(log.CatOrch, "Gemini process started", "subsystem", "gemini", "pid", cmd.Process.Pid)
	bp.SetStatus(client.StatusRunning)

	// Start output parser goroutines
	bp.StartGoroutines()

	return p, nil
}

// Ensure Process implements client.HeadlessProcess at compile time.
var _ client.HeadlessProcess = (*Process)(nil)
