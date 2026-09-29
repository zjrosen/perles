package registry

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/zjrosen/perles/internal/registry/domain"

	"github.com/stretchr/testify/require"
)

// createWorkflowFS wraps YAML content in the expected workflows/test/template.yaml structure.
// Optionally include template files to pass template existence validation.
func createWorkflowFS(yamlContent string, templateFiles ...string) fstest.MapFS {
	fs := fstest.MapFS{
		"workflows/test/template.yaml": &fstest.MapFile{
			Data: []byte(yamlContent),
		},
	}
	// Add template files
	for _, tf := range templateFiles {
		fs["workflows/test/"+tf] = &fstest.MapFile{Data: []byte("# " + tf)}
	}
	return fs
}

// createWorkflowFSWithTemplates creates a minimal valid workflow FS with templates.
func createWorkflowFSWithTemplates(yamlContent string) fstest.MapFS {
	return createWorkflowFS(yamlContent,
		"step1.md", "step2.md", "only.md",
		"producer.md", "consumer.md", "artifact.md",
		"a.md", "b.md", "first.md", "second.md",
		"node1.md", "node2.md",
	)
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
  - namespace: "workflow"
    key: "test-workflow"
    version: "v1"
    name: "Test Workflow"
    description: "A test workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
        outputs:
          - key: "output"
            file: "output.md"
`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "valid multiple workflows",
			yamlContent: `
registry:
  - namespace: "workflow"
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
  - namespace: "workflow"
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
          - key: "artifact"
            file: "artifact.md"
      - key: "consumer"
        name: "Consumer"
        template: "consumer.md"
        inputs:
          - key: "artifact"
            file: "artifact.md"
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
  - namespace: "workflow"
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
  - namespace: "workflow"
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
  - namespace: "workflow"
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
  - namespace: "workflow"
    key: "duplicate-outputs"
    version: "v1"
    name: "Duplicate Outputs"
    description: ""
    nodes:
      - key: "node1"
        name: "Node 1"
        template: "node1.md"
        outputs:
          - key: "output"
            file: "output.md"
      - key: "node2"
        name: "Node 2"
        template: "node2.md"
        outputs:
          - key: "output"
            file: "output.md"
`,
			wantErr:     true,
			errContains: "duplicate",
		},
		{
			name: "dangling input reference",
			yamlContent: `
registry:
  - namespace: "workflow"
    key: "dangling-input"
    version: "v1"
    name: "Dangling Input"
    description: ""
    nodes:
      - key: "consumer"
        name: "Consumer"
        template: "consumer.md"
        inputs:
          - key: "missing"
            file: "missing.md"
`,
			wantErr:     true,
			errContains: "dangling",
		},
		{
			name: "unknown after reference",
			yamlContent: `
registry:
  - namespace: "workflow"
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
  - namespace: "workflow"
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
  - namespace: "workflow"
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
  - namespace: "workflow"
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
  - namespace: "workflow"
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
			// Create mock filesystem with workflows/test/template.yaml
			// For most tests, use createWorkflowFSWithTemplates to pass template existence validation.
			// Some error tests (empty array, invalid YAML) need special handling.
			var fs fstest.MapFS
			switch tt.name {
			case "empty workflows array", "invalid YAML syntax":
				// These tests don't need templates
				fs = createWorkflowFS(tt.yamlContent)
			default:
				// All other tests get templates so they can reach their intended validation points
				fs = createWorkflowFSWithTemplates(tt.yamlContent)
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
  - namespace: "workflow"
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
          - key: "artifact"
            file: "artifact.md"
      - key: "consumer"
        name: "Consumer"
        template: "consumer.md"
        inputs:
          - key: "artifact"
            file: "artifact.md"
        after:
          - "producer"
`
	fs := createWorkflowFSWithTemplates(yamlContent)

	registrations, err := LoadRegistryFromYAML(fs)
	require.NoError(t, err, "LoadRegistryFromYAML() unexpected error")
	require.Len(t, registrations, 1, "expected 1 registration")

	reg := registrations[0]

	// Verify registration metadata
	require.Equal(t, "workflow", reg.Namespace(), "Type() mismatch")
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
		wantErr  bool
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
				Inputs:   []ArtifactDef{{Key: "input", File: "input.md"}},
			},
			wantOpts: 1,
		},
		{
			name: "outputs only",
			node: NodeDef{
				Key:      "test",
				Name:     "Test",
				Template: "test.md",
				Outputs:  []ArtifactDef{{Key: "output", File: "output.md"}},
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
				Inputs:   []ArtifactDef{{Key: "input", File: "input.md"}},
				Outputs:  []ArtifactDef{{Key: "output", File: "output.md"}},
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
		{
			name: "missing input key",
			node: NodeDef{
				Key:      "test",
				Name:     "Test",
				Template: "test.md",
				Inputs:   []ArtifactDef{{File: "input.md"}},
			},
			wantErr: true,
		},
		{
			name: "missing output file",
			node: NodeDef{
				Key:      "test",
				Name:     "Test",
				Template: "test.md",
				Outputs:  []ArtifactDef{{Key: "output"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := buildNodeOptions(tt.node)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
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

func TestYAMLLoader_ParsesSystemPromptField(t *testing.T) {
	yamlContent := `
registry:
  - namespace: "workflow"
    key: "with-system-prompt"
    version: "v1"
    name: "Workflow With System Prompt"
    description: "A workflow with system prompt"
    system_prompt: "coordinator_template.md"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
        assignee: "worker-1"
`
	fs := createWorkflowFS(yamlContent, "step1.md")
	// Add the system prompt template file
	fs["workflows/test/coordinator_template.md"] = &fstest.MapFile{Data: []byte("# System Prompt")}

	registrations, err := LoadRegistryFromYAML(fs)
	require.NoError(t, err, "LoadRegistryFromYAML() unexpected error")
	require.Len(t, registrations, 1, "expected 1 registration")

	reg := registrations[0]
	require.Equal(t, "workflows/test/coordinator_template.md", reg.SystemPrompt(), "SystemPrompt() should return resolved path")
}

func TestYAMLLoader_EmptySystemPrompt_AllowedForNonWorkflow(t *testing.T) {
	// Non-orchestration workflow (no assignee fields) should not require system_prompt
	yamlContent := `
registry:
  - namespace: "workflow"
    key: "no-assignee"
    version: "v1"
    name: "Non-Orchestration Workflow"
    description: "A workflow without assignee"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`
	fs := createWorkflowFS(yamlContent, "step1.md")

	registrations, err := LoadRegistryFromYAML(fs)
	require.NoError(t, err, "LoadRegistryFromYAML() should not error for non-orchestration workflows without system_prompt")
	require.Len(t, registrations, 1, "expected 1 registration")
	require.Empty(t, registrations[0].SystemPrompt(), "SystemPrompt() should be empty")
}

func TestYAMLLoader_EmptySystemPrompt_DefaultsForOrchestrationWorkflow(t *testing.T) {
	// Orchestration workflow (has assignee) without system_prompt should use default
	yamlContent := `
registry:
  - namespace: "workflow"
    key: "uses-default-system-prompt"
    version: "v1"
    name: "Orchestration With Default System Prompt"
    description: "An orchestration workflow using default system_prompt"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
        assignee: "worker-1"
`
	// Create FS with the step1.md template AND the default system prompt at workflows/
	fsys := fstest.MapFS{
		"workflows/test/template.yaml":      {Data: []byte(yamlContent)},
		"workflows/test/step1.md":           {Data: []byte("# Step 1 template")},
		"workflows/v1-epic-instructions.md": {Data: []byte("# Default system prompt")},
	}

	registrations, err := LoadRegistryFromYAML(fsys)
	require.NoError(t, err, "LoadRegistryFromYAML() should not error when default system_prompt exists")
	require.Len(t, registrations, 1, "expected 1 registration")
	require.Equal(t, "workflows/v1-epic-instructions.md", registrations[0].SystemPrompt(),
		"SystemPrompt() should be set to the default")
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
					{Key: "step1", Name: "Step 1", Template: "step1.md", Inputs: []ArtifactDef{{Key: "input", File: "input.md"}}},
					{Key: "step2", Name: "Step 2", Template: "step2.md", Outputs: []ArtifactDef{{Key: "output", File: "output.md"}}},
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

func TestValidateTemplatePath_PathTraversal(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:        "simple parent traversal",
			path:        "../etc/passwd",
			wantErr:     true,
			errContains: "path traversal",
		},
		{
			name:        "multiple parent traversal",
			path:        "../../../etc/passwd",
			wantErr:     true,
			errContains: "path traversal",
		},
		{
			name:        "embedded parent traversal",
			path:        "foo/../bar",
			wantErr:     true,
			errContains: "path traversal",
		},
		{
			name:        "traversal with dots only",
			path:        "..",
			wantErr:     true,
			errContains: "path traversal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTemplatePath(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.errContains))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateTemplatePath_AbsolutePath(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:        "unix absolute path",
			path:        "/etc/passwd",
			wantErr:     true,
			errContains: "absolute",
		},
		{
			name:        "unix root path",
			path:        "/",
			wantErr:     true,
			errContains: "absolute",
		},
		{
			name:        "windows drive letter",
			path:        "C:\\Windows\\System32",
			wantErr:     true,
			errContains: "absolute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTemplatePath(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.errContains))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateTemplatePath_ValidPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{
			name: "empty path",
			path: "",
		},
		{
			name: "simple filename",
			path: "template.md",
		},
		{
			name: "relative path",
			path: "templates/template.md",
		},
		{
			name: "deeply nested relative path",
			path: "a/b/c/d/template.md",
		},
		{
			name: "dot in filename",
			path: "v1.0.template.md",
		},
		{
			name: "current dir prefix",
			path: "./template.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTemplatePath(tt.path)
			require.NoError(t, err)
		})
	}
}

func TestValidateAssignee(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "empty is valid",
			input:   "",
			wantErr: false,
		},
		{
			name:    "worker-1 is valid",
			input:   "worker-1",
			wantErr: false,
		},
		{
			name:    "worker-99 is valid",
			input:   "worker-99",
			wantErr: false,
		},
		{
			name:    "worker-10 is valid",
			input:   "worker-10",
			wantErr: false,
		},
		{
			name:    "human is valid (for checkpoints)",
			input:   "human",
			wantErr: false,
		},
		{
			name:    "worker-0 is invalid",
			input:   "worker-0",
			wantErr: true,
		},
		{
			name:    "worker-100 is invalid (too high)",
			input:   "worker-100",
			wantErr: true,
		},
		{
			name:    "invalid-role is invalid",
			input:   "invalid-role",
			wantErr: true,
		},
		{
			name:    "coordinator is invalid",
			input:   "coordinator",
			wantErr: true,
		},
		{
			name:    "Worker-1 (uppercase) is invalid",
			input:   "Worker-1",
			wantErr: true,
		},
		{
			name:    "Human (uppercase) is invalid",
			input:   "Human",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAssignee(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestLoadRegistryFromYAML_OversizedYAML(t *testing.T) {
	// Create a filesystem with an oversized template.yaml
	// Generate content > 1MB
	var content strings.Builder
	content.WriteString(`registry:
  - namespace: "test"
    key: "oversized"
    version: "v1"
    name: "Oversized"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`)
	// Add padding comments to exceed 1MB
	for i := 0; i < 15000; i++ {
		content.WriteString("# " + strings.Repeat("x", 100) + "\n")
	}

	fs := fstest.MapFS{
		"workflows/oversized/template.yaml": &fstest.MapFile{
			Data: []byte(content.String()),
		},
		"workflows/oversized/step1.md": &fstest.MapFile{
			Data: []byte("# Step 1"),
		},
	}

	// The oversized file should be skipped, resulting in "no workflow registrations found"
	_, err := LoadRegistryFromYAML(fs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no workflow registrations found")
}

func TestLoadRegistryFromYAML_DuplicateIDs(t *testing.T) {
	// Create a filesystem with duplicate namespace/key in different directories
	fs := fstest.MapFS{
		"workflows/workflow-a/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "test"
    key: "dup"
    version: "v1"
    name: "Duplicate A"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`),
		},
		"workflows/workflow-a/step1.md": &fstest.MapFile{
			Data: []byte("# Step 1"),
		},
		"workflows/workflow-b/template.yaml": &fstest.MapFile{
			Data: []byte(`registry:
  - namespace: "test"
    key: "dup"
    version: "v1"
    name: "Duplicate B"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`),
		},
		"workflows/workflow-b/step1.md": &fstest.MapFile{
			Data: []byte("# Step 1"),
		},
	}

	// Duplicates log a warning but don't fail - both registrations are loaded
	// (Later AddOrReplace logic handles shadowing)
	regs, err := LoadRegistryFromYAML(fs)
	require.NoError(t, err)
	require.Len(t, regs, 2, "both duplicate registrations should be loaded")
}

func TestLoadRegistryFromYAMLWithSource(t *testing.T) {
	yamlContent := `
registry:
  - namespace: "test"
    key: "source-test"
    version: "v1"
    name: "Source Test"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`
	fs := fstest.MapFS{
		"workflows/test/template.yaml": &fstest.MapFile{
			Data: []byte(yamlContent),
		},
		"workflows/test/step1.md": &fstest.MapFile{
			Data: []byte("# Step 1"),
		},
	}

	// Test with SourceBuiltIn
	regsBuiltIn, err := LoadRegistryFromYAMLWithSource(fs, registry.SourceBuiltIn)
	require.NoError(t, err)
	require.Len(t, regsBuiltIn, 1)
	require.Equal(t, registry.SourceBuiltIn, regsBuiltIn[0].Source())

	// Test with SourceUser
	regsUser, err := LoadRegistryFromYAMLWithSource(fs, registry.SourceUser)
	require.NoError(t, err)
	require.Len(t, regsUser, 1)
	require.Equal(t, registry.SourceUser, regsUser[0].Source())
}

func TestLoadRegistryFromYAML_InvalidAssignee(t *testing.T) {
	yamlContent := `
registry:
  - namespace: "test"
    key: "invalid-assignee"
    version: "v1"
    name: "Invalid Assignee"
    description: ""
    instructions: "instructions.md"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
        assignee: "invalid-role"
`
	fs := fstest.MapFS{
		"workflows/test/template.yaml": &fstest.MapFile{
			Data: []byte(yamlContent),
		},
		"workflows/test/step1.md": &fstest.MapFile{
			Data: []byte("# Step 1"),
		},
		"workflows/test/instructions.md": &fstest.MapFile{
			Data: []byte("# Instructions"),
		},
	}

	_, err := LoadRegistryFromYAML(fs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid assignee")
}

func TestLoadRegistryFromYAML_MissingTemplate(t *testing.T) {
	yamlContent := `
registry:
  - namespace: "test"
    key: "missing-template"
    version: "v1"
    name: "Missing Template"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "nonexistent.md"
`
	fs := fstest.MapFS{
		"workflows/test/template.yaml": &fstest.MapFile{
			Data: []byte(yamlContent),
		},
	}

	_, err := LoadRegistryFromYAML(fs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestLoadRegistryFromYAML_PathTraversal(t *testing.T) {
	yamlContent := `
registry:
  - namespace: "test"
    key: "path-traversal"
    version: "v1"
    name: "Path Traversal"
    description: ""
    epic_template: "../../../etc/passwd"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`
	fs := createWorkflowFS(yamlContent)

	_, err := LoadRegistryFromYAML(fs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "path traversal")
}

func TestLoadRegistryFromYAML_AbsolutePath(t *testing.T) {
	yamlContent := `
registry:
  - namespace: "test"
    key: "absolute-path"
    version: "v1"
    name: "Absolute Path"
    description: ""
    epic_template: "/etc/passwd"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`
	fs := createWorkflowFS(yamlContent)

	_, err := LoadRegistryFromYAML(fs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "absolute")
}

func TestValidateTemplateExists(t *testing.T) {
	fs := fstest.MapFS{
		"workflows/test/template.md":     &fstest.MapFile{Data: []byte("# Template")},
		"workflows/test/instructions.md": &fstest.MapFile{Data: []byte("# Instructions")},
		"workflows/test/node1.md":        &fstest.MapFile{Data: []byte("# Node 1")},
	}

	tests := []struct {
		name    string
		def     WorkflowDef
		wantErr bool
	}{
		{
			name: "all templates exist",
			def: WorkflowDef{
				EpicTemplate: "workflows/test/template.md",
				SystemPrompt: "workflows/test/instructions.md",
				Nodes: []NodeDef{
					{Key: "node1", Template: "workflows/test/node1.md"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing initial prompt",
			def: WorkflowDef{
				EpicTemplate: "workflows/test/missing.md",
				Nodes: []NodeDef{
					{Key: "node1", Template: "workflows/test/node1.md"},
				},
			},
			wantErr: true,
		},
		{
			name: "missing system prompt",
			def: WorkflowDef{
				SystemPrompt: "workflows/test/missing-system-prompt.md",
				Nodes: []NodeDef{
					{Key: "node1", Template: "workflows/test/node1.md"},
				},
			},
			wantErr: true,
		},
		{
			name: "missing node template",
			def: WorkflowDef{
				EpicTemplate: "workflows/test/template.md",
				Nodes: []NodeDef{
					{Key: "node1", Template: "workflows/test/missing-node.md"},
				},
			},
			wantErr: true,
		},
		{
			name: "empty templates are allowed",
			def: WorkflowDef{
				EpicTemplate: "",
				SystemPrompt: "",
				Nodes: []NodeDef{
					{Key: "node1", Template: "workflows/test/node1.md"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTemplateExists(fs, tt.def)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestLoadRegistryFromYAML_Arguments(t *testing.T) {
	yamlContent := `
registry:
  - namespace: "workflow"
    key: "with-arguments"
    version: "v1"
    name: "Workflow With Arguments"
    description: "A workflow with user-configurable parameters"
    arguments:
      - key: "feature_name"
        label: "Feature Name"
        description: "Name of the feature to implement"
        type: "text"
        required: true
      - key: "worker_count"
        label: "Worker Count"
        description: "Number of workers to spawn"
        type: "number"
        required: false
        default: "4"
      - key: "context"
        label: "Additional Context"
        description: "Provide any background information"
        type: "textarea"
        required: false
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`
	fs := createWorkflowFSWithTemplates(yamlContent)

	regs, err := LoadRegistryFromYAML(fs)
	require.NoError(t, err)
	require.Len(t, regs, 1)

	args := regs[0].Arguments()
	require.Len(t, args, 3)

	// Check first argument
	require.Equal(t, "feature_name", args[0].Key())
	require.Equal(t, "Feature Name", args[0].Label())
	require.Equal(t, "Name of the feature to implement", args[0].Description())
	require.Equal(t, registry.ArgumentTypeText, args[0].Type())
	require.True(t, args[0].Required())
	require.Equal(t, "", args[0].DefaultValue())

	// Check second argument
	require.Equal(t, "worker_count", args[1].Key())
	require.Equal(t, registry.ArgumentTypeNumber, args[1].Type())
	require.False(t, args[1].Required())
	require.Equal(t, "4", args[1].DefaultValue())

	// Check third argument
	require.Equal(t, "context", args[2].Key())
	require.Equal(t, registry.ArgumentTypeTextarea, args[2].Type())
}

func TestLoadRegistryFromYAML_Arguments_InvalidType(t *testing.T) {
	yamlContent := `
registry:
  - namespace: "workflow"
    key: "invalid-arg-type"
    version: "v1"
    name: "Invalid Argument Type"
    description: ""
    arguments:
      - key: "bad"
        label: "Bad Arg"
        description: "Has invalid type"
        type: "checkbox"
        required: false
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`
	fs := createWorkflowFSWithTemplates(yamlContent)

	_, err := LoadRegistryFromYAML(fs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be text, number, textarea, select, multi-select, or epic-search")
}

func TestLoadRegistryFromYAML_Arguments_DuplicateKey(t *testing.T) {
	yamlContent := `
registry:
  - namespace: "workflow"
    key: "duplicate-arg-key"
    version: "v1"
    name: "Duplicate Argument Key"
    description: ""
    arguments:
      - key: "name"
        label: "Name"
        description: ""
        type: "text"
        required: true
      - key: "name"
        label: "Another Name"
        description: ""
        type: "text"
        required: false
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`
	fs := createWorkflowFSWithTemplates(yamlContent)

	_, err := LoadRegistryFromYAML(fs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate key")
}

func TestLoadRegistryFromYAML_Arguments_Empty(t *testing.T) {
	yamlContent := `
registry:
  - namespace: "workflow"
    key: "no-arguments"
    version: "v1"
    name: "No Arguments"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`
	fs := createWorkflowFSWithTemplates(yamlContent)

	regs, err := LoadRegistryFromYAML(fs)
	require.NoError(t, err)
	require.Len(t, regs, 1)

	// No arguments is valid
	require.Nil(t, regs[0].Arguments())
}

// --- Tests for collaborative workflow pattern (zero nodes + system_prompt) ---

func TestIsEpicDrivenWorkflow(t *testing.T) {
	tests := []struct {
		name string
		def  WorkflowDef
		want bool
	}{
		{
			name: "classic epic-driven: single epic_id arg, no nodes",
			def: WorkflowDef{
				Arguments: []ArgumentDef{
					{Key: "epic_id", Type: "text"},
				},
				Nodes: nil,
			},
			want: true,
		},
		{
			name: "epic_id arg with empty nodes slice",
			def: WorkflowDef{
				Arguments: []ArgumentDef{
					{Key: "epic_id", Type: "text"},
				},
				Nodes: []NodeDef{},
			},
			want: true,
		},
		{
			name: "not epic-driven: epic_id but has nodes",
			def: WorkflowDef{
				Arguments: []ArgumentDef{
					{Key: "epic_id", Type: "text"},
				},
				Nodes: []NodeDef{
					{Key: "step1", Name: "Step 1", Template: "step1.md"},
				},
			},
			want: false,
		},
		{
			name: "not epic-driven: no arguments",
			def: WorkflowDef{
				Arguments: nil,
				Nodes:     nil,
			},
			want: false,
		},
		{
			name: "not epic-driven: different argument key",
			def: WorkflowDef{
				Arguments: []ArgumentDef{
					{Key: "goal", Type: "textarea"},
				},
				Nodes: nil,
			},
			want: false,
		},
		{
			name: "not epic-driven: multiple arguments including epic_id",
			def: WorkflowDef{
				Arguments: []ArgumentDef{
					{Key: "epic_id", Type: "text"},
					{Key: "extra", Type: "text"},
				},
				Nodes: nil,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEpicDrivenWorkflow(&tt.def)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestIsOrchestrationWorkflow_ZeroNodesWithSystemPrompt(t *testing.T) {
	tests := []struct {
		name string
		def  WorkflowDef
		want bool
	}{
		{
			name: "zero nodes with system_prompt is orchestration",
			def: WorkflowDef{
				SystemPrompt: "instructions.md",
				Nodes:        nil,
				Arguments: []ArgumentDef{
					{Key: "goal", Type: "textarea"},
				},
			},
			want: true,
		},
		{
			name: "zero nodes with system_prompt and empty nodes slice",
			def: WorkflowDef{
				SystemPrompt: "instructions.md",
				Nodes:        []NodeDef{},
				Arguments: []ArgumentDef{
					{Key: "goal", Type: "textarea"},
				},
			},
			want: true,
		},
		{
			name: "zero nodes without system_prompt and not epic-driven is NOT orchestration",
			def: WorkflowDef{
				SystemPrompt: "",
				Nodes:        nil,
				Arguments: []ArgumentDef{
					{Key: "goal", Type: "textarea"},
				},
			},
			want: false,
		},
		{
			name: "zero nodes with system_prompt and no arguments",
			def: WorkflowDef{
				SystemPrompt: "instructions.md",
				Nodes:        nil,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isOrchestrationWorkflow(&tt.def)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBuildRegistrationFromDefWithSource_CollaborativeWorkflow(t *testing.T) {
	// Collaborative pattern: zero nodes + system_prompt + epic_template
	def := WorkflowDef{
		Namespace:    "workflow",
		Key:          "collaborative-planning",
		Version:      "v1",
		Name:         "Collaborative Planning",
		Description:  "Multi-agent planning session",
		EpicTemplate: "workflows/test/epic.md",
		SystemPrompt: "workflows/test/instructions.md",
		Labels:       []string{"category:planning"},
		Arguments: []ArgumentDef{
			{Key: "goal", Label: "Goal", Description: "Planning goal", Type: "textarea", Required: true},
		},
	}

	reg, err := buildRegistrationFromDefWithSource(def, registry.SourceBuiltIn)
	require.NoError(t, err, "collaborative workflow should build successfully")

	// Verify metadata
	require.Equal(t, "workflow", reg.Namespace())
	require.Equal(t, "collaborative-planning", reg.Key())
	require.Equal(t, "v1", reg.Version())
	require.Equal(t, "Collaborative Planning", reg.Name())
	require.Equal(t, "Multi-agent planning session", reg.Description())

	// Verify templates
	require.Equal(t, "workflows/test/epic.md", reg.EpicTemplate())
	require.Equal(t, "workflows/test/instructions.md", reg.SystemPrompt())

	// Verify labels
	require.Equal(t, []string{"category:planning"}, reg.Labels())

	// Verify arguments
	args := reg.Arguments()
	require.Len(t, args, 1)
	require.Equal(t, "goal", args[0].Key())
	require.Equal(t, registry.ArgumentTypeTextarea, args[0].Type())
	require.True(t, args[0].Required())

	// Critical: zero-node workflow must have nil DAG
	require.Nil(t, reg.DAG(), "collaborative workflow should have nil DAG (no pre-created tasks)")
}

func TestBuildRegistrationFromDefWithSource_ZeroNodesNoSystemPromptNotEpicDriven(t *testing.T) {
	// Zero nodes, no system_prompt, not epic-driven = error
	def := WorkflowDef{
		Namespace:   "workflow",
		Key:         "invalid-empty",
		Version:     "v1",
		Name:        "Invalid Empty",
		Description: "",
		Arguments: []ArgumentDef{
			{Key: "goal", Type: "textarea"},
		},
	}

	_, err := buildRegistrationFromDefWithSource(def, registry.SourceBuiltIn)
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one node")
	require.Contains(t, err.Error(), "system_prompt")
}

func TestBuildRegistrationFromDefWithSource_EpicDrivenNoSystemPrompt(t *testing.T) {
	// Epic-driven workflows are allowed without system_prompt (they get default upstream)
	def := WorkflowDef{
		Namespace:   "workflow",
		Key:         "epic-driven",
		Version:     "v1",
		Name:        "Epic Driven",
		Description: "",
		Arguments: []ArgumentDef{
			{Key: "epic_id", Label: "Epic ID", Description: "Existing epic", Type: "text"},
		},
	}

	reg, err := buildRegistrationFromDefWithSource(def, registry.SourceBuiltIn)
	require.NoError(t, err, "epic-driven workflow should build without system_prompt")
	require.Nil(t, reg.DAG(), "epic-driven workflow should have nil DAG")
}

func TestLoadRegistryFromYAML_CollaborativeWorkflow(t *testing.T) {
	yamlContent := `
registry:
  - namespace: "workflow"
    key: "collab-planning"
    version: "v1"
    name: "Collaborative Planning"
    description: "Multi-agent planning via Fabric channels"
    epic_template: "epic.md"
    system_prompt: "instructions.md"
    labels:
      - "category:planning"
    arguments:
      - key: "goal"
        label: "Planning Goal"
        description: "What to plan"
        type: "textarea"
        required: true
`
	fs := fstest.MapFS{
		"workflows/test/template.yaml":   {Data: []byte(yamlContent)},
		"workflows/test/epic.md":         {Data: []byte("# Epic: {{.Args.goal}}")},
		"workflows/test/instructions.md": {Data: []byte("# Coordinator instructions")},
	}

	regs, err := LoadRegistryFromYAML(fs)
	require.NoError(t, err, "collaborative workflow should load successfully")
	require.Len(t, regs, 1)

	reg := regs[0]
	require.Equal(t, "collab-planning", reg.Key())
	require.Equal(t, "Collaborative Planning", reg.Name())
	require.Equal(t, "workflows/test/epic.md", reg.EpicTemplate())
	require.Equal(t, "workflows/test/instructions.md", reg.SystemPrompt())
	require.Nil(t, reg.DAG(), "collaborative workflow should have nil DAG")
	require.Equal(t, []string{"category:planning"}, reg.Labels())

	// Verify arguments loaded
	args := reg.Arguments()
	require.Len(t, args, 1)
	require.Equal(t, "goal", args[0].Key())
	require.Equal(t, registry.ArgumentTypeTextarea, args[0].Type())
	require.True(t, args[0].Required())

	// Verify source tagging
	require.Equal(t, registry.SourceBuiltIn, reg.Source())
}

func TestLoadRegistryFromYAML_CollaborativeWorkflow_DefaultSystemPrompt(t *testing.T) {
	// A zero-node workflow with system_prompt omitted but is recognized as orchestration
	// because it has system_prompt content set upstream. However, since it's not epic-driven
	// and has no system_prompt, it must error per the validation in buildRegistrationFromDefWithSource.
	// This test verifies that zero-node workflows without system_prompt and not epic-driven DO fail.
	yamlContent := `
registry:
  - namespace: "workflow"
    key: "zero-node-no-prompt"
    version: "v1"
    name: "Zero Node No Prompt"
    description: ""
    epic_template: "epic.md"
    arguments:
      - key: "goal"
        label: "Goal"
        description: "The goal"
        type: "textarea"
        required: true
`
	fs := fstest.MapFS{
		"workflows/test/template.yaml": {Data: []byte(yamlContent)},
		"workflows/test/epic.md":       {Data: []byte("# Epic")},
	}

	_, err := LoadRegistryFromYAML(fs)
	require.Error(t, err, "zero-node workflow without system_prompt should fail")
	require.Contains(t, err.Error(), "at least one node")
}

func TestLoadRegistryFromYAML_CollaborativeWorkflow_MissingSystemPromptFile(t *testing.T) {
	// system_prompt is specified but the file doesn't exist
	yamlContent := `
registry:
  - namespace: "workflow"
    key: "missing-prompt-file"
    version: "v1"
    name: "Missing Prompt File"
    description: ""
    epic_template: "epic.md"
    system_prompt: "nonexistent.md"
    arguments:
      - key: "goal"
        label: "Goal"
        description: ""
        type: "textarea"
        required: true
`
	fs := fstest.MapFS{
		"workflows/test/template.yaml": {Data: []byte(yamlContent)},
		"workflows/test/epic.md":       {Data: []byte("# Epic")},
	}

	_, err := LoadRegistryFromYAML(fs)
	require.Error(t, err, "missing system_prompt file should fail")
	require.Contains(t, err.Error(), "not found")
}

func TestLoadRegistryFromYAML_CollaborativeWorkflow_MissingEpicTemplateFile(t *testing.T) {
	// epic_template is specified but the file doesn't exist
	yamlContent := `
registry:
  - namespace: "workflow"
    key: "missing-epic-file"
    version: "v1"
    name: "Missing Epic File"
    description: ""
    epic_template: "nonexistent-epic.md"
    system_prompt: "instructions.md"
    arguments:
      - key: "goal"
        label: "Goal"
        description: ""
        type: "textarea"
        required: true
`
	fs := fstest.MapFS{
		"workflows/test/template.yaml":   {Data: []byte(yamlContent)},
		"workflows/test/instructions.md": {Data: []byte("# Instructions")},
	}

	_, err := LoadRegistryFromYAML(fs)
	require.Error(t, err, "missing epic_template file should fail")
	require.Contains(t, err.Error(), "not found")
}

func TestLoadRegistryFromYAML_InvalidWorkflowFailsWholeLoad(t *testing.T) {
	// Built-in (strict) loading still fails on any invalid workflow, even alongside valid ones.
	fsys := fstest.MapFS{
		"workflows/good/template.yaml": {Data: []byte(`registry:
  - namespace: "workflow"
    key: "good"
    version: "v1"
    name: "Good"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`)},
		"workflows/good/step1.md": {Data: []byte("# Step 1")},
		"workflows/bad/template.yaml": {Data: []byte(`registry:
  - namespace: "workflow"
    key: "bad"
    version: "v1"
    name: "Bad"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "nonexistent.md"
`)},
	}

	_, err := LoadRegistryFromYAML(fsys)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestLoadRegistryFromYAML_SkipInvalid(t *testing.T) {
	validDef := func(key string) string {
		return `
  - namespace: "workflow"
    key: "` + key + `"
    version: "v1"
    name: "` + key + `"
    description: ""
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`
	}
	fsys := fstest.MapFS{
		"workflows/good/template.yaml":  {Data: []byte("registry:" + validDef("good"))},
		"workflows/good/step1.md":       {Data: []byte("# Step 1")},
		"workflows/parse/template.yaml": {Data: []byte("registry:\n  - key: [broken\n")},
		"workflows/traversal/template.yaml": {Data: []byte(`registry:
  - namespace: "workflow"
    key: "traversal"
    version: "v1"
    name: "Traversal"
    description: ""
    epic_template: "../../etc/passwd"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`)},
		"workflows/traversal/step1.md": {Data: []byte("# Step 1")},
		"workflows/no-nodes/template.yaml": {Data: []byte(`registry:
  - namespace: "workflow"
    key: "no-nodes"
    version: "v1"
    name: "No Nodes"
    description: ""
`)},
	}

	regs, err := loadRegistryFromYAML(fsys, registry.SourceUser, loadOptions{skipInvalid: true})
	require.NoError(t, err)
	require.Len(t, regs, 1)
	require.Equal(t, "good", regs[0].Key())
	require.Equal(t, registry.SourceUser, regs[0].Source())

	// All workflows invalid: nothing to load is still reported as an error
	delete(fsys, "workflows/good/template.yaml")
	_, err = loadRegistryFromYAML(fsys, registry.SourceUser, loadOptions{skipInvalid: true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no workflow registrations found")
}

func TestLoadRegistryFromYAML_TemplateFSFallback(t *testing.T) {
	primary := fstest.MapFS{
		"workflows/wf/template.yaml": {Data: []byte(`registry:
  - namespace: "workflow"
    key: "wf"
    version: "v1"
    name: "WF"
    description: ""
    nodes:
      - key: "local"
        name: "Local"
        template: "local.md"
        assignee: "worker-1"
      - key: "shared"
        name: "Shared"
        template: "shared.md"
        assignee: "human"
      - key: "explicit"
        name: "Explicit"
        template: "workflows/explicit.md"
        assignee: "worker-2"
`)},
		"workflows/wf/local.md": {Data: []byte("# primary local")},
	}
	fallback := fstest.MapFS{
		"workflows/v1-epic-instructions.md": {Data: []byte("# fallback instructions")},
		"workflows/shared.md":               {Data: []byte("# fallback shared")},
		"workflows/explicit.md":             {Data: []byte("# fallback explicit")},
		"workflows/wf/local.md":             {Data: []byte("# fallback local (should not be used)")},
	}

	regs, err := loadRegistryFromYAML(primary, registry.SourceUser, loadOptions{templateFS: newLayeredFS(primary, fallback)})
	require.NoError(t, err)
	require.Len(t, regs, 1)
	reg := regs[0]

	require.Equal(t, "workflows/v1-epic-instructions.md", reg.SystemPrompt())
	templatesByKey := make(map[string]string)
	for _, node := range reg.DAG().Nodes() {
		templatesByKey[node.Key()] = node.Template()
	}
	require.Equal(t, "workflows/wf/local.md", templatesByKey["local"])
	require.Equal(t, "workflows/shared.md", templatesByKey["shared"])
	require.Equal(t, "workflows/explicit.md", templatesByKey["explicit"])

	// Without a fallback, the same workflow fails to load
	_, err = loadRegistryFromYAML(primary, registry.SourceUser, loadOptions{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}
