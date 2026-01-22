package registry

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/zjrosen/perles/internal/registry/domain"

	"github.com/stretchr/testify/require"
)

// createWorkflowFS wraps YAML content in the expected workflows/test/registry.yaml structure
func createWorkflowFS(yamlContent string) fstest.MapFS {
	return fstest.MapFS{
		"workflows/test/registry.yaml": &fstest.MapFile{
			Data: []byte(yamlContent),
		},
	}
}

func TestLoadRegistryFromYAML(t *testing.T) {
	tests := []struct {
		name        string
		yamlContent string
		wantCount   int
		wantErr     bool
		errContains string
	}{
		{
			name: "valid single workflow",
			yamlContent: `
registry:
  - namespace: "spec-workflow"
    key: "test-workflow"
    version: "v1"
    name: "Test Workflow"
    description: "A test workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
        outputs:
          - "output.md"
`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "valid multiple workflows",
			yamlContent: `
registry:
  - namespace: "spec-workflow"
    key: "workflow1"
    version: "v1"
    name: "Workflow 1"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
  - namespace: "lang::guidelines"
    key: "workflow2"
    version: "v1"
    name: "Workflow 2"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "workflow with all node options",
			yamlContent: `
registry:
  - namespace: "spec-workflow"
    key: "full-options"
    version: "v1"
    name: "Full Options"
    description: "Workflow with all options"
    labels:
      - "test"
      - "example"
    nodes:
      - key: "producer"
        name: "Producer"
        template: "producer.md"
        outputs:
          - "artifact.md"
      - key: "consumer"
        name: "Consumer"
        template: "consumer.md"
        inputs:
          - "artifact.md"
        after:
          - "producer"
`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "workflow with no optional fields",
			yamlContent: `
registry:
  - namespace: "spec-workflow"
    key: "minimal"
    version: "v1"
    name: "Minimal"
    description: ""
    nodes:
      - key: "only"
        name: "Only Step"
        template: "only.md"
`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "empty workflows array",
			yamlContent: `
registry: []
`,
			wantCount:   0,
			wantErr:     true,
			errContains: "no workflow registrations found",
		},
		{
			name: "cycle detection - A after B, B after A",
			yamlContent: `
registry:
  - namespace: "spec-workflow"
    key: "cycle-test"
    version: "v1"
    name: "Cycle Test"
    description: ""
    nodes:
      - key: "nodeA"
        name: "Node A"
        template: "a.md"
        after:
          - "nodeB"
      - key: "nodeB"
        name: "Node B"
        template: "b.md"
        after:
          - "nodeA"
`,
			wantErr:     true,
			errContains: "cycle",
		},
		{
			name: "duplicate node keys",
			yamlContent: `
registry:
  - namespace: "spec-workflow"
    key: "duplicate-keys"
    version: "v1"
    name: "Duplicate Keys"
    description: ""
    nodes:
      - key: "same"
        name: "First"
        template: "first.md"
      - key: "same"
        name: "Second"
        template: "second.md"
`,
			wantErr:     true,
			errContains: "duplicate node",
		},
		{
			name: "duplicate output artifacts",
			yamlContent: `
registry:
  - namespace: "spec-workflow"
    key: "duplicate-outputs"
    version: "v1"
    name: "Duplicate Outputs"
    description: ""
    nodes:
      - key: "node1"
        name: "Node 1"
        template: "node1.md"
        outputs:
          - "output.md"
      - key: "node2"
        name: "Node 2"
        template: "node2.md"
        outputs:
          - "output.md"
`,
			wantErr:     true,
			errContains: "duplicate",
		},
		{
			name: "dangling input reference",
			yamlContent: `
registry:
  - namespace: "spec-workflow"
    key: "dangling-input"
    version: "v1"
    name: "Dangling Input"
    description: ""
    nodes:
      - key: "consumer"
        name: "Consumer"
        template: "consumer.md"
        inputs:
          - "missing.md"
`,
			wantErr:     true,
			errContains: "dangling",
		},
		{
			name: "unknown after reference",
			yamlContent: `
registry:
  - namespace: "spec-workflow"
    key: "unknown-after"
    version: "v1"
    name: "Unknown After"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
        after:
          - "nonexistent"
`,
			wantErr:     true,
			errContains: "unknown",
		},
		{
			name: "missing required field - type",
			yamlContent: `
registry:
  - key: "no-type"
    version: "v1"
    name: "No Type"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`,
			wantErr:     true,
			errContains: "type",
		},
		{
			name: "missing required field - key",
			yamlContent: `
registry:
  - namespace: "spec-workflow"
    version: "v1"
    name: "No Key"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`,
			wantErr:     true,
			errContains: "key",
		},
		{
			name: "missing required field - version",
			yamlContent: `
registry:
  - namespace: "spec-workflow"
    key: "no-version"
    name: "No Version"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`,
			wantErr:     true,
			errContains: "version",
		},
		{
			name: "empty nodes array",
			yamlContent: `
registry:
  - namespace: "spec-workflow"
    key: "no-nodes"
    version: "v1"
    name: "No Nodes"
    description: ""
    nodes: []
`,
			wantErr:     true,
			errContains: "at least one node",
		},
		{
			name: "invalid YAML syntax",
			yamlContent: `
registry:
  - namespace: spec::workflow
    key: [invalid
`,
			wantErr:     true,
			errContains: "parse",
		},
		{
			name: "error message contains workflow key context",
			yamlContent: `
registry:
  - namespace: "spec-workflow"
    key: "specific-workflow-key"
    version: "v1"
    name: "Test"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
        after:
          - "nonexistent"
`,
			wantErr:     true,
			errContains: "specific-workflow-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock filesystem with workflows/test/registry.yaml
			fs := createWorkflowFS(tt.yamlContent)

			registrations, err := LoadRegistryFromYAML(fs)

			if tt.wantErr {
				require.Error(t, err, "LoadRegistryFromYAML() expected error containing %q", tt.errContains)
				require.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.errContains),
					"LoadRegistryFromYAML() error = %q, want error containing %q", err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err, "LoadRegistryFromYAML() unexpected error")
			require.Equal(t, tt.wantCount, len(registrations),
				"LoadRegistryFromYAML() got %d registrations, want %d", len(registrations), tt.wantCount)
		})
	}
}

func TestLoadRegistryFromYAML_FileNotFound(t *testing.T) {
	// Empty filesystem - no workflows directory
	fs := fstest.MapFS{}

	_, err := LoadRegistryFromYAML(fs)
	require.Error(t, err, "LoadRegistryFromYAML() expected error for missing file")
	require.Contains(t, err.Error(), "workflow",
		"LoadRegistryFromYAML() error = %q, want error mentioning workflows", err.Error())
}

func TestLoadRegistryFromYAML_RegistrationDetails(t *testing.T) {
	yamlContent := `
registry:
  - namespace: "spec-workflow"
    key: "test-key"
    version: "v2"
    name: "Test Name"
    description: "Test Description"
    labels:
      - "label1"
      - "label2"
    nodes:
      - key: "producer"
        name: "Producer"
        template: "producer.md"
        outputs:
          - "artifact.md"
      - key: "consumer"
        name: "Consumer"
        template: "consumer.md"
        inputs:
          - "artifact.md"
        after:
          - "producer"
`
	fs := createWorkflowFS(yamlContent)

	registrations, err := LoadRegistryFromYAML(fs)
	require.NoError(t, err, "LoadRegistryFromYAML() unexpected error")
	require.Len(t, registrations, 1, "expected 1 registration")

	reg := registrations[0]

	// Verify registration metadata
	require.Equal(t, "spec-workflow", reg.Namespace(), "Type() mismatch")
	require.Equal(t, "test-key", reg.Key(), "Key() mismatch")
	require.Equal(t, "v2", reg.Version(), "Version() mismatch")
	require.Equal(t, "Test Name", reg.Name(), "Name() mismatch")
	require.Equal(t, "Test Description", reg.Description(), "Description() mismatch")

	// Verify labels
	labels := reg.Labels()
	require.Equal(t, []string{"label1", "label2"}, labels, "Labels() mismatch")

	// Verify chain/DAG
	dag := reg.DAG()
	require.NotNil(t, dag, "DAG() returned nil")

	nodes := dag.Nodes()
	require.Len(t, nodes, 2, "expected 2 nodes")

	// Verify producer node
	producer := nodes[0]
	require.Equal(t, "producer", producer.Key(), "producer Key() mismatch")
	require.Equal(t, "Producer", producer.Name(), "producer Name() mismatch")
	require.Equal(t, "workflows/test/producer.md", producer.Template(), "producer Template() mismatch (should be resolved to full path)")
	outputs := producer.Outputs()
	require.Len(t, outputs, 1, "producer should have 1 output")
	require.Equal(t, "artifact.md", outputs[0].Filename(), "producer output filename mismatch")

	// Verify consumer node
	consumer := nodes[1]
	require.Equal(t, "consumer", consumer.Key(), "consumer Key() mismatch")
	inputs := consumer.Inputs()
	require.Len(t, inputs, 1, "consumer should have 1 input")
	require.Equal(t, "artifact.md", inputs[0].Filename(), "consumer input filename mismatch")
	after := consumer.After()
	require.Equal(t, []string{"producer"}, after, "consumer After() mismatch")

	// Verify DAG dependencies work correctly
	deps := dag.DependenciesOf("consumer")
	require.Len(t, deps, 1, "DependenciesOf(consumer) should have 1 dependency")
	require.Equal(t, "producer", deps[0].Key(), "DependenciesOf(consumer) key mismatch")
}

func TestBuildNodeOptions(t *testing.T) {
	tests := []struct {
		name     string
		node     NodeDef
		wantOpts int
	}{
		{
			name: "no options",
			node: NodeDef{
				Key:      "test",
				Name:     "Test",
				Template: "test.md",
			},
			wantOpts: 0,
		},
		{
			name: "inputs only",
			node: NodeDef{
				Key:      "test",
				Name:     "Test",
				Template: "test.md",
				Inputs:   []string{"input.md"},
			},
			wantOpts: 1,
		},
		{
			name: "outputs only",
			node: NodeDef{
				Key:      "test",
				Name:     "Test",
				Template: "test.md",
				Outputs:  []string{"output.md"},
			},
			wantOpts: 1,
		},
		{
			name: "after only",
			node: NodeDef{
				Key:      "test",
				Name:     "Test",
				Template: "test.md",
				After:    []string{"prev"},
			},
			wantOpts: 1,
		},
		{
			name: "all options",
			node: NodeDef{
				Key:      "test",
				Name:     "Test",
				Template: "test.md",
				Inputs:   []string{"input.md"},
				Outputs:  []string{"output.md"},
				After:    []string{"prev"},
				Assignee: "worker-1",
			},
			wantOpts: 4,
		},
		{
			name: "assignee only",
			node: NodeDef{
				Key:      "test",
				Name:     "Test",
				Template: "test.md",
				Assignee: "research-lead",
			},
			wantOpts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := buildNodeOptions(tt.node)
			require.Len(t, opts, tt.wantOpts, "buildNodeOptions() option count mismatch")

			// Verify options are valid by applying them to a node
			node := registry.NewNode(tt.node.Key, tt.node.Name, tt.node.Template)
			for _, opt := range opts {
				opt(node)
			}

			// Verify inputs were applied
			if len(tt.node.Inputs) > 0 {
				require.Len(t, node.Inputs(), len(tt.node.Inputs), "Inputs not applied correctly")
			}

			// Verify outputs were applied
			if len(tt.node.Outputs) > 0 {
				require.Len(t, node.Outputs(), len(tt.node.Outputs), "Outputs not applied correctly")
			}

			// Verify after was applied
			if len(tt.node.After) > 0 {
				require.Len(t, node.After(), len(tt.node.After), "After not applied correctly")
			}

			// Verify assignee was applied
			if tt.node.Assignee != "" {
				require.Equal(t, tt.node.Assignee, node.Assignee(), "Assignee not applied correctly")
			}
		})
	}
}

func TestYAMLLoader_ParsesInstructionsField(t *testing.T) {
	yamlContent := `
registry:
  - namespace: "spec-workflow"
    key: "with-instructions"
    version: "v1"
    name: "Workflow With Instructions"
    description: "A workflow with instructions"
    instructions: "coordinator_template.md"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
        assignee: "worker-1"
`
	fs := createWorkflowFS(yamlContent)
	// Add the instructions template file
	fs["workflows/test/coordinator_template.md"] = &fstest.MapFile{Data: []byte("# Instructions")}

	registrations, err := LoadRegistryFromYAML(fs)
	require.NoError(t, err, "LoadRegistryFromYAML() unexpected error")
	require.Len(t, registrations, 1, "expected 1 registration")

	reg := registrations[0]
	require.Equal(t, "workflows/test/coordinator_template.md", reg.Instructions(), "Instructions() should return resolved path")
}

func TestYAMLLoader_EmptyInstructions_AllowedForNonWorkflow(t *testing.T) {
	// Non-orchestration workflow (no assignee fields) should not require instructions
	yamlContent := `
registry:
  - namespace: "spec-workflow"
    key: "no-assignee"
    version: "v1"
    name: "Non-Orchestration Workflow"
    description: "A workflow without assignee"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`
	fs := createWorkflowFS(yamlContent)

	registrations, err := LoadRegistryFromYAML(fs)
	require.NoError(t, err, "LoadRegistryFromYAML() should not error for non-orchestration workflows without instructions")
	require.Len(t, registrations, 1, "expected 1 registration")
	require.Empty(t, registrations[0].Instructions(), "Instructions() should be empty")
}

func TestYAMLLoader_EmptyInstructions_ErrorForOrchestrationWorkflow(t *testing.T) {
	// Orchestration workflow (has assignee) without instructions should fail
	yamlContent := `
registry:
  - namespace: "spec-workflow"
    key: "missing-instructions"
    version: "v1"
    name: "Orchestration Without Instructions"
    description: "An orchestration workflow missing instructions"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
        assignee: "worker-1"
`
	fs := createWorkflowFS(yamlContent)

	_, err := LoadRegistryFromYAML(fs)
	require.Error(t, err, "LoadRegistryFromYAML() should error for orchestration workflows without instructions")
	require.Contains(t, err.Error(), "requires 'instructions' field",
		"error should mention 'instructions' field requirement")
	require.Contains(t, err.Error(), "spec-workflow/missing-instructions",
		"error should include namespace/key for context")
}

func TestIsOrchestrationWorkflow_TrueWhenAssigneePresent(t *testing.T) {
	tests := []struct {
		name string
		def  WorkflowDef
	}{
		{
			name: "single node with assignee",
			def: WorkflowDef{
				Nodes: []NodeDef{
					{Key: "step1", Assignee: "worker-1"},
				},
			},
		},
		{
			name: "multiple nodes, first has assignee",
			def: WorkflowDef{
				Nodes: []NodeDef{
					{Key: "step1", Assignee: "worker-1"},
					{Key: "step2"},
				},
			},
		},
		{
			name: "multiple nodes, last has assignee",
			def: WorkflowDef{
				Nodes: []NodeDef{
					{Key: "step1"},
					{Key: "step2", Assignee: "worker-2"},
				},
			},
		},
		{
			name: "multiple nodes, all have assignees",
			def: WorkflowDef{
				Nodes: []NodeDef{
					{Key: "step1", Assignee: "worker-1"},
					{Key: "step2", Assignee: "worker-2"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isOrchestrationWorkflow(&tt.def)
			require.True(t, result, "isOrchestrationWorkflow() should return true when any node has assignee")
		})
	}
}

func TestIsOrchestrationWorkflow_FalseWhenNoAssignee(t *testing.T) {
	tests := []struct {
		name string
		def  WorkflowDef
	}{
		{
			name: "empty nodes array",
			def: WorkflowDef{
				Nodes: []NodeDef{},
			},
		},
		{
			name: "single node without assignee",
			def: WorkflowDef{
				Nodes: []NodeDef{
					{Key: "step1"},
				},
			},
		},
		{
			name: "multiple nodes, none have assignee",
			def: WorkflowDef{
				Nodes: []NodeDef{
					{Key: "step1"},
					{Key: "step2"},
					{Key: "step3"},
				},
			},
		},
		{
			name: "nodes with other fields but no assignee",
			def: WorkflowDef{
				Nodes: []NodeDef{
					{Key: "step1", Name: "Step 1", Template: "step1.md", Inputs: []string{"input.md"}},
					{Key: "step2", Name: "Step 2", Template: "step2.md", Outputs: []string{"output.md"}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isOrchestrationWorkflow(&tt.def)
			require.False(t, result, "isOrchestrationWorkflow() should return false when no node has assignee")
		})
	}
}
