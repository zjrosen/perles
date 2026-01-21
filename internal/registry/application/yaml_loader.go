package registry

import (
	"fmt"
	"io/fs"

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

// LoadRegistryFromYAML loads workflow registrations from the embedded registry.yaml file.
// It parses the YAML and builds registrations using the existing ChainBuilder and Builder APIs.
func LoadRegistryFromYAML(fsys fs.FS) ([]*registry.Registration, error) {
	content, err := fs.ReadFile(fsys, "registry.yaml")
	if err != nil {
		return nil, fmt.Errorf("read registry.yaml: %w", err)
	}

	var file RegistryFile
	if err := yaml.Unmarshal(content, &file); err != nil {
		return nil, fmt.Errorf("parse registry.yaml: %w", err)
	}

	registrations := make([]*registry.Registration, 0, len(file.Registrations))
	for _, def := range file.Registrations {
		reg, err := buildRegistrationFromDef(def)
		if err != nil {
			return nil, fmt.Errorf("workflow %s: %w", def.Key, err)
		}
		registrations = append(registrations, reg)
	}

	return registrations, nil
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
