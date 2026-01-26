package registry

import (
	"bytes"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
	"text/template"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/config"
	"github.com/zjrosen/perles/internal/registry/domain"
	"github.com/zjrosen/perles/internal/templates"
)

// createTestFS creates a MapFS for testing with workflow subdirectories containing template.yaml and templates
func createTestFS() fstest.MapFS {
	return fstest.MapFS{
		"workflows/planning-standard/template.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "workflow"
    key: "planning-standard"
    version: "v1"
    name: "Standard Planning Workflow"
    description: "Three-phase workflow"
    nodes:
      - key: "research"
        name: "Research"
        template: "v1-research.md"
        outputs:
          - key: "research"
            file: "research.md"
      - key: "propose"
        name: "Propose"
        template: "v1-proposal.md"
        inputs:
          - key: "research"
            file: "research.md"
        outputs:
          - key: "proposal"
            file: "proposal.md"
        after:
          - "research"
      - key: "plan"
        name: "Plan"
        template: "v1-plan.md"
        inputs:
          - key: "proposal"
            file: "proposal.md"
        outputs:
          - key: "plan"
            file: "plan.md"
        after:
          - "propose"
      - key: "eval"
        name: "Evaluation"
        template: "v1-evaluation.md"
        inputs:
          - key: "research"
            file: "research.md"
          - key: "proposal"
            file: "proposal.md"
          - key: "plan"
            file: "plan.md"
        outputs:
          - key: "evaluation"
            file: "evaluation.md"
        after:
          - "plan"
      - key: "implement"
        name: "Implement"
        template: "v1-implement.md"
        inputs:
          - key: "plan"
            file: "plan.md"
        after:
          - "eval"
`),
		},
		"workflows/planning-simple/template.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "workflow"
    key: "planning-simple"
    version: "v1"
    name: "Simple Planning Workflow"
    description: "Two-phase workflow"
    nodes:
      - key: "research-propose"
        name: "Research & Propose"
        template: "v1-research-proposal.md"
        outputs:
          - key: "research"
            file: "research.md"
          - key: "proposal"
            file: "proposal.md"
      - key: "plan"
        name: "Plan"
        template: "v1-plan.md"
        inputs:
          - key: "proposal"
            file: "proposal.md"
        outputs:
          - key: "plan"
            file: "plan.md"
        after:
          - "research-propose"
      - key: "implement"
        name: "Implement"
        template: "v1-implement.md"
        inputs:
          - key: "plan"
            file: "plan.md"
        after:
          - "plan"
`),
		},
		"workflows/go-guidelines/template.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "lang::guidelines"
    key: "go-guidelines"
    version: "v1"
    name: "Go Guidelines"
    description: "Go language guidelines"
    labels:
      - "language"
      - "go"
    nodes:
      - key: "coding"
        name: "Coding Guidelines"
        template: "go-coding.md"
`),
		},
		// Templates in their respective workflow directories
		"workflows/planning-standard/v1-research.md":        &fstest.MapFile{Data: []byte("# Research Template\nContent here")},
		"workflows/planning-standard/v1-proposal.md":        &fstest.MapFile{Data: []byte("# Proposal Template\nContent here")},
		"workflows/planning-standard/v1-plan.md":            &fstest.MapFile{Data: []byte("# Plan Template\nContent here")},
		"workflows/planning-standard/v1-evaluation.md":      &fstest.MapFile{Data: []byte("# Evaluation Template\nContent here")},
		"workflows/planning-standard/v1-implement.md":       &fstest.MapFile{Data: []byte("# Implement Template\nContent here")},
		"workflows/planning-simple/v1-research-proposal.md": &fstest.MapFile{Data: []byte("# Research & Proposal Template\nContent here")},
		"workflows/planning-simple/v1-plan.md":              &fstest.MapFile{Data: []byte("# Plan Template\nContent here")},
		"workflows/planning-simple/v1-implement.md":         &fstest.MapFile{Data: []byte("# Implement Template\nContent here")},
		"workflows/go-guidelines/go-coding.md":              &fstest.MapFile{Data: []byte("# Go Coding Guidelines\nContent here")},
	}
}

// createRegistryServiceWithFS creates a RegistryService from a given filesystem
func createRegistryServiceWithFS(fsys fstest.MapFS) (*RegistryService, error) {
	reg := registry.NewRegistry()

	registrations, err := LoadRegistryFromYAML(fsys)
	if err != nil {
		return nil, err
	}

	for _, r := range registrations {
		_ = reg.Add(r)
	}

	return &RegistryService{
		registry:   reg,
		templateFS: fs.ReadFileFS(fsys),
	}, nil
}

func TestNewRegistryService(t *testing.T) {
	testFS := createTestFS()
	svc, err := createRegistryServiceWithFS(testFS)
	require.NoError(t, err)

	require.NotNil(t, svc)
	require.NotNil(t, svc.registry)
}

func TestRegistryService_List(t *testing.T) {
	testFS := createTestFS()
	svc, err := createRegistryServiceWithFS(testFS)
	require.NoError(t, err)

	list := svc.List()

	require.Len(t, list, 3)

	// Check for standard workflow
	var foundStandard, foundSimple, foundGuidelines bool
	for _, reg := range list {
		if reg.Namespace() == "workflow" && reg.Key() == "planning-standard" {
			foundStandard = true
			require.Equal(t, "v1", reg.Version())
			require.Len(t, reg.DAG().Nodes(), 5)
		}
		if reg.Namespace() == "workflow" && reg.Key() == "planning-simple" {
			foundSimple = true
			require.Equal(t, "v1", reg.Version())
			require.Len(t, reg.DAG().Nodes(), 3)
		}
		if reg.Namespace() == "lang::guidelines" && reg.Key() == "go-guidelines" {
			foundGuidelines = true
			require.Equal(t, "v1", reg.Version())
		}
	}
	require.True(t, foundStandard, "standard workflow not found")
	require.True(t, foundSimple, "simple workflow not found")
	require.True(t, foundGuidelines, "guidelines not found")
}

func TestPlanningStandard_EvalNode(t *testing.T) {
	testFS := createTestFS()
	svc, err := createRegistryServiceWithFS(testFS)
	require.NoError(t, err)

	reg, err := svc.GetByKey("workflow", "planning-standard")
	require.NoError(t, err)

	nodes := reg.DAG().Nodes()

	// Find eval node
	var evalNode *registry.Node
	for _, n := range nodes {
		if n.Key() == "eval" {
			evalNode = n
			break
		}
	}
	require.NotNil(t, evalNode, "eval node should exist")

	// Verify After dependency on plan
	require.Contains(t, evalNode.After(), "plan", "eval should depend on plan")

	// Verify inputs (research.md, proposal.md, plan.md)
	inputNames := make([]string, len(evalNode.Inputs()))
	for i, input := range evalNode.Inputs() {
		inputNames[i] = input.Filename()
	}
	require.ElementsMatch(t, []string{"research.md", "proposal.md", "plan.md"}, inputNames, "eval inputs should be research.md, proposal.md, plan.md")

	// Verify output (evaluation.md)
	require.Len(t, evalNode.Outputs(), 1, "eval should have exactly one output")
	require.Equal(t, "evaluation.md", evalNode.Outputs()[0].Filename(), "eval output should be evaluation.md")

	// Verify template (full path after resolution)
	require.Equal(t, "workflows/planning-standard/v1-evaluation.md", evalNode.Template(), "eval template should be resolved to full path")
}

func TestRegistryService_GetByNamespace(t *testing.T) {
	testFS := createTestFS()
	svc, err := createRegistryServiceWithFS(testFS)
	require.NoError(t, err)

	regs := svc.GetByNamespace("workflow")

	require.Len(t, regs, 2)
}

func TestRegistryService_GetByNamespace_NotFound(t *testing.T) {
	testFS := createTestFS()
	svc, err := createRegistryServiceWithFS(testFS)
	require.NoError(t, err)

	regs := svc.GetByNamespace("nonexistent")

	require.Empty(t, regs)
}

func TestRegistryService_GetByKey(t *testing.T) {
	testFS := createTestFS()
	svc, err := createRegistryServiceWithFS(testFS)
	require.NoError(t, err)

	reg, err := svc.GetByKey("workflow", "planning-simple")

	require.NoError(t, err)
	require.Equal(t, "workflow", reg.Namespace())
	require.Equal(t, "planning-simple", reg.Key())
	require.Equal(t, "v1", reg.Version())
}

func TestRegistryService_GetByKey_NotFound(t *testing.T) {
	testFS := createTestFS()
	svc, err := createRegistryServiceWithFS(testFS)
	require.NoError(t, err)

	_, err = svc.GetByKey("workflow", "nonexistent")
	require.ErrorIs(t, err, registry.ErrNotFound)
}

func TestRegistryService_GetByLabels(t *testing.T) {
	testFS := createTestFS()
	svc, err := createRegistryServiceWithFS(testFS)
	require.NoError(t, err)

	regs := svc.GetByLabels("language")
	require.Len(t, regs, 1)
	require.Equal(t, "go-guidelines", regs[0].Key())

	// Test with multiple labels (AND logic)
	regs = svc.GetByLabels("language", "go")
	require.Len(t, regs, 1)

	// Test with non-matching label
	regs = svc.GetByLabels("nonexistent")
	require.Empty(t, regs)
}

func TestRegistryService_GetTemplate(t *testing.T) {
	testFS := createTestFS()
	svc, err := createRegistryServiceWithFS(testFS)
	require.NoError(t, err)

	tests := []struct {
		name       string
		identifier string
		wantErr    bool
		errType    error
	}{
		{
			name:       "invalid identifier format",
			identifier: "invalid",
			wantErr:    true,
		},
		{
			name:       "registration not found (4 parts)",
			identifier: "nonexistent::type::v1::research",
			wantErr:    true,
		},
		{
			name:       "chain not found",
			identifier: "workflow::planning-standard::v1::nonexistent",
			wantErr:    true,
			errType:    ErrChainNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.GetTemplate(tt.identifier)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errType != nil {
					require.ErrorIs(t, err, tt.errType)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRegistryService_GetTemplate_Success(t *testing.T) {
	testFS := createTestFS()
	svc, err := createRegistryServiceWithFS(testFS)
	require.NoError(t, err)

	// Test that parsing, registration lookup, and template reading all work
	content, err := svc.GetTemplate("workflow::planning-standard::v1::research")
	require.NoError(t, err)
	require.NotEmpty(t, content)
	require.Contains(t, content, "#") // Templates should have markdown headers
}

// Helper to create a test registry
func createTestRegistry() *registry.Registry {
	reg := registry.NewRegistry()

	standardChain, _ := registry.NewChain().
		Node("research", "Research", "v1-research.md").
		Node("propose", "Propose", "v1-proposal.md").
		Node("plan", "Plan", "v1-plan.md").
		Build()
	standard, _ := registry.NewBuilder("workflow").
		Key("planning-standard").
		Version("v1").
		Name("Standard Planning Workflow").
		Description("Three-phase workflow").
		SetChain(standardChain).
		Build()
	_ = reg.Add(standard)

	simpleChain, _ := registry.NewChain().
		Node("research-propose", "Research & Propose", "v1-research-proposal.md").
		Node("plan", "Plan", "v1-plan.md").
		Build()
	simple, _ := registry.NewBuilder("workflow").
		Key("planning-simple").
		Version("v1").
		Name("Simple Planning Workflow").
		Description("Two-phase workflow").
		SetChain(simpleChain).
		Build()
	_ = reg.Add(simple)

	return reg
}

// Helper to create test registry with inputs/outputs for RenderTemplate tests
func createTestRegistryWithArtifacts() *registry.Registry {
	reg := registry.NewRegistry()

	chain, _ := registry.NewChain().
		Node("research", "Research", "test-template.md",
			registry.Outputs("research.md"),
		).
		Node("propose", "Propose", "test-template.md",
			registry.Inputs("research.md"),
			registry.Outputs("proposal.md"),
		).
		Build()

	registration, _ := registry.NewBuilder("workflow").
		Key("test-workflow").
		Version("v1").
		Name("Test Workflow").
		Description("Workflow for testing").
		ArtifactPath("docs").
		SetChain(chain).
		Build()
	_ = reg.Add(registration)

	return reg
}

func TestTemplateContext_WithConfig(t *testing.T) {
	ctx := TemplateContext{
		Slug: "my-feature",
		Config: map[string]string{
			"document_path": "docs/proposals",
		},
	}

	tmpl := template.Must(template.New("test").Parse("Path: {{.Config.document_path}}"))
	var buf bytes.Buffer

	err := tmpl.Execute(&buf, ctx)

	require.NoError(t, err)
	require.Equal(t, "Path: docs/proposals", buf.String())
}

// TestRenderTemplate_ValidContext tests successful rendering with all context fields
func TestRenderTemplate_ValidContext(t *testing.T) {
	// Create test filesystem with template containing Go template syntax
	testFS := fstest.MapFS{
		"test-template.md": &fstest.MapFile{
			Data: []byte("# {{.Name}}\n\nSlug: {{.Slug}}\nInput: {{.Inputs.research}}\nOutput: {{.Outputs.proposal}}"),
		},
	}

	svc := &RegistryService{
		registry:   createTestRegistryWithArtifacts(),
		templateFS: fs.ReadFileFS(testFS),
	}

	ctx := TemplateContext{
		Slug: "my-feature",
		Name: "My Feature",
	}

	result, err := svc.RenderTemplate("workflow::test-workflow::v1::propose", ctx)

	require.NoError(t, err)
	require.Contains(t, result, "# My Feature")
	require.Contains(t, result, "Slug: my-feature")
	require.Contains(t, result, "Input: docs/research.md")
	require.Contains(t, result, "Output: docs/proposal.md")
}

func TestRenderTemplate_ConfigInjection(t *testing.T) {
	testFS := fstest.MapFS{
		"test-template.md": &fstest.MapFile{
			Data: []byte("Config: {{.Config.document_path}}"),
		},
	}

	svc := &RegistryService{
		registry:   createTestRegistryWithArtifacts(),
		templateFS: fs.ReadFileFS(testFS),
	}

	ctx := TemplateContext{
		Slug: "my-feature",
		Config: map[string]string{
			"document_path": "docs/proposals",
		},
	}

	result, err := svc.RenderTemplate("workflow::test-workflow::v1::research", ctx)

	require.NoError(t, err)
	require.Contains(t, result, "Config: docs/proposals")
}

func TestRenderTemplate_ConfigEmpty(t *testing.T) {
	testFS := fstest.MapFS{
		"test-template.md": &fstest.MapFile{
			Data: []byte("Config: {{.Config.document_path}}"),
		},
	}

	svc := &RegistryService{
		registry:   createTestRegistryWithArtifacts(),
		templateFS: fs.ReadFileFS(testFS),
	}

	ctx := TemplateContext{
		Slug:   "my-feature",
		Config: map[string]string{},
	}

	result, err := svc.RenderTemplate("workflow::test-workflow::v1::research", ctx)

	require.NoError(t, err)
	require.Contains(t, result, "Config: <no value>")
}

func TestTemplate_ResearchProposal_ConfigPath(t *testing.T) {
	svc, err := NewRegistryService(templates.RegistryFS(), "")
	require.NoError(t, err)

	ctx := TemplateContext{
		Slug: "test-feature",
		Name: "test-feature",
		Date: "2026-01-26",
		Args: map[string]string{
			"goal": "Test goal",
		},
		Config: config.TemplatesConfig{DocumentPath: "docs/custom"}.ToTemplateConfig(),
	}

	result, err := svc.RenderTemplate("workflow::research-proposal::v1::setup", ctx)

	require.NoError(t, err)
	require.Contains(t, result, "docs/custom/2026-01-26--test-feature/")
	require.Contains(t, result, "docs/custom/2026-01-26--test-feature/research-proposal.md")
}

func TestTemplate_ResearchProposal_DefaultPath(t *testing.T) {
	svc, err := NewRegistryService(templates.RegistryFS(), "")
	require.NoError(t, err)

	ctx := TemplateContext{
		Slug: "test-feature",
		Name: "test-feature",
		Date: "2026-01-26",
		Args: map[string]string{
			"goal": "Test goal",
		},
		Config: config.TemplatesConfig{}.ToTemplateConfig(),
	}

	result, err := svc.RenderTemplate("workflow::research-proposal::v1::setup", ctx)

	require.NoError(t, err)
	require.Contains(t, result, "docs/proposals/2026-01-26--test-feature/")
	require.Contains(t, result, "docs/proposals/2026-01-26--test-feature/research-proposal.md")
}

// TestRenderTemplate_MissingSlug tests error when slug is empty
func TestRenderTemplate_MissingSlug(t *testing.T) {
	testFS := fstest.MapFS{
		"test-template.md": &fstest.MapFile{Data: []byte("# Template")},
	}

	svc := &RegistryService{
		registry:   createTestRegistryWithArtifacts(),
		templateFS: fs.ReadFileFS(testFS),
	}

	ctx := TemplateContext{
		// Slug intentionally empty
		Name: "My Feature",
	}

	_, err := svc.RenderTemplate("workflow::test-workflow::v1::research", ctx)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrSlugRequired)
}

// TestRenderTemplate_ArtifactPathResolution tests that inputs/outputs are correctly mapped
func TestRenderTemplate_ArtifactPathResolution(t *testing.T) {
	testFS := fstest.MapFS{
		"test-template.md": &fstest.MapFile{
			Data: []byte("Research: {{.Inputs.research}}\nProposal: {{.Outputs.proposal}}"),
		},
	}

	svc := &RegistryService{
		registry:   createTestRegistryWithArtifacts(),
		templateFS: fs.ReadFileFS(testFS),
	}

	ctx := TemplateContext{
		Slug: "test-slug",
	}

	// Use "propose" node which has both inputs and outputs
	result, err := svc.RenderTemplate("workflow::test-workflow::v1::propose", ctx)

	require.NoError(t, err)
	require.Contains(t, result, "Research: docs/research.md")
	require.Contains(t, result, "Proposal: docs/proposal.md")
}

// TestRenderTemplate_DynamicArtifactPaths tests that artifact filenames support Go template syntax
func TestRenderTemplate_DynamicArtifactPaths(t *testing.T) {
	testFS := fstest.MapFS{
		"workflows/dynamic-test/template.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "workflow"
    key: "dynamic-test"
    version: "v1"
    name: "Dynamic Test"
    description: "Tests dynamic artifact paths"
    nodes:
      - key: "report"
        name: "Generate Report"
        template: "report.md"
        outputs:
          - key: "report"
            file: "report_{{.Args.app}}_{{.Args.version}}.md"
`),
		},
		"workflows/dynamic-test/report.md": &fstest.MapFile{
			// Use stable key "report" to access the dynamic filename
			Data: []byte("Output: {{.Outputs.report}}"),
		},
	}

	svc, err := NewRegistryService(testFS, "")
	require.NoError(t, err)

	ctx := TemplateContext{
		Slug: "test-slug",
		Args: map[string]string{
			"app":     "myapp",
			"version": "v2",
		},
	}

	result, err := svc.RenderTemplate("workflow::dynamic-test::v1::report", ctx)

	require.NoError(t, err)
	// The stable key "report" maps to the rendered dynamic filename
	require.Contains(t, result, "Output: report_myapp_v2.md")
}

// TestRenderTemplate_DynamicArtifactPaths_WithPrefix tests dynamic paths with artifact path prefix
func TestRenderTemplate_DynamicArtifactPaths_WithPrefix(t *testing.T) {
	testFS := fstest.MapFS{
		"workflows/dynamic-prefix-test/template.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "workflow"
    key: "dynamic-prefix-test"
    version: "v1"
    name: "Dynamic Prefix Test"
    description: "Tests dynamic artifact paths with prefix"
    path: ".spec"
    nodes:
      - key: "report"
        name: "Generate Report"
        template: "report.md"
        outputs:
          - key: "analysis"
            file: "{{.Args.name}}_analysis.md"
`),
		},
		"workflows/dynamic-prefix-test/report.md": &fstest.MapFile{
			// Use stable key "analysis" to access the dynamic filename
			Data: []byte("Output: {{.Outputs.analysis}}"),
		},
	}

	svc, err := NewRegistryService(testFS, "")
	require.NoError(t, err)

	ctx := TemplateContext{
		Slug: "test-slug",
		Args: map[string]string{
			"name": "myfeature",
		},
	}

	result, err := svc.RenderTemplate("workflow::dynamic-prefix-test::v1::report", ctx)

	require.NoError(t, err)
	// Path should include the prefix and rendered filename
	require.Contains(t, result, "Output: .spec/myfeature_analysis.md")
}

// TestRenderTemplate_DynamicArtifactPaths_WithDateArg tests dynamic filenames with date variables
func TestRenderTemplate_DynamicArtifactPaths_WithDateArg(t *testing.T) {
	testFS := fstest.MapFS{
		"workflows/dynamic-date-test/template.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "workflow"
    key: "dynamic-date-test"
    version: "v1"
    name: "Dynamic Date Test"
    description: "Tests dynamic artifact paths with date in filename"
    nodes:
      - key: "report"
        name: "Generate Report"
        template: "report.md"
        outputs:
          - key: "daily_report"
            file: "{{.Date}}-{{.Args.app}}-report.md"
`),
		},
		"workflows/dynamic-date-test/report.md": &fstest.MapFile{
			// Use stable key to access the dynamic filename
			Data: []byte("Output: {{.Outputs.daily_report}}"),
		},
	}

	svc, err := NewRegistryService(testFS, "")
	require.NoError(t, err)

	ctx := TemplateContext{
		Slug: "test-slug",
		Date: "2025-01-22",
		Args: map[string]string{
			"app": "myapp",
		},
	}

	result, err := svc.RenderTemplate("workflow::dynamic-date-test::v1::report", ctx)

	require.NoError(t, err)
	// The dynamic filename should be rendered and accessible via index
	require.Contains(t, result, "Output: 2025-01-22-myapp-report.md")
}

// TestRenderArtifactFilename_NoTemplate tests fast path when no template syntax present
func TestRenderArtifactFilename_NoTemplate(t *testing.T) {
	ctx := TemplateContext{Slug: "test"}

	result := renderArtifactFilename("research.md", ctx)

	require.Equal(t, "research.md", result)
}

// TestRenderArtifactFilename_InvalidTemplate tests graceful fallback on invalid template
func TestRenderArtifactFilename_InvalidTemplate(t *testing.T) {
	ctx := TemplateContext{Slug: "test"}

	// Unclosed template should return original
	result := renderArtifactFilename("{{.Unclosed", ctx)

	require.Equal(t, "{{.Unclosed", result)
}

// TestRenderTemplate_SyntaxError tests error handling for invalid template syntax
func TestRenderTemplate_SyntaxError(t *testing.T) {
	testFS := fstest.MapFS{
		"test-template.md": &fstest.MapFile{
			Data: []byte("Invalid: {{.Unclosed"),
		},
	}

	svc := &RegistryService{
		registry:   createTestRegistryWithArtifacts(),
		templateFS: fs.ReadFileFS(testFS),
	}

	ctx := TemplateContext{
		Slug: "test-slug",
	}

	_, err := svc.RenderTemplate("workflow::test-workflow::v1::research", ctx)

	require.Error(t, err)
	require.Contains(t, err.Error(), "parse template")
}

// TestRenderTemplate_MissingOptionalFields tests that missing optional fields render as empty
func TestRenderTemplate_MissingOptionalFields(t *testing.T) {
	testFS := fstest.MapFS{
		"test-template.md": &fstest.MapFile{
			Data: []byte("Feature: [{{.Name}}]\nSlug: [{{.Slug}}]"),
		},
	}

	svc := &RegistryService{
		registry:   createTestRegistryWithArtifacts(),
		templateFS: fs.ReadFileFS(testFS),
	}

	ctx := TemplateContext{
		Slug: "minimal-slug",
		// Name intentionally empty
	}

	result, err := svc.RenderTemplate("workflow::test-workflow::v1::research", ctx)

	require.NoError(t, err)
	// Empty Name should render as empty between brackets
	require.Contains(t, result, "Feature: []")
	// Slug should render with value
	require.Contains(t, result, "Slug: [minimal-slug]")
}

// TestRenderTemplate_IdentifierErrors tests error cases for invalid identifiers
func TestRenderTemplate_IdentifierErrors(t *testing.T) {
	testFS := fstest.MapFS{
		"test-template.md": &fstest.MapFile{Data: []byte("# Template")},
	}

	svc := &RegistryService{
		registry:   createTestRegistryWithArtifacts(),
		templateFS: fs.ReadFileFS(testFS),
	}

	tests := []struct {
		name       string
		identifier string
		wantErr    string
	}{
		{
			name:       "invalid format",
			identifier: "invalid",
			wantErr:    "parse identifier",
		},
		{
			name:       "registration not found",
			identifier: "workflow::nonexistent::v1::research",
			wantErr:    "find registration",
		},
		{
			name:       "chain not found",
			identifier: "workflow::test-workflow::v1::nonexistent",
			wantErr:    "chain not found",
		},
		{
			name:       "version mismatch",
			identifier: "workflow::test-workflow::v2::research",
			wantErr:    "version mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := TemplateContext{Slug: "test"}
			_, err := svc.RenderTemplate(tt.identifier, ctx)

			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// === GetSystemPromptTemplate tests ===

// createTestChain creates a minimal chain for testing registrations
func createTestChain() *registry.Chain {
	chain, _ := registry.NewChain().
		Node("test", "Test Node", "test.md").
		Build()
	return chain
}

func TestGetSystemPromptTemplate_ReturnsContent(t *testing.T) {
	// Create test FS with system prompt file included
	testFS := fstest.MapFS{
		"workflows/test-wf/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "test-ns"
    key: "test-key"
    version: "v1"
    name: "Test"
    description: "Test workflow"
    system_prompt: "my-system-prompt.md"
    nodes:
      - key: "test"
        name: "Test Node"
        template: "test.md"
        assignee: "worker-1"
`),
		},
		"workflows/test-wf/my-system-prompt.md": &fstest.MapFile{
			Data: []byte("# My System Prompt\n\nContent here."),
		},
		"workflows/test-wf/test.md": &fstest.MapFile{Data: []byte("# Test")},
	}

	svc, err := NewRegistryService(testFS, "")
	require.NoError(t, err)

	// Get the registration from the service
	reg, err := svc.GetByKey("test-ns", "test-key")
	require.NoError(t, err)

	content, err := svc.GetSystemPromptTemplate(reg)
	require.NoError(t, err)
	require.NotEmpty(t, content)
	require.Contains(t, content, "My System Prompt")
}

func TestGetSystemPromptTemplate_ErrorWhenNilRegistration(t *testing.T) {
	testFS := createTestFS()
	svc, err := NewRegistryService(testFS, "")
	require.NoError(t, err)

	content, err := svc.GetSystemPromptTemplate(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "registration is nil")
	require.Empty(t, content)
}

func TestGetSystemPromptTemplate_ErrorWhenEmptySystemPrompt(t *testing.T) {
	testFS := createTestFS()
	svc, err := NewRegistryService(testFS, "")
	require.NoError(t, err)

	// Create a registration without system prompt
	reg, err := registry.NewBuilder("test-ns").
		Key("test-key").
		Version("v1").
		Name("Test").
		SetChain(createTestChain()).
		Build()
	require.NoError(t, err)

	content, err := svc.GetSystemPromptTemplate(reg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "has no system_prompt template specified")
	require.Contains(t, err.Error(), "test-key")
	require.Empty(t, content)
}

func TestGetSystemPromptTemplate_ErrorWhenNotFound(t *testing.T) {
	testFS := createTestFS()
	svc, err := NewRegistryService(testFS, "")
	require.NoError(t, err)

	// Create a registration with system prompt pointing to nonexistent file
	reg, err := registry.NewBuilder("test-ns").
		Key("test-key").
		Version("v1").
		Name("Test").
		SystemPrompt("nonexistent.md").
		SetChain(createTestChain()).
		Build()
	require.NoError(t, err)

	content, err := svc.GetSystemPromptTemplate(reg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read system_prompt template")
	require.Contains(t, err.Error(), "nonexistent.md")
	require.Empty(t, content)
}

// === NewRegistryService with user workflows tests ===

func TestNewRegistryService_LoadsBothSources(t *testing.T) {
	// Create a built-in FS with a workflow
	builtinFS := fstest.MapFS{
		"workflows/builtin/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "workflow"
    key: "builtin-wf"
    version: "v1"
    name: "Built-in Workflow"
    description: "A built-in workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "builtin-step1.md"
`),
		},
		"workflows/builtin/builtin-step1.md": &fstest.MapFile{Data: []byte("# Built-in Step 1")},
	}

	// Create temp directory for user workflows
	tmpDir := t.TempDir()

	// Create user workflows directory structure
	userWorkflowsDir := tmpDir + "/workflows/user-wf"
	require.NoError(t, os.MkdirAll(userWorkflowsDir, 0755))

	// Create user workflow template.yaml
	require.NoError(t, os.WriteFile(userWorkflowsDir+"/template.yaml", []byte(`registry:
  - namespace: "user-workflow"
    key: "user-wf"
    version: "v1"
    name: "User Workflow"
    description: "A user-defined workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "user-step1.md"
`), 0644))

	// Create user template
	require.NoError(t, os.WriteFile(userWorkflowsDir+"/user-step1.md", []byte("# User Step 1"), 0644))

	// Create service with both sources
	svc, err := NewRegistryService(builtinFS, tmpDir)
	require.NoError(t, err)

	// Verify both workflows are loaded
	list := svc.List()
	require.Len(t, list, 2)

	// Check built-in workflow
	builtinReg, err := svc.GetByKey("workflow", "builtin-wf")
	require.NoError(t, err)
	require.Equal(t, registry.SourceBuiltIn, builtinReg.Source())

	// Check user workflow
	userReg, err := svc.GetByKey("user-workflow", "user-wf")
	require.NoError(t, err)
	require.Equal(t, registry.SourceUser, userReg.Source())
}

func TestNewRegistryService_MissingUserDir(t *testing.T) {
	// Create a built-in FS
	builtinFS := fstest.MapFS{
		"workflows/builtin/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "workflow"
    key: "builtin-wf"
    version: "v1"
    name: "Built-in Workflow"
    description: "A built-in workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "builtin-step1.md"
`),
		},
		"workflows/builtin/builtin-step1.md": &fstest.MapFile{Data: []byte("# Built-in Step 1")},
	}

	// Use a nonexistent directory
	svc, err := NewRegistryService(builtinFS, "/nonexistent/path")
	require.NoError(t, err)

	// Verify only built-in workflow is loaded
	list := svc.List()
	require.Len(t, list, 1)
	require.Equal(t, "builtin-wf", list[0].Key())
}

func TestNewRegistryService_EmptyUserDir(t *testing.T) {
	// Create a built-in FS
	builtinFS := fstest.MapFS{
		"workflows/builtin/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "workflow"
    key: "builtin-wf"
    version: "v1"
    name: "Built-in Workflow"
    description: "A built-in workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "builtin-step1.md"
`),
		},
		"workflows/builtin/builtin-step1.md": &fstest.MapFile{Data: []byte("# Built-in Step 1")},
	}

	// Create empty temp directory (no workflows subdirectory)
	tmpDir := t.TempDir()

	// Create service - should succeed with just built-in workflows
	svc, err := NewRegistryService(builtinFS, tmpDir)
	require.NoError(t, err)

	// Verify only built-in workflow is loaded
	list := svc.List()
	require.Len(t, list, 1)
	require.Equal(t, "builtin-wf", list[0].Key())
}

func TestRegistryService_UserWorkflowSource(t *testing.T) {
	// Create a built-in FS
	builtinFS := fstest.MapFS{
		"workflows/builtin/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "workflow"
    key: "builtin-wf"
    version: "v1"
    name: "Built-in Workflow"
    description: "A built-in workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "builtin-step1.md"
`),
		},
		"workflows/builtin/builtin-step1.md": &fstest.MapFile{Data: []byte("# Built-in Step 1")},
	}

	// Create temp directory for user workflows
	tmpDir := t.TempDir()
	userWorkflowsDir := tmpDir + "/workflows/user-wf"
	require.NoError(t, os.MkdirAll(userWorkflowsDir, 0755))

	require.NoError(t, os.WriteFile(userWorkflowsDir+"/template.yaml", []byte(`registry:
  - namespace: "user-workflow"
    key: "user-wf"
    version: "v1"
    name: "User Workflow"
    description: "A user-defined workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "user-step1.md"
`), 0644))
	require.NoError(t, os.WriteFile(userWorkflowsDir+"/user-step1.md", []byte("# User Step 1"), 0644))

	svc, err := NewRegistryService(builtinFS, tmpDir)
	require.NoError(t, err)

	// Verify user workflow is tagged with SourceUser
	userReg, err := svc.GetByKey("user-workflow", "user-wf")
	require.NoError(t, err)
	require.Equal(t, registry.SourceUser, userReg.Source())
	require.Equal(t, "user", userReg.Source().String())
}

func TestRegistryService_ShadowingBehavior(t *testing.T) {
	// Create a built-in FS with a workflow
	builtinFS := fstest.MapFS{
		"workflows/shadow-target/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "workflow"
    key: "shadow-target"
    version: "v1"
    name: "Original Built-in"
    description: "Original workflow to be shadowed"
    nodes:
      - key: "step1"
        name: "Original Step"
        template: "original-step.md"
`),
		},
		"workflows/shadow-target/original-step.md": &fstest.MapFile{Data: []byte("# Original Content")},
	}

	// Create temp directory for user workflows that will shadow
	tmpDir := t.TempDir()
	userWorkflowsDir := tmpDir + "/workflows/shadow-target"
	require.NoError(t, os.MkdirAll(userWorkflowsDir, 0755))

	// Create user workflow with same namespace+key
	require.NoError(t, os.WriteFile(userWorkflowsDir+"/template.yaml", []byte(`registry:
  - namespace: "workflow"
    key: "shadow-target"
    version: "v1"
    name: "User Shadowed"
    description: "User workflow that shadows built-in"
    nodes:
      - key: "step1"
        name: "User Step"
        template: "user-step.md"
`), 0644))
	require.NoError(t, os.WriteFile(userWorkflowsDir+"/user-step.md", []byte("# User Override Content"), 0644))

	svc, err := NewRegistryService(builtinFS, tmpDir)
	require.NoError(t, err)

	// Verify only one workflow with this key exists (shadowing occurred)
	list := svc.List()
	require.Len(t, list, 1)

	// Verify it's the user version
	reg, err := svc.GetByKey("workflow", "shadow-target")
	require.NoError(t, err)
	require.Equal(t, "User Shadowed", reg.Name())
	require.Equal(t, registry.SourceUser, reg.Source())
}

func TestRegistryService_TemplateResolution(t *testing.T) {
	// Create a built-in FS with a workflow
	builtinFS := fstest.MapFS{
		"workflows/builtin-wf/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "workflow"
    key: "builtin-wf"
    version: "v1"
    name: "Built-in Workflow"
    description: "A built-in workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "builtin-step1.md"
`),
		},
		"workflows/builtin-wf/builtin-step1.md": &fstest.MapFile{Data: []byte("# Built-in Content")},
	}

	// Create temp directory for user workflows
	tmpDir := t.TempDir()
	userWorkflowsDir := tmpDir + "/workflows/user-wf"
	require.NoError(t, os.MkdirAll(userWorkflowsDir, 0755))

	require.NoError(t, os.WriteFile(userWorkflowsDir+"/template.yaml", []byte(`registry:
  - namespace: "user-workflow"
    key: "user-wf"
    version: "v1"
    name: "User Workflow"
    description: "A user-defined workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "user-step1.md"
`), 0644))
	require.NoError(t, os.WriteFile(userWorkflowsDir+"/user-step1.md", []byte("# User Content"), 0644))

	svc, err := NewRegistryService(builtinFS, tmpDir)
	require.NoError(t, err)

	// Test that built-in templates resolve from embedded FS
	builtinContent, err := svc.GetTemplate("workflow::builtin-wf::v1::step1")
	require.NoError(t, err)
	require.Contains(t, builtinContent, "Built-in Content")

	// Test that user templates resolve from user FS
	userContent, err := svc.GetTemplate("user-workflow::user-wf::v1::step1")
	require.NoError(t, err)
	require.Contains(t, userContent, "User Content")
}

func TestRegistryService_GetRegistrationFS(t *testing.T) {
	// Create a built-in FS with a workflow
	builtinFS := fstest.MapFS{
		"workflows/builtin-wf/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "workflow"
    key: "builtin-wf"
    version: "v1"
    name: "Built-in Workflow"
    description: "A built-in workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "builtin-step1.md"
`),
		},
		"workflows/builtin-wf/builtin-step1.md": &fstest.MapFile{Data: []byte("# Built-in Content")},
	}

	// Create temp directory for user workflows
	tmpDir := t.TempDir()
	userWorkflowsDir := tmpDir + "/workflows/user-wf"
	require.NoError(t, os.MkdirAll(userWorkflowsDir, 0755))

	require.NoError(t, os.WriteFile(userWorkflowsDir+"/template.yaml", []byte(`registry:
  - namespace: "user-workflow"
    key: "user-wf"
    version: "v1"
    name: "User Workflow"
    description: "A user-defined workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "user-step1.md"
`), 0644))
	require.NoError(t, os.WriteFile(userWorkflowsDir+"/user-step1.md", []byte("# User Content"), 0644))

	svc, err := NewRegistryService(builtinFS, tmpDir)
	require.NoError(t, err)

	// Get registrations
	builtinReg, err := svc.GetByKey("workflow", "builtin-wf")
	require.NoError(t, err)
	userReg, err := svc.GetByKey("user-workflow", "user-wf")
	require.NoError(t, err)

	// Verify getRegistrationFS returns correct FS for each
	builtinRegFS := svc.getRegistrationFS(builtinReg)
	require.Equal(t, builtinFS, builtinRegFS)

	userRegFS := svc.getRegistrationFS(userReg)
	require.NotNil(t, userRegFS)
	require.NotEqual(t, builtinFS, userRegFS) // Should be different FS
}

func TestRegistryService_GetRegistrationFS_FallbackToTemplateFS(t *testing.T) {
	// Create a service without regToFS map (simulates old NewRegistryService path)
	testFS := createTestFS()
	svc, err := createRegistryServiceWithFS(testFS)
	require.NoError(t, err)

	// Get any registration
	reg, err := svc.GetByKey("workflow", "planning-standard")
	require.NoError(t, err)

	// Verify getRegistrationFS returns templateFS when regToFS is nil
	regFS := svc.getRegistrationFS(reg)
	require.NotNil(t, regFS)
}

// === RenderEpicTemplate tests ===

func TestRenderEpicTemplate_BuiltInWorkflow(t *testing.T) {
	// Create a built-in FS with epic template
	builtinFS := fstest.MapFS{
		"workflows/builtin/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "workflow"
    key: "builtin-wf"
    version: "v1"
    name: "Built-in Workflow"
    description: "A built-in workflow"
    epic_template: "workflows/builtin/epic.md"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "builtin-step1.md"
`),
		},
		"workflows/builtin/builtin-step1.md": &fstest.MapFile{Data: []byte("# Built-in Step 1")},
		"workflows/builtin/epic.md": &fstest.MapFile{
			Data: []byte("# Epic: {{.Name}}\nDate: {{.Date}}\nSlug: {{.Slug}}"),
		},
	}

	svc, err := NewRegistryService(builtinFS, "")
	require.NoError(t, err)

	reg, err := svc.GetByKey("workflow", "builtin-wf")
	require.NoError(t, err)

	ctx := TemplateContext{
		Slug: "my-feature",
		Name: "My Feature",
		Date: "2025-01-22",
	}

	result, err := svc.RenderEpicTemplate(reg, ctx)
	require.NoError(t, err)
	require.Contains(t, result, "# Epic: My Feature")
	require.Contains(t, result, "Date: 2025-01-22")
	require.Contains(t, result, "Slug: my-feature")
}

func TestRenderEpicTemplate_ConfigInjection(t *testing.T) {
	builtinFS := fstest.MapFS{
		"workflows/builtin/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "workflow"
    key: "builtin-wf"
    version: "v1"
    name: "Built-in Workflow"
    description: "A built-in workflow"
    epic_template: "workflows/builtin/epic.md"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "builtin-step1.md"
`),
		},
		"workflows/builtin/builtin-step1.md": &fstest.MapFile{Data: []byte("# Built-in Step 1")},
		"workflows/builtin/epic.md": &fstest.MapFile{
			Data: []byte("Config: {{.Config.document_path}}"),
		},
	}

	svc, err := NewRegistryService(builtinFS, "")
	require.NoError(t, err)

	reg, err := svc.GetByKey("workflow", "builtin-wf")
	require.NoError(t, err)

	ctx := TemplateContext{
		Slug: "my-feature",
		Config: map[string]string{
			"document_path": "docs/proposals",
		},
	}

	result, err := svc.RenderEpicTemplate(reg, ctx)
	require.NoError(t, err)
	require.Contains(t, result, "Config: docs/proposals")
}

func TestRenderEpicTemplate_UserWorkflow(t *testing.T) {
	// Create a minimal built-in FS
	builtinFS := fstest.MapFS{
		"workflows/placeholder/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "workflow"
    key: "placeholder"
    version: "v1"
    name: "Placeholder"
    description: "Placeholder workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "placeholder.md"
`),
		},
		"workflows/placeholder/placeholder.md": &fstest.MapFile{Data: []byte("# Placeholder")},
	}

	// Create temp directory for user workflows
	tmpDir := t.TempDir()

	// Create user workflows directory structure
	userWorkflowsDir := tmpDir + "/workflows/user-wf"
	require.NoError(t, os.MkdirAll(userWorkflowsDir, 0755))

	// Create user workflow with epic template
	require.NoError(t, os.WriteFile(userWorkflowsDir+"/template.yaml", []byte(`registry:
  - namespace: "workflow"
    key: "user-wf"
    version: "v1"
    name: "User Workflow"
    description: "A user-defined workflow"
    epic_template: "workflows/user-wf/user-epic.md"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "user-step1.md"
`), 0644))

	// Create user templates
	require.NoError(t, os.WriteFile(userWorkflowsDir+"/user-step1.md", []byte("# User Step 1"), 0644))
	require.NoError(t, os.WriteFile(userWorkflowsDir+"/user-epic.md", []byte("# User Epic: {{.Name}}\nCustom user template for {{.Slug}}"), 0644))

	// Create service with both sources
	svc, err := NewRegistryService(builtinFS, tmpDir)
	require.NoError(t, err)

	// Get user workflow
	reg, err := svc.GetByKey("workflow", "user-wf")
	require.NoError(t, err)
	require.Equal(t, registry.SourceUser, reg.Source())

	ctx := TemplateContext{
		Slug: "user-feature",
		Name: "User Feature",
		Date: "2025-01-22",
	}

	result, err := svc.RenderEpicTemplate(reg, ctx)
	require.NoError(t, err)
	require.Contains(t, result, "# User Epic: User Feature")
	require.Contains(t, result, "Custom user template for user-feature")
}

func TestRenderEpicTemplate_NilRegistration(t *testing.T) {
	builtinFS := fstest.MapFS{
		"workflows/test/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "workflow"
    key: "test"
    version: "v1"
    name: "Test"
    description: "Test workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`),
		},
		"workflows/test/step1.md": &fstest.MapFile{Data: []byte("# Step 1")},
	}

	svc, err := NewRegistryService(builtinFS, "")
	require.NoError(t, err)

	ctx := TemplateContext{Slug: "test"}

	_, err = svc.RenderEpicTemplate(nil, ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "registration is nil")
}

func TestRenderEpicTemplate_NoTemplate(t *testing.T) {
	builtinFS := fstest.MapFS{
		"workflows/test/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "workflow"
    key: "test"
    version: "v1"
    name: "Test"
    description: "Test workflow without template"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`),
		},
		"workflows/test/step1.md": &fstest.MapFile{Data: []byte("# Step 1")},
	}

	svc, err := NewRegistryService(builtinFS, "")
	require.NoError(t, err)

	reg, err := svc.GetByKey("workflow", "test")
	require.NoError(t, err)

	ctx := TemplateContext{Slug: "test"}

	_, err = svc.RenderEpicTemplate(reg, ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "registration has no epic_template")
}

func TestRenderEpicTemplate_SlugRequired(t *testing.T) {
	builtinFS := fstest.MapFS{
		"workflows/test/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "workflow"
    key: "test"
    version: "v1"
    name: "Test"
    description: "Test workflow"
    epic_template: "workflows/test/epic.md"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`),
		},
		"workflows/test/step1.md": &fstest.MapFile{Data: []byte("# Step 1")},
		"workflows/test/epic.md":  &fstest.MapFile{Data: []byte("# Epic")},
	}

	svc, err := NewRegistryService(builtinFS, "")
	require.NoError(t, err)

	reg, err := svc.GetByKey("workflow", "test")
	require.NoError(t, err)

	ctx := TemplateContext{} // No slug

	_, err = svc.RenderEpicTemplate(reg, ctx)
	require.ErrorIs(t, err, ErrSlugRequired)
}
