package registry

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"text/template"

	"github.com/zjrosen/perles/internal/log"
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
	Slug    string            // Required: feature slug (e.g., "my-feature")
	Name    string            // Optional: human-readable name
	Date    string            // Optional: current date for timestamp
	Inputs  map[string]string // Auto-computed: artifact name → full path
	Outputs map[string]string // Auto-computed: artifact name → full path
	Args    map[string]string // User-provided argument values (key → value)
}

// regKey is a struct key for per-registration FS tracking.
// Using a struct key avoids delimiter collision risks with string concatenation.
type regKey struct {
	namespace string
	key       string
}

// RegistryService handles template registry operations
type RegistryService struct {
	registry   *registry.Registry
	templateFS fs.FS            // Primary FS (embedded templates from internal/templates)
	userFS     fs.FS            // User FS (may be nil if no user workflows)
	regToFS    map[regKey]fs.FS // Per-registration FS tracking for template resolution
}

// NewRegistryService creates a registry service loading both embedded and
// user-defined workflows. User workflows can shadow built-in workflows by
// having the same namespace+key.
//
// Parameters:
//   - embeddedFS: The embedded filesystem containing built-in workflows
//   - userBaseDir: The base directory for user workflows (e.g., ~/.perles).
//     If empty or the directory doesn't exist, only built-in workflows are loaded.
func NewRegistryService(embeddedFS fs.FS, userBaseDir string) (*RegistryService, error) {
	reg := registry.NewRegistry()
	regToFS := make(map[regKey]fs.FS)

	// 1. Load built-in workflows with SourceBuiltIn
	builtins, err := LoadRegistryFromYAMLWithSource(embeddedFS, registry.SourceBuiltIn)
	if err != nil {
		return nil, fmt.Errorf("load built-in registrations: %w", err)
	}
	for _, r := range builtins {
		_ = reg.Add(r)
		regToFS[regKey{namespace: r.Namespace(), key: r.Key()}] = embeddedFS
	}

	svc := &RegistryService{
		registry:   reg,
		templateFS: embeddedFS,
		regToFS:    regToFS,
	}

	// 2. Load user workflows (if directory exists)
	if userBaseDir != "" {
		userRegs, userFS, err := LoadUserRegistryFromDir(userBaseDir)
		if err != nil {
			return nil, fmt.Errorf("load user registrations: %w", err)
		}
		if userFS != nil {
			svc.userFS = userFS
			for _, r := range userRegs {
				replaced := reg.AddOrReplace(r)
				if replaced != nil {
					log.Info(log.CatConfig, "user workflow shadowing built-in",
						"namespace", r.Namespace(), "key", r.Key())
				}
				regToFS[regKey{namespace: r.Namespace(), key: r.Key()}] = userFS
			}
		}
	}

	return svc, nil
}

// getRegistrationFS returns the filesystem to use for a given registration.
// User workflows use their source FS, built-in workflows use the embedded FS.
func (s *RegistryService) getRegistrationFS(reg *registry.Registration) fs.FS {
	if s.regToFS == nil {
		return s.templateFS
	}
	key := regKey{namespace: reg.Namespace(), key: reg.Key()}
	if regFS, ok := s.regToFS[key]; ok {
		return regFS
	}
	return s.templateFS
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

// findRegistrationAndNode parses an identifier and returns both the matching registration and node.
// This is used when template resolution needs the registration to determine the correct FS.
func (s *RegistryService) findRegistrationAndNode(identifier string) (*registry.Registration, *registry.Node, error) {
	// Parse the identifier
	parts, err := registry.ParseIdentifier(identifier)
	if err != nil {
		return nil, nil, fmt.Errorf("parse identifier: %w", err)
	}

	// Find the registration by namespace and key
	reg, err := s.registry.GetByKey(parts.Namespace, parts.Key)
	if err != nil {
		return nil, nil, fmt.Errorf("find registration: %w", err)
	}

	// Verify version matches
	if reg.Version() != parts.Version {
		return nil, nil, fmt.Errorf("version mismatch: expected %s, got %s", parts.Version, reg.Version())
	}

	// Find the node by key
	for _, node := range reg.DAG().Nodes() {
		if node.Key() == parts.ChainKey {
			return reg, node, nil
		}
	}

	return nil, nil, ErrChainNotFound
}

// GetTemplate retrieves template content by identifier
// Format: {namespace}::{key}::{version}::{chain-key}
// Example: workflow::planning-standard::v1::research
func (s *RegistryService) GetTemplate(identifier string) (string, error) {
	reg, node, err := s.findRegistrationAndNode(identifier)
	if err != nil {
		return "", err
	}

	// Read the template content from the correct FS for this registration
	regFS := s.getRegistrationFS(reg)
	content, err := fs.ReadFile(regFS, node.Template())
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

	// Find the registration and node
	reg, node, err := s.findRegistrationAndNode(identifier)
	if err != nil {
		return "", err
	}

	// Read template content from the correct FS for this registration
	regFS := s.getRegistrationFS(reg)
	content, err := fs.ReadFile(regFS, node.Template())
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", node.Template(), ErrTemplateNotFound)
	}

	// Build artifact paths from node inputs/outputs
	// Note: ctx.Inputs/Outputs are populated after this, so template rendering
	// within artifact filenames can use {{.Slug}}, {{.Date}}, {{.Args.x}} etc.
	artifactPath := reg.ArtifactPath()
	ctx.Inputs = buildArtifactPaths(node.Inputs(), artifactPath, ctx)
	ctx.Outputs = buildArtifactPaths(node.Outputs(), artifactPath, ctx)

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
// Unlike RenderTemplate which uses node identifiers, this takes the registration and context.
// The registration is used to determine which filesystem to read the template from
// (built-in vs user-defined workflows).
func (s *RegistryService) RenderEpicTemplate(reg *registry.Registration, ctx TemplateContext) (string, error) {
	if ctx.Slug == "" {
		return "", ErrSlugRequired
	}
	if reg == nil {
		return "", fmt.Errorf("registration is nil")
	}

	templateFile := reg.Template()
	if templateFile == "" {
		return "", fmt.Errorf("registration has no template")
	}

	// Use the correct filesystem for this registration (built-in vs user)
	regFS := s.getRegistrationFS(reg)

	// Read template content directly by filename
	content, err := fs.ReadFile(regFS, templateFile)
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

// buildArtifactPaths converts a slice of artifacts to a map of key -> rendered path.
// Artifact filenames can contain Go template syntax (e.g., "{{.Date}}-{{.Args.app}}-report.md")
// which is rendered using the provided context.
//
// The artifact's Key() is used as the map key, providing a stable identifier for template access.
// If pathPrefix is empty, the path is just the rendered filename.
func buildArtifactPaths(artifacts []*registry.Artifact, pathPrefix string, ctx TemplateContext) map[string]string {
	paths := make(map[string]string)
	for _, a := range artifacts {
		// Render filename template (supports {{.Date}}, {{.Args.x}}, etc.)
		filename := renderArtifactFilename(a.Filename(), ctx)

		// Use the artifact's stable key for template access
		key := a.Key()
		if pathPrefix == "" {
			paths[key] = filename
		} else {
			paths[key] = fmt.Sprintf("%s/%s", pathPrefix, filename)
		}
	}
	return paths
}

// renderArtifactFilename renders a filename template with the given context.
// If the filename contains no template syntax or rendering fails, returns the original filename.
func renderArtifactFilename(filename string, ctx TemplateContext) string {
	// Fast path: no template syntax
	if !strings.Contains(filename, "{{") {
		return filename
	}

	tmpl, err := template.New("").Parse(filename)
	if err != nil {
		return filename // Return original on parse error
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return filename // Return original on execution error
	}

	return buf.String()
}

// GetInstructionsTemplate returns coordinator instructions for a registration.
// The registration must have a non-empty Instructions() field.
func (s *RegistryService) GetInstructionsTemplate(reg *registry.Registration) (string, error) {
	if reg == nil {
		return "", fmt.Errorf("registration is nil")
	}
	if reg.Instructions() == "" {
		return "", fmt.Errorf("registration %s has no instructions template specified", reg.Key())
	}

	// Read from the correct FS for this registration
	regFS := s.getRegistrationFS(reg)
	content, err := fs.ReadFile(regFS, reg.Instructions())
	if err != nil {
		return "", fmt.Errorf("read instructions template %q: %w", reg.Instructions(), err)
	}

	return string(content), nil
}
