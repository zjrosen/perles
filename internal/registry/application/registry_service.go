package registry

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"text/template"

	"github.com/zjrosen/perles/internal/registry/domain"
)

// RegistryService errors
var (
	ErrTemplateNotFound = errors.New("template not found")
	ErrChainNotFound    = errors.New("chain not found in registration")
	ErrSlugRequired     = errors.New("slug is required for template rendering")
)

// TemplateContext holds variables for template rendering
type TemplateContext struct {
	Slug        string            // Required: feature slug (e.g., "my-feature")
	FeatureName string            // Optional: human-readable name
	Date        string            // Optional: current date for timestamp
	Inputs      map[string]string // Auto-computed: artifact name → full path
	Outputs     map[string]string // Auto-computed: artifact name → full path
}

// RegistryService handles template registry operations
type RegistryService struct {
	registry            *registry.Registry
	templateFS          fs.FS // Registry templates (from internal/templates)
	workflowTemplatesFS fs.FS // Workflow templates (from internal/orchestration/workflow/templates)
}

// NewRegistryService creates a new registry service with default registrations.
// templateFS contains the registry.yaml and spec workflow templates.
// workflowTemplatesFS contains epic_driven.md and other orchestration templates.
func NewRegistryService(templateFS, workflowTemplatesFS fs.FS) (*RegistryService, error) {
	reg := registry.NewRegistry()

	// Load all registrations from YAML
	registrations, err := LoadRegistryFromYAML(templateFS)
	if err != nil {
		return nil, fmt.Errorf("load registry: %w", err)
	}

	for _, r := range registrations {
		_ = reg.Add(r)
	}

	return &RegistryService{
		registry:            reg,
		templateFS:          templateFS,
		workflowTemplatesFS: workflowTemplatesFS,
	}, nil
}

// List returns all registrations
func (s *RegistryService) List() []*registry.Registration {
	return s.registry.List()
}

// GetByNamespace returns all registrations for a namespace
func (s *RegistryService) GetByNamespace(namespace string) []*registry.Registration {
	return s.registry.GetByNamespace(namespace)
}

// GetByKey returns a specific registration by namespace and key
func (s *RegistryService) GetByKey(namespace, key string) (*registry.Registration, error) {
	return s.registry.GetByKey(namespace, key)
}

// GetByLabels returns registrations matching all specified labels (AND logic)
func (s *RegistryService) GetByLabels(labels ...string) []*registry.Registration {
	return s.registry.GetByLabels(labels...)
}

// findNodeByIdentifier parses an identifier and returns the matching node.
// This handles identifier parsing, registration lookup, version validation, and node finding.
func (s *RegistryService) findNodeByIdentifier(identifier string) (*registry.Node, error) {
	// Parse the identifier
	parts, err := registry.ParseIdentifier(identifier)
	if err != nil {
		return nil, fmt.Errorf("parse identifier: %w", err)
	}

	// Find the registration by namespace and key
	reg, err := s.registry.GetByKey(parts.Namespace, parts.Key)
	if err != nil {
		return nil, fmt.Errorf("find registration: %w", err)
	}

	// Verify version matches
	if reg.Version() != parts.Version {
		return nil, fmt.Errorf("version mismatch: expected %s, got %s", parts.Version, reg.Version())
	}

	// Find the node by key
	for _, node := range reg.DAG().Nodes() {
		if node.Key() == parts.ChainKey {
			return node, nil
		}
	}

	return nil, ErrChainNotFound
}

// GetTemplate retrieves template content by identifier
// Format: {namespace}::{key}::{version}::{chain-key}
// Example: spec-workflow::planning-standard::v1::research
func (s *RegistryService) GetTemplate(identifier string) (string, error) {
	node, err := s.findNodeByIdentifier(identifier)
	if err != nil {
		return "", err
	}

	// Read the template content
	content, err := fs.ReadFile(s.templateFS, node.Template())
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", node.Template(), ErrTemplateNotFound)
	}

	return string(content), nil
}

// RenderTemplate renders a template with the given context.
// Returns error if slug is empty (required field).
func (s *RegistryService) RenderTemplate(identifier string, ctx TemplateContext) (string, error) {
	if ctx.Slug == "" {
		return "", ErrSlugRequired
	}

	// Find the node
	node, err := s.findNodeByIdentifier(identifier)
	if err != nil {
		return "", err
	}

	// Read template content
	content, err := fs.ReadFile(s.templateFS, node.Template())
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", node.Template(), ErrTemplateNotFound)
	}

	// Build artifact paths from node inputs/outputs
	ctx.Inputs = buildArtifactPaths(node.Inputs(), ctx.Slug)
	ctx.Outputs = buildArtifactPaths(node.Outputs(), ctx.Slug)

	// Execute template
	tmpl, err := template.New("").Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

// RenderEpicTemplate renders an epic template (root-level template) with the given context.
// Unlike RenderTemplate which uses node identifiers, this takes a direct template filename.
func (s *RegistryService) RenderEpicTemplate(templateFile string, ctx TemplateContext) (string, error) {
	if ctx.Slug == "" {
		return "", ErrSlugRequired
	}

	// Read template content directly by filename
	content, err := fs.ReadFile(s.templateFS, templateFile)
	if err != nil {
		return "", fmt.Errorf("read epic template %s: %w", templateFile, ErrTemplateNotFound)
	}

	// Execute template
	tmpl, err := template.New("").Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("parse epic template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("execute epic template: %w", err)
	}

	return buf.String(), nil
}

// buildArtifactPaths converts a slice of artifacts to a map of name -> path.
// research.md -> research (key), .spec/slug/research.md (path)
func buildArtifactPaths(artifacts []*registry.Artifact, slug string) map[string]string {
	paths := make(map[string]string)
	for _, a := range artifacts {
		// Strip .md extension for cleaner template syntax
		key := strings.TrimSuffix(a.Filename(), ".md")
		paths[key] = fmt.Sprintf(".spec/%s/%s", slug, a.Filename())
	}
	return paths
}

// GetEpicDrivenTemplate returns the generic coordinator instructions template.
// This is the base prompt for all epic-driven workflows, containing instructions
// for how the coordinator should use MCP tools and follow epic-based workflows.
//
// The template is loaded from "epic_driven.md" in the workflow templates filesystem.
func (s *RegistryService) GetEpicDrivenTemplate() (string, error) {
	if s.workflowTemplatesFS == nil {
		return "", fmt.Errorf("read epic_driven template: workflow templates FS not configured")
	}
	content, err := fs.ReadFile(s.workflowTemplatesFS, "epic_driven.md")
	if err != nil {
		return "", fmt.Errorf("read epic_driven template: %w", err)
	}
	return string(content), nil
}
