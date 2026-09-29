package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/presentation"
)

// useNilRegistryService clears the package-level registryService (only runApp
// initializes it) for the duration of a test.
func useNilRegistryService(t *testing.T) {
	t.Helper()
	orig := registryService
	registryService = nil
	t.Cleanup(func() { registryService = orig })
}

// resetRegistryListFlags clears the registry:list flag state after a test.
func resetRegistryListFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		registryListCmd.Flags().Lookup("namespace").Changed = false
		regNamespace = ""
	})
}

func decodeRegistrations(t *testing.T, out string) []presentation.RegistrationDTO {
	t.Helper()
	var regs []presentation.RegistrationDTO
	require.NoError(t, json.Unmarshal([]byte(out), &regs), "registry:list should print JSON")
	return regs
}

func TestRegistryListCommand_NilRegistryService_DoesNotPanic(t *testing.T) {
	isolateConfig(t)
	useNilRegistryService(t)
	resetRegistryListFlags(t)

	var out string
	var err error
	require.NotPanics(t, func() {
		out, err = executeRoot(t, "registry:list")
	})

	require.NoError(t, err)
	regs := decodeRegistrations(t, out)
	require.NotEmpty(t, regs, "built-in registrations should be listed")
}

func TestRegistryListCommand_NamespaceFilter_IncludesUserWorkflows(t *testing.T) {
	home, _ := isolateConfig(t)
	useNilRegistryService(t)
	resetRegistryListFlags(t)

	userDir := filepath.Join(home, ".perles", "workflows", "my-flow")
	require.NoError(t, os.MkdirAll(userDir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "template.yaml"), []byte(userWorkflowYAML), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "step1.md"), []byte("# Step 1"), 0o600))

	out, err := executeRoot(t, "registry:list", "-n", "workflow")

	require.NoError(t, err)
	regs := decodeRegistrations(t, out)
	keys := make([]string, 0, len(regs))
	for _, reg := range regs {
		require.Equal(t, "workflow", reg.Namespace)
		keys = append(keys, reg.Key)
	}
	require.Contains(t, keys, "cook", "built-in workflow should be listed")
	require.Contains(t, keys, "my-flow", "user workflow from ~/.perles/workflows should be listed")
}

// userWorkflowYAML is a minimal user workflow template in namespace "workflow".
const userWorkflowYAML = `registry:
  - namespace: "workflow"
    key: "my-flow"
    version: "v1"
    name: "My Flow"
    description: "A user workflow for testing"
    nodes:
      - key: "step1"
        name: "Step 1"
        template: "step1.md"
`
