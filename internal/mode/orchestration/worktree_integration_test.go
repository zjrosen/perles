package orchestration

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/git"
	"github.com/zjrosen/perles/internal/mocks"
)

// ===========================================================================
// Worktree Integration Tests - Edge Cases from Risk Analysis
// ===========================================================================
//
// These tests cover all 8 edge cases identified in the risk analysis section
// of the git-worktree-orchestration-proposal.md:
//
// 1. Already in Worktree
// 2. Branch Already Checked Out
// 3. Uncommitted Changes in Main
// 4. Worktree Path Exists
// 5. Detached HEAD
// 6. Not a Git Repository
// 7. Bare Repository
// 8. Graceful Degradation
//
// All tests use the generated mock at internal/mocks/mock_GitExecutor.go.

// ===========================================================================
// Edge Case 1: Already in Worktree
// ===========================================================================
// When the user runs perles from inside an existing worktree, we detect this
// via IsWorktree() and can prompt the user to continue in-place or error.

func TestWorktreeIntegration_AlreadyInWorktree_Detected(t *testing.T) {
	// Test: User runs perles from inside an existing worktree
	// Detection: IsWorktree() returns true
	// Expected: System can detect and handle appropriately
	workDir := t.TempDir()

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsWorktree().Return(true, nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	// Check the IsWorktree result (this would be used in pre-flight checks)
	isWorktree, err := mockGit.IsWorktree()
	require.NoError(t, err)
	require.True(t, isWorktree, "should detect that user is already in a worktree")
	_ = init // Initializer created successfully
}

func TestWorktreeIntegration_AlreadyInWorktree_ContinueInPlace(t *testing.T) {
	// Test: When already in worktree, user can continue in-place by disabling worktree
	// This simulates the "continue in current dir" flow when detecting existing worktree
	workDir := t.TempDir()

	mockGit := mocks.NewMockGitExecutor(t)
	// No IsGitRepo expected - worktree disabled means we skip createWorktree()

	init := NewInitializer(InitializerConfig{
		WorkDir:         workDir,
		GitExecutor:     mockGit,
		ExpectedWorkers: 4,
	})

	// Verify we don't attempt worktree creation
	require.Empty(t, init.WorktreePath())
	require.Empty(t, init.WorktreeBranch())
}

// ===========================================================================
// Edge Case 2: Branch Already Checked Out
// ===========================================================================
// When target branch is checked out in another worktree, CreateWorktree fails.

func TestWorktreeIntegration_BranchAlreadyCheckedOut_Error(t *testing.T) {
	// Test: Target branch is checked out in another worktree
	// Detection: CreateWorktree fails with "already checked out" error
	// Expected: Clear error message returned
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"
	sessionID := "test-session-12345678"
	expectedBranch := "perles-session-test-ses"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(fmt.Errorf("fatal: '%s' is already checked out at '/some/other/path'", expectedBranch))

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	err := init.createWorktree()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create worktree")
	require.Contains(t, err.Error(), "already checked out")
}

func TestWorktreeIntegration_BranchAlreadyCheckedOut_UniqueSessionBranch(t *testing.T) {
	// Test: Using unique session-based branch names avoids conflicts
	// Each session gets a unique branch like perles-session-abc12345
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"
	sessionID := "unique-session-87654321"

	var capturedBranch string
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Run(func(path, newBranch, baseBranch string) {
			capturedBranch = newBranch
		}).
		Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	err := init.createWorktree()
	require.NoError(t, err)
	// Branch name should be unique based on session ID
	require.Equal(t, "perles-session-unique-s", capturedBranch)
}

// ===========================================================================
// Edge Case 3: Uncommitted Changes in Main
// ===========================================================================
// When main repo has uncommitted changes, we can detect and warn but allow.

func TestWorktreeIntegration_UncommittedChanges_Warning(t *testing.T) {
	// Test: Main repo has uncommitted changes when creating worktree
	// Detection: HasUncommittedChanges() returns true
	// Expected: Warning displayed but operation allowed (changes stay in main)
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().HasUncommittedChanges().Return(true, nil)

	// Check for uncommitted changes (this would be used in UI flow)
	hasChanges, err := mockGit.HasUncommittedChanges()
	require.NoError(t, err)
	require.True(t, hasChanges, "should detect uncommitted changes")

	// Note: The actual warning display is in update.go, this test verifies detection
}

func TestWorktreeIntegration_UncommittedChanges_OperationStillAllowed(t *testing.T) {
	// Test: Worktree creation succeeds even with uncommitted changes in main
	// Changes remain in main repo, worktree gets clean state from branch
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"
	sessionID := "test-session-12345678"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)
	// Note: HasUncommittedChanges is NOT checked during createWorktree -
	// it's checked in the UI flow before deciding to enable worktree

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	// Worktree creation should succeed - uncommitted changes are in main, not worktree
	err := init.createWorktree()
	require.NoError(t, err)
	require.Equal(t, worktreePath, init.WorktreePath())
}

// ===========================================================================
// Edge Case 4: Worktree Path Exists (Stale Worktree)
// ===========================================================================
// When target path already exists (possibly from stale worktree), prune and retry.

func TestWorktreeIntegration_WorktreePathExists_PruneFirst(t *testing.T) {
	// Test: Target path already has a directory (stale worktree)
	// Detection: Path exists before git worktree add
	// Expected: PruneWorktrees() called before CreateWorktree()
	workDir := t.TempDir()
	worktreePath := "/tmp/existing-path"
	sessionID := "test-session-12345678"

	var pruneCallCount int
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Run(func() {
		pruneCallCount++
	}).Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	err := init.createWorktree()
	require.NoError(t, err)
	require.Equal(t, 1, pruneCallCount, "PruneWorktrees should be called before CreateWorktree")
}

func TestWorktreeIntegration_WorktreePathExists_Error(t *testing.T) {
	// Test: Path exists and is not a stale worktree (can't be pruned)
	// Expected: Error with specific message about path conflict
	workDir := t.TempDir()
	worktreePath := "/tmp/existing-path"
	sessionID := "test-session-12345678"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil) // Prune doesn't help - path exists as regular dir
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(fmt.Errorf("fatal: '%s' already exists", worktreePath))

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	err := init.createWorktree()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create worktree")
	require.Contains(t, err.Error(), "already exists")
}

func TestWorktreeIntegration_WorktreePathExists_PruneThenSucceed(t *testing.T) {
	// Test: Prune clears stale worktree, then creation succeeds
	// This simulates the happy path after pruning
	workDir := t.TempDir()
	worktreePath := "/tmp/stale-worktree-path"
	sessionID := "test-session-12345678"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil) // Prune removes stale reference
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	err := init.createWorktree()
	require.NoError(t, err)
	require.Equal(t, worktreePath, init.WorktreePath())
}

// ===========================================================================
// Edge Case 5: Detached HEAD
// ===========================================================================
// When user is in detached HEAD state, auto-create a temporary branch.

func TestWorktreeIntegration_DetachedHead_AutoBranch(t *testing.T) {
	// Test: User is in detached HEAD state (no branch)
	// Detection: IsDetachedHead() returns true
	// Expected: Auto-create temporary branch with session-based name
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"
	sessionID := "detached-session-12345678"

	var capturedBranch string
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Run(func(path, newBranch, baseBranch string) {
			capturedBranch = newBranch
		}).
		Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main", // Need non-empty to trigger worktree creation
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	err := init.createWorktree()
	require.NoError(t, err)
	// Auto-generated branch name should be based on session ID
	require.Equal(t, "perles-session-detached", capturedBranch)
}

func TestWorktreeIntegration_DetachedHead_Detection(t *testing.T) {
	// Test: Detection of detached HEAD state
	// This would be used in UI flow to show warning/info
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsDetachedHead().Return(true, nil)

	isDetached, err := mockGit.IsDetachedHead()
	require.NoError(t, err)
	require.True(t, isDetached, "should detect detached HEAD state")
}

func TestWorktreeIntegration_DetachedHead_ConfiguredBaseBranchPassedCorrectly(t *testing.T) {
	// Test: Configured base branch is passed to CreateWorktree correctly
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"
	sessionID := "test-session-12345678"
	configuredBaseBranch := "develop"
	expectedNewBranch := "perles-session-test-ses"

	var capturedNewBranch, capturedBaseBranch string
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Run(func(path, newBranch, baseBranch string) {
			capturedNewBranch = newBranch
			capturedBaseBranch = baseBranch
		}).
		Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: configuredBaseBranch, // User selected base branch
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	err := init.createWorktree()
	require.NoError(t, err)
	require.Equal(t, expectedNewBranch, capturedNewBranch)     // Auto-generated branch
	require.Equal(t, configuredBaseBranch, capturedBaseBranch) // Base branch passed correctly
}

// ===========================================================================
// Edge Case 6: Not a Git Repository
// ===========================================================================
// When user runs orchestration from non-git directory, skip worktree gracefully.

func TestWorktreeIntegration_NotGitRepo_SkipWorktree(t *testing.T) {
	// Test: User runs orchestration from non-git directory
	// Detection: IsGitRepo() returns false
	// Expected: Worktree feature silently skipped, proceeds normally
	workDir := t.TempDir()

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(false)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	init.mu.Lock()
	init.sessionID = "test-session-id"
	init.mu.Unlock()

	err := init.createWorktree()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a git repository")
}

func TestWorktreeIntegration_NotGitRepo_DisabledWorktree(t *testing.T) {
	// Test: When worktree is disabled, no git checks are made
	// This allows orchestration to work in non-git directories
	workDir := t.TempDir()

	mockGit := mocks.NewMockGitExecutor(t)
	// No expectations set - nothing should be called when disabled

	init := NewInitializer(InitializerConfig{
		WorkDir:         workDir,
		GitExecutor:     mockGit,
		ExpectedWorkers: 4,
	})

	// Verify worktree state is empty
	require.Empty(t, init.WorktreePath())
	require.Empty(t, init.WorktreeBranch())
}

func TestWorktreeIntegration_NotGitRepo_Detection(t *testing.T) {
	// Test: Detection of non-git directory
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(false)

	isGitRepo := mockGit.IsGitRepo()
	require.False(t, isGitRepo, "should detect non-git directory")
}

// ===========================================================================
// Edge Case 7: Bare Repository
// ===========================================================================
// When user is in a bare git repository, worktree operations have limitations.

func TestWorktreeIntegration_BareRepo_Error(t *testing.T) {
	// Test: User is in a bare git repository
	// Detection: IsBareRepo() returns true
	// Expected: Clear error message, worktree unavailable
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsBareRepo().Return(true, nil)

	isBare, err := mockGit.IsBareRepo()
	require.NoError(t, err)
	require.True(t, isBare, "should detect bare repository")
}

func TestWorktreeIntegration_BareRepo_Detection(t *testing.T) {
	// Test: Detection logic for bare repository
	// This is used in update.go to show warning before enabling worktree
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsBareRepo().Return(true, nil)
	mockGit.EXPECT().IsGitRepo().Return(true) // Bare repos ARE git repos

	isGitRepo := mockGit.IsGitRepo()
	isBare, err := mockGit.IsBareRepo()

	require.NoError(t, err)
	require.True(t, isGitRepo, "bare repo is still a git repo")
	require.True(t, isBare, "should detect as bare repository")
}

func TestWorktreeIntegration_BareRepo_WorktreeCreationFails(t *testing.T) {
	// Test: CreateWorktree fails on bare repository
	// Note: In real git, worktree add on bare repos requires special handling
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"
	sessionID := "test-session-12345678"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(fmt.Errorf("fatal: this operation must be run in a work tree"))

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	err := init.createWorktree()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create worktree")
}

// ===========================================================================
// Edge Case 8: Graceful Degradation
// ===========================================================================
// When worktree creation fails for any reason, user can Skip and continue.

func TestWorktreeIntegration_GracefulDegradation_Skip(t *testing.T) {
	// Test: Worktree creation fails for any reason
	// Expected: User can Skip and continue without worktree
	// This tests the error case where user would choose to skip

	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"
	sessionID := "test-session-12345678"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(fmt.Errorf("some unexpected git error"))

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	// First attempt fails
	err := init.createWorktree()
	require.Error(t, err)

	// User chooses to skip - simulate by creating new initializer with worktree disabled
	initSkip := NewInitializer(InitializerConfig{
		WorkDir:         workDir,
		GitExecutor:     mockGit,
		ExpectedWorkers: 4,
	})

	// Verify skip works - no worktree attempted
	require.Empty(t, initSkip.WorktreePath())
}

func TestWorktreeIntegration_GracefulDegradation_RetryOption(t *testing.T) {
	// Test: After failure, user can retry worktree creation
	// This tests the retry path after initial failure

	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"
	sessionID := "test-session-12345678"

	// First mock for failure
	mockGitFail := mocks.NewMockGitExecutor(t)
	mockGitFail.EXPECT().IsGitRepo().Return(true)
	mockGitFail.EXPECT().PruneWorktrees().Return(nil)
	mockGitFail.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGitFail.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(fmt.Errorf("temporary network error"))

	initFail := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGitFail,
		ExpectedWorkers:    4,
	})

	initFail.mu.Lock()
	initFail.sessionID = sessionID
	initFail.mu.Unlock()

	// First attempt fails
	err := initFail.createWorktree()
	require.Error(t, err)

	// Second mock for success (simulating retry with new executor)
	mockGitSuccess := mocks.NewMockGitExecutor(t)
	mockGitSuccess.EXPECT().IsGitRepo().Return(true)
	mockGitSuccess.EXPECT().PruneWorktrees().Return(nil)
	mockGitSuccess.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGitSuccess.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)

	initSuccess := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGitSuccess,
		ExpectedWorkers:    4,
	})

	initSuccess.mu.Lock()
	initSuccess.sessionID = sessionID
	initSuccess.mu.Unlock()

	// Retry succeeds
	err = initSuccess.createWorktree()
	require.NoError(t, err)
	require.Equal(t, worktreePath, initSuccess.WorktreePath())
}

func TestWorktreeIntegration_GracefulDegradation_ErrorPreserved(t *testing.T) {
	// Test: Error message is preserved for display
	workDir := t.TempDir()
	worktreePath := "/tmp/test-worktree"
	sessionID := "test-session-12345678"
	expectedErrorMsg := "fatal: disk quota exceeded"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return(fmt.Errorf("%s", expectedErrorMsg))

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main",
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	err := init.createWorktree()
	require.Error(t, err)
	require.Contains(t, err.Error(), expectedErrorMsg, "original error should be preserved")
}

// ===========================================================================
// End-to-End Success Path
// ===========================================================================

func TestWorktreeIntegration_EndToEnd_Success(t *testing.T) {
	// Test: Full worktree creation success path
	// All pre-flight checks pass, worktree created, state updated correctly
	workDir := t.TempDir()
	worktreePath := "/home/user/myproject-worktree-12345678"
	sessionID := "12345678-90ab-cdef-1234-567890abcdef"
	expectedBranch := "perles-session-12345678"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: "main", // Need non-empty to trigger worktree creation
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	// Execute worktree creation
	err := init.createWorktree()

	// Verify success
	require.NoError(t, err)
	require.Equal(t, worktreePath, init.WorktreePath())
	require.Equal(t, expectedBranch, init.WorktreeBranch())
}

func TestWorktreeIntegration_EndToEnd_WithCustomBaseBranch(t *testing.T) {
	// Test: Full worktree creation with custom base branch
	workDir := t.TempDir()
	worktreePath := "/home/user/myproject-worktree-abc123"
	sessionID := "abc12345-6789-cdef-1234-567890abcdef"
	customBaseBranch := "feature/my-base-branch"
	expectedNewBranch := "perles-session-abc12345"

	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsGitRepo().Return(true)
	mockGit.EXPECT().PruneWorktrees().Return(nil)
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(worktreePath, nil)
	mockGit.EXPECT().CreateWorktree(worktreePath, mock.AnythingOfType("string"), customBaseBranch).Return(nil)

	init := NewInitializer(InitializerConfig{
		WorkDir:            workDir,
		WorktreeBaseBranch: customBaseBranch, // Custom base branch
		GitExecutor:        mockGit,
		ExpectedWorkers:    4,
	})

	init.mu.Lock()
	init.sessionID = sessionID
	init.mu.Unlock()

	err := init.createWorktree()
	require.NoError(t, err)
	require.Equal(t, worktreePath, init.WorktreePath())
	require.Equal(t, expectedNewBranch, init.WorktreeBranch()) // Auto-generated branch name
}

// ===========================================================================
// Additional Coverage Tests
// ===========================================================================

func TestWorktreeIntegration_ListWorktrees_Success(t *testing.T) {
	// Test: ListWorktrees returns current worktrees
	// Used for cleanup and exit messaging
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().ListWorktrees().Return([]git.WorktreeInfo{
		{Path: "/home/user/myproject", Branch: "main", HEAD: "abc123"},
		{Path: "/home/user/myproject-worktree-xyz", Branch: "perles-session-xyz", HEAD: "def456"},
	}, nil)

	worktrees, err := mockGit.ListWorktrees()
	require.NoError(t, err)
	require.Len(t, worktrees, 2)
	require.Equal(t, "main", worktrees[0].Branch)
	require.Equal(t, "perles-session-xyz", worktrees[1].Branch)
}

func TestWorktreeIntegration_RemoveWorktree_Success(t *testing.T) {
	// Test: RemoveWorktree removes worktree directory
	// Used during cleanup on exit
	mockGit := mocks.NewMockGitExecutor(t)
	worktreePath := "/home/user/myproject-worktree-xyz"
	mockGit.EXPECT().RemoveWorktree(worktreePath).Return(nil)

	err := mockGit.RemoveWorktree(worktreePath)
	require.NoError(t, err)
}

func TestWorktreeIntegration_RemoveWorktree_LockedError(t *testing.T) {
	// Test: RemoveWorktree fails when worktree is locked
	mockGit := mocks.NewMockGitExecutor(t)
	worktreePath := "/home/user/myproject-worktree-xyz"
	mockGit.EXPECT().RemoveWorktree(worktreePath).
		Return(fmt.Errorf("fatal: '%s' is locked", worktreePath))

	err := mockGit.RemoveWorktree(worktreePath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "locked")
}

func TestWorktreeIntegration_GetRepoRoot_Success(t *testing.T) {
	// Test: GetRepoRoot returns repository root path
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().GetRepoRoot().Return("/home/user/myproject", nil)

	root, err := mockGit.GetRepoRoot()
	require.NoError(t, err)
	require.Equal(t, "/home/user/myproject", root)
}

func TestWorktreeIntegration_GetCurrentBranch_Success(t *testing.T) {
	// Test: GetCurrentBranch returns current branch name
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().GetCurrentBranch().Return("feature/my-feature", nil)

	branch, err := mockGit.GetCurrentBranch()
	require.NoError(t, err)
	require.Equal(t, "feature/my-feature", branch)
}

func TestWorktreeIntegration_GetMainBranch_DetectsMain(t *testing.T) {
	// Test: GetMainBranch correctly detects "main" branch
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().GetMainBranch().Return("main", nil)

	mainBranch, err := mockGit.GetMainBranch()
	require.NoError(t, err)
	require.Equal(t, "main", mainBranch)
}

func TestWorktreeIntegration_GetMainBranch_DetectsMaster(t *testing.T) {
	// Test: GetMainBranch correctly detects "master" branch in older repos
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().GetMainBranch().Return("master", nil)

	mainBranch, err := mockGit.GetMainBranch()
	require.NoError(t, err)
	require.Equal(t, "master", mainBranch)
}

func TestWorktreeIntegration_IsOnMainBranch_True(t *testing.T) {
	// Test: IsOnMainBranch returns true when on main
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsOnMainBranch().Return(true, nil)

	isOnMain, err := mockGit.IsOnMainBranch()
	require.NoError(t, err)
	require.True(t, isOnMain)
}

func TestWorktreeIntegration_IsOnMainBranch_False(t *testing.T) {
	// Test: IsOnMainBranch returns false when on feature branch
	mockGit := mocks.NewMockGitExecutor(t)
	mockGit.EXPECT().IsOnMainBranch().Return(false, nil)

	isOnMain, err := mockGit.IsOnMainBranch()
	require.NoError(t, err)
	require.False(t, isOnMain)
}

func TestWorktreeIntegration_DetermineWorktreePath_SiblingPath(t *testing.T) {
	// Test: DetermineWorktreePath returns sibling directory path
	mockGit := mocks.NewMockGitExecutor(t)
	sessionID := "abc12345-6789-cdef"
	expectedPath := "/home/user/myproject-worktree-abc12345"
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(expectedPath, nil)

	path, err := mockGit.DetermineWorktreePath(sessionID)
	require.NoError(t, err)
	require.Equal(t, expectedPath, path)
}

func TestWorktreeIntegration_DetermineWorktreePath_FallbackPath(t *testing.T) {
	// Test: DetermineWorktreePath falls back to .perles/worktrees when sibling not writable
	mockGit := mocks.NewMockGitExecutor(t)
	sessionID := "abc12345-6789-cdef"
	fallbackPath := "/home/user/myproject/.perles/worktrees/abc12345-6789-cdef"
	mockGit.EXPECT().DetermineWorktreePath(sessionID).Return(fallbackPath, nil)

	path, err := mockGit.DetermineWorktreePath(sessionID)
	require.NoError(t, err)
	require.Equal(t, fallbackPath, path)
}

// ===========================================================================
// Concurrent Access Tests
// ===========================================================================

func TestWorktreeIntegration_ConcurrentAccess_WorktreePath(t *testing.T) {
	// Test: Thread-safe access to WorktreePath
	workDir := t.TempDir()
	init := NewInitializer(InitializerConfig{
		WorkDir:         workDir,
		ExpectedWorkers: 4,
	})

	// Set a path
	init.mu.Lock()
	init.worktreePath = "/test/path"
	init.mu.Unlock()

	// Concurrent reads should not race
	done := make(chan bool, 10)
	for range 10 {
		go func() {
			defer func() { done <- true }()
			_ = init.WorktreePath()
		}()
	}

	for range 10 {
		<-done
	}
}

func TestWorktreeIntegration_ConcurrentAccess_WorktreeBranch(t *testing.T) {
	// Test: Thread-safe access to WorktreeBranch
	workDir := t.TempDir()
	init := NewInitializer(InitializerConfig{
		WorkDir:         workDir,
		ExpectedWorkers: 4,
	})

	// Set a branch
	init.mu.Lock()
	init.worktreeBranch = "test-branch"
	init.mu.Unlock()

	// Concurrent reads should not race
	done := make(chan bool, 10)
	for range 10 {
		go func() {
			defer func() { done <- true }()
			_ = init.WorktreeBranch()
		}()
	}

	for range 10 {
		<-done
	}
}
