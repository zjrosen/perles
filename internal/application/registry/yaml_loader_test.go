package registry

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/zjrosen/perles/internal/domain/registry"

	"github.com/stretchr/testify/require"
)

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
			wantCount: 0,
			wantErr:   false,
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
			// Create mock filesystem with registry.yaml
			fs := fstest.MapFS{
				"registry.yaml": &fstest.MapFile{
					Data: []byte(tt.yamlContent),
				},
			}

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
	// Empty filesystem - no registry.yaml
	fs := fstest.MapFS{}

	_, err := LoadRegistryFromYAML(fs)
	require.Error(t, err, "LoadRegistryFromYAML() expected error for missing file")
	require.Contains(t, err.Error(), "registry.yaml",
		"LoadRegistryFromYAML() error = %q, want error mentioning registry.yaml", err.Error())
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
	fs := fstest.MapFS{
		"registry.yaml": &fstest.MapFile{
			Data: []byte(yamlContent),
		},
	}

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
	require.Equal(t, "producer.md", producer.Template(), "producer Template() mismatch")
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
