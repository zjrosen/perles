package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRealExecutor_NewRealExecutor tests the constructor.
func TestRealExecutor_NewRealExecutor(t *testing.T) {
	workDir := "/some/path"
	executor := NewRealExecutor(workDir)

	if executor == nil {
		t.Fatal("NewRealExecutor returned nil")
	}
	if executor.workDir != workDir {
		t.Errorf("workDir = %q, want %q", executor.workDir, workDir)
	}
}

// TestRealExecutor_IsGitRepo tests the IsGitRepo method.
func TestRealExecutor_IsGitRepo(t *testing.T) {
	t.Run("in git repo", func(t *testing.T) {
		// Use the current repo (perles)
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}

		executor := NewRealExecutor(cwd)
		if !executor.IsGitRepo() {
			t.Error("IsGitRepo() = false, want true (running in perles repo)")
		}
	})

	t.Run("not in git repo", func(t *testing.T) {
		// Use /tmp which should not be a git repo
		executor := NewRealExecutor("/tmp")
		if executor.IsGitRepo() {
			t.Error("IsGitRepo() = true for /tmp, want false")
		}
	})
}

// TestRealExecutor_GetCurrentBranch tests the GetCurrentBranch method.
func TestRealExecutor_GetCurrentBranch(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	executor := NewRealExecutor(cwd)
	branch, err := executor.GetCurrentBranch()
	if err != nil {
		t.Fatalf("GetCurrentBranch() error = %v", err)
	}

	if branch == "" {
		t.Error("GetCurrentBranch() returned empty string")
	}

	// Branch should be a valid name (no refs/heads/ prefix)
	if strings.HasPrefix(branch, "refs/") {
		t.Errorf("GetCurrentBranch() = %q, should not have refs/ prefix", branch)
	}
}

// TestRealExecutor_GetMainBranch tests the GetMainBranch method.
func TestRealExecutor_GetMainBranch(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	executor := NewRealExecutor(cwd)
	mainBranch, err := executor.GetMainBranch()
	if err != nil {
		t.Fatalf("GetMainBranch() error = %v", err)
	}

	// Should return either "main" or "master"
	if mainBranch != "main" && mainBranch != "master" {
		t.Logf("GetMainBranch() = %q (custom main branch)", mainBranch)
	}

	if mainBranch == "" {
		t.Error("GetMainBranch() returned empty string")
	}
}

// TestRealExecutor_IsOnMainBranch tests the IsOnMainBranch method.
func TestRealExecutor_IsOnMainBranch(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	executor := NewRealExecutor(cwd)

	// This should not error (regardless of whether we're on main)
	_, err = executor.IsOnMainBranch()
	if err != nil {
		t.Fatalf("IsOnMainBranch() error = %v", err)
	}
}

// TestRealExecutor_HasUncommittedChanges tests the HasUncommittedChanges method.
func TestRealExecutor_HasUncommittedChanges(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	executor := NewRealExecutor(cwd)

	// This should not error
	_, err = executor.HasUncommittedChanges()
	if err != nil {
		t.Fatalf("HasUncommittedChanges() error = %v", err)
	}
}

// TestRealExecutor_GetRepoRoot tests the GetRepoRoot method.
func TestRealExecutor_GetRepoRoot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	executor := NewRealExecutor(cwd)
	root, err := executor.GetRepoRoot()
	if err != nil {
		t.Fatalf("GetRepoRoot() error = %v", err)
	}

	if root == "" {
		t.Error("GetRepoRoot() returned empty string")
	}

	// Root should be an absolute path
	if !filepath.IsAbs(root) {
		t.Errorf("GetRepoRoot() = %q, want absolute path", root)
	}
}

// TestRealExecutor_IsBareRepo tests the IsBareRepo method.
func TestRealExecutor_IsBareRepo(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	executor := NewRealExecutor(cwd)
	isBare, err := executor.IsBareRepo()
	if err != nil {
		t.Fatalf("IsBareRepo() error = %v", err)
	}

	// The perles repo should not be bare
	if isBare {
		t.Error("IsBareRepo() = true for perles repo, want false")
	}
}

// TestRealExecutor_IsDetachedHead tests the IsDetachedHead method.
func TestRealExecutor_IsDetachedHead(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	executor := NewRealExecutor(cwd)

	// This should not error
	_, err = executor.IsDetachedHead()
	if err != nil {
		t.Fatalf("IsDetachedHead() error = %v", err)
	}
}

// TestRealExecutor_IsWorktree tests the IsWorktree method.
func TestRealExecutor_IsWorktree(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	executor := NewRealExecutor(cwd)
	isWorktree, err := executor.IsWorktree()
	if err != nil {
		t.Fatalf("IsWorktree() error = %v", err)
	}

	// The main perles repo should not be a worktree
	if isWorktree {
		t.Logf("IsWorktree() = true (running in a worktree)")
	}
}

// TestRealExecutor_DetermineWorktreePath tests worktree path determination.
func TestRealExecutor_DetermineWorktreePath(t *testing.T) {
	t.Run("normal repo", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}

		executor := NewRealExecutor(cwd)
		path, err := executor.DetermineWorktreePath("abc123def456")
		if err != nil {
			t.Fatalf("DetermineWorktreePath() error = %v", err)
		}

		if path == "" {
			t.Error("DetermineWorktreePath() returned empty string")
		}

		// Path should contain the short session ID
		if !strings.Contains(path, "abc123de") {
			t.Errorf("DetermineWorktreePath() = %q, should contain session ID prefix", path)
		}
	})

	t.Run("short session ID", func(t *testing.T) {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}

		executor := NewRealExecutor(cwd)
		path, err := executor.DetermineWorktreePath("short")
		if err != nil {
			t.Fatalf("DetermineWorktreePath() error = %v", err)
		}

		// Should handle short session ID without panic
		if !strings.Contains(path, "short") {
			t.Errorf("DetermineWorktreePath() = %q, should contain session ID", path)
		}
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
			if result != tc.safe {
				t.Errorf("isSafeParentDir(%q) = %v, want %v", tc.dir, result, tc.safe)
			}
		})
	}
}

// TestRealExecutor_ListWorktrees tests listing worktrees.
func TestRealExecutor_ListWorktrees(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	executor := NewRealExecutor(cwd)
	worktrees, err := executor.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees() error = %v", err)
	}

	// There should be at least one worktree (the main one)
	if len(worktrees) == 0 {
		t.Error("ListWorktrees() returned empty list, expected at least main worktree")
	}

	// First worktree should be the main repo
	if len(worktrees) > 0 {
		main := worktrees[0]
		if main.Path == "" {
			t.Error("First worktree has empty Path")
		}
		if main.HEAD == "" {
			t.Error("First worktree has empty HEAD")
		}
	}
}

// TestRealExecutor_PruneWorktrees tests pruning worktrees.
func TestRealExecutor_PruneWorktrees(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	executor := NewRealExecutor(cwd)

	// Prune should succeed (even if nothing to prune)
	err = executor.PruneWorktrees()
	if err != nil {
		t.Fatalf("PruneWorktrees() error = %v", err)
	}
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

			if len(got) != len(tc.want) {
				t.Fatalf("parseWorktreeList() returned %d worktrees, want %d", len(got), len(tc.want))
			}

			for i := range got {
				if got[i].Path != tc.want[i].Path {
					t.Errorf("worktree[%d].Path = %q, want %q", i, got[i].Path, tc.want[i].Path)
				}
				if got[i].HEAD != tc.want[i].HEAD {
					t.Errorf("worktree[%d].HEAD = %q, want %q", i, got[i].HEAD, tc.want[i].HEAD)
				}
				if got[i].Branch != tc.want[i].Branch {
					t.Errorf("worktree[%d].Branch = %q, want %q", i, got[i].Branch, tc.want[i].Branch)
				}
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
				if !errors.Is(err, tc.wantError) {
					t.Errorf("parseGitError() = %v, want error containing %v", err, tc.wantError)
				}
			} else {
				// For unknown errors, should still contain the stderr
				if !strings.Contains(err.Error(), tc.stderr) {
					t.Errorf("parseGitError() = %v, should contain stderr %q", err, tc.stderr)
				}
			}
		})
	}
}

// TestRealExecutor_CreateWorktree_Integration tests creating and removing worktrees.
// This test creates a real worktree and then cleans up.
func TestRealExecutor_CreateWorktree_Integration(t *testing.T) {
	// Skip if not in a git repo
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

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
	if err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}

	// Verify worktree was created
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Error("Worktree directory was not created")
	}

	// List worktrees and verify our new one exists
	worktrees, err := executor.ListWorktrees()
	if err != nil {
		t.Fatalf("ListWorktrees() error = %v", err)
	}

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
			if wt.Branch != branchName {
				t.Errorf("Worktree branch = %q, want %q", wt.Branch, branchName)
			}
			break
		}
	}
	if !found {
		t.Errorf("New worktree not found in ListWorktrees(). Expected path %q (real: %q), got worktrees: %+v",
			worktreePath, worktreePathReal, worktrees)
	}

	// Remove worktree
	err = executor.RemoveWorktree(worktreePath)
	if err != nil {
		t.Fatalf("RemoveWorktree() error = %v", err)
	}

	// Verify worktree was removed
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Error("Worktree directory still exists after removal")
	}

	// Clean up the test branch
	cmd := exec.Command("git", "branch", "-D", branchName)
	cmd.Dir = cwd
	_ = cmd.Run() // Ignore error - branch may already be deleted
}

// TestRealExecutor_ErrorParsing_BranchConflict tests error detection for branch conflicts.
func TestRealExecutor_ErrorParsing_BranchConflict(t *testing.T) {
	// Test that the error is correctly wrapped
	err := parseGitError("fatal: 'main' is already checked out at '/other/worktree'", errors.New("exit status 128"))
	if !errors.Is(err, ErrBranchAlreadyCheckedOut) {
		t.Errorf("Expected ErrBranchAlreadyCheckedOut, got %v", err)
	}
}

// TestRealExecutor_ErrorParsing_PathExists tests error detection for path conflicts.
func TestRealExecutor_ErrorParsing_PathExists(t *testing.T) {
	err := parseGitError("fatal: '/path/to/worktree' already exists", errors.New("exit status 128"))
	if !errors.Is(err, ErrPathAlreadyExists) {
		t.Errorf("Expected ErrPathAlreadyExists, got %v", err)
	}
}

// TestRealExecutor_ErrorParsing_Locked tests error detection for locked worktrees.
func TestRealExecutor_ErrorParsing_Locked(t *testing.T) {
	err := parseGitError("fatal: '/path/to/worktree' is locked", errors.New("exit status 128"))
	if !errors.Is(err, ErrWorktreeLocked) {
		t.Errorf("Expected ErrWorktreeLocked, got %v", err)
	}
}

// TestInterfaceCompliance verifies RealExecutor implements GitExecutor.
func TestInterfaceCompliance(t *testing.T) {
	var _ GitExecutor = (*RealExecutor)(nil)
}

// TestUnsafeParentDirs tests the unsafe parent directory map.
func TestUnsafeParentDirs(t *testing.T) {
	// These should all be in the unsafe map
	for dir := range unsafeParentDirs {
		if !unsafeParentDirs[dir] {
			t.Errorf("unsafeParentDirs[%q] = false, want true", dir)
		}
	}

	// These should NOT be in the unsafe map
	safeDirs := []string{
		"/Users",
		"/home",
		"/Users/username/projects",
		"/opt",
	}
	for _, dir := range safeDirs {
		if unsafeParentDirs[dir] {
			t.Errorf("unsafeParentDirs[%q] = true, want false", dir)
		}
	}
}
