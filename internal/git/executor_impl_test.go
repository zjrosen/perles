package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		// Use a temp dir which should not be a git repo
		tempDir := t.TempDir()
		executor := NewRealExecutor(tempDir)
		require.False(t, executor.IsGitRepo(), "IsGitRepo() = true for temp dir, want false")
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

// TestRealExecutor_CreateWorktreeWithContext_Success tests creating a worktree with context.
// This test creates an isolated temp git repo to avoid polluting the project repo.
func TestRealExecutor_CreateWorktreeWithContext_Success(t *testing.T) {
	// Create a temp directory for the main repo
	repoDir := t.TempDir()

	// Initialize a git repo with an initial commit
	initCmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, args := range initCmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git command %v failed: %s", args, out)
	}

	// Create an initial commit (required for worktrees)
	testFile := filepath.Join(repoDir, "README.md")
	require.NoError(t, os.WriteFile(testFile, []byte("# Test\n"), 0644))
	commitCmds := [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "Initial commit"},
	}
	for _, args := range commitCmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git command %v failed: %s", args, out)
	}

	executor := NewRealExecutor(repoDir)
	require.True(t, executor.IsGitRepo(), "temp dir should be a git repo")

	// Create a temp directory for the worktree (outside the repo)
	worktreeDir := t.TempDir()
	worktreePath := filepath.Join(worktreeDir, "test-worktree-ctx")
	branchName := "test-worktree-ctx-branch"

	// Create worktree with context (success case)
	ctx := context.Background()
	err := executor.CreateWorktreeWithContext(ctx, worktreePath, branchName, "")
	require.NoError(t, err, "CreateWorktreeWithContext() error")

	// Verify worktree was created
	_, err = os.Stat(worktreePath)
	require.False(t, os.IsNotExist(err), "Worktree directory was not created")

	// Cleanup: Remove the worktree
	err = executor.RemoveWorktree(worktreePath)
	require.NoError(t, err, "RemoveWorktree() error")
}

// TestRealExecutor_CreateWorktreeWithContext_Timeout tests that context timeout is respected.
// This test uses an already-cancelled context to verify timeout behavior.
func TestRealExecutor_CreateWorktreeWithContext_Timeout(t *testing.T) {
	// Create a temp directory for the main repo
	repoDir := t.TempDir()

	// Initialize a git repo with an initial commit
	initCmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test User"},
	}
	for _, args := range initCmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git command %v failed: %s", args, out)
	}

	// Create an initial commit (required for worktrees)
	testFile := filepath.Join(repoDir, "README.md")
	require.NoError(t, os.WriteFile(testFile, []byte("# Test\n"), 0644))
	commitCmds := [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "Initial commit"},
	}
	for _, args := range commitCmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git command %v failed: %s", args, out)
	}

	executor := NewRealExecutor(repoDir)
	require.True(t, executor.IsGitRepo(), "temp dir should be a git repo")

	// Create a temp directory for the worktree (outside the repo)
	worktreeDir := t.TempDir()
	worktreePath := filepath.Join(worktreeDir, "test-worktree-timeout")
	branchName := "test-worktree-timeout-branch"

	// Use an already-expired context to simulate timeout
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer cancel()

	// CreateWorktreeWithContext should fail with timeout error
	err := executor.CreateWorktreeWithContext(ctx, worktreePath, branchName, "")
	require.Error(t, err, "CreateWorktreeWithContext with expired context should error")
	require.ErrorIs(t, err, ErrWorktreeTimeout, "Error should be ErrWorktreeTimeout")

	// Worktree should not have been created
	_, statErr := os.Stat(worktreePath)
	require.True(t, os.IsNotExist(statErr), "Worktree directory should not exist after timeout")
}

// TestErrWorktreeTimeout tests the timeout error type.
func TestErrWorktreeTimeout(t *testing.T) {
	// Verify the error is defined and usable
	require.NotNil(t, ErrWorktreeTimeout)
	require.Contains(t, ErrWorktreeTimeout.Error(), "timed out")
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

// TestRealExecutor_GetDiff_Success tests GetDiff with a valid ref.
func TestRealExecutor_GetDiff_Success(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// Diff HEAD against itself should return empty (no changes)
	diff, err := executor.GetDiff("HEAD")
	require.NoError(t, err, "GetDiff(HEAD) error")
	// The diff may or may not be empty depending on working tree state
	// We just verify no error occurred
	t.Logf("GetDiff(HEAD) returned %d bytes", len(diff))
}

// TestRealExecutor_GetDiff_InvalidRef tests GetDiff with an invalid ref.
func TestRealExecutor_GetDiff_InvalidRef(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// Invalid ref should return an error
	_, err = executor.GetDiff("nonexistent-ref-abc123xyz")
	require.Error(t, err, "GetDiff(invalid-ref) should return error")
}

// TestRealExecutor_GetDiff_EmptyDiff tests GetDiff when there are no changes.
func TestRealExecutor_GetDiff_EmptyDiff(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// HEAD..HEAD should always be empty
	diff, err := executor.GetDiff("HEAD..HEAD")
	require.NoError(t, err, "GetDiff(HEAD..HEAD) error")
	require.Empty(t, diff, "GetDiff(HEAD..HEAD) should be empty")
}

// TestRealExecutor_GetDiffStat_Success tests GetDiffStat returns numstat format.
func TestRealExecutor_GetDiffStat_Success(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// Get diff stat against HEAD~1 if there are commits
	// If there's only one commit, this will fail gracefully
	stat, err := executor.GetDiffStat("HEAD~1")
	if err != nil {
		// May fail if repo has only one commit
		t.Logf("GetDiffStat(HEAD~1) error (may be expected): %v", err)
		return
	}

	// If there are changes, numstat format should have tab-separated fields
	// Format: "additions\tdeletions\tpath"
	if stat != "" {
		lines := strings.Split(stat, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			// Each line should have tab-separated values
			parts := strings.Split(line, "\t")
			require.GreaterOrEqual(t, len(parts), 3, "numstat line should have at least 3 fields: %q", line)
		}
	}
	t.Logf("GetDiffStat(HEAD~1) returned %d bytes", len(stat))
}

// TestRealExecutor_GetFileDiff_Success tests GetFileDiff returns diff for a single file.
func TestRealExecutor_GetFileDiff_Success(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// Get diff for a file that exists (using HEAD to compare working tree)
	// The executor.go file should exist
	diff, err := executor.GetFileDiff("HEAD", "internal/git/executor.go")
	require.NoError(t, err, "GetFileDiff() error")

	// The diff may be empty if no working tree changes
	t.Logf("GetFileDiff(HEAD, executor.go) returned %d bytes", len(diff))
}

// TestRealExecutor_GetFileDiff_NonexistentFile tests GetFileDiff with a non-existent file.
func TestRealExecutor_GetFileDiff_NonexistentFile(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// Get diff for a file that doesn't exist
	diff, err := executor.GetFileDiff("HEAD", "nonexistent-file-xyz.txt")
	// Git returns empty diff for non-existent files (not an error)
	require.NoError(t, err, "GetFileDiff() for non-existent file should not error")
	require.Empty(t, diff, "GetFileDiff() for non-existent file should be empty")
}

// TestRealExecutor_GetDiff_NotGitRepo tests diff operations outside a git repo.
func TestRealExecutor_GetDiff_NotGitRepo(t *testing.T) {
	// Use a temp dir which should not be a git repo
	tempDir := t.TempDir()
	executor := NewRealExecutor(tempDir)

	_, err := executor.GetDiff("HEAD")
	require.Error(t, err, "GetDiff() outside git repo should error")
	require.ErrorIs(t, err, ErrNotGitRepo, "GetDiff() should return ErrNotGitRepo")
}

// TestErrDiffTimeout tests the timeout error type.
func TestErrDiffTimeout(t *testing.T) {
	// Just verify the error is defined and usable
	require.NotNil(t, ErrDiffTimeout)
	require.Contains(t, ErrDiffTimeout.Error(), "timed out")
}

// TestRunGitOutputWithContext_Timeout tests the timeout mechanism.
// Note: This test doesn't actually trigger a timeout (would require a slow command),
// but verifies the context handling code path.
func TestRunGitOutputWithContext_Timeout(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// Use an already-cancelled context to simulate timeout
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// The command should fail due to cancelled context
	_, err = executor.runGitOutputWithContext(ctx, "status")
	require.Error(t, err, "runGitOutputWithContext with cancelled context should error")
}

// TestDiffTimeout_Value tests that diffTimeout constant is set correctly.
func TestDiffTimeout_Value(t *testing.T) {
	// Verify timeout is 5 seconds as per spec
	require.Equal(t, 5*time.Second, diffTimeout, "diffTimeout should be 5 seconds")
}

// TestRealExecutor_GetCommitLog_Success tests GetCommitLog with a normal repo.
func TestRealExecutor_GetCommitLog_Success(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// Get the last 10 commits
	commits, err := executor.GetCommitLog(10)
	require.NoError(t, err, "GetCommitLog(10) error")

	// The perles repo should have commits
	require.NotEmpty(t, commits, "GetCommitLog() returned empty for perles repo")

	// Verify the commit structure
	for i, c := range commits {
		require.NotEmpty(t, c.Hash, "commit[%d].Hash is empty", i)
		require.Len(t, c.Hash, 40, "commit[%d].Hash should be 40 chars, got %d: %s", i, len(c.Hash), c.Hash)
		require.NotEmpty(t, c.ShortHash, "commit[%d].ShortHash is empty", i)
		require.GreaterOrEqual(t, len(c.ShortHash), 7, "commit[%d].ShortHash should be at least 7 chars, got %d: %s", i, len(c.ShortHash), c.ShortHash)
		require.NotEmpty(t, c.Subject, "commit[%d].Subject is empty", i)
		require.NotEmpty(t, c.Author, "commit[%d].Author is empty", i)
		require.False(t, c.Date.IsZero(), "commit[%d].Date is zero", i)
	}

	t.Logf("GetCommitLog(10) returned %d commits", len(commits))
}

// TestRealExecutor_GetCommitLog_LimitRespected tests that the limit parameter is respected.
func TestRealExecutor_GetCommitLog_LimitRespected(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// Get only 1 commit
	commits, err := executor.GetCommitLog(1)
	require.NoError(t, err, "GetCommitLog(1) error")

	require.Len(t, commits, 1, "GetCommitLog(1) should return exactly 1 commit")
}

// TestRealExecutor_GetCommitLog_NotGitRepo tests GetCommitLog outside a git repo.
func TestRealExecutor_GetCommitLog_NotGitRepo(t *testing.T) {
	// Use a temp dir which should not be a git repo
	tempDir := t.TempDir()
	executor := NewRealExecutor(tempDir)

	_, err := executor.GetCommitLog(10)
	require.Error(t, err, "GetCommitLog() outside git repo should error")
	require.ErrorIs(t, err, ErrNotGitRepo, "GetCommitLog() should return ErrNotGitRepo")
}

// TestRealExecutor_GetCommitLog_DetachedHead tests GetCommitLog in detached HEAD state.
// This test just verifies the function works regardless of HEAD state.
func TestRealExecutor_GetCommitLog_DetachedHead(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// Check if we're in detached HEAD (common in CI)
	isDetached, err := executor.IsDetachedHead()
	require.NoError(t, err)

	// GetCommitLog should work in both normal and detached HEAD states
	commits, err := executor.GetCommitLog(5)
	require.NoError(t, err, "GetCommitLog() error (detached=%v)", isDetached)
	require.NotEmpty(t, commits, "GetCommitLog() should return commits even in detached HEAD")

	if isDetached {
		t.Log("GetCommitLog() succeeded in detached HEAD state")
	}
}

// TestParseCommitLog tests the commit log parser with various inputs.
func TestParseCommitLog(t *testing.T) {
	// ASCII Record Separator used as delimiter in git log output
	d := "\x1e"

	tests := []struct {
		name  string
		input string
		want  []CommitInfo
	}{
		{
			name:  "single commit",
			input: "abc123def456789012345678901234567890abcd" + d + "abc123d" + d + "Fix auth bug" + d + "John Doe" + d + "2024-01-15T10:30:00-05:00\n",
			want: []CommitInfo{
				{
					Hash:      "abc123def456789012345678901234567890abcd",
					ShortHash: "abc123d",
					Subject:   "Fix auth bug",
					Author:    "John Doe",
					Date:      time.Date(2024, 1, 15, 10, 30, 0, 0, time.FixedZone("", -5*60*60)),
				},
			},
		},
		{
			name: "multiple commits",
			input: "abc123def456789012345678901234567890abcd" + d + "abc123d" + d + "First commit" + d + "Alice" + d + "2024-01-15T10:30:00Z\n" +
				"def456abc789012345678901234567890abcdef01" + d + "def456a" + d + "Second commit" + d + "Bob" + d + "2024-01-14T09:00:00Z\n",
			want: []CommitInfo{
				{
					Hash:      "abc123def456789012345678901234567890abcd",
					ShortHash: "abc123d",
					Subject:   "First commit",
					Author:    "Alice",
					Date:      time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
				},
				{
					Hash:      "def456abc789012345678901234567890abcdef01",
					ShortHash: "def456a",
					Subject:   "Second commit",
					Author:    "Bob",
					Date:      time.Date(2024, 1, 14, 9, 0, 0, 0, time.UTC),
				},
			},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "commit with pipe in subject",
			input: "abc123def456789012345678901234567890abcd" + d + "abc123d" + d + "Add foo | bar feature" + d + "Dev" + d + "2024-01-15T10:30:00Z\n",
			want: []CommitInfo{
				{
					Hash:      "abc123def456789012345678901234567890abcd",
					ShortHash: "abc123d",
					Subject:   "Add foo | bar feature",
					Author:    "Dev",
					Date:      time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
				},
			},
		},
		{
			name:  "invalid line (too few fields)",
			input: "invalid" + d + "line",
			want:  nil,
		},
		{
			name:  "no trailing newline",
			input: "abc123def456789012345678901234567890abcd" + d + "abc123d" + d + "No newline" + d + "Dev" + d + "2024-01-15T10:30:00Z",
			want: []CommitInfo{
				{
					Hash:      "abc123def456789012345678901234567890abcd",
					ShortHash: "abc123d",
					Subject:   "No newline",
					Author:    "Dev",
					Date:      time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseCommitLog(tc.input)

			require.Len(t, got, len(tc.want), "parseCommitLog() returned wrong number of commits")

			for i := range got {
				require.Equal(t, tc.want[i].Hash, got[i].Hash, "commit[%d].Hash", i)
				require.Equal(t, tc.want[i].ShortHash, got[i].ShortHash, "commit[%d].ShortHash", i)
				require.Equal(t, tc.want[i].Subject, got[i].Subject, "commit[%d].Subject", i)
				require.Equal(t, tc.want[i].Author, got[i].Author, "commit[%d].Author", i)
				require.True(t, tc.want[i].Date.Equal(got[i].Date), "commit[%d].Date: want %v, got %v", i, tc.want[i].Date, got[i].Date)
			}
		})
	}
}

// TestParseCommitLog_InvalidDate tests date parsing fallback.
func TestParseCommitLog_InvalidDate(t *testing.T) {
	d := "\x1e"
	input := "abc123def456789012345678901234567890abcd" + d + "abc123d" + d + "Test" + d + "Dev" + d + "invalid-date"
	commits := parseCommitLog(input)

	require.Len(t, commits, 1)
	require.True(t, commits[0].Date.IsZero(), "Invalid date should result in zero time")
}

// TestRealExecutor_GetCommitLogForRef_EmptyRef tests that empty ref returns HEAD commits.
func TestRealExecutor_GetCommitLogForRef_EmptyRef(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// Empty ref should return same commits as GetCommitLog
	commitsForRef, err := executor.GetCommitLogForRef("", 5)
	require.NoError(t, err, "GetCommitLogForRef('', 5) error")
	require.NotEmpty(t, commitsForRef, "GetCommitLogForRef('', 5) should return commits")

	// Compare with GetCommitLog - should be identical
	commits, err := executor.GetCommitLog(5)
	require.NoError(t, err, "GetCommitLog(5) error")

	require.Equal(t, len(commits), len(commitsForRef), "GetCommitLogForRef('') should return same count as GetCommitLog")
	for i := range commits {
		require.Equal(t, commits[i].Hash, commitsForRef[i].Hash, "commit[%d].Hash mismatch", i)
	}
}

// TestRealExecutor_GetCommitLogForRef_ValidBranch tests that valid branch ref returns commits.
func TestRealExecutor_GetCommitLogForRef_ValidBranch(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// Use HEAD which always exists (works in CI with detached HEAD)
	ref := "HEAD"

	// Get commits for HEAD
	commits, err := executor.GetCommitLogForRef(ref, 5)
	require.NoError(t, err, "GetCommitLogForRef(%q, 5) error", ref)
	require.NotEmpty(t, commits, "GetCommitLogForRef(%q, 5) should return commits", ref)

	// Verify commit structure
	for i, c := range commits {
		require.NotEmpty(t, c.Hash, "commit[%d].Hash is empty", i)
		require.Len(t, c.Hash, 40, "commit[%d].Hash should be 40 chars", i)
		require.NotEmpty(t, c.ShortHash, "commit[%d].ShortHash is empty", i)
		require.NotEmpty(t, c.Subject, "commit[%d].Subject is empty", i)
		require.NotEmpty(t, c.Author, "commit[%d].Author is empty", i)
	}

	t.Logf("GetCommitLogForRef(%q, 5) returned %d commits", ref, len(commits))
}

// TestRealExecutor_GetCommitLogForRef_InvalidRef tests that invalid ref returns error.
func TestRealExecutor_GetCommitLogForRef_InvalidRef(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// Invalid ref should return error
	_, err = executor.GetCommitLogForRef("nonexistent-branch-xyz123", 5)
	require.Error(t, err, "GetCommitLogForRef for invalid ref should return error")
	require.Contains(t, err.Error(), "unknown revision", "Error should mention unknown revision")
}

// TestRealExecutor_GetCommitLogForRef_Limit tests that limit parameter is respected.
func TestRealExecutor_GetCommitLogForRef_Limit(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	// Use HEAD which always exists (works in CI with detached HEAD)
	ref := "HEAD"

	// Get 1 commit
	commits1, err := executor.GetCommitLogForRef(ref, 1)
	require.NoError(t, err)
	require.Len(t, commits1, 1, "GetCommitLogForRef with limit=1 should return 1 commit")

	// Get 3 commits
	commits3, err := executor.GetCommitLogForRef(ref, 3)
	require.NoError(t, err)
	require.LessOrEqual(t, len(commits3), 3, "GetCommitLogForRef with limit=3 should return at most 3 commits")

	// If we got 3 commits, verify the first one matches
	if len(commits3) >= 1 && len(commits1) >= 1 {
		require.Equal(t, commits1[0].Hash, commits3[0].Hash, "First commit should be the same regardless of limit")
	}
}

// TestRealExecutor_GetCommitLogForRef_NotGitRepo tests GetCommitLogForRef outside a git repo.
func TestRealExecutor_GetCommitLogForRef_NotGitRepo(t *testing.T) {
	// Use a temp dir which should not be a git repo
	tempDir := t.TempDir()
	executor := NewRealExecutor(tempDir)

	_, err := executor.GetCommitLogForRef("main", 10)
	require.Error(t, err, "GetCommitLogForRef() outside git repo should error")
	require.ErrorIs(t, err, ErrNotGitRepo, "GetCommitLogForRef() should return ErrNotGitRepo")
}

// TestRealExecutor_ValidateBranchName_Valid tests ValidateBranchName with valid branch names.
func TestRealExecutor_ValidateBranchName_Valid(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	validNames := []string{
		"feature/foo",
		"fix-123",
		"release-v1.0",
		"main",
		"develop",
		"my-feature-branch",
		"feature/my/nested/branch",
		"123-numeric-prefix",
		"UPPERCASE",
		"mixedCase",
	}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			err := executor.ValidateBranchName(name)
			require.NoError(t, err, "ValidateBranchName(%q) should return nil for valid name", name)
		})
	}
}

// TestRealExecutor_ValidateBranchName_InvalidSpaces tests ValidateBranchName rejects names with spaces.
func TestRealExecutor_ValidateBranchName_InvalidSpaces(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	err = executor.ValidateBranchName("my branch")
	require.ErrorIs(t, err, ErrInvalidBranchName, "ValidateBranchName('my branch') should return ErrInvalidBranchName")
}

// TestRealExecutor_ValidateBranchName_InvalidTilde tests ValidateBranchName rejects names with tilde.
func TestRealExecutor_ValidateBranchName_InvalidTilde(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	err = executor.ValidateBranchName("feat~name")
	require.ErrorIs(t, err, ErrInvalidBranchName, "ValidateBranchName('feat~name') should return ErrInvalidBranchName")
}

// TestRealExecutor_ValidateBranchName_InvalidCaret tests ValidateBranchName rejects names with caret.
func TestRealExecutor_ValidateBranchName_InvalidCaret(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	err = executor.ValidateBranchName("fix^123")
	require.ErrorIs(t, err, ErrInvalidBranchName, "ValidateBranchName('fix^123') should return ErrInvalidBranchName")
}

// TestRealExecutor_ValidateBranchName_InvalidLeadingDot tests ValidateBranchName rejects names starting with dot.
func TestRealExecutor_ValidateBranchName_InvalidLeadingDot(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	err = executor.ValidateBranchName(".hidden")
	require.ErrorIs(t, err, ErrInvalidBranchName, "ValidateBranchName('.hidden') should return ErrInvalidBranchName")
}

// TestRealExecutor_ValidateBranchName_InvalidLockSuffix tests ValidateBranchName rejects names ending with .lock.
func TestRealExecutor_ValidateBranchName_InvalidLockSuffix(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	executor := NewRealExecutor(cwd)

	err = executor.ValidateBranchName("name.lock")
	require.ErrorIs(t, err, ErrInvalidBranchName, "ValidateBranchName('name.lock') should return ErrInvalidBranchName")
}

// TestParseGitError_InvalidBranchName tests parseGitError correctly handles invalid branch name errors.
func TestParseGitError_InvalidBranchName(t *testing.T) {
	originalErr := errors.New("exit status 128")

	err := parseGitError("fatal: 'my branch' is not a valid branch name", originalErr)
	require.ErrorIs(t, err, ErrInvalidBranchName, "parseGitError should return ErrInvalidBranchName for invalid branch name stderr")
}
