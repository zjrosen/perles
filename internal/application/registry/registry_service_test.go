package registry

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/domain/registry"
)

// createTestFS creates a MapFS for testing with a valid registry.yaml and templates
func createTestFS() fstest.MapFS {
	return fstest.MapFS{
		"registry.yaml": &fstest.MapFile{
			Data: []byte(`
registry:
  - namespace: "spec-workflow"
    key: "planning-standard"
    version: "v1"
    name: "Standard Planning Workflow"
    description: "Three-phase workflow"
    nodes:
      - key: "research"
        name: "Research"
        template: "v1-research.md"
        outputs:
          - "research.md"
      - key: "propose"
        name: "Propose"
        template: "v1-proposal.md"
        inputs:
          - "research.md"
        outputs:
          - "proposal.md"
        after:
          - "research"
      - key: "plan"
        name: "Plan"
        template: "v1-plan.md"
        inputs:
          - "proposal.md"
        outputs:
          - "plan.md"
        after:
          - "propose"
      - key: "eval"
        name: "Evaluation"
        template: "v1-evaluation.md"
        inputs:
          - "research.md"
          - "proposal.md"
          - "plan.md"
        outputs:
          - "evaluation.md"
        after:
          - "plan"
      - key: "implement"
        name: "Implement"
        template: "v1-implement.md"
        inputs:
          - "plan.md"
        after:
          - "eval"
  - namespace: "spec-workflow"
    key: "planning-simple"
    version: "v1"
    name: "Simple Planning Workflow"
    description: "Two-phase workflow"
    nodes:
      - key: "research-propose"
        name: "Research & Propose"
        template: "v1-research-proposal.md"
        outputs:
          - "research.md"
          - "proposal.md"
      - key: "plan"
        name: "Plan"
        template: "v1-plan.md"
        inputs:
          - "proposal.md"
        outputs:
          - "plan.md"
        after:
          - "research-propose"
      - key: "implement"
        name: "Implement"
        template: "v1-implement.md"
        inputs:
          - "plan.md"
        after:
          - "plan"
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
		"v1-research.md":          &fstest.MapFile{Data: []byte("# Research Template\nContent here")},
		"v1-proposal.md":          &fstest.MapFile{Data: []byte("# Proposal Template\nContent here")},
		"v1-plan.md":              &fstest.MapFile{Data: []byte("# Plan Template\nContent here")},
		"v1-research-proposal.md": &fstest.MapFile{Data: []byte("# Research & Proposal Template\nContent here")},
		"v1-evaluation.md":        &fstest.MapFile{Data: []byte("# Evaluation Template\nContent here")},
		"v1-implement.md":         &fstest.MapFile{Data: []byte("# Implement Template\nContent here")},
		"go-coding.md":            &fstest.MapFile{Data: []byte("# Go Coding Guidelines\nContent here")},
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
		if reg.Namespace() == "spec-workflow" && reg.Key() == "planning-standard" {
			foundStandard = true
			require.Equal(t, "v1", reg.Version())
			require.Len(t, reg.DAG().Nodes(), 5)
		}
		if reg.Namespace() == "spec-workflow" && reg.Key() == "planning-simple" {
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

	reg, err := svc.GetByKey("spec-workflow", "planning-standard")
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

	// Verify template
	require.Equal(t, "v1-evaluation.md", evalNode.Template(), "eval template should be v1-evaluation.md")
}

func TestRegistryService_GetByNamespace(t *testing.T) {
	testFS := createTestFS()
	svc, err := createRegistryServiceWithFS(testFS)
	require.NoError(t, err)

	regs := svc.GetByNamespace("spec-workflow")

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

	reg, err := svc.GetByKey("spec-workflow", "planning-simple")

	require.NoError(t, err)
	require.Equal(t, "spec-workflow", reg.Namespace())
	require.Equal(t, "planning-simple", reg.Key())
	require.Equal(t, "v1", reg.Version())
}

func TestRegistryService_GetByKey_NotFound(t *testing.T) {
	testFS := createTestFS()
	svc, err := createRegistryServiceWithFS(testFS)
	require.NoError(t, err)

	_, err = svc.GetByKey("spec-workflow", "nonexistent")
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
			identifier: "spec-workflow::planning-standard::v1::nonexistent",
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
	content, err := svc.GetTemplate("spec-workflow::planning-standard::v1::research")
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
	standard, _ := registry.NewBuilder("spec-workflow").
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
	simple, _ := registry.NewBuilder("spec-workflow").
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

	registration, _ := registry.NewBuilder("spec-workflow").
		Key("test-workflow").
		Version("v1").
		Name("Test Workflow").
		Description("Workflow for testing").
		SetChain(chain).
		Build()
	_ = reg.Add(registration)

	return reg
}

// TestRenderTemplate_ValidContext tests successful rendering with all context fields
func TestRenderTemplate_ValidContext(t *testing.T) {
	// Create test filesystem with template containing Go template syntax
	testFS := fstest.MapFS{
		"test-template.md": &fstest.MapFile{
			Data: []byte("# {{.FeatureName}}\n\nSlug: {{.Slug}}\nInput: {{.Inputs.research}}\nOutput: {{.Outputs.proposal}}"),
		},
	}

	svc := &RegistryService{
		registry:   createTestRegistryWithArtifacts(),
		templateFS: fs.ReadFileFS(testFS),
	}

	ctx := TemplateContext{
		Slug:        "my-feature",
		FeatureName: "My Feature",
	}

	result, err := svc.RenderTemplate("spec-workflow::test-workflow::v1::propose", ctx)

	require.NoError(t, err)
	require.Contains(t, result, "# My Feature")
	require.Contains(t, result, "Slug: my-feature")
	require.Contains(t, result, "Input: .spec/my-feature/research.md")
	require.Contains(t, result, "Output: .spec/my-feature/proposal.md")
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
		FeatureName: "My Feature",
	}

	_, err := svc.RenderTemplate("spec-workflow::test-workflow::v1::research", ctx)

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
	result, err := svc.RenderTemplate("spec-workflow::test-workflow::v1::propose", ctx)

	require.NoError(t, err)
	require.Contains(t, result, "Research: .spec/test-slug/research.md")
	require.Contains(t, result, "Proposal: .spec/test-slug/proposal.md")
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

	_, err := svc.RenderTemplate("spec-workflow::test-workflow::v1::research", ctx)

	require.Error(t, err)
	require.Contains(t, err.Error(), "parse template")
}

// TestRenderTemplate_MissingOptionalFields tests that missing optional fields render as empty
func TestRenderTemplate_MissingOptionalFields(t *testing.T) {
	testFS := fstest.MapFS{
		"test-template.md": &fstest.MapFile{
			Data: []byte("Feature: [{{.FeatureName}}]\nSlug: [{{.Slug}}]"),
		},
	}

	svc := &RegistryService{
		registry:   createTestRegistryWithArtifacts(),
		templateFS: fs.ReadFileFS(testFS),
	}

	ctx := TemplateContext{
		Slug: "minimal-slug",
		// FeatureName intentionally empty
	}

	result, err := svc.RenderTemplate("spec-workflow::test-workflow::v1::research", ctx)

	require.NoError(t, err)
	// Empty FeatureName should render as empty between brackets
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
			identifier: "spec-workflow::nonexistent::v1::research",
			wantErr:    "find registration",
		},
		{
			name:       "chain not found",
			identifier: "spec-workflow::test-workflow::v1::nonexistent",
			wantErr:    "chain not found",
		},
		{
			name:       "version mismatch",
			identifier: "spec-workflow::test-workflow::v2::research",
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
