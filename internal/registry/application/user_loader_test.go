package registry

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/registry/domain"
)

func TestUserRegistryDir(t *testing.T) {
	dir := UserRegistryDir()

	// Should return a non-empty path
	require.NotEmpty(t, dir, "UserRegistryDir() should return a path")

	// Should end with .perles/workflows
	require.True(t, filepath.Base(dir) == "workflows", "UserRegistryDir() should end with 'workflows'")
	require.True(t, filepath.Base(filepath.Dir(dir)) == ".perles", "UserRegistryDir() parent should be '.perles'")
}

func TestUserRegistryBaseDir(t *testing.T) {
	dir := UserRegistryBaseDir()

	// Should return a non-empty path
	require.NotEmpty(t, dir, "UserRegistryBaseDir() should return a path")

	// Should end with .perles
	require.True(t, filepath.Base(dir) == ".perles", "UserRegistryBaseDir() should end with '.perles'")
}

func TestUserRegistryBaseDir_IsParentOfUserRegistryDir(t *testing.T) {
	baseDir := UserRegistryBaseDir()
	workflowsDir := UserRegistryDir()

	// UserRegistryDir should be a subdirectory of UserRegistryBaseDir
	expected := filepath.Join(baseDir, "workflows")
	require.Equal(t, expected, workflowsDir, "UserRegistryDir() should be UserRegistryBaseDir()/workflows")
}

func TestLoadUserRegistryFromDir_NotExist(t *testing.T) {
	// Test with a non-existent directory
	regs, fsys, err := LoadUserRegistryFromDir("/nonexistent/path/that/does/not/exist", nil)

	// Should return nil, nil, nil - not an error
	require.NoError(t, err, "LoadUserRegistryFromDir() should not error for non-existent directory")
	require.Nil(t, regs, "LoadUserRegistryFromDir() should return nil registrations")
	require.Nil(t, fsys, "LoadUserRegistryFromDir() should return nil fs")
}

func TestLoadUserRegistryFromDir_EmptyBaseDir(t *testing.T) {
	// Test with empty base directory
	regs, fsys, err := LoadUserRegistryFromDir("", nil)

	require.NoError(t, err, "LoadUserRegistryFromDir() should not error for empty path")
	require.Nil(t, regs, "LoadUserRegistryFromDir() should return nil registrations")
	require.Nil(t, fsys, "LoadUserRegistryFromDir() should return nil fs")
}

func TestLoadUserRegistryFromDir_Empty(t *testing.T) {
	// Create a temporary directory with empty workflows subdirectory
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0755))

	regs, fsys, err := LoadUserRegistryFromDir(tmpDir, nil)

	// Empty directory should return nil registrations but with a valid FS
	// (error from "no workflow registrations found" is logged and handled gracefully)
	require.NoError(t, err, "LoadUserRegistryFromDir() should not error for empty directory")
	require.Nil(t, regs, "LoadUserRegistryFromDir() should return nil registrations for empty dir")
	require.NotNil(t, fsys, "LoadUserRegistryFromDir() should return non-nil fs for valid directory")
}

func TestLoadUserRegistryFromDir_NoWorkflowsSubdir(t *testing.T) {
	// Create a temporary directory WITHOUT workflows subdirectory
	tmpDir := t.TempDir()

	regs, fsys, err := LoadUserRegistryFromDir(tmpDir, nil)

	// No workflows subdir should return nil, nil, nil (graceful)
	require.NoError(t, err, "LoadUserRegistryFromDir() should not error when workflows subdir missing")
	require.Nil(t, regs, "LoadUserRegistryFromDir() should return nil registrations")
	require.Nil(t, fsys, "LoadUserRegistryFromDir() should return nil fs")
}

func TestLoadUserRegistryFromDir_ValidWorkflow(t *testing.T) {
	// Use the test fixtures
	testDataDir := "testdata/user_workflows"

	// Create a temporary directory structure that mirrors what we expect
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows", "valid-workflow")
	require.NoError(t, os.MkdirAll(workflowsDir, 0755))

	// Copy the test fixture files
	registryYAML := `registry:
  - namespace: "user-workflow"
    key: "my-custom-workflow"
    version: "v1"
    name: "My Custom Workflow"
    description: "A user-defined workflow for testing"
    labels:
      - "user"
      - "test"
    nodes:
      - key: "research"
        name: "Research Phase"
        template: "my-template.md"
        outputs:
          - key: "research"
            file: "research.md"
`
	require.NoError(t, os.WriteFile(filepath.Join(workflowsDir, "template.yaml"), []byte(registryYAML), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(workflowsDir, "my-template.md"), []byte("# Template"), 0644))

	regs, fsys, err := LoadUserRegistryFromDir(tmpDir, nil)

	require.NoError(t, err, "LoadUserRegistryFromDir() should not error for valid workflow")
	require.NotNil(t, fsys, "LoadUserRegistryFromDir() should return non-nil fs")
	require.Len(t, regs, 1, "LoadUserRegistryFromDir() should return 1 registration")

	// Verify the registration details
	reg := regs[0]
	require.Equal(t, "user-workflow", reg.Namespace(), "Namespace should match")
	require.Equal(t, "my-custom-workflow", reg.Key(), "Key should match")
	require.Equal(t, "v1", reg.Version(), "Version should match")
	require.Equal(t, "My Custom Workflow", reg.Name(), "Name should match")
	require.Equal(t, registry.SourceUser, reg.Source(), "Source should be SourceUser")
	require.Equal(t, []string{"user", "test"}, reg.Labels(), "Labels should match")

	// Verify the DAG nodes
	dag := reg.DAG()
	require.NotNil(t, dag, "DAG should not be nil")
	nodes := dag.Nodes()
	require.Len(t, nodes, 1, "Should have 1 node")
	require.Equal(t, "research", nodes[0].Key(), "Node key should match")

	// Suppress unused variable warning
	_ = testDataDir
}

func TestLoadUserRegistryFromDir_InvalidYAML(t *testing.T) {
	// Create a temporary directory with invalid YAML
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows", "invalid-workflow")
	require.NoError(t, os.MkdirAll(workflowsDir, 0755))

	// Write invalid YAML
	invalidYAML := `registry:
  - namespace: invalid
    key: [this is broken
`
	require.NoError(t, os.WriteFile(filepath.Join(workflowsDir, "template.yaml"), []byte(invalidYAML), 0644))

	regs, fsys, err := LoadUserRegistryFromDir(tmpDir, nil)

	// Invalid YAML should be logged and return nil registrations with valid FS (graceful)
	require.NoError(t, err, "LoadUserRegistryFromDir() should not error for invalid YAML (logs warning)")
	require.Nil(t, regs, "LoadUserRegistryFromDir() should return nil registrations for invalid YAML")
	require.NotNil(t, fsys, "LoadUserRegistryFromDir() should return non-nil fs even with invalid YAML")
}

func TestLoadUserRegistryFromDir_FileInsteadOfDir(t *testing.T) {
	// Create a temporary file instead of directory
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notadir")
	require.NoError(t, os.WriteFile(filePath, []byte("content"), 0644))

	regs, fsys, err := LoadUserRegistryFromDir(filePath, nil)

	// File instead of directory should return nil gracefully
	require.NoError(t, err, "LoadUserRegistryFromDir() should not error for file path")
	require.Nil(t, regs, "LoadUserRegistryFromDir() should return nil registrations")
	require.Nil(t, fsys, "LoadUserRegistryFromDir() should return nil fs")
}

// writeUserFiles writes files (path relative to baseDir -> content) under baseDir.
func writeUserFiles(t *testing.T, baseDir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(baseDir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0644))
	}
}

// jokeContestUserYAML mirrors the custom-workflows docs example: an orchestration workflow
// with no system_prompt and a human review node that uses the shared v1-human-review.md.
const jokeContestUserYAML = `registry:
  - namespace: "workflow"
    key: "joke-contest"
    version: "v1"
    name: "Joke Contest"
    description: "Two workers write jokes in parallel, then a third judges"
    epic_template: "v1-joke-contest-epic.md"
    nodes:
      - key: "joke-1"
        name: "Joker 1 - Write Joke"
        template: "v1-joke-contest-joke-1.md"
        assignee: "worker-1"
      - key: "joke-2"
        name: "Joker 2 - Write Joke"
        template: "v1-joke-contest-joke-2.md"
        assignee: "worker-2"
      - key: "review"
        name: "Human Review"
        template: "v1-human-review.md"
        assignee: "human"
        after:
          - "joke-1"
          - "joke-2"
      - key: "judge"
        name: "Judge - Pick Winner"
        template: "v1-joke-contest-judge.md"
        assignee: "worker-3"
        after:
          - "review"
`

// jokeContestUserFiles returns the docs example layout without any shared templates.
func jokeContestUserFiles() map[string]string {
	return map[string]string{
		"workflows/joke-contest/template.yaml":             jokeContestUserYAML,
		"workflows/joke-contest/v1-joke-contest-epic.md":   "# Epic",
		"workflows/joke-contest/v1-joke-contest-joke-1.md": "# Joke 1",
		"workflows/joke-contest/v1-joke-contest-joke-2.md": "# Joke 2",
		"workflows/joke-contest/v1-joke-contest-judge.md":  "# Judge",
	}
}

// sharedBuiltinFS returns a built-in FS containing only the shared templates.
func sharedBuiltinFS() fstest.MapFS {
	return fstest.MapFS{
		"workflows/v1-epic-instructions.md": {Data: []byte("# Built-in Epic Instructions")},
		"workflows/v1-human-review.md":      {Data: []byte("# Built-in Human Review")},
	}
}

// nodeTemplate returns the template path of the node with the given key.
func nodeTemplate(t *testing.T, reg *registry.Registration, key string) string {
	t.Helper()
	for _, node := range reg.DAG().Nodes() {
		if node.Key() == key {
			return node.Template()
		}
	}
	t.Fatalf("node %q not found", key)
	return ""
}

func TestLoadUserRegistryFromDir_FallsBackToBuiltinSharedTemplates(t *testing.T) {
	// Docs example layout: no v1-epic-instructions.md or v1-human-review.md in the user dir.
	tmpDir := t.TempDir()
	writeUserFiles(t, tmpDir, jokeContestUserFiles())

	regs, fsys, err := LoadUserRegistryFromDir(tmpDir, sharedBuiltinFS())

	require.NoError(t, err)
	require.NotNil(t, fsys)
	require.Len(t, regs, 1, "workflow should load using built-in shared templates")

	reg := regs[0]
	require.Equal(t, registry.SourceUser, reg.Source())
	require.Equal(t, "workflows/v1-epic-instructions.md", reg.SystemPrompt(), "default system_prompt should resolve to shared template")
	require.Equal(t, "workflows/v1-human-review.md", nodeTemplate(t, reg, "review"))
	require.Equal(t, "workflows/joke-contest/v1-joke-contest-joke-1.md", nodeTemplate(t, reg, "joke-1"))

	// The returned FS reads fallback templates from the built-in FS
	content, err := fs.ReadFile(fsys, reg.SystemPrompt())
	require.NoError(t, err)
	require.Equal(t, "# Built-in Epic Instructions", string(content))

	content, err = fs.ReadFile(fsys, nodeTemplate(t, reg, "review"))
	require.NoError(t, err)
	require.Equal(t, "# Built-in Human Review", string(content))

	// ...and workflow-local templates from the user dir
	content, err = fs.ReadFile(fsys, nodeTemplate(t, reg, "joke-1"))
	require.NoError(t, err)
	require.Equal(t, "# Joke 1", string(content))
}

func TestLoadUserRegistryFromDir_NoBuiltinFallbackMissingSharedTemplate(t *testing.T) {
	// Without a built-in FS, the same layout can't resolve the shared templates.
	tmpDir := t.TempDir()
	writeUserFiles(t, tmpDir, jokeContestUserFiles())

	regs, fsys, err := LoadUserRegistryFromDir(tmpDir, nil)

	require.NoError(t, err)
	require.NotNil(t, fsys)
	require.Nil(t, regs)
}

func TestLoadUserRegistryFromDir_UserTemplatesTakePrecedenceOverBuiltin(t *testing.T) {
	tmpDir := t.TempDir()
	files := jokeContestUserFiles()
	// Workflow-local override of the default system prompt
	files["workflows/joke-contest/v1-epic-instructions.md"] = "# Workflow-local Epic Instructions"
	// User-level shared override of the human review template
	files["workflows/v1-human-review.md"] = "# User Shared Human Review"
	writeUserFiles(t, tmpDir, files)

	regs, fsys, err := LoadUserRegistryFromDir(tmpDir, sharedBuiltinFS())

	require.NoError(t, err)
	require.Len(t, regs, 1)
	reg := regs[0]

	require.Equal(t, "workflows/joke-contest/v1-epic-instructions.md", reg.SystemPrompt())
	content, err := fs.ReadFile(fsys, reg.SystemPrompt())
	require.NoError(t, err)
	require.Equal(t, "# Workflow-local Epic Instructions", string(content))

	require.Equal(t, "workflows/v1-human-review.md", nodeTemplate(t, reg, "review"))
	content, err = fs.ReadFile(fsys, nodeTemplate(t, reg, "review"))
	require.NoError(t, err)
	require.Equal(t, "# User Shared Human Review", string(content), "user shared template should win over built-in")
}

func TestLoadUserRegistryFromDir_InvalidWorkflowSkipped(t *testing.T) {
	tmpDir := t.TempDir()
	writeUserFiles(t, tmpDir, map[string]string{
		// Valid workflow
		"workflows/good/template.yaml": `registry:
  - namespace: "workflow"
    key: "good"
    version: "v1"
    name: "Good"
    description: "Valid workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`,
		"workflows/good/step1.md": "# Good Step 1",
		// Missing template
		"workflows/missing-template/template.yaml": `registry:
  - namespace: "workflow"
    key: "missing-template"
    version: "v1"
    name: "Missing Template"
    description: "References a template that doesn't exist"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "nonexistent.md"
`,
		// Malformed YAML
		"workflows/broken-yaml/template.yaml": "registry:\n  - namespace: broken\n    key: [this is broken\n",
		// One valid and one invalid registration in the same file
		"workflows/mixed/template.yaml": `registry:
  - namespace: "workflow"
    key: "mixed-bad"
    version: "v1"
    name: "Mixed Bad"
    description: "Invalid assignee"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
        assignee: "not-a-worker"
  - namespace: "workflow"
    key: "mixed-good"
    version: "v1"
    name: "Mixed Good"
    description: "Valid sibling registration"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`,
		"workflows/mixed/step1.md": "# Mixed Step 1",
	})

	regs, fsys, err := LoadUserRegistryFromDir(tmpDir, nil)

	require.NoError(t, err)
	require.NotNil(t, fsys)

	keys := make([]string, 0, len(regs))
	for _, r := range regs {
		keys = append(keys, r.Key())
	}
	require.ElementsMatch(t, []string{"good", "mixed-good"}, keys, "only invalid workflows should be skipped")
}

func TestLoadUserRegistryFromDir_UnreadableWorkflowDirSkipped(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced on this platform/user")
	}

	tmpDir := t.TempDir()
	writeUserFiles(t, tmpDir, map[string]string{
		"workflows/good/template.yaml": `registry:
  - namespace: "workflow"
    key: "good"
    version: "v1"
    name: "Good"
    description: "Valid workflow"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`,
		"workflows/good/step1.md":            "# Good Step 1",
		"workflows/unreadable/template.yaml": "registry: []\n",
	})
	unreadable := filepath.Join(tmpDir, "workflows", "unreadable")
	require.NoError(t, os.Chmod(unreadable, 0))
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0755) })

	regs, _, err := LoadUserRegistryFromDir(tmpDir, nil)

	require.NoError(t, err)
	require.Len(t, regs, 1)
	require.Equal(t, "good", regs[0].Key())
}
