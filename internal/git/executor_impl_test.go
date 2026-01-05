package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRealExecutor_NewRealExecutor tests the constructor.
func TestRealExecutor_NewRealExecutor(t *testing.T) {
	workDir := "/some/path"
	executor := NewRealExecutor(workDir)

	require.NotNil(t, executor, "NewRealExecutor returned nil")
	require.Equal(t, workDir, executor.workDir)
}

// TestRealExecutor_IsGitRepo tests the IsGitRepo method.
func TestRealExecutor_IsGitRepo(t *testing.T) {
	t.Run("in git repo", func(t *testing.T) {
		// Use the current repo (perles)
		cwd, err := os.Getwd()
		require.NoError(t, err)

		executor := NewRealExecutor(cwd)
		require.True(t, executor.IsGitRepo(), "IsGitRepo() = false, want true (running in perles repo)")
	})

	t.Run("not in git repo", func(t *testing.T) {
		// Use /tmp which should not be a git repo
		executor := NewRealExecutor("/tmp")
		require.False(t, executor.IsGitRepo(), "IsGitRepo() = true for /tmp, want false")
	})
}

// TestRealExecutor_GetCurrentBranch tests the GetCurrentBranch method.
func TestRealExecutor_GetCurrentBranch(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)
	branch, err := executor.GetCurrentBranch()

	// In CI (detached HEAD), we get ErrDetachedHead - that's valid
	if errors.Is(err, ErrDetachedHead) {
		t.Log("GetCurrentBranch() returned ErrDetachedHead (detached HEAD state, common in CI)")
		return
	}

	require.NoError(t, err, "GetCurrentBranch() error")
	require.NotEmpty(t, branch, "GetCurrentBranch() returned empty string")

	// Branch should be a valid name (no refs/heads/ prefix)
	require.False(t, strings.HasPrefix(branch, "refs/"), "GetCurrentBranch() = %q, should not have refs/ prefix", branch)
}

// TestRealExecutor_GetMainBranch tests the GetMainBranch method.
func TestRealExecutor_GetMainBranch(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)
	mainBranch, err := executor.GetMainBranch()
	require.NoError(t, err, "GetMainBranch() error")

	// Should return either "main" or "master"
	if mainBranch != "main" && mainBranch != "master" {
		t.Logf("GetMainBranch() = %q (custom main branch)", mainBranch)
	}

	require.NotEmpty(t, mainBranch, "GetMainBranch() returned empty string")
}

// TestRealExecutor_IsOnMainBranch tests the IsOnMainBranch method.
func TestRealExecutor_IsOnMainBranch(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// This should not error (regardless of whether we're on main)
	_, err = executor.IsOnMainBranch()
	require.NoError(t, err, "IsOnMainBranch() error")
}

// TestRealExecutor_HasUncommittedChanges tests the HasUncommittedChanges method.
func TestRealExecutor_HasUncommittedChanges(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// This should not error
	_, err = executor.HasUncommittedChanges()
	require.NoError(t, err, "HasUncommittedChanges() error")
}

// TestRealExecutor_GetRepoRoot tests the GetRepoRoot method.
func TestRealExecutor_GetRepoRoot(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)
	root, err := executor.GetRepoRoot()
	require.NoError(t, err, "GetRepoRoot() error")
	require.NotEmpty(t, root, "GetRepoRoot() returned empty string")

	// Root should be an absolute path
	require.True(t, filepath.IsAbs(root), "GetRepoRoot() = %q, want absolute path", root)
}

// TestRealExecutor_IsBareRepo tests the IsBareRepo method.
func TestRealExecutor_IsBareRepo(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)
	isBare, err := executor.IsBareRepo()
	require.NoError(t, err, "IsBareRepo() error")

	// The perles repo should not be bare
	require.False(t, isBare, "IsBareRepo() = true for perles repo, want false")
}

// TestRealExecutor_IsDetachedHead tests the IsDetachedHead method.
func TestRealExecutor_IsDetachedHead(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// This should not error
	_, err = executor.IsDetachedHead()
	require.NoError(t, err, "IsDetachedHead() error")
}

// TestRealExecutor_IsWorktree tests the IsWorktree method.
func TestRealExecutor_IsWorktree(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)
	isWorktree, err := executor.IsWorktree()
	require.NoError(t, err, "IsWorktree() error")

	// The main perles repo should not be a worktree
	if isWorktree {
		t.Logf("IsWorktree() = true (running in a worktree)")
	}
}

// TestRealExecutor_DetermineWorktreePath tests worktree path determination.
func TestRealExecutor_DetermineWorktreePath(t *testing.T) {
	t.Run("normal repo", func(t *testing.T) {
		cwd, err := os.Getwd()
		require.NoError(t, err)

		executor := NewRealExecutor(cwd)
		path, err := executor.DetermineWorktreePath("abc123def456")
		require.NoError(t, err, "DetermineWorktreePath() error")
		require.NotEmpty(t, path, "DetermineWorktreePath() returned empty string")

		// Path should contain the short session ID
		require.Contains(t, path, "abc123de", "DetermineWorktreePath() should contain session ID prefix")
	})

	t.Run("short session ID", func(t *testing.T) {
		cwd, err := os.Getwd()
		require.NoError(t, err)

		executor := NewRealExecutor(cwd)
		path, err := executor.DetermineWorktreePath("short")
		require.NoError(t, err, "DetermineWorktreePath() error")

		// Should handle short session ID without panic
		require.Contains(t, path, "short", "DetermineWorktreePath() should contain session ID")
	})
}

// TestRealExecutor_DetermineWorktreePath_RestrictedParent tests unsafe parent handling.
func TestRealExecutor_DetermineWorktreePath_RestrictedParent(t *testing.T) {
	// Test the isSafeParentDir function directly
	tests := []struct {
		dir  string
		safe bool
	}{
		{"/", false},
		{"/System", false},
		{"/System/Library", false},
		{"/usr", false},
		{"/usr/local", false},
		{"/bin", false},
		{"/sbin", false},
		{"/etc", false},
		{"/var", false},
		{"/private", false},
		{"/private/tmp", false},
		// Note: /home and /Users would be safe, but we can't test writability easily
	}

	for _, tc := range tests {
		t.Run(tc.dir, func(t *testing.T) {
			result := isSafeParentDir(tc.dir)
			require.Equal(t, tc.safe, result, "isSafeParentDir(%q)", tc.dir)
		})
	}
}

// TestRealExecutor_ListWorktrees tests listing worktrees.
func TestRealExecutor_ListWorktrees(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)
	worktrees, err := executor.ListWorktrees()
	require.NoError(t, err, "ListWorktrees() error")

	// There should be at least one worktree (the main one)
	require.NotEmpty(t, worktrees, "ListWorktrees() returned empty list, expected at least main worktree")

	// First worktree should be the main repo
	if len(worktrees) > 0 {
		main := worktrees[0]
		require.NotEmpty(t, main.Path, "First worktree has empty Path")
		require.NotEmpty(t, main.HEAD, "First worktree has empty HEAD")
	}
}

// TestRealExecutor_PruneWorktrees tests pruning worktrees.
func TestRealExecutor_PruneWorktrees(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// Prune should succeed (even if nothing to prune)
	err = executor.PruneWorktrees()
	require.NoError(t, err, "PruneWorktrees() error")
}

// TestParseWorktreeList tests the worktree list parser.
func TestParseWorktreeList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []WorktreeInfo
	}{
		{
			name: "single worktree",
			input: `worktree /path/to/repo
HEAD abc123def456
branch refs/heads/main

`,
			want: []WorktreeInfo{
				{Path: "/path/to/repo", HEAD: "abc123def456", Branch: "main"},
			},
		},
		{
			name: "multiple worktrees",
			input: `worktree /path/to/repo
HEAD abc123def456
branch refs/heads/main

worktree /path/to/worktree
HEAD def456abc789
branch refs/heads/feature

`,
			want: []WorktreeInfo{
				{Path: "/path/to/repo", HEAD: "abc123def456", Branch: "main"},
				{Path: "/path/to/worktree", HEAD: "def456abc789", Branch: "feature"},
			},
		},
		{
			name: "detached head",
			input: `worktree /path/to/repo
HEAD abc123def456
detached

`,
			want: []WorktreeInfo{
				{Path: "/path/to/repo", HEAD: "abc123def456", Branch: ""},
			},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name: "no trailing newline",
			input: `worktree /path/to/repo
HEAD abc123def456
branch refs/heads/main`,
			want: []WorktreeInfo{
				{Path: "/path/to/repo", HEAD: "abc123def456", Branch: "main"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseWorktreeList(tc.input)

			require.Len(t, got, len(tc.want), "parseWorktreeList() returned wrong number of worktrees")

			for i := range got {
				require.Equal(t, tc.want[i].Path, got[i].Path, "worktree[%d].Path", i)
				require.Equal(t, tc.want[i].HEAD, got[i].HEAD, "worktree[%d].HEAD", i)
				require.Equal(t, tc.want[i].Branch, got[i].Branch, "worktree[%d].Branch", i)
			}
		})
	}
}

// TestParseGitError tests git error parsing.
func TestParseGitError(t *testing.T) {
	originalErr := errors.New("exit status 128")

	tests := []struct {
		name      string
		stderr    string
		wantError error
	}{
		{
			name:      "branch already checked out",
			stderr:    "fatal: 'feature' is already checked out at '/path/to/worktree'",
			wantError: ErrBranchAlreadyCheckedOut,
		},
		{
			name:      "path already exists",
			stderr:    "fatal: '/path/to/worktree' already exists",
			wantError: ErrPathAlreadyExists,
		},
		{
			name:      "worktree locked",
			stderr:    "fatal: '/path/to/worktree' is locked",
			wantError: ErrWorktreeLocked,
		},
		{
			name:      "not a git repository",
			stderr:    "fatal: not a git repository (or any of the parent directories): .git",
			wantError: ErrNotGitRepo,
		},
		{
			name:      "unknown error",
			stderr:    "fatal: some other error",
			wantError: nil, // Should not match any specific error
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := parseGitError(tc.stderr, originalErr)

			if tc.wantError != nil {
				require.ErrorIs(t, err, tc.wantError, "parseGitError() should return expected error")
			} else {
				// For unknown errors, should still contain the stderr
				require.Contains(t, err.Error(), tc.stderr, "parseGitError() should contain stderr")
			}
		})
	}
}

// TestRealExecutor_CreateWorktree_Integration tests creating and removing worktrees.
// This test creates a real worktree and then cleans up.
func TestRealExecutor_CreateWorktree_Integration(t *testing.T) {
	// Skip if not in a git repo
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)
	if !executor.IsGitRepo() {
		t.Skip("Not in a git repository")
	}

	// Create a temp directory for the worktree
	tempDir := t.TempDir()
	worktreePath := filepath.Join(tempDir, "test-worktree")
	branchName := "test-worktree-branch-" + filepath.Base(tempDir)

	// Create worktree with new branch based on HEAD (empty baseBranch)
	err = executor.CreateWorktree(worktreePath, branchName, "")
	require.NoError(t, err, "CreateWorktree() error")

	// Verify worktree was created
	_, err = os.Stat(worktreePath)
	require.False(t, os.IsNotExist(err), "Worktree directory was not created")

	// List worktrees and verify our new one exists
	worktrees, err := executor.ListWorktrees()
	require.NoError(t, err, "ListWorktrees() error")

	// Resolve symlinks for comparison (macOS uses /var -> /private/var)
	worktreePathReal, _ := filepath.EvalSymlinks(worktreePath)
	if worktreePathReal == "" {
		worktreePathReal = worktreePath
	}

	found := false
	for _, wt := range worktrees {
		wtPathReal, _ := filepath.EvalSymlinks(wt.Path)
		if wtPathReal == "" {
			wtPathReal = wt.Path
		}
		if wtPathReal == worktreePathReal || wt.Path == worktreePath {
			found = true
			require.Equal(t, branchName, wt.Branch, "Worktree branch mismatch")
			break
		}
	}
	require.True(t, found, "New worktree not found in ListWorktrees(). Expected path %q (real: %q), got worktrees: %+v",
		worktreePath, worktreePathReal, worktrees)

	// Remove worktree
	err = executor.RemoveWorktree(worktreePath)
	require.NoError(t, err, "RemoveWorktree() error")

	// Verify worktree was removed
	_, err = os.Stat(worktreePath)
	require.True(t, os.IsNotExist(err), "Worktree directory still exists after removal")

	// Clean up the test branch
	cmd := exec.Command("git", "branch", "-D", branchName)
	cmd.Dir = cwd
	_ = cmd.Run() // Ignore error - branch may already be deleted
}

// TestRealExecutor_ErrorParsing_BranchConflict tests error detection for branch conflicts.
func TestRealExecutor_ErrorParsing_BranchConflict(t *testing.T) {
	// Test that the error is correctly wrapped
	err := parseGitError("fatal: 'main' is already checked out at '/other/worktree'", errors.New("exit status 128"))
	require.ErrorIs(t, err, ErrBranchAlreadyCheckedOut)
}

// TestRealExecutor_ErrorParsing_PathExists tests error detection for path conflicts.
func TestRealExecutor_ErrorParsing_PathExists(t *testing.T) {
	err := parseGitError("fatal: '/path/to/worktree' already exists", errors.New("exit status 128"))
	require.ErrorIs(t, err, ErrPathAlreadyExists)
}

// TestRealExecutor_ErrorParsing_Locked tests error detection for locked worktrees.
func TestRealExecutor_ErrorParsing_Locked(t *testing.T) {
	err := parseGitError("fatal: '/path/to/worktree' is locked", errors.New("exit status 128"))
	require.ErrorIs(t, err, ErrWorktreeLocked)
}

// TestInterfaceCompliance verifies RealExecutor implements GitExecutor.
func TestInterfaceCompliance(t *testing.T) {
	var _ GitExecutor = (*RealExecutor)(nil)
}

// TestUnsafeParentDirs tests the unsafe parent directory map.
func TestUnsafeParentDirs(t *testing.T) {
	// These should all be in the unsafe map
	for dir := range unsafeParentDirs {
		require.True(t, unsafeParentDirs[dir], "unsafeParentDirs[%q] should be true", dir)
	}

	// These should NOT be in the unsafe map
	safeDirs := []string{
		"/Users",
		"/home",
		"/Users/username/projects",
		"/opt",
	}
	for _, dir := range safeDirs {
		require.False(t, unsafeParentDirs[dir], "unsafeParentDirs[%q] should be false", dir)
	}
}
