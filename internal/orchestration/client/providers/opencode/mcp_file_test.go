package opencode

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"

	"github.com/zjrosen/perles/internal/log"
)

// blockCommentRegex matches block /* */ comments only.
// Note: We don't strip single-line // comments to avoid issues with URLs containing ://.
// OpenCode's JSONC typically uses block comments which are safer to strip.
var blockCommentRegex = regexp.MustCompile(`/\*[\s\S]*?\*/`)

// stripJSONComments removes JavaScript-style block comments from JSONC content.
// It only removes /* */ style comments, not // comments, because // can appear
// in URLs (like https://) which would corrupt the JSON.
func stripJSONComments(input []byte) []byte {
	return blockCommentRegex.ReplaceAll(input, nil)
}

// setupMCPConfig creates or updates the opencode.jsonc file with MCP configuration.
// If cfg.MCPConfig is empty, it's a no-op.
// The function merges mcp entries without overwriting other settings.
//
// Deprecated: This function is no longer used for spawning processes.
// MCP configuration is now passed via the OPENCODE_CONFIG_CONTENT environment
// variable for better process isolation. This function is kept for test
// backwards compatibility.
func setupMCPConfig(cfg Config) error {
	// No-op if MCPConfig is empty
	if cfg.MCPConfig == "" {
		return nil
	}

	// Parse the provided MCP config to extract mcp section
	var mcpConfig map[string]any
	if err := json.Unmarshal([]byte(cfg.MCPConfig), &mcpConfig); err != nil {
		return fmt.Errorf("failed to parse MCPConfig JSON: %w", err)
	}

	// OpenCode config file is opencode.jsonc in project root
	configPath := filepath.Join(cfg.WorkDir, "opencode.jsonc")

	// Read existing opencode.jsonc if it exists
	existingSettings := make(map[string]any)
	existingData, err := os.ReadFile(configPath) //#nosec G304 -- path is constructed from validated config
	if err == nil {
		// File exists, strip comments and parse it
		cleanedData := stripJSONComments(existingData)
		if err := json.Unmarshal(cleanedData, &existingSettings); err != nil {
			return fmt.Errorf("failed to parse existing opencode.jsonc: %w", err)
		}
	} else if !os.IsNotExist(err) {
		// Some other error reading the file
		return fmt.Errorf("failed to read opencode.jsonc: %w", err)
	}
	// If file doesn't exist, existingSettings remains an empty map

	// Get or create mcp map in existing settings
	var existingMCP map[string]any
	if existing, ok := existingSettings["mcp"]; ok {
		existingMCP, ok = existing.(map[string]any)
		if !ok {
			return fmt.Errorf("existing mcp is not a valid object")
		}
	} else {
		existingMCP = make(map[string]any)
	}

	// Merge new mcp servers into existing
	if newMCP, ok := mcpConfig["mcp"]; ok {
		newServers, ok := newMCP.(map[string]any)
		if !ok {
			return fmt.Errorf("mcp in MCPConfig is not a valid object")
		}
		maps.Copy(existingMCP, newServers)
	}

	// Update settings with merged mcp
	existingSettings["mcp"] = existingMCP

	// Write the merged settings back with proper formatting
	outputData, err := json.MarshalIndent(existingSettings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(configPath, outputData, 0600); err != nil {
		return fmt.Errorf("failed to write opencode.jsonc: %w", err)
	}

	log.Debug(log.CatOrch, "Wrote MCP config to opencode.jsonc", "subsystem", "opencode", "path", configPath)
	return nil
}
