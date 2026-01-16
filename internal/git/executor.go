package git

import (
	"context"
	"time"
)

// BranchInfo holds information about a git branch.
type BranchInfo struct {
	Name      string // Branch name (e.g., "main", "feature/auth")
	IsCurrent bool   // True if this is the currently checked out branch
}

// CommitInfo holds information about a git commit.
type CommitInfo struct {
	Hash      string    // Full 40-char SHA
	ShortHash string    // 7-char abbreviated hash
	Subject   string    // First line of commit message
	Author    string    // Author name
	Date      time.Time // Commit timestamp
	IsPushed  bool      // True if commit exists on the remote tracking branch
}

// GitExecutor defines the interface for git worktree operations.
// This abstraction allows for easy testing with mock implementations.
type GitExecutor interface {
	// CreateWorktreeWithContext creates a new worktree at path with a new branch.
	// newBranch is the name of the new branch to create (e.g., perles-session-abc123).
	// baseBranch is the starting point for the new branch (e.g., main, develop).
	// If baseBranch is empty, uses current HEAD as the starting point.
	// Returns ErrWorktreeTimeout if the context deadline is exceeded.
	CreateWorktreeWithContext(ctx context.Context, path, newBranch, baseBranch string) error
	RemoveWorktree(path string) error
	PruneWorktrees() error
	ListWorktrees() ([]WorktreeInfo, error)
	ListBranches() ([]BranchInfo, error)
	BranchExists(name string) bool
	// ValidateBranchName validates a branch name using git check-ref-format --branch.
	// Returns nil if valid, ErrInvalidBranchName if invalid.
	ValidateBranchName(name string) error
	IsGitRepo() bool
	IsWorktree() (bool, error)
	IsBareRepo() (bool, error)
	IsDetachedHead() (bool, error)
	GetCurrentBranch() (string, error)
	GetMainBranch() (string, error)
	IsOnMainBranch() (bool, error)
	GetRepoRoot() (string, error)
	HasUncommittedChanges() (bool, error)
	DetermineWorktreePath(sessionID string) (string, error)

	// Diff operations for viewing git diffs
	// GetDiff returns the unified diff output for the given ref (e.g., "HEAD~1", "main").
	GetDiff(ref string) (string, error)
	// GetDiffStat returns the --numstat output for the given ref.
	GetDiffStat(ref string) (string, error)
	// GetFileDiff returns the diff for a single file against the given ref.
	GetFileDiff(ref, path string) (string, error)
	// GetWorkingDirDiff returns the diff of uncommitted changes (staged + unstaged vs HEAD).
	GetWorkingDirDiff() (string, error)
	// GetUntrackedFiles returns the list of untracked files (new files not yet staged).
	GetUntrackedFiles() ([]string, error)
	// GetCommitDiff returns the diff for a specific commit (what changed in that commit).
	GetCommitDiff(hash string) (string, error)
	// GetFileContent returns the content of a file in the working directory.
	// Used for displaying untracked files that have no diff.
	GetFileContent(path string) (string, error)

	// Commit log operations
	// GetCommitLog returns the most recent commits, up to the specified limit.
	// Returns an empty slice for empty repositories.
	GetCommitLog(limit int) ([]CommitInfo, error)
	// GetCommitLogForRef returns commit history for a specific ref (branch, tag, etc.).
	// If ref is empty, returns commits for HEAD (same behavior as GetCommitLog).
	GetCommitLogForRef(ref string, limit int) ([]CommitInfo, error)

	// Remote operations
	// GetRemoteURL returns the URL for the named remote (e.g., "origin").
	// Returns empty string and nil error if remote doesn't exist.
	GetRemoteURL(name string) (string, error)
}

// WorktreeInfo holds information about a git worktree.
type WorktreeInfo struct {
	Path   string
	Branch string
	HEAD   string
}
