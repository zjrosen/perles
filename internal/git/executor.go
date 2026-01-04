package git

// BranchInfo holds information about a git branch.
type BranchInfo struct {
	Name      string // Branch name (e.g., "main", "feature/auth")
	IsCurrent bool   // True if this is the currently checked out branch
}

// GitExecutor defines the interface for git worktree operations.
// This abstraction allows for easy testing with mock implementations.
type GitExecutor interface {
	// CreateWorktree creates a new worktree at path with a new branch.
	// newBranch is the name of the new branch to create (e.g., perles-session-abc123).
	// baseBranch is the starting point for the new branch (e.g., main, develop).
	// If baseBranch is empty, uses current HEAD as the starting point.
	CreateWorktree(path, newBranch, baseBranch string) error
	RemoveWorktree(path string) error
	PruneWorktrees() error
	ListWorktrees() ([]WorktreeInfo, error)
	ListBranches() ([]BranchInfo, error)
	BranchExists(name string) bool
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
}

// WorktreeInfo holds information about a git worktree.
type WorktreeInfo struct {
	Path   string
	Branch string
	HEAD   string
}
