package registry

import (
	"fmt"
	"io/fs"
	stdpath "path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zjrosen/perles/internal/log"
	"github.com/zjrosen/perles/internal/registry/domain"
)

// maxYAMLSize is the maximum allowed size for template.yaml files (1MB)
const maxYAMLSize = 1 << 20 // 1MB

// validAssigneePattern matches valid assignee formats: worker-N where N is 1-99, or "human"
var validAssigneePattern = regexp.MustCompile(`^(worker-[1-9][0-9]?|human)$`)

// RegistryFile is the root structure for template.yaml
type RegistryFile struct {
	Registrations []WorkflowDef `yaml:"registry"`
}

// WorkflowDef defines a single workflow registration in YAML
type WorkflowDef struct {
	Namespace    string        `yaml:"namespace"`     // e.g., "workflow", "lang-guidelines"
	Key          string        `yaml:"key"`           // e.g., "planning-standard"
	Version      string        `yaml:"version"`       // e.g., "v1"
	Name         string        `yaml:"name"`          // Human-readable name
	Description  string        `yaml:"description"`   // Description for AI agents
	EpicTemplate string        `yaml:"epic_template"` // Template filename for epic content
	SystemPrompt string        `yaml:"system_prompt"` // Template filename for system prompt content (required for orchestration workflows)
	Path         string        `yaml:"path"`          // Optional path prefix for artifacts (default: ".spec")
	Labels       []string      `yaml:"labels"`        // Optional labels for filtering
	Arguments    []ArgumentDef `yaml:"arguments"`     // User-configurable parameters
	Nodes        []NodeDef     `yaml:"nodes"`         // Workflow nodes (chain)
}

// ArgumentDef defines a user-configurable parameter in YAML
type ArgumentDef struct {
	Key         string   `yaml:"key"`         // Unique identifier (used in templates as {{.Args.key}})
	Label       string   `yaml:"label"`       // Human-readable label for form field
	Description string   `yaml:"description"` // Help text/placeholder for form field
	Type        string   `yaml:"type"`        // Input type: text, number, textarea, select, multi-select
	Required    bool     `yaml:"required"`    // Whether the argument is required
	Default     string   `yaml:"default"`     // Default value (optional)
	Options     []string `yaml:"options"`     // Available choices for select/multi-select types
}

// NodeDef defines a single node in a workflow chain
type NodeDef struct {
	Key      string        `yaml:"key"`      // Unique identifier within the workflow
	Name     string        `yaml:"name"`     // Human-readable name
	Template string        `yaml:"template"` // Template filename (e.g., "v1-research.md")
	Inputs   []ArtifactDef `yaml:"inputs"`   // Input artifacts (string or {key, file} object)
	Outputs  []ArtifactDef `yaml:"outputs"`  // Output artifacts (string or {key, file} object)
	After    []string      `yaml:"after"`    // Node keys this node depends on
	Assignee string        `yaml:"assignee"` // Worker role to assign this task to
}

// ArtifactDef defines an input/output artifact in YAML.
// Requires explicit key and file fields:
//
//	outputs:
//	  - key: "report"
//	    file: "{{.Date}}-report.md"
type ArtifactDef struct {
	Key  string `yaml:"key"`  // Stable key for template access (required)
	File string `yaml:"file"` // Filename, may contain Go template syntax (required)
}

// validateTemplatePath checks if a template path is safe (no path traversal or absolute paths).
// Returns an error if the path attempts path traversal (../) or is an absolute path.
func validateTemplatePath(p string) error {
	if p == "" {
		return nil
	}

	// Check for absolute paths (Unix-style or Windows-style)
	if stdpath.IsAbs(p) || (len(p) >= 2 && p[1] == ':') {
		return fmt.Errorf("invalid template path: %q (absolute paths not allowed)", p)
	}

	// Clean the path and check for path traversal attempts
	cleaned := stdpath.Clean(p)
	if strings.HasPrefix(cleaned, "..") {
		return fmt.Errorf("invalid template path: %q (path traversal attempt)", p)
	}

	// Check for embedded .. sequences that Clean() might not catch
	if strings.Contains(p, "..") {
		return fmt.Errorf("invalid template path: %q (path traversal attempt)", p)
	}

	return nil
}

// validateAssignee checks if an assignee value is a valid worker role.
// Valid roles are: worker-1 through worker-99, or "human" for human checkpoints.
// Returns an error if the assignee is not empty and doesn't match the valid pattern.
func validateAssignee(assignee string) error {
	if assignee == "" {
		return nil
	}
	if !validAssigneePattern.MatchString(assignee) {
		return fmt.Errorf("invalid assignee %q: must be worker-1 through worker-99 or 'human'", assignee)
	}
	return nil
}

// LoadRegistryFromYAML loads workflow registrations from all template.yaml files in workflows subdirectories.
// It scans for workflows/*/template.yaml, parses them, and merges all registrations.
// Template paths are adjusted to be relative to the workflows directory.
// All registrations are tagged with SourceBuiltIn.
func LoadRegistryFromYAML(fsys fs.FS) ([]*registry.Registration, error) {
	return LoadRegistryFromYAMLWithSource(fsys, registry.SourceBuiltIn)
}

// LoadRegistryFromYAMLWithSource loads workflow registrations with a specific source tag.
// This allows distinguishing between built-in and user-defined workflows.
// Includes validation for file size (1MB max), duplicate IDs, assignees, and template existence.
// Any invalid workflow fails the whole load; templates are resolved within fsys only.
func LoadRegistryFromYAMLWithSource(fsys fs.FS, source registry.Source) ([]*registry.Registration, error) {
	return loadRegistryFromYAML(fsys, source, loadOptions{})
}

// loadOptions controls template resolution and error handling in loadRegistryFromYAML.
type loadOptions struct {
	// templateFS is used to resolve, validate, and later read templates. It may layer
	// fallback templates (e.g. built-in shared templates) under the walked FS.
	// Nil means templates are resolved within the walked FS only.
	templateFS fs.FS
	// skipInvalid logs and skips an invalid template.yaml or registration instead of
	// failing the whole load, so one broken workflow doesn't hide the others.
	skipInvalid bool
}

// loadRegistryFromYAML walks fsys for workflows/*/template.yaml files and builds registrations.
func loadRegistryFromYAML(fsys fs.FS, source registry.Source, opts loadOptions) ([]*registry.Registration, error) {
	templateFS := opts.templateFS
	if templateFS == nil {
		templateFS = fsys
	}

	var allRegistrations []*registry.Registration

	// Track seen registrations for duplicate detection within this source
	seen := make(map[string]string) // namespace/key -> file path

	// Find all template.yaml files in workflows subdirectories
	err := fs.WalkDir(fsys, "workflows", func(path string, d fs.DirEntry, err error) error {
		// skip aborts the load, or logs and skips the invalid item when opts.skipInvalid is set.
		skip := func(err error, fields ...any) error {
			if !opts.skipInvalid {
				return err
			}
			fields = append(fields, "path", path, "error", err.Error())
			log.Warn(log.CatConfig, "skipping invalid workflow", fields...)
			return nil
		}

		if err != nil {
			// The walk root itself must be readable; unreadable entries below it can be skipped.
			if path == "workflows" {
				return err
			}
			return skip(err)
		}

		// Only process template.yaml files
		if d.IsDir() || d.Name() != "template.yaml" {
			return nil
		}

		// Check file size limit
		info, err := d.Info()
		if err != nil {
			return skip(fmt.Errorf("stat %s: %w", path, err))
		}
		if info.Size() > maxYAMLSize {
			log.Warn(log.CatConfig, "skipping oversized template.yaml",
				"path", path,
				"size", info.Size(),
				"maxSize", maxYAMLSize)
			return nil
		}

		content, err := fs.ReadFile(fsys, path)
		if err != nil {
			return skip(fmt.Errorf("read %s: %w", path, err))
		}

		var file RegistryFile
		if err := yaml.Unmarshal(content, &file); err != nil {
			return skip(fmt.Errorf("parse %s: %w", path, err))
		}

		// Get the workflow directory for path resolution
		// Use path.Dir (not filepath.Dir) since fs.FS always uses forward slashes
		workflowDir := stdpath.Dir(path)

		for _, def := range file.Registrations {
			// Check for duplicate namespace+key within this source
			regKey := def.Namespace + "/" + def.Key
			if firstPath, exists := seen[regKey]; exists {
				log.Warn(log.CatConfig, "duplicate registration",
					"key", regKey,
					"first", firstPath,
					"duplicate", path)
			}
			seen[regKey] = path

			reg, err := buildRegistrationFromYAMLDef(def, path, workflowDir, fsys, templateFS, source)
			if err != nil {
				if err := skip(err, "key", regKey); err != nil {
					return err
				}
				continue
			}
			allRegistrations = append(allRegistrations, reg)

			log.Debug(log.CatConfig, "loaded workflow registration",
				"key", regKey,
				"source", source.String(),
				"path", path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan workflow registries: %w", err)
	}

	if len(allRegistrations) == 0 {
		return nil, fmt.Errorf("no workflow registrations found in workflows/*/template.yaml")
	}

	return allRegistrations, nil
}

// buildRegistrationFromYAMLDef validates a workflow definition from the template.yaml at path,
// resolves its template paths, and builds the registration.
// Workflow-local templates are looked up in fsys; shared templates and existence checks use templateFS.
func buildRegistrationFromYAMLDef(def WorkflowDef, path, workflowDir string, fsys, templateFS fs.FS, source registry.Source) (*registry.Registration, error) {
	// Set default system_prompt for orchestration workflows if not specified
	if isOrchestrationWorkflow(&def) && def.SystemPrompt == "" {
		def.SystemPrompt = defaultSystemPrompt
	}

	// Validate template paths before resolution
	if err := validateTemplatePath(def.EpicTemplate); err != nil {
		return nil, fmt.Errorf("workflow %s/%s in %s: %w", def.Namespace, def.Key, path, err)
	}
	if err := validateTemplatePath(def.SystemPrompt); err != nil {
		return nil, fmt.Errorf("workflow %s/%s in %s: system_prompt: %w", def.Namespace, def.Key, path, err)
	}
	for i, node := range def.Nodes {
		if err := validateTemplatePath(node.Template); err != nil {
			return nil, fmt.Errorf("workflow %s/%s node %d in %s: %w", def.Namespace, def.Key, i, path, err)
		}
		if err := validateAssignee(node.Assignee); err != nil {
			return nil, fmt.Errorf("workflow %s/%s node %d in %s: %w", def.Namespace, def.Key, i, path, err)
		}
	}

	// Resolve template paths relative to the workflow directory
	resolvedDef := resolveTemplatePaths(def, workflowDir, fsys, templateFS)

	// Validate template existence at load time
	if err := validateTemplateExists(templateFS, resolvedDef); err != nil {
		return nil, fmt.Errorf("workflow %s/%s in %s: %w", def.Namespace, def.Key, path, err)
	}

	reg, err := buildRegistrationFromDefWithSource(resolvedDef, source)
	if err != nil {
		return nil, fmt.Errorf("workflow %s in %s: %w", def.Key, path, err)
	}
	return reg, nil
}

// validateTemplateExists checks that all template files referenced in a workflow definition exist.
// This catches missing templates at load time rather than at render time.
func validateTemplateExists(fsys fs.FS, def WorkflowDef) error {
	// Check initial prompt template (may be empty for orchestration-only workflows)
	if def.EpicTemplate != "" {
		if _, err := fs.Stat(fsys, def.EpicTemplate); err != nil {
			return fmt.Errorf("epic_template %q not found", def.EpicTemplate)
		}
	}

	// Check system prompt template (may be empty for non-orchestration workflows)
	if def.SystemPrompt != "" {
		if _, err := fs.Stat(fsys, def.SystemPrompt); err != nil {
			return fmt.Errorf("system_prompt %q not found", def.SystemPrompt)
		}
	}

	// Check all node templates
	for _, node := range def.Nodes {
		if node.Template != "" {
			if _, err := fs.Stat(fsys, node.Template); err != nil {
				return fmt.Errorf("node %q template %q not found", node.Key, node.Template)
			}
		}
	}

	return nil
}

// resolveTemplatePaths resolves template paths to be relative to the workflows directory.
// It first checks if the template exists in the workflow's directory, then falls back to workflows/.
func resolveTemplatePaths(def WorkflowDef, workflowDir string, fsys, templateFS fs.FS) WorkflowDef {
	def.EpicTemplate = resolveTemplatePath(def.EpicTemplate, workflowDir, fsys, templateFS)
	def.SystemPrompt = resolveTemplatePath(def.SystemPrompt, workflowDir, fsys, templateFS)

	for i := range def.Nodes {
		def.Nodes[i].Template = resolveTemplatePath(def.Nodes[i].Template, workflowDir, fsys, templateFS)
	}

	return def
}

// resolveTemplatePath resolves a single template path.
// Returns the path relative to the workflows directory.
// Bare filenames are looked up in the workflow's own directory in fsys, then in the
// shared workflows/ directory of templateFS (fsys, possibly layered over built-in templates),
// so workflow-local files always win over shared ones.
func resolveTemplatePath(template, workflowDir string, fsys, templateFS fs.FS) string {
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
	if _, err := fs.Stat(templateFS, sharedPath); err == nil {
		return sharedPath
	}

	// Return the workflow-local path as default (will error on load if not found in templateFS)
	return workflowPath
}

// defaultSystemPrompt is the default system prompt template for orchestration workflows.
const defaultSystemPrompt = "v1-epic-instructions.md"

// buildRegistrationFromDefWithSource converts a WorkflowDef into a registry.Registration with a specific source.
func buildRegistrationFromDefWithSource(def WorkflowDef, source registry.Source) (*registry.Registration, error) {
	// Build the chain from node definitions.
	// Skip for workflows with no nodes:
	//   - Epic-driven workflows (existing epic, no pre-created tasks)
	//   - Collaborative workflows (creates epic but tasks emerge from discussion)
	// Non-epic-driven zero-node workflows must have a system_prompt to be valid.
	var chain *registry.Chain
	if len(def.Nodes) == 0 && !isEpicDrivenWorkflow(&def) && def.SystemPrompt == "" {
		return nil, fmt.Errorf("workflow must have at least one node, or a system_prompt for node-less orchestration")
	}
	if len(def.Nodes) > 0 {
		chainBuilder := registry.NewChain()
		for i, node := range def.Nodes {
			opts, err := buildNodeOptions(node)
			if err != nil {
				return nil, fmt.Errorf("node %d (%s): %w", i, node.Key, err)
			}
			chainBuilder = chainBuilder.Node(node.Key, node.Name, node.Template, opts...)
		}

		var err error
		chain, err = chainBuilder.Build()
		if err != nil {
			return nil, fmt.Errorf("build chain: %w", err)
		}
	}

	// Build arguments from definitions
	arguments, err := buildArguments(def.Arguments)
	if err != nil {
		return nil, fmt.Errorf("build arguments: %w", err)
	}

	// Build the registration
	builder := registry.NewBuilder(def.Namespace).
		Key(def.Key).
		Version(def.Version).
		Name(def.Name).
		Description(def.Description).
		EpicTemplate(def.EpicTemplate).
		SystemPrompt(def.SystemPrompt).
		ArtifactPath(def.Path).
		Source(source).
		SetChain(chain)

	if len(def.Labels) > 0 {
		builder = builder.Labels(def.Labels...)
	}

	if len(arguments) > 0 {
		builder = builder.Arguments(arguments...)
	}

	return builder.Build()
}

// buildArguments converts ArgumentDef slice to domain Argument slice with validation.
func buildArguments(defs []ArgumentDef) ([]*registry.Argument, error) {
	if len(defs) == 0 {
		return nil, nil
	}

	args := make([]*registry.Argument, 0, len(defs))
	seen := make(map[string]bool)

	for i, def := range defs {
		// Check for duplicate keys
		if seen[def.Key] {
			return nil, fmt.Errorf("argument %d: duplicate key %q", i, def.Key)
		}
		seen[def.Key] = true

		arg, err := registry.NewArgumentWithOptions(
			def.Key,
			def.Label,
			def.Description,
			registry.ArgumentType(def.Type),
			def.Required,
			def.Default,
			def.Options,
		)
		if err != nil {
			return nil, fmt.Errorf("argument %d (%s): %w", i, def.Key, err)
		}

		args = append(args, arg)
	}

	return args, nil
}

// isOrchestrationWorkflow checks if the workflow definition is an orchestration workflow.
// Orchestration workflows have at least one node with an assignee field, OR are epic-driven,
// OR have a system_prompt with no nodes (collaborative workflows that create an epic without pre-defined tasks).
func isOrchestrationWorkflow(def *WorkflowDef) bool {
	// Epic-driven workflows are orchestration workflows
	if isEpicDrivenWorkflow(def) {
		return true
	}
	// Zero-node workflows with a system_prompt are orchestration workflows
	// (e.g., collaborative planning that creates an epic but tasks emerge from discussion)
	if len(def.Nodes) == 0 && def.SystemPrompt != "" {
		return true
	}
	// Standard orchestration: at least one node with assignee
	for _, node := range def.Nodes {
		if node.Assignee != "" {
			return true
		}
	}
	return false
}

// isEpicDrivenWorkflow checks if the workflow uses an existing epic from the tracker.
// An epic-driven workflow has a single "epic_id" argument and no nodes.
func isEpicDrivenWorkflow(def *WorkflowDef) bool {
	// Must have exactly one argument named "epic_id"
	if len(def.Arguments) != 1 {
		return false
	}
	if def.Arguments[0].Key != "epic_id" {
		return false
	}
	// Must have no nodes
	return len(def.Nodes) == 0
}

// buildNodeOptions converts NodeDef inputs/outputs/after/assignee into NodeOption functions.
// Returns error if artifact validation fails.
func buildNodeOptions(node NodeDef) ([]registry.NodeOption, error) {
	var opts []registry.NodeOption

	if len(node.Inputs) > 0 {
		artifacts, err := buildArtifactsFromDefs(node.Inputs)
		if err != nil {
			return nil, fmt.Errorf("inputs: %w", err)
		}
		opts = append(opts, registry.InputArtifacts(artifacts...))
	}
	if len(node.Outputs) > 0 {
		artifacts, err := buildArtifactsFromDefs(node.Outputs)
		if err != nil {
			return nil, fmt.Errorf("outputs: %w", err)
		}
		opts = append(opts, registry.OutputArtifacts(artifacts...))
	}
	if len(node.After) > 0 {
		opts = append(opts, registry.After(node.After...))
	}
	if node.Assignee != "" {
		opts = append(opts, registry.Assignee(node.Assignee))
	}

	return opts, nil
}

// buildArtifactsFromDefs converts ArtifactDef slice to Artifact slice.
// Returns error if any artifact is missing required key or file fields.
func buildArtifactsFromDefs(defs []ArtifactDef) ([]*registry.Artifact, error) {
	artifacts := make([]*registry.Artifact, len(defs))
	for i, def := range defs {
		if def.Key == "" {
			return nil, fmt.Errorf("artifact %d: key is required", i)
		}
		if def.File == "" {
			return nil, fmt.Errorf("artifact %d (%s): file is required", i, def.Key)
		}
		artifacts[i] = registry.NewArtifact(def.Key, def.File)
	}
	return artifacts, nil
}
