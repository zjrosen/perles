package beads

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// skipIfBDNotEnabled skips tests that require real BD operations.
// Tests are skipped unless:
//   - BD_INTEGRATION_TESTS=1 environment variable is set, OR
//   - The test is run with -tags=integration
//
// This allows tests to run in CI with BD available while skipping
// on developer machines without BD setup.
func skipIfBDNotEnabled(t *testing.T) {
	t.Helper()

	// Check environment variable first
	if os.Getenv("BD_INTEGRATION_TESTS") == "1" {
		// Environment says to run BD tests, but verify bd is available
		if _, err := exec.LookPath("bd"); err != nil {
			require.FailNow(t, "BD_INTEGRATION_TESTS=1 but bd CLI not available")
		}
		return // Run the test
	}

	// Fall back to checking if bd is available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not available (set BD_INTEGRATION_TESTS=1 to require BD tests)")
	}
}

// TestRealExecutor_ImplementsInterface verifies RealExecutor implements BeadsExecutor.
// This is a compile-time check that will fail if the interface is not implemented.
func TestRealExecutor_ImplementsInterface(t *testing.T) {
	var _ BeadsExecutor = (*RealExecutor)(nil)
}

// TestNewRealExecutor tests the constructor.
func TestNewRealExecutor(t *testing.T) {
	executor := NewRealExecutor("/test/dir")
	require.NotNil(t, executor)
	require.Equal(t, "/test/dir", executor.workDir)
}

// TestRealExecutor_ShowIssue_InvalidIssue tests error handling for nonexistent issues.
// This test requires BD CLI. Set BD_INTEGRATION_TESTS=1 to run.
func TestRealExecutor_ShowIssue_InvalidIssue(t *testing.T) {
	skipIfBDNotEnabled(t)

	executor := NewRealExecutor("")
	issue, err := executor.ShowIssue("nonexistent-xyz")
	require.Error(t, err, "expected error for nonexistent issue")
	require.Nil(t, issue)
}

// TestRealExecutor_AddComment_InvalidIssue tests error handling for nonexistent issues.
// This test requires BD CLI. Set BD_INTEGRATION_TESTS=1 to run.
func TestRealExecutor_AddComment_InvalidIssue(t *testing.T) {
	skipIfBDNotEnabled(t)

	executor := NewRealExecutor("")
	err := executor.AddComment("nonexistent-xyz", "test-author", "test comment")
	require.Error(t, err, "expected error for nonexistent issue")
}

// TestRealExecutor_DelegationMethods tests that delegation methods call the underlying functions.
// This test requires BD CLI. Set BD_INTEGRATION_TESTS=1 to run.
func TestRealExecutor_DelegationMethods(t *testing.T) {
	skipIfBDNotEnabled(t)

	executor := NewRealExecutor("")

	t.Run("UpdateStatus", func(t *testing.T) {
		err := executor.UpdateStatus("nonexistent-xyz", StatusInProgress)
		require.Error(t, err, "expected error for nonexistent issue")
	})

	t.Run("UpdatePriority", func(t *testing.T) {
		err := executor.UpdatePriority("nonexistent-xyz", PriorityHigh)
		require.Error(t, err, "expected error for nonexistent issue")
	})

	t.Run("UpdateType", func(t *testing.T) {
		err := executor.UpdateType("nonexistent-xyz", TypeBug)
		require.Error(t, err, "expected error for nonexistent issue")
	})

	t.Run("CloseIssue", func(t *testing.T) {
		err := executor.CloseIssue("nonexistent-xyz", "test reason")
		require.Error(t, err, "expected error for nonexistent issue")
	})

	t.Run("ReopenIssue", func(t *testing.T) {
		err := executor.ReopenIssue("nonexistent-xyz")
		require.Error(t, err, "expected error for nonexistent issue")
	})

	t.Run("DeleteIssues", func(t *testing.T) {
		err := executor.DeleteIssues([]string{"nonexistent-xyz"})
		require.Error(t, err, "expected error for nonexistent issue")
	})

	t.Run("SetLabels", func(t *testing.T) {
		err := executor.SetLabels("nonexistent-xyz", []string{"test-label"})
		require.Error(t, err, "expected error for nonexistent issue")
	})
}

// TestRealExecutor_DeleteIssues_Empty tests that empty slice returns nil.
func TestRealExecutor_DeleteIssues_Empty(t *testing.T) {
	executor := NewRealExecutor("")
	err := executor.DeleteIssues([]string{})
	require.NoError(t, err, "empty slice should not error")
}
