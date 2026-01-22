package registry

import (
	"fmt"
	"io/fs"
	stdpath "path"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zjrosen/perles/internal/registry/domain"
)

// RegistryFile is the root structure for registry.yaml
type RegistryFile struct {
	Registrations []WorkflowDef `yaml:"registry"`
}

// WorkflowDef defines a single workflow registration in YAML
type WorkflowDef struct {
	Namespace    string    `yaml:"namespace"`    // e.g., "spec-workflow", "lang-guidelines"
	Key          string    `yaml:"key"`          // e.g., "planning-standard"
	Version      string    `yaml:"version"`      // e.g., "v1"
	Name         string    `yaml:"name"`         // Human-readable name
	Description  string    `yaml:"description"`  // Description for AI agents
	Template     string    `yaml:"template"`     // Template filename for epic description
	Instructions string    `yaml:"instructions"` // Template filename for coordinator instructions (required for orchestration workflows)
	Labels       []string  `yaml:"labels"`       // Optional labels for filtering
	Nodes        []NodeDef `yaml:"nodes"`        // Workflow nodes (chain)
}

// NodeDef defines a single node in a workflow chain
type NodeDef struct {
	Key      string   `yaml:"key"`      // Unique identifier within the workflow
	Name     string   `yaml:"name"`     // Human-readable name
	Template string   `yaml:"template"` // Template filename (e.g., "v1-research.md")
	Inputs   []string `yaml:"inputs"`   // Input artifact filenames
	Outputs  []string `yaml:"outputs"`  // Output artifact filenames
	After    []string `yaml:"after"`    // Node keys this node depends on
	Assignee string   `yaml:"assignee"` // Worker role to assign this task to
}

// LoadRegistryFromYAML loads workflow registrations from all registry.yaml files in workflows subdirectories.
// It scans for workflows/*/registry.yaml, parses them, and merges all registrations.
// Template paths are adjusted to be relative to the workflows directory.
func LoadRegistryFromYAML(fsys fs.FS) ([]*registry.Registration, error) {
	var allRegistrations []*registry.Registration

	// Find all registry.yaml files in workflows subdirectories
	err := fs.WalkDir(fsys, "workflows", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Only process registry.yaml files
		if d.IsDir() || d.Name() != "registry.yaml" {
			return nil
		}

		content, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		var file RegistryFile
		if err := yaml.Unmarshal(content, &file); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		// Get the workflow directory for path resolution
		// Use path.Dir (not filepath.Dir) since fs.FS always uses forward slashes
		workflowDir := stdpath.Dir(path)

		for _, def := range file.Registrations {
			// Resolve template paths relative to the workflow directory
			resolvedDef := resolveTemplatePaths(def, workflowDir, fsys)

			reg, err := buildRegistrationFromDef(resolvedDef)
			if err != nil {
				return fmt.Errorf("workflow %s in %s: %w", def.Key, path, err)
			}
			allRegistrations = append(allRegistrations, reg)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan workflow registries: %w", err)
	}

	if len(allRegistrations) == 0 {
		return nil, fmt.Errorf("no workflow registrations found in workflows/*/registry.yaml")
	}

	return allRegistrations, nil
}

// resolveTemplatePaths resolves template paths to be relative to the workflows directory.
// It first checks if the template exists in the workflow's directory, then falls back to workflows/.
func resolveTemplatePaths(def WorkflowDef, workflowDir string, fsys fs.FS) WorkflowDef {
	def.Template = resolveTemplatePath(def.Template, workflowDir, fsys)
	def.Instructions = resolveTemplatePath(def.Instructions, workflowDir, fsys)

	for i := range def.Nodes {
		def.Nodes[i].Template = resolveTemplatePath(def.Nodes[i].Template, workflowDir, fsys)
	}

	return def
}

// resolveTemplatePath resolves a single template path.
// Returns the path relative to the workflows directory.
func resolveTemplatePath(template, workflowDir string, fsys fs.FS) string {
	if template == "" {
		return ""
	}

	// If already a path (contains /), use as-is
	if strings.Contains(template, "/") {
		return template
	}

	// Check if template exists in workflow directory first
	// Use path.Join (not filepath.Join) since fs.FS always uses forward slashes
	workflowPath := stdpath.Join(workflowDir, template)
	if _, err := fs.Stat(fsys, workflowPath); err == nil {
		return workflowPath
	}

	// Fall back to shared templates in workflows/ directory
	sharedPath := stdpath.Join("workflows", template)
	if _, err := fs.Stat(fsys, sharedPath); err == nil {
		return sharedPath
	}

	// Return the workflow-local path as default (will error on load if not found)
	return workflowPath
}

// buildRegistrationFromDef converts a WorkflowDef into a registry.Registration.
func buildRegistrationFromDef(def WorkflowDef) (*registry.Registration, error) {
	// Validate instructions field for orchestration workflows
	if isOrchestrationWorkflow(&def) && def.Instructions == "" {
		return nil, fmt.Errorf("registration %s/%s requires 'instructions' field (orchestration workflows must specify coordinator instructions template)", def.Namespace, def.Key)
	}

	// Build the chain from node definitions
	chainBuilder := registry.NewChain()
	for _, node := range def.Nodes {
		opts := buildNodeOptions(node)
		chainBuilder = chainBuilder.Node(node.Key, node.Name, node.Template, opts...)
	}

	chain, err := chainBuilder.Build()
	if err != nil {
		return nil, fmt.Errorf("build chain: %w", err)
	}

	// Build the registration
	builder := registry.NewBuilder(def.Namespace).
		Key(def.Key).
		Version(def.Version).
		Name(def.Name).
		Description(def.Description).
		Template(def.Template).
		Instructions(def.Instructions).
		SetChain(chain)

	if len(def.Labels) > 0 {
		builder = builder.Labels(def.Labels...)
	}

	return builder.Build()
}

// isOrchestrationWorkflow checks if the workflow definition is an orchestration workflow.
// Orchestration workflows have at least one node with an assignee field.
func isOrchestrationWorkflow(def *WorkflowDef) bool {
	for _, node := range def.Nodes {
		if node.Assignee != "" {
			return true
		}
	}
	return false
}

// buildNodeOptions converts NodeDef inputs/outputs/after/assignee into NodeOption functions.
func buildNodeOptions(node NodeDef) []registry.NodeOption {
	var opts []registry.NodeOption

	if len(node.Inputs) > 0 {
		opts = append(opts, registry.Inputs(node.Inputs...))
	}
	if len(node.Outputs) > 0 {
		opts = append(opts, registry.Outputs(node.Outputs...))
	}
	if len(node.After) > 0 {
		opts = append(opts, registry.After(node.After...))
	}
	if node.Assignee != "" {
		opts = append(opts, registry.Assignee(node.Assignee))
	}

	return opts
}
