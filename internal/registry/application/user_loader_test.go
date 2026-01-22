package registry

import (
	"os"
	"path/filepath"
	"testing"

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
	regs, fsys, err := LoadUserRegistryFromDir("/nonexistent/path/that/does/not/exist")

	// Should return nil, nil, nil - not an error
	require.NoError(t, err, "LoadUserRegistryFromDir() should not error for non-existent directory")
	require.Nil(t, regs, "LoadUserRegistryFromDir() should return nil registrations")
	require.Nil(t, fsys, "LoadUserRegistryFromDir() should return nil fs")
}

func TestLoadUserRegistryFromDir_EmptyBaseDir(t *testing.T) {
	// Test with empty base directory
	regs, fsys, err := LoadUserRegistryFromDir("")

	require.NoError(t, err, "LoadUserRegistryFromDir() should not error for empty path")
	require.Nil(t, regs, "LoadUserRegistryFromDir() should return nil registrations")
	require.Nil(t, fsys, "LoadUserRegistryFromDir() should return nil fs")
}

func TestLoadUserRegistryFromDir_Empty(t *testing.T) {
	// Create a temporary directory with empty workflows subdirectory
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, "workflows")
	require.NoError(t, os.MkdirAll(workflowsDir, 0755))

	regs, fsys, err := LoadUserRegistryFromDir(tmpDir)

	// Empty directory should return nil registrations but with a valid FS
	// (error from "no workflow registrations found" is logged and handled gracefully)
	require.NoError(t, err, "LoadUserRegistryFromDir() should not error for empty directory")
	require.Nil(t, regs, "LoadUserRegistryFromDir() should return nil registrations for empty dir")
	require.NotNil(t, fsys, "LoadUserRegistryFromDir() should return non-nil fs for valid directory")
}

func TestLoadUserRegistryFromDir_NoWorkflowsSubdir(t *testing.T) {
	// Create a temporary directory WITHOUT workflows subdirectory
	tmpDir := t.TempDir()

	regs, fsys, err := LoadUserRegistryFromDir(tmpDir)

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

	regs, fsys, err := LoadUserRegistryFromDir(tmpDir)

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

	regs, fsys, err := LoadUserRegistryFromDir(tmpDir)

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

	regs, fsys, err := LoadUserRegistryFromDir(filePath)

	// File instead of directory should return nil gracefully
	require.NoError(t, err, "LoadUserRegistryFromDir() should not error for file path")
	require.Nil(t, regs, "LoadUserRegistryFromDir() should return nil registrations")
	require.Nil(t, fsys, "LoadUserRegistryFromDir() should return nil fs")
}
