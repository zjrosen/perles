package workflow

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// frontmatter represents the YAML frontmatter in a workflow template.
type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Category    string `yaml:"category"`
	Workers     int    `yaml:"workers"`
}

// frontmatterDelimiter is the standard YAML frontmatter delimiter.
const frontmatterDelimiter = "---"

// LoadBuiltinWorkflows loads all built-in workflow templates from the embedded filesystem.
func LoadBuiltinWorkflows() ([]Workflow, error) {
	return loadWorkflowsFromFS(builtinTemplates, "templates", SourceBuiltIn)
}

// loadWorkflowsFromFS loads workflow templates from a filesystem at the given directory path.
func loadWorkflowsFromFS(fsys fs.FS, dir string, source Source) ([]Workflow, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("reading workflow directory: %w", err)
	}

	var workflows []Workflow
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		// Use path.Join (not filepath.Join) for embedded filesystems which always use forward slashes
		fsPath := path.Join(dir, entry.Name())
		content, err := fs.ReadFile(fsys, fsPath)
		if err != nil {
			return nil, fmt.Errorf("reading workflow file %s: %w", fsPath, err)
		}

		wf, err := parseWorkflow(string(content), entry.Name(), source)
		if err != nil {
			// Skip workflows with invalid frontmatter, but log would be nice
			continue
		}

		workflows = append(workflows, wf)
	}

	return workflows, nil
}

// parseWorkflow parses a workflow from its content and filename.
func parseWorkflow(content, filename string, source Source) (Workflow, error) {
	fm, err := parseFrontmatter(content)
	if err != nil {
		return Workflow{}, fmt.Errorf("parsing frontmatter: %w", err)
	}

	// Derive ID from filename (e.g., "debate.md" -> "debate")
	id := strings.TrimSuffix(filename, ".md")

	return Workflow{
		ID:          id,
		Name:        fm.Name,
		Description: fm.Description,
		Category:    fm.Category,
		Workers:     fm.Workers,
		Content:     content,
		Source:      source,
	}, nil
}

// parseFrontmatter extracts and parses YAML frontmatter from markdown content.
// Frontmatter is expected to be at the start of the file, delimited by "---".
func parseFrontmatter(content string) (frontmatter, error) {
	var fm frontmatter

	// Frontmatter must start at the beginning
	if !strings.HasPrefix(content, frontmatterDelimiter) {
		return fm, fmt.Errorf("content does not start with frontmatter delimiter")
	}

	// Find the ending delimiter
	rest := content[len(frontmatterDelimiter):]
	yamlContent, _, found := strings.Cut(rest, "\n"+frontmatterDelimiter)
	if !found {
		return fm, fmt.Errorf("no closing frontmatter delimiter found")
	}

	// Extract the YAML content (skip the leading newline if present)
	yamlContent = strings.TrimPrefix(yamlContent, "\n")

	// Parse the YAML
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(yamlContent)))
	if err := decoder.Decode(&fm); err != nil {
		return fm, fmt.Errorf("parsing YAML: %w", err)
	}

	// Validate required fields
	if fm.Name == "" {
		return fm, fmt.Errorf("frontmatter missing required field: name")
	}

	return fm, nil
}

// ParseWorkflowFile parses a workflow from file content, filename, and optional file path.
// This is useful for loading user-defined workflows from the filesystem.
func ParseWorkflowFile(content, filename, filePath string, source Source) (Workflow, error) {
	wf, err := parseWorkflow(content, filename, source)
	if err != nil {
		return Workflow{}, err
	}
	wf.FilePath = filePath
	return wf, nil
}

// UserWorkflowDir returns the default user workflow directory path.
// Returns ~/.perles/workflows/
func UserWorkflowDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".perles", "workflows")
}

// EnsureUserWorkflowDir creates the user workflow directory if it doesn't exist.
// Returns the directory path and any error encountered.
func EnsureUserWorkflowDir() (string, error) {
	dir := UserWorkflowDir()
	if dir == "" {
		return "", fmt.Errorf("could not determine home directory")
	}

	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", fmt.Errorf("creating workflow directory: %w", err)
	}

	return dir, nil
}

// LoadUserWorkflows loads user-defined workflows from ~/.perles/workflows/.
// Returns an empty slice if the directory doesn't exist (not an error).
// Workflows with invalid frontmatter are skipped (logged as warnings).
func LoadUserWorkflows() ([]Workflow, error) {
	return LoadUserWorkflowsFromDir(UserWorkflowDir())
}

// LoadUserWorkflowsFromDir loads user-defined workflows from a specific directory.
// Returns an empty slice if the directory doesn't exist (not an error).
// Workflows with invalid frontmatter are skipped.
func LoadUserWorkflowsFromDir(dir string) ([]Workflow, error) {
	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist - not an error, just no user workflows
			return nil, nil
		}
		return nil, fmt.Errorf("checking workflow directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workflow path is not a directory: %s", dir)
	}

	// Read directory entries
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading workflow directory: %w", err)
	}

	var workflows []Workflow
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())
		content, err := os.ReadFile(filePath) //nolint:gosec // filePath is constructed from validated directory entries
		if err != nil {
			// Skip files we can't read
			continue
		}

		wf, err := ParseWorkflowFile(string(content), entry.Name(), filePath, SourceUser)
		if err != nil {
			// Skip workflows with invalid frontmatter
			// In production, we'd log a warning here
			continue
		}

		workflows = append(workflows, wf)
	}

	return workflows, nil
}
